# recommender — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/recommender/recommender.go` (388 LOC, was 313 in v1)

## Purpose

LLM-powered music recommendation engine using DeepSeek API. Builds a listening profile from the DB, sends it to DeepSeek, parses the JSON response into recommendations. Also handles conversational chat and AI playlist generation.

## Configuration

```go
type DeepSeekConfig struct {
    APIKey   string
    Model    string  // default "deepseek-v4-flash"
    Thinking string  // default "medium", only used with "reasoner" models
    BaseURL  string  // default "https://api.deepseek.com"
}
```

## Routes

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/recommendations` | Latest cached recommendations |
| `POST` | `/api/recommendations/refresh` | Generate new recommendations (DeepSeek, ~90s timeout) |
| `POST` | `/api/recommendations/playlist` | Generate AI playlist (NEW v2, NOT cached) |
| `POST` | `/api/recommendations/chat` | Conversational chat about music taste |

## Data Types

```go
type Item struct {
    Title   string `json:"title"`
    Artist  string `json:"artist"`
    Reason  string `json:"reason"`
    TrackID *int64 `json:"track_id,omitempty"`
    Type    string `json:"type"` // "library" | "discover"
}

type RecsPayload struct {
    Summary string `json:"summary"`
    Trends  string `json:"trends"`
    Items   []Item `json:"items"`
}

type PlaylistPayload struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    Tracks      []PlaylistTrack `json:"tracks"`
}
```

## Key Functions

### buildProfile (line 240)
Builds a compact natural-language summary:
- Top artists last 90d (from `plays` joined with `tracks`)
- Top genres (up to 8)
- Recently played (last 15)
- Library snapshot (total tracks + artists)
Returns a string, not JSON.

### callDeepSeek (line 302)
1. Constructs prompt with profile data
2. Calls DeepSeek via `deepseekChat()` with temperature 0.7
3. Strips markdown code fences from response
4. Parses JSON into `RecsPayload`
5. **v2 change:** Resolves `track_id` for ALL items (not just "library" type)
6. If a "discover" item happens to exist in DB, upgrades to "library" and sets track_id
7. On JSON parse error, falls back to `RecsPayload{Summary: reply}` (silent fallback!)

### deepseekChat (line 384)
- Marshals messages + config into DeepSeek API request format
- `thinking_effort` only set for "reasoner" models
- Sends `POST /v1/chat/completions` with Bearer auth
- Returns raw content string

### Playlist endpoint (line 114)
- Builds profile → sends curator prompt → parses JSON → returns `PlaylistPayload`
- Does NOT cache results (unlike refresh endpoint)
- Prompt requests 8-12 tracks, creative thematic name, mix of owned/discover

### Chat endpoint (line 175)
- Accepts `{"message": "..."}` 
- Uses `response_format: {type: "json_object"}` for structured output
- Dual mode: `{reply: string}` for text, `{reply, playlist}` for playlist generation
- Falls back to plain text response on JSON parse failure (line 222-224)

## Known Issues

1. **Silent fallback** — JSON parse error returns `RecsPayload{Summary: reply}` with no logging
2. **Playlist not cached** — each call hits DeepSeek (~tokens), unlike refresh which stores to DB
3. **No rate limiting** — multiple rapid calls will burn DeepSeek credits
4. **temperature=0.7** — non-deterministic output, same profile may yield different recs
5. **No timeout for playlist generation** — could hang on slow DeepSeek responses
6. **Prompt injection risk** — user chat messages go directly to DeepSeek

## Working Here

- Changing recommendation prompt: edit `callDeepSeek()` (line 302)
- Changing chat behavior: edit `chat()` (line 175) or system prompt
- Adding caching for playlists: store in recommendations table or new table
- Adding web search integration: inject search results into chat prompt
  - Full plan in `references/websearch-integration.md`
