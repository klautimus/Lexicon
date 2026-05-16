# Lexicon — Project Root Context

> **Purpose:** Zero-context onboarding for any AI agent resuming work on Lexicon.
> **Path:** `C:\Users\kevin\CascadeProjects\lexicon`
> **Version:** 2.1.0 (May 2026)
> **Last updated:** 2026-05-16

## What Is Lexicon?

A Plex-like media center for **podcasts and music**, with an LLM (DeepSeek) that watches listening habits, finds trends, and recommends what to listen to next. Windows-native desktop app distributed as a single InnoSetup installer.

## Tech Stack

| Layer | Technology |
|-------|------------|
| Backend | Go 1.22+ + chi router + SQLite (WAL, FTS5) |
| Frontend | React 18 + TypeScript + Vite 5 + TailwindCSS 3 + Recharts + Lucide |
| AI | DeepSeek API (model: `deepseek-v4-flash`) |
| External APIs | Spotify Web API (PKCE OAuth) |
| External tools | SpotiFLAC, yt-dlp, spotDL, ffmpeg, ngrok (all bundled as .exe) |

## Folder Layout

```
lexicon/
├── backend/               # Go server (← start here for backend work)
│   ├── cmd/server/        # main.go — entry point, wiring, router
│   ├── internal/          # Business logic packages
│   │   ├── config/        # Env var loader (.env)
│   │   ├── db/            # SQLite schema + migrations
│   │   ├── scanner/       # Filesystem walk + ID3/FLAC metadata extraction
│   │   ├── library/       # Track/album/artist/podcast CRUD + FTS5 search
│   │   ├── streamer/      # Range-request audio streaming
│   │   ├── history/       # Play recording (POST /api/history/play)
│   │   ├── analytics/     # SQL aggregations (top artists, heatmap, etc.)
│   │   ├── recommender/   # DeepSeek client — recommendations, chat, playlists
│   │   ├── spotify/       # Spotify PKCE OAuth, token mgmt, history sync
│   │   ├── playlists/     # Playlist CRUD (NEW v2)
│   │   └── downloader/    # 3-tier download pipeline (NEW v2)
│   ├── data/              # SQLite DB file (lexicon.db)
│   ├── go.mod / go.sum
│   └── .env               # Secrets (gitignored)
│   └── .env.example       # Template showing all available vars 🆕 v2
├── frontend/
│   ├── src/
│   │   ├── App.tsx         # Shell + routing + providers
│   │   ├── lib/api.ts      # Typed API client (all endpoints)
│   │   ├── lib/spotify.ts  # Spotify Web Playback SDK wrapper
│   │   ├── player/PlayerContext.tsx  # Global audio player + history hooks
│   │   ├── contexts/ToastContext.tsx # Toast notification system (NEW v2)
│   │   ├── components/     # PlayerBar, TrackList, MobilePlayerBar, MobileNavBar
│   │   └── pages/          # Home, Music, Podcasts, Search, Analytics, Recs, 
│   │                       #   Downloads (NEW), Playlists (NEW), Playlist (NEW), Settings
│   ├── vite.config.ts
│   ├── tailwind.config.js
│   └── package.json
├── release/
│   ├── build.ps1           # PowerShell build script
│   ├── lexicon.iss         # InnoSetup installer script
│   ├── lexicon.exe         # Compiled binary
│   └── tools/              # Bundled .exe tools for installer
├── tools/
│   ├── spotiflac.exe       # Prebuilt SpotiFLAC binary (11.8MB)
│   └── spotiflac-src/      # Wails desktop app source for SpotiFLAC
├── .skills/                # Agent context files (← development_context.md files live here)
└── README.md
```

> **Note:** Several `*_PLAN.md` files exist in the root (ANDROID_DOWNLOAD_PLAN.md, CHAT_PLAYLIST_FIX_PLAN.md, etc.) — these are **planning artifacts**, not active code. They document feature ideas and implementation plans but are not part of the running application.

## CRITICAL ARCHITECTURAL CONSTRAINT

**Lexicon is Windows-only.** Single `lexicon.exe` Go binary with embedded frontend (`//go:embed`). All external tools must be Windows .exe files bundled in the installer. No Python, no WSL, no Docker dependencies. Python-based tools (SearXNG, Trafilatura) can only be added by bundling embedded Python.

## Context Storage System

Each folder contains a `development_context.md` file that provides complete context for that part of the application. These files are consumed by the `lexicon-codebase-review` skill. When an agent is dropped into a folder, it should read the local `development_context.md` first, then follow parent references as needed.

## Quick Start

```powershell
# Backend
cd C:\Users\kevin\CascadeProjects\lexicon\backend
go run ./cmd/server

# Frontend
cd C:\Users\kevin\CascadeProjects\lexicon\frontend
npm run dev
```

Backend listens on `http://localhost:8787`. Frontend on `http://localhost:5173` (proxies `/api` to backend).

## Known Cross-Cutting Issues

1. **No authentication** — CORS is wildcard (`*`), no auth middleware. Fine for local use, not for exposed deployments.
2. **Download jobs lost on restart** — Jobs are in-memory only (`map[string]*Job`). Server restart clears all job history.
3. **Track type duplication** — `Track` struct defined separately in `library/library.go` and `playlists/playlists.go`. Changes to one must be manually propagated.
4. **No error row checking** — Multiple locations in library.go and analytics.go skip `rows.Scan()` errors.
5. **Heatmap timezone-dependent** — Uses SQL `strftime` with server `TZ`, not configurable.
6. **Recommender fallback silent** — Falls back to `RecsPayload{Summary: reply}` on JSON parse error with no logging.
7. **Ngrok free tier** — Blocks ngrok tunnels; user must dismiss interstitial page.
8. **Listen time tracking** — Audio load failures silently swallowed (`.catch(() => {})` in PlayerContext).

## Working on This Project

- Load the `lexicon-codebase-review` skill before making any changes.
- Read the specific folder's `development_context.md` for details on that module.
- Backend changes: update config.go then main.go, then the specific internal package.
- Frontend changes: update api.ts types, then the relevant page/component.
- Build: run `release/build.ps1` on Windows dev machine.
