# Chat-Integrated Playlist Generation — Implementation Plan

## Executive Summary

Enable users to request AI-generated playlists conversationally through the existing chat assistant (e.g. *"make me a workout playlist"*, *"something chill for studying"*). The system detects playlist intent within the same chat endpoint, returns a structured playlist payload alongside a conversational message, and renders it using the existing `playlistPreview` UI — reusing 100% of the current download/creation orchestration.

**Scope**: Backend prompt engineering + response parsing, frontend type update + branching logic. No new endpoints, no new infrastructure.

---

## Current Architecture

| Component | Current Behavior |
|-----------|-----------------|
| `POST /api/recommendations/chat` | Text-in → `{reply: string}`. DeepSeek answers music taste questions with profile context. |
| `POST /api/recommendations/playlist` | Button-triggered → `PlaylistPayload`. Frontend then orchestrates track resolution, downloads, and playlist creation. |
| `RecsPage.tsx` | Chat UI is a simple text log. Playlist UI is a separate preview panel with download/creation buttons. |

**Problem**: Playlist generation is siloed behind a button. Users must leave chat to create a themed playlist.

---

## Recommended Approach: Unified Dual-Mode Chat Endpoint

After evaluating all angles (see Sequential Thinking analysis below), the optimal design is a **single chat endpoint that returns either text or a playlist**, using DeepSeek itself for intent detection via an enhanced system prompt.

### Why this approach wins

- **Zero new infrastructure**: No separate intent classifier, no new API routes, no state/session management.
- **One LLM call per interaction**: Same latency as current chat.
- **100% reuse of existing playlist UI**: The `playlistPreview` panel, `createAiPlaylist()` flow, track resolution polling, and download orchestration are all battle-tested and reused unchanged.
- **Graceful degradation**: If DeepSeek returns malformed JSON or misclassifies intent, the system falls back to treating the response as plain text.

### Rejected alternatives

| Alternative | Why rejected |
|-------------|-----------|
| Separate `/chat-playlist` endpoint | Requires frontend to detect intent and route, or makes two LLM calls. Adds complexity with no benefit. |
| Rule-based intent classifier (regex) | Fragile. "I'd like a playlist" works, "can you build me something to run to" fails. |
| Slash commands (`/playlist`) | Less conversational. Defeats the "magical" UX goal. |
| Stateful multi-turn refinement (v2) | Out of scope. Adds session storage requirements. Can be layered on later. |

---

## Files to Modify

1. `backend/internal/recommender/recommender.go`
2. `frontend/src/lib/api.ts`
3. `frontend/src/pages/RecsPage.tsx`

---

## Detailed Changes

### 1. Backend: `recommender.go` — Dual-mode chat handler

**Location**: `chat()` handler (lines 174–197)

**Change**: Replace the static system prompt with a dual-mode prompt that instructs DeepSeek to return structured JSON when the user asks for a playlist. After receiving the response, attempt to parse as JSON. If it contains a `playlist` field, return it as a structured payload. Otherwise return `{reply: rawText}`.

**New system prompt**:
```
You are Lexicon, a music & podcast recommendation assistant.
The user has the following listening profile:

{profile}

INSTRUCTIONS:
- If the user asks you to create, generate, or make a playlist, return ONLY valid JSON with this exact shape:
  {"message": "Your conversational reply here", "playlist": {"name": "...", "description": "...", "tracks": [{"title": "...", "artist": "...", "reason": "..."}]}}
- The playlist should have 8-12 tracks, a creative thematic name, and reference patterns from the profile.
- If the user is NOT asking for a playlist, answer their question concisely as normal text (NOT JSON).
- When suggesting items in text mode, format them as a short bulleted list.
```

**New response types**:
```go
type chatResp struct {
    Reply    string           `json:"reply"`      // always present
    Playlist *PlaylistPayload `json:"playlist,omitempty"` // only for playlist requests
}
```

**Handler logic**:
1. Build profile (same as now).
2. Send dual-mode system prompt + user message to DeepSeek.
3. Strip code fences from reply (same as `playlist()` and `callDeepSeek()`).
4. Attempt `json.Unmarshal` into an anonymous struct with `message` + `playlist`.
5. If parse succeeds AND `playlist` is non-nil → return `chatResp{Reply: message, Playlist: &playlist}`.
6. If parse fails → return `chatResp{Reply: rawReplyText}`.

**Risk mitigation**: The existing `playlist()` endpoint is left untouched. The chat endpoint’s JSON parsing is wrapped in a `try/ignore` block — any failure falls back to text mode, so regular chat questions are never broken.

---

### 2. Frontend: `api.ts` — Update chat return type

**Location**: `api.chat()` (lines 43–47)

