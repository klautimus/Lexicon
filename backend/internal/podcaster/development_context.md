# podcaster — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/podcaster/podcaster.go` (~1260 LOC) — 🆕 NEW in v2.8.0, updated v3.5.3
> **Last updated:** 2026-05-21

## Purpose

Manages podcast feed subscriptions and episode downloads. Uses gofeed for RSS parsing and (when no direct audio URL is available) poddl as fallback. Downloaded episodes are indexed into the tracks table as media_kind='podcast'.

## Unified Job Tracking (v2.10.0)

Every download — single episode or bulk feed — registers a job with the `downloader` package via the `JobSink` interface and shows up on the `/api/download/jobs` endpoint with `kind="podcast"`. Subprocess stdout/stderr (poddl) is streamed line-by-line into the job log so users can see real-time progress on the Downloads page. HTTP responses log status code + body snippet on non-200, so the actual server error (e.g. `403 access denied`) is recorded instead of a generic `"HTTP 403"`.

Pass `*downloader.API` into `podcaster.New(...)` as the 4th argument. May be nil — downloads still work but won't appear on the Downloads page.

## Config

```go
type Config struct {
    PoddlBin     string  // path to poddl.exe
    OutputDir    string  // where episodes are saved (default: ".")
    AutoDownload bool    // auto-download new episodes
}
```

Env vars: `PODDL_BIN`, `PODCAST_DIR`

## Routes

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/podcasts/feeds` | List subscribed feeds |
| `POST` | `/api/podcasts/subscribe` | Subscribe to RSS feed URL |
| `DELETE` | `/api/podcasts/feeds/{id}` | Unsubscribe |
| `GET` | `/api/podcasts/feeds/{id}/episodes` | List episodes for a feed |
| `POST` | `/api/podcasts/feeds/{id}/sync` | Sync feed (re-fetch RSS) |
| `POST` | `/api/podcasts/episodes/{id}/download` | Download single episode |
| `POST` | `/api/podcasts/feeds/{id}/download` | Download all episodes in feed |
| `GET` | `/api/podcasts/status` | Check poddl availability |

## Download Logic

### Single Episode (`doDownloadEpisode`)
- **If episode has a direct `audio_url` (from RSS enclosure):** Downloads directly via Go's `net/http` client — NOT via poddl. poddl only accepts RSS feed URLs, not direct audio URLs.
- **If no direct URL:** Falls back to poddl with feed URL + `-r -t 1` flags (download latest episode only)
- **Timeout:** 5-minute timeout on both direct downloads and poddl calls via `context.WithTimeout`
- After download, updates `podcast_episodes` table: `downloaded=1, file_path=..., file_size=...`
- On error: sets `download_error` column with specific error message
- Triggers rescan after successful download

### Full Feed (`doDownloadFeed`)
- Passes feed URL to poddl with `-r -h` flags: `poddl <feedURL> -o <outputDir> -r -h`
  - `-r`: newest episodes first (matches pub_date DESC order)
  - `-h`: quit when first existing file is found (efficient incremental downloads)
- **Timeout:** 30-minute timeout via `context.WithTimeout`
- **File-to-episode matching:** Uses timestamp snapshot approach:
  1. Records `time.Now()` before poddl runs
  2. After poddl completes, finds all audio files with ModTime >= snapshot
  3. Sorts by mtime DESC (newest first, matching poddl's `-r` download order)
  4. Queries undownloaded episodes sorted by pub_date DESC (newest first)
  5. Matches positionally: files[i] → episodes[i] (newest file → newest episode)
- Collects detailed logs of each file-to-episode match
- Triggers rescan after successful downloads

### poddl CLI Usage
```bash
# poddl expects: poddl.exe <url> <output_path>
# -o flag needed when other arguments are passed
poddl.exe "https://example.com/feed.xml" -o C:\podcasts -r -t 1
```

**CRITICAL:** poddl is an RSS feed parser/downloader. It does NOT accept direct audio URLs. Passing an MP3 URL to poddl will cause it to fail or hang. Always use Go's HTTP client for direct audio downloads.

## DB Tables

### `podcast_feeds`
- `id`, `url` (unique), `title`, `description`, `image_url`, `author`, `link`, `language`
- `last_fetched_at`, `last_error`, `auto_download`, `download_folder`, `created_at`

### `podcast_episodes`
- `id`, `feed_id` (FK), `guid`, `title`, `description`, `pub_date`, `duration_sec`
- `audio_url`, `audio_type`, `audio_size`
- `downloaded`, `file_path`, `file_size`, `download_error`
- `created_at`, `UNIQUE(feed_id, guid)`

## Known Issues (Fixed in v2.9.0)

- ✅ **PODDL_BIN not set in .env** — poddl.exe was bundled but path was empty in .env. Fixed by setting `PODDL_BIN=C:/Users/kevin/CascadeProjects/lexicon/tools/poddl.exe`
- ✅ **doDownloadEpisode passed feedURL instead of audioURL** — poddl received the RSS feed and tried to download ALL episodes. Fixed to use the episode's direct audio URL.
- ✅ **Frontend polling was broken** — 3-second interval fired once without checking download status. Fixed to poll every 2s and check `episode.downloaded` flag.
- ✅ **poddl given direct audio URL** — poddl only accepts RSS feed URLs. Passing `https://cdn.example.com/episode.mp3` caused poddl to hang/fail trying to parse binary as XML. Fixed: direct audio URLs now downloaded via Go's `net/http` client. poddl only used for RSS feed downloads.
- ✅ **doDownloadFeed episode matching** — `findLatestAudioFile` returned the same file for every episode. Fixed: timestamp snapshot approach with positional matching (newest file → newest episode). Added `-h` flag to poddl for efficient incremental downloads.

