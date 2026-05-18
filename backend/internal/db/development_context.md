# db — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/db/db.go` (186 LOC)
> **Last updated:** 2026-05-17

## Purpose

Opens SQLite database with WAL mode and runs schema migrations. All other packages receive the `*sql.DB` handle and run queries directly — there is no ORM or repository layer.

## Open

```go
func Open(path string) (*sql.DB, error)
```

Uses `modernc.org/sqlite` (pure Go SQLite driver). Enables WAL journal mode, 5s busy timeout, foreign keys. Creates parent directory if needed.

## Schema (8 tables)

| Table | Purpose |
|-------|---------|
| `tracks` | Audio file metadata (path, title, artist, album, genre, mime, etc.) |
| `tracks_fts` | FTS5 virtual table for full-text search (title, artist, album, genre) |
| `plays` | Listening history (track_id, started_at, duration, completed, source) |
| `playlists` | Playlist names and creation timestamps |
| `playlist_items` | Playlist ↔ Track junction (position-based ordering) |
| `recommendations` | Cached DeepSeek recommendations (JSON payload, type, prompt_hash) |
| `spotify_tokens` | Spotify OAuth tokens (singleton row, id=1) |
| `spotify_pkce` | PKCE state → code_verifier mapping (temporary, legacy — verifiers now in-memory) |
| `download_jobs` | Download job metadata (id, url, status, tool, timestamps) |

## FTS Triggers

Three triggers keep `tracks_fts` in sync:
- `tracks_ai` — after INSERT → insert into fts
- `tracks_ad` — after DELETE → delete from fts
- `tracks_au` — after UPDATE → delete old + insert new

## Migration Strategy

```go
func Migrate(db *sql.DB) error
```

1. Runs the full `schema` constant (all `CREATE TABLE IF NOT EXISTS`)
2. Checks for missing columns via `columnExists()` and adds them with `ALTER TABLE`
3. Creates missing indexes

This is **addive only** — no destructive migrations, no version tracking.

### Added columns (post-v1):
- `recommendations.type` — differentiates general vs playlist cache
- `tracks.spotify_id` — links to Spotify catalog
- `tracks.external_url` — external source URL
- Unique partial index on `tracks.spotify_id` (WHERE NOT NULL)

## Working Here

- Adding a new table: add `CREATE TABLE IF NOT EXISTS` to `schema` constant.
- Adding a column: add `ALTER TABLE` guarded by `columnExists()` in `Migrate()`.
- Adding an index: add `CREATE INDEX IF NOT EXISTS` in `Migrate()`.
