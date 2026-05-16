# spotify — Development Context

> **Parent:** [backend](../development_context.md)
> **Files:** `spotify.go` (50 LOC), `oauth.go` (227 LOC), `client.go` (157 LOC), `sync.go` (186 LOC)

## Purpose

Full Spotify Web API integration using PKCE OAuth flow. Handles authentication, token management, API calls, and background syncing of listening history into Lexicon's `plays` table.

## Package Structure

### spotify.go — Main struct
```go
type Spotify struct {
    db          *sql.DB
    clientID    string
    redirectURI string
    frontendURL string
}
```
Routes registered:
- `GET /api/spotify/auth-url` — returns PKCE authorization URL
- `GET /api/spotify/callback` — OAuth callback, exchanges code for tokens
- `GET /api/spotify/status` — connection status (`{configured, connected, display_name, ...}`)
- `POST /api/spotify/disconnect` — deletes tokens
- `POST /api/spotify/sync` — triggers manual sync
- `GET /api/spotify/token` — returns access token for Web Playback SDK

### oauth.go — PKCE flow
1. Generates `code_verifier` (SHA256) and `state` (random)
2. Stores `state → code_verifier` in `spotify_pkce` table
3. Redirects user to Spotify authorize URL
4. Callback exchanges `code + code_verifier` for access/refresh tokens
5. Stores tokens in `spotify_tokens` table (singleton, id=1)
6. Refreshes access token when expired

### client.go — Token-managed HTTP client
Wraps `http.Client` with automatic Bearer token injection and refresh. Used by sync.go and any Spotify API calls.

### sync.go — Background syncer
`Syncer.Start(ctx)` launches a background loop that:
1. Waits 5 seconds after startup (grace period for server boot)
2. Runs initial sync, then every **30 minutes** (minimum gap: 25 minutes to prevent overlap)
3. Fetches recently played tracks from Spotify API (`GET /v1/me/player/recently-played`)
4. Uses millisecond cursor from Spotify API (stored as seconds in DB)
5. For each track: upserts into `tracks` table by `spotify_id`, then inserts a play into `plays`
6. Deduplicates plays by `(track_id, started_at, source='spotify')` — won't re-import same play
7. Cursor tracks `last_synced_at` in `spotify_tokens` table

**⚠️ CRITICAL BUG — Genre never populated:** The upsert in `ingestPlay()` (line 134-144) does NOT include the `genre` column in the INSERT:
```go
INSERT INTO tracks(path, title, artist, album_artist, album, year, duration_sec, mime, media_kind, spotify_id, external_url)
VALUES(NULL, ?, ?, ?, ?, ?, ?, '', 'music', ?, ?)
```
No `genre` field! This means Spotify-synced tracks never have genre data, which is why **Top Genres in analytics is always empty/null**.

### Manual Sync
`POST /api/spotify/sync` triggers `Syncer.RunOnce()` in a goroutine — returns immediately with `{"started": true}`. Non-blocking.

### SDK Token
`GET /api/spotify/token` returns `{"access_token": "..."}` for the Spotify Web Playback SDK. Token is refreshed automatically before expiry.

## Known Issues

1. **Genre not synced** — sync.go INSERT doesn't update genre on tracks (top genres always empty)
2. **PKCE race condition** — two simultaneous auth attempts could corrupt state mapping
3. **No token encryption** — tokens stored in plaintext in SQLite
4. **Single user only** — `spotify_tokens` table has a PRIMARY KEY constraint of id=1
5. **Sync duplicates** — if a track is played both locally and on Spotify, no dedup
6. **Premium required** — Web Playback SDK requires Spotify Premium (403 otherwise)

## Working Here

- Changing OAuth scopes: edit the scope string in oauth.go
- Adding Spotify API calls: use the token-managed client from client.go
- Fixing genre sync: update the INSERT in sync.go to include genre from Spotify track data
- Adding multi-user: remove the id=1 constraint, add user_id column
