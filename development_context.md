# Lexicon — Project Root Context

> **Purpose:** Zero-context onboarding for any AI agent resuming work on Lexicon.
> **Path:** `C:\Users\kevin\CascadeProjects\lexicon`
> **Version:** 3.2.0 (May 2026)
> **Last updated:** 2026-05-18

## What Is Lexicon?

A Plex-like media center for **podcasts and music**, with an LLM (DeepSeek) that watches listening habits, finds trends, and recommends what to listen to next. Windows-native desktop app distributed as a single InnoSetup installer.

## Tech Stack

| Layer | Technology |
|-------|------------|
| Backend | Go 1.22+ + chi router + SQLite (WAL, FTS5) |
| Frontend | React 18 + TypeScript + Vite 5 + TailwindCSS 3 + Recharts + Lucide |
| AI | DeepSeek API (model: `deepseek-v4-flash`) |
| External APIs | Spotify Web API (PKCE OAuth) |
| External tools | SpotiFLAC, yt-dlp, spotDL, ffmpeg, poddl, ngrok (all bundled as .exe) |
| Web Search | SearXNG (local) + Trafilatura (local extraction) |

## Folder Layout

```
lexicon/
├── backend/               # Go server (← start here for backend work)
│   ├── cmd/server/        # main.go — entry point, wiring, router
│   ├── internal/          # Business logic packages
│   │   ├── config/        # Env var loader (.env)
│   │   ├── db/            # SQLite schema + migrations
│   │   ├── models/        # Canonical Track struct (shared)
│   │   ├── auth/          # API key auth middleware
│   │   ├── scanner/       # Filesystem walk + ID3/FLAC metadata extraction
│   │   ├── library/       # Track/album/artist/podcast CRUD + FTS5 search
│   │   ├── streamer/      # Range-request audio streaming
│   │   ├── history/       # Play recording (POST /api/history/play)
│   │   ├── analytics/     # SQL aggregations (top artists, heatmap, etc.)
│   │   ├── recommender/   # DeepSeek client — recommendations, chat, playlists
│   │   ├── spotify/       # Spotify PKCE OAuth, token mgmt, history sync
│   │   ├── playlists/     # Playlist CRUD
│   │   ├── downloader/    # 3-tier download pipeline
│   │   ├── podcaster/     # Podcast RSS + poddl subprocess
│   │   ├── playerws/      # WebSocket hub for device control
│   │   └── websearch/     # DuckDuckGo + SearXNG search + page extraction
│   ├── data/              # SQLite DB file (lexicon.db)
│   ├── go.mod / go.sum
│   └── .env               # Secrets (gitignored)
│   └── .env.example       # Template showing all available vars
├── frontend/
│   ├── src/
│   │   ├── App.tsx         # Shell + routing + providers
│   │   ├── lib/api.ts      # Typed API client (all endpoints)
│   │   ├── lib/spotify.ts  # Spotify Web Playback SDK wrapper
│   │   ├── lib/playerws.ts # WebSocket player client
│   │   ├── player/PlayerContext.tsx  # Global audio player + history hooks
│   │   ├── contexts/ToastContext.tsx # Toast notification system
│   │   ├── contexts/DownloadContext.tsx # Cross-route download state
│   │   ├── components/     # PlayerBar, TrackList, MobilePlayerBar, MobileNavBar, DevicePicker, ErrorBoundary
│   │   └── pages/          # Home, Music, Podcasts, Search, Analytics, Recs,
│   │                       #   Downloads, Playlists, Playlist, Settings
│   ├── vite.config.ts
│   ├── tailwind.config.js
│   └── package.json
├── release/
│   ├── build.ps1           # PowerShell build script
│   ├── lexicon.iss         # InnoSetup installer script
│   ├── lexicon.ico         # Windows icon (multi-resolution)
│   ├── gen_icon.py         # Icon generator script
│   ├── lexicon.exe         # Compiled binary
│   └── tools/              # Bundled .exe tools for installer
├── tools/
│   ├── spotiflac.exe       # Prebuilt SpotiFLAC binary (11.3MB)
│   ├── poddl.exe           # Prebuilt poddl binary (1.3MB)
│   └── spotiflac-src/      # Wails desktop app source for SpotiFLAC
└── README.md
```

