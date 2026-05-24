# Podcast Download Dedup — Code Review

**Reviewer:** Atlas (kanban task t_0aaea475)
**Date:** 2026-05-22
**Reviewed:** `backend/internal/podcaster/podcaster.go` — dedup implementation
**Plan reference:** `/mnt/c/Users/kevin/CascadeProjects/lexicon/release/download-dedup-plan.md`

---

## Summary

The podcast dedup implementation adds meaningful cross-user awareness — `checkEpisodeDedup()` correctly searches ALL users by GUID and avoids redundant downloads. However, the implementation stops halfway: it reuses existing file paths and marks episodes as downloaded, but **never creates track records for the requesting user**, making dedup'd episodes unplayable. Additionally, a build-breaking unused import, multiple error-handling gaps, and an extension mismatch in `findLatestAudioFile` need fixing.

**Verdict: ❌ NOT READY — 7 issues found (1 critical, 2 high, 1 medium, 3 low). Fix tasks created.**

---

## Review Checklist

| # | Question | Result | Details |
|---|----------|--------|---------|
| 1 | Does dedup check search ALL users' tracks? | ✅ PASS | `checkEpisodeDedup` matches on GUID across all `podcast_episodes` — no user_id or feed_id filter |
| 2 | New track record created for requesting user? | ❌ FAIL | Only `podcast_episodes` updated; no `tracks` row created. `userID` param is dead code |
| 3 | Requesting user's user_id set correctly? | ❌ N/A | `userID` passed but never used in function body |
| 4 | Are there race conditions? | ⚠️ MINOR | TOCTOU between GUID query and duplicate search — benign |
| 5 | Is error handling complete? | ❌ FAIL | Multiple gaps (see Issues H1, H2, M1) |
| 6 | Does `go build ./internal/...` pass? | ❌ FAIL | Unused `models` import in downloader.go:33 |
| 7 | Are edge cases not handled? | ❌ FAIL | Extension gap in `findLatestAudioFile` (see Issue L1) |

---

## Issues Found

### C1 — Build Failure: Unused Import (CRITICAL)

**File:** `backend/internal/downloader/downloader.go:33`
```go
"github.com/kevin/lexicon/internal/models"  // imported and not used
```
`go build ./internal/...` fails. The entire project doesn't compile. This import was likely added during a partial `findCrossUserDedup` or `shareTrack` implementation and left dangling.

### C2 — No Track Record Created for Requesting User (CRITICAL)

**File:** `podcaster.go:814, 1174`

The `userID` parameter was added to `doDownloadEpisode()` and `doDownloadFeed()` signatures (parent task: "Both function signatures updated to accept userID int64") but is **never referenced** in either function body. When dedup finds an existing file:

1. `podcast_episodes.downloaded` is set to 1 with the existing file_path ✅
2. `go a.rescan()` fires — the scanner tries to create a `tracks` row ❌
3. `tracks.path` has a UNIQUE constraint — second track record for same path is rejected ❌
4. Requesting user has no playable track; `episodeTrack()` returns the original owner's track

**Impact:** Dedup "saves" the download but the episode is unplayable for the requesting user. The `episodeTrack` bridge endpoint returns the original user's track ID — which library queries may filter out (they use `WHERE user_id=? OR user_id IS NULL`).

### H1 — Job Marked Succeeded on DB Update Failure (HIGH)

**File:** `podcaster.go:852-858`

```go
if _, err := a.db.Exec(
    `UPDATE podcast_episodes SET downloaded=1, file_path=?, file_size=?, download_error=NULL WHERE id=?`,
    existingPath, existingSize, episodeID); err != nil {
    a.jobLog(jobID, "[db] dedup update failed: "+err.Error())
}
if a.jobs != nil && jobID != "" {
    a.jobs.FinishExternalJob(jobID, downloader.StatusSucceeded, "")  // ← ALWAYS succeeds
}
```
When the DB update to persist the dedup match fails, the error is logged but the job is **still marked as `StatusSucceeded`**. The episode remains undownloaded but the user sees a green checkmark.

### H2 — DB Exec Errors Silently Dropped in Dedup Pre-Pass (HIGH)

**File:** `podcaster.go:1220-1222`

```go
a.db.Exec(
    `UPDATE podcast_episodes SET downloaded=1, file_path=?, file_size=?, download_error=NULL WHERE id=?`,
    existingPath, existingSize, epID)
```
No error check at all — not even a log. If the DB is read-only, locked, or corrupted, the dedup match is silently lost.

### M1 — `dedupRows.Err()` Not Checked (MEDIUM)

**File:** `podcaster.go:1201-1234`

The dedup pre-pass in `doDownloadFeed` iterates `dedupRows` but never calls `dedupRows.Err()`. If the SQLite query encounters an error mid-iteration, it's silently swallowed and the loop terminates early. Pattern exists elsewhere in the codebase with correct `rows.Err()` checks (e.g., `listFeeds` line 606, `listEpisodes` line 659).

### L1 — Extension Gap in `findLatestAudioFile` (LOW)

**File:** `podcaster.go:1387-1388`

```go
case ".mp3", ".m4a", ".ogg", ".opus", ".flac", ".aac", ".wav":
```
Missing: `.m4b`, `.mp4`, `.webm` — all present in `findNewAudioFiles` (line 1363) and `guessAudioExt` (line 229). If poddl downloads a `.m4b` podcast file, `findLatestAudioFile` won't find it and reports "no audio file was found."

### L2 — TOCTOU in `checkEpisodeDedup` (LOW)

**File:** `podcaster.go:1014-1053`

Two separate queries — one to get the episode's GUID, one to search for duplicates. Between them, another goroutine could download the episode. Benign race: both goroutines reach the same conclusion. No data corruption, just wasted work in a worst-case scenario.

### L3 — Extension Gap also in `findLatestAudioFile` via `doDownloadFeed` (LOW)

Same as L1 but applies to the `downloadViaPoddl` single-episode fallback path. The bulk `doDownloadFeed` uses `findNewAudioFiles` (correct), but single-episode `downloadViaPoddl` uses `findLatestAudioFile` (missing extensions).

---

## What's Working Well

1. **Cross-user GUID matching** — `checkEpisodeDedup()` correctly searches all users with no user_id or feed_id filter.
2. **File existence verification** — `os.Stat()` before reuse prevents linking to deleted files.
3. **Graceful degradation** — empty GUID returns immediately, missing file falls through to re-download.
4. **Dedup pre-pass before poddl** — `doDownloadFeed` checks all undownloaded episodes before running the costly poddl subprocess.
5. **Rescan triggers** — both dedup paths trigger scanner rescan so new files appear in library.
6. **Semaphore + shutdown** — concurrency control and graceful shutdown are maintained.

---

## Fix Tasks Created

| Task | Severity | Summary |
|------|----------|---------|
| t_fix_build | CRITICAL | Remove unused `models` import from downloader.go |
| t_fix_track_records | CRITICAL | Create track records for dedup'd podcast episodes |
| t_fix_error_handling | HIGH | Fix error handling gaps in podcast dedup |
| t_fix_extensions | LOW | Add missing audio extensions to findLatestAudioFile |
