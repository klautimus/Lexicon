# Chat Playlist Integration — Fix Plan

## What Went Wrong

User asked: *"can you help me create a playlist of happy upbeat songs that match the type of music i normally listen to?"*

DeepSeek returned a fully formatted **text response** with markdown bullet points instead of JSON:

```
Lexicon: Here's a short, happy, upbeat playlist...

- **Saint Motel – "My Type"**
  Bouncy brass, dance-pop energy...
- **Jamiroquai – "Virtual Insanity"**
  Upbeat, funky, and joyful...
```

The model generated **excellent content** (8 well-themed tracks referencing the user's profile) but completely ignored the JSON instruction. The backend's JSON parse attempt failed, so it fell back to text mode.

---

## Root Cause Analysis

### 1. Prompt structure primes natural-language responses

The current system prompt order is:
1. "You are Lexicon..." (identity)
2. **Massive block of natural-language profile data** (artists, genres, recent plays, library stats)
3. "INSTRUCTIONS: If user asks for playlist, return JSON..." (format rule)

The profile data (hundreds of tokens of natural language) comes **before** the critical format instruction. By the time the model reads the JSON rule, it's already been primed to output natural language. LLMs have strong training bias for conversational playlist responses — this overrides a buried conditional instruction.

### 2. Conditional instruction is too polite and easy to override

"If the user asks you to create, generate, or make a playlist, return ONLY valid JSON..."

The word "ONLY" is weak. The model treats it as a preference, not a constraint. It sees a conversational request and defaults to its training: "give a friendly, helpful text response."

### 3. No enforcement mechanism

The code attempts to parse the response as JSON and falls back to text. But there's nothing that **forces** the model to output JSON. It's an honor-system instruction.

---

## The Fix: Two-Pronged Approach

### Prong 1: Restructure the prompt (stronger constraint language + correct ordering)

Move the **CRITICAL FORMAT RULES to the absolute top** of the system prompt, before any profile data or role description. Use explicit negative constraints.

### Prong 2: Force JSON mode via `response_format`

Set `response_format: {type: "json_object"}` on the API request. DeepSeek supports this natively and will **enforce valid JSON output** regardless of what the model "wants" to say. This eliminates the choice entirely.

Combined, these make it physically impossible for the model to output prose.

---

## Changes Required

### 1. Backend: `recommender.go` — New prompt structure + JSON mode

**Current prompt (lines 187-197):**
```go
system := `You are Lexicon, a music & podcast recommendation assistant.
The user has the following listening profile:

` + profile + `

INSTRUCTIONS:
- If the user asks you to create, generate, or make a playlist, return ONLY valid JSON...
- If the user is NOT asking for a playlist, answer their question concisely as normal text (NOT JSON).`
```

**New prompt:**
```go
system := `CRITICAL OUTPUT FORMAT RULES — READ FIRST:
You have TWO possible response formats. Choose based on the user's message.

FORMAT A — Playlist Request:
If the user's message is a request to create, generate, or make a playlist, you MUST respond with ONLY a JSON object. NO markdown. NO bullet points. NO prose outside the JSON. NO code fences.

Required JSON shape:
{"message":"A short conversational reply","playlist":{"name":"Creative Playlist Name","description":"1-2 sentence vibe","tracks":[{"title":"...","artist":"...","reason":"..."}]}}

Rules for Format A:
- 8-12 tracks
- Creative thematic name (not generic)
- Reference patterns from the user's profile
- NO text outside the JSON object
- NO markdown formatting
- NO explanations before or after the JSON

FORMAT B — Normal Question:
If the user is NOT asking for a playlist, answer concisely as normal text.
When suggesting items, format as a short bulleted list.

---
USER LISTENING PROFILE:
` + profile + `

---
REMEMBER: Check the user's message. Playlist request → Format A (JSON ONLY). Anything else → Format B (text).`
```

**Also set ResponseFormat on the API request:**
```go
reqBody := dsRequest{
    Model:           a.cfg.Model,
    Messages:        dmsgs,
    Temperature:     0.7,
    ThinkingEffort:  a.cfg.Thinking,
    ResponseFormat:  &dsRespFmt{Type: "json_object"},  // <-- NEW: forces JSON
}
```

Since we're now in JSON mode, the response is ALWAYS JSON. The frontend already parses JSON. The handler simplifies to:

1. Always parse the response as JSON (guaranteed valid by API)
2. Check if the parsed object has a `playlist` field with tracks
3. If yes → return `{reply, playlist}`
4. If no → extract just the `message` field as text

No more fallback parsing needed.

### 2. Frontend: No changes needed

The frontend's `send()` function already handles the `{reply, playlist?}` response shape correctly:
- `if (r.playlist)` → show playlist preview
- `else` → show text reply

The frontend code is already correct. The bug was entirely on the backend (model not outputting JSON).

---

## Why This Will Work

1. **JSON mode is API-enforced** — DeepSeek's `json_object` response format guarantees valid JSON. The model literally cannot output markdown bullet points.
2. **Prompt puts rules first** — The format instruction is at the TOP, before any priming data. The model sees the constraint before being influenced by profile content.
3. **Negative constraints are explicit** — "NO markdown. NO bullet points. NO prose outside the JSON." These are harder to ignore than a positive instruction.
4. **The model already generates great content** — It proved it can make excellent playlists. We just need to force the right container format.

---

## Edge Cases

| Scenario | Handling |
|----------|----------|
| User asks "what's a good playlist?" (curiosity, not request) | The model may still return JSON with an empty playlist. The frontend checks `r.playlist?.tracks?.length > 0`. If empty, treat as text. |
| User asks "do you like playlists?" | Model returns `{"message": "..."}` with no playlist field. Frontend shows text. |
| DeepSeek JSON mode returns unexpected shape | API guarantees valid JSON, so `json.Unmarshal` always succeeds. We just check field presence. |
| Model returns `{"message":"...","playlist":{"tracks":[]}}` | Frontend checks track count. Empty playlist → text mode. |

---

## Testing Strategy

1. **Test playlist request**: "Make me a workout playlist" → verify JSON response with playlist
2. **Test text question**: "What are my top artists?" → verify JSON response with message only
3. **Test ambiguous**: "What's a good playlist for studying?" → verify playlist JSON
4. **Test non-request**: "Do you like music?" → verify text-only JSON

---

## Estimated Effort

- `recommender.go`: ~30 lines changed (prompt restructure + JSON mode activation)
- `api.ts`: No changes
- `RecsPage.tsx`: No changes
- **Total**: ~30 lines in 1 file

---

*Analysis based on: prompt structure investigation, LLM instruction-following behavior, DeepSeek API capabilities, and the actual failed response content.*
