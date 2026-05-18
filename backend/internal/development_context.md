# backend/internal — Package Overview

> **Parent:** [backend development context](../development_context.md)
> **Last updated:** 2026-05-17

## Architecture Pattern

Every internal package follows the same structure:
1. A struct holding a `*sql.DB` reference (+ additional config as needed)
2. A `New()` constructor
3. A `Mount(r chi.Router)` method that registers routes
4. Handler methods that query the DB and return JSON

```go
type API struct { db *sql.DB }
func New(db *sql.DB) *API { return &API{db: db} }
func (a *API) Mount(r chi.Router) { r.Get("/api/...", a.handler) }
```

All packages are constructed in `cmd/server/main.go` and mounted to a single `chi.NewRouter()`.

## Package Dependency Map

```
main.go
  ├── config.Load()          → reads env vars
  ├── db.Open() + db.Migrate() → opens SQLite + runs schema
  ├── scanner.New(db)        → scan filesystem
  ├── library.New(db)        → track/album/artist CRUD
  ├── models package         → canonical Track struct (shared by library, playlists)
  ├── auth.RequireAPIKey     → middleware for write operations
  ├── playlists.New(db)      → playlist CRUD
  ├── streamer.New(db)       → audio streaming
  ├── history.New(db)        → play recording
  ├── analytics.New(db, tz)  → SQL aggregations
  ├── websearch.New(enabled) → web search (standalone, no DB)
  ├── spotify.New(db, cfg)   → Spotify OAuth + sync
  ├── recommender.New(db, cfg, ws, spotify) → DeepSeek + web search + Spotify enrichment
  ├── downloader.New(cfg, db, doRescan) → 3-tier download pipeline
  ├── podcaster.New(db, cfg, doRescan) → podcast RSS + poddl subprocess
  └── playerws.New()         → WebSocket hub for device control
```

## Packages

| Package | LOC | Changed v2? | See |
|---------|-----|:---:|-----|
| `config` | 89 | ✅ | [config/](config/development_context.md) |
| `db` | 224 | ✅ | [db/](db/development_context.md) |
| `models` | 108 | 🆕 | — |
| `auth` | 27 | 🆕 | — |
| `scanner` | 108 | ✅ | [scanner/](scanner/development_context.md) |
| `library` | 263 | ✅ | [library/](library/development_context.md) |
| `streamer` | 45 | ✅ | [streamer/](streamer/development_context.md) |
| `history` | 91 | ✅ | [history/](history/development_context.md) |
| `analytics` | 160 | ✅ | [analytics/](analytics/development_context.md) |
| `recommender` | 793 | ✅ | [recommender/](recommender/development_context.md) |
| `spotify` | ~1001 | ✅ | [spotify/](spotify/development_context.md) |
| `playlists` | 233 | 🆕 | [playlists/](playlists/development_context.md) |
| `downloader` | 1182 | 🆕 | [downloader/](downloader/development_context.md) |
| `podcaster` | ~721 | 🆕 | [podcaster/](podcaster/development_context.md) |
| `playerws` | 269 | 🆕 | — |
| `websearch` | ~540 | 🆕 | — |

## Cross-Package Concerns

- **Shared Track model:** `models.Track` is the canonical struct. Both `library` and `playlists` import from `models`. No duplication.
- **DB handle sharing:** All packages receive the same `*sql.DB`. No connection pooling or per-package isolation.
- **Error handling:** Most packages check `rows.Err()` after row iteration. A few read-only endpoints may still miss `rows.Scan()` errors — low impact.
- **Auth:** `RequireAPIKey` middleware protects POST/PUT/DELETE. No-op when `LEXICON_API_KEY` is empty.
- **CORS:** Dynamic `AllowOriginFunc` allows private network origins. Link-local (169.254.x.x) excluded.

## Dependencies (go.mod)

Key external packages:
- `github.com/go-chi/chi/v5` — HTTP router
- `github.com/go-chi/cors` — CORS middleware
- `github.com/joho/godotenv` — .env loader
- `modernc.org/sqlite` — pure-Go SQLite driver (no CGO)
- `github.com/dhowden/tag` — ID3/FLAC/MP4 tag reader
- `github.com/google/uuid` — UUID generation (downloader)
- `github.com/skip2/go-qrcode` — QR code generation
- `github.com/PuerkitoBio/goquery` — HTML parsing (websearch)
- `github.com/mmcdole/gofeed` — RSS feed parsing (podcaster)
- `github.com/gorilla/websocket` — WebSocket hub (playerws)
