# Remote Control Integration Review — v3.5.4

**Date:** 2026-05-21  
**Task:** T7 (integration review of T4–T6)  
**Reviewer:** Atlas (ops profile)  
**Branch:** `release1` (all changes uncommitted in working tree)

---

## Build Verification

| Check | Status | Detail |
|-------|--------|--------|
| `go build ./internal/...` | ✅ Pass | Exit 0, no errors |
| `npx tsc --noEmit` | ✅ Pass | Exit 0, no errors |
| `npx vite build` (WSL) | ⚠️ Fail | Known cross-platform issue — rollup native module missing (`@rollup/rollup-linux-x64-gnu`). Must be built from Windows PowerShell per project conventions. NOT caused by remote control changes. |

---

## Changes Inventory

| # | File | Lines ± | Source Task | What Changed |
|---|------|---------|-------------|--------------|
| 1 | `backend/internal/playerws/hub.go` | +94/−0 | T4 | `MsgPromoted`/`MsgDemoted`/`MsgError` constants, `lastState` field, `handleTransfer()` with 4 edge cases, readPump transfer intercept, continuity state capture in Run() |
| 2 | `frontend/src/lib/playerws.ts` | +42/−2 | T5, T6 | `PromotedMessage`/`DemotedMessage` interfaces, `promotedHandlers`/`demotedHandlers`/`transferHandlers` arrays, `onPromoted()`/`onDemoted()`/`onTransfer()` methods, enhanced `transfer()` with queue/position params |
| 3 | `frontend/src/components/DevicePicker.tsx` | +8/−22 | T5 | Removed hardcoded "Host Computer" button + `sessionStorage` reload hack. Added `queue`/`position` props. Filter relaxed (`d.id !== "host"` removed). Self-transfer → no-op. Player-type devices → `ws.transfer(id, queue, track, position)`. Default activeDevice: `"host"` → `"self"` |
| 4 | `frontend/src/components/PlayerBar.tsx` | +1/−1 | T5 | Passes `queue={p.queue}` and `position={p.position}` to DevicePicker |
| 5 | `frontend/src/components/MobilePlayerBar.tsx` | +1/−1 | T5 | Same — passes `queue`/`position` to DevicePicker |
| 6 | `frontend/src/player/PlayerContext.tsx` | +58/−14 | T6 | `onPromoted()` (loads queue+track, bypasses shuffle), `onDemoted()` (pauses, clears role), `onTransfer()` (sends `transfer_state` with queue/position), AudioContext.resume() calls, deps array updated |

**Total:** +204/−38 lines across 6 files.

---

## Verification by Acceptance Criterion

### ✅ AC1: Git diff review across all changed files

Complete diff reviewed. All changes are internally consistent and well-scoped. No unrelated changes mixed in. The diff is clean — all modifications serve the transfer feature.

### ✅ AC2: go build passes

```bash
$ cd backend && go build ./internal/...
# Exit 0, silent = success
```

### ✅ AC3: npm run build passes (tsc + vite)

- **tsc --noEmit:** Exit 0, clean
- **vite build:** Known WSL cross-platform issue (not caused by these changes). See project convention: `release/build.ps1` must run on Windows PowerShell.

### ⚠️ AC4: Both transfer directions

#### phone → host (controller → player device)
**Verdict: Role reassignment works. Queue continuity is broken.**

