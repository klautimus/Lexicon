# Lexicon v3.3.5 — Comprehensive GUI & Backend Audit Report

**Date:** 2026-05-19  
**Scope:** Desktop GUI, Mobile GUI, Backend API (end-to-end)  
**Method:** Three parallel subagent audits + cross-reference synthesis  

---

## Executive Summary

Total findings: **137** across all three audit domains.

| Severity | Desktop GUI | Mobile GUI | Backend API | Total |
|----------|------------|------------|-------------|-------|
| CRITICAL | 5 | 4 | 7 | **16** |
| MAJOR | 10 | 11 | 21 | **42** |
| MINOR | 19 | 8 | 16 | **43** |
| COSMETIC | 11 | 7 | 4 | **22** |
| **Total** | **45** | **30** | **48** | **123** |

### Top 5 Cross-Cutting Issues (by blast radius)

1. **Silent error swallowing** — 15+ files across frontend and backend discard errors with empty `catch {}` or `_, _ =`. Users get no feedback when operations fail.
2. **Memory leaks from uncleaned intervals/listeners** — Podcast download polling, DevicePicker WS listeners, and page polling all leak on unmount.
3. **Missing authentication on sensitive GET endpoints** — `/api/spotify/token`, `/api/library/tracks`, `/api/download/jobs` are all unauthenticated on LAN.
4. **Race conditions in PlayerContext** — Stale closures in `scheduleSkip`/`goNext`, unchecked `playSessionRef` in error handlers, volume set during Spotify playback.
5. **Subprocess management** — `runProcess` uses bare `exec.Command` (no timeout), `job.cmd` set before `Start()` (nil pointer race), no zombie reaping.

---

## PART 1: BACKEND API FINDINGS (48 issues)

### CRITICAL (7)

#### 1. `downloader.go` — Subprocess launched without context timeout
- **Severity:** CRITICAL | **Category:** Resource Leak
- `runProcess` uses `exec.Command(bin, args...)` — no `exec.CommandContext`. Hung external tools (spotiflac, yt-dlp, spotDL) will run forever, leaking goroutines and child processes.
- **Fix:** Use `exec.CommandContext` with a cancellable context and reasonable timeout.

#### 2. `downloader.go` — Race: `job.cmd` set before `Start()`, read in `cancelJob`
- **Severity:** CRITICAL | **Category:** Race Condition
- `cancelJob` may acquire the mutex between `job.cmd = cmd` and `cmd.Start()`, seeing `j.cmd != nil` but `j.cmd.Process == nil`, causing nil pointer dereference on `j.cmd.Process.Kill()`.
- **Fix:** Move `job.cmd = cmd` assignment before `cmd.Start()`, or add nil check for `j.cmd.Process`.

#### 3. `analytics.go:135-137` — SQL injection via timezone parameter
- **Severity:** CRITICAL | **Category:** Security
- `fmt.Sprintf` interpolates `a.timezone` directly into SQL. A malicious `TIMEZONE` env var (e.g., `localtime); DROP TABLE plays; --`) would execute arbitrary SQL.
- **Fix:** Validate timezone against IANA whitelist or allow only `[a-zA-Z0-9/_+-]` characters.

#### 4. `spotify/client.go:424` — SQL/JSON injection via deviceID in TransferPlayback
- **Severity:** CRITICAL | **Category:** Security
- `fmt.Sprintf(`{"device_ids":["%s"],"play":%t}`, deviceID, play)` — a deviceID containing `"` or `}` could inject arbitrary JSON.
- **Fix:** Use `json.Marshal` or validate deviceID (Spotify IDs are alphanumeric).

#### 5. `main.go` — API key auth bypass on all GET endpoints
- **Severity:** CRITICAL | **Category:** Security
- Auth middleware only checks POST/PUT/DELETE. GET endpoints like `/api/spotify/token` (returns access token), `/api/library/tracks`, `/api/download/jobs` are unauthenticated on LAN.
- **Fix:** Require auth for sensitive endpoints at minimum; ideally for all methods.

