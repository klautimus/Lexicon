# Lexicon Implementation Plan: Podcast Integration, Device Control & Playlist Enhancement

> **Date:** 2026-05-17
> **Version:** 1.0
> **Scope:** Three feature additions to Lexicon v2.7.1

---

## Table of Contents

1. [Feature 1: poddl Podcast Integration](#feature-1-poddl-podcast-integration)
2. [Feature 2: Device Selection & Remote Control](#feature-2-device-selection--remote-control)
3. [Feature 3: AI Playlist Track Count Enhancement](#feature-3-ai-playlist-track-count-enhancement)
4. [Build & Deployment Notes](#build--deployment-notes)

---

## Feature 1: poddl Podcast Integration

### 1.1 Overview

Integrate [freshe/poddl](https://github.com/freshe/poddl) — a cross-platform CLI podcast downloader — into Lexicon. Users can paste RSS feed URLs to subscribe to podcasts, browse episodes, download them, and have episodes appear in the podcast catalogue. The LLM chat interface can also discover and download podcasts via natural language.

### 1.2 Architecture Analysis & Approach Selection

**Approach A (Chosen): Bundle poddl.exe as external tool**
- Follows existing pattern: spotiflac.exe, yt-dlp.exe, spotdl.exe all work this way
- Minimal code changes — subprocess call from Go, parse output, trigger rescan
- poddl is Go-based, compiles to single .exe, Windows-compatible
- Can be bundled in installer like other tools

**Approach B (Rejected): Port poddl logic into Go backend directly**
- More control but duplicates effort
- poddl's RSS parsing and download logic would need to be reimplemented
- Harder to maintain (upstream updates to poddl wouldn't benefit Lexicon)

**Approach C (Rejected): Use poddl as Go library**
- poddl is designed as a CLI tool, not a library
- Would require refactoring poddl's internal API
- Tight coupling with upstream changes

### 1.3 Backend Changes

#### 1.3.1 New Package: `backend/internal/podcaster/`

**File: `backend/internal/podcaster/podcaster.go`** (~400 LOC)

```go
type API struct {
    db          *sql.DB
    cfg         Config
    downloadDir string
    rescanFunc  func()
}

type Config struct {
    PoddlBin    string  // path to poddl.exe
    OutputDir   string  // where to store podcast files
    AutoDownload bool   // auto-download new episodes for subscribed feeds
}
```

**Key methods:**
- `Subscribe(feedURL string)` — Fetch RSS, parse feed metadata, store in `podcast_feeds`, trigger initial episode sync
- `Unsubscribe(feedID int64)` — Remove feed and all associated episodes (optionally keep downloaded files)
- `ListFeeds()` — Return all subscribed feeds with episode counts
- `ListEpisodes(feedID int64)` — Return episodes for a feed with download status
- `DownloadEpisode(episodeID int64)` — Download single episode via poddl
- `DownloadFeed(feedID int64, filter EpisodeFilter)` — Download all/filtered episodes
- `SyncFeed(feedID int64)` — Re-fetch RSS, update episode list, optionally auto-download new
- `SyncAllFeeds()` — Background sync of all subscribed feeds

**RSS Parsing:**
Use `github.com/mmcdole/gofeed` (pure Go, no CGO) for RSS/Atom parsing:
```go
import "github.com/mmcdole/gofeed"

fp := gofeed.NewParser()
feed, err := fp.ParseURL(feedURL)
// feed.Title, feed.Description, feed.Image.URL
// feed.Items[].Title, .Description, .PublishedParsed
// feed.Items[].Enclosures[].URL, .Length, .Type
```

**poddl subprocess execution:**
```go
// poddl usage: poddl -o <output_dir> <rss_url>
// For single episode: poddl -o <output_dir> --episode <guid> <rss_url>
cmd := exec.Command(a.cfg.PoddlBin, "-o", outputDir, feedURL)
// Stream stdout/stderr into job log (same pattern as downloader.go)
```

#### 1.3.2 Database Schema (New Tables)

Add to `backend/internal/db/db.go` in the `Migrate()` function:

```sql
CREATE TABLE IF NOT EXISTS podcast_feeds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    title TEXT,
    description TEXT,
    image_url TEXT,
    author TEXT,
    link TEXT,
    language TEXT,
    last_fetched_at INTEGER,
    last_error TEXT,
    auto_download INTEGER NOT NULL DEFAULT 0,
    download_folder TEXT,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
CREATE INDEX IF NOT EXISTS idx_podcast_feeds_url ON podcast_feeds(url);

CREATE TABLE IF NOT EXISTS podcast_episodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_id INTEGER NOT NULL REFERENCES podcast_feeds(id) ON DELETE CASCADE,
    guid TEXT NOT NULL,
    title TEXT,
    description TEXT,
    pub_date INTEGER,
    duration_sec INTEGER,
    audio_url TEXT,
    audio_type TEXT,
    audio_size INTEGER,
    downloaded INTEGER NOT NULL DEFAULT 0,
    file_path TEXT,
    file_size INTEGER,
    download_error TEXT,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    UNIQUE(feed_id, guid)
);
CREATE INDEX IF NOT EXISTS idx_podcast_episodes_feed ON podcast_episodes(feed_id);
CREATE INDEX IF NOT EXISTS idx_podcast_episodes_downloaded ON podcast_episodes(downloaded);
```

#### 1.3.3 API Routes

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/podcasts/feeds` | List subscribed feeds with episode counts |
| `POST` | `/api/podcasts/subscribe` | Subscribe to a feed (`{"url": "..."}`) |
| `DELETE` | `/api/podcasts/feeds/{id}` | Unsubscribe from a feed |
| `GET` | `/api/podcasts/feeds/{id}/episodes` | List episodes for a feed |
| `POST` | `/api/podcasts/feeds/{id}/sync` | Re-fetch RSS and update episodes |
| `POST` | `/api/podcasts/episodes/{id}/download` | Download a specific episode |
| `POST` | `/api/podcasts/feeds/{id}/download` | Download all undownloaded episodes |
| `GET` | `/api/podcasts/status` | Check poddl.exe availability |

#### 1.3.4 Config Changes

Add to `backend/internal/config/config.go`:
```go
PoddlBin    string  // PODDL_BIN env var
PodcastDir  string  // PODCAST_DIR env var (defaults to first media root)
```

Add to `.env.example`:
```
PODDL_BIN=
PODCAST_DIR=
```

#### 1.3.5 Main.go Wiring

In `backend/cmd/server/main.go`:
```go
podcastAPI := podcaster.New(database, podcaster.Config{
    PoddlBin:     cfg.PoddlBin,
    OutputDir:    cfg.PodcastDir,
    AutoDownload: true,
}, doRescan)
podcastAPI.Mount(r)

// Background feed sync (every 30 minutes)
go func() {
    ticker := time.NewTicker(30 * time.Minute)
    for range ticker.C {
        podcastAPI.SyncAllFeeds()
    }
}()
```

#### 1.3.6 LLM Chat Integration for Podcasts

Extend the chat handler in `recommender.go` to detect podcast-related requests:

**Approach:** Add podcast detection to the chat system prompt and provide a `PodcastTool` that the LLM can invoke.

1. Add podcast context to the chat system prompt:
```
You have access to a podcast subscription system. You can:
- Subscribe users to podcasts by RSS URL
- Search for podcasts by topic/keyword (using web search)
- Download specific episodes or entire shows
- List the user's subscribed podcasts

When the user asks about podcasts, use the podcast system to help them.
```

2. Add a `handlePodcastIntent()` function that parses the LLM's response for podcast actions:
```go
type PodcastIntent struct {
    Action    string `json:"action"`     // "subscribe", "download", "list", "search"
    FeedURL   string `json:"feed_url"`   // RSS URL for subscribe
    Query     string `json:"query"`      // search query
    EpisodeID int64  `json:"episode_id"` // specific episode
}
```

3. The chat handler detects podcast intents and routes them to the podcaster API, then returns the result to the LLM for a natural language response.

### 1.4 Frontend Changes

#### 1.4.1 New API Client Methods (`frontend/src/lib/api.ts`)

```typescript
export const api = {
  // ... existing methods ...

  // Podcast feeds
  podcastFeeds: () => j<PodcastFeed[]>('/podcasts/feeds'),
  podcastSubscribe: (url: string) =>
    j<PodcastFeed>('/podcasts/subscribe', { method: 'POST', body: JSON.stringify({ url }) }),
  podcastUnsubscribe: (id: number) =>
    j<{ ok: boolean }>(`/podcasts/feeds/${id}`, { method: 'DELETE' }),
  podcastEpisodes: (feedId: number) =>
    j<PodcastEpisode[]>(`/podcasts/feeds/${feedId}/episodes`),
  podcastSync: (feedId: number) =>
    j<{ ok: boolean }>(`/podcasts/feeds/${id}/sync`, { method: 'POST' }),
  podcastDownloadEpisode: (episodeId: number) =>
    j<{ ok: boolean }>(`/podcasts/episodes/${episodeId}/download`, { method: 'POST' }),
  podcastDownloadFeed: (feedId: number) =>
    j<{ ok: boolean }>(`/podcasts/feeds/${feedId}/download`, { method: 'POST' }),
  podcastStatus: () => j<{ available: boolean; bin?: string }>('/podcasts/status'),
};

export interface PodcastFeed {
  id: number;
  url: string;
  title: string;
  description: string;
  image_url: string;
  author: string;
  episode_count: number;
  downloaded_count: number;
  last_fetched_at: number;
  auto_download: boolean;
}

export interface PodcastEpisode {
  id: number;
  feed_id: number;
  guid: string;
  title: string;
  description: string;
  pub_date: number;
  duration_sec: number;
  audio_url: string;
  downloaded: boolean;
  file_path: string;
}
```

#### 1.4.2 Redesigned PodcastsPage (`frontend/src/pages/PodcastsPage.tsx`)

**Current state:** 16 lines — just a flat list of podcast tracks from the library.

**New design:** Full podcast management interface with two-panel layout.

```
┌─────────────────────────────────────────────────────────────┐
│  Podcasts                              [+ Add Podcast]     │
├──────────────────┬──────────────────────────────────────────┤
│                  │                                          │
│  ┌────────────┐  │  Episode Title 1                    ▶   │
│  │  [image]   │  │  Published: May 15 · 45 min             │
│  │  Show Name │  │  Description text...                    │
│  │  24 eps    │  │                                          │
│  └────────────┘  │  Episode Title 2                    ▶   │
│                  │  Published: May 10 · 38 min             │
│  ┌────────────┐  │  Description text...                    │
│  │  [image]   │  │                                          │
│  │  Show Name │  │  Episode Title 3              [Download]│
│  │  12 eps    │  │  Published: May 8 · 52 min              │
│  └────────────┘  │  Description text...                    │
│                  │                                          │
│  ──────────────  │  ...                                     │
│  Subscribed: 5   │                                          │
│                  │                                          │
└──────────────────┴──────────────────────────────────────────┘
```

**Component structure:**
```
PodcastsPage.tsx (wrapper, ~80 lines)
  ├── PodcastSidebar.tsx (~120 lines) — Feed list with artwork cards
  ├── EpisodeList.tsx (~150 lines) — Episode list with download/play
  ├── AddPodcastModal.tsx (~100 lines) — RSS URL input + search
  └── PodcastChat.tsx (~80 lines) — LLM chat for podcast discovery
```

**Key UI features:**
- **Left sidebar:** Scrollable list of subscribed podcast cards (artwork, title, episode count, sync status). Click to select.
- **Right panel:** Episode list for selected feed. Each episode shows: title, date, duration, download status, play/download buttons.
- **Add Podcast button:** Opens modal with RSS URL input field + "Search by name" that uses web search to find RSS feeds.
- **Podcast Chat panel:** Collapsible chat at the bottom: "Find me podcasts about true crime" → LLM searches, returns results with one-click subscribe.
- **Auto-refresh:** Poll for download progress every 2 seconds (same pattern as DownloadsPage).
- **Episode actions:** Play (if downloaded), Download (if not), Delete local file.
- **Feed actions:** Sync now, Toggle auto-download, Unsubscribe.

#### 1.4.3 New Component: AddPodcastModal

A beautiful modal with two tabs:
1. **"Paste RSS URL"** — Text input for direct RSS URL pasting. Validate and preview feed metadata before subscribing.
2. **"Search"** — Text input for searching podcasts by name/topic. Uses web search to find RSS feeds. Shows results with artwork, description, and subscribe button.

#### 1.4.4 Navigation Update

Update `App.tsx` to show podcast count badge on the Podcasts nav item.

### 1.5 Tool Bundling

- Download poddl.exe Windows binary from GitHub releases
- Place in `tools/poddl.exe`
- Update `build.ps1` to copy to `release/tools/`
- Update `lexicon.iss` to include in installer

---

## Feature 2: Device Selection & Remote Control

### 2.1 Overview

Enable users to choose which device plays audio. When accessing Lexicon from a phone/tablet on the LAN, users can either:
1. Play audio on their current device (streaming from the host)
2. Control playback on the host computer (remote control)
3. Transfer Spotify playback to any Spotify Connect device

### 2.2 Architecture Analysis & Approach Selection

**Approach A (Chosen): Hybrid WebSocket + Spotify Connect**
- WebSocket channel for local file control between devices
- Spotify Connect API for Spotify device selection
- Unified device picker UI in PlayerBar
- No additional infrastructure needed (WebSocket server runs on existing Go backend)

**Approach B (Rejected): Custom audio streaming server**
- Would require building a separate audio streaming protocol
- Duplicates what HTTP range requests already provide
- Over-engineered for the use case

**Approach C (Rejected): Snapcast/multi-room audio**
- Requires additional software installation
- Too complex for Lexicon's scope
- Doesn't integrate with Spotify

### 2.3 Backend Changes

#### 2.3.1 WebSocket Endpoint

**File: `backend/internal/playerws/hub.go`** (~250 LOC)

A lightweight WebSocket hub that manages device connections and routes control messages.

```go
type Hub struct {
    clients    map[string]*Client  // deviceID -> Client
    register   chan *Client
    unregister chan *Client
    broadcast  chan []byte
    mu         sync.RWMutex
}

type Client struct {
    hub      *Hub
    conn     *websocket.Conn
    send     chan []byte
    deviceID string
    role     string  // "player" (host audio output) or "controller" (remote control)
    name     string  // "Host Computer", "iPhone", etc.
}
```

**Message Protocol:**

```json
// Controller → Server → Player
{
    "type": "play",
    "track_id": 123,
    "position": 0
}

{
    "type": "pause"
}

{
    "type": "resume"
}

{
    "type": "next"
}

{
    "type": "prev"
}

{
    "type": "seek",
    "position": 45.5
}

{
    "type": "set_queue",
    "tracks": [...],
    "start_index": 0
}

{
    "type": "transfer",
    "target": "host" | "self" | "spotify:device_id"
}

// Player → Server → All Controllers
{
    "type": "state",
    "playing": true,
    "track": {...},
    "position": 30.5,
    "duration": 180,
    "device": "host"
}

// Server → All Clients (device list update)
{
    "type": "devices",
    "list": [
        {"id": "host", "name": "Host Computer", "type": "player", "active": true},
        {"id": "controller-abc", "name": "iPhone Safari", "type": "controller", "active": false},
        {"id": "spotify:xyz", "name": "Living Room Speaker", "type": "spotify", "active": false}
    ]
}
```

**Route:** `GET /api/ws/player` (WebSocket upgrade)

**Device identification:**
- Host browser: Sets `role: "player"`, `deviceID: "host"` (only one player allowed)
- Other browsers: Set `role: "controller"`, `deviceID: random UUID`
- Device name auto-detected from User-Agent or set by user

#### 2.3.2 Spotify Device Integration

Add to `backend/internal/spotify/client.go`:

```go
// GetDevices returns all available Spotify Connect devices
func GetDevices(ctx context.Context, accessToken string) ([]SpotifyDevice, error) {
    // GET https://api.spotify.com/v1/me/player/devices
}

// TransferPlayback transfers playback to a specific device
func TransferPlayback(ctx context.Context, accessToken string, deviceID string, play bool) error {
    // PUT https://api.spotify.com/v1/me/player
    // {"device_ids": [deviceID], "play": play}
}
```

Add API route: `GET /api/spotify/devices` — Returns available Spotify Connect devices.

#### 2.3.3 Player State Synchronization

The host browser maintains the "master" player state. When a controller sends a command:
1. Controller sends command via WebSocket
2. Hub routes command to the player client
3. Player executes the command (play, pause, seek, etc.)
4. Player broadcasts new state to all controllers
5. All controllers update their UI

For local file playback on the host:
- The host browser's HTMLAudioElement is the actual audio output
- Controllers send commands that the host executes
- Position/duration are broadcast back to controllers

For "play on this device" (streaming):
- Each device can independently stream from `/api/stream/{id}`
- No WebSocket needed — just use the existing stream endpoint
- The device picker simply switches between "remote control" and "local playback" modes

### 2.4 Frontend Changes

#### 2.4.1 New Hook: `useDeviceManager`

**File: `frontend/src/hooks/useDeviceManager.ts`** (~120 lines)

```typescript
interface Device {
    id: string;
    name: string;
    type: 'player' | 'controller' | 'spotify';
    active: boolean;
}

interface DeviceManager {
    devices: Device[];
    currentDevice: Device | null;
    isHost: boolean;
    transferTo(deviceId: string): void;
    playOnThisDevice(trackId: number): void;
}
```

#### 2.4.2 WebSocket Client

**File: `frontend/src/lib/playerws.ts`** (~100 lines)

```typescript
class PlayerWebSocket {
    private ws: WebSocket;
    private deviceID: string;
    private role: 'player' | 'controller';

    connect();
    send(command: PlayerCommand);
    onState(callback: (state: PlayerState) => void);
    onDevices(callback: (devices: Device[]) => void);
}
```

#### 2.4.3 PlayerBar Device Picker UI

Add a device selector button to `PlayerBar.tsx` (next to the volume slider):

```
[🔊] [━━━━━━━━━━] [📱 Device ▼]
```

Clicking opens a beautiful dropdown:

```
┌─────────────────────────────────────┐
│  Playing On                         │
│  ┌─────────────────────────────────┐│
│  │ 🖥️  Host Computer     ✓ Now    ││
│  │     Playing: Song Title         ││
│  └─────────────────────────────────┘│
│  ┌─────────────────────────────────┐│
│  │ 📱  This Device                 ││
│  │     Stream audio here           ││
│  └─────────────────────────────────┘│
│  ─ Spotify Connect ──────────────── │
│  ┌─────────────────────────────────┐│
│  │ 🔊  Living Room Speaker         ││
│  │     Spotify Connect             ││
│  └─────────────────────────────────┘│
│  ┌─────────────────────────────────┐│
│  │ 📱  Kevin's iPhone              ││
│  │     Controller                  ││
│  └─────────────────────────────────┘│
└─────────────────────────────────────┘
```

**UI Design Notes:**
- Active device shown with a checkmark and "Now Playing" indicator
- "Host Computer" option: Controls the host's audio output (remote control mode)
- "This Device": Streams audio to the current browser (local playback mode)
- Spotify Connect devices listed separately with speaker icon
- Each device shows its status (playing, idle, offline)
- Smooth animations on transfer (fade out on old device, fade in on new)

#### 2.4.4 PlayerContext Modifications

Extend `PlayerContext.tsx` to:
1. Connect to WebSocket on mount
2. Register as "player" (if host) or "controller"
3. When acting as controller: send commands via WebSocket instead of controlling HTMLAudioElement
4. When acting as player: execute commands from WebSocket, broadcast state
5. Handle device transfer: stop local playback, send transfer command

#### 2.4.5 Mobile Player Bar

The `MobilePlayerBar.tsx` also gets the device picker button. On mobile, the default is "This Device" (stream to phone). Users can switch to "Host Computer" to control the host remotely.

### 2.5 Device Discovery Flow

1. Frontend loads, extracts `deviceID` from localStorage (or generates new UUID)
2. Connects to `ws://HOST:8787/api/ws/player?deviceID=xxx&name=xxx`
3. If no player is connected, this device becomes the player
4. If a player already exists, this device becomes a controller
5. Hub broadcasts updated device list to all clients
6. For Spotify devices: fetch from Spotify API, merge into device list

---

## Feature 3: AI Playlist Track Count Enhancement

### 3.1 Problem Analysis

Current state in `recommender.go`:
- Line 198: `"8-12 tracks"` hardcoded in playlist prompt
- Line 297: `"8-12 tracks"` hardcoded in chat prompt
- No user control over track count
- Users requesting more tracks in chat get ignored

### 3.2 Solution

#### 3.2.1 Backend Changes

**1. Add `count` parameter to playlist endpoint:**

```go
// In recommender.go playlist() method
count, _ := strconv.Atoi(r.URL.Query().Get("count"))
if count <= 0 {
    count = 25  // new default
}
if count > 100 {
    count = 100  // hard cap
}
```

**2. Update playlist prompt to use dynamic count:**

```go
prompt := fmt.Sprintf(`Given this user's listening profile, generate a cohesive playlist.
Return ONLY valid JSON with this exact shape:
{
  "name": "catchy playlist name",
  "description": "1-2 sentence vibe description",
  "tracks": [
    {"title":"...", "artist":"...", "reason":"brief why this fits"}
  ]
}

Rules:
- Generate exactly %d tracks
- Name should be creative and thematic (not generic like "My Playlist")
- Mix of songs the user likely has and new discoveries
- Be specific and personal — reference patterns from the profile
- Output ONLY valid JSON, no prose, no code fences.
%s
PROFILE:
%s`, count, searchContext, profile)
```

**3. Update chat prompt similarly:**

```go
// In chat() method, extract count from user message
count := extractCountFromMessage(req.Message) // parse "30-track", "50 songs", etc.
if count <= 0 {
    count = 25
}

system := fmt.Sprintf(`You are a music curator. ALWAYS respond with ONLY a single valid JSON object.

If the user asks for a playlist, use this shape:
{"message":"A short conversational reply","playlist":{"name":"Creative Playlist Name","description":"1-2 sentence vibe","tracks":[{"title":"...","artist":"...","reason":"..."}]}}

Rules:
- For playlist requests: generate exactly %d tracks, creative thematic name, reference patterns from the profile.
- For normal questions: answer concisely in the "message" field.
- ALWAYS include the "message" field.
- NEVER output text outside the JSON object.
%s
---
USER LISTENING PROFILE:
---
REMEMBER: Always return a JSON object with a "message" field.`, count, searchContext, profile)
```

**4. Add count extraction helper:**

```go
func extractCountFromMessage(msg string) int {
    // Match patterns like "30-track", "50 songs", "20 tracks", "100 song playlist"
    re := regexp.MustCompile(`(\d+)\s*(?:[- ]?(?:track|song))`)
    matches := re.FindStringSubmatch(strings.ToLower(msg))
    if len(matches) >= 2 {
        n, _ := strconv.Atoi(matches[1])
        if n > 0 && n <= 100 {
            return n
        }
    }
    return 0  // use default
}
```

**5. Update API route to accept count:**

The `generatePlaylist` API method already accepts a `force` parameter. Add `count`:

```go
// In api.ts
generatePlaylist: (force?: boolean, count?: number) => {
    let url = '/recommendations/playlist';
    const params = new URLSearchParams();
    if (force) params.set('force', 'true');
    if (count) params.set('count', count.toString());
    if (params.toString()) url += '?' + params.toString();
    return j<PlaylistPayload>(url, { method: 'POST' });
}
```

#### 3.2.2 Frontend Changes

**1. Add track count slider to RecsPage.tsx:**

Between the "Generate Playlist" button and the recommendations display, add a track count control:

```
┌─────────────────────────────────────────────────────────────┐
│  [Generate Playlist]  Tracks: [━━●━━━━━━━] 25    [Refresh]  │
│                        5 ←――――――――――――→ 50                  │
└─────────────────────────────────────────────────────────────┘
```

Implementation:
```tsx
const [trackCount, setTrackCount] = useState(25);

// In the button row:
<div className="flex items-center gap-3">
    <span className="text-sm text-muted">Tracks:</span>
    <input
        type="range"
        min={5}
        max={50}
        value={trackCount}
        onChange={(e) => setTrackCount(Number(e.target.value))}
        className="w-32 accent-accent"
    />
    <span className="text-sm font-medium w-8">{trackCount}</span>
</div>

// Pass to generate:
downloads.generateAiPlaylist(false, trackCount);
```

**2. Update DownloadContext to pass count:**

```typescript
generateAiPlaylist: async (force = false, count = 25) => {
    const r = await api.generatePlaylist(force, count);
    // ... rest of existing logic
}
```

**3. Chat naturally handles count:**

When the user types "make me a 40-track road trip playlist", the backend extracts "40" and generates exactly 40 tracks. No frontend changes needed for chat — it's all in the prompt parsing.

### 3.3 LLM Prompt Engineering Notes

To improve track count adherence:
1. Use "Generate exactly N tracks" instead of "N tracks" (more imperative)
2. Add a validation step: if the LLM returns fewer than N*0.8 tracks, make a follow-up request
3. Consider splitting very large requests (>30) into two batches to avoid LLM truncation

```go
// Post-processing validation
if len(out.Tracks) < int(float64(count)*0.8) {
    // Request additional tracks
    additional := count - len(out.Tracks)
    followupPrompt := fmt.Sprintf("Add %d more tracks to this playlist. Return ONLY a JSON array of [{\"title\":\"...\",\"artist\":\"...\",\"reason\":\"...\"}]", additional)
    // ... make followup request and append
}
```

---

## Build & Deployment Notes

### New Dependencies

**Go (go.mod):**
```
github.com/mmcdole/gofeed v1.1.3     // RSS parsing
github.com/gorilla/websocket v1.5.0   // WebSocket hub
```

**npm (package.json):**
No new dependencies needed — WebSocket API is built into browsers.

### New Environment Variables

```
PODDL_BIN=           # Path to poddl.exe (auto-detected if in tools/)
PODCAST_DIR=         # Where to store downloaded podcast episodes
```

### Build Script Updates (`build.ps1`)

Add to the tool bundling section:
```powershell
# poddl.exe
if (Test-Path "tools/poddl.exe") {
    Copy-Item "tools/poddl.exe" "release/tools/" -Force
}
```

### InnoSetup Updates (`lexicon.iss`)

Add to the `[Files]` section:
```iss
Source: "release\tools\poddl.exe"; DestDir: "{app}\tools"; Flags: ignorevar
```

### Migration Notes

New tables (`podcast_feeds`, `podcast_episodes`) use `CREATE TABLE IF NOT EXISTS` — no migration needed for existing users. The tables are created on first run.

### File Summary

**New files:**
- `backend/internal/podcaster/podcaster.go` (~400 LOC)
- `backend/internal/playerws/hub.go` (~250 LOC)
- `frontend/src/hooks/useDeviceManager.ts` (~120 LOC)
- `frontend/src/lib/playerws.ts` (~100 LOC)
- `frontend/src/components/AddPodcastModal.tsx` (~100 LOC)
- `frontend/src/components/PodcastSidebar.tsx` (~120 LOC)
- `frontend/src/components/EpisodeList.tsx` (~150 LOC)
- `frontend/src/components/PodcastChat.tsx` (~80 LOC)
- `frontend/src/components/DevicePicker.tsx` (~100 LOC)

**Modified files:**
- `backend/internal/db/db.go` — Add podcast_feeds, podcast_episodes tables
- `backend/internal/config/config.go` — Add PoddlBin, PodcastDir config fields
- `backend/internal/recommender/recommender.go` — Dynamic track count, podcast chat intents
- `backend/internal/spotify/client.go` — Add GetDevices, TransferPlayback
- `backend/cmd/server/main.go` — Wire podcaster, WebSocket hub, background sync
- `frontend/src/lib/api.ts` — Add podcast API methods, update generatePlaylist
- `frontend/src/pages/PodcastsPage.tsx` — Complete redesign
- `frontend/src/pages/RecsPage.tsx` — Add track count slider
- `frontend/src/player/PlayerContext.tsx` — WebSocket integration
- `frontend/src/components/PlayerBar.tsx` — Add device picker button
- `frontend/src/components/MobilePlayerBar.tsx` — Add device picker button
- `frontend/src/contexts/DownloadContext.tsx` — Add count parameter to generateAiPlaylist
- `release/build.ps1` — Bundle poddl.exe
- `release/lexicon.iss` — Include poddl.exe in installer
- `backend/.env.example` — Add PODDL_BIN, PODCAST_DIR

### Estimated Total Impact

- **New LOC:** ~1,200 (Go) + ~800 (TypeScript/React) = ~2,000 total
- **Modified LOC:** ~300 (across 14 files)
- **New DB tables:** 2
- **New API endpoints:** 10 (8 podcast + 1 WebSocket + 1 Spotify devices)
- **New config vars:** 2
- **New bundled tool:** 1 (poddl.exe)
