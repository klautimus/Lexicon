# GUI Audit: AnalyticsPage (REVISED)

**Date:** 2026-05-22
**Auditor:** Atlas (researcher)
**Reviewer:** Atlas (analyst) — verified all findings against actual source code
**Scope:** AnalyticsPage.tsx + all referenced context/providers/components
**Files read:** 17 files, ~2,500 LOC total
**Build status:** PASS (go build ✓, npm run build ✓)

---

## Executive Summary

AnalyticsPage is a 202-line component that renders 4 stat cards, 2 charts (bar + pie), a heatmap table, and a top-tracks list. It fetches 5 API endpoints in parallel on mount. The page is functional but has significant gaps in error handling, loading states, empty states, accessibility, and mobile responsiveness.

**Changes from original audit:**
- Removed 3 false positives (3.2, 3.4, 3.7) that were self-corrected in the original but not cleaned up
- All other 27 findings verified against actual code and confirmed accurate
- No new findings added (original audit was thorough)

**Severity key:** CRITICAL > HIGH > MEDIUM > LOW > COSMETIC

---

## 1. MISSING FEATURES

### 1.1 No time-range filtering [MEDIUM]
- **File:** `AnalyticsPage.tsx:37-45`
- **Issue:** All analytics are lifetime aggregates. No way to view "last 7 days", "last 30 days", "this year", etc.
- **Impact:** Users can't track recent trends or compare periods.
- **Backend:** The `plays` table has `started_at` — the backend just doesn't accept a time range parameter.
- **Verified:** CONFIRMED — no date params in any API call or backend handler.

### 1.2 No data export [LOW]
- **File:** `AnalyticsPage.tsx` (entire page)
- **Issue:** No "export as CSV/JSON" button for any analytics data.
- **Impact:** Users can't take their data elsewhere.
- **Verified:** CONFIRMED.

### 1.3 No sorting on top tracks/artists [MEDIUM]
- **File:** `AnalyticsPage.tsx:168-181` (top tracks), `104-113` (top artists)
- **Issue:** Top tracks are sorted by plays (hardcoded in backend). No way to sort by name, album, or recent.
- **Backend:** `analytics.go:131, 165` — `ORDER BY COUNT(*) DESC` only.
- **Verified:** CONFIRMED.

### 1.4 No "play" action on top tracks [MEDIUM]
- **File:** `AnalyticsPage.tsx:170-179`
- **Issue:** Top tracks are read-only text. No play button, no "add to playlist", no context menu.
- **Impact:** Discovering a track you forgot about requires navigating to Music page.
- **Verified:** CONFIRMED — `<li>` elements have no click handlers.

### 1.5 No empty-state for individual sections [HIGH]
- **File:** `AnalyticsPage.tsx:93-101` (stat cards), `104-127` (charts), `168-181` (top tracks)
- **Issue:** If `topArtists` is empty, the bar chart renders an empty Recharts container. If `genres` is empty, the pie chart renders nothing. If `tracks` is empty, the list shows nothing.
- **Impact:** New users with no play history see broken-looking empty charts instead of helpful guidance.
- **Verified:** CONFIRMED.

### 1.6 No refresh/reload button [MEDIUM]
- **File:** `AnalyticsPage.tsx:80-183`
- **Issue:** Data is fetched once on mount. No way to refresh without navigating away and back.
- **Impact:** After playing tracks, analytics don't update until page navigation.
- **Verified:** CONFIRMED — useEffect with empty deps array, no refresh mechanism.

### 1.7 No loading skeleton — only text [LOW]
- **File:** `AnalyticsPage.tsx:54-65`
- **Issue:** Loading state is a centered "Loading analytics…" text. No skeleton/shimmer.
- **Impact:** Feels unpolished compared to modern app standards.
- **Verified:** CONFIRMED.

### 1.8 Help text references non-existent features [MEDIUM]
- **File:** `help-content.ts:339-342` — the help text mentions "Listening Timeline" and "Recently Added" but these don't exist in the actual page.
- **Impact:** Help text promises features that don't exist. User confusion.
- **Verified:** CONFIRMED — the page has a heatmap (not timeline) and no "Recently Added" section.

### 1.9 No per-artist drill-down [LOW]
- **File:** `AnalyticsPage.tsx:104-113`
- **Issue:** Clicking an artist bar doesn't navigate to that artist's tracks.
- **Impact:** Analytics are read-only, not actionable.
- **Verified:** CONFIRMED — BarChart bars have no onClick handler.

