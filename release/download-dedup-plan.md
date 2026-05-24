# Cross-User Download Dedup Implementation Plan

**Date:** 2026-05-22
**Designer:** Atlas
**Status:** Draft — for review

This plan covers the design of a cross-user download deduplication system for Lexicon v3.7.0. When user B requests a track that user A already downloaded, user B should get a track record pointing to the same file — no re-download needed.

---

## 1. Current State Analysis

### 1.1 What Already Works

The codebase already has several pieces of the dedup puzzle:

**`findLibraryTrack()`** in `downloader.go` (lines 403-458) searches ALL tracks in the DB — no user filter. Four strategies: exact title+artist match, prefix match, FTS5, LIKE. Already cross-user by design.

**`searchEnqueue()`** (lines 461-556) calls `findLibraryTrack()` before downloading. When found, it creates a completed job with `TrackID` set. However, it does NOT create a new track record for the requesting user — it just returns the existing track ID.

**`UserID` field** exists on `Job` struct (line 143), `tracks` table (line 49 of db.go), `plays`, `playlists`, and other tables. The multi-user auth layer is fully operational.

**`auth.UserFromContext(r.Context())`** reliably extracts the authenticated user from every HTTP request. `getUserID(r)` (line 40) is used in both enqueue and searchEnqueue.

### 1.2 Current Blockers

**Blocker 1: `tracks.path` UNIQUE constraint** (db.go line 30):
```sql
path TEXT NOT NULL UNIQUE
```
Two users CANNOT have separate track records for the same file. This is the fundamental blocker to cross-user file sharing.

**Blocker 2: `enqueue()` never checks library** (lines 343-395):
The Spotify URL download path downloads unconditionally. If user A already has all tracks from an album and user B pastes the same Spotify URL, SpotiFLAC re-downloads everything. The files land in the same output directory, and the scanner tries to create new track records — which conflict on UNIQUE path.

**Blocker 3: `searchEnqueue()` doesn't create user tracks**:
When `findLibraryTrack()` finds a match (line 484), it creates a "resolved" job but doesn't create a new `tracks` row for the requesting user. User B sees someone else's track in the library but doesn't own it.

**Blocker 4: Podcaster has no cross-user awareness**:
`downloadDirectAudio()` and `downloadViaPoddl()` don't check if another user already downloaded the same episode. No `podcast_episodes.file_path` cross-reference before download.

**Blocker 5: `upgradeTrack()` deletes then re-downloads**:
It deletes the old file (line 1777) then re-downloads. If the old file was shared by two users, one user's upgrade destroys the other user's file. No cross-user check before deletion.

### 1.3 Download Entry Points (Complete)

| Entry Point | File | Function | Checks Library? | Creates Track? |
|---|---|---|---|---|
| Spotify URL | `downloader.go:343` | `enqueue()` | **NO** | Via scanner after download |
| Search query | `downloader.go:461` | `searchEnqueue()` | YES (findLibraryTrack) | Only via background polling |
| Track upgrade | `downloader.go:1736` | `upgradeTrack()` | NO | Deletes old, re-downloads |
| Podcast episode | `podcaster.go:814` | `doDownloadEpisode()` | NO | Updates `podcast_episodes` |
| Podcast feed | `podcaster.go` | `doDownloadFeed()` | NO | Updates `podcast_episodes` |
| AI playlist | `recommender.go` → `searchEnqueue` | Search pipeline | YES | Via searchEnqueue |

---

## 2. Dedup Check Points

### 2.1 `enqueue()` — Spotify URL Download (PRIORITY: HIGH)

**Current:** Downloads unconditionally, scanner later creates track records (which hit UNIQUE constraint).

**Proposed:** Add library check BEFORE enqueueing:
```go
// Before creating the job, check if any tracks from this URL already exist
if a.db != nil {
    existingTrackIDs := a.findTracksByURL(r.Context(), url)
    if len(existingTrackIDs) > 0 {
        // Create shared track records for this user
        userID := getUserID(r)
        for _, tid := range existingTrackIDs {
            a.shareTrack(r.Context(), tid, userID)
        }
        // Return "already in library" response
        ...
    }
}
```

