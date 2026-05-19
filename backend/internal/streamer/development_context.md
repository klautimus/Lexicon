# streamer — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/streamer/streamer.go` (53 LOC)
> **Last updated:** 2026-05-18

## Purpose

Streams audio files with HTTP range-request support. Simple pass-through: looks up file path in DB, opens file, serves via `http.ServeContent`.

## Route

```
GET /api/stream/{id}
```

## Handler

1. Handle `OPTIONS` preflight — return 200 immediately (for CORS)
2. Parse `id` from URL param
3. Query `SELECT path, mime FROM tracks WHERE id=?`
4. `os.Open(path)`
5. `f.Stat()` for size/modtime
6. Set CORS headers (`Access-Control-Allow-Origin`, etc.) + `Content-Type`, `Accept-Ranges: bytes`, `Cache-Control: public, max-age=86400`
7. `http.ServeContent(w, r, name, modTime, f)` — handles Range headers automatically

## CORS Headers

Set on every response to enable Web Audio API access from cross-origin contexts:
- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, HEAD, OPTIONS`
- `Access-Control-Allow-Headers: Range, Accept-Encoding`
- `Access-Control-Expose-Headers: Content-Length, Content-Range`

## Working Here

- Changing streaming behavior: edit the `stream()` handler
- Adding transcoding: insert ffmpeg subprocess before `ServeContent`
