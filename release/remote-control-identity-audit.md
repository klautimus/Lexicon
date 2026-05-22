# Remote Control Identity & Role Assignment System — Audit

**Date:** 2026-05-21
**Audited by:** Atlas (researcher, kanban task t_af7a347e)
**Scope:** playerws hub (backend), playerws.ts (frontend WS client), DevicePicker.tsx, PlayerContext.tsx

---

## 1. How Device Registration Works

### Backend: `backend/internal/playerws/hub.go`

**ServeHTTP (line 166–199)** — the WebSocket upgrade handler:

```
ws://host/api/ws/player?deviceID=X&role=Y&name=Z
```

| Query Param | Default | Description |
|-------------|---------|-------------|
| `deviceID` | `r.RemoteAddr` | Unique device identifier |
| `role` | `"controller"` | Either `"player"` or `"controller"` |
| `name` | same as `role` | Human-readable device name |

On upgrade:
1. A `Client{hub, conn, send chan, deviceID, role, name}` struct is created.
2. Sent into `hub.register` channel.
3. `writePump()` and `readPump()` goroutines are spawned.

**Registration handler (Run loop, lines 97–109):**

```go
case client := <-h.register:
    h.mu.Lock()
    // If a new player registers, demote the old one
    if client.role == "player" {
        for id, c := range h.clients {
            if c.role == "player" && id != client.deviceID {
                c.role = "controller"
            }
        }
    }
    h.clients[client.deviceID] = client
    h.mu.Unlock()
    h.broadcastDevices()
```

**Key behavior: SINGLE-PLAYER HUB.** Only one client can have the `"player"` role at a time. When a new player registers, all existing players are demoted to `"controller"` (in-memory only — no message is sent to the demoted client except through the subsequent `broadcastDevices()`).

**broadcastDevices() (lines 136–164):**
Sends a `{type: "devices", list: [...]}` message to ALL connected clients. Each device entry:
```json
{"id": "dev-a1b2c3d4", "name": "Windows PC", "type": "player", "active": true}
```
- `id` = client.deviceID
- `name` = client.name
- `type` = client.role (string: "player" or "controller")
- `active` = (role == "player")

**readPump message routing (lines 228–245):**
- Controller messages (play, pause, resume, next, prev, seek, transfer, set_queue) are forwarded to the current player.
- Player `state` messages are broadcast to ALL controllers.
- **BUG:** `MsgTransfer` ("transfer") is defined (line 33) but has no special handling in readPump. It is forwarded from controller→player like any other command, but the player-side handling (PlayerContext.tsx) just pauses audio and sets `isWsPlayerRef.current = false` — it does NOT actually transfer playback to the target device.

### Frontend: `frontend/src/lib/playerws.ts`

**Constructor (lines 50–63):**
1. **Device ID generation:** `"dev-" + Math.random().toString(36).substring(2, 10)` → e.g., `"dev-a1b2c3d4"`. Stored in `localStorage` (survives browser restarts, shared across tabs).
2. **Role assignment:** First-come-first-served via `sessionStorage.getItem("playerActive")`. If the key is absent, this tab becomes `"player"` and sets the key to `"1"`. Otherwise, `"controller"`.
3. **Name detection:** Parses `navigator.userAgent` → "iPhone", "iPad", "Android Device", "Windows PC", "Mac", or "Browser".

**Connection URL:**
```
ws://{host}/api/ws/player?deviceID={dev-a1b2c3d4}&role={player|controller}&name={name}
```

---

## 2. What Device Info the Frontend Receives

### DeviceList Interface (playerws.ts, lines 23–31)

```typescript
export interface DeviceList {
  type: "devices";
  list: Array<{
    id: string;       // e.g., "dev-a1b2c3d4"
    name: string;     // e.g., "Windows PC"
    type: string;     // "player" or "controller"
    active: boolean;   // true if this device is the current player
  }>;
}
```

### localStorage Caching (DevicePicker.tsx, lines 26–28)

The device list is cached to `localStorage` under key `"playerDevices"` every time a `devices` message arrives:
```typescript
localStorage.setItem("playerDevices", JSON.stringify(m));
```

