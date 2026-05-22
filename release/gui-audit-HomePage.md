# GUI Audit: HomePage

**Date:** 2026-05-22
**Auditor:** Atlas (researcher)
**Scope:** HomePage.tsx and all supporting frontend files referenced in task t_8ad6fdbd
**Files reviewed:**
- `frontend/src/pages/HomePage.tsx` (220 lines)
- `frontend/src/App.tsx` (283 lines)
- `frontend/src/player/PlayerContext.tsx` (688 lines)
- `frontend/src/contexts/DownloadContext.tsx` (374 lines)
- `frontend/src/contexts/ToastContext.tsx` (93 lines)
- `frontend/src/contexts/UserContext.tsx` (72 lines)
- `frontend/src/contexts/HelpContext.tsx` (45 lines)
- `frontend/src/lib/api.ts` (460 lines)
- `frontend/src/components/PlayerBar.tsx` (154 lines)
- `frontend/src/components/MobilePlayerBar.tsx` (203 lines)
- `frontend/src/components/DevicePicker.tsx` (173 lines)
- `frontend/src/components/DownloadProgressBar.tsx` (120 lines)
- `frontend/src/components/MobileNavBar.tsx` (141 lines)
- `frontend/src/components/TrackList.tsx` (584 lines)
- `frontend/src/components/HelpModal.tsx` (79 lines)
- `frontend/src/help-content.ts` (420 lines)
- `frontend/src/index.css` (28 lines)
- `frontend/src/pages/LoginPage.tsx` (131 lines)

---

## CRITICAL BUGS (Fix Immediately)

### BUG-1: Error states are dead code — stats/recent errors NEVER display
**Severity:** CRITICAL
**File:** `frontend/src/pages/HomePage.tsx` lines 19-20
**Detail:** The `useEffect` at lines 18-30 catches errors with empty `.catch(() => {})` blocks. The error state variables `statsError` (line 10) and `recentError` (line 11) are initialized to `false` and are **never** set to `true`. The error UI rendering at lines 157-160 and 189-192 is therefore unreachable dead code — users always see a blank/loading state on API failure instead of a helpful error with retry button.

Compare with the retry button handlers at lines 159 and 191 which reference `setStatsError` and `setRecentError`, but those setters are never called on the failure path.

**Fix:** Change lines 19-20 from:
```ts
api.stats().then(setStats).catch(() => {});
api.recent().then(setRecent).catch(() => {});
```
to:
```ts
api.stats().then(setStats).catch(() => setStatsError(true));
api.recent().then(setRecent).catch(() => setRecentError(true));
```

### BUG-2: RFC 1918 private range check is incomplete for 172.x.x.x
**Severity:** MEDIUM
**File:** `frontend/src/pages/HomePage.tsx` line 125
**Detail:** The condition `window.location.hostname.startsWith("172.")` matches the entire `172.0.0.0/8` range. Per RFC 1918, only `172.16.0.0/12` (172.16.x.x – 172.31.x.x) is private. Addresses like `172.32.1.1` or `172.99.0.1` are public IPs but would pass the check and trigger the wrong warning message at line 127.

**Fix:** Replace the `startsWith("172.")` check with a proper RFC 1918 validation function or at minimum:
```ts
const h = window.location.hostname;
const is172Private = h.startsWith("172.") && (() => {
  const secondOctet = parseInt(h.split(".")[1], 10);
  return secondOctet >= 16 && secondOctet <= 31;
})();
```

### BUG-3: Duplicate track keys in recent plays list
**Severity:** MEDIUM
**File:** `frontend/src/pages/HomePage.tsx` line 196
**Detail:** `key={r.id}` uses the play event ID. If the same track appears twice consecutively, React will see duplicate keys, potentially causing rendering artifacts. The `r.id` is the play history row ID, which should be unique — but this is fragile if the API ever returns duplicate entries. Safer to use index as fallback: `key={`${r.id}-${i}`}`.

---

## MISSING FEATURES (Prioritized)

