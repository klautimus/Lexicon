# Downloader Package — Development Context

## Overview

The `downloader` package integrates multiple download tools (SpotiFLAC, yt-dlp, spotDL) to fetch audio files from Spotify URLs and free-text search queries. It manages job lifecycle, concurrency, logging, and database persistence.

## Architecture

- **`API`** — Main struct holding config, DB connection, job map, and semaphore for concurrency control.
- **`Job`** — Represents a download task with status, log, timing, and tool tracking.
- **`Config`** — Holds binary paths, output directory, format settings, and API keys.

### Download Flow

1. **Spotify URL** → `run()` → SpotiFLAC (primary) → yt-dlp (fallback) → spotDL (last resort)
2. **Search query** → `runSearch()` → yt-dlp (with optional DeepSeek query parsing)

### Concurrency

A semaphore (`a.sema`) limits concurrent downloads. Jobs are tracked in-memory and persisted to SQLite.

## Phase 1: Post-Download File Validation & Auto-Retry

### Problem

~12% of files downloaded by yt-dlp won't play because yt-dlp's ffmpeg post-processor fails silently, leaving files with wrong content/extension. The `isValidAudioFile()` function existed but was never called. `validateOutput()` was a no-op.

### Changes Made

#### New Functions

- **`verifyDownloadedFile(path string) error`** — Validates a downloaded file by:
  - Checking file exists and is ≥ 10KB
  - Running ffprobe to verify an audio stream is present (if `FfprobeBin` is configured)

- **`findDownloadedFile(before time.Time) string`** — Searches the output directory for recently created audio files (modified within a 5-minute window around the job start time). Returns the most recently modified matching file.

#### Modified Functions

- **`validateOutput(job *Job) string`** — Replaced no-op with actual validation logic. Calls `findDownloadedFile` + `verifyDownloadedFile`. Returns an error string (empty string = valid).

- **`runSearch()`** — After yt-dlp succeeds, validates the downloaded file. If invalid:
  1. Logs the validation failure
  2. Retries with `ytsearch2:` (second YouTube result) and `m4a` format (avoids ffmpeg conversion)
  3. Validates the retry file
  4. Fails the job if retry also produces an invalid file

- **`run()` (Tier 2 yt-dlp fallback)** — Same validation + retry pattern integrated into the `fallbackErr == nil` block.

### Retry Strategy

- Uses `ytsearch2:` to get the second YouTube result (different from the first attempt)
- Uses `m4a` format to avoid ffmpeg post-processing (which is the root cause of corruption)
- Includes `--postprocessor-args "ffmpeg:-abort_on_error 1 -v warning"` for hard failures
- Uses `--extractor-args "youtube:player_client=android"` for reliability

### Configuration

- `FfprobeBin` field in `Config` — path to ffprobe binary. Auto-detected from `FfmpegBin` if empty. Validation is skipped if not set (graceful degradation).

## File Locations

- Main file: `backend/internal/downloader/downloader.go` (1188 LOC)
- Key line ranges:
  - `verifyDownloadedFile`: ~line 986
  - `findDownloadedFile`: ~line 1016
  - `validateOutput`: ~line 1050
  - `runSearch` validation + retry block: ~line 882
  - `run` Tier 2 validation + retry block: ~line 673
  - `run` Tier 3 validation block: ~line 791

## Phase 2: Hardened yt-dlp Flags

Added to both `run()` and `runSearch()` yt-dlp argument lists:
- `--abort-on-error` — Fail fast on download errors
- `--retries 3 --fragment-retries 10` — Resilience against network issues
- `--postprocessor-args "ffmpeg:-abort_on_error 1 -v warning"` — Make ffmpeg failures fatal
- `--extractor-args "youtube:player_client=android"` — More reliable YouTube access

## Phase 3: Player Auto-Skip (Frontend)

In `frontend/src/player/PlayerContext.tsx`:
- `onError` handler auto-skips to next track after 1.5s delay
- `loadAndPlay()` catch handler also auto-skips
- `consecutiveErrorsRef` tracks consecutive failures (max 5) to prevent infinite loops
- Counter resets on successful playback

## Phase 4: Scanner Size Validation

In `backend/internal/scanner/scanner.go`:
- `indexFile()` skips files < 10KB (logged as suspicious)
- Catches corrupt files already in the library from before the fix

## Phase 5: External Job API (v2.10.0)

The `Job` struct gained a `Kind` field (`"music"` or `"podcast"`) and three new public methods on `*API` let other packages (notably `podcaster`) participate in the unified job system:

- `RegisterExternalJob(kind, url, output, tool string) string` — creates a new job in `running` state, persists to `download_jobs`, returns the generated job ID
- `AppendExternalLog(id, line string)` — appends a line to the in-memory log of an external job
- `FinishExternalJob(id string, status Status, errMsg string)` — finalizes an external job (succeeded/failed) and writes the final state to the DB

These methods don't acquire the concurrency semaphore or trigger rescan — the external caller handles those. The DB schema gained a `kind TEXT NOT NULL DEFAULT 'music'` column with idempotent migration.

## Phase 6: Post-Download Track Resolution (v3.3.5)

### Problem
AI playlist creation had a race condition: after yt-dlp downloaded a file, the async rescan hadn't indexed it yet when the frontend tried to find the track in the library. This caused tracks to be added to the playlist with wrong IDs or marked as failed.

### Changes Made

#### Backend (`downloader.go` — `runSearch`)
After yt-dlp succeeds and the async rescan is triggered, a background goroutine polls the database for up to 2 minutes (40 attempts × 3s) looking for a track whose `path` matches the downloaded file. When found, it sets `job.TrackID`, which the frontend can then use as Case A (immediate track_id) instead of racing with the scanner.

#### Frontend (`contexts/DownloadContext.tsx` — `createAiPlaylist`)
- Sub-case B2: increased retry budget from 15×2s (30s) to 60×3s (3 minutes)
- Added explicit `api.scan()` call before starting search retries to kick the scanner
- Added 3-second initial delay after scan trigger to give scanner time to start
- Added user-visible error toast when a track can't be found after all retries
