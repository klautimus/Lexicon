# Lexicon Download + Cross-User Dedup Architecture

**Date:** 2026-05-22
**Author:** Atlas (kanban task t_a8007d12)
**Status:** Exploration — for implementation planning

---

## 1. Current Download Flows

### 1.1 URL Download (`POST /api/download`)

```
enqueue() [line 343]
  ├── Validates Spotify URL
  ├── Creates Job{UserID: getUserID(r), Kind: "music", Mode: "url"}
  ├── Persists to download_jobs table (with user_id)
  ├── NO library dedup check
  └── go run(job, shutdownCtx)

run() [line 807]
  ├── Acquires semaphore slot
  ├── Tier 1: SpotiFLAC (primary tool)
  │   ├── On success: finish(Succeeded) → go rescan()
  │   └── On failure: parse "Summary: X Success, Y Failed"
  ├── Tier 2: yt-dlp (fallback)
  │   ├── ytsearch1:<parsedQuery> with bestaudio/best
  │   ├── On success: verifyDownloadedFile() → finish(Succeeded) → go rescan()
  │   └── On failure: continue to Tier 3
  └── Tier 3: spotDL (final fallback)
      ├── Downloads from YouTube Music
      ├── On success: verifyDownloadedFile() → finish(Succeeded) → go rescan()
      └── On failure: finish(Failed)
```

**Key observation:** `enqueue()` never calls `findLibraryTrack()`. Every Spotify URL download starts fresh, even if the same track already exists in the library.

### 1.2 Search Download (`POST /api/download/search`)

```
searchEnqueue() [line 461]
  ├── Validates query is non-empty
  ├── CALLS findLibraryTrack() ← DEDUP CHECK
  │   └── If found: creates instant-success job with TrackID, returns immediately
  ├── Creates Job{UserID: getUserID(r), Kind: "music", Mode: "search"}
  ├── Persists to download_jobs table (with user_id)
  └── go runSearch(job, shutdownCtx)

runSearch() [line 1060]
  ├── Acquires semaphore slot
  ├── Optional DeepSeek query parsing (structured metadata + Spotify URL)
  ├── Optional Spotify API search (Client Credentials flow)
  ├── Optional SpotiFLAC attempt (if Spotify URL resolved)
  ├── yt-dlp: ytsearch1:<query> with bestaudio/best + match-filter duration < 600
  │   ├── On success: verifyDownloadedFile() → finish(Succeeded) → go rescan()
  │   └── On failure: retry with ytsearch2 + m4a
  └── Post-success: goroutine polls DB for track resolution (2 min)
```

**Key observation:** `searchEnqueue()` is the ONLY download path with a library dedup check. URL downloads and podcast downloads have NONE.

### 1.3 Podcast Episode Download (`POST /api/podcasts/episodes/{id}/download`)

```
downloadEpisode() [line 766]
  ├── Looks up episode (audio_url, title) and feed (url, title)
  ├── Registers external job via downloader.RegisterExternalJob()
  ├── NO library dedup check
  └── go doDownloadEpisode()

doDownloadEpisode() [line 814]
  ├── If direct audio_url: downloadDirectAudio() via HTTP
  │   └── Writes to outputDir/<episodeFilename()>
  └── If no direct URL: downloadViaPoddl() via poddl CLI
      └── poddl <feedURL> -o <outputDir> -r -t 1

On success:
  ├── UPDATE podcast_episodes SET downloaded=1, file_path=?, file_size=?
  ├── FinishExternalJob(Succeeded)
  └── go rescan()  ← Scanner indexes the file into tracks table
```

**Key observation:** No `findLibraryTrack()` call. Podcast files are downloaded regardless of existing library content. The scanner creates/updates the track record with `user_id=NULL`.

### 1.4 AI Playlist Generation → Download (`POST /api/recommendations/playlist`)

```
recommender: generatePlaylist() [line ~800]
  ├── DeepSeek generates playlist {name, description, tracks: [{title, artist, reason}]}
  └── resolveTrackIDs() [line 915]
      ├── For each item: SELECT id FROM tracks WHERE LOWER(title)=LOWER(?) AND LOWER(IFNULL(artist,''))=LOWER(?)
      ├── NO user_id filter on this query
      └── If found: sets item.TrackID, upgrades type from "discover" → "library"

Frontend: DownloadContext.tsx createAiPlaylist()
  └── For each AI track:
      ├── Calls api.downloadSearch(artist + " - " + title)
      ├── Backend: searchEnqueue() → findLibraryTrack() dedup check
      └── Polls job, then api.scan(), then retries library search
```