On DevicePicker open, the initial device list is read from this cache:
```typescript
const msg = JSON.parse(localStorage.getItem("playerDevices") || '{"list":[]}');
const wsDevices: Device[] = msg.list || [];
```

### Live Updates

When the DevicePicker is open, it registers a handler via `ws.onDevices()` that receives real-time `devices` messages from the WebSocket and updates state + localStorage cache.

---

## 3. How Role Assignment Works

### Frontend (playerws.ts, lines 54–57)

```typescript
const isFirst = !sessionStorage.getItem("playerActive");
this.role = isFirst ? "player" : "controller";
if (isFirst) sessionStorage.setItem("playerActive", "1");
```

**Claimed mechanism: "first-come-first-served" using sessionStorage.**

**Reality: This does NOT work across tabs.** `sessionStorage` is **per-origin, per-tab** — each browser tab gets its own isolated copy. This means:

- **Tab 1** opens Lexicon → `sessionStorage.playerActive` is empty → becomes `"player"` → connects to WS as player
- **Tab 2** opens Lexicon → `sessionStorage.playerActive` is empty (separate tab!) → becomes `"player"` → connects to WS as player → **backend demotes Tab 1 to controller**

**Every new tab steals the player role.** The sessionStorage guard is completely ineffective for cross-tab coordination.

### Backend Enforcement (hub.go, lines 99–106)

The backend enforces single-player semantics: when a new player registers, all existing players are demoted. But this happens **in-memory only** — there is no message sent to the demoted client telling it "you are no longer the player." The demoted client only discovers this via the subsequent `broadcastDevices()` message, where its own entry shows `type: "controller"` and `active: false`.

### PlayerContext.tsx Role Tracking (lines 86–87, 387)

```typescript
const wsRef = useRef<ReturnType<typeof getPlayerWebSocket> | null>(null);
const isWsPlayerRef = useRef<boolean>(false);
// ...
isWsPlayerRef.current = ws.isPlayer();
```

`isWsPlayerRef` is set once at mount and never updated when the backend demotes the client. This means a demoted client continues to think it's the player, broadcasting state, accepting commands — until a page reload.

### Transfer to "Host" (DevicePicker.tsx, lines 69–72)

```typescript
} else if (device.id === "host") {
    sessionStorage.setItem("playerActive", "1");
    window.location.reload();
}
```

This is the only place `sessionStorage.setItem("playerActive", "1")` is called outside the constructor — it's a **nuclear option**: force this tab to become the player on reload.

---

## 4. The 'host' Concept — Complete Map

The string `"host"` appears in **5 locations** across the codebase, and it does NOT correspond to any real WebSocket device ID.

### Location 1: DevicePicker default active device (line 17)
```typescript
const [activeDevice, setActiveDevice] = useState<string>("host");
```
Always starts as "host" — before any real device info arrives.

### Location 2: DevicePicker handleTransfer (line 69)
```typescript
} else if (device.id === "host") {
    sessionStorage.setItem("playerActive", "1");
    window.location.reload();
}
```
"host" transfer = full page reload to take over as player.

### Location 3: DevicePicker "Host Computer" button (line 129)
```typescript
onClick={() => handleTransfer({ id: "host", name: "Host Computer", type: "player", active: activeDevice === "host" })}
```
Hardcoded synthetic device entry.

### Location 4: DevicePicker WS device filter (line 144)
```typescript
.filter((d) => d.id !== "host" && d.type !== "spotify")
```
Prevents a real WS device named "host" from being listed — this is defensive but unnecessary since no real device has this ID.

### Location 5: PlayerContext broadcastState (line 436)
```typescript
wsRef.current.send({
    type: "state",
    playing: s.playing,
    track: s.current,
    position: s.position,
    duration: s.duration,
    device: "host",
});
```
The player broadcasts state with `device: "host"` — a **hardcoded magic string** that does not match the player's actual deviceID (`dev-a1b2c3d4`). This means remote controllers receive state updates from a device that doesn't appear in the device list by ID.

### What "host" Actually Means

"host" is an **abstraction** for "the computer running the Lexicon backend server that has speakers connected." It represents the physical audio output, not any specific WebSocket client. The DevicePicker treats "Host Computer" as a special entity separate from all WebSocket-connected browsers.

