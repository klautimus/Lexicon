# Lexicon Remote Control — Comprehensive Implementation Plan

**Author:** Atlas (Synthesizer — T3)
**Date:** 2026-05-21
**Status:** Ready for implementation
**Depends on:** T1 (signal flow map), T2 (identity audit) — synthesized from source code

---

## Root Cause Summary

The remote control "transfer" feature has four broken layers:

| Layer | What's broken | Impact |
|-------|--------------|--------|
| **Hub (hub.go)** | `MsgTransfer` constant exists but has zero handling logic. Transfer messages from controllers are forwarded to the current player, which just pauses audio. No role reassignment, no validation. | Phone→host transfer completely fails. |
| **Device Picker (DevicePicker.tsx)** | "host" is a magic string with no mapping to real WebSocket device IDs. The "transfer to host" code uses `sessionStorage` + `window.location.reload()` — a nuclear page-reload hack. | Cannot transfer to the host computer without reloading the app. |
| **Player Context (PlayerContext.tsx)** | The transfer handler only pauses audio and sets `isWsPlayerRef.current = false`. No queue/state forwarding. No promoted/demoted handlers. `broadcastState()` hardcodes `device: "host"` instead of the real device ID. | Transferred playback is dead — new player has no queue. Controllers receive state messages from a phantom "host" device. |
| **Role Assignment (playerws.ts)** | `sessionStorage` is **per-tab**, not shared — every new browser tab claims to be the player. The backend demotes the previous player, but the demoted tab's `isWsPlayerRef` stays `true`, causing it to keep broadcasting state and accepting commands. | Multiple tabs fight over player role. State broadcasts from demoted tabs confuse controllers. |

---

## 1. Hub Role Reassignment (`backend/internal/playerws/hub.go`)

### 1.1 Message Flow Today

```
Controller (phone)                      Hub                        Player (host)
      │                                  │                              │
      │── {type:"transfer", target: X}──▶│                              │
      │                                  │── readPump: controller msg ──│
      │                                  │   forwards to player ───────▶│
      │                                  │                              │── PlayerContext receives
      │                                  │                              │   "transfer" → pauses audio
      │                                  │                              │   sets isWsPlayerRef=false
      │                                  │                              │   NOTHING ELSE
```

**The hub never intercepts the transfer.** The message passes straight through to the player, which can't do anything useful with it.

### 1.2 Required New Message Types

Add to the `const` block (after line 37):

```go
MsgPromoted  = "promoted"   // Sent to device that becomes the new player
MsgDemoted   = "demoted"    // Sent to the old player, telling it to stop
MsgTransferFailed = "transfer_failed"  // Sent back to controller when target not found
```

Add to the `Message` struct (after line 51):

```go
Queue     json.RawMessage `json:"queue,omitempty"`      // Forwarded queue during transfer
QueueIdx  int             `json:"queue_index,omitempty"` // Current index in queue
Reason    string          `json:"reason,omitempty"`      // Error reason for transfer_failed
```

### 1.3 New Hub Method: `handleTransfer()`

Add after `broadcastDevices()` (around line 164). This is the core fix.

```go
// handleTransfer reassigns the player role from the current player to the target device.
// It validates the target exists, demotes the old player, promotes the new one,
// and forwards the queue/state for seamless continuation.
func (h *Hub) handleTransfer(fromClient *Client, msg Message) {
    targetID := msg.Target

    h.mu.Lock()
    defer h.mu.Unlock()

    // 1. Validate target exists
    target, ok := h.clients[targetID]
    if !ok {
        // Target disconnected — notify sender
        errMsg, _ := json.Marshal(Message{
            Type:   MsgTransferFailed,
            Target: targetID,
            Reason: "device not connected",
        })
        select {
        case fromClient.send <- errMsg:
        default:
        }
        return
    }

    // 2. Validate target is not already the player (self-transfer)
    if target.role == "player" {
        errMsg, _ := json.Marshal(Message{
            Type:   MsgTransferFailed,
            Target: targetID,
            Reason: "target is already the player",
        })
        select {
        case fromClient.send <- errMsg:
        default:
        }
        return
    }

    // 3. Demote current player
    var oldPlayerID string
    var oldPlayer *Client
    for id, c := range h.clients {
        if c.role == "player" {
            c.role = "controller"
            oldPlayerID = id
            oldPlayer = c
            break
        }
    }

    // 4. Promote target to player
    target.role = "player"

    // 5. Send promoted message to new player (with queue/state forwarding)
    promotedMsg, _ := json.Marshal(Message{
        Type:     MsgPromoted,
        Queue:    msg.Queue,
        QueueIdx: msg.QueueIdx,
        Track:    msg.Track,
        Position: msg.Position,
        Playing:  msg.Playing,
    })
    select {
    case target.send <- promotedMsg:
    default:
    }

    // 6. Send demoted message to old player (stop audio)
    if oldPlayer != nil {
        demotedMsg, _ := json.Marshal(Message{
            Type: MsgDemoted,
        })
        select {
        case oldPlayer.send <- demotedMsg:
        default:
        }
    }

    // 7. Broadcast updated device list
    h.broadcastDevices()

    log.Printf("[playerws] transfer: %s → %s (player role reassigned)", oldPlayerID, targetID)
}
```