## Fixed in v2.10.0

- ✅ **Podcast folder never indexed into the library** — `cmd/server/main.go` only iterated `MEDIA_ROOTS` for both initial scan and rescan, so files in `PODCAST_DIR` (which is normally outside `MEDIA_ROOTS`) never reached the `tracks` table. Added a `scanRoots(cfg)` helper that returns `MEDIA_ROOTS ∪ {PODCAST_DIR}` deduplicated, used in both code paths. Episodes downloaded after this fix appear in the library tagged as `media_kind='podcast'` (the scanner already had the path-based detection).
- ✅ **Podcast downloads invisible on Downloads page** — Now register with `downloader.API` via `JobSink` interface; appear with `kind="podcast"` badge alongside music jobs.
- ✅ **`exit status 0xffffffff` was the only error users saw** — poddl stdout/stderr now streamed into the job log line-by-line (`streamProcessOutput`) so users see real-time output and the actual error message.
- ✅ **No `os.MkdirAll` before `os.Create`** — output dir is now created before any file write so a missing `PODCAST_DIR` doesn't fail the download.
- ✅ **No User-Agent on HTTP requests** — set `User-Agent: Lexicon/1.0 (+podcast)` to avoid 403s from CDNs (acast, buzzsprout) that reject Go's default UA.
- ✅ **HTTP errors said only "HTTP 403"** — non-200 responses now record status + first 512 chars of body so the user sees the actual error message.
- ✅ **Filename collisions** — episodes are now saved as `<feedID>-<episodeID>-<sanitizedTitle>.<ext>` to guarantee uniqueness across feeds.
- ✅ **Stale `download_error` after success** — `downloaded=1` updates now set `download_error=NULL`.
- ✅ **`doDownloadFeed` errors invisible** — bulk feed download errors are now written to both the unified job log and `podcast_feeds.last_error`.
- ✅ **Frontend polling timed out at 2 minutes** — bumped to 30 minutes (3s × 600 attempts) and added `download_error` detection so failed downloads finish cleanly instead of waiting for timeout.

## Phase 7: Playback Position Tracking (v3.3.5)

### Problem
Podcast episodes had no playback position tracking. Users couldn't resume from where they left off, and there was no way to see which episodes had been partially or fully listened to.

