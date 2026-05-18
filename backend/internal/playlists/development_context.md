# playlists — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/playlists/playlists.go` (233 LOC) — 🆕 NEW in v2
> **Last updated:** 2026-05-17

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

Uses the **canonical** `models.Track` from `backend/internal/models/models.go`:
```go
type Playlist struct {
    ID            int64  `json:"id"`
    Name          string `json:"name"`
    TrackCount    int    `json:"track_count"`
    TotalDuration int    `json:"total_duration"`
    CreatedAt     int64  `json:"created_at"`
}

type PlaylistTrack struct {
    models.Track  // embedded canonical Track
    Position int  `json:"position"`
}

type PlaylistWithTracks struct {
    ID            int64          `json:"id"`
    Name          string         `json:"name"`
    TrackCount    int            `json:"track_count"`
    TotalDuration int            `json:"total_duration"`
    CreatedAt     int64          `json:"created_at"`
    Tracks        []PlaylistTrack `json:"tracks"`
}
```

## Position-Based Ordering

`playlist_items.position` auto-increments via SQL:
```sql
INSERT INTO playlist_items (playlist_id, track_id, position)
SELECT ?, ?, COALESCE((SELECT MAX(position)+1 FROM playlist_items WHERE playlist_id=?), 0)
```

Removing a track by position leaves a gap — no renumbering happens.

## Working Here

- Adding reordering: create a PUT endpoint that accepts new position array
- Adding duplicate check: query before INSERT in addTrack
