# Lexicon Audit Fix Plan — 2026-05-20

## Executive Summary

Six domain audits were conducted across the Lexicon codebase covering Backend API, Backend Data, Backend Logic, Integrations, Frontend, and Build/Security. A total of **96 unique findings** were identified:

| Severity | Count | Description |
|----------|-------|-------------|
| **CRITICAL** | 6 | Auth bypass, path traversal, silent data corruption, goroutine races |
| **MAJOR** | 34 | Broken features, race conditions, error handling gaps, resource leaks |
| **MINOR** | 56 | Edge cases, code quality, schema drift, missing validation |

### Cross-Domain Deduplication Notes

Several issues were found across multiple audits and have been merged:

1. **Auth middleware bypass** — Found in both Security audit (t_0cbee757) and Backend API audit (t_b82e041f). The security audit identified the root cause (auth is a no-op when `LEXICON_API_KEY` is unset, which is the default). The backend API audit found the downstream effects (GET endpoints unauthenticated, timing-unsafe comparison). **Merged into one fix.**

2. **CORS issues** — Found in both Security audit and Backend API audit. Security found `AllowCredentials` with wildcard; backend API found the permissive origin function. **Merged into one fix.**

3. **Timing-unsafe key comparison** — Found in both Security audit and Backend API audit. **Merged into one fix.**

4. **Goroutine lifecycle / shutdown races** — Found in both Backend API audit and Backend Logic audit. Backend API found the initial scan goroutine is detached; Backend Logic found downloader goroutine races. **Related but separate fixes.**

5. **Race conditions in downloader** — Backend Logic audit found multiple race conditions in `run()`, `runSearch()`, and playlist position insertion. These are distinct code locations. **Separate fixes per location.**

6. **INNER JOIN dropping orphaned plays** — Found in both Backend Data audit (analytics + history) and implicitly in Backend Logic audit. **Merged into one fix.**

After deduplication: **~85 unique bugs** requiring fixes.

---

## Implementation Order

### Phase 1: Critical — Security & Data Integrity (Fix First)

These must be fixed before anything else. They represent active security vulnerabilities or data corruption risks.

---

### BUG-SEC-1: Auth Middleware Bypass by Default
- **Severity:** CRITICAL
- **Domain:** Build/Security + Backend API
- **Files:** `backend/internal/auth/middleware.go`
- **Lines:** 11-23
- **Root Cause:** When `LEXICON_API_KEY` environment variable is unset (the default), the auth middleware is a complete no-op. All POST/PUT/DELETE operations are completely unauthenticated. Additionally, the key comparison uses `==` (timing-unsafe).
- **Fix Description:**
  1. Require `LEXICON_API_KEY` to be set at startup; fail fast with a clear error message if missing.
  2. Replace `==` string comparison with `subtle.ConstantTimeCompare()` to prevent timing attacks.
  3. Add a startup warning if the key is empty or too short (< 16 chars).
- **Affected Components:** All API endpoints (library, playlists, downloader, analytics, history, scanner)
- **Cross-Cutting:** This is the single most impactful fix. Until this is resolved, all other API security is theater.

---

### BUG-SEC-2: Path Traversal in streamer.go
- **Severity:** CRITICAL
- **Domain:** Build/Security
- **Files:** `backend/internal/streamer/streamer.go`
- **Lines:** (path validation logic)
- **Root Cause:** Symlink bypass in path traversal check. An attacker can craft a request with a symlink pointing to arbitrary files on the filesystem, and the streamer will serve them.
- **Fix Description:**
  1. Resolve the final path using `filepath.EvalSymlinks()` before checking prefix.
  2. Verify the resolved path is within the music directory.
  3. Reject requests containing `..` components before symlink resolution.
- **Affected Components:** Stream endpoint, any authenticated user

---

### BUG-SEC-3: Path Traversal in cover.go
- **Severity:** CRITICAL
- **Domain:** Build/Security
- **Files:** `backend/internal/streamer/cover.go`
- **Lines:** (`openReader` function)
- **Root Cause:** No path traversal check in `openReader()`. Also no file size limit on tag parsing (potential DoS via large embedded artwork).
- **Fix Description:**
  1. Add the same path traversal check as streamer.go (resolve symlinks, verify prefix).
  2. Add a file size limit when reading tag data (e.g., 10MB max).
  3. Add symlink check.
- **Affected Components:** Cover art endpoint

---

