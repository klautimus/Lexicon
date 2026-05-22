# Lexicon Remote Control — Full Signal Flow Analysis

> Generated 2026-05-21. Covers hub.go v3.5.x (382 lines, with handleTransfer + lastState cache), playerws.ts (204 lines), PlayerContext.tsx (653 lines), DevicePicker.tsx (187 lines).

---

## 1. Architecture Overview

```
┌──────────────────┐          ┌──────────────────┐
│   Controller     │          │     Player        │
│  (phone/tablet)  │          │  (host desktop)   │
│                  │          │                   │
│  playerws.ts     │          │  playerws.ts      │
│  role=controller │          │  role=player      │
│  PlayerContext   │          │  PlayerContext    │
│  DevicePicker    │          │  DevicePicker     │
└────────┬─────────┘          └────────┬──────────┘
         │                             │
         │  WS /api/ws/player          │  WS /api/ws/player
         │  ?role=controller            │  ?role=player
         │                             │
         ▼                             ▼
┌─────────────────────────────────────────────────────┐
│                   Hub (hub.go)                       │
│                                                     │
│  ┌─────────┐   ┌──────────┐   ┌──────────────┐     │
│  │register │──▶│  Run()   │──▶│broadcastDevices│    │
│  └─────────┘   │  loop    │   └──────────────┘     │
│                │          │                         │
│  ┌─────────┐   │ ┌──────┐│   ┌──────────────┐     │
│  │broadcast│──▶│ │clients││──▶│ lastState    │     │
│  └─────────┘   │ │ map  ││   │ cache        │     │
│                │ └──────┘│   └──────────────┘     │
│  ┌─────────┐   │          │                         │
│  │unregister│──▶│          │   ┌──────────────┐     │
│  └─────────┘   └──────────┘   │handleTransfer│     │
│                               └──────────────┘     │
└─────────────────────────────────────────────────────┘
```

---

## 2. Client Registration Flow

### 2.1 Frontend: playerws.ts constructor (lines 50-62)

```typescript
constructor() {
    this.deviceID = localStorage.getItem("playerDeviceID") || this.generateID();
    localStorage.setItem("playerDeviceID", this.deviceID);

    const isFirst = !sessionStorage.getItem("playerActive");
    this.role = isFirst ? "player" : "controller";
    if (isFirst) sessionStorage.setItem("playerActive", "1");

    this.name = this.detectName();
    this.url = `ws://host/api/ws/player?deviceID=${...}&role=${this.role}&name=${...}`;
}
```

**Device ID:** Persistent across sessions via `localStorage`. Format: `dev-<8 random chars>`.

**Role assignment (BROKEN for cross-device):**
- Uses `sessionStorage.getItem("playerActive")` — per-browser, per-origin, per-tab.
- On a DESKTOP browser (host): sessionStorage is empty → `isFirst=true` → `role="player"`. Sets `playerActive="1"`.
- On a PHONE browser: sessionStorage is ALSO empty (different browser) → `isFirst=true` → `role="player"` too!
- **Result: Every device independently decides it should be the "player."**

**Name detection:** Based on `navigator.userAgent` — "Windows PC", "iPhone", "iPad", "Android Device", "Mac", "Browser".

### 2.2 Backend: Hub.ServeHTTP (lines 262-295) + Hub.Run() register case (lines 101-114)

```
Client connects → WS upgrade → Client{deviceID, role, name}
  → h.register <- client  [sends to Run() channel]

Run() register case:
  1. If role=="player", demote ALL existing players to "controller"
  2. Add client to h.clients map
  3. broadcastDevices() — send updated list to everyone
```

**Demotion logic (lines 103-109):**
```go
if client.role == "player" {
    for id, c := range h.clients {
        if c.role == "player" && id != client.deviceID {
            c.role = "controller"  // ← mutates Client in-place
        }
    }
}
```

**Key insight:** The hub's demotion fixes the frontend's broken role assignment, BUT it demotes the WRONG device. When a phone connects as "player", the desktop host (the actual audio-output device) gets demoted to "controller." The phone becomes the new "player" despite having no meaningful audio output context.

### 2.3 Registration Race Timeline

```
T0: Desktop opens Lexicon
    → sessionStorage empty → role="player"
    → Hub: desktop registered as player. playerActive="1" set.