**Challenge:** Determining "all tracks from this URL" is non-trivial. SpotiFLAC is opaque — we don't know what tracks it will produce until it runs. Options:
1. **Spotify API pre-fetch:** Query Spotify API for track listing from URL, check each track against `findLibraryTrack()`. Requires Spotify client credentials.
2. **Run SpotiFLAC in --dry-run mode:** Not supported by current binary.
3. **Post-hoc dedup:** Let SpotiFLAC run, then match downloaded files to existing tracks and share instead of creating new. This is the most reliable approach but wastes bandwidth.

**Recommended approach:** Option 3 (post-hoc). Wrap the `run()` function's success path: after download and rescan, query for tracks created by scanner in the download window, and if any match existing cross-user tracks, share them. This is less elegant but doesn't require Spotify API integration.

### 2.2 `searchEnqueue()` — Search-Based Download (PRIORITY: HIGH)

**Current:** `findLibraryTrack()` finds matches cross-user, returns existing track ID, but doesn't create a track record for requesting user.

**Proposed:** When `findLibraryTrack()` finds a match:
```go
trackID, err := a.findLibraryTrack(r.Context(), query)
if err == nil && trackID > 0 {
    // Check if requesting user already has this track
    uid := getUserID(r)
    ownsTrack := a.userOwnsTrack(r.Context(), uid, trackID)
    if !ownsTrack {
        // Share: create new track record for this user pointing to same file
        newTrackID, err := a.shareTrack(r.Context(), trackID, uid)
        if err == nil {
            trackID = newTrackID
        }
    }
    // Return resolved job with (possibly new) trackID
    ...
}
```

**This is the simplest and highest-impact dedup change.** It reuses existing `findLibraryTrack()` and only needs a new `shareTrack()` helper.

### 2.3 `upgradeTrack()` — Track Re-download (PRIORITY: MEDIUM)

**Current:** Deletes old file, then re-downloads. If others share the file, their tracks point to a deleted file.

**Proposed change flow:**
1. Check if file is shared (other users have tracks pointing to same path)
2. If shared: do NOT delete the file. Download to a new path (append user ID or timestamp to filename). Replace requesting user's path only.
3. If not shared (single user): keep current behavior — delete and replace.
```go
otherUserCount := countOtherUsersForPath(r.Context(), req.TrackID, oldPath)
if otherUserCount > 0 {
    // File is shared — download to new path, don't delete
    // New path: <dir>/<title> - <artist>_v2.<ext>
    keepOldFile = true
} else {
    // Single owner — safe to delete and replace
    os.Remove(oldPath)
}
```

### 2.4 Podcaster — Episode Download (PRIORITY: MEDIUM)

**Current:** Downloads unconditionally based on audio URL.

**Proposed:** Before downloading, check if ANY user already has this episode:
```go
// In doDownloadEpisode, before download:
existingPath := ""
db.QueryRowContext(ctx, 
    `SELECT file_path FROM podcast_episodes 
     WHERE audio_url=? AND downloaded=1 AND file_path IS NOT NULL 
     LIMIT 1`, audioURL).Scan(&existingPath)
if existingPath != "" {
    // File already exists — verify it's still on disk
    if _, err := os.Stat(existingPath); err == nil {
        // Share: mark this episode as downloaded with same file_path
        db.Exec(`UPDATE podcast_episodes SET downloaded=1, file_path=? WHERE id=?`,
            existingPath, episodeID)
        // Also create a tracks record via scanner, or directly
        if a.rescan != nil { go a.rescan() }
        return
    }
}
```

**Additional check:** Cross-reference `tracks` table for podcasts. If a track with matching path/artist/title already exists, share it instead of creating new.

### 2.5 Podcaster — Feed Download (PRIORITY: LOW)

Same logic as episode download but applied per-episode in the feed loop. For `doDownloadFeed()`, check each episode before downloading.