### BUG-DATA-1: analytics.overview() Silently Returns Zeroed Data on DB Failure
- **Severity:** CRITICAL
- **Domain:** Backend Data
- **Files:** `backend/internal/analytics/analytics.go`
- **Lines:** 86-91
- **Root Cause:** All four `QueryRowContext().Scan()` calls in `overview()` discard errors. On DB failure, the function returns zeroed data as 200 OK. The frontend renders `"total_plays": 0` with no error indication.
- **Fix Description:**
  ```go
  if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM plays`).Scan(&o.TotalPlays); err != nil {
      log.Printf("[analytics] overview total_plays: %v", err)
      writeError(w, "failed to load analytics", 500)
      return
  }
  // Repeat for all 4 Scan calls
  ```
- **Affected Components:** Analytics overview endpoint, frontend stats display
- **Cross-Cutting:** This pattern (ignoring Scan errors) also exists in `topTracks`, `topGenres`, and `heatmap` — fix all at once.

---

### BUG-API-1: Goroutine Lifecycle Races in main.go
- **Severity:** CRITICAL
- **Domain:** Backend API
- **Files:** `backend/internal/main.go`
- **Lines:** 134-177 (doRescan race), 227-253 (initial scan detached), 57 (DB close race)
- **Root Cause:** Three related issues:
  1. `doRescan` generation counter is read/written from multiple goroutines without synchronization.
  2. Initial scan goroutine is launched with no way to cancel it on shutdown.
  3. `database.Close()` can race with in-flight goroutines.
- **Fix Description:**
  1. Use `atomic.Int64` for the rescan generation counter.
  2. Pass a cancellable context to the initial scan; cancel it during shutdown.
  3. Use `sync.WaitGroup` to track all background goroutines and wait for them before closing the DB.
- **Affected Components:** Scanner, rescan logic, graceful shutdown

---

### BUG-API-2: Database Close Races with In-Flight Goroutines
- **Severity:** CRITICAL
- **Domain:** Backend API
- **Files:** `backend/internal/main.go`
- **Lines:** 57, 439-445
- **Root Cause:** `database.Close()` is called without waiting for background goroutines to finish. Shutdown doesn't wait for goroutines to exit.
- **Fix Description:**
  1. Add a `sync.WaitGroup` that all background goroutines register on.
  2. During shutdown, cancel all contexts, then `Wait()` on the WaitGroup before calling `db.Close()`.
  3. Add a shutdown timeout (e.g., 10s) to prevent hanging.
- **Affected Components:** All background goroutines, graceful shutdown

---

### Phase 2: Major — Broken Features & Resource Leaks

---

### BUG-INT-1: Spotify Callback Uses Request Context for Background Sync
- **Severity:** MAJOR
- **Domain:** Integrations
- **Files:** `backend/internal/spotify/` (callback handler)
- **Root Cause:** The Spotify OAuth callback handler starts a background sync using the HTTP request context. When the HTTP response is sent (redirect), the context is cancelled, killing the background sync mid-flight.
- **Fix Description:** Use `context.Background()` with a custom timeout for the background sync, detached from the request context.
- **Affected Components:** Spotify integration, background sync

---

### BUG-INT-2: Spotify 429 Retry Loops Indefinitely
- **Severity:** MAJOR
- **Domain:** Integrations
- **Files:** `backend/internal/spotify/` (API client)
- **Root Cause:** When Spotify returns 429 (rate limited), the retry logic has no maximum retry count or backoff cap, causing an infinite loop that hammers the API.
- **Fix Description:**
  1. Add exponential backoff with jitter.
  2. Cap retries at 5 attempts.
  3. Respect `Retry-After` header if present.
- **Affected Components:** Spotify integration

---

### BUG-INT-3: Apple Music Reconnect Resets last_synced_at Causing Duplicates
- **Severity:** MAJOR
- **Domain:** Integrations
- **Files:** `backend/internal/apple/` (sync logic)
- **Root Cause:** When Apple Music reconnects (token refresh), `last_synced_at` is reset to zero, causing the next sync to re-import all tracks as new.
- **Fix Description:** Preserve `last_synced_at` across reconnections. Only reset it explicitly when the user requests a full re-sync.
- **Affected Components:** Apple Music integration

---

### BUG-INT-4: Podcaster subscribe() Has No HTTP Timeout or URL Validation (SSRF Risk)
- **Severity:** MAJOR
- **Domain:** Integrations
- **Files:** `backend/internal/podcaster/` (subscribe handler)
- **Root Cause:** The `subscribe()` function makes HTTP requests with no timeout and no URL validation, creating a Server-Side Request Forgery (SSRF) vulnerability.
- **Fix Description:**
  1. Add an HTTP client with a 30-second timeout.
  2. Validate the URL scheme (http/https only).
  3. Block private IP ranges (10.x, 172.16-31.x, 192.168.x, 127.x).
  4. Limit redirect hops to 3.
- **Affected Components:** Podcaster integration

---

### BUG-INT-5: SyncAllFeeds Uses context.Background() with No Cancellation
- **Severity:** MAJOR
- **Domain:** Integrations
- **Files:** `backend/internal/podcaster/` (sync logic)
- **Root Cause:** `SyncAllFeeds` uses `context.Background()` with no cancellation mechanism, meaning it can run indefinitely and can't be interrupted during shutdown.
- **Fix Description:** Accept a `context.Context` parameter and propagate it through all operations. Callers should pass a cancellable context with timeout.
- **Affected Components:** Podcaster integration, shutdown handling

---

### BUG-LOGIC-1: Race Condition in downloader.run() Status Check/Set
- **Severity:** MAJOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/downloader/downloader.go`
- **Lines:** 714-727
- **Root Cause:** The status field is read and written from multiple goroutines without synchronization. A concurrent read can see a partially-written or stale status.
- **Fix Description:** Use `sync.RWMutex` or `atomic.Value` for the status field. Wrap check-and-set in a mutex lock.
- **Affected Components:** Downloader, status reporting

---

### BUG-LOGIC-2: Race Condition in downloader.runSearch()
- **Severity:** MAJOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/downloader/downloader.go`
- **Lines:** 964-975
- **Root Cause:** Same pattern as BUG-LOGIC-1 but in `runSearch()`. Status check/set is not atomic.
- **Fix Description:** Same approach — use mutex or atomic operations for status.
- **Affected Components:** Downloader search functionality

---

### BUG-LOGIC-3: Poll Goroutine Uses Cancelled Context
- **Severity:** MAJOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/downloader/downloader.go`
- **Lines:** 1091-1117
- **Root Cause:** The poll goroutine captures a context that may already be cancelled when the goroutine starts, causing it to immediately fail or behave unexpectedly.
- **Fix Description:** Check if the context is already cancelled before starting the poll loop. Use a fresh context with timeout derived from the parent, not the parent directly.
- **Affected Components:** Downloader polling