> **Note:** Several `*_PLAN.md` files exist in the root — these are **planning artifacts**, not active code.

## CRITICAL ARCHITECTURAL CONSTRAINT

**Lexicon is Windows-only.** Single `lexicon.exe` Go binary with embedded frontend (`//go:embed`). All external tools must be Windows .exe files bundled in the installer. No Python, no WSL, no Docker dependencies.

## Context Storage System

Each folder contains a `development_context.md` file that provides complete context for that part of the application. **These files must be updated whenever code changes are made.** They are the primary onboarding mechanism for any AI agent.

### Context Update Protocol (MANDATORY)
1. **After ANY code change:** Update the affected `development_context.md` files
2. **Update at minimum:** "Last updated" date, LOC counts, new/removed files, bug status
3. **After adding a feature:** Create `development_context.md` for new folders, update parent references
4. **After fixing a bug:** Mark it as fixed or remove from "Known Issues"
5. **Before declaring work complete:** Verify context files match actual code

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

1. **Unchecked Scan errors in read-only endpoints** — `library.go` (albums/artists/podcasts) and `playlists.go` (list) don't check `rows.Scan()` errors. Low impact (read-only), but inconsistent with fixes elsewhere.
2. **WebSearchEnabled defaults to true** — If user has no internet, chat/playlist endpoints could timeout waiting for search providers.
3. **`math/rand` in searx.go** — Uses `math/rand` instead of `crypto/rand` for shuffling search instances. Not security-critical but not idiomatic for Go 1.22+.

## Previously-Fixed Issues (for reference)

All of these were fixed as of 2026-05-16 (v2.4.0) and should NOT be listed as active:
- ✅ No authentication → Fixed with `RequireAPIKey` middleware + `LEXICON_API_KEY` env var
- ✅ Download jobs lost on restart → Fixed with `download_jobs` SQLite table + startup recovery
- ✅ Track type duplication → Fixed with shared `internal/models` package
- ✅ No row error checking → Fixed across 6 files
- ✅ Heatmap timezone → Fixed with `TIMEZONE` env var
- ✅ Recommender silent fallback → Fixed with retry wrapper + proper error logging
- ✅ Ngrok free tier blocks playback → Fixed with `ngrok-skip-browser-warning: 1` header
- ✅ Listen time always 0 → Fixed with audio error toast in PlayerContext
- ✅ Album truncation → Fixed with CSS `max-w-48` + `title` tooltip
- ✅ Spotify genres empty → Fixed with batched artist genre fetches in sync
- ✅ PKCE race condition → Fixed with in-memory `sync.Map` for verifiers
- ✅ No download rate limiting → Fixed with `DOWNLOAD_CONCURRENCY` semaphore
- ✅ Playlist generation not cached → Fixed with profile hash + 1h TTL cache
- ✅ RecsPage state lost on navigation → Fixed with DownloadContext provider
- ✅ No MIME validation → Fixed with `isValidAudioFile()` in downloader
- ✅ yt-dlp download corruption (~12% failure rate) → Fixed with ffprobe validation + auto-retry (v2.7.1)
- ✅ yt-dlp postprocessor-args bug → Removed invalid `--postprocessor-args` (v2.7.2)
- ✅ PlayerContext auto-skip race condition → Fixed with `scheduleSkip()`/`clearSkipTimeout()` (v2.8.2)
- ✅ gofeed.Person SQL fix → Extract `.Name` string before passing to SQL (v2.8.3)
- ✅ Poddl not bundled → Downloaded poddl.exe, added to tools/ and installer (v2.8.4)
- ✅ Poddl download pipeline broken → Fixed: use audioURL instead of feedURL, set PODDL_BIN in .env, fix frontend polling (v2.9.0)

## Working on This Project

- Load the `lexicon-codebase-review` skill before making any changes.
- Read the specific folder's `development_context.md` for details on that module.
- Backend changes: update config.go then main.go, then the specific internal package.
- Frontend changes: update api.ts types, then the relevant page/component.
- Build: run `release/build.ps1` on Windows dev machine.
- **Update `development_context.md` files after every change.**
