# Dedup-Upgrade Implementation Review

**Date:** 2026-05-22
**Reviewer:** Atlas
**Reviewed:** `findCrossUserDedup()` + dedup logic in `upgradeTrack()` (downloader.go lines 1727-1885)

---

## Build Verification

```
go build ./internal/... → exit 0, clean (no errors)
```

✅ **Passes.**

---

## Review Checklist

### 1. Does the dedup check search ALL users' tracks?

✅ **YES.** The SQL query in `findCrossUserDedup()` (line 1747) has NO `user_id` filter:

```sql
SELECT path, mime, size_bytes, IFNULL(user_id, 0)
FROM tracks
WHERE LOWER(title)=LOWER(?) AND LOWER(artist)=LOWER(?)
  AND media_kind='music' AND id != ?
  AND (LOWER(path) LIKE '%.opus' OR LOWER(path) LIKE '%.m4a' ...)
ORDER BY ... LIMIT 1
```

Cross-user by design. NULL user_id handled via `IFNULL(user_id, 0)`.

### 2. When a duplicate is found, is a new track record created pointing to the existing file_path?

⚠️ **DIFFERENT APPROACH — correct for this context.** The implementation UPDATEs the requesting user's EXISTING track record to point to the shared file:

```go
UPDATE tracks SET path=?, mime=?, size_bytes=?, mtime=? WHERE id=?
```

This is correct for `upgradeTrack()` — the user already has a track record being upgraded from lower-quality to higher-quality. No new record needed.

**Note:** The broader plan envisions `shareTrack()` creating NEW records for the `searchEnqueue()` path, but that's out of scope for this implementation.

### 3. Is the requesting user's user_id set correctly?

✅ **YES.** `getUserID(r)` (line 1871) — wraps `auth.UserFromContext(r.Context())` — correctly used in the `download_jobs` INSERT. The dedup check itself is user-agnostic (searches ALL users), and the UPDATE targets the requesting user's track ID.

### 4. Are there race conditions?

⚠️ **Two identified:**

| # | Gap | Severity | Details |
|---|-----|----------|---------|
| R1 | TOCTOU: os.Stat→UPDATE | Low | File verified at line 1781, but could be deleted by original owner before UPDATE at line 1855. Result: requesting user's track points to deleted file. Mitigated by scanner's skip-missing-files behavior. |
| R2 | Old file deleted without sharing check | Medium | Line 1847: `os.Remove(oldPath)` runs unconditionally when dedup succeeds. If other users share tracks pointing to THIS user's old (lower-quality) file, their tracks now dangle. See plan section 10.1. |

**R2 is the more serious gap.** The implementation treats the old file as exclusive to the requesting user, without checking `COUNT(*) FROM tracks WHERE path=? AND user_id != ?`.

### 5. Is error handling complete?

⚠️ **Mostly complete, one gap.**

| Check | Status | Notes |
|-------|--------|-------|
| `findCrossUserDedup` query error | ✅ | Logged, falls through to normal download |
| `dedup == nil` (no match) | ✅ | Falls through to normal download |
| DB UPDATE error after dedup hit | ✅ | Logged, returns 500 |
| `os.Remove` old file error | ✅ | Best-effort, logged |
| `download_jobs` INSERT error | ⚠️ | `_, _ =` — silently swallowed (consistent with other job inserts) |
| **`rows.Err()` after iteration** | ❌ | **BUG — missing after `rows.Scan()`** |

**B1: Missing `rows.Err()` check in `findCrossUserDedup()` (lines 1770-1777):**

```go
if !rows.Next() {
    return nil, nil        // ← if Next() returns false due to iteration error,
}                           //   not just no rows, we silently return nil
var m dedupMatch
if err := rows.Scan(...); err != nil {
    return nil, err
}
// MISSING: rows.Err() check
```

For a `LIMIT 1` query, the probability of an iteration error is low. But it's a gap — if the query returns one row successfully scanned, then encounters an error on the next Next() call, `rows.Err()` would be non-nil but unchecked. The function would incorrectly return a valid match instead of the error.

### 6. Does go build ./internal/... pass?

✅ **YES.** Exit code 0, no errors.

### 7. Are there edge cases not handled?

| # | Edge Case | Status | Plan Ref |
|---|-----------|--------|----------|
| E1 | `rows.Err()` not checked | ❌ Not handled | N/A (found in review) |
| E2 | Old file deleted with other sharers | ❌ Not handled | Plan §10.1 |
| E3 | TOCTOU race os.Stat→UPDATE | ⚠️ Known limitation | Plan §10.2 |
| E4 | Dedup only in upgradeTrack | ⚠️ Scope limitation | Plan phases 1-2 not yet done |
| E5 | NULL user_id tracks | ✅ Handled (`IFNULL(user_id, 0)`) | — |
| E6 | Self-match (same track) | ✅ Excluded (`AND id != ?`) | — |
| E7 | File deleted on disk since indexing | ✅ Checked (`os.Stat`) | — |
| E8 | No high-quality format available | ✅ Returns nil → normal download | — |
| E9 | Format preference (opus>m4a>webm>mp4) | ✅ Explicit ORDER BY CASE | §10.4 |
| E10 | Concurrent upgrades | ⚠️ Not handled | Two users upgrade same track simultaneously → both try to delete old file, both UPDATE to dedup path |

---

## Summary

**Strengths:**
- Clean, focused implementation — one function (`findCrossUserDedup`) ≈ 45 lines
- Cross-user search is correct — no user_id filter + IFNULL for NULL users
- File existence verified with `os.Stat` before sharing
- Quality prioritization with explicit ORDER BY CASE
- Error handling gracefully falls through to normal download on failure
- Build passes clean

**Issues found: 3**

| ID | Severity | Description | Action |
|----|----------|-------------|--------|
| B1 | Minor | Missing `rows.Err()` after `rows.Scan()` in `findCrossUserDedup` | Fix |
| R2 | Medium | Old file deleted without checking other users' tracks | Fix |
| R1 | Low | TOCTOU gap between os.Stat and UPDATE | Accept (low-probability, scanner-resilient) |

**Not in scope (by design):** searchEnqueue/enqueue dedup, shareTrack(), schema migration — these are Phase 1-2 items from the plan not yet implemented.

---

## Recommendation

Create 2 fix tasks:

1. **Fix B1 + R2** in a single pass (both in same ~20 line region of downloader.go)
2. **Review the fixes** (per task instructions: "If bugs found, CREATE fix tasks with review children")

After fixes, the implementation is production-ready for the upgradeTrack dedup feature.
