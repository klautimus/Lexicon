# Lexicon

A self-hosted music library and discovery platform — scan your local collection, stream lossless audio, connect Spotify, download from the web, and let an AI curator build you playlists based on your taste.

- **Backend:** Go + chi router + SQLite (WAL + FTS5) — single binary, zero external services
- **Frontend:** React 18 + Vite + TypeScript + TailwindCSS + Recharts + Lucide icons
- **AI:** DeepSeek V4 Flash — recommendations, playlist generation, search query parsing
- **Platform:** Windows-native distribution via InnoSetup installer with bundled tools

## Features

### Music Library
- **Local file scanner** — walks `MEDIA_ROOTS` recursively, extracts metadata (ID3, FLAC, MP4) via `dhowden/tag`, upserts into SQLite by path. Incremental re-scan via mtime comparison.
- **Format support** — MP3, FLAC, M4A, OGG, Opus, WAV, AAC
- **Range-request audio streaming** — scrub anywhere in a track without buffering the whole file
- **FTS5 full-text search** — instant search across titles, albums, artists
- **Embedded cover art serving** — album art extracted from audio file metadata
- **Music vs podcast detection** — auto-classified by genre tag and path keywords

### Listening Intelligence
- **Play tracking** — every play recorded with actual seconds-listened and completion status. Plays under 5 seconds are discarded.
- **Analytics dashboard** — top artists/tracks/genres, time-of-day heatmap, listening overview. Pure SQL aggregations, no extra services.
- **AI Recommendations** — DeepSeek builds a profile from your listening history (last 90 days) and generates 8–12 suggestions split between "re-listen to this" (library) and "you'd love this" (discover). Library hits auto-resolve to playable tracks.
- **Conversational recommender** — chat about your taste, ask for focused-work playlists, get artist deep-dives. Same profile context as recommendations.
- **AI Playlist Generation** — describe a mood or theme, DeepSeek curates a 8–12 track playlist with reasoning per track. Auto-downloads missing tracks before adding them.

### Spotify Integration
- **PKCE OAuth flow** — no client secret needed, full Spotify Web API access
- **History sync** — merges your Spotify listening history into Lexicon's play tracking
- **Token management** — automatic refresh, persistent storage in SQLite

### Playlists
- **Full CRUD** — create, rename, delete playlists with inline editing
- **Track management** — add/remove tracks by position, per-row "Add to Playlist" dropdown on every track list
- **Create-new inline** — create a playlist directly from any track's dropdown without leaving the page
- **Play All** — one-click playback of entire playlists

### Web Download
- **3-tier fallback pipeline:**
  1. **SpotiFLAC** — primary tool, lossless FLAC from Qobuz/Tidal via Spotify links
  2. **yt-dlp** — YouTube audio extraction with metadata embedding (falls back on SpotiFLAC failure)
  3. **spotDL** — YouTube Music search (final fallback)
- **Dual input modes** — paste a Spotify URL or type a free-text search
- **Job management** — queued downloads with live log streaming, cancel support, per-job tool tracking
- **Auto-rescan** — library refreshed automatically after each successful download
- **DeepSeek query parsing** — free-text searches parsed into structured artist/title metadata for better download accuracy
- **"Search & Download from Web"** — when Music or Search pages have no local results, one-click web download

### UI/UX
- **Plex-like interface** — persistent player bar, sidebar navigation, responsive layout
- **Toast notifications** — success/error/info feedback on all operations throughout the UI
- **Mobile-responsive** — adaptive nav bar and player bar for small screens
- **Persistent player state** — playback continues across page navigation

## Quick Start