**Key observation:** The recommender's `resolveTrackIDs()` is global (no user_id filter). The frontend uses `downloadSearch` which has `findLibraryTrack()` dedup — but only within the same server session, not across user boundaries.

### 1.5 Track Upgrade (`POST /api/library/upgrade`)

```
upgradeTrack() [line 1736]
  ├── Looks up track by ID: SELECT path, title, artist FROM tracks WHERE id=? AND media_kind='music'
  ├── Deletes old file (best-effort)
  ├── Creates Job{IsSearch: true, TrackID: req.TrackID}
  └── go runSearchWithTrackID(job, trackID)

runSearchWithTrackID() [line 1824]
  ├── Calls runSearch() (standard search pipeline)
  └── On success: UPDATE tracks SET path=?, mime=?, size_bytes=?, mtime=? WHERE id=?
```

**Key observation:** Track upgrade is by track ID — no dedup concern since it replaces an existing track.

---

## 2. How `findLibraryTrack()` Works

**Location:** `downloader.go` lines 401-459

```
findLibraryTrack(ctx, query) → (int64, error)

Strategy 1: "Artist - Title" format parsing
  ├── 1a: Exact title + artist match (case-insensitive)
  │   SELECT id FROM tracks WHERE LOWER(title)=LOWER(?) AND LOWER(IFNULL(artist,''))=LOWER(?)
  ├── 1b: Title prefix match + artist match
  │   SELECT id FROM tracks WHERE LOWER(title) LIKE LOWER(?) AND LOWER(IFNULL(artist,''))=LOWER(?)

Strategy 2: FTS5 full-text search
  └── SELECT t.id FROM tracks_fts f JOIN tracks t ON t.id=f.rowid WHERE tracks_fts MATCH ? ORDER BY rank LIMIT 1

Strategy 3: LIKE fallback (most lenient)
  └── SELECT id FROM tracks WHERE LOWER(title) LIKE LOWER(?) OR LOWER(IFNULL(artist,'')) LIKE LOWER(?)
```

**What it does NOT do:**
- Does NOT filter by `user_id`
- Does NOT check `media_kind` (could match podcast against music)
- Does NOT check if the file actually exists on disk
- Does NOT verify file path validity

**Where it IS called:**
- `searchEnqueue()` — before starting a search download

**Where it is NOT called:**
- `enqueue()` — URL downloads skip the check entirely
- `downloadEpisode()` — podcast downloads have no dedup
- `upgradeTrack()` — uses track ID, not dedup
- `run()` / `runSearch()` — these are post-enqueue, no mid-flight dedup

---

## 3. How Tracks Are Created in the DB

### 3.1 Scanner: `scanner.go` `indexFile()` (line 111)

```sql
INSERT INTO tracks(path,title,artist,album_artist,album,track_no,disc_no,year,genre,mime,size_bytes,cover_path,added_at,media_kind,mtime,loudness_integrated,loudness_true_peak,loudness_range,user_id)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(path) DO UPDATE SET
    title=excluded.title, artist=excluded.artist, ...
```

- `user_id` is always passed as `nil` — new tracks have NULL user_id
- `ON CONFLICT(path)` — path is the unique constraint
- On first migration, all NULL user_id tracks get assigned to admin (user_id=1)

### 3.2 Downloader: Does NOT create tracks directly

The downloader downloads files, then triggers `go a.rescan()`. The scanner's `indexFile()` creates/updates the track record when it encounters the new file on disk.

Post-download, `runSearch()` has a polling goroutine (lines 1272-1306) that waits up to 2 minutes for the scanner to index the file, then sets `job.TrackID`.

### 3.3 Podcaster: Relies on scanner for track creation

Podcast downloads write files to `outputDir`. The `go a.rescan()` call triggers the scanner, which indexes them as `media_kind='podcast'` with `user_id=NULL`.

The `episodeTrack` endpoint bridges podcasts to tracks by file_path:
```sql
SELECT id FROM tracks WHERE path=?
```

---

## 4. How `file_path` Is Stored and Sharing Implications

### 4.1 Schema constraint

```sql
CREATE TABLE tracks (
    path TEXT NOT NULL UNIQUE,
    user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    ...
)
```

**`path` is UNIQUE.** There can only be ONE track row per file path, regardless of user_id. This is the fundamental constraint shaping the dedup design.

### 4.2 Implications

