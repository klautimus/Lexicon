# Lexicon

A self-hosted music library and discovery platform — scan your local collection, stream lossless audio, connect Spotify, download from the web, manage podcasts, and let an AI curator build you playlists based on your taste.

- **Backend:** Go + chi router + SQLite (WAL + FTS5) — single binary, zero external services
- **Frontend:** React 18 + Vite + TypeScript + TailwindCSS + Recharts + Lucide icons
- **AI:** DeepSeek V4 Flash — recommendations, playlist generation, search query parsing, web search enrichment
- **Platform:** Windows-native distribution via InnoSetup installer with bundled tools

## Features

### Music Library
- **Local file scanner** — walks `MEDIA_ROOTS` recursively, extracts metadata (ID3, FLAC, MP4) via `dhowden/tag`, upserts into SQLite by path. Incremental re-scan via mtime comparison.
- **Format support** — MP3, FLAC, M4A, M4B, OGG, Opus, WAV, AAC
- **Range-request audio streaming** — scrub anywhere in a track without buffering the whole file
- **FTS5 full-text search** — instant search across titles, albums, artists
- **Embedded cover art serving** — album art extracted from audio file metadata
- **Music vs podcast detection** — auto-classified by genre tag and path keywords
- **Post-download validation** — ffprobe-based verification of downloaded files; auto-retry with alternate source on corruption (~12% yt-dlp failure rate fixed)
- **Scanner size validation** — files under 10KB are skipped as suspicious

### Listening Intelligence
- **Play tracking** — every play recorded with actual seconds-listened and completion status. Plays under 5 seconds are discarded.
- **Analytics dashboard** — top artists/tracks/genres, time-of-day heatmap, listening overview. Pure SQL aggregations, no extra services.
- **AI Recommendations** — DeepSeek builds a profile from your listening history (last 90 days) and Spotify profile (top artists/tracks/playlists/saved/followed) to generate 8–12 suggestions split between "re-listen to this" (library) and "you'd love this" (discover). Library hits auto-resolve to playable tracks.
- **Conversational recommender** — chat about your taste, ask for focused-work playlists, get artist deep-dives. Same profile context as recommendations. Web search enrichment for factual questions.
- **AI Playlist Generation** — describe a mood or theme, DeepSeek curates a 5–50 track playlist with reasoning per track. Auto-downloads missing tracks before adding them. Configurable track count slider.

### Spotify Integration
- **PKCE OAuth flow** — no client secret needed, full Spotify Web API access
- **History sync** — merges your Spotify listening history into Lexicon's play tracking
- **Token management** — automatic refresh, persistent storage in SQLite
- **Device control** — Spotify Connect device discovery and playback transfer
- **WebSocket player control** — multi-device playback control via gorilla/websocket hub

### Playlists
- **Full CRUD** — create, rename, delete playlists with inline editing
- **Track management** — add/remove tracks by position, per-row "Add to Playlist" dropdown on every track list
- **Create-new inline** — create a playlist directly from any track's dropdown without leaving the page
- **Play All** — one-click playback of entire playlists

### Podcasts
- **RSS feed management** — subscribe/unsubscribe to podcast RSS feeds, sync for new episodes
- **Episode download** — download individual episodes or bulk-download entire feeds via poddl.exe
- **Direct audio download** — episodes with direct audio URLs downloaded via Go's net/http client (poddl only used for RSS-only feeds)
- **Incremental feed sync** — poddl `-h` flag stops on first existing file for efficient updates
- **Episode playback** — downloaded episodes indexed into library and playable through the main player
- **Download progress polling** — frontend polls for completion/error status with 30-minute timeout
- **Error visibility** — per-episode download error display with detailed messages

### Web Download
- **3-tier fallback pipeline:**
  1. **SpotiFLAC** — primary tool, lossless FLAC from Qobuz/Tidal via Spotify links
  2. **yt-dlp** — YouTube audio extraction with metadata embedding (falls back on SpotiFLAC failure)
  3. **spotDL** — YouTube Music search (final fallback)
- **Dual input modes** — paste a Spotify URL or type a free-text search
- **Job management** — queued downloads with live log streaming, cancel support, per-job tool tracking. Jobs persisted to SQLite with startup recovery.
- **Auto-rescan** — library refreshed automatically after each successful download
- **DeepSeek query parsing** — free-text searches parsed into structured artist/title metadata for better download accuracy
- **"Search & Download from Web"** — when Music or Search pages have no local results, one-click web download
- **Concurrency control** — configurable download concurrency (default 2) via semaphore

### Web Search Integration
- **DuckDuckGo + SearXNG** — Go-only web search package for chat enrichment and factual questions
- **Page extraction** — HTML-to-text extraction for search result pages
- **Graceful degradation** — works without internet; timeouts prevent blocking

