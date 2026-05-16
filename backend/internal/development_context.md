# backend/internal — Package Overview

> **Parent:** [backend development context](../development_context.md)

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
  ├── playlists.New(db)      → playlist CRUD (new v2)
  ├── streamer.New(db)       → audio streaming
  ├── history.New(db)        → play recording
  ├── analytics.New(db)      → SQL aggregations
  ├── recommender.New(db, cfg) → DeepSeek integration
  ├── spotify.New(db, cfg)   → Spotify OAuth + sync
  └── downloader.New(cfg, db, doRescan) → 3-tier download pipeline
```

## Packages

| Package | LOC | Changed v2? | See |
|---------|-----|:---:|-----|
| `config` | 58 | ✅ | [config/](config/development_context.md) |
| `db` | 160 | No | [db/](db/development_context.md) |
| `scanner` | 101 | No | [scanner/](scanner/development_context.md) |
| `library` | 261 | No | [library/](library/development_context.md) |
| `streamer` | 44 | No | [streamer/](streamer/development_context.md) |
| `history` | 87 | No | [history/](history/development_context.md) |
| `analytics` | 123 | No | [analytics/](analytics/development_context.md) |
| `recommender` | 388 | ✅ | [recommender/](recommender/development_context.md) |
| `spotify` | 620 | No | [spotify/](spotify/development_context.md) |
| `playlists` | 215 | 🆕 | [playlists/](playlists/development_context.md) |
| `downloader` | 765 | 🆕 | [downloader/](downloader/development_context.md) |

## Cross-Package Concerns

- **Track type duplication:** `Track` struct is defined in both `library` and `playlists`. Must keep in sync.
- **DB handle sharing:** All packages receive the same `*sql.DB`. No connection pooling or per-package isolation.
- **Error handling inconsistency:** Some packages ignore `rows.Scan()` errors (`_`), others return 500.
- **No middleware:** No auth, no rate limiting, no request logging beyond chi's built-in Logger middleware.

## Dependencies (go.mod)

Key external packages:
- `github.com/go-chi/chi/v5` — HTTP router
- `github.com/go-chi/cors` — CORS middleware
- `github.com/joho/godotenv` — .env loader
- `modernc.org/sqlite` — pure-Go SQLite driver (no CGO)
- `github.com/dhowden/tag` — ID3/FLAC/MP4 tag reader
- `github.com/google/uuid` — UUID generation (downloader)
