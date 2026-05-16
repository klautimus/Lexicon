# scanner — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/scanner/scanner.go` (101 LOC)

## Purpose

Walks `MEDIA_ROOTS` recursively, extracts metadata from audio files using `dhowden/tag`, and upserts into the `tracks` table. Incremental — skips files whose `mtime` matches the DB row.

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
7. `INSERT ... ON CONFLICT(path) DO UPDATE` — upsert by path

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

## Working Here

- Adding a new audio format: add to `audioExts` map
- Changing metadata extraction: edit `indexFile()` — the `dhowden/tag` library interface
- Changing podcast detection: edit the `kind` assignment logic