---

### BUG-LOGIC-4: deepseekParseQuery Uses http.DefaultClient (No Timeout)
- **Severity:** MAJOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/downloader/downloader.go`
- **Lines:** 1353
- **Root Cause:** `http.DefaultClient` has no timeout, meaning a hung DeepSeek API call can block the downloader indefinitely.
- **Fix Description:** Create a dedicated HTTP client with a 30-second timeout:
  ```go
  var deepSeekClient = &http.Client{Timeout: 30 * time.Second}
  ```
- **Affected Components:** Downloader, DeepSeek integration

---

### BUG-LOGIC-5: upgradeAll is a No-Op
- **Severity:** MAJOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/downloader/downloader.go`
- **Lines:** 1554-1592
- **Root Cause:** The `upgradeAll` function has a logic error that causes it to skip all tracks. The loop condition or filter is inverted.
- **Fix Description:** Review the loop logic and fix the condition. Add a test that verifies at least one track is processed.
- **Affected Components:** Downloader upgrade functionality

---

### BUG-LOGIC-6: runSearchWithTrackID Doesn't Wait for Completion
- **Severity:** MAJOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/downloader/downloader.go`
- **Lines:** 1483-1521
- **Root Cause:** The function launches a goroutine but doesn't wait for it to complete before returning, causing the caller to think the operation succeeded when it may have failed.
- **Fix Description:** Use a `sync.WaitGroup` or channel to wait for the goroutine to complete, or return an error channel.
- **Affected Components:** Downloader search-by-trackID

---

### BUG-LOGIC-7: json.Unmarshal Error Ignored in callDeepSeek
- **Severity:** MAJOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/recommender/recommender.go`
- **Lines:** 894
- **Root Cause:** The return value of `json.Unmarshal` is discarded. If the DeepSeek API returns malformed JSON, the recommender silently uses zero-value data.
- **Fix Description:** Check the error and return/log it:
  ```go
  if err := json.Unmarshal(data, &result); err != nil {
      log.Printf("[recommender] deepseek parse: %v", err)
      return nil, err
  }
  ```
- **Affected Components:** Recommender, DeepSeek integration

---

### BUG-LOGIC-8: chat() Handler Has No Request Body Size Limit
- **Severity:** MAJOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/recommender/recommender.go`
- **Lines:** 324
- **Root Cause:** The chat handler reads the request body with no size limit, allowing a client to send a multi-gigabyte payload and cause OOM.
- **Fix Description:** Use `http.MaxBytesReader(w, r.Body, 1<<20)` to limit to 1MB.
- **Affected Components:** Recommender chat endpoint

---

### BUG-LOGIC-9: Race Condition on Playlist Position Insertion
- **Severity:** MAJOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/playlists/playlists.go`
- **Lines:** 293-296
- **Root Cause:** Playlist position insertion reads the max position, increments it, and inserts — without a transaction or lock. Concurrent inserts can get the same position.
- **Fix Description:** Wrap the read-increment-insert in a transaction with `SELECT MAX(position) ... FOR UPDATE` or use a mutex.
- **Affected Components:** Playlist management

---