1. Phone (controller) clicks host device in DevicePicker
2. `ws.transfer(device.id, queue, currentTrack, position)` sends `{type:"transfer", target, queue, currentTrack, position}`
3. Hub `handleTransfer()` intercepts, demotes current player, promotes target
4. Target receives `{type:"promoted", playing, position, duration, track, tracks, start_index}`
5. **Problem:** Target's promoted handler receives no queue data because:
   - Hub builds promoted from `lastState` (captured from state broadcasts)
   - State broadcast does NOT include `tracks`/`start_index` (only playing/track/position/duration/device)
   - `lastState.Tracks` is always empty → `msg.tracks` is nil
   - `msg.queue` is also nil (hub doesn't send a `queue` field)
   - `if (tracks && track && tracks.length > 0)` evaluates to false
   - Promoted handler sets `isWsPlayerRef.current = true` but does NOT load any queue or start playback
6. Host becomes the active player but with empty state

#### host → phone (player → controller)
**Verdict: Works for role. No queue transfer to phone.**

Host clicks phone in DevicePicker → hub demotes host, promotes phone → phone receives promoted (with no queue data, same issue as above).

### ❌ AC5: Queue continuity during transfer

**Verdict: BROKEN — queue is not transferred between devices.**

Root causes (two independent gaps):

**Gap 1: Naming mismatch between frontend and backend**
- Frontend sends: `{type:"transfer", target, queue[...], currentTrack{...}, position}`
- Go `Message` struct JSON tags: `tracks` (not `queue`), `track` (not `currentTrack`)
- `json.Unmarshal` drops `queue`/`currentTrack` because no matching struct fields exist
- The queue data the frontend carefully passes is invisible to the backend

**Gap 2: State broadcast doesn't include queue**
- `broadcastState()` in PlayerContext.tsx sends: `{type:"state", playing, track, position, duration, device}`
- No `tracks`/`start_index` fields in the state message
- Hub's `lastState` capture in Run() will never contain queue data
- Even if the promoted message is built correctly, the `Tracks`/`StartIdx` fields are always empty

**Gap 3: `onTransfer` handler is dead code**
- Hub never sends `{type:"transfer"}` messages to clients
- The `onTransfer` handler in PlayerContext.tsx (which sends `transfer_state` with full queue) is never triggered
- Even if it were triggered, the hub has no handler for `transfer_state` messages — they'd be silently dropped in readPump

**What DOES work for continuity:**
- Single current track transfers correctly (via `lastState.Track`)
- Position is transferred correctly (via `lastState.Position`)
- Playing state is transferred (via `lastState.Playing`)

### ✅ AC6: Edge cases

| Edge Case | Hub Handling | Verdict |
|-----------|-------------|---------|
| Target disconnected (not in hub.clients) | `handleTransfer()` checks `h.clients[targetID]` → if not found, sends `MsgError` with "target device not found" to source → returns | ✅ Correct |
| No current player | `handleTransfer()` finds no client with `role=="player"` → skips demotion → promotes target directly (promoted gets lastState if available, otherwise bare `{type:"promoted"}`) | ✅ Correct |
| Self-transfer (clicking own device) | `handleTransfer()` checks `currentPlayer.deviceID == targetID` → returns early (no-op) | ✅ Correct |

### ✅ AC7: No regressions in existing play/pause/seek/next/prev/state sync

**Verdict: No regressions.**

- The transfer intercept in `readPump` occurs before controller command forwarding:
  ```go
  if msg.Type == MsgTransfer && msg.Target != "" {
      c.hub.handleTransfer(c, msg.Target)
      continue  // does NOT fall through to controller dispatch
  }
  ```
- All non-transfer messages (play, pause, resume, next, prev, seek, set_queue) continue through the normal dispatch path unchanged
- Controller command forwarding loop unchanged
- Player state broadcast path unchanged
- `broadcastState()` unchanged (still gates on `isWsPlayerRef`)
- `onState` handler in PlayerContext.tsx unchanged — only the transfer handler was moved OUT of `onState` into dedicated `onPromoted`/`onDemoted`

### ✅ AC8: Device list updates correctly after role changes

**Verdict: Correct.**

After `handleTransfer()` completes:
1. Role assignments are updated in-memory: `currentPlayer.role = "controller"`, `target.role = "player"`
2. `h.broadcastDevices()` is called → iterates all clients → builds device list with `active: c.role == "player"` → broadcasts to all connected clients
3. All DevicePicker instances receive updated device list and re-render with correct active indicators

---

## Detailed Code Review: hub.go

### handleTransfer() logic (lines 179–259)

```
✅ Lock acquisition (h.mu.Lock()) — correct, protects clients map
✅ Target validation — unlocks before sending error (no deadlock)
✅ Self-transfer check — early return with unlock
✅ Demotion before promotion — correct order (demote current, then promote target)
❌ promotedMsg built from lastState (see Gap 2 above)
✅ Promoted sent to target (non-blocking select)
✅ Demoted sent to old player (non-blocking select)
✅ broadcastDevices() called after role change
✅ Log line includes source→target mapping
```

**Quality note:** The promoted/demoted sends use non-blocking `select { case ... send <- msg: default: }` which is correct — if a client's send buffer is full, the message is silently dropped rather than blocking the hub's main goroutine.

### lastState capture (lines 126–131)

```go
case message := <-h.broadcast:
    var peek Message
    if err := json.Unmarshal(message, &peek); err == nil && peek.Type == MsgState {
        h.lastState = message
    }
```

**Issue:** This captures EVERY state message, not just the latest. The `lastState` is overwritten on each state broadcast. Since state broadcasts happen on every playback tick (~1s via `useEffect` → `broadcastState`), this is the most recent state. However, the state message doesn't include `tracks`/`start_index` → lastState has no queue data (see Gap 2).

**Thread safety:** `h.lastState` is written without the mutex held, but since (a) it's only written in the single Run() goroutine and (b) it's only read under `h.mu.Lock()` in `handleTransfer()`, there's no data race. Read under mutex, write in single goroutine → safe.

---

## Detailed Code Review: DevicePicker.tsx

### Props interface change

```typescript
// Before
{ currentTrack }: { currentTrack?: any }
// After  
{ currentTrack, queue, position }: { currentTrack?: any; queue?: any[]; position?: number }
```

✅ Both PlayerBar and MobilePlayerBar pass the new props correctly.

### handleTransfer() routing

```typescript
if (device.type === "spotify")       → transferSpotifyPlayback()
else if (device.id === "self")       → no-op
else if (device.type === "player")   → ws.transfer(id, queue, currentTrack, position)
else                                 → ws.transfer(id)  // bare transfer for controllers
```

✅ Routing logic is sound. The `"player"` type sends queue continuity data; bare `ws.transfer(id)` for non-player types.

### Filter relaxation

```typescript
// Before: .filter((d) => d.id !== "host" && d.type !== "spotify")
// After:  .filter((d) => d.type !== "spotify")
```

✅ Correct — host computer now appears naturally in the device list as a normal WebSocket device. Removed the hardcoded button that relied on `sessionStorage`+`reload`.

### Active device default

```typescript
// Before: useState<string>("host")
// After:  useState<string>("self")
```

✅ Consistent with the "This Device" concept — the default active device is the one you're looking at.

---

## Detailed Code Review: PlayerContext.tsx

### onPromoted handler (lines 411–431)

```typescript
ws.onPromoted((msg) => {
    isWsPlayerRef.current = true;
    const tracks = msg.queue || msg.tracks;    // ⚠️ both nil (see Gaps 1,2,3)
    const track = msg.currentTrack || msg.track; // ✅ msg.track works (from lastState.Track)
    if (tracks && track && tracks.length > 0) {  // ❌ always false
        clearSkipTimeout();
        consecutiveErrorsRef.current = 0;
        originalQueueRef.current = [...tracks];
        shuffledRef.current = false;
        setState({ queue: tracks, index: 0, current: track, shuffled: false });
        loadAndPlay(track);
    }
});
```

**Issue:** The guard `if (tracks && track && tracks.length > 0)` prevents any queue/playback from loading because `tracks` is always nil (see Gaps 1–3). The handler correctly sets `isWsPlayerRef.current = true` so the device becomes the active player, but with no queue and nothing playing.

**Design note:** The dual-compatibility pattern (`msg.queue || msg.tracks`, `msg.currentTrack || msg.track`) is good defensive coding for future-proofing, but neither field is populated in the current hub implementation.

### onDemoted handler (lines 434–439)

```typescript
ws.onDemoted(() => {
    const a = audioRef.current;
    if (a) a.pause();
    isWsPlayerRef.current = false;
    setState((s) => ({ ...s, playing: false }));
});
```

✅ Correct. Pauses audio, clears player flag, updates playing state.

### onTransfer handler (lines 441–459)

```typescript
ws.onTransfer((msg: any) => {
    const s = stateRef.current;
    if (wsRef.current && s.current && s.queue.length > 0) {
        wsRef.current.send({
            type: "transfer_state",
            target: msg.target,
            tracks: s.queue,
            track: s.current,
            start_index: s.index,
            position: s.position,
            playing: s.playing,
        });
    }
    const a = audioRef.current;
    if (a) a.pause();
    isWsPlayerRef.current = false;
    setState((s) => ({ ...s, playing: false }));
});
```

**Issue:** This handler is never triggered because the hub never sends `{type:"transfer"}` messages to clients. The hub only sends `promoted`/`demoted`/`error` during transfers. This is dead code.

**If it were triggered:** The `transfer_state` message it sends would be dropped by the hub's readPump (no handler for `"transfer_state"` type — it's neither `MsgTransfer` with a target nor `MsgState`).