### The Identity Gap

| Concept | Real ID | What the Code Uses |
|---------|---------|-------------------|
| Tab playing audio | `dev-a1b2c3d4` (WebSocket) | "host" (magic string) |
| The actual computer | n/a | "Host Computer" (UI label) |
| "This Device" (current tab) | `dev-a1b2c3d4` | "self" (another magic string) |

The device list broadcast by the hub contains real IDs (`dev-a1b2c3d4`), but the state broadcast uses `"host"`. Controllers receiving state messages cannot correlate the `device: "host"` field with any entry in the device list.

---

## 5. Device Metadata Available

### From WebSocket `devices` message

| Field | Source | Values |
|-------|--------|--------|
| `id` | `deviceID` query param → `localStorage` | `"dev-" + 8 hex chars` |
| `name` | `name` query param → UA detection | `"Windows PC"`, `"Mac"`, `"iPhone"`, `"Android Device"`, `"Browser"` |
| `type` | `role` query param → sessionStorage | `"player"` or `"controller"` |
| `active` | computed from role | `true` if `role == "player"` |

### From Spotify Connect API

| Field | Source | Values |
|-------|--------|--------|
| `id` | Spotify API | Spotify device ID string |
| `name` | Spotify API | User-set device name |
| `type` | Spotify API | `"Computer"`, `"Smartphone"`, `"Speaker"`, etc. |
| `is_active` | Spotify API | boolean |

### What's NOT Available