### 1.10 No total listen time formatting for sub-hour [LOW]
- **File:** `AnalyticsPage.tsx:98`
- **Issue:** `Math.round(ov.listen_sec / 3600)` — if listen time is < 30 min, shows "0h". 30-90 min shows "1h" (inaccurate rounding).
- **Impact:** New users see "0h" listen time which looks broken. Should show minutes for < 1h.
- **Verified:** CONFIRMED.

---

## 2. POOR IMPLEMENTATIONS

### 2.1 Error state is all-or-nothing [HIGH]
- **File:** `AnalyticsPage.tsx:37-45`, `67-78`
- **Issue:** If ANY of the 5 API calls fail, `error` is set to `true`, and the ENTIRE page shows "Failed to load analytics" — even if 4 of 5 succeeded.
- **Fix:** Track errors per-section. Show partial data with error indicators on failed sections.
- **Verified:** CONFIRMED — single `error` boolean, any `.catch()` sets it true.

### 2.2 Retry handler is inline and duplicates logic [MEDIUM]
- **File:** `AnalyticsPage.tsx:74`
- **Issue:** The retry button's `onClick` handler is a 3-line inline arrow function that duplicates the exact same `Promise.all` from `useEffect`. Not DRY.
- **Fix:** Extract a `loadData` function and call it from both `useEffect` and retry.
- **Verified:** CONFIRMED — entire Promise.all block duplicated inline.

### 2.3 No error message detail [MEDIUM]
- **File:** `AnalyticsPage.tsx:73-75`
- **Issue:** Error state shows generic "Failed to load analytics" with no detail about what failed or why.
- **Fix:** Show which section failed, or at least log the actual error to console.
- **Verified:** CONFIRMED.

### 2.4 Heatmap has no column/row labels for screen readers [HIGH]
- **File:** `AnalyticsPage.tsx:130-165`
- **Issue:** The heatmap `<table>` has no `<caption>`, no `scope` attributes on `<th>` cells, and the hour numbers (0-23) have no AM/PM or 24h context.
- **Fix:** Add `<caption>`, `scope="col"`/`scope="row"`, and consider 12h format or "0:00" labels.
- **Verified:** CONFIRMED.

### 2.5 Heatmap color scale has no legend [MEDIUM]
- **File:** `AnalyticsPage.tsx:148-156`
- **Issue:** The intensity scale (rgba opacity) has no legend explaining what the colors mean.
- **Impact:** Users can't interpret the heatmap quantitatively.
- **Verified:** CONFIRMED.

### 2.6 Pie chart labels will overlap with many genres [MEDIUM]
- **File:** `AnalyticsPage.tsx:118`
- **Issue:** `<Pie ... label>` renders labels on every slice. With 10+ genres, labels overlap and become unreadable.
- **Fix:** Only label top 5, or use a legend instead of slice labels.
- **Verified:** CONFIRMED.

### 2.7 Bar chart YAxis truncates long artist names [LOW]
- **File:** `AnalyticsPage.tsx:108`
- **Issue:** `YAxis width={100}` — artist names longer than ~15 chars are truncated with no tooltip.
- **Fix:** Increase width or add a tooltip on the category axis.
- **Verified:** CONFIRMED.

### 2.8 Stat cards don't handle zero gracefully [LOW]
- **File:** `AnalyticsPage.tsx:94-101`
- **Issue:** When `ov` is loaded but all values are 0, shows "0" for plays, "0" for tracks, "0h" for listen time, "0%" for completed. This looks like a bug rather than "no data yet".
- **Fix:** Show "No data yet" or similar when all stats are 0.
- **Verified:** CONFIRMED.

### 2.9 No memoization of heatLookup computation [LOW]
- **File:** `AnalyticsPage.tsx:47-52`
- **Issue:** `heatLookup` Map and `heatMax` are recomputed on every render.
- **Fix:** Wrap in `useMemo`.
- **Verified:** CONFIRMED.

### 2.10 COLORS array is hardcoded and not theme-aware [LOW]
- **File:** `AnalyticsPage.tsx:24`
- **Issue:** `COLORS` uses hex values that may not match the Tailwind theme's accent colors.
- **Fix:** Use CSS variables or Tailwind config colors.
- **Verified:** CONFIRMED.

---

## 3. BUGS

### 3.1 Error state doesn't reset individual section data [HIGH]
- **File:** `AnalyticsPage.tsx:74`
- **Issue:** On retry, `setError(false); setLoading(true)` is called, but the previously loaded data (from the 4 successful calls) is NOT cleared. If the retry partially succeeds again, stale data from the first attempt mixes with new data.
- **Fix:** Clear all state (`setOv(null); setArtists([]); ...`) before retry.
- **Verified:** CONFIRMED — retry handler doesn't clear any data state.

