# Lexicon

A Plex-like media center for **podcasts and music**, with an LLM (DeepSeek) that watches your listening habits, finds trends, and recommends what to listen to next.

- **Backend:** Go + chi + SQLite (FTS5) — single binary, zero-config
- **Frontend:** React + Vite + TypeScript + TailwindCSS + Recharts + lucide-react
- **LLM:** DeepSeek (model `deepseek-v4-flash`, thinking effort `medium`)

## Features

| Phase | Feature | Status |
|-------|---------|:-:|
| 1 | Local file scanner (MP3/FLAC/M4A/OGG/Opus/WAV/AAC) | ✅ |
| 1 | Metadata extraction (ID3, FLAC tags, MP4 atoms) | ✅ |
| 1 | Range-request audio streaming | ✅ |
| 1 | Library API (tracks/albums/artists/podcasts) | ✅ |
| 1 | FTS5 full-text search | ✅ |
| 1 | Embedded cover-art serving | ✅ |
| 1 | Plex-like web UI with persistent player bar | ✅ |
| 2 | Listening history tracking | ✅ |
| 2 | Analytics dashboard (top artists/tracks/genres, time-of-day heatmap) | ✅ |
| 2 | Spotify / Apple Podcasts connectors | 🔜 |
| 3 | DeepSeek-powered recommendation engine | ✅ |
| 3 | "Discover" page with library + new-artist suggestions | ✅ |
| 3 | Conversational chat about your taste | ✅ |
| 4 | Playlists, smart playlists, PWA, mood/energy auto-tagging | 🔜 |

## Quick start

### Prerequisites
- **Go 1.22+** — install from <https://go.dev/dl/>. Verify with `go version`.
- **Node 20+** — install from <https://nodejs.org>. Verify with `node -v`.

### 1. Backend

```powershell
cd C:\Users\kevin\CascadeProjects\lexicon\backend
# Edit .env — set MEDIA_ROOTS to your music/podcast folders (semicolon-separated on Windows)
# DEEPSEEK_API_KEY is already filled in for you.
go mod tidy
go run ./cmd/server
```

Backend listens on `http://localhost:8787`. Initial scan runs in the background; watch the log for `[scanner] initial scan complete`.

### 2. Frontend

```powershell
cd C:\Users\kevin\CascadeProjects\lexicon\frontend
npm install
npm run dev
```

Open <http://localhost:5173>. The Vite dev server proxies `/api` to the Go backend.

## Configuration (`backend/.env`)

```env
PORT=8787
DB_PATH=./data/lexicon.db
# Multiple roots separated by ';' on Windows, ':' or ';' elsewhere
MEDIA_ROOTS=C:/Users/kevin/Music;D:/Podcasts

DEEPSEEK_API_KEY=sk-...
DEEPSEEK_MODEL=deepseek-v4-flash
DEEPSEEK_THINKING=medium
DEEPSEEK_BASE_URL=https://api.deepseek.com
```

## How it works

### Scanner
Walks `MEDIA_ROOTS` recursively, extracts ID3/FLAC/MP4 metadata via `dhowden/tag`, and upserts tracks into SQLite. Files whose `mtime` matches the DB row are skipped, so re-scans are cheap. Re-trigger anytime with the **Rescan library** button (or `POST /api/scan`).

### Music vs podcast detection
A track is classified as `podcast` if its genre or path contains "podcast"; otherwise `music`. You can override by tagging your podcast files with the genre `Podcast` before scanning.

### Listening history
Every time you play something, the React `PlayerContext` fires `POST /api/history/play` with the track id, the actual seconds-listened (computed from `<audio>.currentTime` deltas), and a `completed` flag (true on `ended`). Plays under 5 seconds are ignored.

### Analytics
Pure SQL aggregations over the `plays` table — no extra services needed. Time-of-day heatmap uses SQLite's `strftime` with `localtime`.

### LLM recommendations
`POST /api/recommendations/refresh` builds a compact natural-language profile (top artists/genres last 90 days, recently played, library snapshot) and asks DeepSeek for a JSON list of 8–12 recommendations split between **library** ("you should re-listen to X") and **discover** ("you'd love Y"). Library hits are auto-resolved to `track_id` so you can play them in one click.

`POST /api/recommendations/chat` lets you ask follow-up questions ("what should I put on for focused work?") with the same profile as system context.

## API summary

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/health` | Liveness |
| POST | `/api/scan` | Trigger rescan |
| GET | `/api/library/stats` | Counts |
| GET | `/api/library/tracks?kind=music&limit=200` | List tracks |
| GET | `/api/library/albums` `/artists` `/podcasts` | Group views |
| GET | `/api/library/search?q=...` | FTS5 search |
| GET | `/api/library/track/:id` | Single track |
| GET | `/api/library/cover/:id` | Embedded cover art |
| GET | `/api/stream/:id` | Audio stream (range supported) |
| POST | `/api/history/play` | Record a play |
| GET | `/api/history/recent` | Last 50 plays |
| GET | `/api/analytics/{overview,top-artists,top-tracks,top-genres,heatmap}` | Analytics |
| GET | `/api/recommendations` | Latest cached recs |
| POST | `/api/recommendations/refresh` | Generate new recs (DeepSeek) |
| POST | `/api/recommendations/chat` | Chat with the recommender |

## Project structure

```
lexicon/
├── backend/
│   ├── cmd/server/main.go           # entry point + router
│   ├── internal/
│   │   ├── config/                  # env loader
│   │   ├── db/                      # sqlite + migrations + FTS5
│   │   ├── scanner/                 # filesystem walk + tag extraction
│   │   ├── library/                 # CRUD + search + cover art
│   │   ├── streamer/                # range-request audio
│   │   ├── history/                 # play recording
│   │   ├── analytics/               # SQL aggregations
│   │   └── recommender/             # DeepSeek client + profile builder
│   ├── go.mod
│   └── .env
└── frontend/
    ├── src/
    │   ├── App.tsx                  # layout + routing
    │   ├── lib/api.ts               # typed API client
    │   ├── player/PlayerContext.tsx # global audio player + history hooks
    │   ├── components/{PlayerBar,TrackList}.tsx
    │   └── pages/                   # Home, Music, Podcasts, Search, Analytics, Recs
    ├── tailwind.config.js
    └── package.json
```

## Roadmap

- [ ] Spotify OAuth connector → merge external history
- [ ] Apple Podcasts / Podcast Index API connector
- [ ] Playlists + smart playlists
- [ ] PWA manifest + service worker
- [ ] Natural-language search ("upbeat 70s funk")
- [ ] Auto-tagging tracks with mood/energy via DeepSeek
- [ ] Multi-user profiles
