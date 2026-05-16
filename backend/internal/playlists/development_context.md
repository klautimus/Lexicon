# playlists — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/playlists/playlists.go` (215 LOC) — 🆕 NEW in v2

## Purpose

Full CRUD API for playlists. Manages `playlists` and `playlist_items` tables. Position-based ordering with auto-increment on add.

## Routes

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/playlists` | List all playlists with track counts + total duration |
| `POST` | `/api/playlists` | Create playlist (`{"name": "..."}`) |
| `GET` | `/api/playlists/{id}` | Get playlist with embedded tracks |
| `PUT` | `/api/playlists/{id}` | Rename playlist |
| `DELETE` | `/api/playlists/{id}` | Delete (cascades to items via FK) |
| `POST` | `/api/playlists/{id}/tracks` | Add track (`{"track_id": 123}`) |
| `DELETE` | `/api/playlists/{id}/tracks/{position}` | Remove track by position |

## Data Types

```go
type Playlist struct {
    ID            int64  `json:"id"`
    Name          string `json:"name"`
    TrackCount    int    `json:"track_count"`
    TotalDuration int    `json:"total_duration"` // computed client-side in list; server-computed in get
    CreatedAt     int64  `json:"created_at"`
}

type PlaylistWithTracks struct {
    Playlist
    Tracks []Track `json:"tracks"`
}
```

## Position-Based Ordering

`playlist_items.position` auto-increments via SQL:
```sql
INSERT INTO playlist_items (playlist_id, track_id, position)
SELECT ?, ?, COALESCE((SELECT MAX(position)+1 FROM playlist_items WHERE playlist_id=?), 0)
```

Removing a track by position leaves a gap — no renumbering happens.

## ⚠️ TRACK TYPE DUPLICATION

The `Track` struct and `scanTrack()` function are **duplicated** from `library/library.go`. They have identical fields but are separate definitions. Changes to `library.Track` (adding a field, changing JSON tags) must be manually propagated here.

```go
// Identical to library.Track but defined separately:
type Track struct {
    ID          int64  `json:"id"`
    Title       string `json:"title"`
    // ... all same fields as library.Track
}
```

## List View

Lists all playlists with aggregated stats via LEFT JOIN:
```sql
SELECT p.id, p.name, COUNT(i.track_id), COALESCE(SUM(t.duration_sec),0), p.created_at
FROM playlists p
LEFT JOIN playlist_items i ON i.playlist_id = p.id
LEFT JOIN tracks t ON t.id = i.track_id
GROUP BY p.id
ORDER BY p.created_at DESC
```

## Get with Tracks

Fetches playlist metadata, then joins `playlist_items` with `tracks` to embed full track list. Total duration computed by summing track durations in Go loop.

## Known Issues

1. **Track type duplication** — must manually sync with `library.Track`
2. **Position gaps** — removing track at position 2 from [0,1,2,3] leaves [0,1,3]
3. **No duplicate prevention** — same track can be added multiple times
4. **No reordering API** — no endpoint to move tracks within a playlist
5. **No bulk add** — one track per request

## Working Here

- Adding reordering: create a PUT endpoint that accepts new position array
- Adding duplicate check: query before INSERT in addTrack
- Syncing Track type: when changing `library.Track`, also update here
