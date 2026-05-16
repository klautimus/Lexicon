# library — Development Context

> **Parent:** [backend](../development_context.md)
> **Files:** `backend/internal/library/library.go` (236 LOC), `cover.go` (25 LOC)

## Purpose

Primary data access layer for the `tracks` table. Provides CRUD operations, full-text search (FTS5), grouped views (albums/artists/podcasts), cover art serving, and library stats.

## Routes

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/library/stats` | Counts of tracks/albums/artists/podcasts |
| `GET` | `/api/library/tracks?kind=music&limit=200&offset=0` | Paginated track list |
| `GET` | `/api/library/albums` | Albums grouped by artist |
| `GET` | `/api/library/artists` | Artists with track/album counts |
| `GET` | `/api/library/podcasts` | Podcast shows with episode counts |
| `GET` | `/api/library/search?q=...` | FTS5 full-text search (max 100 results) |
| `GET` | `/api/library/track/{id}` | Single track by ID |
| `DELETE` | `/api/library/track/{id}` | ⚠️ **Deletes file from disk** + removes DB row (cascades to plays) |
| `GET` | `/api/library/cover/{id}` | Embedded cover art (24h cache, reads from audio file via dhowden/tag) |

## Key Handlers Not Immediately Obvious

### deleteTrack (line 201)
⚠️ **Destructive side effect:** Before deleting the DB row, this handler calls `os.Remove(path)` to delete the actual audio file from disk. A failed file removal does NOT prevent the DB deletion — the handler continues to delete the DB row regardless.

### cover (line 222)
Reads embedded cover art from the audio file using `dhowden/tag.Picture()`. Serves with `Cache-Control: public, max-age=86400` (24h browser cache). Returns 404 if the file can't be opened or has no embedded art. Uses helper functions in `cover.go`.

### stats (line 246)
Returns `{tracks, albums, artists, podcasts}` counts. Albums = COUNT DISTINCT of non-empty album names. Artists = COUNT DISTINCT of COALESCE(album_artist, artist). Podcasts = COUNT DISTINCT of podcast show names.

## Track Model

```go
type Track struct {
    ID          int64  `json:"id"`
    Title       string `json:"title"`
    Artist      string `json:"artist"`
    AlbumArtist string `json:"album_artist"`
    Album       string `json:"album"`
    TrackNo     int    `json:"track_no"`
    DiscNo      int    `json:"disc_no"`
    Year        int    `json:"year"`
    Genre       string `json:"genre"`
    DurationSec int    `json:"duration_sec"`
    MediaKind   string `json:"media_kind"` // "music" or "podcast"
    Mime        string `json:"mime"`
    SpotifyID   string `json:"spotify_id,omitempty"`
    ExternalURL string `json:"external_url,omitempty"`
}
```

## FTS5 Search

Splits query into tokens, wraps each in double-quotes, joins with `AND`:
```sql
SELECT t.* FROM tracks_fts f JOIN tracks t ON t.id=f.rowid
WHERE tracks_fts MATCH ? ORDER BY rank LIMIT 100
```

## Known Issues

- **Track type duplication** — `Track` struct duplicated in `playlists/playlists.go`. Changes must be manually synchronized.
- **No row error checking** — `rows.Scan()` errors ignored in albums/artists/podcasts/tracks handlers (e.g., `t, _ := scanTrack(rows)`)
- **No pagination on Music page** — frontend hardcodes 500 limit
- Search uses `AND` join — all terms must match (no OR support)
- Cover art: reads from `cover_path` column set by scanner; no fallback

## Working Here

- Adding a new track field: update `Track` struct, `trackCols` constant, `scanTrack()` function, and the DB schema in `db.go`
- Adding a new query endpoint: add to `Mount()`, write handler
- Changing search behavior: edit the FTS5 query construction in `search()`