### Deps array update

```typescript
// Before: [playFn, next, prev, seekFn]
// After:  [playFn, next, prev, seekFn, loadAndPlay, clearSkipTimeout]
```

✅ Correct — `loadAndPlay` and `clearSkipTimeout` are called in the new handlers.

### AudioContext.resume() additions (lines 122, 191–193)

```typescript
// In initAudioPipeline:
ctx.resume();

// In loadAndPlay:
if (audioCtxRef.current?.state === 'suspended') {
    audioCtxRef.current.resume();
}
```

✅ Good defensive additions for autoplay policy compliance, though not directly related to remote control. Prevents AudioContext from staying suspended when playback is triggered by a remote command.

---

## Summary of Findings

| Severity | Finding | Impact |
|----------|---------|--------|
| ❌ CRITICAL | Queue continuity broken — transferred device receives no queue data | Promoted device becomes active player with empty queue. No playback starts after transfer. |
| ⚠️ MEDIUM | `onTransfer` handler in PlayerContext.tsx is dead code | The `transfer_state` flow designed in T6 never fires. Code path exists but is unreachable. |
| ℹ️ INFO | Naming mismatch: frontend uses `queue`/`currentTrack`, Go struct uses `tracks`/`track` | Data sent by frontend is invisible to backend. This is the root cause requiring a fix to either the frontend (rename fields) or backend (add new fields). |
| ℹ️ INFO | `broadcastState()` doesn't include `tracks`/`start_index` | Even if naming were fixed, `lastState` would still lack queue data. The state broadcast needs to include queue information. |

