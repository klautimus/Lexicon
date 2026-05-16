# streamer — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/streamer/streamer.go` (44 LOC)

## Purpose

Streams audio files with HTTP range-request support. Simple pass-through: looks up file path in DB, opens file, serves via `http.ServeContent`.

## Route

```
GET /api/stream/{id}
```

## Handler

1. Parse `id` from URL param
2. Query `SELECT path, mime FROM tracks WHERE id=?`
3. `os.Open(path)`
4. `f.Stat()` for size/modtime
5. `http.ServeContent(w, r, name, modTime, f)` — handles Range headers automatically

## Known Issues

- No caching headers — repeated range requests re-stat the file
- No transcoding — streams raw file format, client must support it
- No access control — any track ID can be streamed
- File not found returns 404 but doesn't clean up orphaned DB rows

## Working Here

- Changing streaming behavior: edit the `stream()` handler
- Adding transcoding: insert ffmpeg subprocess before `ServeContent`
- Adding caching: set `Cache-Control` or `ETag` headers