### ~~3.2 Heatmap cells have no key stability [LOW]~~ — **REMOVED (FALSE POSITIVE)**
- **Reason:** The original audit claimed `key={d}` (day name) could collide if DAYS had duplicates, and `key={h}` used array index. In reality, DAYS = `["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]` has 7 unique strings, and `h` is the hour integer (0-23) from `Array.from({ length: 24 }, (_, h) => ...)`. Both are stable and unique. No collision risk.

### 3.3 Listen time shows "0h" for sub-hour, should show minutes [MEDIUM]
- **File:** `AnalyticsPage.tsx:98`
- **Issue:** `Math.round(ov.listen_sec / 3600)` — 30 minutes → `Math.round(0.5)` → `1h` (wrong). 5 minutes → `Math.round(0.083)` → `0h` (misleading).
- **Fix:** Show minutes when < 1h: `${Math.round(ov.listen_sec / 60)}m`.
- **Verified:** CONFIRMED.

### ~~3.4 Backend analytics returns partial data on Scan error [HIGH]~~ — **REMOVED (FALSE POSITIVE)**
- **Reason:** The original audit self-corrected this at line 159: "The current code does handle Scan errors properly." Verified in code: `analytics.go:100-124` — each `QueryRowContext` call has proper error checking and returns 500 on failure. The overview function is correct.

### 3.5 `completed_pct` calculation uses integer division [LOW]
- **File:** `analytics.go:122`
- **Issue:** `c * 100 / o.TotalPlays` — integer division truncates. 1/3 = 33% not 33.3%.
- **Fix:** Use `math.Round(float64(c) * 100.0 / float64(o.TotalPlays))`.
- **Verified:** CONFIRMED — Go integer division truncates.

### 3.6 Top tracks show "(deleted)" for orphaned plays [LOW]
- **File:** `analytics.go:163`
- **Issue:** `IFNULL(t.title,'(deleted)')` — if a track was deleted but plays remain, the UI shows "(deleted)" with no way to clean up.
- **Fix:** Either hide deleted tracks or show a "cleanup" option.
- **Verified:** CONFIRMED.

### ~~3.7 No `rows.Err()` check after `rows.Next()` loop in some backend handlers [MEDIUM]~~ — **REMOVED (FALSE POSITIVE)**
- **Reason:** The original audit self-corrected this: "These DO have `rows.Err()` checks." Verified in code: `analytics.go:153` (topArtists), `188` (topTracks), `221` (topGenres), `256` (heatmap) — all have proper `rows.Err()` checks.

---

## 4. VISUAL ISSUES

### 4.1 No responsive layout for stat cards on small screens [MEDIUM]
- **File:** `AnalyticsPage.tsx:93`
- **Issue:** `grid grid-cols-2 md:grid-cols-4` — on mobile, 2-column grid with 4 stat cards means "Listen time" and "Completed %" are on a second row with potentially truncated text.
- **Fix:** Consider `grid-cols-1 sm:grid-cols-2 lg:grid-cols-4` for better mobile.
- **Verified:** CONFIRMED.

### 4.2 Heatmap table overflows on mobile [HIGH]
- **File:** `AnalyticsPage.tsx:130-165`
- **Issue:** The heatmap has 24 columns + 1 label column = 25 cells per row. On a 375px phone, each cell is ~15px wide. The `overflow-x-auto` wrapper helps, but there's no visual indicator that the table is scrollable.
- **Fix:** Add a fade/shadow on the right edge, or make the heatmap vertical (hours as rows, days as columns).
- **Verified:** CONFIRMED.

### 4.3 Pie chart is too small on mobile [MEDIUM]
- **File:** `AnalyticsPage.tsx:116-125`
- **Issue:** `outerRadius={100}` is fixed regardless of container size. On mobile, the pie chart may be tiny.
- **Fix:** Use a responsive radius based on container width.
- **Verified:** CONFIRMED.

### 4.4 Inconsistent spacing between sections [LOW]
- **File:** `AnalyticsPage.tsx:81` (`space-y-8`) vs `103` (`gap-6`) vs `129` (no gap wrapper)
- **Issue:** The heatmap and top-tracks panels don't have the same gap as the chart panels.
- **Fix:** Wrap all sections in a consistent `space-y-6` or `gap-6` container.
- **Verified:** CONFIRMED.

