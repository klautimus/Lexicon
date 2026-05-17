# Spotify "redirect_uri: Not matching configuration" Fix Plan

## Problem Summary
When clicking "Connect Spotify" in Lexicon Settings, Spotify returns:
```
redirect_uri: Not matching configuration
```
This error means the `redirect_uri` parameter sent by Lexicon's backend does not exactly match any URI registered in the Spotify Developer Dashboard.

---

## Root Cause Analysis

### Inconsistent hostname usage across the codebase
Spotify requires **exact string matching** for redirect URIs. `localhost` and `127.0.0.1` are treated as completely different strings.

The Lexicon codebase was using `localhost` and `127.0.0.1` inconsistently:

| File | What it used | Notes |
|------|-------------|-------|
| `backend/.env` | `http://127.0.0.1:8787/api/spotify/callback` | Correct, explicit IP |
| `backend/internal/config/config.go` (default) | `http://localhost:8787/api/spotify/callback` | **Fallback mismatch** |
| `backend/cmd/server/main.go` (default) | `http://localhost:8787` for FrontendURL | **Mismatch** |
| `frontend/src/pages/SettingsPage.tsx` | Tells user to register `localhost:8787` | **UI vs .env mismatch** |

### What actually happens
1. User clicks "Connect Spotify" → frontend navigates to `/api/spotify/auth-url`
2. Backend `oauth.go` redirects to Spotify with `redirect_uri` from config
3. If `backend/.env` is **not loaded** (e.g., binary run from wrong directory), the Go fallback default `localhost:8787` is used
4. User's Spotify Dashboard has `127.0.0.1:8787` but **not** `localhost:8787`
5. Spotify rejects the request: `redirect_uri: Not matching configuration`

### Other issues found
- The user's Spotify Dashboard has `http://127.0.0.1:133/api/spotify/callback` — port `133` is likely a typo and should be removed.
- No startup log existed to verify what redirect_uri the server was actually using.

---

## Fixes Applied

### 1. `backend/internal/config/config.go`
**Changed:** Default `SPOTIFY_REDIRECT_URI` fallback from `localhost` to `127.0.0.1`

```go
// Before:
SpotifyRedirectURI: env("SPOTIFY_REDIRECT_URI", "http://localhost:8787/api/spotify/callback"),

// After:
SpotifyRedirectURI: env("SPOTIFY_REDIRECT_URI", "http://127.0.0.1:8787/api/spotify/callback"),
```

**Why:** Ensures fallback matches `.env` and user's dashboard even if `.env` fails to load.

### 2. `backend/cmd/server/main.go`
**Changed:** Default `SpotifyFrontendURL` fallback from `localhost` to `127.0.0.1`

```go
// Before:
cfg.SpotifyFrontendURL = "http://localhost:" + cfg.Port

// After:
cfg.SpotifyFrontendURL = "http://127.0.0.1:" + cfg.Port
```

**Added:** Startup log so the user can verify the configured redirect_uri:

```go
if cfg.SpotifyClientID != "" {
    log.Printf("[spotify] redirect_uri=%s", cfg.SpotifyRedirectURI)
}
```

### 3. `frontend/src/pages/SettingsPage.tsx`
**Changed:** Setup instructions to use `127.0.0.1` instead of `localhost`

```tsx
// Before:
http://localhost:8787/api/spotify/callback

// After:
http://127.0.0.1:8787/api/spotify/callback
```

---

## Verification Steps

### Step 1: Clean up Spotify Dashboard
1. Go to [developer.spotify.com/dashboard](https://developer.spotify.com/dashboard)
2. Open your Lexicon app → **Settings** → **Redirect URIs**
3. **Remove** `http://127.0.0.1:133/api/spotify/callback` (typo — port 133)
4. **Ensure** `http://127.0.0.1:8787/api/spotify/callback` is present
5. (Optional but recommended) **Add** `http://localhost:8787/api/spotify/callback` as a backup
6. Click **Save**

### Step 2: Rebuild and restart the backend
```powershell
cd C:\Users\kevin\CascadeProjects\lexicon\backend
go build -o server.exe ./cmd/server
server.exe
```

> **Important:** Run `server.exe` from the `backend/` directory so it can find `.env`.

### Step 3: Verify startup log
Look for this line in the server output:
```
[spotify] redirect_uri=http://127.0.0.1:8787/api/spotify/callback
```
If you see `localhost` here instead of `127.0.0.1`, the `.env` file is not being loaded.

### Step 4: Test linking
1. Open Lexicon in browser
2. Go to **Settings**
3. Click **Connect Spotify**
4. Check browser Network tab → look at the redirect to `accounts.spotify.com`
5. Verify the `redirect_uri` query parameter is exactly: `http://127.0.0.1:8787/api/spotify/callback`
6. Authorize the app — it should now succeed

---

## Why `127.0.0.1` instead of `localhost`?

- `localhost` can resolve to IPv6 (`::1`) on some Windows systems, while `127.0.0.1` is always IPv4.
- Spotify's redirect URI matching is strict string comparison.
- Using the explicit IP `127.0.0.1` avoids any DNS resolution ambiguity.

---

## Files Modified

| File | Change |
|------|--------|
| `backend/internal/config/config.go` | Default redirect URI: `localhost` → `127.0.0.1` |
| `backend/cmd/server/main.go` | Default frontend URL: `localhost` → `127.0.0.1`; added startup log |
| `frontend/src/pages/SettingsPage.tsx` | UI instructions: `localhost` → `127.0.0.1` |