### FEAT-1: No loading skeleton for stats section
**Priority:** HIGH
**File:** `frontend/src/pages/HomePage.tsx` lines 162-168
**Detail:** While stats are loading, `stats` is `null` and the UI shows "—" for all values. This creates a jarring flash of empty content. A skeleton/shimmer loading state (4 animated placeholder cards) would dramatically improve perceived performance.

### FEAT-2: No loading skeleton for recent plays section  
**Priority:** HIGH
**File:** `frontend/src/pages/HomePage.tsx` lines 182-207
**Detail:** Same issue — the recent plays section appears empty during load. Should show 5-6 skeleton rows.

### FEAT-3: No personalization — hardcoded "Welcome back"
**Priority:** MEDIUM
**File:** `frontend/src/pages/HomePage.tsx` line 140
**Detail:** Always shows "Welcome back" regardless of user, time of day, or state. With the auth system (UserContext.tsx line 6-7 has `user` with `display_name`/`username`), this could say "Welcome back, Kevin" or use time-of-day greeting ("Good morning", "Good evening").

### FEAT-4: No total listening time in stats
**Priority:** MEDIUM
**Detail:** The stats cards show track/album/artist/podcast counts but no total listening time (hours listened). The backend `overview` endpoint already returns `listen_sec`. This is a key engagement metric for music apps.

### FEAT-5: Recently played items are not clickable/playable
**Priority:** MEDIUM
**File:** `frontend/src/pages/HomePage.tsx` lines 194-206
**Detail:** The recent plays list shows track name + artist + timestamp, but clicking any item does nothing. It should navigate to the track or start playback. Compare with MusicPage where clicking a track plays it.

### FEAT-6: No "Recently Added" section
**Priority:** MEDIUM
**Detail:** Music dashboards typically show "Recently Added" alongside "Recently Played". The backend supports this via library queries sorted by `added_at`. This is a discovery feature that helps users find content they just downloaded.

### FEAT-7: Hardcoded "10 items" limit with no "Show All"
**Priority:** MEDIUM
**File:** `frontend/src/pages/HomePage.tsx` line 195
**Detail:** `recent.slice(0, 10)` truncates with no way to see more. Should either link to Analytics page or have an expandable section. The backend `recent` endpoint has no limit param documented.

### FEAT-8: No quick-access playlists or "Jump to" shortcuts
**Priority:** LOW
**Detail:** Power users benefit from pinned/favorite playlists or frequently-accessed albums on the home page. Currently the only way to reach playlists is through the sidebar.

### FEAT-9: QR code panel has no loading state
**Priority:** LOW
**File:** `frontend/src/pages/HomePage.tsx` lines 56-60
**Detail:** The QR `<img>` loads from `/api/qr` but has no loading indicator or error fallback. If the endpoint is slow, users see a broken image icon briefly.