### 1.4 Modify `readPump()` — Intercept Transfer Messages

In `readPump()` (lines 228-245), add a transfer interception BEFORE the controller-forwarding logic. The transfer message must be handled by the hub, not forwarded to the player.

Replace lines 228-245:

```go
    // Transfer messages are handled by the hub, not forwarded
    if msg.Type == MsgTransfer {
        c.hub.handleTransfer(c, msg)
        continue
    }

    // Only controllers can send commands; players send state updates
    if c.role == "controller" {
        // Forward command to the player
        c.hub.mu.RLock()
        for _, player := range c.hub.clients {
            if player.role == "player" {
                select {
                case player.send <- data:
                default:
                }
                break
            }
        }
        c.hub.mu.RUnlock()
    } else if c.role == "player" && msg.Type == MsgState {
        // Player broadcasting state — forward to all controllers
        c.hub.broadcast <- data
    }
```

### 1.5 Modify `register` — Send Proper Promoted/Demoted Messages

Replace lines 99-105 in the `register` case:

```go
        case client := <-h.register:
            h.mu.Lock()
            // If a new player registers, demote the old one with proper notification
            if client.role == "player" {
                for id, c := range h.clients {
                    if c.role == "player" && id != client.deviceID {
                        c.role = "controller"
                        // Notify old player of demotion
                        demotedMsg, _ := json.Marshal(Message{Type: MsgDemoted})
                        select {
                        case c.send <- demotedMsg:
                        default:
                        }
                    }
                }
                // Notify new player of promotion
                promotedMsg, _ := json.Marshal(Message{Type: MsgPromoted})
                select {
                case client.send <- promotedMsg: // sent after client is added to clients map
                default:
                }
            }
            h.clients[client.deviceID] = client
            h.mu.Unlock()
```

Note: the promoted message to `client` must be sent AFTER `client` is added to `h.clients`, because the send channel isn't being read until `writePump()` starts (which happens in the goroutine spawned in `ServeHTTP`). The `client.send` channel has buffer 256 — this single message fits fine.

Actually, simpler approach: send the promoted message AFTER adding to map:

```go
        case client := <-h.register:
            h.mu.Lock()
            if client.role == "player" {
                for id, c := range h.clients {
                    if c.role == "player" && id != client.deviceID {
                        c.role = "controller"
                        demotedMsg, _ := json.Marshal(Message{Type: MsgDemoted})
                        select {
                        case c.send <- demotedMsg:
                        default:
                        }
                    }
                }
            }
            h.clients[client.deviceID] = client
            h.mu.Unlock()
            // Notify new player AFTER adding to map and releasing lock
            if client.role == "player" {
                promotedMsg, _ := json.Marshal(Message{Type: MsgPromoted})
                select {
                case client.send <- promotedMsg:
                default:
                }
            }
            log.Printf(...)
            h.broadcastDevices()
```

### 1.6 Files Changed

| File | Lines changed | What |
|------|--------------|------|
| `hub.go` | ~+80 | New `handleTransfer()`, modified `readPump()`, modified `register`, new message types, new Message fields |

---

## 2. Device Picker Refactor (`frontend/src/components/DevicePicker.tsx`)

### 2.1 The "host" Problem

The DevicePicker has five hardcoded references to the magic string `"host"`:

| Line | Usage | Problem |
|------|-------|---------|
| 17 | `useState<string>("host")` | Default active device is a phantom ID |
| 69 | `device.id === "host"` | Branch for sessionStorage+reload hack |
| 128 | Button `id: "host"` | Hardcoded button that doesn't map to any real device |
| 139 | `activeDevice === "host"` | Check mark display |
| 144 | `.filter((d) => d.id !== "host")` | Excludes the magic string from real devices |

**Root issue:** There's no mapping from "the host computer" to a real WebSocket device ID. The host computer has a real device ID (e.g., `dev-a1b2c3d4`) stored in `localStorage.playerDeviceID`.

### 2.2 Required Changes

#### Step 2.2.1: Detect the local device ID

Add a `localDeviceID` constant derived from the WebSocket singleton:

```tsx
const ws = getPlayerWebSocket();
const localDeviceID = ws.getDeviceID(); // "dev-a1b2c3d4"
```

#### Step 2.2.2: Map "host" button to the real device

Replace the "Host Computer" button (lines 128-140) to use the REAL device ID from the device list instead of the "host" magic string.

The device list from the WebSocket broadcasts contains the host computer's entry with its real ID. We need to find and promote it:

```tsx
// Find the host computer entry in the WS device list
const hostDevice = devices.find((d) => d.id === localDeviceID);

{hostDevice && (
  <button
    onClick={() => handleTransfer(hostDevice)}
    className="w-full flex items-center gap-3 px-3 py-2.5 hover:bg-panel2 transition-colors text-left"
  >
    <Monitor size={14} className={activeDevice === localDeviceID ? "text-accent" : "text-muted"} />
    <div className="flex-1 min-w-0">
      <p className="text-sm font-medium truncate">Host Computer</p>
      <p className="text-xs text-muted">
        {currentTrack ? `Playing: ${currentTrack.title}` : "Control host playback"}
      </p>
    </div>
    {activeDevice === localDeviceID && <Check size={14} className="text-accent" />}
  </button>
)}
```

#### Step 2.2.3: Replace sessionStorage+reload hack with WebSocket transfer

Replace lines 69-72:

```tsx
// OLD (sessionStorage + reload):
} else if (device.id === "host") {
  sessionStorage.setItem("playerActive", "1");
  window.location.reload();

// NEW (WebSocket transfer):
} else if (device.id === localDeviceID) {
  // Transfer to host: send via WebSocket with current queue/state
  ws.transfer(device.id);
```

#### Step 2.2.4: Fix "This Device" (self) handler

The "self" button (lines 115-125) currently does nothing meaningful. When the user clicks "This Device", we need to promote THIS browser to player. This requires the WebSocket to support a self-promotion:

```tsx
// "This Device" becomes the player
onClick={() => {
  ws.transfer(localDeviceID); // transfer to self = become player
  setActiveDevice(localDeviceID);
  setOpen(false);
}}
```

But this needs hub.go to handle self-transfer. See edge cases (section 5.4).

#### Step 2.2.5: Fix activeDevice default

Replace line 17:

```tsx
// OLD:
const [activeDevice, setActiveDevice] = useState<string>("host");

// NEW — use the local device ID:
const [activeDevice, setActiveDevice] = useState<string>(localDeviceID);
```

#### Step 2.2.6: Derive activeName/activeType from real device data

Replace lines 80-81 to look up the local device properly:

```tsx
const activeDeviceObj = devices.find((d) => d.id === activeDevice);
const activeName = activeDeviceObj?.name || "This Device";
const activeType = activeDeviceObj?.type || (ws.isPlayer() ? "player" : "controller");
```

#### Step 2.2.7: Remove "host" filter

Replace line 144 — the filter should no longer exclude "host" since we're not adding it:

```tsx
// OLD:
.filter((d) => d.id !== "host" && d.type !== "spotify")

// NEW:
.filter((d) => d.type !== "spotify")
```

#### Step 2.2.8: Sync `activeDevice` with hub's real state

Currently `activeDevice` is set only when the user clicks. It drifts from reality when the backend demotes/promotes devices. Add a sync effect that watches the `devices` messages and updates `activeDevice` to match the actual active player:

```tsx
// In the devices message handler (after setDevices):
const active = wsDevices.find((d: Device) => d.active);
if (active && active.id !== activeDevice) {
    setActiveDevice(active.id);
}
```

This fixes the "activeDevice drifts from reality" bug (identity audit issue 7.6).

#### Step 2.2.9: Cross-tab role stealing mitigation

The `sessionStorage` per-tab isolation means every new tab claims to be the player. While the backend's single-player enforcement prevents audio chaos, the frontend needs to accept the backend's authority. In PlayerContext, when the device list shows this client as `active: false` but `isWsPlayerRef` is `true`, demote ourselves:

```typescript
// In PlayerContext, inside the devices message handler:
const me = msg.list?.find((d: any) => d.id === ws.getDeviceID());
if (me && !me.active && isWsPlayerRef.current) {
    // Backend says we're not the player — accept it
    const a = audioRef.current;
    if (a) a.pause();
    isWsPlayerRef.current = false;
    sessionStorage.removeItem("playerActive");
}
```

This is a belt-and-suspenders fix alongside the explicit promoted/demoted messages.

### 2.3 Files Changed

| File | Lines changed | What |
|------|--------------|------|
| `DevicePicker.tsx` | ~30 modified, ~5 removed | Map host→real device ID, replace reload with WS transfer, fix activeDevice, sync with hub state |

