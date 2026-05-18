# analytics — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/analytics/analytics.go` (160 LOC)
> **Last updated:** 2026-05-17

## Purpose

Pure SQL aggregations over the `plays` table — no extra services needed. Provides overview stats, top artists/tracks/genres, and a time-of-day heatmap.

## Routes

| Method | Path | Returns |
|--------|------|---------|
| `GET` | `/api/analytics/overview` | `{total_plays, unique_tracks, listen_sec, completed_pct}` |
| `GET` | `/api/analytics/top-artists` | `[{artist, plays, listen_sec}]` (top 20) |
| `GET` | `/api/analytics/top-tracks` | `[{id, title, artist, plays}]` (top 20) |
| `GET` | `/api/analytics/top-genres` | `[{genre, plays}]` (top 15) |
| `GET` | `/api/analytics/heatmap` | `[{dow, hour, plays}]` (7×24 grid) |

## Heatmap Query

Uses `TIMEZONE` env var (default "local" → "localtime"):
```sql
SELECT CAST(strftime('%w', started_at, 'unixepoch', '<tz>') AS INTEGER) AS dow,
       CAST(strftime('%H', started_at, 'unixepoch', '<tz>') AS INTEGER) AS hour,
       COUNT(*) FROM plays GROUP BY dow, hour
```

## Known Issues

- **Top genres may be empty** — depends on local files having genre tags and Spotify sync populating genres
- **No date range filtering** — always returns all-time stats
- **No caching** — every request hits the DB

## Working Here

- Adding a new aggregate view: add route to `Mount()`, write handler function
- Adding date filtering: add query params, modify SQL WHERE clauses