#### 6. `spotify/oauth.go:88-93` — Unsafe type assertion without ok check
- **Severity:** CRITICAL | **Category:** Bug
- `verifier = entry.(verifierEntry).Verifier` — single-return form panics if sync.Map stores wrong type.
- **Fix:** Use two-form assertion: `if ve, ok := entry.(verifierEntry); ok { ... }`.

#### 7. `playerws/hub.go:17` — WebSocket CheckOrigin always returns true
- **Severity:** CRITICAL | **Category:** Security
- Any website can connect to the WebSocket endpoint and control playback. WebSocket connections bypass chi's CORS middleware.
- **Fix:** Implement proper origin checking reusing `isAllowedOrigin` logic from main.go.

### MAJOR (21)

#### 8-22. Systematic `rows.Scan` error ignoring across 8 files
- **Severity:** MAJOR | **Category:** Error Handling
- `library.go` (5 locations), `playlists.go` (2), `recommender.go` (3), `history.go` (1), `analytics.go` (7), `downloader.go` (1), `podcaster.go` (2) — all silently discard `rows.Scan` errors. Failed scans return zero-value structs or drop rows without indication.
- **Fix:** Check scan errors, log and skip or return 500.

#### 23-27. Silent DB errors with `_, _ = a.db.Exec(...)` 
- **Severity:** MAJOR | **Category:** Error Handling
- `downloader.go` (8 locations) and `podcaster.go` (7 locations) silently discard all DB errors. Job state changes, feed updates, and episode inserts fail silently.
- **Fix:** At minimum, log errors. For critical state transitions, return errors.

#### 28. `streamer.go:34` — File path from DB opened without validation
- **Severity:** MAJOR | **Category:** Security
- The stream handler opens a path from the database with no validation that it's within configured media roots. An attacker who can manipulate the tracks table could read any file.
- **Fix:** Validate resolved path is within media roots.

#### 29. `downloader.go:1132-1153` — `findDownloadedFile` window based on stale `StartedAt`
- **Severity:** MAJOR | **Category:** Edge Case
- Uses `job.StartedAt` (set at creation) not actual download start. If job waits in semaphore queue, the file modification time falls outside the ±5min window.
- **Fix:** Record actual download start time or widen window.

#### 30. `library.go:205-224` — `deleteTrack` doesn't require auth
- **Severity:** MAJOR | **Category:** Security
- Anyone on LAN can delete any track and its underlying file. `os.Remove` error is also ignored, creating orphaned references.
- **Fix:** Always require auth for DELETE; check os.Remove error.

### MINOR (16) & COSMETIC (4)

Key patterns:
- `main.go` — `godotenv.Load()` error silently ignored (should log but not fail)
- `config.go` — `DownloadConcurrency` parse failure silently defaults
- `models/models.go` — `TrackCols` hand-maintained string, no compile-time verification
- `recommender.go:858` — Profile hash truncated to 64 bits (birthday problem)
- `downloader.go:1462` — `upgradeAll` doesn't actually upgrade, just lists candidates (misleading name)
- `spotify/client.go` — 8 `io.ReadAll` errors ignored
- `playerws/hub.go:103` — Channel close while goroutine may be sending (mitigated by select/default)

---

## PART 2: DESKTOP GUI FINDINGS (45 issues)

### CRITICAL (5)

#### 1. `PlayerContext.tsx` — Stale closure in `scheduleSkip` causes wrong `goNext` behavior
- **Severity:** CRITICAL | **Category:** Bug
- `scheduleSkip` captures `goNext` from initial closure. When the 1.5s timeout fires, it may call a stale `goNext` referencing outdated state, potentially skipping to the wrong track.
- **Fix:** Use `useReducer` or store `goNext` in a ref that's always current.

