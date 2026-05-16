# frontend/src — Component-Level Context

> **Parent:** [frontend development context](../development_context.md)

## Entry Points

### `main.tsx` (13 LOC)
Standard React 18 entry point. Wraps `<App />` in `<React.StrictMode>` and `<BrowserRouter>` (React Router v6). Renders into `#root` element in `index.html`.

### `App.tsx` (136 LOC)
- **Provider hierarchy:** `PlayerProvider → ToastProvider → Layout`
- **Desktop (≥768px):** Sidebar nav (56px wide) + main content area + persistent PlayerBar at bottom
- **Mobile (<768px):** Full-width content + MobilePlayerBar + MobileNavBar at bottom
- **Routing:** React Router v6 with 10 routes (2 new in v2: Downloads, Playlists)
- **Rescan button** in sidebar calls `api.scan()`

### `index.css`
Tailwind directives + custom scrollbar styling + dark theme variables.

## API Client (`lib/api.ts` — 234 LOC)

Typed wrapper around fetch. All methods return typed Promises.

Key pattern:
```typescript
const API = "/api";
async function j<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(API + path, { headers: {...}, ...init });
  if (!r.ok) throw new Error(`${r.status} ${await r.text()}`);
  return r.json();
}
export const api = {
  tracks: (kind?, limit=200) => j<Track[]>(`/library/tracks?...`),
  streamUrl: (id: number) => `${API}/stream/${id}`,
  recordPlay: (data) => j(`/history/play`, { method: "POST", body: JSON.stringify(data) }),
  // ... 30+ typed endpoints
};
```

**v2 additions:** `generatePlaylist()`, `download*(...)`, `playlists*()`, `createPlaylist()`, `addToPlaylist()`, `deleteTrack()`

## Player Context (`player/PlayerContext.tsx` — 377 LOC)

**The most critical frontend component.** Provides global audio playback across the entire app.

### State
```typescript
interface PlayerState {
  current: Track | null;    // currently playing track
  queue: Track[];            // play queue
  index: number;             // position in queue
  playing: boolean;
  position: number;          // seconds
  duration: number;          // seconds
  volume: number;            // 0-1
  source: "local" | "spotify" | null;
  error: string | null;
  shuffled: boolean;
  repeatMode: "off" | "all" | "one";
}
```

### Dual-Source Playback

**Local files** (HTMLAudioElement):
- `audio.src = api.streamUrl(track.id)`
- Time tracking: `setInterval` every 1s, computes delta from `audio.currentTime`
- Only counts delta if `0 < delta < 2` (filters out seeks)
- Skips plays under 5 seconds (in `flushLocalPlay()`)

**Spotify** (Web Playback SDK):
- Detected by `track.spotify_id` presence
- Creates Spotify Player instance with Lexicon device name
- Polls player state every 1s for position/duration sync
- Play/pause/seek/volume delegated to Spotify SDK

### History Recording
`flushLocalPlay(completed: boolean)`:
- Posts to `POST /api/history/play` with `track_id, duration_played_sec, completed, started_at`
- Called on: track end (`ended` event), track change, unmount
- ⚠️ `.catch(() => {})` — silently swallows API errors!

### Shuffle
Fisher-Yates shuffle. Preserves `originalQueueRef` for unshuffling.

### Repeat Modes
- `off`: stops at queue end
- `all`: wraps to queue start
- `one`: replays current track (calls `loadAndPlay` again)

## Toast Context (`contexts/ToastContext.tsx` — 93 LOC) 🆕 v2

Fixes the "silent error swallowing" from v1. Provides:
- `toast.success(msg)`, `toast.error(msg)`, `toast.info(msg)`
- Auto-dismiss after **4,500ms** (4.5 seconds) via `setTimeout`
- IDs generated with `Math.random().toString(36).slice(2)` (not UUID — collision possible but unlikely)
- Fixed position: top-right, z-50
- Colored left border: green (success), red (error), accent (info)
- Icons: CheckCircle / AlertCircle / Info from lucide-react
- Close button (X) on each toast
- Used by: RecsPage, MusicPage, SearchPage (not yet universal)

## Mobile Detection (`hooks/useIsMobile.ts` — 18 LOC)

