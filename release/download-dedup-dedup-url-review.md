# Download Dedup URL — Review Report

**Date:** 2026-05-22
**Reviewer:** Atlas (ops)
**Task:** t_ad061158
**Parent Implementation:** t_c187994a

---

## Review Summary

The dedup-url implementation is functional at a structural level — schema migration works, `shareTrack()` correctly creates per-user track records, and `searchEnqueue()` performs pre-download dedup. However, several correctness issues were found: the post-hoc dedup in `enqueue()` misses files in subdirectories (the common case for SpotiFLAC), `searchEnqueue` returns wrong track IDs in two scenarios, and there's no match-confidence gating. **4 bugs filed for fix.**

---

## Review Item Checklist

| # | Check | Result |
|---|-------|--------|
| 1 | Does dedup check search ALL users' tracks? | ✅ PASS |
| 2 | When duplicate found, is new track record created pointing to existing file_path? | ✅ PASS (shareTrack) |
| 3 | Is requesting user's user_id set correctly? | ✅ PASS |
| 4 | Are there race conditions? | ⚠️ PARTIAL — concurrent shareTrack INSERT can fail; mitigated by UNIQUE index |
| 5 | Is error handling complete? | ⚠️ PARTIAL — 2 bugs found (trackID misuse on errors) |
| 6 | Does `go build ./internal/...` pass? | ✅ PASS |
| 7 | Are there edge cases not handled? | ❌ FAIL — 4 issues found |

---

## Findings

### BUG-1: `dedupRunOutput` misses files in subdirectories (CRITICAL)

**Location:** `downloader.go:1990-2065`, function `dedupRunOutput()`

**Problem:** `dedupRunOutput` uses `os.ReadDir(outputDir)` which only reads the **top-level** directory. SpotiFLAC with default `-folder-format` creates files in `Artist/Album/track.flac` subdirectory structure. These files are never checked by the dedup.

```go
// Line 1999 — only reads immediate children, not recursive
entries, err := os.ReadDir(outputDir)
```

**Impact:** The post-hoc dedup for `enqueue()` (Spotify URL downloads) is effectively broken. For any non-empty `-folder-format`, no files are found and no sharing occurs. The scanner creates new track records for each user, and the second user's download is wasted bandwidth.

**Fix:** Replace `os.ReadDir` with `filepath.Walk` (or `filepath.WalkDir`) to recurse into subdirectories. The file path building at line 2020 (`filepath.Join(outputDir, entry.Name())`) should use relative paths from Walk instead.

**Files:** `backend/internal/downloader/downloader.go`

---

### BUG-2: Wrong `TrackID` when user already owns the track (HIGH)

**Location:** `downloader.go:652-660`, function `searchEnqueue()`