---

## 3. Player Context Handlers (`frontend/src/player/PlayerContext.tsx`)

### 3.1 Current State

The player context has ONE handler for transfer (lines 408-412):

```typescript
} else if (msg.type === "transfer") {
    const a = audioRef.current;
    if (a) a.pause();
    isWsPlayerRef.current = false;
}
```

This:
- Pauses audio (good)
- Sets `isWsPlayerRef.current = false` (good — stops accepting remote commands)
- Does NOT forward queue/state to new player (BAD)
- Does NOT clean up audio context (minor)

### 3.2 Required Changes

#### Step 3.2.1: Add `promoted` handler

When a device receives the `promoted` message (it's now the player), it must:
1. Set `isWsPlayerRef.current = true`
2. If queue data was forwarded, load it and start playing from the forwarded position

```typescript
} else if (msg.type === "promoted") {
    // This device is now the player
    isWsPlayerRef.current = true;

    // If queue/state was forwarded, resume playback
    if (msg.queue && Array.isArray(msg.queue) && msg.queue.length > 0) {
        const tracks = msg.queue as Track[];
        const idx = msg.queue_index ?? 0;
        playFn(tracks, idx).then(() => {
            // Seek to forwarded position after track loads
            if (msg.position && msg.position > 0) {
                setTimeout(() => {
                    const a = audioRef.current;
                    if (a) a.currentTime = msg.position;
                }, 500); // small delay for audio element to be ready
            }
            // If the old player was paused, pause here too
            if (msg.playing === false) {
                setTimeout(() => {
                    const a = audioRef.current;
                    if (a) a.pause();
                }, 600);
            }
        });
    }
}
```

#### Step 3.2.2: Add `demoted` handler

When demoted, stop audio and become a controller:

```typescript
} else if (msg.type === "demoted") {
    // This device is no longer the player
    const a = audioRef.current;
    if (a) a.pause();
    isWsPlayerRef.current = false;
    flushLocalPlay(false);
    // Clear the audio source to free resources
    if (a) {
        a.removeAttribute("src");
        a.load();
    }
    setState((s) => ({ ...s, playing: false, current: null, queue: [], index: -1 }));
}
```

#### Step 3.2.3: Modify the `transfer` handler — forward queue/state

The old player receiving `transfer` should forward its current state before demoting:

```typescript
} else if (msg.type === "transfer") {
    // Forward current queue/state to the hub before pausing
    const s = stateRef.current;
    ws.send({
        type: "transfer",
        target: msg.target, // pass through the target
        queue: s.queue,
        queue_index: s.index,
        track: s.current,
        position: s.position,
        playing: s.playing,
    });
    // Now pause and demote
    const a = audioRef.current;
    if (a) a.pause();
    isWsPlayerRef.current = false;
}
```

Wait — this creates a loop. The controller sends `{type: "transfer", target: X}`. This goes to hub, which forwards to player. The player's handler fires, which sends ANOTHER transfer message with queue data. We need the hub to intercept the FIRST transfer, not the response.

**Correct flow:**

1. Controller sends `{type: "transfer", target: X}` to hub
2. Hub intercepts (section 1.4), does NOT forward to player yet
3. Hub asks current player for state: sends `{type: "prepare_transfer"}` to player
4. Player responds with queue/state: sends `{type: "transfer_state", queue: [...], ...}` 
5. Hub receives this, then promotes target + demotes old player + forwards state to target

Actually, this adds complexity. Simpler approach: have the hub send a special message to the old player to extract state, then handle the rest.

**Simplest correct approach:** The hub intercepts the transfer. It gathers state from the old player's client struct (which we add queue tracking to), then does the promotion.

But the hub doesn't currently track queue state — that's all in the frontend's React state.

**Practical approach — two-message protocol:**

1. Controller sends `{type: "transfer", target: X}` 
2. Hub intercepts it
3. Hub sends `{type: "transfer_request"}` to the current player (asking for state)
4. Player receives this, replies with `{type: "transfer_state", queue: [...], queue_index: N, track: {...}, position: P, playing: B, target: X}`
5. Hub receives `transfer_state`, calls `handleTransfer()` (now refactored to accept the state data)
6. Hub promotes target, demotes old player, forwards state

OR even simpler: modify the transfer handler in PlayerContext to forward state BEFORE the hub intercepts. The hub sees the transfer come through, extracts the queue data from it, and does the promotion.

**Actually the cleanest approach:** The controller sends `{type: "transfer", target: X}`. The hub intercepts it. Rather than forwarding to the player, the hub sends `{type: "request_state"}` to the player. The player responds with its current state as a `{type: "transfer_state", ...}` message. The hub then calls `handleTransfer()` with this state.

But wait — the player's WS `send()` goes through the hub's readPump. So the player sends a message of type `transfer_state` which arrives at readPump. We need the hub to handle `transfer_state` messages specially.

Let me revise. Here's the simpler protocol:

**Protocol:**

New message types: `transfer_request`, `transfer_state`

```
Controller → Hub:   {type:"transfer", target:"dev-xyz"}
Hub → Old Player:   {type:"transfer_request", target:"dev-xyz"}
Old Player → Hub:   {type:"transfer_state", target:"dev-xyz", queue:[...], queue_index:N, track:{...}, position:P, playing:B}
Hub → New Player:   {type:"promoted", queue:[...], queue_index:N, track:{...}, position:P, playing:B}
Hub → Old Player:   {type:"demoted"}
Hub → All:          {type:"devices", list:[...]}
```

The hub handles `transfer_state` in readPump by calling `handleTransfer()`.

#### Step 3.2.4: Handle `transfer_request` in PlayerContext

```typescript
} else if (msg.type === "transfer_request") {
    // Hub is asking for our current state before transfer
    const s = stateRef.current;
    ws.send({
        type: "transfer_state",
        target: msg.target,
        queue: s.queue,
        queue_index: s.index,
        track: s.current,
        position: s.position,
        playing: s.playing,
    });
}
```

#### Step 3.2.5: Handle `transfer_state` in hub.go readPump

In readPump, add:

```go
    // Transfer state response from player — complete the transfer
    if msg.Type == MsgTransferState {
        c.hub.handleTransfer(c, msg)
        continue
    }
```

And update `handleTransfer` to use the queue data from the message (already structured for this in section 1.3).

#### Step 3.2.6: Add `transfer_failed` handler

```typescript
} else if (msg.type === "transfer_failed") {
    toast.error(`Transfer failed: ${msg.reason || "unknown error"}`);
}
```

### 3.3 Updated `onState` handler (complete)

The full updated handler in the `useEffect` (lines 390-412, expanded):

```typescript
ws.onState((msg: any) => {
    if (msg.type === "play" && msg.track_id) {
        if (!isWsPlayerRef.current) return;
        api.track(msg.track_id).then((t) => {
            playFn([t], 0);
        }).catch(() => {});
    } else if (msg.type === "pause") {
        if (!isWsPlayerRef.current) return;
        const a = audioRef.current;
        if (a) a.pause();
    } else if (msg.type === "resume") {
        if (!isWsPlayerRef.current) return;
        const a = audioRef.current;
        if (a) a.play().catch(() => {});
    } else if (msg.type === "next") {
        if (!isWsPlayerRef.current) return;
        next();
    } else if (msg.type === "prev") {
        if (!isWsPlayerRef.current) return;
        prev();
    } else if (msg.type === "seek" && msg.position !== undefined) {
        if (!isWsPlayerRef.current) return;
        seekFn(msg.position);
    } else if (msg.type === "transfer_request") {
        // Hub is asking for our state before transfer
        const s = stateRef.current;
        ws.send({
            type: "transfer_state",
            target: msg.target,
            queue: s.queue,
            queue_index: s.index,
            track: s.current,
            position: s.position,
            playing: s.playing,
        });
    } else if (msg.type === "demoted") {
        // This device is no longer the player
        const a = audioRef.current;
        if (a) a.pause();
        isWsPlayerRef.current = false;
        flushLocalPlay(false);
        if (a) {
            a.removeAttribute("src");
            a.load();
        }
        setState((s) => ({ ...s, playing: false, current: null, queue: [], index: -1 }));
    } else if (msg.type === "promoted") {
        // This device is now the player
        isWsPlayerRef.current = true;
        if (msg.queue && Array.isArray(msg.queue) && msg.queue.length > 0) {
            const tracks = msg.queue as Track[];
            const idx = msg.queue_index ?? 0;
            playFn(tracks, idx).then(() => {
                if (msg.position && msg.position > 0) {
                    setTimeout(() => {
                        const a = audioRef.current;
                        if (a) a.currentTime = msg.position;
                    }, 500);
                }
                if (msg.playing === false) {
                    setTimeout(() => {
                        const a = audioRef.current;
                        if (a) a.pause();
                    }, 600);
                }
            });
        }
    } else if (msg.type === "transfer_failed") {
        toast.error(`Transfer failed: ${msg.reason || "unknown error"}`);
    }
});
```

### 3.4 Export `setPodcastEpisodeId` in PlayerCtx interface

Already exported — no changes needed.

### 3.5 Fix `broadcastState` — Stop Using "host" Magic String

In `broadcastState()` (lines 427-438), replace the hardcoded `device: "host"` with the actual WebSocket device ID:

```typescript
const broadcastState = useCallback(() => {
    if (!isWsPlayerRef.current || !wsRef.current) return;
    const s = stateRef.current;
    wsRef.current.send({
        type: "state",
        playing: s.playing,
        track: s.current,
        position: s.position,
        duration: s.duration,
        device: wsRef.current.getDeviceID(), // was: "host"
    });
}, []);
```

This ensures controllers can correlate state messages with device list entries. Currently state broadcasts say `device: "host"` but no device in the list has that ID.

### 3.6 Files Changed

| File | Lines changed | What |
|------|--------------|------|
| `PlayerContext.tsx` | ~+60 | promoted/demoted handlers, transfer_request responder, transfer_failed toast |

---

## 4. Queue Continuity

### 4.1 Protocol

The full transfer protocol ensures queue continuity:

```
1. Controller detects user click on target device
2. Controller sends: {type:"transfer", target:"<target-device-id>"}
3. Hub intercepts transfer message in readPump
4. Hub validates target exists, is not already player
5. Hub sends to current player: {type:"transfer_request", target:"<target-id>"}
6. Current player responds: {type:"transfer_state", queue:[...], queue_index:N, track:{id,title,artist,...}, position:P, playing:B}
7. Hub receives transfer_state, calls handleTransfer():
   a. Demotes old player role → "controller"
   b. Promotes target role → "player"
   c. Sends target: {type:"promoted", queue:[...], queue_index:N, track:{...}, position:P, playing:B}
   d. Sends old player: {type:"demoted"}
   e. Broadcasts updated device list
8. New player (target) receives "promoted":
   a. Sets isWsPlayerRef = true
   b. Loads forwarded queue starting at queue_index
   c. Seeks to forwarded position
   d. Resumes or pauses per playing flag
9. Old player receives "demoted":
   a. Pauses audio
   b. Clears queue
   c. Sets isWsPlayerRef = false
```

### 4.2 Data Forwarded

| Field | Type | Purpose |
|-------|------|---------|
| `queue` | `Track[]` | Full queue array (typically 1-500 tracks) |
| `queue_index` | `number` | Current position in queue |
| `track` | `Track` | Currently playing track (full object with id, title, artist, etc.) |
| `position` | `number` | Current playback position in seconds |
| `playing` | `boolean` | Whether playback was active at time of transfer |

### 4.3 Limitations

- **Spotify tracks:** Cannot transfer local playback state for Spotify tracks. If the old player was playing a Spotify track, the new player receives the track info but must have its own Spotify SDK initialized.
- **Podcast position:** Podcast episode ID is NOT forwarded. The user must manually resume the podcast on the new device.
- **Volume:** Not forwarded — each device keeps its own volume setting.
- **Shuffle state:** The shuffled queue order is forwarded as-is, but `shuffled` flag is not forwarded.
- **Repeat mode:** Not forwarded — new device defaults to `"off"`.

---

## 5. Edge Cases

### 5.1 Disconnected Target

**Scenario:** Controller sends transfer to a device that disconnected between the device list broadcast and the transfer command.

**Handling:** `handleTransfer()` checks `h.clients[targetID]` — if not found, sends `{type:"transfer_failed", reason:"device not connected"}` back to the controller. The DevicePicker shows a toast error.

**Code location:** `hub.go` — `handleTransfer()` lines validating target (section 1.3).

### 5.2 No Current Player

**Scenario:** There is no active player (all devices are controllers). A transfer is requested.

**Handling:** In `handleTransfer()`, if no client with `role == "player"` is found, skip the demotion step and just promote the target. The old player state forwarding is skipped (there is no state to forward).

**Code location:** `hub.go` — `handleTransfer()` section where old player is looked up. If `oldPlayer == nil`, skip the demoted message and skip the `transfer_request` step. Go straight to promotion.

```go
// Find current player (may not exist)
var oldPlayer *Client
for _, c := range h.clients {
    if c.role == "player" {
        oldPlayer = c
        break
    }
}

if oldPlayer != nil {
    // Request state from old player, then complete transfer
    // (existing flow)
} else {
    // No current player — promote target directly with no queue
    target.role = "player"
    promotedMsg, _ := json.Marshal(Message{Type: MsgPromoted})
    select {
    case target.send <- promotedMsg:
    default:
    }
    h.broadcastDevices()
}
```

### 5.3 Multiple Simultaneous Transfers

**Scenario:** Two controllers send transfer commands at nearly the same time.

**Handling:** `handleTransfer()` holds `h.mu.Lock()` for its entire duration. The second transfer will be serialized after the first completes. By the time the second transfer runs, the target from the first transfer is now the player. The second transfer will find the correct current player and work correctly.

**Potential issue:** If Controller A transfers to Device X, and simultaneously Controller B transfers to Device Y, the winner is whoever's message arrives at the hub first. Both transfers will succeed sequentially — Device Y ends up as the final player. This is acceptable behavior (last write wins).

**Code location:** `hub.go` — `handleTransfer()` uses mutex for entire operation.

### 5.4 Self-Transfer ("This Device")

**Scenario:** User clicks "This Device" in the DevicePicker, which is already the player (or a controller wanting to become player).

**Handling — case A (already player):** `handleTransfer()` checks `target.role == "player"` and returns `{type:"transfer_failed", reason:"target is already the player"}`. The DevicePicker shows this as a toast. This is correct — no action needed.

**Handling — case B (controller → self):** The DevicePicker sends `{type:"transfer", target: localDeviceID}`. Hub processes it: demotes old player, promotes self, forwards queue. This works correctly — the current browser becomes the player and takes over playback.

**DevicePicker "This Device" button (line 116):** Should send `ws.transfer(localDeviceID)` instead of doing nothing meaningful.

### 5.5 Browser Tab with Old Role on Reload

**Scenario:** After a transfer demotes a device, the user reloads that device's page.

**Handling:** On reload, `sessionStorage` is cleared (it's session-scoped), so `playerActive` is gone. The first device to connect after reload becomes player again via the first-come-first-served logic in `PlayerWebSocket` constructor (lines 54-57 of playerws.ts).

**Mitigation:** The `playerActive` sessionStorage flag should be set/cleared in response to promoted/demoted messages, not just on initial load. On `promoted`: `sessionStorage.setItem("playerActive", "1")`. On `demoted`: `sessionStorage.removeItem("playerActive")`.

Add to promoted handler:
```typescript
sessionStorage.setItem("playerActive", "1");
```

Add to demoted handler:
```typescript
sessionStorage.removeItem("playerActive");
```

### 5.6 Network Partition / WS Disconnect During Transfer

**Scenario:** The old player disconnects between receiving `transfer_request` and sending `transfer_state`.

**Handling:** The hub should have a timeout. If `transfer_state` is not received within 5 seconds, the hub proceeds with the transfer without state forwarding (promote target with empty queue). Add a timeout mechanism:

```go
// After sending transfer_request, wait up to 5s for transfer_state response
// If timeout, proceed with transfer but without queue data
```

**Implementation:** Use a goroutine with `time.After`. If the transfer_state arrives, cancel the timeout. If the timeout fires, proceed with empty state.

This is a nice-to-have for robustness but can be added in a follow-up. The current implementation will hang waiting for transfer_state if the old player disconnects.

### 5.7 Very Large Queue (>few hundred tracks)

**Scenario:** User has a queue of 1000+ tracks. Forwarding this via WebSocket message.

**Handling:** WebSocket messages are limited to 4096 bytes (`c.conn.SetReadLimit(4096)` at hub.go line 207). A queue of 1000 tracks with full Track objects can easily exceed this.

**Fix:** Increase read limit for transfer_state messages, OR compress the queue data, OR only forward the current track + index (let the new player fetch the rest from the API).

**Recommended approach:** Only forward `queue_index` and `track` (the currently playing track). The new player can reconstruct the queue by:
1. If it was a playlist, re-fetch the playlist
2. If it was a library queue, reconstruct from the track context
3. As a fallback, just play the single forwarded track

```typescript
// In transfer_request handler — only forward essential state:
ws.send({
    type: "transfer_state",
    target: msg.target,
    queue_index: s.index,
    track: s.current,
    position: s.position,
    playing: s.playing,
});
```

And increase the WebSocket read limit in hub.go for this message type:

```go
// Before reading transfer_state, temporarily increase limit
// Or better: set a higher base limit. 4096 is too low for any realistic payload.
c.conn.SetReadLimit(65536) // 64KB — enough for moderate queues
```

### 5.8 Transfer with Spotify Track Playing

**Scenario:** Old player is playing a Spotify track via the Spotify SDK. Transfer is requested.

**Handling:** The `transfer_request` handler in PlayerContext sends the current track info. If `sourceRef.current === "spotify"`, the track has a `spotify_id`. The new player receives this track in the promoted message. In `loadAndPlay()`, it checks `t.spotify_id` and tries to play via Spotify SDK. If the new device doesn't have Spotify SDK initialized, it will fail with an error.

**Mitigation:** In the promoted handler, check if the forwarded track is a Spotify track. If so, and this device can't play Spotify, show a message: "This track requires Spotify. Starting local playback instead."

### 5.9 Transfer to "This Device" When It's Already Player

**Handling:** Already covered in 5.4 case A. Hub returns `transfer_failed` with reason "target is already the player". The DevicePicker can also short-circuit this client-side by checking `activeDevice === localDeviceID` before sending.

---

## 6. Implementation Order

### Phase 1: Backend Hub (hub.go) — Foundation

1. Add new message type constants (`MsgPromoted`, `MsgDemoted`, `MsgTransferFailed`, `MsgTransferRequest`, `MsgTransferState`)
2. Add new fields to `Message` struct (`Queue`, `QueueIdx`, `Reason`)
3. Write `handleTransfer()` method
4. Modify `readPump()` — intercept `transfer` messages, handle `transfer_state` messages
5. Modify `register` — send promoted/demoted messages on role change
6. Increase `SetReadLimit` from 4096 to 65536
7. Build: `cd backend && go build ./internal/...` to verify compilation

### Phase 2: Frontend PlayerWebSocket (playerws.ts) — Protocol Support

1. Add `transfer_request`, `transfer_state`, `promoted`, `demoted`, `transfer_failed` to message type awareness (no code changes needed — the type dispatch is in PlayerContext, not playerws.ts)
2. No changes needed to playerws.ts itself — it already passes all messages through

### Phase 3: Frontend PlayerContext (PlayerContext.tsx) — Role Handlers

1. Add `transfer_request` handler (responds with state)
2. Add `promoted` handler (takes over as player, loads queue)
3. Add `demoted` handler (stops audio, clears state)
4. Add `transfer_failed` handler (shows error toast)
5. Add sessionStorage management in promoted/demoted
6. Build: `cd frontend && npm run build` (on Windows PowerShell)

### Phase 4: Frontend DevicePicker (DevicePicker.tsx) — UI Fix

1. Get `localDeviceID` from WebSocket singleton
2. Replace "host" magic string with localDeviceID
3. Replace sessionStorage+reload with WebSocket transfer
4. Fix "This Device" handler to use WebSocket transfer
5. Fix activeDevice default
6. Remove "host" filter from device list
7. Build: `cd frontend && npm run build` (on Windows PowerShell)

### Phase 5: Integration Testing

1. Start backend + frontend
2. Open two browser tabs (simulate host + phone)
3. Verify device list shows both devices
4. Play music on one device (player)
5. Transfer to the other device
6. Verify: playback stops on old device, starts on new device at same position
7. Verify: device list updates (active indicator moves)
8. Verify: transfer from phone back to host
9. Test edge cases: disconnect target, self-transfer
10. Test with Spotify track

### Phase 6: Follow-up (Optional)

- Add transfer timeout (5s wait for transfer_state, then proceed without state)
- Only forward queue_index + current track (not full queue) to avoid message size issues
- Forward shuffle/repeat mode
- Forward podcast episode context
- Handle Spotify track transfer gracefully

---

## 7. Complete File Change Summary

| File | Changes | Risk |
|------|---------|------|
| `backend/internal/playerws/hub.go` | +~100 lines, modified readPump, new handleTransfer, modified register | Medium — core routing logic |
| `frontend/src/player/PlayerContext.tsx` | +~90 lines, modified onState handler, fixed broadcastState, added cross-tab demotion sync | Medium — state management |
| `frontend/src/components/DevicePicker.tsx` | ~30 lines modified, ~5 removed, added activeDevice sync | Low — UI-only changes |
| `frontend/src/lib/playerws.ts` | 0 lines (protocol passes through) | None |
| `frontend/src/components/MobilePlayerBar.tsx` | 0 lines (reuses DevicePicker) | None |

---

## 8. Verification Checklist

After implementation, verify each scenario:

- [ ] **Phone → Host transfer:** Phone controller clicks "Host Computer" → host starts playing same track at same position → phone stops
- [ ] **Host → Phone transfer:** Host clicks phone device → phone starts playing → host stops
- [ ] **Queue continuity:** Transfer mid-queue → new device plays same queue position → can skip to next track
- [ ] **Paused transfer:** Transfer while paused → new device loads track paused at correct position
- [ ] **Device list update:** After transfer, both devices show correct active indicator
- [ ] **Disconnected target:** Shows error toast, playback continues on current player
- [ ] **Self-transfer (This Device):** Shows "already the player" or works if controller
- [ ] **Multiple transfers:** Rapid back-and-forth transfers work correctly
- [ ] **Page reload after demotion:** Reloaded device doesn't steal player role (sessionStorage cleared)
- [ ] **Spotify track transfer:** Graceful handling if new device can't play Spotify
- [ ] **No player exists:** Transfer promotes target with empty queue, works correctly
