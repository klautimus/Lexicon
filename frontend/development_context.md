# Lexicon Frontend — Development Context

> **Zero-context onboarding for the React frontend.**
> **Parent:** [Lexicon root](../development_context.md)
> **Stack:** React 18, TypeScript, Vite 5, TailwindCSS 3, React Router, Recharts, Lucide Icons
> **Last updated:** 2026-05-17

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
├── public/               # Static assets (icon.svg, manifest.json)
└── src/
    ├── main.tsx           # React entry point
    ├── App.tsx            # Shell: providers, routing, nav, player bar
    ├── index.css          # Tailwind + custom styles
    ├── lib/
    │   ├── api.ts         # Typed API client (all backend endpoints)
    │   ├── spotify.ts     # Spotify Web Playback SDK wrapper
    │   └── playerws.ts    # WebSocket player client
    ├── player/
    │   └── PlayerContext.tsx  # Global audio player state + history recording
    ├── contexts/
    │   ├── ToastContext.tsx   # Toast notification system
    │   └── DownloadContext.tsx # Cross-route download state
    ├── hooks/
    │   └── useIsMobile.ts     # Detect mobile viewport
    ├── components/
    │   ├── PlayerBar.tsx      # Persistent playback bar (desktop)
    │   ├── TrackList.tsx      # Track table with add-to-playlist
    │   ├── MobilePlayerBar.tsx # Compact player bar (mobile)
    │   ├── MobileNavBar.tsx    # Bottom nav bar (mobile)
    │   ├── DevicePicker.tsx    # Spotify Connect + WebSocket device selector
    │   └── ErrorBoundary.tsx   # Error boundary wrapper
    └── pages/
        ├── HomePage.tsx       # Dashboard + QR code for LAN
        ├── MusicPage.tsx      # Music library + download integration
        ├── PodcastsPage.tsx   # Podcast library (two-panel layout)
        ├── AnalyticsPage.tsx  # Charts (Recharts)
        ├── RecsPage.tsx       # Discover + AI playlist + download-on-demand
        ├── SearchPage.tsx     # Search + download integration
        ├── SettingsPage.tsx   # Spotify connection settings
        ├── DownloadsPage.tsx  # Download job manager
        ├── PlaylistsPage.tsx  # Playlist grid
        └── PlaylistPage.tsx   # Playlist detail with tracks
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
ErrorBoundary
  └── ToastProvider (ToastContext — notifications)
      └── PlayerProvider (PlayerContext — audio player + history)
          └── DownloadProvider (DownloadContext — cross-route download state)
              └── DesktopLayout | MobileLayout
                  ├── Sidebar/Nav
                  ├── <Routes> (page content)
                  └── PlayerBar / MobilePlayerBar
```

## Key Routes

| Path | Page | v2 Change |
|------|------|:---:|
| `/` | HomePage | ✅ QR code for LAN |
| `/music` | MusicPage | ✅ filters + download |
| `/podcasts` | PodcastsPage | ✅✅ Two-panel layout with feed sidebar + episode list |
| `/playlists` | PlaylistsPage | 🆕 |
| `/playlists/:id` | PlaylistPage | 🆕 |
| `/downloads` | DownloadsPage | 🆕 |
| `/analytics` | AnalyticsPage | No |
| `/discover` | RecsPage | ✅ expanded |
| `/search` | SearchPage | ✅ download integration |
| `/settings` | SettingsPage | No |

## Working on the Frontend

- Reading the src/development_context.md for component-level details
- API changes: update types in `src/lib/api.ts` first, then pages
- New page: add route in `App.tsx`, create component, add nav item
- Build: `npm run build` outputs to `dist/`