### FEAT-10: No pull-to-refresh or manual refresh on home page
**Priority:** LOW
**Detail:** Data loads once on mount (line 18, `useEffect` with `[]` deps). If the user is on the home page while downloads complete, stats won't update. There's no refresh button (unlike MusicPage's Load More). A simple refresh button or auto-refresh on navigation would help.

---

## POOR IMPLEMENTATIONS

### IMPL-1: Stat component redefined every render
**File:** `frontend/src/pages/HomePage.tsx` lines 213-220
**Detail:** The `Stat` component is defined inside the module scope but outside the component function, which is fine. However, it accepts `value: number | string` and renders it directly. This works but the typing should be tighter — the skeleton state sends `undefined` cast as `"—"` which is a string. Not a bug, but fragile.

### IMPL-2: Timezone-unaware date formatting
**File:** `frontend/src/pages/HomePage.tsx` line 202
**Detail:** `new Date(r.started_at * 1000).toLocaleString()` uses the browser's locale settings. With the auth system adding users in different timezones, and the server in PDT (UTC-7), there's no consistency guarantee. The backend stores `started_at` as unix timestamp — the frontend should at minimum show relative time ("2 hours ago") using something like `date-fns formatDistanceToNow` or a simple inline calculation.

### IMPL-3: QR panel state managed with 5 separate useState calls
**File:** `frontend/src/pages/HomePage.tsx` lines 12-16
**Detail:** `showQr`, `localUrl`, `networkInfo`, `copied`, `showNetworkHelp` — five separate `useState` calls for what is essentially one UI panel's state. Could be consolidated into a single `useReducer` or at least grouped. This makes the component harder to follow and more prone to stale-state bugs.

### IMPL-4: Network info help uses hardcoded English warning messages
**File:** `frontend/src/pages/HomePage.tsx` lines 116-130
**Detail:** Several warning/help strings are hardcoded English paragraphs inside JSX. These should be extracted to constants or the help system for consistency and potential i18n support.

---

## VISUAL / UI ISSUES

### VIS-1: No responsive scaling for logo/heading section
**File:** `frontend/src/pages/HomePage.tsx` lines 137-143
**Detail:** `text-3xl` heading on line 140 has no `md:` responsive variant. On mobile it may be too large relative to the icon (line 138: `w-10 h-10`). The heading and subtitle should scale down on small screens.

### VIS-2: QR dismiss button lacks visual affordance
**File:** `frontend/src/pages/HomePage.tsx` line 88-94
**Detail:** The dismiss button (`<X size={16} />`) has `className "flex-shrink-0 p-1 text-muted hover:text-text"` — no background, no border, just a small X icon. Very easy to miss on a large panel. Should have a more visible hover state or a background circle.

### VIS-3: Stats grid cramped on mobile
**File:** `frontend/src/pages/HomePage.tsx` line 162
**Detail:** `grid grid-cols-2 md:grid-cols-4 gap-4` — on mobile, 2-column grid with `gap-4` and `p-4` cards can feel cramped, especially on phones < 375px wide. Consider `gap-2` on mobile.

### VIS-4: No visual separation between page sections
**File:** `frontend/src/pages/HomePage.tsx` line 51
**Detail:** The outer container has `space-y-8` which is good, but there's no visual section headers/dividers. The QR section, Stats, and Recent Plays blur together slightly. Subtle dividers or more varied spacing would help.

### VIS-5: Recent plays timestamp styling is too subtle
**File:** `frontend/src/pages/HomePage.tsx` lines 201-203
**Detail:** `text-muted text-xs` for timestamps — nearly invisible against the panel background. Consider a slightly brighter color or right-aligning with proper tabular numbers.

---

## ACCESSIBILITY ISSUES

### A11Y-1: Dismiss button missing aria-label
**File:** `frontend/src/pages/HomePage.tsx` line 88-94
**Detail:** `<button onClick={() => setShowQr(false)} ... title="Dismiss">` — has `title` but no `aria-label`. Screen readers will announce this only as "button". Fix: add `aria-label="Dismiss connection banner"`.

### A11Y-2: Stats cards lack semantic structure
**File:** `frontend/src/pages/HomePage.tsx` lines 213-220
**Detail:** Stats are plain `<div>` elements with `text-xs text-muted uppercase tracking-wide` labels and `text-2xl font-semibold` values. They should use `<dl>/<dt>/<dd>` or at minimum `role="group"` with `aria-label` per stat card for screen reader context.

### A11Y-3: Help buttons are tiny touch targets
**File:** `frontend/src/pages/HomePage.tsx` lines 149-155, 174-180 and `HelpButton.tsx` line 67-78
**Detail:** Help buttons use `p-1` (4px padding) with a 14px icon. Touch target is roughly 22px × 22px, below the WCAG 2.5.5 minimum of 44×44px. The standalone `HelpButton` component (HelpModal.tsx line 72) is 20×20px (`w-5 h-5`), also below minimum.

### A11Y-4: Recent plays list items not interactive but appear to be
**File:** `frontend/src/pages/HomePage.tsx` lines 194-206
**Detail:** The `<li>` items have no `tabIndex`, no `role="button"`, and no click handler. If they remain non-interactive, they should at least have `cursor-default` to avoid confusion. If made interactive (see FEAT-5), they need proper keyboard handling.

### A11Y-5: QR code image alt text is okay but could be more descriptive
**File:** `frontend/src/pages/HomePage.tsx` line 58
**Detail:** `alt="QR code for mobile connection"` is acceptable but could be more actionable: `alt="Scan this QR code to connect a mobile device to Lexicon"`.

---

## PERFORMANCE ISSUES

### PERF-1: copyUrl callback recreated on every render
**File:** `frontend/src/pages/HomePage.tsx` lines 32-48
**Detail:** The `copyUrl` function is defined inside the component body without `useCallback`. It's only called from the clipboard button, so it re-renders needlessly. Should be wrapped in `useCallback`.

### PERF-2: No memoization of Stat component renders
**File:** `frontend/src/pages/HomePage.tsx` lines 213-220
**Detail:** `Stat` is a simple component, but it re-renders whenever parent state changes (even irrelevant state like QR panel). This is negligible with only 4 instances, but using `React.memo(Stat)` would be proper practice.

### PERF-3: Stats and recent queried on every mount, no caching
**File:** `frontend/src/pages/HomePage.tsx` lines 18-20
**Detail:** `api.stats()` and `api.recent()` fire on every page mount. Since the user navigates back to Home from other pages frequently, this creates unnecessary API calls. The `DownloadProvider` already proves cross-route state lifting works — stats could be similarly lifted.

### PERF-4: No code splitting for HomePage
**Detail:** HomePage is imported directly in App.tsx (line 30). For a page that's only visible at `/`, `React.lazy()` + `Suspense` would reduce initial bundle parsing time. However, since it's the landing page, this is a low priority.

---

## CROSS-FILE OBSERVATIONS (Related Systems)

### AUTH-GUARD: Login page accessible via direct URL but /login route has no mobile layout
**File:** `frontend/src/App.tsx` lines 253-254
**Detail:** `/login` is rendered outside `AuthGuard` and outside both `DesktopLayout`/`MobileLayout`. The LoginPage (131 lines) uses its own full-screen layout. This is intentional but means no mobile nav bar, no user context, no help system on login — worth noting as a design choice.

### USER-CONTEXT: Session validation on mount is good but lacks retry
**File:** `frontend/src/contexts/UserContext.tsx` lines 21-43
**Detail:** On mount, validates saved token against `/auth/me`. If the server is temporarily unreachable (e.g., during restart), the token is cleared and user is logged out. No retry or offline handling. For a local desktop app this is acceptable but could be more resilient.

### HELP-CONTENT: home.qr entry exists but isn't linked from HomePage
**File:** `frontend/src/help-content.ts` lines 59-72
**Detail:** There's a `home.qr` help entry with good content, but the QR network info section (HomePage.tsx lines 96-132) doesn't use it. The "Connection help" button at line 79-85 toggles a raw JSX section instead of using the help system's structured modal. Inconsistent with the rest of the app's help pattern.

---

## PRIORITIZED FIX ROADMAP

### Phase 1: Critical Bugs (this sprint)
1. **BUG-1** — Fix error state handling in HomePage (stats/recent `.catch()`)
2. **BUG-2** — Fix RFC 1918 range check for 172.x.x.x
3. **A11Y-3** — Increase help button touch targets to 44×44px minimum

### Phase 2: High-Value Features (next sprint)
1. **FEAT-1** — Add loading skeletons for stats cards
2. **FEAT-2** — Add loading skeletons for recent plays
3. **FEAT-5** — Make recent plays items clickable/playable
4. **FEAT-3** — Personalize welcome greeting with user name
5. **A11Y-1** — Add aria-label to QR dismiss button

### Phase 3: Polish (subsequent sprints)
1. **FEAT-4** — Add total listening time stat card
2. **FEAT-6** — Add "Recently Added" section
3. **FEAT-7** — Add "Show All" for recent plays or link to Analytics
4. **IMPL-2** — Use relative timestamps for recent plays
5. **VIS-1-5** — Visual polish items
6. **PERF-3** — Lift stats state to avoid re-fetching on navigation