---

## 3. Cross-User File Matching

### 3.1 `findLibraryTrack()` — Already Cross-User

This function (lines 403-458) is the primary matching engine. It searches:
1. Exact `LOWER(title) = ? AND LOWER(artist) = ?` — no user_id filter
2. Prefix title match
3. FTS5 (full-text search with ranking)
4. LIKE on any field

**It already works cross-user.** No changes needed to the function itself.

### 3.2 New Matching Needed: `findTracksByAudioURL()`

For podcaster dedup, we need to find existing tracks by audio URL. This doesn't exist yet.

```go
func (a *API) findTrackByAudioURL(ctx context.Context, audioURL string) (int64, error) {
    var id int64
    err := a.db.QueryRowContext(ctx,
        `SELECT t.id FROM tracks t 
         JOIN podcast_episodes e ON t.path = e.file_path 
         WHERE e.audio_url = ? AND e.downloaded = 1 
         AND t.path IS NOT NULL 
         LIMIT 1`, audioURL).Scan(&id)
    return id, err
}
```

### 3.3 Fuzzy Matching Considerations

`findLibraryTrack()` strategy 3 (FTS5) and strategy 4 (LIKE) can produce false positives. For cross-user sharing, false positives mean user B gets linked to user A's track that isn't actually the same song.

**Mitigation:** When sharing, require at least strategy 1 or 2 (exact or prefix artist+title match). Don't share based on FTS5 or LIKE results alone. Add a `matchConfidence` return to `findLibraryTrack()`:
```go
type matchConfidence int
const (
    matchExact   matchConfidence = 1  // strategy 1a — title+artist exact
    matchPrefix  matchConfidence = 2  // strategy 1b — title prefix + artist exact
    matchFTS     matchConfidence = 3  // strategy 2 — FTS5
    matchLike    matchConfidence = 4  // strategy 3 — LIKE
)
```

Only auto-share for confidence levels 1 and 2.

---

## 4. File Sharing Model

### 4.1 `shareTrack()` — Core Operation