| Scenario | Behavior |
|----------|----------|
| User A downloads "Beatles - Hey Jude.mp3" to `/music/Beatles - Hey Jude.mp3` | Track created with path `/music/Beatles - Hey Jude.mp3`, user_id=1 (admin default) |
| User B downloads same track to same path | Scanner hits `ON CONFLICT(path) DO UPDATE` — track is shared |
| User B downloads to different path `/music/user2/Beatles - Hey Jude.mp3` | NEW track created with different path — DUPLICATE on disk |

### 4.3 Output directory structure

The downloader writes to `a.cfg.Output` (from `SPOTIFLAC_OUTPUT` / `MEDIA_ROOTS` env var). All users download to the same output directory. The yt-dlp output template is:
```
-o <outputDir>/%(artist)s - %(title)s.%(ext)s
```

This means: if two users search for "Beatles Hey Jude" via `downloadSearch()`, both jobs would try to write to the same file path. The second job might:
1. Overwrite the first job's file (if yt-dlp overwrites)
2. Create a different file if yt-dlp uses a different naming scheme
3. Race with the scanner (first scanner pass creates track, second pass updates it)

---

## 5. The `user_id` Column on Tracks

### 5.1 Current state

- **Schema:** `user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`
- **Index:** `CREATE INDEX IF NOT EXISTS idx_tracks_user ON tracks(user_id)`
- **Creation:** Scanner passes `nil` → tracks initially have NULL user_id
- **Migration:** On first startup with zero users, all NULL tracks get assigned to admin (user_id=1)
- **Library API:** Does NOT filter by user_id (returns all tracks to all users)
- **Recommender:** Does NOT filter by user_id (resolves tracks globally)
- **`findLibraryTrack()`:** Does NOT filter by user_id (matches globally)

### 5.2 What `user_id` currently controls

- **`download_jobs.user_id`**: Filters jobs in `listJobs()` and `progress()` — a user only sees their own download jobs
- **`playlists.user_id`**: Playlists are scoped to user — `ON DELETE CASCADE`
- **`plays.user_id`**: Play history is scoped to user
- **`recommendations.user_id`**: Recommendations are scoped to user

### 5.3 What `user_id` does NOT control

- **`tracks.user_id`**: All users see all tracks
- **Library search/browse**: No user filtering
- **Dedup checks**: No user filtering
- **Streaming**: Any user can stream any track (path-based access)

---

## 6. Current Dedup Coverage

| Download Path | Dedup Check? | Where? | User-Scoped? |
|---|---|---|---|
| URL download (`enqueue`) | ❌ None | — | — |
| Search download (`searchEnqueue`) | ✅ Yes | `findLibraryTrack()` before download | ❌ Global |
| Podcast download | ❌ None | — | — |
| AI playlist generation | ✅ Partial | Recommender resolves TrackID; frontend uses `downloadSearch` | ❌ Global |
| Track upgrade | N/A | Uses existing track ID | N/A |

---

## 7. Where Dedup Should Be Inserted

### 7.1 Primary dedup point: Global, path-based

Since `tracks.path` is UNIQUE, the most efficient cross-user dedup is:

**Check if any track already matches the artist+title query BEFORE downloading.**

Insert a call to `findLibraryTrack()` (or a new `findLibraryTrackForDedup()`) in:
1. **`enqueue()`** — before starting a Spotify URL download
2. **`downloadEpisode()`** — before starting a podcast download
3. **`runSearch()`** — a second check (not just in `searchEnqueue()`), in case another user downloaded while this job was queued

### 7.2 What to do when a match is found

| Match scenario | Action |
|---|---|
| Same query, track exists, file on disk | Skip download, return existing TrackID |
| Same query, track exists, file MISSING | Re-download (file was deleted by owner) |
| Same query, track exists, different path | Link to existing file (or re-download if preferred) |
| Same query, no track exists | Download as normal |

### 7.3 Edge Cases

| Edge Case | Risk | Mitigation |
|---|---|---|
| **File deleted by owner** | Dedup returns track with missing file | `os.Stat()` the path before returning dedup hit; fall through to download on Stat error |
| **File moved** | Track path is stale | `os.Stat()` check handles this |
| **Filename collision** | Two users download same song, yt-dlp writes to same file | First download creates file; second job's yt-dlp may overwrite or fail; dedup check prevents second download entirely |
| **Concurrent downloads** | Two users enqueue same search simultaneously | Semaphore doesn't help (different jobs). Need a mutex or DB-level lock on the dedup query. Simplest: accept the race — both download, scanner's ON CONFLICT handles DB integrity |
| **Podcast dedup** | Two users subscribe to same feed, both download same episode | Scanner indexes file → ON CONFLICT(path) handles DB. But both users consume bandwidth. Dedup via `findLibraryTrack()` would save bandwidth |
| **Different audio quality** | User wants FLAC, existing track is MP3 | Dedup should check format/quality preference. Simplest: always re-download if existing mime doesn't match desired format |

