# streamer — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/streamer/streamer.go` (45 LOC)
> **Last updated:** 2026-05-17

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
5. Sets `Content-Type`, `Accept-Ranges: bytes`, `Cache-Control: public, max-age=86400`
6. `http.ServeContent(w, r, name, modTime, f)` — handles Range headers automatically

## Working Here

- Changing streaming behavior: edit the `stream()` handler
- Adding transcoding: insert ffmpeg subprocess before `ServeContent`