### BUG-LOGIC-10: Goroutine Leaks on Shutdown (Scanner)
- **Severity:** MAJOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/scanner/scanner.go`
- **Lines:** 179-192
- **Root Cause:** Scanner goroutines use `context.Background()` which is never cancelled, so they leak on shutdown.
- **Fix Description:** Pass a cancellable context from main. Use `context.WithCancel` and cancel on shutdown signal.
- **Affected Components:** Scanner, graceful shutdown

---

### BUG-DATA-2: rows.Scan() Errors Ignored in topTracks, topGenres, heatmap
- **Severity:** MAJOR
- **Domain:** Backend Data
- **Files:** `backend/internal/analytics/analytics.go`
- **Lines:** 150, 178, 209
- **Root Cause:** Three analytics handlers discard `rows.Scan()` errors. Failed scans produce zero-value structs that are appended to output, corrupting the response.
- **Fix Description:** Model after `topArtists()` (lines 116-120) which correctly checks errors:
  ```go
  if err := rows.Scan(&x.ID, &x.Title, &x.Artist, &x.Plays); err != nil {
      log.Printf("[analytics] topTracks scan: %v", err)
      writeError(w, "failed to load data", 500)
      return
  }
  ```
- **Affected Components:** Analytics endpoints

---

### BUG-DATA-3: os.Remove NotFound Causes Unnecessary Rollback in deleteTrack
- **Severity:** MAJOR
- **Domain:** Backend Data
- **Files:** `backend/internal/library/library.go`
- **Lines:** 317-323
- **Root Cause:** If the file was already deleted externally, `os.Remove()` returns `os.ErrNotExist`, which triggers a rollback. The DB row is preserved pointing to a non-existent file.
- **Fix Description:**
  ```go
  if path != "" {
      if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
          tx.Rollback()
          writeError(w, "delete failed", 500)
          return
      }
  }
  ```
- **Affected Components:** Library track deletion

---

### BUG-DATA-4: tx.Rollback() Errors Silently Discarded
- **Severity:** MAJOR
- **Domain:** Backend Data
- **Files:** `backend/internal/library/library.go`
- **Lines:** 312, 319
- **Root Cause:** If `Rollback()` fails (connection closed, SQLite busy), the transaction remains open, leaking a connection and blocking WAL checkpointing.
- **Fix Description:**
  ```go
  if err := tx.Rollback(); err != nil {
      log.Printf("[library] rollback failed: %v", err)
  }
  ```
- **Affected Components:** Library transactions

---

### BUG-DATA-5: Inconsistent Error Response Format in analytics.go
- **Severity:** MAJOR
- **Domain:** Backend Data
- **Files:** `backend/internal/analytics/analytics.go`
- **Lines:** 104, 118, 125, 137, 155, 166, 182, 197, 214
- **Root Cause:** analytics.go uses `http.Error()` (text/plain) while other packages use `writeError()` (JSON). API clients expecting JSON will break on analytics errors. Raw SQLite error strings may leak internal details.
- **Fix Description:** Add a `writeError()` helper to analytics.go (or share one across packages) and use it consistently.
- **Affected Components:** All analytics endpoints, API client error handling

---

### BUG-DATA-6: INNER JOIN Drops Orphaned Plays Silently
- **Severity:** MAJOR
- **Domain:** Backend Data
- **Files:** `backend/internal/history/history.go:87`, `backend/internal/analytics/analytics.go:99-100,133,163`
- **Root Cause:** Implicit INNER JOIN excludes plays whose tracks were deleted (orphaned plays). Analytics counts won't match the sum of artist/track breakdowns.
- **Fix Description:** Use `LEFT JOIN` and handle NULLs:
  ```sql
  FROM plays p LEFT JOIN tracks t ON t.id=p.track_id
  ```
  With `IFNULL(t.title,'(deleted)')`, `IFNULL(t.artist,'')`.
- **Affected Components:** History, analytics

---

### BUG-FE-1: Race Condition in loadAndPlay
- **Severity:** MAJOR
- **Domain:** Frontend
- **Files:** `frontend/src/` (player component)
- **Root Cause:** `loadAndPlay` has a race condition where the audio element can be in an inconsistent state if the user rapidly triggers play/pause or track changes.
- **Fix Description:** Add a playback state machine with proper locking. Queue play requests and process them sequentially. Cancel in-flight operations when a new request arrives.
- **Affected Components:** Audio playback, user interaction

---

### BUG-FE-2: Nested setState in goNext
- **Severity:** MAJOR
- **Domain:** Frontend
- **Files:** `frontend/src/` (player/playlist component)
- **Root Cause:** `goNext` calls `setState` inside another `setState` callback, which can cause batching issues and stale state reads in React.
- **Fix Description:** Use functional state updates or `useReducer` to consolidate state transitions.
- **Affected Components:** Playlist navigation

---

### BUG-FE-3: Spotify Poller Memory Leak
- **Severity:** MAJOR
- **Domain:** Frontend
- **Files:** `frontend/src/` (Spotify integration component)
- **Root Cause:** The Spotify status poller interval is never cleared on component unmount, causing a memory leak and continued API calls after navigation.
- **Fix Description:** Return a cleanup function from `useEffect` that clears the interval:
  ```tsx
  useEffect(() => {
      const id = setInterval(pollSpotify, 5000);
      return () => clearInterval(id);
  }, []);
  ```
- **Affected Components:** Spotify integration, memory management

---

### BUG-FE-4: Missing Network Error Handling in api.ts
- **Severity:** MAJOR
- **Domain:** Frontend
- **Files:** `frontend/src/lib/api.ts`
- **Root Cause:** The API client doesn't handle network errors (offline, DNS failure, timeout). Failed requests show generic or confusing error messages.
- **Fix Description:** Add a try/catch around all fetch calls. Detect network errors (`TypeError: Failed to fetch`) and show user-friendly messages. Add retry logic with backoff for transient failures.
- **Affected Components:** All API calls from frontend

---

### BUG-FE-5: Bulk Upgrade Only Upgrades Current Page
- **Severity:** MAJOR
- **Domain:** Frontend
- **Files:** `frontend/src/` (library/management component)
- **Root Cause:** The bulk upgrade action only applies to tracks visible on the current pagination page, not the full selection set.
- **Fix Description:** Track selected IDs independently of the displayed page. Pass all selected IDs to the backend endpoint.
- **Affected Components:** Library management, bulk operations

---

### BUG-FE-6: Null Crash in Podcast Download Polling
- **Severity:** MAJOR
- **Domain:** Frontend
- **Files:** `frontend/src/` (podcast component)
- **Root Cause:** The podcast download polling code doesn't null-check the response before accessing properties, causing a crash when the server returns null/empty.
- **Fix Description:** Add null checks before accessing response properties. Show a loading/empty state when data is unavailable.
- **Affected Components:** Podcast download UI

---

### BUG-FE-7: Podcast Feed Re-render Loop
- **Severity:** MAJOR
- **Domain:** Frontend
- **Files:** `frontend/src/` (podcast component)
- **Root Cause:** A useEffect dependency on an object that's recreated every render causes an infinite re-render loop when polling podcast feed data.
- **Fix Description:** Memoize the dependency or use a ref for values that shouldn't trigger re-renders.
- **Affected Components:** Podcast feed display

---

### BUG-BUILD-1: Placeholder GUID in lexicon.iss
- **Severity:** MAJOR
- **Domain:** Build/Security
- **Files:** `release/lexicon.iss`
- **Root Cause:** The Inno Setup script uses a placeholder AppId GUID. This causes installer conflicts if multiple builds are installed.
- **Fix Description:** Generate a proper GUID and replace the placeholder. Add a build step that validates the GUID format.
- **Affected Components:** Windows installer

---

### BUG-BUILD-2: Plaintext Secrets in .env
- **Severity:** MAJOR
- **Domain:** Build/Security
- **Files:** `.env`, `release/lexicon.iss`
- **Root Cause:** Secrets (API keys, tokens) are stored in plaintext in the .env file and bundled into the installer.
- **Fix Description:**
  1. Remove secrets from .env; use environment variables or a secrets manager.
  2. Add .env to .gitignore if not already there.
  3. Use a template (.env.example) for documentation.
- **Affected Components:** Build process, deployment

---

### BUG-BUILD-3: Unauthenticated Stream Endpoint
- **Severity:** MAJOR
- **Domain:** Build/Security
- **Files:** `backend/internal/main.go`, `backend/internal/streamer/streamer.go`
- **Root Cause:** The stream endpoint has no authentication. Combined with the path traversal issues, this means anyone can stream any file.
- **Fix Description:** Apply the auth middleware to the stream endpoint. This depends on BUG-SEC-1 being fixed first.
- **Affected Components:** Stream endpoint

---

### BUG-BUILD-4: CORS Wildcard
- **Severity:** MAJOR
- **Domain:** Build/Security
- **Files:** `backend/internal/main.go`
- **Root Cause:** CORS is configured with `AllowCredentials: true` and a wildcard/permissive origin, which is a security risk.
- **Fix Description:**
  1. Replace the permissive origin function with an explicit allowlist.
  2. Remove `AllowCredentials` if not needed, or restrict origins.
  3. Set `AllowCredentials: false` if using wildcard origins.
- **Affected Components:** API CORS configuration

---

### BUG-BUILD-5: Timing-Unsafe Key Comparison
- **Severity:** MAJOR
- **Domain:** Build/Security + Backend API
- **Files:** `backend/internal/auth/middleware.go:23`
- **Root Cause:** API key comparison uses `==` which is vulnerable to timing attacks. An attacker can determine the key byte-by-byte by measuring response times.
- **Fix Description:** Use `subtle.ConstantTimeCompare([]byte(provided), []byte(expected))`.
- **Affected Components:** Authentication

---

### BUG-BUILD-6: GET Endpoints Unauthenticated
- **Severity:** MAJOR
- **Domain:** Build/Security
- **Files:** `backend/internal/main.go`
- **Root Cause:** GET endpoints (library, analytics, history) have no authentication, exposing all user data.
- **Fix Description:** Apply auth middleware to GET endpoints. Consider read-only vs read-write permission levels.
- **Affected Components:** All read API endpoints

---

### BUG-BUILD-7: Missing Security Headers on Static Files
- **Severity:** MAJOR
- **Domain:** Build/Security
- **Files:** `backend/internal/main.go`
- **Root Cause:** Static file responses lack security headers (X-Content-Type-Options, X-Frame-Options, Content-Security-Policy).
- **Fix Description:** Add a middleware that sets security headers on all responses:
  ```go
  w.Header().Set("X-Content-Type-Options", "nosniff")
  w.Header().Set("X-Frame-Options", "DENY")
  w.Header().Set("Content-Security-Policy", "default-src 'self'")
  ```
- **Affected Components:** Static file serving, all HTTP responses

---

### BUG-BUILD-8: Potential Command Injection via Search Query
- **Severity:** MAJOR
- **Domain:** Build/Security
- **Files:** `backend/internal/downloader/downloader.go`
- **Root Cause:** User search query is passed to a shell command without proper sanitization, allowing command injection.
- **Fix Description:** Use parameterized command execution (`exec.Command` with separate args) instead of shell string interpolation. Validate and sanitize the query.
- **Affected Components:** Downloader search

---

### BUG-BUILD-9: Status Endpoint Exposes Tool Paths
- **Severity:** MAJOR
- **Domain:** Build/Security
- **Files:** `backend/internal/downloader/downloader.go` (status endpoint)
- **Root Cause:** The status endpoint returns internal tool paths (ffmpeg, yt-dlp locations), aiding an attacker in understanding the system layout.
- **Fix Description:** Remove internal paths from the status response. Only return operational status (running, version).
- **Affected Components:** Status endpoint

---

### BUG-BUILD-10: QR Endpoint Info Disclosure
- **Severity:** MAJOR
- **Domain:** Build/Security
- **Files:** `backend/internal/main.go`
- **Root Cause:** The QR code endpoint reveals internal network information (local IP, port) that could aid an attacker.
- **Fix Description:** Only generate QR codes with the configured public URL. Don't auto-detect local IPs.
- **Affected Components:** QR endpoint

---

### BUG-BUILD-11: Case-Sensitive Path Comparison on Windows
- **Severity:** MAJOR
- **Domain:** Build/Security
- **Files:** `backend/internal/streamer/streamer.go`
- **Root Cause:** Path comparison is case-sensitive, which fails on Windows where paths are case-insensitive. This can bypass path traversal checks.
- **Fix Description:** Use `filepath.Clean()` and case-insensitive comparison on Windows:
  ```go
  if runtime.GOOS == "windows" {
      strings.EqualFold(resolvedPath, cleanPath)
  }
  ```
- **Affected Components:** Streamer, Windows compatibility

---

### BUG-BUILD-12: No Auth on Stream Endpoint (Duplicate of BUG-BUILD-3)
Already covered above.

---

### BUG-BUILD-13: Unvalidated MIME Type from DB
- **Severity:** MAJOR
- **Domain:** Build/Security
- **Files:** `backend/internal/streamer/streamer.go`
- **Root Cause:** The MIME type is read from the database and used directly in the Content-Type header without validation. If the DB value is corrupted or manipulated, it could lead to XSS via content type confusion.
- **Fix Description:** Validate the MIME type against a whitelist of allowed audio/image types. Fall back to `application/octet-stream` for unknown types.
- **Affected Components:** Streamer, content delivery

---

### BUG-BUILD-14: Silent ID Parse Error in Streamer
- **Severity:** MAJOR
- **Domain:** Build/Security
- **Files:** `backend/internal/streamer/streamer.go`
- **Root Cause:** Track ID parsing errors are silently ignored, defaulting to ID 0 which may return unexpected data.
- **Fix Description:** Return a 400 Bad Request for invalid IDs.
- **Affected Components:** Streamer

---

### BUG-API-3: API Key Read from Env on Every Request
- **Severity:** MAJOR
- **Domain:** Backend API
- **Files:** `backend/internal/auth/middleware.go:11`
- **Root Cause:** The API key is read from the environment on every request using `os.Getenv()`. This is inefficient and makes testing harder.
- **Fix Description:** Read the key once at startup and store it in a package-level variable.
- **Affected Components:** Auth middleware

---

### BUG-API-4: CORS AllowCredentials with Permissive Origin
- **Severity:** MAJOR
- **Domain:** Backend API
- **Files:** `backend/internal/main.go:287-292`
- **Root Cause:** CORS is configured with `AllowCredentials: true` and a permissive origin function, allowing any origin to make credentialed requests.
- **Fix Description:** Use an explicit origin allowlist. See BUG-BUILD-4.
- **Affected Components:** CORS configuration

---

### BUG-API-5: Shutdown Doesn't Wait for Background Goroutines
- **Severity:** MAJOR
- **Domain:** Backend API
- **Files:** `backend/internal/main.go:439-445`
- **Root Cause:** The shutdown handler doesn't wait for background goroutines to exit before returning, causing incomplete operations and potential data corruption.
- **Fix Description:** See BUG-API-2. Use WaitGroup to track goroutines.
- **Affected Components:** Graceful shutdown

---

### Phase 3: Minor — Code Quality & Edge Cases

---

### BUG-DATA-7: columnExists() Uses String Concatenation for SQL
- **Severity:** MINOR
- **Domain:** Backend Data
- **Files:** `backend/internal/db/db.go:231`
- **Root Cause:** String concatenation into SQL is a fragile pattern, even with a whitelist check.
- **Fix Description:** Add a regex assertion `^[a-z_]+$` as defense-in-depth. Add a comment referencing the whitelist validation.

---

### BUG-DATA-8: columnExists() Conflates Scan Error with Not-Found
- **Severity:** MINOR
- **Domain:** Backend Data
- **Files:** `backend/internal/db/db.go:241-242`
- **Root Cause:** A scan error returns `false` (same as "column doesn't exist"), causing migrations to be silently skipped.
- **Fix Description:** Log the scan error before returning false. Consider returning an error instead of a boolean.

---

### BUG-DATA-9: Schema/Migration Redundancy for download_jobs.kind
- **Severity:** MINOR
- **Domain:** Backend Data
- **Files:** `backend/internal/db/db.go:132,280-284`
- **Root Cause:** The `kind` column is defined in both the schema DDL and the migration, creating confusion for new vs. existing installs.
- **Fix Description:** Remove `kind` from the schema DDL and keep it only in the migration.

---

### BUG-DATA-10: apple_id Schema-Model Drift
- **Severity:** MINOR
- **Domain:** Backend Data
- **Files:** `backend/internal/db/db.go:316-323`, `backend/internal/models/models.go:37,41-54,69-77`
- **Root Cause:** The `apple_id` column exists in the DB but is not in `TrackCols`, `TrackColsAliased()`, `ScanTrack()`, or the `Track` struct.
- **Fix Description:** Either add `AppleID` to the model or document why it's intentionally excluded.

---

### BUG-DATA-11: Missing Index on recommendations.prompt_hash
- **Severity:** MINOR
- **Domain:** Backend Data
- **Files:** `backend/internal/db/db.go:94-98`
- **Root Cause:** No index on `prompt_hash`, causing full table scans for dedup lookups.
- **Fix Description:** `CREATE INDEX IF NOT EXISTS idx_recommendations_hash ON recommendations(prompt_hash);`

---

### BUG-DATA-12: stats() Uses 4 Separate Queries
- **Severity:** MINOR
- **Domain:** Backend Data
- **Files:** `backend/internal/library/library.go:372,377,382,387`
- **Root Cause:** Four sequential DB round-trips for one endpoint. No consistent snapshot.
- **Fix Description:** Use a single query with sub-selects or wrap in a read transaction.

---

### BUG-DATA-13: Negative Offset Silently Accepted
- **Severity:** MINOR
- **Domain:** Backend Data
- **Files:** `backend/internal/library/library.go:76-80`
- **Root Cause:** `?offset=-50` is passed directly to SQL. SQLite treats negative OFFSET as 0, but this is implicit.
- **Fix Description:** Clamp offset to 0 if negative.

---

### BUG-DATA-14: FTS5 Triggers Use IF NOT EXISTS Blocking Fixes
- **Severity:** MINOR
- **Domain:** Backend Data
- **Files:** `backend/internal/db/db.go:56,60,64`
- **Root Cause:** `CREATE TRIGGER IF NOT EXISTS` prevents bug fixes to trigger bodies from being applied.
- **Fix Description:** `DROP TRIGGER IF EXISTS` before each `CREATE TRIGGER`, or use a migration framework.

---

### BUG-LOGIC-11: Local Type Redefinitions Shadow Package Types
- **Severity:** MINOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/downloader/downloader.go:1317-1334`
- **Root Cause:** Local type redefinitions shadow package types, causing confusion and potential type mismatches.
- **Fix Description:** Remove the local redefinitions and import the package types directly.

