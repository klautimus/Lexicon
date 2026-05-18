# frontend/src — Component-Level Context

> **Parent:** [frontend development context](../development_context.md)
> **Last updated:** 2026-05-17

## Entry Points

### `main.tsx` (~5 LOC)
Standard React 18 entry point. Wraps `<App />` in `<React.StrictMode>` and `<BrowserRouter>`. Renders into `#root` element in `index.html`.

### `App.tsx` (140 LOC)
- **Provider hierarchy:** `ErrorBoundary > ToastProvider > PlayerProvider > DownloadProvider > Layout`
- **Desktop (≥768px):** Sidebar nav + main content area + persistent PlayerBar at bottom
- **Mobile (<768px):** Full-width content + MobilePlayerBar + MobileNavBar at bottom
- **Routing:** React Router v6 with 10 routes
- **Rescan button** in sidebar calls `api.scan()`

### `index.css`
Tailwind directives + custom scrollbar styling + dark theme variables.

## API Client (`lib/api.ts` — 328 LOC)

Typed wrapper around fetch. All methods return typed Promises. Includes `ngrok-skip-browser-warning: 1` header on all requests.

Key pattern:
```typescript
const API = "/api";
async function j<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(API + path, { headers: {...}, ...init });
  if (!r.ok) throw new Error(`${r.status} ${await r.text()}`);
  return r.json();
}
```

## Player Context (`player/PlayerContext.tsx` — 525 LOC)

**The most critical frontend component.** Provides global audio playback across the entire app.

### Dual-Source Playback
- **Local files** (HTMLAudioElement): `audio.src = api.streamUrl(track.id)`
- **Spotify** (Web Playback SDK): Detected by `track.spotify_id` presence

### History Recording
`flushLocalPlay(completed: boolean)` posts to `POST /api/history/play`. Called on track end, track change, and unmount.

### Shuffle & Repeat
- Fisher-Yates shuffle. Preserves `originalQueueRef` for unshuffling.
- Repeat modes: `off` (stop at end), `all` (wrap to start), `one` (replay current)

### Error Handling & Auto-Skip (v2.7)
- `onError` handler auto-skips to next track after 1.5s delay
- `loadAndPlay()` catch handler also auto-skips
- `consecutiveErrorsRef` tracks consecutive failures; stops playback after 5
- Counter resets on successful playback
- **Skip timeout lifecycle (v2.8.2):** Always use `scheduleSkip()`, always call `clearSkipTimeout()` before new playback

## Toast Context (`contexts/ToastContext.tsx` — 93 LOC)

- `toast.success(msg)`, `toast.error(msg)`, `toast.info(msg)`
- Auto-dismiss after 4,500ms
- Fixed position: top-right, z-50
- Colored left border + icon + close button

## Download Context (`contexts/DownloadContext.tsx` — 350 LOC)

Lifts download/playlist state from RecsPage for cross-route persistence:
- Download tracking (`downloadingIds`, `completedIds`, `completedTrackIds`)
- AI playlist state (`playlistPreview`, `playlistTrackStatus`, `createdPlaylistId`)
- Actions: `downloadItem()`, `trackDownload()`, `generateAiPlaylist()`, `createAiPlaylist()`

## Components

### ErrorBoundary (45 LOC)
Wraps the entire app. Catches React render errors and displays a fallback UI.

### PlayerBar.tsx (152 LOC)
Desktop persistent playback bar. Shows: track info, progress bar (seekable), play/pause/next/prev, volume slider, shuffle/repeat toggles, DevicePicker.

### TrackList.tsx (534 LOC)
3-component file: `TrackList` (wrapper) + `DesktopTable` (desktop table) + `MobileCardList` (mobile cards).
- "Add to Playlist" dropdown per row
- Album name truncated via CSS `max-w-48` + `title` tooltip

### MobilePlayerBar.tsx (203 LOC)
Compact, expandable player for mobile viewports. Includes DevicePicker in expanded view.

### MobileNavBar.tsx (138 LOC)
Bottom navigation bar with icons matching the desktop sidebar.

### DevicePicker.tsx (187 LOC)
Device selector dropdown — Spotify Connect + WebSocket devices. Mobile-hardened.

## Pages

| Page | LOC | v2 Change | Key Features |
|------|-----|:---:|------|
| **HomePage** | 174 | ✅ | Stats, recent plays, QR code for LAN connection, network debug info |
| **MusicPage** | 221 | ✅ | Client-side filter, "Search & Download from Web" on no results, pagination (Load More) |
| **PodcastsPage** | ~464 | ✅✅ | Two-panel layout: feed sidebar + episode list. Per-episode download, download-all button, download error display, polling for completion |
| **PlaylistsPage** | 139 | 🆕 | Grid of playlist cards, create new form |
| **PlaylistPage** | 335 | 🆕 | Track table, inline rename, Play All, track removal |
| **DownloadsPage** | 319 | 🆕 | Dual-mode (URL/search), 1.5s polling, expandable logs, cancel |
| **AnalyticsPage** | 160 | No | Recharts: bar chart, pie chart, heatmap |
| **RecsPage** | 321 | ✅ | AI playlist generation, download-on-demand, playlist preview with per-track status, chat |
| **SearchPage** | 128 | ✅ | FTS5 search, "Search & Download from Web" on no results |
| **SettingsPage** | 176 | No | Spotify connect/disconnect |

## Cross-Patterns

**Download integration** (MusicPage, SearchPage, RecsPage):
1. Call `api.downloadSearch(query)` or `api.download(url)`
2. Poll `api.downloadJob(id)` every 1.5-2s via `setInterval`
3. On success: refresh data + show toast
4. Clean up interval in `useEffect` return

**Playlist creation** (TrackList, RecsPage):
1. `api.createPlaylist(name)` → get playlist ID
2. `api.addToPlaylist(playlistId, trackId)` for each track

## Working Here

- API changes: update types in `lib/api.ts` first, then pages
- New page: add to `App.tsx` routes + nav items, create component
- New feature across pages: pattern-match existing implementations
