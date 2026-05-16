# Lexicon Frontend — Development Context

> **Zero-context onboarding for the React frontend.**
> **Parent:** [Lexicon root](../development_context.md)
> **Stack:** React 18, TypeScript, Vite 5, TailwindCSS 3, React Router, Recharts, Lucide Icons

## Purpose

Single-page React application that provides the UI for Lexicon. Communicates with the Go backend exclusively through the typed API client in `src/lib/api.ts`.

## Project Structure

```
frontend/
├── package.json          # Dependencies + scripts
├── vite.config.ts        # Vite config (port 5173, proxy /api → :8787)
├── tailwind.config.js    # Tailwind CSS config
├── tsconfig.json         # TypeScript config
├── postcss.config.js     # PostCSS (Tailwind)
├── index.html            # HTML entry point
├── node_modules/         # Dependencies
├── dist/                 # Production build (output)
├── public/               # Static assets
└── src/
    ├── main.tsx           # React entry point
    ├── App.tsx            # Shell: providers, routing, nav, player bar
    ├── index.css          # Tailwind + custom styles
    ├── lib/
    │   ├── api.ts         # Typed API client (all backend endpoints)
    │   └── spotify.ts     # Spotify Web Playback SDK wrapper
    ├── player/
    │   └── PlayerContext.tsx  # Global audio player state + history recording
    ├── contexts/
    │   └── ToastContext.tsx   # Toast notification system (NEW v2)
    ├── components/
    │   ├── PlayerBar.tsx      # Persistent playback bar (desktop)
    │   ├── TrackList.tsx      # Track table with add-to-playlist (NEW v2)
    │   ├── MobilePlayerBar.tsx # Compact player bar (mobile)
    │   └── MobileNavBar.tsx    # Bottom nav bar (mobile)
    ├── hooks/
    │   └── useIsMobile.ts     # Detect mobile viewport
    └── pages/
        ├── HomePage.tsx       # Dashboard
        ├── MusicPage.tsx      # Music library + download integration (NEW v2)
        ├── PodcastsPage.tsx   # Podcast library
        ├── AnalyticsPage.tsx  # Charts (Recharts)
        ├── RecsPage.tsx       # Discover + AI playlist + download-on-demand (NEW v2)
        ├── SearchPage.tsx     # Search + download integration (NEW v2)
        ├── SettingsPage.tsx   # Spotify connection settings
        ├── DownloadsPage.tsx  # Download job manager (NEW v2)
        ├── PlaylistsPage.tsx  # Playlist grid (NEW v2)
        └── PlaylistPage.tsx   # Playlist detail with tracks (NEW v2)
```

## Dev Setup

```powershell
cd C:\Users\kevin\CascadeProjects\lexicon\frontend
npm install
npm run dev
```

Vite dev server runs on `http://localhost:5173`. All `/api/*` requests are proxied to `http://localhost:8787` (Go backend).

## Vite Config (`vite.config.ts`)

```typescript
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    host: true,
    allowedHosts: true,        // Allow ngrok tunnels
    proxy: { "/api": "http://localhost:8787" },
    hmr: { clientPort: 443 },  // HMR over ngrok HTTPS
  },
});
```

## Provider Hierarchy

```
App
  └── PlayerProvider (PlayerContext — audio player + history)
      └── ToastProvider (ToastContext — notifications)
          └── DesktopLayout | MobileLayout
              ├── Sidebar/Nav
              ├── <Routes> (page content)
              └── PlayerBar / MobilePlayerBar
```

## Key Routes

| Path | Page | v2 Change |
|------|------|:---:|
| `/` | HomePage | No |
| `/music` | MusicPage | ✅ filters + download |
| `/podcasts` | PodcastsPage | No |
| `/playlists` | PlaylistsPage | 🆕 |
| `/playlists/:id` | PlaylistPage | 🆕 |
| `/downloads` | DownloadsPage | 🆕 |
| `/analytics` | AnalyticsPage | No |
| `/discover` | RecsPage | ✅ expanded |
| `/search` | SearchPage | ✅ download integration |
| `/settings` | SettingsPage | No |

## Known Frontend Issues

1. **Listen time always 0** — `flushLocalPlay()` calls `api.recordPlay()` but swallows errors with `.catch(() => {})`. If the API call fails, no retry or user feedback.
2. **Silent error swallowing** — Multiple `.catch(() => {})` in PlayerContext (audio.play(), recordPlay, Spotify calls)
3. **Album name truncation** — TrackList uses CSS `truncate` without max-width on album column
4. **RecsPage state bleed** — `completedIds`/`downloadingIds`/`playlistTrackStatus` state lost on navigation
5. **No pagination on Music page** — hardcoded 500 limit (client-side filtering helps)
6. **Toast notifications** — only used in RecsPage, MusicPage, SearchPage (not universal)

## Working on the Frontend

- Reading the src/development_context.md for component-level details
- API changes: update types in `src/lib/api.ts` first, then pages
- New page: add route in `App.tsx`, create component, add nav item
- Build: `npm run build` outputs to `dist/`