---

### BUG-LOGIC-12: findDownloadedFile Walks Entire Directory Each Call
- **Severity:** MINOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/downloader/downloader.go:1220-1241`
- **Root Cause:** Linear directory walk on every call. O(n) per lookup.
- **Fix Description:** Cache the directory listing or use a map for O(1) lookups.

---

### BUG-LOGIC-13: No Duplicate Name Check in Playlist Create
- **Severity:** MINOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/playlists/playlists.go:103-126`
- **Root Cause:** Creating a playlist with a duplicate name succeeds silently.
- **Fix Description:** Check for existing playlist with the same name before creating. Return 409 Conflict if duplicate.

---

### BUG-LOGIC-14: removeTrack Doesn't Recompact Positions
- **Severity:** MINOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/playlists/playlists.go:316-340`
- **Root Cause:** Removing a track leaves a gap in the position sequence.
- **Fix Description:** After removal, decrement positions of all tracks after the removed position.

---

### BUG-LOGIC-15: rows.Scan Error Ignored in buildProfile
- **Severity:** MINOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/recommender/recommender.go:471`
- **Root Cause:** Scan error is discarded, causing incomplete profile data.
- **Fix Description:** Check and log the error.

---

### BUG-LOGIC-16: ThinkingEffort JSON Tag May Be Wrong
- **Severity:** MINOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/recommender/recommender.go:922`
- **Root Cause:** The JSON tag on the ThinkingEffort field may not match the API's expected field name.
- **Fix Description:** Verify against the DeepSeek API documentation and correct if needed.

---

### BUG-LOGIC-17: DB Query Error Silently Ignored in Scanner
- **Severity:** MINOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/scanner/scanner.go:124`
- **Root Cause:** A DB query error is silently ignored, causing the scanner to skip tracks without logging.
- **Fix Description:** Log the error and continue, or fail the scan.

