# models — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/models/models.go` (108 LOC) — 🆕 NEW in v2
> **Last updated:** 2026-05-17

## Purpose

Canonical Track model shared by `library` and `playlists` packages. Eliminates struct duplication.

## Key Types

```go
type Track struct {
    ID          int64
    Path        string
    Title       string
    Artist      string
    AlbumArtist string
    Album       string
    TrackNo     int
    DiscNo      int
    Year        int
    Genre       string
    DurationSec int
    Mime        string
    SizeBytes   int
    MediaKind   string  // "music" | "podcast"
    CoverPath   string
    AddedAt     int64
    Mtime       int64
    SpotifyID   sql.NullString
    ExternalURL sql.NullString
}
```

## Key Constants

- `TrackCols` — comma-separated column names for SELECT queries
- `TrackColsAliased(prefix)` — same but with table alias prefix for JOINs
- `ScanTrack(rows)` — scans a `*sql.Row` into a `Track` struct

## CRITICAL: TrackCols MUST match DB schema exactly

The column names in TrackCols must exist in the `tracks` table, and the column order must match ScanTrack's `Scan` argument order. Getting either wrong produces silent runtime failures.

Source of truth is `backend/internal/db/db.go` — the `CREATE TABLE tracks` statement.

**DO NOT invent column names.** Use `size_bytes` (not `file_size`), `added_at` (not `created_at`), `mtime` (not `modified_at`).

## Working Here

- Adding a field: update `Track` struct, `TrackCols` const, `ScanTrack()` function, AND the DB schema in `db.go`
- Both `library` and `playlists` import from this package — changes affect both
