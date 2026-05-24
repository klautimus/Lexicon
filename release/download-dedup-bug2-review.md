# BUG-2 Fix Review: searchEnqueue wrong trackID in dedup path

**Date:** 2026-05-22
**Reviewer:** Atlas (ops)
**Task:** t_3b584e7e
**Parent Fix:** t_b9f0d457
**Verdict:** ✅ APPROVED with observations

---

## Review Summary

The BUG-2 fix is correct and complete. Both sub-issues are properly addressed: when the user already owns a track, their own track ID is resolved (not the cross-user result); when shareTrack fails, TrackID=0 prevents a bad job from being created and the request falls through to normal download. All four scenarios (own, no-own→share, share-fail, unauthenticated) are handled correctly. Build passes clean. One non-blocking efficiency observation about redundant DB queries.

---

## Checklist

| # | Check | Result |
|---|-------|--------|
| 1 | User already owning track → requesting user's own track ID used? | ✅ PASS |
| 2 | shareTrack failure → TrackID=0 (not another user's ID)? | ✅ PASS |
| 3 | Happy paths (no-own→share, own→skip-share) working? | ✅ PASS |
| 4 | `go build ./internal/...` passes? | ✅ PASS |
| 5 | Path lookup for own-track efficient (no N+1 queries)? | ⚠️ OBS |

---

## Detailed Review

### 1. User-owns → own track ID ✅

**Code:** `downloader.go:509-521`

When `userOwnsTrack(ctx, uid, trackID)` returns true:
1. Looks up the source path of the found track (`SELECT path FROM tracks WHERE id=?`)
2. Queries for the requesting user's own track at that path (`SELECT id FROM tracks WHERE user_id=? AND path=? LIMIT 1`)
3. If found: `sharedTrackID = ownID`, `dedupSourceID = 0` — the job gets the user's OWN track ID
4. If own-track lookup fails (userOwnsTrack confirmed ownership but path query somehow fails): falls back to `trackID` from `findLibraryTrack` — this is an intentional safety net for an extremely unlikely DB error, documented in the parent task metadata

**Verdict:** Correct. The requesting user's track ID is used when they already own the track.

### 2. shareTrack failure → TrackID=0 ✅

**Code:** `downloader.go:524-527, 535-572`

When `shareTrack` returns an error:
1. `sharedTrackID = 0`, `dedupSourceID = 0` ← explicitly zeroed
2. The `if sharedTrackID > 0` guard at line 535 is NOT entered — no job created with wrong TrackID
3. Falls through to line 572 `// shareTrack failed — fall through to normal download`
4. Normal download path creates a fresh job with `TrackID: 0` (unset)

**Verdict:** Correct. No job is created with another user's track ID on shareTrack failure. The 0-valued TrackID cleanly falls through to normal download.

### 3. Happy paths ✅

**No-own → share (lines 522-531):**
- `!userOwnsTrack` → calls `shareTrack(ctx, trackID, uid)`
- Success: `sharedTrackID = newID` → job created with correct shared track ID
- The shared track is a new row with the requesting user's `user_id`, so TrackID points to their own record

**Own → skip share (lines 509-521):**
- `userOwnsTrack` → own-track lookup sets `sharedTrackID = ownID`
- `shareTrack` is never called — no duplicate rows, no wasted work

**Unauthenticated (line 508):**
- `uid > 0` guard skips all ownership/sharing logic
- Falls through with `sharedTrackID = trackID` (findLibraryTrack result)
- Dedup job still fires — correct for unauthenticated use

**Confidence gating (line 500):**
- Only enters dedup when `confidence <= matchPrefix` (matchExact=1 or matchPrefix=2)
- FTS and LIKE matches are excluded — no false-positive auto-shares

**Verdict:** All paths correct and complete.

### 4. Build ✅

```
go build ./internal/... → exit 0, no errors
```

### 5. Query efficiency ⚠️ OBS

**Not N+1**, but there are redundant queries in the user-owns path:

```
userOwnsTrack (lines 2163-2182):
  Query 1: SELECT path FROM tracks WHERE id=?
  Query 2: SELECT COUNT(*) FROM tracks WHERE user_id=? AND path=?

Owning branch (lines 512-517):
  Query 3: SELECT path FROM tracks WHERE id=?       ← redundant with Query 1
  Query 4: SELECT id FROM tracks WHERE user_id=? AND path=? LIMIT 1  ← same as Query 2 but returns id
```

4 queries where 2 would suffice. `userOwnsTrack` already retrieves the path, but it's not passed back to the caller. The optimal approach would be to either:

- Skip `userOwnsTrack` and do the own-track lookup inline (2 queries total)
- Or refactor `userOwnsTrack` to return the path and own-track ID

**Impact:** Low. This code path only triggers when a user requests a track they already own via cross-user dedup — a rare edge case. Not a correctness issue, not blocking.

---

## Non-Blocking Observations

### OBS-1: confidence=0 sentinel
`findLibraryTrack` returns `(0, 0, sql.ErrNoRows)` on miss (line 474). The `MatchConfidence` type has no `matchNone = 0` constant defined. The 0-value is implicitly "no match" — consider adding an explicit constant for clarity.

---

## Findings Summary

| # | Finding | Severity | Status |
|---|---------|----------|--------|
| 1 | BUG-2a: own-track ID resolution | — | Fixed ✅ |
| 2 | BUG-2b: shareTrack failure TrackID=0 | — | Fixed ✅ |
| 3 | Redundant DB queries in user-owns path | Low | Observation |
| 4 | Missing matchNone=0 sentinel constant | Low | Observation |

No new bugs found. No fix tasks required.