- **No device capabilities** (can it play audio? what formats? what's its latency?)
- **No network info** (IP, latency, connection quality)
- **No hardware info** (speaker count, audio output device name)
- **No persistent identity** beyond the random `dev-` ID (regenerated if localStorage is cleared)
- **No role transition events** (demoted clients aren't explicitly notified)

---

## 6. DevicePicker Rendering Logic

### Device List Structure (top to bottom)

```
┌─ Playing On ──────────────────────────────┐
│                                             │
│  📱 This Device          (id="self")        │
│     Stream audio here                        │
│                                             │
│  🖥️ Host Computer        (id="host")         │
│     Playing: {track.title}                   │
│                                             │
│  🖥️ Windows PC           (id="dev-a1b2c3d4") │  ← WS devices, filtered
│     player                  ✓ active         │     by d.id !== "host"
│  📱 iPhone               (id="dev-9f8e7d6c") │     && d.type !== "spotify"
│     controller                              │
│                                             │
│  ── Spotify Connect ──                      │
│  🔈 Living Room Speaker  (Spotify ID)        │
│     Speaker • Active                        │
└─────────────────────────────────────────────┘
```

### Data Sources

| Section | Source | Refresh |
|---------|--------|---------|
| "This Device" | Hardcoded | n/a |
| "Host Computer" | Hardcoded | n/a |
| WS devices | `localStorage.playerDevices` + live WS `devices` messages | Real-time + cached |
| Spotify devices | `fetchSpotifyDevices()` API call | Polled every 10s while open |

### Active Device Indication

- `activeDevice` state is local to DevicePicker — it's the last device the user clicked
- Defaults to `"host"` on mount
- Checkmark (✓) shown next to the device matching `activeDevice`
- For WS devices, the `active` field from the hub is also shown with a checkmark — these can disagree with `activeDevice` state

### Transfer Behavior by Device Type

| Device Type | Action |
|-------------|--------|
| `"spotify"` | Calls `transferSpotifyPlayback(device.id, true)` → Spotify API |
| `id === "host"` | Sets `sessionStorage.playerActive = "1"`, then `window.location.reload()` |
| Everything else | Calls `ws.transfer(device.id)` → sends `{type: "transfer", target: device.id}` over WebSocket |

---

## 7. Critical Issues Found

### 7.1 sessionStorage Does NOT Prevent Multiple Players (SEVERITY: HIGH)

**Root cause:** `sessionStorage` is per-tab. Every new tab sees an empty `playerActive` and claims to be the player.

**Impact:** Two tabs on the same machine both think they're the player. The backend demotes the first, but the first tab's `isWsPlayerRef` remains `true` — it keeps broadcasting state, keeps trying to play audio, and doesn't know it was demoted.

**Current mitigation:** The backend's single-player enforcement (demoting old players on new registration) prevents audio chaos, but the frontend state is wrong.

### 7.2 'host' Is a Magic String With No Identity (SEVERITY: MEDIUM)

**Root cause:** The code uses `"host"` as a device identifier in 4 places, but `"host"` never appears in the real device list from the hub. The actual player's WS device ID (e.g., `"dev-a1b2c3d4"`) is never used as an identity for the playing device.

**Impact:**
- Controllers receive state with `device: "host"` but have no device in their list with that ID
- Cannot correlate state messages to device list entries
- `broadcastState()` sends a hardcoded string instead of the actual `ws.getDeviceID()`
- Any future feature that needs to know "which device is playing" will be broken

### 7.3 Transfer Message Has No Server-Side Handler (SEVERITY: MEDIUM)

**Root cause:** `MsgTransfer` is defined (line 33) and controllers can send it, but `readPump` in hub.go has no case for it. The message passes through to the player, which just pauses audio and sets `isWsPlayerRef.current = false`. There's no actual transfer of playback responsibility.

**Impact:** `DevicePicker.handleTransfer` for remote WS devices sends a transfer message that effectively pauses audio on the current player but doesn't start playback on the target.

### 7.4 No Explicit Demotion Notification (SEVERITY: LOW)

**Root cause:** When the backend demotes a player to controller, the client only discovers this via the `devices` message (where its own entry shows `active: false`). There's no explicit `"you were demoted"` message.

**Impact:** The demoted client's `isWsPlayerRef` remains `true` until the next page load. It continues to broadcast state and accept commands that should be going to the actual player.

### 7.5 "This Device" and "Host Computer" Are Redundant/Confusing (SEVERITY: LOW)

The DevicePicker has both:
- "This Device" (`id="self"`) — the current browser tab
- "Host Computer" (`id="host"`) — the physical computer

For the tab that IS the host computer, these two entries represent the same thing. For remote controllers, "This Device" (phone) is distinct from "Host Computer" (desktop running Lexicon). The UI doesn't explain this distinction.

### 7.6 DevicePicker's activeDevice State Drifts From Reality (SEVERITY: LOW)

`activeDevice` is set when the user clicks a device in the picker, but it's never synced with the actual player state from the hub. If the backend demotes the player role to another device, the DevicePicker still shows the last-clicked device as active with a checkmark.

---

## 8. Suggested Fixes (for downstream task)

1. **Replace sessionStorage with BroadcastChannel or a server-enforced role.** The backend already enforces single-player — the frontend should accept the backend's authority. On receiving a `devices` message where this client's `active` is `false` but `isWsPlayerRef` is `true`, set `isWsPlayerRef = false`.

2. **Replace `"host"` with the actual WebSocket deviceID.** In `broadcastState()`, use `ws.getDeviceID()` instead of `"host"`. In DevicePicker, derive the "Host Computer" entry from the WS device list (find the entry with `type === "player"` and display it) instead of hardcoding `id="host"`.

3. **Implement server-side transfer handling.** In hub.go's readPump, add a case for `MsgTransfer` that: (a) tells the current player to stop, (b) tells the target device it's now the player, (c) updates roles and rebroadcasts the device list.

4. **Add explicit demotion messages.** When the backend demotes a player, send a `{type: "demoted"}` message to that client so it can update `isWsPlayerRef` immediately.

5. **Sync DevicePicker activeDevice with hub state.** Watch for `devices` messages and update `activeDevice` to match the actual active player's device ID.

---

## Source Files Audited

| File | Lines | Role |
|------|-------|------|
| `backend/internal/playerws/hub.go` | 280 | WebSocket hub, registration, broadcasting, message routing |
| `frontend/src/lib/playerws.ts` | 204 | Frontend WS client, device ID generation, role assignment, command sending |
| `frontend/src/components/DevicePicker.tsx` | 187 | Device selection UI, transfer handling, device caching |
| `frontend/src/player/PlayerContext.tsx` | 653 | Player state management, WS integration, state broadcasting |