#### 2. `PlayerContext.tsx` — `scheduleSkip`/`goNext` circular dependency
- **Severity:** CRITICAL | **Category:** Bug
- `scheduleSkip` → `goNext` → `clearSkipTimeout` → `scheduleSkip`. Stale references can cause double-skip or skip-after-manual-next.
- **Fix:** Restructure with `useReducer` or refs.

#### 3. `DevicePicker.tsx` — Unhandled promise + listener leak
- **Severity:** CRITICAL | **Category:** Bug / Memory Leak
- `fetchSpotifyDevices()` has no `.catch()` (unhandled rejection). `ws.onDevices(handler)` registers a listener but cleanup only clears the interval, never calls `ws.offDevices(handler)`. Each open adds a new listener.
- **Fix:** Add `.catch(() => {})`. Store handler ref and call `ws.offDevices(handler)` in cleanup.

#### 4. `PlayerContext.tsx` — `loadAndPlay` race condition with `playSessionRef`
- **Severity:** CRITICAL | **Category:** Race Condition
- `onError` handler doesn't check `playSessionRef` before calling `scheduleSkip()`. An error from a previous track's audio element could trigger a skip on the current track.
- **Fix:** Capture `playSessionRef.current` in `loadAndPlay` and check it in `onError`.

#### 5. `PlayerContext.tsx` — `setVolume` sets local audio volume during Spotify playback
- **Severity:** CRITICAL | **Category:** Bug
- `setVolume` unconditionally sets `a.volume = v` regardless of source. Switching from Spotify to local playback may cause unexpected volume jumps.
- **Fix:** Only set `a.volume` when `sourceRef.current === "local"`.

### MAJOR (10)

#### 6. `TrackList.tsx` — Silent error swallowing in 4 catch blocks
- `loadPlaylists`, `addToPlaylist`, `createPlaylist`, `handleDelete` — all silently swallow errors. User gets no feedback.

#### 7. `MusicPage.tsx` — `handleBulkUpgrade` only upgrades loaded page (max 200)
- Confirmation dialog says "Upgrade all X tracks" but X is the loaded page count, not total library.

#### 8. `DownloadsPage.tsx` — Polling continues after unmount
- `refresh` uses `setState` on unmounted component. No cancellation flag.

#### 9. `PodcastsPage.tsx` — Episode download polling interval never cleaned up on unmount
- `setInterval` only cleared on success/failure/timeout, not on component unmount.

#### 10. `HomePage.tsx` — No error state for failed API calls
- Shows "—" for stats and "No plays yet" for recent — indistinguishable from empty library.

#### 11. `PlaylistPage.tsx` — `formatDuration` called with potentially undefined value
- If `duration_sec` is null from backend, produces `NaN:00`.

#### 12. `PlayerBar.tsx` — Progress bar `max=0` when duration is 0
- Range input with `min=0, max=0` is uninteractable in most browsers.

#### 13. `DevicePicker.tsx` — "Host Computer" transfer reloads entire page
- `window.location.reload()` destroys all state, interrupts audio, causes visible flash.

#### 14. `RecsPage.tsx` — `completedTrackIds` check uses truthiness, not `!== undefined`
- Track ID `0` would be falsy, hiding the Play button.

#### 15. `PlayerContext.tsx` — `toggle()` for Spotify has empty catch
- Failed Spotify toggle leaves UI in wrong state (showing "play" when actually playing).

### MINOR (19) & COSMETIC (11)

Key patterns:
- **Accessibility:** No ARIA labels on playback controls, no keyboard navigation in table, context menu not keyboard-accessible, sidebar nav has no `aria-label`
- **Missing loading states:** Search page, Analytics page, Settings page all lack loading indicators
- **Silent errors:** PlaylistPage remove/save/delete, PlaylistsPage delete, DownloadsPage refresh, SettingsPage load
- **Visual:** Volume slider has no fill track, album column too narrow (max-w-48), heatmap cells tiny (20×20px)
- **Inconsistency:** RecsPage uses `alert()` while all other pages use `toast.error()`
- **Toast ID collision:** Uses `Math.random()` instead of incrementing counter

