# history — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/history/history.go` (91 LOC)
> **Last updated:** 2026-05-17

## Purpose

Records when a track is played and returns recent play history. The frontend's `PlayerContext` calls `POST /api/history/play` with timing data.

## Routes

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/history/play` | Record a play event |
| `GET` | `/api/history/recent` | Last 50 plays with track info |

## Record Play (`POST`)

Request body:
```json
{
  "track_id": 123,
  "duration_played_sec": 180,
  "completed": true,
  "source": "local",
  "started_at": 1715894400
}
```
- `started_at` defaults to `time.Now().Unix()` if not provided
- `source` defaults to `"local"` if not provided
- `completed` converted from bool to int (0/1)
- Plays under 5 seconds are ignored by the **frontend** (not enforced here)

## Known Issues

- No deduplication — rapid seek events could generate duplicate rows
- No minimum duration enforcement — backend accepts any duration_played_sec

## Working Here

- Adding a new play source: add to the `source` column (no enum constraint)
- Adding analytics on plays: query the `plays` table from the analytics package