T1: Phone opens Lexicon  
    → sessionStorage empty (different browser) → role="player"
    → Hub: demotes desktop → controller
    → Hub: phone registered as player
    → Hub: broadcastDevices() — phone is now "active", desktop is "controller"

T2: Desktop user tries to play music locally
    → Audio plays on desktop (PlayerContext still has local audio)
    → But desktop is now "controller" in hub → commands from phone go to... phone itself
    → State broadcast from desktop goes to all, but controllers ignore state messages (BUG #7)
```

---

## 3. Command Message Flow (play, pause, resume, next, prev, seek)

### 3.1 Controller Sends Command

```
[Controller - phone]
  DevicePicker or PlayerBar calls ws.play(trackId) / ws.pause() / etc.
    → playerws.ts send(): this.ws.send(JSON.stringify({type: "play", track_id: 5}))
    → WebSocket → Hub
```

### 3.2 Hub readPump Routes Command

```go
// hub.go lines 324-347
if msg.Type == MsgTransfer && msg.Target != "" {
    c.hub.handleTransfer(c, msg.Target)  // ← special case (see §5)
    continue
}

if c.role == "controller" {
    // Forward to the FIRST client with role=="player"
    for _, player := range c.hub.clients {
        if player.role == "player" {
            player.send <- data    // ← raw JSON, unchanged
            break                  // ← only sends to first player found
        }
    }
} else if c.role == "player" && msg.Type == MsgState {
    c.hub.broadcast <- data
}
// Any other case: message is SILENTLY DROPPED
```

**This routing has a critical asymmetry:**
- Controller→Player: forwards `{type:"play",...}` unchanged to the player
- Player→Controllers: only forwards `{type:"state",...}` via broadcast
- If a player sends `{type:"play",...}` (non-"state"): **DROPPED**
- If a controller sends `{type:"state",...}`: treated as a command → forwarded to player, never broadcast

### 3.3 Player Receives Command — HANDLER BUG

The frontend message dispatch in playerws.ts (lines 87-98):

```typescript
this.ws.onmessage = (ev) => {
    const msg = JSON.parse(ev.data);
    if (msg.type === "state") {
        this.stateHandlers.forEach((h) => h(msg));    // ← only for "state"
    } else if (msg.type === "devices") {
        this.deviceHandlers.forEach((h) => h(msg));   // ← only for "devices"
    }
    this.handlers.forEach((h) => h(msg));             // ← generic (EMPTY!)
};
```

**❌ CRITICAL BUG #1 — Message routing mismatch:**

PlayerContext.tsx registers its handler via `ws.onState()` (line 390), which pushes to `stateHandlers`. The handler is only invoked when `msg.type === "state"`.

BUT the handler code (lines 392-411) checks for command types:
```typescript
ws.onState((msg: any) => {
    if (!isWsPlayerRef.current) return;
    if (msg.type === "play" && msg.track_id) { ... }
    else if (msg.type === "pause") { ... }
    else if (msg.type === "resume") { ... }
    else if (msg.type === "next") { ... }
    else if (msg.type === "prev") { ... }
    else if (msg.type === "seek" && msg.position !== undefined) { ... }
    else if (msg.type === "transfer") { ... }
});
```

**The handler is registered for "state" messages but pattern-matches against "play"/"pause"/etc. — which are DIFFERENT message types that never arrive here.**

Since the `handlers` array (generic) is never populated (no public API on PlayerWebSocket to add to it), ALL controller→player commands are silently dropped.

**This means: NO remote control commands work. Not phone→host, not host→phone. Period.**

### 3.4 The `stateHandlers` vs `handlers` Architecture

```
PlayerWebSocket API surface:
  onState(handler)  → pushes to stateHandlers   (private, only type==="state")
  onDevices(handler) → pushes to deviceHandlers (private, only type==="devices")
  [NO onMessage / on() method exposed]

Message dispatch:
  type==="state"    → stateHandlers  → PlayerContext handler (but checks wrong subtypes)
  type==="devices"  → deviceHandlers → DevicePicker handler (works correctly)
  type==="play"     → handlers       → EMPTY (no API to populate) → DROPPED
  type==="pause"    → handlers       → EMPTY → DROPPED
  type==="promoted" → handlers       → EMPTY → DROPPED
  type==="demoted"  → handlers       → EMPTY → DROPPED
  type==="error"    → handlers       → EMPTY → DROPPED
```

---

## 4. State Broadcast Flow

### 4.1 Player Sends State

```typescript
// PlayerContext.tsx lines 427-438
const broadcastState = useCallback(() => {
    if (!isWsPlayerRef.current || !wsRef.current) return;
    const s = stateRef.current;
    wsRef.current.send({
        type: "state",
        playing: s.playing,
        track: s.current,
        position: s.position,
        duration: s.duration,
        device: "host",  // ← HARDCODED, should be ws.getDeviceID()
    });
}, []);  // ← empty deps = STABLE, never changes

// Line 440-442: called ONCE on mount
useEffect(() => {
    broadcastState();
}, [broadcastState]);
```

**❌ BUG #8 — State broadcast fires once, never updates:**

`broadcastState` has `useCallback(fn, [])` — stable reference. The effect at line 440-442 runs once on mount. When the track changes, when the user seeks, when playback pauses/resumes — nothing re-triggers `broadcastState`. Controllers see a single frozen snapshot, if they see anything at all.

**⚠️ Hardcoded `device: "host"`:** Should be the actual WebSocket deviceID (`ws.getDeviceID()`).

### 4.2 Hub Broadcasts State

```go
// hub.go Run() broadcast case (lines 126-141)
case message := <-h.broadcast:
    // Capture for transfer continuity
    var peek Message
    if json.Unmarshal(message, &peek) == nil && peek.Type == MsgState {
        h.lastState = message  // ← cached for handleTransfer
    }
    // Forward to ALL clients (including other players, if any)
    for _, client := range h.clients {
        client.send <- message
    }
```

### 4.3 Controllers Receive State

State messages arrive at controllers with `type: "state"`. The PlayerContext handler fires via `stateHandlers`, but:

```typescript
ws.onState((msg: any) => {
    if (!isWsPlayerRef.current) return;  // ← BUG #7
    // ...
});
```

**❌ BUG #7 — Controllers discard all state messages:**

On controllers, `isWsPlayerRef.current` is `false` (initialized from `ws.isPlayer()` at line 387, which returns `this.role === "player"`). So the handler returns immediately. Controllers have ZERO visibility into what's playing, position, duration, etc.

---

## 5. Transfer Flow

### 5.1 Full Transfer Signal Path

```
Step 1: Controller clicks transfer target in DevicePicker
  → DevicePicker handleTransfer():
      if device.id === "host":
          sessionStorage.setItem("playerActive", "1");  // ← "host" magic string
          window.location.reload();                     // ← destructive reload
      else if device.id !== "host":
          ws.transfer(device.id);
          → send({type: "transfer", target: "<deviceID>"})

Step 2: Hub readPump intercepts transfer (line 324-328)
  → c.hub.handleTransfer(c, msg.Target)
  → Skips normal controller→player forwarding

Step 3: handleTransfer() (lines 179-259)
  1. Validate target exists in h.clients[targetID]
     → If not found: send {type:"error", ...} back to source → nobody handles it (BUG #2)
  2. Find current player (first client with role=="player")
  3. Self-transfer check: if targetID == currentPlayer.deviceID → no-op
  4. Demote current player: currentPlayer.role = "controller"
  5. Promote target: target.role = "player"
  6. Build "promoted" message with lastState (track, position, duration, queue)
  7. Send promoted → target.send (direct, not broadcast)
  8. Send demoted → old player.send (direct, not broadcast)
  9. broadcastDevices() → everyone gets updated list

Step 4: New player receives {type:"promoted", ...}
  → playerws.ts onmessage: type==="promoted" → not "state", not "devices"
  → falls to handlers.forEach → EMPTY → DROPPED
  → ❌ BUG #3: No handler for "promoted" messages. The new player never knows it was promoted.

Step 5: Old player receives {type:"demoted"}
  → Same issue: falls to empty handlers → DROPPED
  → ❌ BUG #3: No handler for "demoted" messages. The old player keeps isWsPlayerRef=true,
    continues broadcasting state, continues trying to process commands (which are bugged anyway).

Step 6: PlayerContext.tsx transfer handler (lines 408-411)
  → Registered via onState(), only fires for type==="state" messages
  → Transfer messages are type==="transfer" → never reach this handler
  → ❌ BUG #5: The transfer handler is unreachable. Even if it worked, it only does:
      a.pause(); isWsPlayerRef.current = false;
    It doesn't forward state to the target, doesn't reconnect as controller, doesn't
    send any acknowledgment.
```

### 5.2 The 'host' Magic String Problem

```typescript
// DevicePicker.tsx lines 69-72
} else if (device.id === "host") {
    sessionStorage.setItem("playerActive", "1");
    window.location.reload();
}
```

**❌ BUG #9 — "host" has no mapping to real WebSocket device IDs:**

1. "host" is a hardcoded UI string, never appears in the hub's device list
2. The hub's device IDs are like `dev-a1b2c3d4` (generated by playerws.ts)
3. `sessionStorage.setItem("playerActive", "1")` then `window.location.reload()`
4. After reload: playerws.ts constructor checks `sessionStorage.getItem("playerActive")` → it IS set → `isFirst=false` → `role="controller"`
5. **Result: Clicking "Host Computer" makes this device a CONTROLLER, not a player.** The comment says "take over as player" but the logic does the exact opposite.

Even if you wanted to take over as player: (a) the page reload is destructive — loses all UI state, queue, current track; (b) the hub still has the old player registered — no new "player" registration happens because the reloaded page is a "controller".

### 5.3 The 'self' Magic String Problem

```typescript
// DevicePicker.tsx line 116
<button onClick={() => handleTransfer({ 
    id: "self", name: "This Device", type: "controller", 
    active: activeDevice === "self" 
})}>
```

This falls to `ws.transfer("self")` → sends `{type:"transfer", target:"self"}`. The hub's `handleTransfer` looks up deviceID="self" → not found → sends error back. Nobody handles errors.

### 5.4 Active Device Tracking

```typescript
// DevicePicker.tsx line 17
const [activeDevice, setActiveDevice] = useState<string>("host");
```

**❌ BUG #11 — Hardcoded to "host", never synced with hub state:**

`activeDevice` starts as "host" and only changes in `handleTransfer()` (line 76: `setActiveDevice(device.id)`). But hub-initiated role changes (demotion on new player registration, transfer from another controller) never update this. The DevicePicker always shows "host" as active, even when this device has been demoted to controller.

---

## 6. Edge Case Analysis

### 6.1 Disconnected Target

**Path:** Controller sends `{type:"transfer", target:"<offline-deviceID>"}`

**Hub:** `handleTransfer` validates `h.clients[targetID]` → not found → sends `{type:"error", ...}` back to source.

**❌ Nobody handles "error" messages:** The error message arrives at the controller's WebSocket. playerws.ts onmessage: type==="error" → not "state", not "devices" → goes to empty `handlers` → DROPPED. User sees nothing happen.

### 6.2 No Current Player

**Path:** All clients are controllers. Someone sends `{type:"transfer", target:"<controllerID>"}`

**Hub:** `handleTransfer`: no `currentPlayer` found → skips demotion (line 213-215) → promotes target directly → sends `{type:"promoted", ...}` to target.

**❌ Target never processes "promoted" (BUG #3).** The role changes in the hub's memory but the frontend never knows.

### 6.3 Self-Transfer

**Path:** Current player tries to transfer to itself.

**Hub:** `handleTransfer` line 207-210: `currentPlayer.deviceID == targetID` → no-op, returns.

**Frontend:** PlayerContext transfer handler (lines 408-411) would pause audio and set isWsPlayerRef=false, but this handler is unreachable (BUG #5).

### 6.4 Multiple Controllers, Conflicting Commands

**Path:** Controller A sends `{type:"play", track_id:5}`, Controller B simultaneously sends `{type:"pause"}`

**Hub:** Both commands are serialized through the Run() loop. Both forwarded to the player in order: play → pause.

**Player:** Both messages dropped by empty `handlers` (BUG #1). No conflict — nothing happens at all.

### 6.5 Player Disconnects During Transfer

**Hub:** `handleTransfer` holds `h.mu.Lock()` for the entire operation. Concurrent `unregister` is blocked until the lock is released. The disconnect processes after transfer completes. Outcome depends on which device disconnected:
- If old player disconnects: `demoted` message send fails silently (non-blocking select with default). No effect — old player is already gone.
- If target disconnects: target won't be in `h.clients` when handleTransfer runs (if disconnect happened before lock acquisition).

### 6.6 Race: New Player Connects During Transfer

**Hub:** Both operations go through the Run() loop's `select`. Only one processes at a time. If register beats transfer: new device becomes player (demoting old), then transfer swaps roles again. If transfer beats register: roles swap, then new player demotes everyone. Both outcomes are deterministic for the order of channel receives.

**No frontend locking mechanism exists.**

### 6.7 Two Browser Tabs on Same Machine

Each tab gets a different `deviceID` (line 66: `Math.random()`). Both detect the same name via `detectName()`. Both try to be "player" (sessionStorage is per-tab). Hub demotes the first one. Both appear in device list with identical names but different IDs.

### 6.8 Phone→Host Failure (The Primary Bug Scenario)

```
Prerequisites: Host desktop is running Lexicon, playing music.

1. Phone opens Lexicon
   → sessionStorage empty (different browser) → role = "player"
   → Hub demotes host desktop: host.role = "controller"
   → Hub promotes phone: phone.role = "player"

2. Phone user tries to control host playback (e.g., skip track)
   → ws.next() → sends {type:"next"}

3. Hub readPump: phone.role == "player"
   → Does NOT match the "controller" branch (line 331)
   → msg.Type == "next", not "state" → does NOT match the "player + state" branch (line 343)
   → MESSAGE DROPPED

BREAK POINT A: The phone is "player" but wants to send commands LIKE a controller.
readPump only forwards controller→player, not player→anyone. The phone's next/prev/
play/pause commands are silently ignored.

4. Alternatively, if the host desktop (now "controller") sends a command:
   → host.role == "controller" → forwarded to phone (player)
   → Phone receives {type:"next"}
   → playerws.ts: type=="next" → not "state" → goes to handlers → EMPTY → DROPPED

BREAK POINT B: Even with correct routing, the player never processes commands because
the handler is registered for "state" messages only (BUG #1) and the generic handlers
array is empty.
```

**Conclusion: phone→host remote control is broken at TWO independent layers — role assignment AND message dispatch. Both must be fixed for it to work.**

### 6.9 Host→Phone "Succeeds" (Why it appears to work)

If we correct the role assignment (host stays "player", phone is "controller"):

```
1. Host remains "player" (audio output)
2. Phone connects as "controller"
3. Host→phone path:
   a. Host's ws.send() goes to hub
   b. BUT host is "player" → readPump: c.role == "player"
   c. msg.Type != "state" → DROPPED by hub

Wait — that means host→phone ALSO doesn't work for commands!
```

The only thing that works host→phone (and why it might appear to "succeed" in testing) is:
- **State broadcast:** Player (host) sends state → hub broadcasts → phone receives it
- **BUT the phone discards it** (BUG #7: `!isWsPlayerRef.current` check returns early)

So host→phone doesn't actually work for COMMANDS either. The task's assertion that "host→phone succeeds" may be based on architectural intent (controller→player routing in readPump) rather than end-to-end testing.

---

## 7. Complete Message Type Matrix

| Message Type | Sent By | Routed By Hub | Received By | Processed? |
|---|---|---|---|---|
| `play` | controller | forwarded to player | player | ❌ No handler |
| `pause` | controller | forwarded to player | player | ❌ No handler |
| `resume` | controller | forwarded to player | player | ❌ No handler |
| `next` | controller | forwarded to player | player | ❌ No handler |
| `prev` | controller | forwarded to player | player | ❌ No handler |
| `seek` | controller | forwarded to player | player | ❌ No handler |
| `set_queue` | controller | forwarded to player | player | ❌ No handler |
| `transfer` | any client | intercepted by hub → handleTransfer() | — | ✅ Hub processes |
| `state` | player | broadcast to ALL clients | all | ❌ Controllers: isWsPlayerRef=false return; Player: type mismatch in handler |
| `devices` | hub (auto) | sent to ALL clients | all | ✅ DevicePicker handler |
| `promoted` | hub→target | direct send (not broadcast) | new player | ❌ No handler |
| `demoted` | hub→old player | direct send (not broadcast) | old player | ❌ No handler |
| `error` | hub→requester | direct send (not broadcast) | requester | ❌ No handler |
| `register` | (defined, never used) | — | — | — |

---

## 8. Root Cause Summary

### Break Point A: Role Assignment (playerws.ts line 55-57)
**Every device self-assigns "player" on its first load because sessionStorage is per-browser.**
The hub's demotion logic partially mitigates this but demotes the WRONG device — the actual audio-output host gets demoted when a phone connects.

### Break Point B: Message Dispatch (playerws.ts lines 87-98)
**Command messages (play, pause, next, prev, seek) are never delivered to any handler.**
The `stateHandlers` array only fires for `type==="state"` messages. The `handlers` array is never populated. No `onMessage` API exists. Commands fall into a void.

### Break Point C: Transfer Lifecycle (PlayerContext.tsx lines 390-412 + playerws.ts)
**The promoted/demoted message types have no frontend handlers.** Even though the hub correctly reassigns roles and sends continuity state, the frontend never processes these messages. The new player doesn't know it's been promoted. The old player doesn't know it's been demoted.

### Break Point D: "host" Magic String (DevicePicker.tsx lines 69-72, 116, 128-140)
**"host" has no mapping to real WebSocket device IDs.** Clicking it sets playerActive and reloads the page, which makes the device a controller (opposite of intent). The hardcoded "Host Computer" button is disconnected from the actual WebSocket device registry.

### Break Point E: State Visibility (PlayerContext.tsx lines 391, 427-442)
**Controllers never see playback state** — the `isWsPlayerRef` guard returns immediately. Even the single broadcast-on-mount snapshot is invisible. The `broadcastState` function only fires once.

---

## 9. Fix Requirements

To make remote control work end-to-end, ALL of these must be addressed:

1. **playerws.ts: Add generic message handler API** — expose `onMessage()` that pushes to `handlers` array, OR rework dispatch so command types ("play", "pause", etc.) are routed to a command handler alongside "state".

2. **PlayerContext.tsx: Wire command handler to playback controls** — when a "play" message arrives, call `playFn([track], 0)`. When "pause", call `a.pause()`. Etc. The handler must be registered for the CORRECT message types.

3. **PlayerContext.tsx: Handle "promoted" and "demoted" messages** — `promoted`: set `isWsPlayerRef=true`, load last-known track at position, begin broadcasting state. `demoted`: pause audio, set `isWsPlayerRef=false`, optionally reconnect as controller.

4. **playerws.ts: Fix role assignment** — sessionStorage-based role doesn't work cross-device. Need a proper handshake: connect as "controller" by default, then the hub assigns which device is "player." Or use a flag in localStorage that persists across tabs (but this breaks multi-device too). Best approach: let the hub be the authority — always connect as "controller" initially, hub promotes based on rules.

5. **DevicePicker.tsx: Replace "host" magic string** — use the actual WebSocket deviceID. Remove the sessionStorage+reload transfer path. Use `ws.transfer(actualDeviceID)` for all non-Spotify transfers. Remove the hardcoded "Host Computer" button or map it to the real device ID.

6. **PlayerContext.tsx: Continuous state broadcast** — broadcast state on every meaningful change (track change, play/pause, seek complete). Use a `useEffect` that watches the relevant state fields.

7. **PlayerContext.tsx: Allow controllers to process state** — separate the `isWsPlayerRef` guard: controllers should display state (what's playing, position) even if they don't execute commands. Only command execution should be gated.

8. **DevicePicker.tsx: Sync activeDevice with hub state** — listen to device list updates and set `activeDevice` to the device with `active: true`.

9. **Add error message handling** — both hub error messages and transfer failures should surface in the UI via toast notifications.

10. **DevicePicker.tsx: Handle edge case where a non-player sends transfer** — `handleTransfer` accepts transfers from any client. Consider restricting to controllers only, or requiring the source to be an authorized controller.