**Problem:** When `userOwnsTrack()` returns `true` (user B already has a track for the file path found via user A's track), the `!userOwnsTrack` condition is false, so `shareTrack` is skipped. But `trackID` stays as the result of `findLibraryTrack()` — which is **user A's track ID**, not user B's.

```go
// trackID = 5 (user A's track)
if !a.userOwnsTrack(r.Context(), userID, trackID) {  // false — user B already owns
    // shareTrack is skipped
}
// trackID is still 5 — wrong! It should be user B's track ID (e.g. 12)
job := &Job{..., TrackID: trackID, ...}
```

**Impact:** The job's `TrackID` points to another user's track record. The frontend uses this for playlist addition, playback, etc. — operations that would operate on the wrong user's data. In a multi-user setup, this is a data integrity issue.

**Fix:** After `userOwnsTrack` returns true, look up the requesting user's own track ID for the found file path:
```go
if a.userOwnsTrack(ctx, userID, trackID) {
    // Look up user's own track ID for this path
    var ownID int64
    var sourcePath string
    a.db.QueryRowContext(ctx, `SELECT path FROM tracks WHERE id=?`, trackID).Scan(&sourcePath)
    a.db.QueryRowContext(ctx, `SELECT id FROM tracks WHERE user_id=? AND path=? LIMIT 1`,
        userID, sourcePath).Scan(&ownID)
    if ownID > 0 {
        trackID = ownID
    }
}
```

**Files:** `backend/internal/downloader/downloader.go`

---

### BUG-3: No confidence levels in `findLibraryTrack` (HIGH)

**Location:** `downloader.go:421-477`, function `findLibraryTrack()`

**Problem:** The plan (section 3.3) explicitly recommends only auto-sharing for high-confidence matches (strategies 1a, 1b — exact and prefix artist+title). Strategies 3 (FTS5) and 4 (LIKE) can produce false positives:

- User searches "Beatles - Hey Jude"
- LIKE strategy matches "Beatles - Hey Bulldog" from another user's library
- Wrong track gets shared

The implementation treats all four strategies identically — any match triggers auto-sharing.

**Impact:** False-positive track shares between users. User B gets linked to a track they didn't request. In multi-user setups, this pollutes user B's library with incorrect tracks.

**Fix:** Add confidence levels to `findLibraryTrack()` return value. Only auto-share for `matchExact` (strategy 1a) and `matchPrefix` (strategy 1b). FTS5 and LIKE matches should still be returned (for fallback display) but should NOT trigger auto-sharing in `searchEnqueue`.

```go
type MatchConfidence int
const (
    matchExact  MatchConfidence = 1
    matchPrefix MatchConfidence = 2
    matchFTS    MatchConfidence = 3
    matchLike   MatchConfidence = 4
)

func (a *API) findLibraryTrack(ctx context.Context, query string) (int64, MatchConfidence, error) {
    // ... return confidence alongside trackID
}

// In searchEnqueue:
trackID, confidence, err := a.findLibraryTrack(...)
if err == nil && trackID > 0 {
    if confidence <= matchPrefix {  // only auto-share for exact/prefix
        // ... share logic
    } else {
        // Low confidence — don't auto-share, suggest to user instead
    }
}
```

**Files:** `backend/internal/downloader/downloader.go`

---

### BUG-4: Dead code — `ensureTrackOwnership` + `track_owners` table (HIGH)

**Location:** 
- `downloader.go:520-536`, function `ensureTrackOwnership()`
- `db.go:209-217`, `track_owners` table definition
- `db.go:255`, `validTables` entry

**Problem:** `ensureTrackOwnership()` is defined but **never called** anywhere in the codebase. The `track_owners` table is in the schema but is never populated. The parent implementation (t_c187994a) explicitly chose the `shareTrack()` approach (new `tracks` rows per user) over the `track_owners` junction-table approach, but left both in the code.

**Impact:** 
- Unnecessary schema weight: the `track_owners` table is created on every fresh install but never used
- Code confusion: future developers may wonder which approach is active
- 30+ lines of dead code in the downloader

**Fix:** Remove `ensureTrackOwnership()` from downloader.go and `track_owners` table from db.go schema + validTables. The active approach is `shareTrack()` (per-user `tracks` rows with `UNIQUE(user_id, path)`).

**Files:** `backend/internal/downloader/downloader.go`, `backend/internal/db/db.go`

---

## Non-Blocking Observations

### OBS-1: `shareTrack` concurrent INSERT race
Two goroutines simultaneously calling `shareTrack(ctx, 5, 2)` would both check for existing record (none found), then both INSERT. The second INSERT hits the UNIQUE constraint on `idx_tracks_user_path`. This is handled gracefully (error logged, job proceeds) but in `searchEnqueue` the fallback uses the wrong trackID (see BUG-2). Using `INSERT OR IGNORE` would be more robust but is not critical — the UNIQUE index already prevents corruption.

### OBS-2: `dedupRunOutput` DedupSourceTrackID = -1
Line 2060 sets `DedupSourceTrackID = -1` which is inconsistent with `searchEnqueue` (uses actual trackID). The -1 sentinel has no clear semantic meaning. Consider using 0 (no source) or tracking the actual source track.

### OBS-3: No pendingDownloads map
The plan (section 10.2) recommends an in-memory `pendingDownloads` map to prevent simultaneous downloads of the same query. Not implemented. Low priority — the worst case is wasted bandwidth, not data corruption.

### OBS-4: No `mode` column on `download_jobs`
The `Job.Mode` field (`"url"` / `"search"`) is not persisted to the database. Jobs loaded from DB on restart lose their mode. Low priority — jobs are ephemeral.

---

## Fix Task Plan

| # | Task | Severity | Fix |
|---|------|----------|-----|
| BUG-1 | `dedupRunOutput` subdirectory walk | CRITICAL | Use `filepath.Walk` instead of `os.ReadDir` |
| BUG-2 | Wrong TrackID on user-owns/hare-fail | HIGH | Look up user's own track ID for matched path |
| BUG-3 | No confidence levels in findLibraryTrack | HIGH | Add MatchConfidence return, gate sharing on >= Prefix |
| BUG-4 | Dead code: ensureTrackOwnership + track_owners | HIGH | Remove unused function + table + validTables entry |

---

## Build Verification

```
$ cd /mnt/c/Users/kevin/CascadeProjects/lexicon/backend && go build ./internal/...
(exit 0, no output — clean build)
```
