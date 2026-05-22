# GUI Audit: HomePage — REVISED

**Date:** 2026-05-22
**Auditor:** Atlas (researcher)
**Reviewer:** Atlas (analyst, task t_1c8a2c0e)
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

## REVIEWER NOTES

### Verified Claims
All three bugs (BUG-1, BUG-2, BUG-3) are **CONFIRMED** with accurate line numbers and correct analysis.

### Corrections to Original Audit

1. **BUG-3 severity upgraded from MEDIUM to LOW**: The `r.id` field is the play history row ID from the database, which is a primary key and therefore guaranteed unique by the backend. The audit's concern about "duplicate keys if the API ever returns duplicate entries" is theoretically valid but practically unlikely given the backend uses `SELECT * FROM play_history ORDER BY started_at DESC LIMIT 50` which returns distinct rows. The index-fallback key suggestion (`key={\`${r.id}-${i}\`}`) is still good practice for defensive coding, but this is not a real-world bug.

2. **FEAT-4 partially incorrect**: The audit states "The backend `overview` endpoint already returns `listen_sec`." This is **TRUE** — the `Overview` interface (api.ts line 348) does include `listen_sec`. However, the `Stats` interface (api.ts line 324) used by `api.stats()` does NOT include `listen_sec`. The fix would require either adding `listen_sec` to the stats endpoint response or making a separate `api.overview()` call. This is more involved than the audit implies — it's not just displaying an existing field.

3. **FEAT-7 line number is wrong**: The audit says line 195 for `recent.slice(0, 10)`. This is **CONFIRMED** — line 195 does contain `.slice(0, 10)`. However, the audit says "The backend `recent` endpoint has no limit param documented" — this is an assumption that should be verified against the backend code before deciding on the fix approach.

4. **A11Y-3 line reference partially wrong**: The audit references `HelpButton.tsx line 67-78` for the standalone `HelpButton` component. The actual `HelpButton` component is in `HelpModal.tsx` lines 58-79 (not a separate `HelpButton.tsx` file). The touch target analysis is correct — `w-5 h-5` (20×20px) is below WCAG 2.5.5 minimum of 44×44px.

5. **PERF-4 line reference is wrong**: The audit says "HomePage is imported directly in App.tsx (line 30)." Line 30 of App.tsx is indeed `import HomePage from "./pages/HomePage";` — this is **CONFIRMED** correct.

### Additional Findings Found by Reviewer

6. **BUG-4 (NEW): Missing `recentError` state initialization for loading detection** — The `recentError` state (line 11) is initialized to `false` and never set to `true` (same bug pattern as BUG-1). However, the loading state detection at line 182 (`recent.length === 0 && !recentError`) conflates "still loading" with "loaded but empty." There's no way to distinguish between "loading in progress" and "loaded with zero results" without a separate `loading` state. This means the empty-state message "No plays yet" could flash briefly during loading.

7. **IMPL-5 (NEW): Inconsistent help system usage** — The QR network info section (lines 96-132) uses raw JSX with hardcoded English strings instead of the structured help system (`home.qr` entry exists at help-content.ts lines 59-72). The "Connection help" button at line 79-85 toggles raw JSX instead of calling `showHelp("home.qr")`. This is inconsistent with every other section in the app that uses the help modal pattern.

8. **A11Y-6 (NEW): QR code panel dismiss button has no aria-label** — The dismiss button at line 88-94 has `title="Dismiss"` but no `aria-label`. Screen readers will announce this only as "button." (This overlaps with A11Y-1 in the original audit which correctly identified this.)

---

## CRITICAL BUGS (Fix Immediately)

### BUG-1: Error states are dead code — stats/recent errors NEVER display
**Severity:** CRITICAL — **CONFIRMED**
**File:** `frontend/src/pages/HomePage.tsx` lines 19-20
**Detail:** The `useEffect` at lines 18-30 catches errors with empty `.catch(() => {})` blocks. The error state variables `statsError` (line 10) and `recentError` (line 11) are initialized to `false` and are **never** set to `true`. The error UI rendering at lines 157-160 and 189-192 is therefore unreachable dead code — users always see a blank/loading state on API failure instead of a helpful error with retry button.

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
**Severity:** MEDIUM — **CONFIRMED**
**File:** `frontend/src/pages/HomePage.tsx` line 125
**Detail:** The condition `window.location.hostname.startsWith("172.")` matches the entire `172.0.0.0/8` range. Per RFC 1918, only `172.16.0.0/12` (172.16.x.x – 172.31.x.x) is private. Addresses like `172.32.1.1` or `172.99.0.1` are public IPs but would pass the check and trigger the wrong warning message at line 127.