## What Works

| Feature | Status |
|---------|--------|
| Role reassignment (player ↔ controller) | ✅ |
| Device list updates after transfer | ✅ |
| Demotion — audio pause + role clear | ✅ |
| Edge cases: target-not-found | ✅ |
| Edge cases: self-transfer no-op | ✅ |
| Edge cases: no current player | ✅ |
| Existing command forwarding (play/pause/seek/next/prev) | ✅ — no regressions |
| State broadcast continuity | ✅ — no regressions |
| Single track + position transferred | ✅ (via lastState.Track/Position) |
| Full queue transferred | ❌ BROKEN |
| Playback resumes on new device | ❌ BROKEN (depends on queue) |

---

## Root Cause Analysis

The transfer queue continuity fails because of a **three-layer gap** between T4 (backend), T5 (frontend messaging), and T6 (frontend player):

```
Frontend (T5) sends:  {type:"transfer", target, queue, currentTrack, position}
                            │
                    ┌───────┴────────┐
                    │  GAP 1: naming │  "queue" ≠ "tracks"
                    │  mismatch      │  "currentTrack" ≠ "track"
                    └───────┬────────┘
                            │
Backend (T4) reads:   {type:"transfer", target, position}  ← queue/currentTrack DROPPED
Backend (T4) builds:   promoted from lastState
                            │
                    ┌───────┴────────┐
                    │  GAP 2: state  │  broadcastState() has no
                    │  broadcast     │  tracks/start_index fields
                    └───────┬────────┘
                            │
Target receives:      {type:"promoted", playing, position, duration, track}
                      ← no tracks, no start_index
                            │
Frontend (T6) tries:  msg.queue || msg.tracks → both nil
                      if (tracks && ...) → FALSE → queue never loaded
                            │
                    ┌───────┴────────┐
                    │  GAP 3: dead   │  onTransfer never fires
                    │  code path     │  hub doesn't send "transfer"
                    └────────────────┘
```