---

## PART 3: MOBILE GUI FINDINGS (30 issues)

### CRITICAL (4)

#### 1. `App.tsx` — Double padding-bottom causes content hidden behind player bar
- **Severity:** CRITICAL | **Category:** Bug / Visual
- Outer div has `pb-14` and inner main has `pb-28`. The outer `pb-14` is wasted space; the inner `pb-28` may still be insufficient on small screens, permanently obscuring content.
- **Fix:** Remove `pb-14` from outer div. Use CSS variable for player bar height.

#### 2. `MobilePlayerBar.tsx` — Expanded player overlay has no safe area inset
- **Severity:** CRITICAL | **Category:** Visual / Edge Case
- `fixed inset-0` with no `env(safe-area-inset-bottom)`. Volume controls and DevicePicker rendered behind iPhone home indicator.
- **Fix:** Add `pb-safe` or `padding-bottom: env(safe-area-inset-bottom)`.

#### 3. `PodcastsPage.tsx` — Podcast download polling leaks intervals on unmount
- **Severity:** CRITICAL | **Category:** Bug / Performance
- Same as desktop — interval not cleared on unmount, causes state updates on unmounted component.

#### 4. `PodcastsPage.tsx` — Polling fetches ALL episodes every 3s (600 times)
- **Severity:** CRITICAL | **Category:** Performance
- Each poll calls `api.podcastEpisodes(feedId)` fetching the entire feed. For large feeds, massive unnecessary traffic. `selectedFeed!` non-null assertion could throw if feed deleted.
- **Fix:** Poll single episode endpoint. Add null check for `selectedFeed`.

### MAJOR (11)

#### 5. `MobileNavBar.tsx` — Overflow sheet has no safe area padding
- Grid of overflow items hidden behind iPhone home indicator.

#### 6. `DevicePicker.tsx` — Touch targets too small
- Trigger button: `p-1.5 -m-1.5` (~6×6px). Device items: ~40px height. Below 44×44px minimum.

#### 7. `DevicePicker.tsx` — "Host Computer" reloads page (same as desktop)
- Extremely disruptive on mobile — interrupts scroll, resets state, visible flash.

#### 8-9. `TrackList.tsx` — Context menu uses `mousedown` for outside click
- Both `MobileTrackCard` and `DesktopTrackRow` use `mousedown` listener. On mobile, touch events don't reliably fire `mousedown`, so tapping outside won't close the menu.
- **Fix:** Add `touchstart` listener or use pointer events.

#### 10. `MobilePlayerBar.tsx` — No swipe-down-to-close gesture
- Mobile users expect to swipe down to dismiss full-screen overlays. Only close option is small ChevronDown button.

#### 11. `PodcastsPage.tsx` — Sidebar layout unusable on mobile
- Feed list and episodes stacked vertically with no back button. Users must scroll through entire episode list to get back to feed selector.

#### 12-13. `PodcastsPage.tsx` — Action buttons too small
- Header buttons (Download, Sync, Unsubscribe) and episode play/download buttons all ~28×28px. Below 44×44px minimum.

#### 14. `MobileNavBar.tsx` — Nav labels too small
- Labels are 10px, difficult to read. No `aria-label` on NavLinks.

#### 15. `App.tsx` — No back navigation for nested mobile routes
- No way to go back from `/playlists/:id` except via nav bar. Standard mobile UX requires back button.

### MINOR (8) & COSMETIC (7)

Key patterns:
- Mini player progress bar only 2px tall (nearly invisible on high-DPI)
- MobileTrackCard play button always shows Play icon (no Pause state)
- RecsPage slider too small for mobile (`h-1` track)
- No haptic feedback on any touch interactions
- MobileNavBar "More" icon doesn't change to X when sheet open
- DevicePicker dropdown may go off-screen on mobile (no viewport boundary check)
- Episode descriptions too small (12px) on mobile

