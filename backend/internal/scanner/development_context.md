# scanner — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/scanner/scanner.go` (184 LOC)

## Purpose

Walks `MEDIA_ROOTS` recursively, extracts metadata from audio files using `dhowden/tag`, and upserts into the `tracks` table. Incremental — skips files whose `mtime` matches the DB row. Loudness measurement (EBU R128) runs asynchronously after indexing so it doesn't block the scan pipeline.

## Supported Audio Formats

| Extension | MIME Type |
|-----------|-----------|
| `.mp3` | `audio/mpeg` |
| `.flac` | `audio/flac` |
| `.m4a`, `.m4b` | `audio/mp4` |
| `.aac` | `audio/aac` |
| `.ogg` | `audio/ogg` |
| `.opus` | `audio/opus` |
| `.wav` | `audio/wav` |
| `.mp4` | `audio/mp4` |
| `.webm` | `audio/webm` |

## Key Functions

```go
func (s *Scanner) ScanRoot(ctx context.Context, root string) error
```
Uses `filepath.WalkDir`. For each file with a recognized extension, calls `indexFile()`. Errors on individual files are silently skipped.

```go
func (s *Scanner) indexFile(ctx context.Context, path, mime string) error
```
1. `os.Stat()` → get `mtime`
2. Query DB for existing `mtime` at this path
3. If `mtime` matches → skip (incremental)
4. `tag.ReadFrom(f)` → extract ID3/FLAC/MP4 tags
5. If title empty → use filename (without extension)
6. Classify as `"music"` or `"podcast"` (genre or path contains "podcast")
7. `INSERT ... ON CONFLICT(path) DO UPDATE` — upsert by path (loudness set to 0.0)
8. **Async:** spawns goroutine to measure loudness via ffmpeg and write back to DB

```go
func measureLoudness(ctx context.Context, path string) loudnessResult
```
Runs ffmpeg with loudnorm filter in measurement mode. **Timeout context MUST be created BEFORE `exec.CommandContext`** — otherwise the command uses the parent context and ffmpeg can hang indefinitely. Returns zero values on failure.

## ⚠️ CRITICAL: Timeout Context Ordering

The `context.WithTimeout` call MUST come BEFORE `exec.CommandContext`. If created after, the command is bound to the original (unlimited) context and the timeout is never applied. This caused scans to hang indefinitely on certain MP4/M4A files, appearing as "only 2 tracks scanned."

## ⚠️ CRITICAL: Loudness Is Async

`indexFile` now returns immediately after the DB upsert. Loudness measurement runs in a background goroutine. This means:
- `duration_sec` is NOT set during initial indexing (still 0/NULL)
- Loudness values are 0.0 until the async measurement completes
- The goroutine writes back to the DB via `UPDATE tracks SET loudness_* WHERE path=?`

## Podcast Detection

```go
kind := "music"
if strings.Contains(strings.ToLower(genre), "podcast") ||
   strings.Contains(strings.ToLower(path), "podcast") {
    kind = "podcast"
}
```

## Call Sites

- **Startup:** `main.go` starts a background goroutine that scans all roots
- **Rescan:** `POST /api/scan` triggers via `doRescan` closure
- **Downloader:** After successful download, rescan is called to pick up new files

## Known Issues

- No rate limiting or progress reporting during scan
- Errors on individual files are silently swallowed (returns nil)
- `tag.ReadFrom` opens the file a second time (after `os.Stat`)
- No handling of symlinks or reparse points on Windows
- `mtime` comparison is in seconds — file updated within the same second may be missed
- `duration_sec` is not populated during indexing (remains 0/NULL)

## Phase 4: File Size Validation (v2.7)

- `indexFile()` now skips files < 10KB (logged as suspicious)
- Prevents corrupt/incomplete downloads from being indexed
- Catches files that were downloaded before the downloader validation was added

## v3.3.4: Scan Performance Fix

- **Bug:** `measureLoudness` created timeout context AFTER `exec.CommandContext`, making the timeout ineffective. ffmpeg could hang on certain MP4/M4A files, blocking the entire scan. Only the first ~2 tracks would appear before the hang.
- **Fix 1:** Moved `context.WithTimeout` before `exec.CommandContext` so the 30-second timeout is actually enforced.
- **Fix 2:** Moved loudness measurement to an async goroutine that runs after the DB upsert. `indexFile` now returns immediately, so the scan pipeline is no longer blocked by ffmpeg. Loudness values are written back to the DB when ready.