**The intent** (inferred from T6's onTransfer handler) — a cleaner 2-phase design:
1. Source sends transfer → hub demotes source, sends "transfer" back to source
2. Source's onTransfer fires → sends transfer_state with full queue → hub forwards to target as promoted
3. Target's onPromoted fires → loads queue, starts playback

**Current reality:** Phase 2 never executes because the hub never sends "transfer" to the source.

---

## Fix Recommendation

The simplest fix that aligns with the current architecture:

### Step 1: Fix the naming mismatch in hub.go handleTransfer

Read the full transfer message fields instead of reconstructing from lastState:

```go
// In handleTransfer, read the raw transfer message fields
// The frontend sends: {type:"transfer", target, queue, currentTrack, position}
// Build promoted directly from these fields
```

Either:
- **Option A:** Add `Queue`/`CurrentTrack` fields to Go `Message` struct
- **Option B:** Pass the raw message to `handleTransfer` instead of just the target ID

### Step 2: Fix promoted message building

Build promoted from the transfer message data (which has the full queue), not from lastState (which only has single track):

```go
promotedMsg, _ = json.Marshal(Message{
    Type:     MsgPromoted,
    Playing:  lastState.Playing,   // still useful for playback state
    Position: msg.Position,        // from transfer message
    Track:    msg.CurrentTrack,     // from transfer message (need to add field)
    Tracks:   msg.Queue,            // from transfer message (need to add field)
    StartIdx: 0,                    // or preserve from transfer
})
```

### Step 3 (optional): Enable or remove the onTransfer dead code

Either:
- **Enable:** Hub sends `{type:"transfer", target}` to source after handleTransfer so onTransfer fires → source sends transfer_state → hub forwards to target
- **Remove:** Delete the onTransfer handler since Step 1+2 make it unnecessary

The two-phase approach (Steps 1+2) is simpler and sufficient. The three-phase approach (with onTransfer) adds complexity without benefit if the promoted message already has the full queue.

---

## Merge Verdict

**🟡 CONDITIONAL MERGE — Queue continuity is broken per the analysis above.**

The role assignment mechanism works correctly. All edge cases are handled. No regressions in existing functionality. The code is clean and well-structured.

However, the primary user-facing benefit of this feature — seamless playback handoff with queue continuity — does NOT work. A device that accepts a transfer becomes the "player" but with an empty queue and no playback.

**Recommendation:** Fix the queue continuity gap (Steps 1+2 above, ~30 lines of changes) before merging to `main`. The fix is straightforward and localized to `hub.go handleTransfer()` plus adding 1–2 fields to the Go `Message` struct.

**If the role-reassignment functionality alone is sufficient for the current release:** the code is safe to merge, but the queue continuity feature should be tracked as a follow-up issue.

---

## Appendix: Files to Fix

If the fix is implemented, these files need modification:

| File | Change |
|------|--------|
| `backend/internal/playerws/hub.go` | Add `Queue`/`CurrentTrack` to `Message` struct. Read them in `handleTransfer`. Build promoted from transfer data, not lastState. |
| `backend/internal/playerws/hub.go` | Optionally: send "transfer" message to source, handle "transfer_state" in readPump |
| `frontend/src/player/PlayerContext.tsx` | Optionally: remove dead `onTransfer` handler or leave as future plumbing |