---

## 8. Implementation Approach

### 8.1 Minimal Viable Cross-User Dedup

Add a `findLibraryTrack()` call to `enqueue()` and `downloadEpisode()`, mirroring the existing check in `searchEnqueue()`.

```go
// In enqueue(), after validation:
if a.db != nil {
    trackID, err := a.findLibraryTrack(r.Context(), extractQueryFromURL(job.URL))
    if err == nil && trackID > 0 {
        // Verify file still exists
        var path string
        if err := a.db.QueryRowContext(r.Context(), 
            "SELECT path FROM tracks WHERE id=?", trackID).Scan(&path); err == nil {
            if _, statErr := os.Stat(path); statErr == nil {
                // Track exists — skip download
                job.Status = StatusSucceeded
                job.TrackID = trackID
                job.FinishedAt = time.Now().Unix()
                job.Log = []string{"[dedup] resolved to existing library track"}
                // persist and return...
                return
            }
        }
        // File missing — fall through to download
    }
}
```

### 8.2 Enhanced: User-aware Dedup

If Lexicon moves toward per-user library views:

1. **`findLibraryTrack()` should accept optional `userID` parameter**
2. **Prefer user's own tracks, fall back to other users' tracks**
3. **When borrowing another user's track, set `user_id` on the play record but not on the track**

### 8.3 Full Multi-User Isolation (Future)

Would require:
- Per-user media roots (e.g., `/music/user1/`, `/music/user2/`)
- `tracks.path` uniqueness per-user (not global)
- OR a many-to-many user_tracks junction table

This significantly changes the architecture and is NOT recommended for the current desktop app model.

---

## 9. Database Summary

### Tables with `user_id`

| Table | user_id column | Default | Index |
|---|---|---|---|
| `tracks` | `INTEGER REFERENCES users(id) ON DELETE SET NULL` | NULL → set to admin on first migration | ✅ |
| `plays` | `INTEGER REFERENCES users(id) ON DELETE SET NULL` | NULL | ✅ |
| `playlists` | `INTEGER REFERENCES users(id) ON DELETE CASCADE` | NULL | ✅ |
| `download_jobs` | `INTEGER REFERENCES users(id) ON DELETE SET NULL` | NULL | ✅ |
| `recommendations` | `INTEGER REFERENCES users(id) ON DELETE SET NULL` | NULL | ✅ |
| `podcast_feeds` | `INTEGER REFERENCES users(id) ON DELETE SET NULL` | NULL | ✅ |

### Track uniqueness

```
tracks.path TEXT NOT NULL UNIQUE  ← The constraint driving dedup design
```

---

## 10. Frontend Summary

### Downloads Page (`DownloadsPage.tsx`)
- Lists ALL jobs, filtered by user_id in `listJobs()` server-side
- Polls every 1.5 seconds for job status
- Supports URL mode and search mode
- No client-side dedup awareness

### Search Page (`SearchPage.tsx`)
- Uses `useDownloads().trackDownload()` for "Search & Download from Web"
- After download, triggers `api.scan()` then polls library search for up to 3 min
- No awareness of cross-user dedup

### Download Context (`DownloadContext.tsx`)
- `downloadItem()` calls `api.downloadSearch(key)` 
- `createAiPlaylist()` processes AI tracks sequentially via `downloadSearch()`
- After download: scans, polls library, retries for up to 3 min
- No cross-user dedup awareness

---

## 11. Key Architecture Decisions for Implementation

1. **Dedup at enqueue time, not download time.** By the time `run()` starts, the job should already know if a matching track exists. This prevents unnecessary subprocess spawning.

2. **`os.Stat()` verification is essential.** A track record with a missing file is worse than no track record — the user sees a track that doesn't play. Always verify file existence before returning a dedup hit.

3. **`tracks.path` uniqueness is the anchor.** Since path is UNIQUE, any two downloads of the same content to the same output dir will hit `ON CONFLICT`. The race condition is benign — the scanner handles it.

4. **Podcast dedup is lower priority.** Podcast files are large (50-200MB). Dedup would save significant bandwidth. But the `podcast_episodes` table has its own `file_path` column that would need cross-referencing.

5. **The `user_id` migration assigns ALL NULL tracks to admin.** This means after first boot with the new schema, all existing tracks belong to admin. New tracks from the scanner still get NULL until the next migration run or explicit assignment.
