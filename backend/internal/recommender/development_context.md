# recommender — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/recommender/recommender.go` (855 LOC)
> **Last updated:** 2026-05-18

## Purpose

LLM-powered music recommendation engine using DeepSeek API. Builds a listening profile from the DB, sends it to DeepSeek, parses the JSON response into recommendations. Also handles conversational chat, AI playlist generation, and web search enrichment.

## Configuration

```go
type DeepSeekConfig struct {
    APIKey   string
    Model    string  // default "deepseek-v4-flash"
    Thinking string  // default "medium", only used with "reasoner" models
    BaseURL  string  // default "https://api.deepseek.com"
}
```

The `API` struct also holds:
- `ws *websearch.WebSearch` — for web search enrichment in chat/playlist
- `spotify *spotify.API` — for Spotify profile enrichment (top artists/tracks/playlists/saved/followed)

## Routes

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/recommendations` | Latest cached recommendations |
| `POST` | `/api/recommendations/refresh` | Generate new recommendations (DeepSeek, ~90s timeout) |
| `POST` | `/api/recommendations/playlist` | Generate AI playlist (cached by profile hash, 1h TTL) |
| `POST` | `/api/recommendations/chat` | Conversational chat about music taste (~120s timeout) |

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

### buildProfile()
Builds a compact natural-language summary from local DB:
- Top artists last 90d (from `plays` joined with `tracks`)
- Top genres (up to 8)
- Recently played (last 15)
- Library snapshot (total tracks + artists)

### buildSpotifyProfile() (v3.3.5)
Fetches the user's Spotify data to enrich the LLM profile:
- Top artists (last 6 months, with genres)
- Top tracks (last 6 months)
- User's playlists (names, descriptions, track counts, visibility)
- Saved/liked tracks (sample of 50)
- Followed artists (with genres)

Each API call has its own 15s timeout to avoid one slow call blocking others. The caller passes a parent context (typically 45s for Spotify section), and each individual call gets its own sub-timeout.

Returns empty string if Spotify not connected or token expired.

### callDeepSeek()
1. Constructs prompt with profile data
2. Calls DeepSeek via `deepseekChatWithRetry()` with temperature 0.7
3. Strips markdown code fences from response
4. Parses JSON into `RecsPayload`
5. Resolves `track_id` for ALL items (not just "library" type)
6. If a "discover" item happens to exist in DB, upgrades to "library" and sets track_id

### deepseekChatWithRetry()
- First attempt with the original prompt
- If JSON parse fails, retries once with stricter instructions
- Returns error if retry also fails

### Playlist endpoint
- Builds profile → enriches with Spotify (45s sub-context) → optionally enriches with web search (latest album by top artist) → sends curator prompt → parses JSON → returns `PlaylistPayload`
- **Cached:** Uses profile hash + 1h TTL in `recommendations` table (type='playlist')
- `force=true` query param bypasses cache

### Chat endpoint
- Accepts `{"message": "..."}`
- Uses `response_format: {type: "json_object"}` for structured output
- **Spotify enrichment:** 45s sub-context for `buildSpotifyProfile()`
- **Web search enrichment:** Detects album/artist queries via `websearch.DetectSearchQuery()`, searches for track listings, injects results into system prompt
- Dual mode: `{reply: string}` for text, `{reply, playlist}` for playlist generation
- Falls back to plain text response on JSON parse failure
- **Timeout:** 120s (increased from 60s to accommodate Spotify + DeepSeek)

## Working Here

- Changing recommendation prompt: edit `callDeepSeek()`
- Changing chat behavior: edit `chat()` or system prompt
- Adding web search signals: edit `playlist()` or `chat()` enrichment logic
- Adjusting cache TTL: edit the `strftime('%s','now','-1 hour')` in playlist cache lookup
