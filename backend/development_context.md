# Lexicon Backend — Development Context

> **Zero-context onboarding for the Go backend.**
> **Parent:** [Lexicon root](../development_context.md)
> **Stack:** Go 1.22+, chi router, SQLite (WAL + FTS5), godotenv

## Purpose

The backend is a single-binary Go server that:
1. Scans local media folders for audio files (MP3, FLAC, M4A, OGG, Opus, WAV, AAC)
2. Extracts ID3/FLAC/MP4 metadata
3. Serves a REST API for the React frontend
4. Streams audio with HTTP range support
5. Integrates with Spotify for history sync
6. Calls DeepSeek for AI recommendations and chat
7. Downloads media via external tools (SpotiFLAC → yt-dlp → spotDL)

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
| `cmd/server` | `main.go` | 173 | Entry point, wiring | ✅ |
| `internal/config` | `config.go` | 58 | Env var loading (.env) | ✅ |
| `internal/db` | `db.go` | 160 | SQLite schema, migrations | No |
| `internal/scanner` | `scanner.go` | 101 | Filesystem walk + metadata | No |
| `internal/library` | `library.go` + `cover.go` | 236+25 | Track/album/artist CRUD | No |
| `internal/streamer` | `streamer.go` | 44 | Range-request audio | No |
| `internal/history` | `history.go` | 87 | Play recording | No |
| `internal/analytics` | `analytics.go` | 123 | SQL aggregations | No |
| `internal/recommender` | `recommender.go` | 388 | DeepSeek client | ✅ |
| `internal/spotify` | `spotify.go` + `oauth.go` + `client.go` + `sync.go` | 50+227+157+186 | Spotify integration | No |
| `internal/playlists` | `playlists.go` | 215 | Playlist CRUD | 🆕 |
| `internal/downloader` | `downloader.go` | 765 | 3-tier download pipeline | 🆕 |

## API Routing

All routes are prefixed `/api/`. The chi router distinguishes `/api/*` from static file requests (`/dist/*`). Each package's `Mount()` method registers its routes.

## Known Bugs (Backend-Specific)

1. **Listen time always 0** — `PlayerContext.tsx` swallows audio load failures but the backend correctly accepts plays. Frontend issue, not backend.
2. **Top genres always empty** — Spotify sync doesn't populate genre field (sync.go INSERT unchanged).
3. **Album name truncation** — Frontend issue (TrackList.tsx).
4. **Heatmap timezone** — Uses server `TZ`, not configurable.
5. **No row error checking** — `rows.Scan()` errors ignored in library.go, analytics.go.
6. **Recommender silent fallback** — JSON parse error returns `RecsPayload{Summary: reply}` with no logging.
7. **PKCE race condition** — Same code, not fixed in v2.
8. **Download jobs lost on restart** — In-memory queue, no persistence.
9. **No download rate limiting** — Sequential yt-dlp processes can fire without throttling.
10. **Playlist generation not cached** — Each call hits DeepSeek.
11. **Track type duplication** — `Track` struct defined in both `library` and `playlists` packages.

## Environment Variables

See `backend/.env`. Key vars:
- `MEDIA_ROOTS` — semicolon-separated paths on Windows
- `DEEPSEEK_API_KEY` — required for recommendations
- `SPOTIFY_CLIENT_ID/SECRET` — required for Spotify + downloader
- `SPOTIFLAC_BIN`, `YTDLP_BIN`, `SPOTDL_BIN`, `FFMPEG_BIN` — downloader tool paths

## Working on the Backend

1. Read the specific package's `development_context.md` first
2. Config changes → update `config.go` + `.env`
3. New routes → add to `main.go` + create/update internal package
4. DB changes → update schema in `db.go` with additive migrations
5. Rebuild: `go build -o server.exe ./cmd/server`