### UI/UX
- **Plex-like interface** — persistent player bar, sidebar navigation, responsive layout
- **Toast notifications** — success/error/info feedback on all operations throughout the UI
- **Mobile-responsive** — dual layout system (DesktopLayout/MobileLayout) with adaptive player bar, nav bar, and controls
- **Persistent player state** — playback continues across page navigation
- **Auto-skip on error** — player automatically skips to next track on playback failure (max 5 consecutive errors)
- **Shuffle + Repeat** — shuffle and repeat-one/all modes
- **Volume control** — with desktop-only display to avoid mobile overflow
- **QR code for LAN** — scan to connect mobile devices to the local server
- **API key authentication** — optional `LEXICON_API_KEY` Bearer token for write operations
- **Dynamic CORS** — allows private network origins for LAN access
- **PWA support** — installable as a progressive web app with 192px/512px icons

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
TIMEZONE=America/Vancouver                      # IANA timezone for analytics

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

# Podcast
PODDL_BIN=tools/poddl.exe
PODCAST_DIR=./podcasts

# Web search
WEBSEARCH_ENABLED=true

# Auth (optional — enables API key auth on write operations)
LEXICON_API_KEY=your-secret-key

# CORS (optional — comma-separated allowed origins, defaults to dynamic private-network check)
CORS_ORIGINS=
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
| DELETE | `/api/library/track/:id` | Delete track and file |

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
| POST | `/api/recommendations/refresh` | Generate new recommendations (DeepSeek + Spotify profile) |
| POST | `/api/recommendations/playlist` | Generate AI-curated playlist (DeepSeek) |
| POST | `/api/recommendations/chat` | Conversational recommender chat with web search |

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

### Podcasts
| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/podcasts/feeds` | List subscribed feeds |
| POST | `/api/podcasts/subscribe` | Subscribe to RSS feed |
| DELETE | `/api/podcasts/feeds/:id` | Unsubscribe from feed |
| GET | `/api/podcasts/feeds/:id/episodes` | List episodes for a feed |
| POST | `/api/podcasts/feeds/:id/sync` | Sync feed for new episodes |
| POST | `/api/podcasts/episodes/:id/download` | Download an episode |
| POST | `/api/podcasts/feeds/:id/download` | Bulk download all episodes in a feed |
| GET | `/api/podcasts/episodes/:id/track` | Get library track ID for playback |
| GET | `/api/podcasts/status` | Check poddl availability |

### Spotify
| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/spotify/status` | Connection status |
| GET | `/api/spotify/login` | PKCE OAuth login URL |
| GET | `/api/spotify/callback` | OAuth callback handler |
| GET | `/api/spotify/devices` | List available Spotify devices |
| POST | `/api/spotify/transfer` | Transfer playback to device |