### Changes Made

#### DB (`db.go`)
- Added `playback_position_sec INTEGER NOT NULL DEFAULT 0` to `podcast_episodes`
- Added `listened INTEGER NOT NULL DEFAULT 0` to `podcast_episodes`
- Both columns have idempotent migrations

#### Backend (`podcaster.go`)
- `EpisodeJSON` type: added `PlaybackPositionSec` and `Listened` fields
- `listEpisodes`: SELECT now includes `playback_position_sec` and `listened`
- New endpoint `POST /api/podcasts/episodes/{id}/position` — saves playback position; auto-marks as listened if completed or position > 90% of duration
- New endpoint `GET /api/podcasts/episodes/{id}/position` — returns saved position and listened state

#### Frontend (`PlayerContext.tsx`, `PodcastsPage.tsx`, `api.ts`)
- `PlayerCtx` interface: added `setPodcastEpisodeId(episodeId)` method
- Player tracks current podcast episode ID and saves position every 5 seconds
- Position saved on page unload via `sendBeacon`
- Position saved when switching tracks or playing non-podcast content
- `PodcastsPage`: shows progress bar on partially-listened episodes, "✓ Listened" badge, "Resume from MM:SS" tooltip
- `handlePlayEpisode`: accepts optional `startPositionSec` parameter and seeks after loading
- `api.ts`: added `podcastEpisodePosition()`, `savePodcastEpisodePosition()`, and `PodcastEpisode` type updated with new fields

## Phase 8: Concurrency Control & Shutdown Grace Period (v3.5.3)

### Problem
Podcast downloads suffered from "context canceled" errors during app shutdown. The root cause was that `Shutdown()` immediately fired `shutdownCancel()` with zero grace period. Additionally, there was no concurrency control — any number of download goroutines could be spawned simultaneously.

### Changes Made

#### Concurrency control
- Added `sema chan struct{}` (capacity 3) to `API` struct — mirrors music downloader's semaphore pattern
- Added `inflight atomic.Int64` counter for status API
- Removed dead `mu sync.Mutex` field (never used)
- All 3 fire-and-forget goroutine entry points (`syncFeed`, `downloadEpisode`, `downloadFeed`) now:
  1. Acquire semaphore slot before spawning goroutine
  2. Increment inflight counter
  3. Defer release + decrement
  4. Include panic recovery with stack trace logging

#### Shutdown grace period
- `Shutdown()` rewritten: waits up to 30s for in-flight downloads to complete (by draining semaphore), then cancels context
- Logs: `"[podcaster] all downloads completed before shutdown"` or `"[podcaster] shutdown: 30s grace period expired, cancelling N remaining downloads"`

#### Shutdown sequence (main.go)
- Reordered: subsystems shut down BEFORE HTTP server (previously: HTTP server first killed handlers, then subsystems were cancelled)
- New order: podcastAPI.Shutdown() → dlAPI.Shutdown() → ... → srv.Shutdown()
- Signal name now logged: `"[lexicon] received SIGTERM, shutting down..."` instead of generic `"shutting down..."`

#### Status API
- `GET /api/podcasts/status` now returns `"inflight_downloads"` field showing active download count

#### Hardening
- `doSyncFeed`: added `http.Client{Timeout: 30s}` to gofeed parser (was unbounded)
- `downloadDirectAudio`: removed redundant `http.Client{Timeout: 30m}` (context already has timeout); added dedicated `http.Transport` with generous timeouts for large podcast files
- `downloadDirectAudio`: added 2 GB `io.LimitReader` on response body to prevent disk exhaustion from misconfigured CDNs

- Adding a new feed field: update `podcast_feeds` table in `db.go`, update `subscribe()` and `doSyncFeed()`
- Adding a new episode field: update `podcast_episodes` table in `db.go`, update `subscribe()` and `doSyncFeed()`
- Changing download behavior: edit `doDownloadEpisode()` or `doDownloadFeed()`
- **Always use `audioURL` (direct URL) when available, not `feedURL`**
