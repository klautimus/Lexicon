# spotify — Development Context

> **Parent:** [backend](../development_context.md)
> **Files:** `spotify.go` (65 LOC), `oauth.go` (221 LOC), `client.go` (384 LOC), `sync.go` (219 LOC)
> **Last updated:** 2026-05-17

## Purpose

Full Spotify Web API integration using PKCE OAuth flow. Handles authentication, token management, API calls, and background syncing of listening history into Lexicon's `plays` table.

## Package Structure

### spotify.go — Main struct
```go
type API struct {
    db        *sql.DB
    cfg       Config
    sync      *Syncer
    verifiers sync.Map  // in-memory PKCE verifiers (replaced DB table)
}
```
Routes registered:
- `GET /api/spotify/auth-url` — returns PKCE authorization URL
- `GET /api/spotify/callback` — OAuth callback, exchanges code for tokens
- `GET /api/spotify/status` — connection status
- `POST /api/spotify/disconnect` — deletes tokens
- `POST /api/spotify/sync` — triggers manual sync
- `GET /api/spotify/token` — returns access token for Web Playback SDK

### oauth.go — PKCE flow
1. Generates `code_verifier` (SHA256) and `state` (random)
2. Stores `state → verifier` in **in-memory `sync.Map`** (not DB — fixed race condition)
3. Background goroutine cleans up verifiers older than 10 minutes
4. Redirects user to Spotify authorize URL
5. Callback exchanges `code + code_verifier` for access/refresh tokens
6. Stores tokens in `spotify_tokens` table (singleton, id=1)
7. Refreshes access token when expired (60s buffer)

### client.go — Token-managed HTTP client + API helpers
- `ValidAccessToken(ctx)` — returns non-expired access token, refreshing if necessary
- `spotifyGET()` — HTTP GET with Bearer auth + automatic 429 retry (respects `Retry-After` header)
- `fetchArtistGenres()` — batch fetches artist genres (20 per API call)
- Exported functions for cross-package use: `FetchTopArtists`, `FetchTopTracks`, `FetchUserPlaylists`, `FetchSavedTracks`, `FetchFollowedArtists`

### sync.go — Background syncer
`Syncer.Start(ctx)` launches a background loop that:
1. Waits 5 seconds after startup (grace period)
2. Runs initial sync, then every **30 minutes** (minimum gap: 25 minutes)
3. Fetches recently played tracks from Spotify API
4. For each track: upserts into `tracks` table by `spotify_id`, fetches artist genres, inserts play into `plays`
5. Deduplicates plays by `(track_id, started_at, source='spotify')`
6. Cursor tracks `last_synced_at` in `spotify_tokens` table

## Working Here

- Changing OAuth scopes: edit the scope string in oauth.go
- Adding Spotify API calls: use the token-managed client from client.go
- Adding multi-user: remove the id=1 constraint, add user_id column
