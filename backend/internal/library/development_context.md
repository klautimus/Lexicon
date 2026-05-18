# library — Development Context

> **Parent:** [backend](../development_context.md)
> **Files:** `backend/internal/library/library.go` (263 LOC), `cover.go` (25 LOC)
> **Last updated:** 2026-05-17

## Purpose

Primary data access layer for the `tracks` table. Provides CRUD operations, full-text search (FTS5), grouped views (albums/artists/podcasts), cover art serving, and library stats.

## Routes

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/library/stats` | Counts of tracks/albums/artists/podcasts |
| `GET` | `/api/library/tracks?kind=music&limit=200&offset=0` | Paginated track list (returns `{tracks, total}`) |
| `GET` | `/api/library/albums` | Albums grouped by artist |
| `GET` | `/api/library/artists` | Artists with track/album counts |
| `GET` | `/api/library/podcasts` | Podcast shows with episode counts |
| `GET` | `/api/library/search?q=...` | FTS5 full-text search (max 100 results) |
| `GET` | `/api/library/track/{id}` | Single track by ID |
| `DELETE` | `/api/library/track/{id}` | ⚠️ **Deletes file from disk** + removes DB row (cascades to plays) |
| `GET` | `/api/library/cover/{id}` | Embedded cover art (24h cache, reads from audio file via dhowden/tag) |

## Track Model

Uses the **canonical** `models.Track` struct from `backend/internal/models/models.go`. Do NOT redefine Track in this package — import it.

```go
import "github.com/kevin/lexicon/internal/models"
// Use models.Track, models.TrackCols, models.ScanTrack
```

## FTS5 Search

Splits query into tokens, wraps each in double-quotes, joins with `AND`:
```sql
SELECT t.id, t.title, ... FROM tracks_fts f JOIN tracks t ON t.id=f.rowid
WHERE tracks_fts MATCH ? ORDER BY rank LIMIT 100
```

Uses `models.TrackColsAliased("t")` to avoid ambiguous column names in the JOIN.

## Working Here

- Adding a new track field: update `models.Track` struct, `models.TrackCols` constant, `models.ScanTrack()` function, AND the DB schema in `db.go`.
- Adding a new query endpoint: add to `Mount()`, write handler.
- Changing search behavior: edit the FTS5 query construction in `search()`.