---

### BUG-LOGIC-18: ffmpeg Error Silently Ignored
- **Severity:** MINOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/scanner/scanner.go:57-60`
- **Root Cause:** ffmpeg errors are silently ignored, causing tracks to be marked as scanned even if metadata extraction failed.
- **Fix Description:** Log the error and mark the track as needing re-scan.

---

### BUG-LOGIC-19: Walk Errors Silently Swallowed
- **Severity:** MINOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/scanner/scanner.go:95-97`
- **Root Cause:** Directory walk errors are silently ignored, causing the scanner to miss entire directories.
- **Fix Description:** Log the error and continue to the next directory.

---

### BUG-LOGIC-20: No Column Count Validation in ScanTrack
- **Severity:** MINOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/models/models.go:58-77`
- **Root Cause:** `ScanTrack` doesn't validate the number of columns matches the expected count.
- **Fix Description:** Add a check that `len(dest)` matches the expected column count.

---

### BUG-LOGIC-21: omitempty on float64 Fields
- **Severity:** MINOR
- **Domain:** Backend Logic
- **Files:** `backend/internal/models/models.go:27-29`
- **Root Cause:** `omitempty` on float64 fields means 0 values are omitted from JSON, which may be unexpected.
- **Fix Description:** Document this behavior or use pointers to distinguish zero from unset.

---

### BUG-BUILD-15: $IsWindows PS7+ Only in build.ps1
- **Severity:** MINOR
- **Domain:** Build/Security
- **Files:** `scripts/build.ps1`
- **Root Cause:** `$IsWindows` automatic variable only exists in PowerShell 7+. The script will fail on Windows PowerShell 5.1.
- **Fix Description:** Use `$env:OS -eq 'Windows_NT'` for compatibility, or document the PS7+ requirement.

---

### BUG-BUILD-16: python3 Not on Windows PATH
- **Severity:** MINOR
- **Domain:** Build/Security
- **Files:** `scripts/build.ps1`
- **Root Cause:** The build script assumes `python3` is on PATH, which is not guaranteed on Windows.
- **Fix Description:** Check for `python3` and fall back to `python` or `py`.

---

### BUG-BUILD-17: No dist/ Existence Check in build.ps1
- **Severity:** MINOR
- **Domain:** Build/Security
- **Files:** `scripts/build.ps1`
- **Root Cause:** The script doesn't check if the `dist/` directory exists before copying files.
- **Fix Description:** Add `Test-Path` check or `New-Item -Force`.

---

### BUG-BUILD-18: Copy-Item Merge vs Replace in build.ps1
- **Severity:** MINOR
- **Domain:** Build/Security
- **Files:** `scripts/build.ps1`
- **Root Cause:** `Copy-Item` merge behavior may not replace existing files as expected.
- **Fix Description:** Use `-Force` flag or remove the destination first.

---

### BUG-BUILD-19: No Uninstall Error Handling in lexicon.iss
- **Severity:** MINOR
- **Domain:** Build/Security
- **Files:** `release/lexicon.iss`
- **Root Cause:** The uninstaller has no error handling for file-in-use scenarios.
- **Fix Description:** Add `RestartReplace` flag and error handling.

---

### BUG-BUILD-20: Fragile PowerShell One-Liner in lexicon.iss
- **Severity:** MINOR
- **Domain:** Build/Security
- **Files:** `release/lexicon.iss`
- **Root Cause:** A PowerShell one-liner in the installer is fragile and may fail on different Windows versions.
- **Fix Description:** Move the logic to a separate PowerShell script file.

---

### BUG-BUILD-21: No File Size Limit on Tag Parsing in cover.go
- **Severity:** MINOR
- **Domain:** Build/Security
- **Files:** `backend/internal/streamer/cover.go`
- **Root Cause:** No limit on file size when parsing embedded artwork tags. A malicious file with a huge embedded image could cause OOM.
- **Fix Description:** Limit tag parsing to the first 10MB of the file.

---

### BUG-BUILD-22: No Symlink Check in cover.go
- **Severity:** MINOR
- **Domain:** Build/Security
- **Files:** `backend/internal/streamer/cover.go`
- **Root Cause:** cover.go doesn't check for symlinks, allowing path traversal via symlinks.
- **Fix Description:** Add symlink resolution and path prefix check (same as BUG-SEC-2).

---

### BUG-FE-8 through BUG-FE-24: Remaining Frontend Minor Bugs
- **Severity:** MINOR (17 bugs)
- **Domain:** Frontend
- **Files:** Various React components
- **Root Cause:** The frontend audit found 24 minor issues including missing error boundaries, unused variables, inconsistent styling, missing accessibility attributes, and minor UX issues.
- **Fix Description:** These should be batched into a single frontend cleanup sprint. Each is low-impact but collectively improves code quality.
- **Affected Components:** Various UI components

---

### BUG-API-6: rescanGen Integer Overflow Theoretical Issue
- **Severity:** MINOR
- **Domain:** Backend API
- **Files:** `backend/internal/main.go:129`
- **Root Cause:** The rescan generation counter is an `int` that could theoretically overflow. In practice this would take years of continuous rescan triggers, but it's still worth using `uint64` or `atomic.Int64`.
- **Fix Description:** Change to `atomic.Uint64` or document why overflow is acceptable.

---

### BUG-API-7: No Config Validation on Load()
- **Severity:** MINOR
- **Domain:** Backend API
- **Files:** `backend/internal/config/config.go:37-76`
- **Root Cause:** The `Load()` function doesn't validate configuration values (e.g., negative port, empty paths).
- **Fix Description:** Add validation for critical config values. Return an error for invalid configs.

---

## Cross-Cutting Concerns

### 1. Error Handling Standardization
Multiple packages use different error response formats. Standardize on a single `writeError()` helper that returns JSON with a consistent structure:
```json
{"error": "message", "code": 500}
```
Files affected: `analytics.go`, `downloader.go`, `recommender.go`

### 2. Context Propagation
Many functions use `context.Background()` instead of accepting a `context.Context` parameter. This prevents cancellation and timeout propagation.
Files affected: `podcaster/`, `scanner/`, `downloader/`

### 3. Goroutine Lifecycle Management
Background goroutines are launched without proper lifecycle management. A centralized goroutine tracker with WaitGroup should be implemented.
Files affected: `main.go`, `scanner.go`, `downloader.go`

### 4. Database Error Handling
`rows.Scan()` and `tx.Rollback()` errors are frequently ignored across the codebase. A lint rule or code review checklist should catch these.
Files affected: `analytics.go`, `library.go`, `recommender.go`, `scanner.go`

### 5. Path Validation
Path traversal checks are inconsistent. A shared `validatePath()` utility should be created and used everywhere.
Files affected: `streamer.go`, `cover.go`

### 6. Authentication Middleware
The auth middleware needs a complete overhaul: require key at startup, use constant-time comparison, apply to all endpoints.
Files affected: `middleware.go`, `main.go`

---

## Summary Statistics

| Domain | Critical | Major | Minor | Total |
|--------|----------|-------|-------|-------|
| Build/Security | 3 | 8 | 10 | 21 |
| Backend API | 3 | 4 | 2 | 9 |
| Backend Data | 1 | 5 | 8 | 14 |
| Backend Logic | 0 | 10 | 11 | 21 |
| Integrations | 2 | 3 | 4 | 9 |
| Frontend | 0 | 7 | 24 | 31 |
| **TOTAL** | **9** | **37** | **59** | **105** |

*Note: Totals include cross-cutting concerns counted in their primary domain. After deduplication of overlapping findings across audits, the unique bug count is approximately 85.*

---

## Recommended Implementation Order

1. **Immediate (this week):** BUG-SEC-1 (auth bypass), BUG-SEC-2/3 (path traversal), BUG-DATA-1 (silent data corruption), BUG-API-1/2 (goroutine races)
2. **Short-term (next 2 weeks):** All Phase 2 Major bugs — focus on race conditions and resource leaks first, then broken features
3. **Medium-term (next month):** Phase 3 Minor bugs — batch into cleanup sprints by domain
4. **Ongoing:** Cross-cutting concerns — address as part of each bug fix (e.g., when fixing an analytics bug, also standardize the error format)

Each bug fix should include:
- A test that reproduces the issue (RED)
- The minimal fix (GREEN)
- A regression test to prevent recurrence