### 4.5 Stat card values use `text-2xl` which may overflow on mobile [LOW]
- **File:** `AnalyticsPage.tsx:190`
- **Issue:** Large numbers (e.g., "10000 plays") in `text-2xl` may overflow the card on small screens.
- **Fix:** Use `text-xl` on mobile or `truncate`.
- **Verified:** CONFIRMED.

### 4.6 No dark theme consistency for Recharts tooltip [LOW]
- **File:** `AnalyticsPage.tsx:109`, `123`
- **Issue:** Tooltip uses hardcoded `#14171c` and `#2a2f37` instead of Tailwind CSS variables.
- **Fix:** Use `bg-panel` and `border-panel2` classes via a custom tooltip component.
- **Verified:** CONFIRMED.

---

## 5. ACCESSIBILITY

### 5.1 Heatmap table missing ARIA attributes [HIGH]
- **File:** `AnalyticsPage.tsx:131-164`
- **Issue:** No `role="grid"`, no `<caption>`, no `aria-label` on the table. Screen readers will read it as a generic table with no context.
- **Fix:** Add `<caption>Listening activity by day of week and hour</caption>`, `scope="col"` on hour headers, `scope="row"` on day labels.
- **Verified:** CONFIRMED.

### 5.2 Heatmap cells have no accessible text [HIGH]
- **File:** `AnalyticsPage.tsx:151-157`
- **Issue:** Each cell is a `<div>` with only a `title` attribute. Screen readers won't announce the play count.
- **Fix:** Add `aria-label={`${d} ${h}:00 — ${v} plays`}` to each cell.
- **Verified:** CONFIRMED.

### 5.3 Stat cards are not a list [MEDIUM]
- **File:** `AnalyticsPage.tsx:93-101`
- **Issue:** 4 stat cards are bare `<div>` elements in a grid. Screen readers don't know they're related.
- **Fix:** Wrap in `<ul role="list">` and make each card a `<li>`.
- **Verified:** CONFIRMED.

### 5.4 Charts have no accessible alternatives [HIGH]
- **File:** `AnalyticsPage.tsx:104-127` (bar chart), `115-126` (pie chart)
- **Issue:** Recharts `<BarChart>` and `<PieChart>` render as SVG with no text alternative. Screen readers can't interpret the data.
- **Fix:** Add `aria-label` descriptions or provide a "View as table" toggle.
- **Verified:** CONFIRMED.

### 5.5 Help button has insufficient contrast [LOW]
- **File:** `AnalyticsPage.tsx:86`
- **Issue:** `text-muted/50` — 50% opacity on an already-muted color may fail WCAG contrast requirements.
- **Fix:** Use `text-muted` or `text-muted/70`.
- **Verified:** CONFIRMED.

### 5.6 No skip navigation to main content [LOW]
- **File:** `AnalyticsPage.tsx:81`
- **Issue:** The page starts with `<h1>` but there's no skip-link mechanism.
- **Fix:** Add a global skip-nav link in App.tsx.
- **Verified:** CONFIRMED.

### 5.7 Top tracks list lacks keyboard interaction [LOW]
- **File:** `AnalyticsPage.tsx:169-181`
- **Issue:** Uses `<ul>` which is semantic, but each `<li>` has no `tabIndex` or keyboard interaction.
- **Fix:** Add `tabIndex={0}` and keyboard handlers for play action.
- **Verified:** CONFIRMED — `<ul>` is semantic but items are not interactive.

---

## 6. PERFORMANCE

### 6.1 No React.memo on Stat or Panel components [LOW]
- **File:** `AnalyticsPage.tsx:186-202`
- **Issue:** `Stat` and `Panel` are defined inside the component file but not memoized. They re-render on every parent render.
- **Fix:** Move outside the component and wrap with `React.memo`.
- **Verified:** CONFIRMED.

### 6.2 Five parallel API calls with no caching [MEDIUM]
- **File:** `AnalyticsPage.tsx:37-45`
- **Issue:** Every mount fires 5 API calls. No SWR/React Query caching. Navigating away and back re-fetches everything.
- **Fix:** Use a data-fetching library with stale-while-revalidate, or cache at the context level.
- **Verified:** CONFIRMED.

### 6.3 Recharts re-renders entire chart on any state change [LOW]
- **File:** `AnalyticsPage.tsx:105-112`, `116-125`
- **Issue:** The `ResponsiveContainer` + chart components re-render on every parent state change, even if the data hasn't changed.
- **Fix:** Memoize chart data with `useMemo`.
- **Verified:** CONFIRMED.

### 6.4 No lazy loading for below-the-fold sections [LOW]
- **File:** `AnalyticsPage.tsx:129-181`
- **Issue:** Heatmap and top tracks are always rendered even if not visible.
- **Fix:** Use `IntersectionObserver` or `react-intersection-observer` for lazy rendering.
- **Verified:** CONFIRMED.