```go
// shareTrack creates a new track record for targetUser that points to the
// same file_path as sourceTrackID. Returns the new track ID.
// The new record copies all metadata but gets its own user_id, added_at, and
// play history.
func (a *API) shareTrack(ctx context.Context, sourceTrackID int64, targetUserID int64) (int64, error) {
    // 1. Verify source track exists and file is still on disk
    var source Track
    err := a.db.QueryRowContext(ctx,
        `SELECT `+models.TrackCols+` FROM tracks WHERE id=?`, sourceTrackID).Scan(
        &source.ID, &source.Path, ...)
    if err != nil {
        return 0, fmt.Errorf("source track %d: %w", sourceTrackID, err)
    }
    if _, err := os.Stat(source.Path); os.IsNotExist(err) {
        return 0, fmt.Errorf("source file deleted: %s", source.Path)
    }

    // 2. Check if target user already has a track for this path
    var existingID int64
    err = a.db.QueryRowContext(ctx,
        `SELECT id FROM tracks WHERE user_id=? AND path=? LIMIT 1`,
        targetUserID, source.Path).Scan(&existingID)
    if err == nil {
        return existingID, nil  // already shared
    }

    // 3. Insert new track record with target user_id, same path
    res, err := a.db.ExecContext(ctx,
        `INSERT INTO tracks 
         (path, title, artist, album_artist, album, track_no, disc_no, year, 
          genre, duration_sec, media_kind, mime, size_bytes, cover_path, 
          added_at, mtime, loudness_integrated, loudness_true_peak, loudness_range, 
          spotify_id, external_url, apple_id, user_id)
         VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
        source.Path, source.Title, source.Artist, source.AlbumArtist, source.Album,
        source.TrackNo, source.DiscNo, source.Year, source.Genre, source.DurationSec,
        source.MediaKind, source.Mime, source.SizeBytes, source.CoverPath,
        time.Now().Unix(), source.Mtime, source.LoudnessIntegrated,
        source.LoudnessTruePeak, source.LoudnessRange,
        source.SpotifyID, source.ExternalURL, source.AppleID, targetUserID)
    if err != nil {
        return 0, fmt.Errorf("insert shared track: %w", err)
    }
    newID, _ := res.LastInsertId()

    // 4. FTS5 trigger handles indexing automatically
    return newID, nil
}
```

### 4.2 `userOwnsTrack()` — Ownership Check

```go
func (a *API) userOwnsTrack(ctx context.Context, userID, trackID int64) bool {
    var uid sql.NullInt64
    err := a.db.QueryRowContext(ctx,
        `SELECT user_id FROM tracks WHERE id=?`, trackID).Scan(&uid)
    if err != nil {
        return false
    }
    return uid.Valid && uid.Int64 == userID
}
```

### 4.3 Scanner Implications

After `shareTrack()`, the requesting user gets their own `tracks` row. The FTS5 trigger (lines 61-63) automatically indexes it. Nothing else needed for the scanner — the file is already indexed (by the original owner's scanner run).

**Important:** The scanner's `indexFile()` function creates tracks with UNIQUE path constraint. After we remove that constraint, the scanner will work correctly — each scan creates track records scoped to the scanning user. If two users share a media root, both scanners will create their own track records for the same files.

---

## 5. Podcast Dedup

### 5.1 Episode-Level Dedup

Add a pre-download check in `doDownloadEpisode()`:

```go
func (a *API) doDownloadEpisode(ctx context.Context, episodeID, feedID int64, ...) {
    // ... existing setup ...

    // NEW: Check if any user already downloaded this episode
    var existingPath string
    err := a.db.QueryRowContext(ctx,
        `SELECT e.file_path FROM podcast_episodes e
         WHERE e.audio_url = ? AND e.downloaded = 1 
         AND e.file_path IS NOT NULL AND e.file_path != ''
         ORDER BY e.id LIMIT 1`, audioURL).Scan(&existingPath)
    if err == nil {
        if _, statErr := os.Stat(existingPath); statErr == nil {
            // File exists — reuse it
            a.db.ExecContext(ctx,
                `UPDATE podcast_episodes SET downloaded=1, file_path=?, download_error=NULL WHERE id=?`,
                existingPath, episodeID)
            // Now create/update a tracks record for the scanner
            if a.rescan != nil {
                go a.rescan()
            }
            if a.jobs != nil && jobID != "" {
                a.jobs.FinishExternalJob(jobID, downloader.StatusSucceeded, "")
            }
            return
        }
        // File was deleted — fall through and re-download
    }

    // ... existing download logic ...
}
```

### 5.2 Track-Level Dedup

After podcaster downloads succeed, check if a `tracks` record already exists for the downloaded file (created by scanner for a different user). If so, share it. If not, the scanner will create it.

### 5.3 Feed-Level Dedup

In `doDownloadFeed()`, iterate episodes with the same per-episode check above.

---

## 6. Upgrade Dedup

### 6.1 Current Problem

`upgradeTrack()` deletes the old file unconditionally (line 1777) even if other users share it. This is a data-loss bug in any multi-user scenario.

### 6.2 Proposed Fix

```go
func (a *API) upgradeTrack(w http.ResponseWriter, r *http.Request) {
    // ... existing lookup ...

    // Check if other users share this file
    var otherUsers int
    a.db.QueryRowContext(r.Context(),
        `SELECT COUNT(*) FROM tracks WHERE path=? AND user_id != ?`,
        oldPath, getUserID(r)).Scan(&otherUsers)

    if otherUsers > 0 {
        // File is shared — don't delete. Download to new path.
        // Append a version suffix to avoid collision.
        ext := filepath.Ext(oldPath)
        base := strings.TrimSuffix(oldPath, ext)
        newBase := base + "_upgraded" + ext
        // ... download to newBase, update requesting user's path ...
        // Other users keep old path
    } else {
        // Single owner — safe to delete and replace
        os.Remove(oldPath)
        // ... normal download ...
    }
}
```

### 6.3 Edge Case: All Users Upgrade

If all owners upgrade the same track, old file becomes orphaned. Consider a `file_refcount` approach:
- Count tracks pointing to the file
- Only delete when refcount drops to 0

This is more complex but more correct. For v1, the simple check (don't delete if shared) is sufficient.

---

## 7. Database Schema Changes

### 7.1 Remove UNIQUE on `tracks.path`

**Current:**
```sql
CREATE TABLE tracks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL UNIQUE,
    ...
    user_id INTEGER REFERENCES users(id) ON DELETE SET NULL
);
```

**New:**
```sql
CREATE TABLE tracks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL,
    ...
    user_id INTEGER REFERENCES users(id) ON DELETE SET NULL
);
-- Replace with non-unique index for fast path lookups
CREATE INDEX IF NOT EXISTS idx_tracks_path ON tracks(path);
-- Composite unique to prevent duplicate user+path records
CREATE UNIQUE INDEX IF NOT EXISTS idx_tracks_user_path ON tracks(user_id, path) WHERE user_id IS NOT NULL;
```

### 7.2 Migration Strategy

SQLite doesn't support `DROP CONSTRAINT`. The migration must:
1. Create new table `tracks_v2` with the modified schema
2. Copy all data from `tracks` to `tracks_v2`
3. Drop `tracks`
4. Rename `tracks_v2` to `tracks`
5. Rebuild FTS5 content and triggers

Or, simpler: use the additive column migration pattern. Since the UNIQUE constraint is implicit in the CREATE TABLE statement and SQLite can't alter it, the migration MUST recreate the table.

**Migration approach:**
```go
// In Migrate(), check if we've already done the migration
if !columnExists(db, "tracks_v2_migrated", "done") {
    // 1. Create new table without unique on path
    db.Exec(`CREATE TABLE tracks_v2 (id INTEGER PRIMARY KEY AUTOINCREMENT, 
        path TEXT NOT NULL, ...)`)
    // 2. Copy data
    db.Exec(`INSERT INTO tracks_v2 SELECT * FROM tracks`)
    // 3. Drop old table
    db.Exec(`DROP TABLE tracks`)
    // 4. Rename
    db.Exec(`ALTER TABLE tracks_v2 RENAME TO tracks`)
    // 5. Rebuild indexes
    db.Exec(`CREATE INDEX idx_tracks_path ON tracks(path)`)
    db.Exec(`CREATE UNIQUE INDEX idx_tracks_user_path ON tracks(user_id, path)`)
    // 6. Rebuild FTS5
    db.Exec(`INSERT INTO tracks_fts(tracks_fts) VALUES('rebuild')`)
    // 7. Mark migration done
    db.Exec(`CREATE TABLE tracks_v2_migrated (done INTEGER)`)
    db.Exec(`INSERT INTO tracks_v2_migrated VALUES (1)`)
}
```

### 7.3 Backward Compatibility

For single-user installations (where `user_id` is NULL or all the same), the `idx_tracks_user_path` unique index still prevents duplicates per-user+path. Library queries with `WHERE user_id=? OR user_id IS NULL` continue working.

### 7.4 No New Tables Needed

The sharing model doesn't require a `track_shares` table. A new `tracks` row with a different `user_id` is sufficient. Each user has independent play counts, playlist associations, etc.

---

## 8. API Changes

### 8.1 No New Endpoints Required

The dedup logic is transparent — existing endpoints behavior changes:
- `POST /api/download` — may return "already in library" instead of downloading
- `POST /api/download/search` — already returns existing track IDs, now creates shared records
- `POST /api/library/upgrade` — won't delete files owned by other users

### 8.2 Response Changes

**`POST /api/download` dedup response:**
```json
{
    "id": "job-uuid",
    "status": "succeeded",
    "dedup": true,
    "shared_from_user": "alice",
    "track_id": 42
}
```

**`POST /api/download/search` dedup response:** Same as above, adding `"dedup": true` and `"shared_from_user"`.

### 8.3 New API Status Field

Add `dedup` to `Job` struct:
```go
type Job struct {
    ...
    Dedup bool `json:"dedup,omitempty"` // true when resolved via cross-user dedup
}
```

No new `download_jobs` column needed — this is ephemeral (only relevant for in-memory/same-session jobs).

---

## 9. Frontend Changes

### 9.1 Track Ownership Display

**Library page (`MusicPage.tsx`):** If `track.user_id` differs from current user, show a subtle "shared" indicator. The track behaves normally — playable, addable to playlists.

### 9.2 Download Feedback

**Downloads page (`DownloadsPage.tsx`):** When a job resolves via dedup, show:
- Status: "Already in Library (shared by <username>)"
- Green checkmark instead of download progress
- Track title clickable (links to track in library)

### 9.3 Toast Notifications

When dedup resolves a search:
```typescript
toast.info("Found in your library — no download needed")
```

When a shared track is discovered:
```typescript
toast.success("Added from shared library")
```

### 9.4 Playlist Page — Remove Track

**PlaylistPage.tsx:** When removing a shared track from a playlist, only remove the playlist association. Do NOT offer to delete the file (it belongs to someone else).

### 9.5 No New Pages or Components Needed

The dedup is transparent to most user flows. The main UX changes are:
1. Toast/indicator showing when dedup resolved a download
2. Ownership indicator in track display
3. Safe delete behavior for shared tracks

---

## 10. Edge Cases

### 10.1 File Deleted by Owner

**Scenario:** User A downloads a track. User B shares it (new track record points to same path). User A deletes their copy (via library delete).

**Problem:** User B's track record now points to a deleted file.

**Solution:** 
1. **Delete operation check:** When user A deletes a track, check if other users share the file. If yes, don't delete the file — only remove user A's track record.
2. **Playback guard:** Before playing, stat the file. If missing, show error and optionally trigger a re-share from another owner or re-download.
3. **Scanner resilience:** Scanner already skips missing files gracefully.

**Implementation:**
```go
func (a *API) deleteTrack(w http.ResponseWriter, r *http.Request) {
    // ... existing lookup ...
    
    // Check if other users share this file
    var otherUsers int
    a.db.QueryRowContext(ctx,
        `SELECT COUNT(*) FROM tracks WHERE path=? AND id != ?`, path, trackID).Scan(&otherUsers)
    
    if otherUsers == 0 {
        // Last owner — safe to delete file
        os.Remove(path)
    }
    // Always delete the track record
    // ...
}
```

### 10.2 Race Condition: Simultaneous Downloads

**Scenario:** User A and User B both request the same search query at the same time. Both `searchEnqueue` calls pass `findLibraryTrack()` before either download completes.

**Current state:** Both download. The second scanner run hits UNIQUE path constraint.

**After dedup:** Both download since no track exists yet. The second run still creates a duplicate file on disk. BUT with UNIQUE(user_id, path), the scanner creates separate records for both users. The second download is wasted bandwidth but not destructive.

**Mitigation:** Add an in-memory "downloading" map keyed by normalized query:
```go
a.pendingDownloadsMu.Lock()
key := normalizeDownloadKey(query, getUserID(r))
if _, ok := a.pendingDownloads[key]; ok {
    a.pendingDownloadsMu.Unlock()
    // Another request for same query is already downloading — wait or dedup
    writeJSON(w, map[string]interface{}{"status": "queued", "message": "already downloading"})
    return
}
a.pendingDownloads[key] = struct{}{}
a.pendingDownloadsMu.Unlock()
// Register cleanup in runSearch's deferred path
```

### 10.3 Fuzzy Match False Positives

**Scenario:** User B searches "Beatles - Hey Jude". `findLibraryTrack()` LIKE strategy matches "Beatles - Hey Bulldog" from user A's library.

**Mitigation:** Only auto-share for exact matches (strategies 1a, 1b). FTS5 and LIKE matches should surface as "Did you mean...?" suggestions in the frontend, not auto-share.

### 10.4 Different Formats / Quality

**Scenario:** User A has a 128kbps MP3. User B requests "flac" version. The dedup check finds user A's MP3 and shares it — but user B wanted FLAC.

**Solution:** Don't dedup across formats. Check MIME type:
```sql
SELECT id FROM tracks WHERE LOWER(title)=LOWER(?) AND LOWER(artist)=LOWER(?) AND mime=? LIMIT 1
```
Or simpler: share regardless. The AI playlist generation feature already doesn't distinguish formats — it works by artist+title matching. Format preference is a separate feature.

### 10.5 User Deleted from System

**Scenario:** User A is deleted. Their tracks have `user_id SET NULL` (ON DELETE SET NULL). User B shared those tracks.

**Problem:** Who "owns" the file after the original uploader is deleted?

**Solution:** `ON DELETE SET NULL` already handles this. Tracks with NULL user_id are visible to all users (current behavior). The file stays on disk until the last track reference is deleted.

### 10.6 Storage Growth

**Scenario:** 10 users all share the same 1000-track library. No wasted storage (same files). BUT scanner creates 10,000 track records. FTS5 index has 10x entries.

**Mitigation:** This is acceptable for a desktop app. SQLite handle millions of rows easily. If it becomes a problem, add a `shared BOOLEAN` column and collapse FTS5 entries for shared tracks.

---

## 11. Implementation Phases

### Phase 1: Core Dedup (searchEnqueue)
**Impact:** Highest ROI. Fixes the most common user path.
1. Add `shareTrack()` function
2. Modify `searchEnqueue()` to share when match found
3. Add confidence levels to `findLibraryTrack()`
4. Only auto-share for high-confidence matches

### Phase 2: Schema Migration
1. Remove UNIQUE on `tracks.path`
2. Add UNIQUE(user_id, path)
3. Update scanner to handle duplicate paths
4. Add migration code to `db.Migrate()`

### Phase 3: enqueue() Dedup
1. Post-hoc dedup after SpotiFLAC download
2. Match downloaded files against existing tracks
3. Share instead of creating new

### Phase 4: Podcast Dedup
1. Pre-download audio URL check
2. Cross-reference tracks table
3. Share existing files

### Phase 5: Upgrade Safety
1. Check file ownership before delete
2. Download to new path when shared
3. Orphaned file cleanup

### Phase 6: Frontend Polish
1. Track ownership indicators
2. Dedup toast notifications
3. Safe delete behavior

---

## 12. Files Affected

| File | Changes |
|------|---------|
| `backend/internal/downloader/downloader.go` | Add `shareTrack()`, `userOwnsTrack()`, `countOtherUsers()`. Modify `searchEnqueue()`, `enqueue()`, `upgradeTrack()`. Add confidence levels to `findLibraryTrack()`. Add pending download dedup map. |
| `backend/internal/podcaster/podcaster.go` | Add pre-download episode dedup check. Modify `doDownloadEpisode()`, `doDownloadFeed()`. Add `findEpisodeByAudioURL()`. |
| `backend/internal/db/db.go` | Schema migration: remove UNIQUE on path, add UNIQUE(user_id, path). Add `tracks_v2_migrated` marker. Update FTS5 rebuild. |
| `backend/internal/models/models.go` | Add `Dedup` field to Track JSON response. |
| `backend/internal/scanner/scanner.go` | Handle duplicate paths (different user_ids). FTS5 triggers auto-handle new rows. |
| `backend/internal/library/library.go` | Modify delete to check file ownership. |
| `frontend/src/lib/api.ts` | Add `dedup`, `shared_from_user` to DownloadJob type. |
| `frontend/src/pages/DownloadsPage.tsx` | Show dedup status. |
| `frontend/src/pages/MusicPage.tsx` | Show shared indicator. |
| `frontend/src/components/TrackList.tsx` | Show ownership indicator. |