### Prerequisites
- **Go 1.22+** — [go.dev/dl](https://go.dev/dl/)
- **Node 20+** — [nodejs.org](https://nodejs.org)
- **API keys** — DeepSeek API key (recommendations/download search), Spotify client ID (optional, for Spotify features)

### 1. Backend

```powershell
cd C:\Users\kevin\CascadeProjects\lexicon\backend

# Copy and edit config
copy .env.example .env
# Set MEDIA_ROOTS to your music/podcast folders (semicolon-separated on Windows)
# Add DEEPSEEK_API_KEY for recommendations and download search
# Add SPOTIFY_CLIENT_ID + SPOTIFY_CLIENT_SECRET for Spotify integration

go mod tidy
go run ./cmd/server
```

Backend listens on `http://localhost:8787`. Initial scan runs in the background — watch the log for `[scanner] initial scan complete`.

### 2. Frontend

```powershell
cd C:\Users\kevin\CascadeProjects\lexicon\frontend
npm install
npm run dev
```

Open `http://localhost:5173`. The Vite dev server proxies `/api` to the Go backend.

## Configuration

```env
# Server
PORT=8787
DB_PATH=./data/lexicon.db
MEDIA_ROOTS=C:/Users/kevin/Music;D:/Podcasts   # semicolon-separated on Windows

# DeepSeek (recommendations, playlist generation, download search parsing)
DEEPSEEK_API_KEY=sk-...
DEEPSEEK_MODEL=deepseek-v4-flash
DEEPSEEK_THINKING=medium
DEEPSEEK_BASE_URL=https://api.deepseek.com

# Spotify (optional — enables Spotify features)
SPOTIFY_CLIENT_ID=...
SPOTIFY_CLIENT_SECRET=...

# Download tools (paths to bundled .exe files)
SPOTIFLAC_BIN=tools/spotiflac.exe
SPOTIFLAC_OUTPUT=./downloads
YTDLP_BIN=tools/yt-dlp.exe
SPOTDL_BIN=tools/spotdl.exe
FFMPEG_BIN=tools/ffmpeg.exe
```

## API Reference

### Library
| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/library/stats` | Track/album/artist/podcast counts |
| GET | `/api/library/tracks?kind=music&limit=200` | List tracks with pagination |
| GET | `/api/library/albums` | Albums grouped view |
| GET | `/api/library/artists` | Artists grouped view |
| GET | `/api/library/podcasts` | Podcasts grouped view |
| GET | `/api/library/search?q=...` | FTS5 full-text search |
| GET | `/api/library/track/:id` | Single track detail |
| GET | `/api/library/cover/:id` | Embedded cover art |

### Streaming & History
| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/stream/:id` | Audio stream (range-request supported) |
| POST | `/api/history/play` | Record a play |
| GET | `/api/history/recent` | Last 50 plays |

### Analytics
| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/analytics/overview` | Total plays, unique tracks, completion rate |
| GET | `/api/analytics/top-artists` | Top artists by play count |
| GET | `/api/analytics/top-tracks` | Top tracks by play count |
| GET | `/api/analytics/top-genres` | Top genres by play count |
| GET | `/api/analytics/heatmap` | Time-of-day listening heatmap |

### Recommendations
| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/recommendations` | Latest cached recommendations |
| POST | `/api/recommendations/refresh` | Generate new recommendations (DeepSeek) |
| POST | `/api/recommendations/playlist` | Generate AI-curated playlist (DeepSeek) |
| POST | `/api/recommendations/chat` | Conversational recommender chat |

### Playlists
| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/playlists` | List all playlists |
| POST | `/api/playlists` | Create playlist |
| GET | `/api/playlists/:id` | Get playlist with tracks |
| PUT | `/api/playlists/:id` | Rename playlist |
| DELETE | `/api/playlists/:id` | Delete playlist |
| POST | `/api/playlists/:id/tracks` | Add track to playlist |
| DELETE | `/api/playlists/:id/tracks/:pos` | Remove track by position |

### Download
| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/download/status` | Downloader configuration status |
| POST | `/api/download` | Enqueue Spotify URL download |
| POST | `/api/download/search` | Enqueue free-text search download |
| GET | `/api/download/jobs` | List all jobs (without logs) |
| GET | `/api/download/jobs/:id` | Get job with full log |
| POST | `/api/download/jobs/:id/cancel` | Cancel running job |

### Spotify
| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/spotify/status` | Connection status |
| GET | `/api/spotify/login` | PKCE OAuth login URL |
| GET | `/api/spotify/callback` | OAuth callback handler |

### System
| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/health` | Liveness check |
| POST | `/api/scan` | Trigger library rescan |

## How It Works

### Scanner
Walks `MEDIA_ROOTS` recursively, extracts ID3/FLAC/MP4 metadata via `dhowden/tag`, and upserts tracks into SQLite by path. Files with matching `mtime` are skipped, making re-scans cheap. Re-trigger anytime via the UI or `POST /api/scan`.

### Streaming
Range-request audio streaming — the frontend requests byte ranges and the backend serves them from the file. Handles seeking, partial content (206), and full file requests. No transcoding — serves the original file bytes.

### Recommendations
`POST /api/recommendations/refresh` builds a compact natural-language profile (top artists/genres last 90 days, recently played, library snapshot) and sends it to DeepSeek with a curator prompt. Returns JSON with 8–12 recommendations split between **library** (already in your collection, re-listen) and **discover** (new suggestions). Results are cached in SQLite until the next refresh.

### AI Playlist Generation
`POST /api/recommendations/playlist` sends a curator prompt to DeepSeek that generates a themed playlist with name, description, and 8–12 tracks (each with reasoning). Tracks that exist in the library get instant `track_id` resolution. The frontend then downloads any missing tracks before adding them to the playlist.

### Download Pipeline
Each download job runs through three tiers sequentially:
1. **SpotiFLAC** runs first — if it reports "0 Success, N Failed" the pipeline advances
2. **yt-dlp** searches YouTube with `ytsearch1:<parsed query>` — if it fails, advance
3. **spotDL** searches YouTube Music as the final fallback

Free-text searches skip SpotiFLAC and optionally use DeepSeek to parse the query into structured metadata for better yt-dlp results. All subprocess output is streamed into the job log (capped at 500 lines). On success, a library rescan is automatically triggered.

### Spotify Sync
PKCE OAuth flow (no client secret required). After auth, Lexicon periodically syncs your Spotify listening history and merges it into the local play tracking table. Token refresh is automatic.

## Project Structure

```
lexicon/
├── backend/
│   ├── cmd/server/main.go              # Entry point, dependency injection, route mounting
│   └── internal/
│       ├── config/                     # Environment variable loading
│       ├── db/                         # SQLite schema, migrations, FTS5 triggers
│       ├── scanner/                    # Filesystem walk + metadata extraction
│       ├── library/                    # Track/album/artist CRUD + search + cover art
│       ├── streamer/                   # Range-request audio streaming
│       ├── history/                    # Play recording + recent plays
│       ├── analytics/                  # SQL aggregations for dashboard
│       ├── recommender/                # DeepSeek client: recommendations, playlists, chat
│       ├── spotify/                    # PKCE OAuth, token management, history sync
│       ├── playlists/                  # Playlist CRUD + track management
│       └── downloader/                 # 3-tier download pipeline + job management
├── frontend/
│   └── src/
│       ├── App.tsx                     # Shell, routing, provider hierarchy
│       ├── lib/api.ts                  # Typed API client
│       ├── lib/spotify.ts              # Spotify Web SDK wrapper
│       ├── player/PlayerContext.tsx     # Global audio player + history hooks
│       ├── contexts/ToastContext.tsx    # Toast notification system
│       ├── components/                 # PlayerBar, TrackList (with playlist dropdown)
│       └── pages/                      # Home, Music, Podcasts, Search, Analytics,
│                                       #   Discover/Recs, Downloads, Playlists, Playlist, Settings
├── release/
│   ├── build.ps1                       # PowerShell build script (Windows)
│   └── lexicon.iss                     # InnoSetup 6+ installer script
└── tools/
    ├── spotiflac.exe                   # Prebuilt FLAC downloader (bundled in installer)
    ├── yt-dlp.exe                      # YouTube audio downloader
    ├── spotdl.exe                      # Spotify/YouTube Music downloader
    ├── ffmpeg.exe / ffprobe.exe         # Audio processing
    └── development_context.md          # Tool documentation
```

Each package also contains a `development_context.md` file providing complete self-contained documentation for AI agents resuming work on the project with zero prior context.

## Distribution

Lexicon is distributed as a **Windows-native single-exe installer** built with InnoSetup. The installer bundles:

- `lexicon.exe` — Go backend with embedded frontend (via `//go:embed`)
- All download tools (`spotiflac.exe`, `yt-dlp.exe`, `spotdl.exe`, `ffmpeg.exe`, `ffprobe.exe`)
- Optional: `ngrok.exe` for remote access

Build with:
```powershell
cd release
.\build.ps1
```
Output: `LexiconSetup.exe`

## Known Limitations

- **Download jobs are in-memory** — lost on server restart. No persistence yet.
- **No authentication** — designed for single-user local use behind a firewall
- **No pagination on Music page** — hardcoded 500 track limit (mitigated by client-side filtering)
- **Album name truncation** — no max-width on album column in track table
- **Playlist generation not cached** — repeated clicks cost DeepSeek tokens
- **No download rate limiting** — generating a 12-track playlist fires 12 sequential downloads
- **Windows-only distribution** — backend compiles cross-platform but installer + bundled tools are Windows .exe files

## License

MIT