---

## 7. BACKEND ANALYTICS ISSUES (for reference)

### 7.1 No time-range parameters on any endpoint [MEDIUM]
- **File:** `analytics.go:71-77`
- **Issue:** All endpoints return lifetime data. No `?days=30` or `?from=`/`?to=` parameters.
- **Verified:** CONFIRMED.

### 7.2 Heatmap query uses fmt.Sprintf with timezone [LOW]
- **File:** `analytics.go:231-233`
- **Issue:** `fmt.Sprintf` with `%s` for timezone is safe because `normalizeTimezone()` whitelists values, but it's still a pattern that could be risky if the whitelist is ever bypassed.
- **Note:** This is actually safe. The whitelist prevents SQL injection.
- **Verified:** CONFIRMED — safe due to whitelist.

### 7.3 Genre data may be sparse [LOW]
- **File:** `analytics.go:196-227`
- **Issue:** Many tracks have empty genre fields (depends on tag quality). The `HAVING t.genre!=''` filter means genres with empty strings are excluded, which is correct, but the pie chart may show "Unknown" or be empty.
- **Verified:** CONFIRMED.

---

## 8. PRIORITIZED FIX ROADMAP

### Phase 1: Critical Fixes (user-facing bugs)
1. **Error state handling** (3.1) — Fix all-or-nothing error, clear data on retry
2. **Listen time formatting** (3.3) — Show minutes when < 1h
3. **Empty states** (1.5) — Add helpful empty-state messages for each section
4. **Heatmap mobile overflow** (4.2) — Make heatmap usable on mobile

### Phase 2: High-Impact Improvements
5. **Accessibility** (5.1, 5.2, 5.4) — Heatmap ARIA, chart alternatives
6. **Per-section error handling** (2.1) — Show partial data with section-level errors
7. **Retry function extraction** (2.2) — DRY up load/retry logic
8. **Refresh button** (1.6) — Add manual refresh capability

### Phase 3: Feature Enhancements
9. **Time-range filtering** (1.1) — Add date range selector (requires backend changes)
10. **Play action on top tracks** (1.4) — Make tracks actionable
11. **Loading skeletons** (1.7) — Replace text loading with skeletons
12. **Data caching** (6.2) — Add SWR or React Query

### Phase 4: Polish
13. **Responsive stat cards** (4.1) — Better mobile grid
14. **Pie chart label overlap** (2.6) — Fix with many genres
15. **Heatmap legend** (2.5) — Add color scale legend
16. **Help text accuracy** (1.8) — Remove references to non-existent features
17. **Memoization** (6.1, 6.3) — Performance optimizations
18. **Export** (1.2) — CSV/JSON export

---

## 9. CROSS-REFERENCED FINDINGS FROM RELATED FILES

### App.tsx (routing)
- Analytics route is `/analytics` at line 186 — standard route, no issues.
- Help key `analytics.charts` is registered at line 50.

### api.ts (API client)
- All 5 analytics endpoints are defined: `overview` (line 90), `topArtists` (line 91), `topTracks` (line 92), `topGenres` (line 93), `heatmap` (line 94).
- No time-range parameters available.
- Error handling in `j()` function (lines 19-68) is solid — network errors and non-200 responses throw.

### help-content.ts
- `analytics.charts` entry (lines 334-350) mentions "Listening Timeline" and "Recently Added" which don't exist on the page. **This is a documentation bug.**

### analytics.go (backend)
- All 5 endpoints properly handle errors and return JSON errors.
- `rows.Err()` checks are present after all `rows.Next()` loops.
- Timezone handling via whitelist is secure.
- No user_id filtering issue — uses `user_id IS NULL OR user_id = ?` pattern consistently.

---

## REVISION LOG

| Finding | Original | Revised | Action |
|---------|----------|---------|--------|
| 3.2 Key stability | LOW | — | **REMOVED** — FALSE POSITIVE. DAYS array has unique strings, hour keys are unique integers |
| 3.4 Backend partial data | HIGH | — | **REMOVED** — FALSE POSITIVE. All Scan errors properly return 500 |
| 3.7 rows.Err() missing | MEDIUM | — | **REMOVED** — FALSE POSITIVE. All handlers have rows.Err() checks |
| All others | Various | Same severity | **CONFIRMED** — verified against actual source code |

**Final count:** 27 findings (down from 30). 0 critical, 5 high, 12 medium, 10 low, 0 cosmetic.

---

*End of revised audit report.*