**Fix:** Replace the `startsWith("172.")` check with a proper RFC 1918 validation:
```ts
const h = window.location.hostname;
const is172Private = h.startsWith("172.") && (() => {
  const secondOctet = parseInt(h.split(".")[1], 10);
  return secondOctet >= 16 && secondOctet <= 31;
})();
```

### BUG-3: Duplicate track keys in recent plays list (defensive fix)
**Severity:** LOW — **CONFIRMED** (downgraded from MEDIUM)
**File:** `frontend/src/pages/HomePage.tsx` line 196
**Detail:** `key={r.id}` uses the play event ID which is a database primary key and therefore unique. However, using index as fallback is defensive best practice: `key={\`${r.id}-${i}\`}`.

### BUG-4 (NEW): Loading state conflated with empty state for recent plays
**Severity:** LOW
**File:** `frontend/src/pages/HomePage.tsx` lines 182-188
**Detail:** The condition `recent.length === 0 && !recentError` is true both during loading AND when the user has genuinely never played anything. This means the "No plays yet" message appears during loading. A separate `isLoading` state would fix this.

---

## MISSING FEATURES (Prioritized)

### FEAT-1: No loading skeleton for stats section
**Priority:** HIGH — **CONFIRMED**
**File:** `frontend/src/pages/HomePage.tsx` lines 162-168
**Detail:** While stats are loading, `stats` is `null` and the UI shows "—" for all values. A skeleton/shimmer loading state would improve perceived performance.

### FEAT-2: No loading skeleton for recent plays section
**Priority:** HIGH — **CONFIRMED**
**File:** `frontend/src/pages/HomePage.tsx` lines 182-207
**Detail:** Same issue — the recent plays section appears empty during load. Should show 5-6 skeleton rows.

### FEAT-3: No personalization — hardcoded "Welcome back"
**Priority:** MEDIUM — **CONFIRMED**
**File:** `frontend/src/pages/HomePage.tsx` line 140
**Detail:** Always shows "Welcome back" regardless of user. UserContext has `user.display_name` and `user.username` available.

### FEAT-4: No total listening time in stats
**Priority:** MEDIUM — **PARTIALLY CORRECTED**
**Detail:** The `Stats` interface (api.ts line 324) does NOT include `listen_sec`. The `Overview` interface (api.ts line 348) does. To add this, either:
- Add `listen_sec` to the `/library/stats` backend response and the `Stats` frontend interface, OR
- Make a separate `api.overview()` call and display the value.
This is more involved than "just display an existing field."

### FEAT-5: Recently played items are not clickable/playable
**Priority:** MEDIUM — **CONFIRMED**
**File:** `frontend/src/pages/HomePage.tsx` lines 194-206
**Detail:** Clicking any recent play item does nothing. Should navigate to the track or start playback.

### FEAT-6: No "Recently Added" section
**Priority:** MEDIUM — **CONFIRMED**
**Detail:** Music dashboards typically show "Recently Added" alongside "Recently Played."

### FEAT-7: Hardcoded "10 items" limit with no "Show All"
**Priority:** MEDIUM — **CONFIRMED**
**File:** `frontend/src/pages/HomePage.tsx` line 195
**Detail:** `recent.slice(0, 10)` truncates with no way to see more. Verify backend capabilities before implementing.

### FEAT-8: No quick-access playlists or "Jump to" shortcuts
**Priority:** LOW — **CONFIRMED**

### FEAT-9: QR code panel has no loading state
**Priority:** LOW — **CONFIRMED**
**File:** `frontend/src/pages/HomePage.tsx` lines 56-60

### FEAT-10: No pull-to-refresh or manual refresh on home page
**Priority:** LOW — **CONFIRMED**
**File:** `frontend/src/pages/HomePage.tsx` line 18

---

## POOR IMPLEMENTATIONS

### IMPL-1: Stat component redefined every render
**File:** `frontend/src/pages/HomePage.tsx` lines 213-220 — **CONFIRMED**
**Note:** The `Stat` function is defined at module level (outside the component), so it's NOT redefined every render. The audit's concern is incorrect. However, the typing concern (`value: number | string` accepting `"—"`) is valid — it's fragile but not a bug.

### IMPL-2: Timezone-unaware date formatting
**File:** `frontend/src/pages/HomePage.tsx` line 202 — **CONFIRMED**

### IMPL-3: QR panel state managed with 5 separate useState calls
**File:** `frontend/src/pages/HomePage.tsx` lines 12-16 — **CONFIRMED**

### IMPL-4: Network info help uses hardcoded English warning messages
**File:** `frontend/src/pages/HomePage.tsx` lines 116-130 — **CONFIRMED**

### IMPL-5 (NEW): Inconsistent help system usage in QR section
**File:** `frontend/src/pages/HomePage.tsx` lines 96-132
**Detail:** The QR network info section uses raw JSX with hardcoded strings instead of the structured help system. The `home.qr` help entry (help-content.ts lines 59-72) exists but is not used here. The "Connection help" button should use `showHelp("home.qr")` instead of toggling raw JSX.