**Change**:
```typescript
chat: (message: string) =>
    j<{ reply: string; playlist?: PlaylistPayload }>("/recommendations/chat", {
      method: "POST",
      body: JSON.stringify({ message }),
    }),
```

The `PlaylistPayload` type already exists in this file (line 221), so no new type is needed.

---

### 3. Frontend: `RecsPage.tsx` — Branch on response type

**Location**: `send()` handler (lines 60–75)

**Change**: After receiving the chat response, check if `r.playlist` exists.

**New `send()` logic**:
```typescript
async function send(e: React.FormEvent) {
  e.preventDefault();
  if (!input.trim() || chatBusy) return;
  const msg = input.trim();
  setInput("");
  setChatLog((l) => [...l, { role: "user", text: msg }]);
  setChatBusy(true);
  try {
    const r = await api.chat(msg);
    if (r.playlist) {
      // Playlist mode: show conversational message in chat, then render preview
      setChatLog((l) => [...l, { role: "ai", text: r.reply }]);
      setPlaylistPreview(r.playlist);
      setCreatedPlaylistId(null);
      setPlaylistTrackStatus({});
      const initStatus: Record<string, "pending"> = {};
      r.playlist.tracks.forEach((t) => {
        initStatus[`${t.artist} - ${t.title}`] = "pending";
      });
      setPlaylistTrackStatus(initStatus);
    } else {
      // Text mode: normal chat reply
      setChatLog((l) => [...l, { role: "ai", text: r.reply }]);
    }
  } catch (e: any) {
    setChatLog((l) => [...l, { role: "ai", text: "Error: " + e.message }]);
  } finally {
    setChatBusy(false);
  }
}
```

**What happens visually**:
1. User types *"make me a workout playlist"* and hits Send.
2. Chat log shows: `You: make me a workout playlist`
3. After ~2–5 seconds, chat log shows: `Lexicon: Here's a high-energy playlist based on your recent hip-hop and electronic listens!`
4. Simultaneously, the existing `playlistPreview` panel renders below with the playlist name, description, track list, and **Create Playlist** button.
5. The user clicks **Create Playlist** and the existing `createAiPlaylist()` flow takes over (track resolution, downloads, polling, library addition).

**No changes needed to**: `generateAiPlaylist()`, `createAiPlaylist()`, `findTrackInLibrary()`, `statusIcon()`, or the JSX playlist preview section. They are reused exactly as-is.

---

## Edge Cases & Mitigations

| Edge Case | Handling |
|-----------|----------|
| DeepSeek returns JSON without a `playlist` field | Falls back to `r.reply` text mode. |
| DeepSeek returns malformed JSON | `json.Unmarshal` fails → fallback to text mode, raw response shown in chat. |
| DeepSeek wraps JSON in ```json fences | Strip logic (already used in `playlist()` and `callDeepSeek()`) removes fences before parsing. |
| User asks ambiguous question ("what's a good playlist for running?") | DeepSeek classifies as playlist request → returns JSON. This is the desired behavior. |
| User asks "do you like playlists?" | DeepSeek should return text (not JSON) because it's not a creation request. The prompt explicitly says "create, generate, or make a playlist." |
| Playlist generation in chat fails (DeepSeek error) | `api.chat()` throws → caught in `catch` block, error shown in chat log. |

---

## Testing Strategy

1. **Unit test (backend)**: Mock DeepSeek response with JSON → verify `chatResp.Playlist` is populated.
2. **Unit test (backend)**: Mock DeepSeek response with plain text → verify `chatResp.Playlist` is nil.
3. **Unit test (backend)**: Mock DeepSeek response with malformed JSON → verify graceful fallback to text.
4. **Manual test (frontend)**: Type *"make me a playlist for studying"* → verify playlist preview renders.
5. **Manual test (frontend)**: Type *"what are my top artists?"* → verify normal text reply.
6. **Manual test (frontend)**: Click **Create Playlist** on chat-generated preview → verify full download/creation flow works.

---

## Estimated Effort

- Backend (`recommender.go`): ~20 lines changed (prompt + JSON parse logic)
- Frontend (`api.ts`): ~1 line changed (return type)
- Frontend (`RecsPage.tsx`): ~15 lines changed (branching in `send()`)
- **Total**: ~36 lines across 3 files. Small effort, high UX impact.

---

## Future Enhancements (Out of Scope)

- **Conversational refinement**: *"add 3 more tracks"* or *"make it more upbeat"* — requires chat session state to hold the current playlist draft.
- **One-click add-to-queue**: Add a "Play Now" button to the playlist preview that queues all resolved tracks.
- **Named chat sessions**: Allow users to name and save chat-generated playlists directly from the chat bubble.

---

*Plan generated via sequential thinking analysis of intent detection, response schema design, frontend UX, API surface changes, prompt engineering, and risk assessment.*
