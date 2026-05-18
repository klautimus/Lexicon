# Lexicon Backend — Development Context

> **Zero-context onboarding for the Go backend.**
> **Parent:** [Lexicon root](../development_context.md)
> **Stack:** Go 1.22+, chi router, SQLite (WAL + FTS5), godotenv
> **Last updated:** 2026-05-17

## Purpose

The backend is a single-binary Go server that:
1. Scans local media folders for audio files (MP3, FLAC, M4A, OGG, Opus, WAV, AAC)
2. Extracts ID3/FLAC/MP4 metadata
3. Serves a REST API for the React frontend
4. Streams audio with HTTP range support
5. Integrates with Spotify for history sync
6. Calls DeepSeek for AI recommendations and chat
7. Downloads media via external tools (SpotiFLAC → yt-dlp → spotDL)
8. Manages podcast feeds via RSS + poddl subprocess
9. Provides web search (DuckDuckGo + SearXNG) for chat enrichment
10. WebSocket hub for multi-device playback control

## Entry Point

`cmd/server/main.go` — loads `.env`, opens DB, constructs all sub-APIs, sets up chi router, starts background scan, serves embedded SPA.

Start: `go run ./cmd/server` (from `backend/`)

## Architecture Pattern

Every functional area follows the same pattern:
```go
type API struct { db *sql.DB }          // or *API for more complex ones
func New(db *sql.DB) *API { ... }       // constructor
func (a *API) Mount(r chi.Router) { ... } // route registration
```

Each package is self-contained — owns its routes, handlers, and data types.

## Sub-Packages

| Package | File | LOC | Purpose | Changed in v2? |
|---------|------|-----|---------|:---:|
| `cmd/server` | `main.go` | 395 | Entry point, wiring, CORS, QR, network | ✅ |
| `internal/config` | `config.go` | 89 | Env var loading (.env) | ✅ |
| `internal/db` | `db.go` | 224 | SQLite schema, migrations | ✅ |
| `internal/models` | `models.go` | 108 | Canonical Track struct + ScanTrack | 🆕 |
| `internal/auth` | `middleware.go` | 27 | API key auth (Bearer token) | 🆕 |
| `internal/scanner` | `scanner.go` | 108 | Filesystem walk + metadata | ✅ |
| `internal/library` | `library.go` + `cover.go` | 263+25 | Track/album/artist CRUD | ✅ |
| `internal/streamer` | `streamer.go` | 45 | Range-request audio | ✅ |
| `internal/history` | `history.go` | 91 | Play recording | ✅ |
| `internal/analytics` | `analytics.go` | 160 | SQL aggregations | ✅ |
| `internal/recommender` | `recommender.go` | 793 | DeepSeek client + Spotify enrichment | ✅ |
| `internal/spotify` | 4 files | 122+221+439+219 | Spotify integration | ✅ |
| `internal/playlists` | `playlists.go` | 233 | Playlist CRUD | 🆕 |
| `internal/downloader` | `downloader.go` | 1182 | 3-tier download pipeline | 🆕 |
| `internal/podcaster` | `podcaster.go` | ~566 | Podcast RSS + poddl subprocess | 🆕 |
| `internal/playerws` | `hub.go` | 269 | WebSocket player control | 🆕 |
| `internal/websearch` | 4 files | ~540 | Web search + extraction | 🆕 |

## API Routing

All routes are prefixed `/api/`. The chi router distinguishes `/api/*` from static file requests (`/dist/*`). Each package's `Mount()` method registers its routes.

## Environment Variables

See `backend/.env.example` for the full template. Key vars:
- `MEDIA_ROOTS` — semicolon-separated paths on Windows
- `DEEPSEEK_API_KEY` — required for recommendations
- `SPOTIFY_CLIENT_ID/SECRET` — required for Spotify + downloader
- `SPOTIFLAC_BIN`, `YTDLP_BIN`, `SPOTDL_BIN`, `FFMPEG_BIN` — downloader tool paths
- `PODDL_BIN` — path to poddl.exe for podcast downloads
- `PODCAST_DIR` — where podcast episodes are saved
- `LEXICON_API_KEY` — optional API key for write operations
- `TIMEZONE` — for analytics heatmap (default: "local")
- `CORS_ORIGINS` — optional CORS origins (default: dynamic private-network)
- `DOWNLOAD_CONCURRENCY` — max concurrent downloads (default: 2)
- `WEBSEARCH_ENABLED` — enable web search in chat (default: true)

## Working on the Backend

1. Read the specific package's `development_context.md` first
2. Config changes → update `config.go` + `.env.example`
3. New routes → add to `main.go` + create/update internal package
4. DB changes → update schema in `db.go` with additive migrations
5. Rebuild: `go build -o server.exe ./cmd/server`
6. **Update `development_context.md` files after every change**