### System
| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/health` | Liveness check |
| POST | `/api/scan` | Trigger library rescan |
| GET | `/api/qr` | QR code PNG for LAN connection |
| GET | `/api/network` | Network info for debugging |
| GET | `/api/ws/player` | WebSocket endpoint for multi-device playback control |

## How It Works

### Scanner
Walks `MEDIA_ROOTS` (and `PodcastDir` if separate) recursively, extracts ID3/FLAC/MP4 metadata via `dhowden/tag`, and upserts tracks into SQLite by path. Files with matching `mtime` are skipped, making re-scans cheap. Files under 10KB are skipped as suspicious. Re-trigger anytime via the UI or `POST /api/scan`.

### Streaming
Range-request audio streaming — the frontend requests byte ranges and the backend serves them from the file. Handles seeking, partial content (206), and full file requests. No transcoding — serves the original file bytes. 24h Cache-Control headers.

### Recommendations
`POST /api/recommendations/refresh` builds a compact natural-language profile (top artists/genres last 90 days, recently played, library counts) and enriches it with Spotify profile data (top artists/tracks, playlists, saved tracks, followed artists). Sends to DeepSeek with a curator prompt. Returns JSON with 8–12 recommendations split between **library** (already in your collection, re-listen) and **discover** (new suggestions). Results are cached in SQLite with 1-hour TTL.

### AI Playlist Generation
`POST /api/recommendations/playlist` sends a curator prompt to DeepSeek that generates a themed playlist with name, description, and 5–50 tracks (each with reasoning). Tracks that exist in the library get instant `track_id` resolution. The frontend then downloads any missing tracks before adding them to the playlist.

### Download Pipeline
Each download job runs through three tiers sequentially:
1. **SpotiFLAC** runs first — if it reports "0 Success, N Failed" the pipeline advances
2. **yt-dlp** searches YouTube with `ytsearch1:<parsed query>` — if it fails, advance
3. **spotDL** searches YouTube Music as the final fallback

Free-text searches skip SpotiFLAC and optionally use DeepSeek to parse the query into structured metadata for better yt-dlp results. All subprocess output is streamed into the job log (capped at 500 lines). On success, a library rescan is automatically triggered. Post-download ffprobe validation ensures file integrity.

### Podcast Pipeline
- **Direct audio URLs** → downloaded via Go's `net/http` client with 30-min timeout
- **RSS-only feeds** → downloaded via poddl.exe subprocess with 5-min timeout
- **Bulk feed downloads** → poddl with `-r -h` flags (reverse order, stop on first existing file), timestamp snapshot matching for episode→file mapping
- All downloads register unified jobs visible on the Downloads page

### Spotify Sync
PKCE OAuth flow (no client secret required). After auth, Lexicon periodically syncs your Spotify listening history and merges it into the local play tracking table. Token refresh is automatic. Background syncer runs on startup.

### WebSocket Player Control
Gorilla/websocket hub enables multi-device playback control. One device acts as the player; others can send play/pause/next/prev/seek/transfer commands. State broadcasts keep all controllers in sync.

## Project Structure

```
lexicon/
├── backend/
│   ├── cmd/server/main.go              # Entry point, dependency injection, route mounting
│   └── internal/
│       ├── config/                     # Environment variable loading
│       ├── db/                         # SQLite schema, migrations, FTS5 triggers
│       ├── models/                     # Canonical Track struct (shared across packages)
│       ├── auth/                       # API key auth middleware
│       ├── scanner/                    # Filesystem walk + metadata extraction
│       ├── library/                    # Track/album/artist CRUD + search + cover art
│       ├── streamer/                   # Range-request audio streaming
│       ├── history/                    # Play recording + recent plays
│       ├── analytics/                  # SQL aggregations for dashboard
│       ├── recommender/                # DeepSeek client: recommendations, playlists, chat
│       ├── spotify/                    # PKCE OAuth, token management, history sync, device control
│       ├── playlists/                  # Playlist CRUD + track management
│       ├── downloader/                 # 3-tier download pipeline + job management
│       ├── podcaster/                  # Podcast RSS + poddl subprocess + episode management
│       ├── playerws/                   # WebSocket hub for multi-device playback control
│       └── websearch/                  # DuckDuckGo + SearXNG search + page extraction
├── frontend/
│   └── src/
│       ├── App.tsx                     # Shell, routing, provider hierarchy
│       ├── lib/api.ts                  # Typed API client
│       ├── lib/spotify.ts              # Spotify Web SDK wrapper
│       ├── lib/playerws.ts             # WebSocket player client
│       ├── player/PlayerContext.tsx     # Global audio player + history hooks + auto-skip
│       ├── contexts/ToastContext.tsx    # Toast notification system
│       ├── contexts/DownloadContext.tsx # Cross-route download state
│       ├── components/                 # PlayerBar, TrackList, MobilePlayerBar, MobileNavBar,
│       │                               #   DevicePicker, ErrorBoundary
│       └── pages/                      # Home, Music, Podcasts, Search, Analytics,
│                                       #   Discover/Recs, Downloads, Playlists, Playlist, Settings
├── release/
│   ├── build.ps1                       # PowerShell build script (Windows)
│   ├── lexicon.iss                     # InnoSetup 6+ installer script
│   ├── gen_icon.py                     # Icon generator (SVG → ICO/PNG via cairosvg)
│   ├── lexicon.ico                     # Windows icon (multi-resolution, 16-256px)
│   └── tools/                          # Bundled external .exe files for installer
└── tools/
    ├── spotiflac.exe                   # Prebuilt FLAC downloader (bundled in installer)
    ├── poddl.exe                       # Podcast episode downloader
    ├── yt-dlp.exe                      # YouTube audio downloader
    ├── spotdl.exe                      # Spotify/YouTube Music downloader
    ├── ffmpeg.exe / ffprobe.exe         # Audio processing + validation
    └── development_context.md          # Tool documentation
```

Each package also contains a `development_context.md` file providing complete self-contained documentation for AI agents resuming work on the project with zero prior context.

## Distribution

Lexicon is distributed as a **Windows-native single-exe installer** built with InnoSetup. The installer bundles:

- `lexicon.exe` — Go backend with embedded frontend (via `//go:embed`)
- All download tools (`spotiflac.exe`, `yt-dlp.exe`, `spotdl.exe`, `ffmpeg.exe`, `ffprobe.exe`, `poddl.exe`)
- Optional: `ngrok.exe` for remote access

Build with:
```powershell
cd release
.\build.ps1
```
Output: `LexiconSetup.exe`

## Known Limitations

- **Windows-only distribution** — backend compiles cross-platform but installer + bundled tools are Windows .exe files
- **No multi-user support** — single SQLite database, single API key
- **Podcast downloads require poddl.exe** — only feeds with direct audio URLs work without it
- **Large library performance** — no server-side pagination on track list (client-side filtering mitigates)

## License

MIT