```typescript
const MOBILE_QUERY = "(max-width: 768px)";
```
Uses `window.matchMedia()` with a change listener. Returns boolean. Used by `App.tsx` to switch between DesktopLayout and MobileLayout. No SSR support (assumes browser).

## Spotify SDK (`lib/spotify.ts` — 118 LOC)

- Loads Spotify Web Playback SDK script dynamically
- Singleton player instance keyed on user's connected Spotify account
- Token management: caches access token for 50 minutes, auto-refreshes
- Exports: `getSpotifyPlayer()`, `spotifyPlayURI()`, `spotifyToggle()`, `spotifyPause()`, `spotifySeek()`, `spotifySetVolume()`
- ⚠️ **Premium only** — non-Premium accounts get 403 from play

## Components

### PlayerBar.tsx (94 LOC)
Desktop persistent playback bar at bottom of screen. Shows: album art, track info, progress bar (seekable), play/pause/next/prev, volume slider, shuffle/repeat toggles.

### TrackList.tsx (214 LOC, was 50) 🆕 v2
Now a 2-component file: `TrackList` (table wrapper) + `TrackRow` (per-row logic).
- **New "Add to Playlist" dropdown:** "..." button per row → dropdown with existing playlists + "Create new" option
- Outside-click closes dropdown
- 5 columns: Title, Artist, Album, Duration, Actions ("...")
- Album name truncated via CSS
- **Known issue:** No max-width on album column — long names may overflow

### MobilePlayerBar.tsx (197 LOC)
Compact, expandable player for viewports < 768px.
- **Collapsed state:** Shows cover art thumbnail + track title/artist + play/pause button. Fixed at bottom of screen above MobileNavBar.
- **Expanded state:** Tap collapsed bar to expand — shows full-size cover art, progress bar (seekable), previous/next buttons, shuffle/repeat toggles, volume slider.
- Cover art `onError` sets opacity to 0 instead of showing broken image.
- "Nothing playing" placeholder when no track loaded.

### MobileNavBar.tsx
Bottom navigation bar with icons matching the desktop sidebar. Home, Music, Podcasts, Playlists, Downloads, Analytics, Discover, Search, Settings.

## Pages

| Page | LOC | v2 Change | Key Features |
|------|-----|:---:|------|
| **HomePage** | 61 | No | Stats overview, recent plays |
| **MusicPage** | 152 | ✅ | Client-side filter (title/artist/album), "Search & Download from Web" button on no results, polls download status, auto-refreshes on completion |
| **PodcastsPage** | ~25 | No | Podcast show list |
| **PlaylistsPage** | 118 | 🆕 | Grid of playlist cards (name, track count, duration), create new form |
| **PlaylistPage** | 265 | 🆕 | Track table with duration, inline rename, Play All, track removal (X button) |
| **DownloadsPage** | 312 | 🆕 | Dual-mode: Spotify URL / free-text search, 1.5s polling for job status, expandable per-job log viewer, cancel button, config status display |
| **AnalyticsPage** | 160 | No | Recharts: bar chart (top artists), pie chart (genres), heatmap (day×hour grid) |
| **RecsPage** | 498 | ✅ | AI playlist generation ("Generate Playlist"), download-on-demand for discover items, playlist preview with per-track status (pending/present/downloading/completed/failed), "Create Playlist" auto-downloads missing tracks |
| **SearchPage** | 128 | ✅ | FTS5 search, "No results" state with "Search & Download from Web" button, download status polling |
| **SettingsPage** | 176 | No | Spotify connect/disconnect, connection status display |

## Cross-Page Patterns

**Download integration** (used by MusicPage, SearchPage, RecsPage):
1. Call `api.downloadSearch(query)` or `api.download(url)`
2. Poll `api.downloadJob(id)` every 1.5-2s via `setInterval`
3. On success: refresh relevant data + show toast
4. Clean up interval in `useEffect` return
5. Toast on error with `useToast()`

**Playlist creation** (used by TrackList, RecsPage):
1. `api.createPlaylist(name)` → get playlist ID
2. `api.addToPlaylist(playlistId, trackId)` for each track
3. Toast on success/failure

## Working Here

- See individual component files for implementation details
- API changes: update types in `lib/api.ts` first, then pages
- New page: add to `App.tsx` routes + nav items, create component
- New feature across pages: pattern-match existing implementations