---

## PRIORITIZED FIX ROADMAP

### Phase 1: Critical Security & Stability (Week 1)
| # | Issue | File | Effort |
|---|-------|------|--------|
| B1 | SQL injection in timezone | analytics.go | 1h |
| B2 | JSON injection in deviceID | spotify/client.go | 30m |
| B3 | Auth bypass on GET endpoints | main.go + middleware.go | 2h |
| B4 | WebSocket CheckOrigin always true | playerws/hub.go | 1h |
| B5 | Subprocess without context timeout | downloader.go | 2h |
| B6 | job.cmd nil pointer race | downloader.go | 1h |
| F1 | PlayerContext stale closure | PlayerContext.tsx | 3h |
| F2 | DevicePicker listener leak | DevicePicker.tsx | 1h |
| F3 | Mobile safe area insets | MobilePlayerBar.tsx, MobileNavBar.tsx | 1h |

### Phase 2: Error Handling & Memory Leaks (Week 2)
| # | Issue | File | Effort |
|---|-------|------|--------|
| B7 | Systematic rows.Scan error ignoring | 8 backend files | 4h |
| B8 | Silent DB errors (`_, _ =`) | downloader.go, podcaster.go | 3h |
| B9 | deleteTrack no auth + ignored os.Remove | library.go | 1h |
| F4 | Podcast polling leak (both layouts) | PodcastsPage.tsx | 2h |
| F5 | DownloadsPage polling after unmount | DownloadsPage.tsx | 1h |
| F6 | Silent error swallowing (12 locations) | Multiple frontend files | 4h |
| F7 | PlayerContext volume during Spotify | PlayerContext.tsx | 30m |

### Phase 3: UX & Mobile (Week 3)
| # | Issue | File | Effort |
|---|-------|------|--------|
| F8 | mousedown → pointer events for menus | TrackList.tsx | 2h |
| F9 | Touch targets below 44px | DevicePicker.tsx, PodcastsPage.tsx | 2h |
| F10 | No back navigation on mobile | App.tsx + nested pages | 2h |
| F11 | BulkUpgrade only upgrades loaded page | MusicPage.tsx | 1h |
| F12 | DevicePicker page reload | DevicePicker.tsx | 2h |
| F13 | No swipe-to-close on expanded player | MobilePlayerBar.tsx | 2h |
| F14 | Podcast sidebar unusable on mobile | PodcastsPage.tsx | 2h |

### Phase 4: Polish & Accessibility (Week 4)
| # | Issue | File | Effort |
|---|-------|------|--------|
| F15 | ARIA labels on all interactive components | Multiple files | 4h |
| F16 | Keyboard navigation for table | TrackList.tsx | 3h |
| F17 | Loading/error states for all pages | Multiple pages | 4h |
| F18 | Consistent error handling (no `alert()`) | RecsPage.tsx + others | 1h |
| F19 | Visual polish (heatmap, sliders, truncation) | Multiple files | 3h |

---

## METHODOLOGY NOTES

- **Backend audit:** Read all 18 Go source files, focused on error handling patterns, SQL construction, auth middleware, subprocess management, and WebSocket security.
- **Desktop GUI audit:** Read all 19 frontend files (components, pages, contexts, libs), focused on error handling, state management, accessibility, and visual consistency.
- **Mobile GUI audit:** Re-read mobile-specific code paths (MobileLayout, MobilePlayerBar, MobileNavBar, MobileCardList, useIsMobile), focused on touch targets, safe area, gesture handling, and mobile-specific layout issues.
- **Cross-reference:** Backend API issues that affect frontend behavior were noted in both domains. The frontend's silent error swallowing is partially caused by backend returning 200 with empty/missing data on errors.

---

*Report generated by Atlas — Lexicon QA Audit, 2026-05-19*