---

## VISUAL / UI ISSUES

### VIS-1: No responsive scaling for logo/heading section
**File:** `frontend/src/pages/HomePage.tsx` lines 137-143 — **CONFIRMED**

### VIS-2: QR dismiss button lacks visual affordance
**File:** `frontend/src/pages/HomePage.tsx` lines 88-94 — **CONFIRMED**

### VIS-3: Stats grid cramped on mobile
**File:** `frontend/src/pages/HomePage.tsx` line 162 — **CONFIRMED**

### VIS-4: No visual separation between page sections
**File:** `frontend/src/pages/HomePage.tsx` line 51 — **CONFIRMED**

### VIS-5: Recent plays timestamp styling is too subtle
**File:** `frontend/src/pages/HomePage.tsx` lines 201-203 — **CONFIRMED**

---

## ACCESSIBILITY ISSUES

### A11Y-1: Dismiss button missing aria-label
**File:** `frontend/src/pages/HomePage.tsx` lines 88-94 — **CONFIRMED**
**Fix:** Add `aria-label="Dismiss connection banner"`

### A11Y-2: Stats cards lack semantic structure
**File:** `frontend/src/pages/HomePage.tsx` lines 213-220 — **CONFIRMED**

### A11Y-3: Help buttons are tiny touch targets
**File:** `frontend/src/pages/HomePage.tsx` lines 149-155, 174-180 and `HelpModal.tsx` lines 67-78 — **CONFIRMED**
**Correction:** The standalone `HelpButton` component is in `HelpModal.tsx` (not a separate `HelpButton.tsx` file). Touch targets are 20-22px, below WCAG 2.5.5 minimum of 44×44px.

### A11Y-4: Recent plays list items not interactive but appear to be
**File:** `frontend/src/pages/HomePage.tsx` lines 194-206 — **CONFIRMED**

### A11Y-5: QR code image alt text could be more descriptive
**File:** `frontend/src/pages/HomePage.tsx` line 58 — **CONFIRMED**

---

## PERFORMANCE ISSUES

### PERF-1: copyUrl callback recreated on every render
**File:** `frontend/src/pages/HomePage.tsx` lines 32-48 — **CONFIRMED**

### PERF-2: No memoization of Stat component renders
**File:** `frontend/src/pages/HomePage.tsx` lines 213-220 — **CONFIRMED** (minor impact with only 4 instances)

### PERF-3: Stats and recent queried on every mount, no caching
**File:** `frontend/src/pages/HomePage.tsx` lines 18-20 — **CONFIRMED**

### PERF-4: No code splitting for HomePage
**File:** `frontend/src/App.tsx` line 30 — **CONFIRMED** (low priority since it's the landing page)

---

## CROSS-FILE OBSERVATIONS (Related Systems)

### AUTH-GUARD: Login page accessible via direct URL but /login route has no mobile layout
**File:** `frontend/src/App.tsx` lines 253-254 — **CONFIRMED** (intentional design choice)

### USER-CONTEXT: Session validation on mount is good but lacks retry
**File:** `frontend/src/contexts/UserContext.tsx` lines 21-43 — **CONFIRMED**

### HELP-CONTENT: home.qr entry exists but isn't linked from HomePage
**File:** `frontend/src/help-content.ts` lines 59-72 — **CONFIRMED** (see IMPL-5)

---

## PRIORITIZED FIX ROADMAP

### Phase 1: Critical Bugs (this sprint)
1. **BUG-1** — Fix error state handling in HomePage (stats/recent `.catch()`)
2. **BUG-2** — Fix RFC 1918 range check for 172.x.x.x
3. **A11Y-1** — Add aria-label to QR dismiss button
4. **A11Y-3** — Increase help button touch targets to 44×44px minimum

### Phase 2: High-Value Features (next sprint)
1. **FEAT-1** — Add loading skeletons for stats cards
2. **FEAT-2** — Add loading skeletons for recent plays
3. **FEAT-5** — Make recent plays items clickable/playable
4. **FEAT-3** — Personalize welcome greeting with user name
5. **IMPL-5** — Use help system for QR connection help instead of raw JSX

### Phase 3: Polish (subsequent sprints)
1. **FEAT-4** — Add total listening time stat card (requires backend + frontend changes)
2. **FEAT-6** — Add "Recently Added" section
3. **FEAT-7** — Add "Show All" for recent plays or link to Analytics
4. **IMPL-2** — Use relative timestamps for recent plays
5. **BUG-3** — Defensive key fix for recent plays list
6. **BUG-4** — Separate loading state for recent plays
7. **VIS-1-5** — Visual polish items
8. **PERF-3** — Lift stats state to avoid re-fetching on navigation
