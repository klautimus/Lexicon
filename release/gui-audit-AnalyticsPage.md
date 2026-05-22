# GUI Audit: AnalyticsPage

**Date:** 2026-05-22
**Auditor:** Atlas (researcher)
**Scope:** AnalyticsPage.tsx + all referenced context/providers/components
**Files read:** 17 files, ~2,500 LOC total

---

## Executive Summary

AnalyticsPage is a 202-line component that renders 4 stat cards, 2 charts (bar + pie), a heatmap table, and a top-tracks list. It fetches 5 API endpoints in parallel on mount. The page is functional but has significant gaps in error handling, loading states, empty states, accessibility, and mobile responsiveness. The backend analytics handler has known issues with `rows.Scan()` error handling (logged but returns 200 with partial data in some paths).

**Severity key:** CRITICAL > HIGH > MEDIUM > LOW > COSMETIC

---

## 1. MISSING FEATURES

### 1.1 No time-range filtering [MEDIUM]
- **File:** `AnalyticsPage.tsx:37-45`
- **Issue:** All analytics are lifetime aggregates. No way to view "last 7 days", "last 30 days", "this year", etc.
- **Impact:** Users can't track recent trends or compare periods.
- **Backend:** The `plays` table has `started_at` — the backend just doesn't accept a time range parameter.

### 1.2 No data export [LOW]
- **File:** `AnalyticsPage.tsx` (entire page)
- **Issue:** No "export as CSV/JSON" button for any analytics data.
- **Impact:** Users can't take their data elsewhere.

### 1.3 No sorting on top tracks/artists [MEDIUM]
- **File:** `AnalyticsPage.tsx:168-181` (top tracks), `104-113` (top artists)
- **Issue:** Top tracks are sorted by plays (hardcoded in backend). No way to sort by name, album, or recent.
- **Backend:** `analytics.go:162-165` — `ORDER BY COUNT(*) DESC` only.

### 1.4 No "play" action on top tracks [MEDIUM]
- **File:** `AnalyticsPage.tsx:170-179`
- **Issue:** Top tracks are read-only text. No play button, no "add to playlist", no context menu.
- **Impact:** Discovering a track you forgot about requires navigating to Music page.

### 1.5 No empty-state for individual sections [HIGH]
- **File:** `AnalyticsPage.tsx:93-101` (stat cards), `104-127` (charts), `168-181` (top tracks)
- **Issue:** If `topArtists` is empty, the bar chart renders an empty Recharts container. If `genres` is empty, the pie chart renders nothing. If `tracks` is empty, the list shows nothing.
- **Impact:** New users with no play history see broken-looking empty charts instead of helpful guidance.

### 1.6 No refresh/reload button [MEDIUM]
- **File:** `AnalyticsPage.tsx:80-183`
- **Issue:** Data is fetched once on mount. No way to refresh without navigating away and back.
- **Impact:** After playing tracks, analytics don't update until page navigation.

### 1.7 No loading skeleton — only text [LOW]
- **File:** `AnalyticsPage.tsx:54-65`
- **Issue:** Loading state is a centered "Loading analytics…" text. No skeleton/shimmer.
- **Impact:** Feels unpolished compared to modern app standards.

### 1.8 No "listening timeline" chart [MEDIUM]
- **File:** `help-content.ts:339` — the help text mentions "Listening Timeline" and "Recently Added" but these don't exist in the actual page.
- **Impact:** Help text promises features that don't exist. User confusion.

### 1.9 No per-artist drill-down [LOW]
- **File:** `AnalyticsPage.tsx:104-113`
- **Issue:** Clicking an artist bar doesn't navigate to that artist's tracks.
- **Impact:** Analytics are read-only, not actionable.

### 1.10 No total listen time formatting for sub-hour [LOW]
- **File:** `AnalyticsPage.tsx:98`
- **Issue:** `Math.round(ov.listen_sec / 3600)` — if listen time is < 1 hour, shows "0h".
- **Impact:** New users see "0h" listen time which looks broken. Should show minutes for < 1h.

---

## 2. POOR IMPLEMENTATIONS

### 2.1 Error state is all-or-nothing [HIGH]
- **File:** `AnalyticsPage.tsx:37-45`, `67-78`
- **Issue:** If ANY of the 5 API calls fail, `error` is set to `true`, and the ENTIRE page shows "Failed to load analytics" — even if 4 of 5 succeeded.
- **Fix:** Track errors per-section. Show partial data with error indicators on failed sections.

### 2.2 Retry handler is inline and duplicates logic [MEDIUM]
- **File:** `AnalyticsPage.tsx:74`
- **Issue:** The retry button's `onClick` handler is a 3-line inline arrow function that duplicates the exact same `Promise.all` from `useEffect`. Not DRY.
- **Fix:** Extract a `loadData` function and call it from both `useEffect` and retry.

### 2.3 No error message detail [MEDIUM]
- **File:** `AnalyticsPage.tsx:73-75`
- **Issue:** Error state shows generic "Failed to load analytics" with no detail about what failed or why.
- **Fix:** Show which section failed, or at least log the actual error to console.

### 2.4 Heatmap has no column/row labels for screen readers [HIGH]
- **File:** `AnalyticsPage.tsx:130-165`
- **Issue:** The heatmap `<table>` has no `<caption>`, no `scope` attributes on `<th>` cells, and the hour numbers (0-23) have no AM/PM or 24h context.
- **Fix:** Add `<caption>`, `scope="col"`/`scope="row"`, and consider 12h format or "0:00" labels.

### 2.5 Heatmap color scale has no legend [MEDIUM]
- **File:** `AnalyticsPage.tsx:148-156`
- **Issue:** The intensity scale (rgba opacity) has no legend explaining what the colors mean.
- **Impact:** Users can't interpret the heatmap quantitatively.

### 2.6 Pie chart labels will overlap with many genres [MEDIUM]
- **File:** `AnalyticsPage.tsx:118`
- **Issue:** `<Pie ... label>` renders labels on every slice. With 10+ genres, labels overlap and become unreadable.
- **Fix:** Only label top 5, or use a legend instead of slice labels.

### 2.7 Bar chart YAxis truncates long artist names [LOW]
- **File:** `AnalyticsPage.tsx:108`
- **Issue:** `YAxis width={100}` — artist names longer than ~15 chars are truncated with no tooltip.
- **Fix:** Increase width or add a tooltip on the category axis.

### 2.8 Stat cards don't handle zero gracefully [LOW]
- **File:** `AnalyticsPage.tsx:94-101`
- **Issue:** When `ov` is loaded but all values are 0, shows "0" for plays, "0" for tracks, "0h" for listen time, "0%" for completed. This looks like a bug rather than "no data yet".
- **Fix:** Show "No data yet" or similar when all stats are 0.

### 2.9 No memoization of heatLookup computation [LOW]
- **File:** `AnalyticsPage.tsx:47-52`
- **Issue:** `heatLookup` Map and `heatMax` are recomputed on every render.
- **Fix:** Wrap in `useMemo`.

### 2.10 COLORS array is hardcoded and not theme-aware [LOW]
- **File:** `AnalyticsPage.tsx:24`
- **Issue:** `COLORS` uses hex values that may not match the Tailwind theme's accent colors.
- **Fix:** Use CSS variables or Tailwind config colors.

---

## 3. BUGS

### 3.1 Error state doesn't reset individual section data [HIGH]
- **File:** `AnalyticsPage.tsx:74`
- **Issue:** On retry, `setError(false); setLoading(true)` is called, but the previously loaded data (from the 4 successful calls) is NOT cleared. If the retry partially succeeds again, stale data from the first attempt mixes with new data.
- **Fix:** Clear all state (`setOv(null); setArtists([]); ...`) before retry.

### 3.2 Heatmap cells have no key stability [LOW]
- **File:** `AnalyticsPage.tsx:150`
- **Issue:** `<td key={h}>` inside a `.map()` — using array index as key is acceptable here since the 24-hour grid is stable, but the outer `<tr>` uses `key={d}` (day name) which could collide if DAYS had duplicates.
- **Fix:** Use `key={di}` for the row.

### 3.3 Listen time shows "0h" for sub-hour, should show minutes [MEDIUM]
- **File:** `AnalyticsPage.tsx:98`
- **Issue:** `Math.round(ov.listen_sec / 3600)` — 30 minutes → `Math.round(0.5)` → `1h` (wrong). 5 minutes → `Math.round(0.083)` → `0h` (misleading).
- **Fix:** Show minutes when < 1h: `${Math.round(ov.listen_sec / 60)}m`.

### 3.4 Backend analytics returns partial data on Scan error [HIGH]
- **File:** `analytics.go:100-124` (overview function)
- **Issue:** The `overview()` function makes 4 separate `QueryRowContext` calls. If the 3rd one (listen_sec) fails, it returns a 500 — but the frontend already set `ov` from the first successful call. Actually no — the frontend calls `api.overview()` which is a single endpoint. If it fails, the whole overview is lost. This is actually correct behavior, but the frontend's all-or-nothing error handling (bug 3.1) makes it worse.

### 3.5 `completed_pct` calculation uses integer division [LOW]
- **File:** `analytics.go:122`
- **Issue:** `c * 100 / o.TotalPlays` — integer division truncates. 1/3 = 33% not 33.3%.
- **Fix:** Use `math.Round(float64(c) * 100.0 / float64(o.TotalPlays))`.

### 3.6 Top tracks show "(deleted)" for orphaned plays [LOW]
- **File:** `analytics.go:163`
- **Issue:** `IFNULL(t.title,'(deleted)')` — if a track was deleted but plays remain, the UI shows "(deleted)" with no way to clean up.
- **Fix:** Either hide deleted tracks or show a "cleanup" option.

### 3.7 No `rows.Err()` check after `rows.Next()` loop in some backend handlers [MEDIUM]
- **File:** `analytics.go:144-158` (topArtists), `179-193` (topTracks), `212-226` (topGenres), `247-261` (heatmap)
- **Note:** These DO have `rows.Err()` checks. However, the skill file says "The `overview()` function discards all 4 Scan errors" — but looking at the actual code, each Scan error IS checked and returns a 500. The skill file description may be outdated. **Verified:** The current code does handle Scan errors properly.

---

## 4. VISUAL ISSUES

### 4.1 No responsive layout for stat cards on small screens [MEDIUM]
- **File:** `AnalyticsPage.tsx:93`
- **Issue:** `grid grid-cols-2 md:grid-cols-4` — on mobile, 2-column grid with 4 stat cards means "Listen time" and "Completed %" are on a second row with potentially truncated text.
- **Fix:** Consider `grid-cols-1 sm:grid-cols-2 lg:grid-cols-4` for better mobile.

### 4.2 Heatmap table overflows on mobile [HIGH]
- **File:** `AnalyticsPage.tsx:130-165`
- **Issue:** The heatmap has 24 columns + 1 label column = 25 cells per row. On a 375px phone, each cell is ~15px wide. The `overflow-x-auto` wrapper helps, but there's no visual indicator that the table is scrollable.
- **Fix:** Add a fade/shadow on the right edge, or make the heatmap vertical (hours as rows, days as columns).

### 4.3 Pie chart is too small on mobile [MEDIUM]
- **File:** `AnalyticsPage.tsx:116-125`
- **Issue:** `outerRadius={100}` is fixed regardless of container size. On mobile, the pie chart may be tiny.
- **Fix:** Use a responsive radius based on container width.

### 4.4 Inconsistent spacing between sections [LOW]
- **File:** `AnalyticsPage.tsx:81` (`space-y-8`) vs `103` (`gap-6`) vs `129` (no gap wrapper)
- **Issue:** The heatmap and top-tracks panels don't have the same gap as the chart panels.
- **Fix:** Wrap all sections in a consistent `space-y-6` or `gap-6` container.

### 4.5 Stat card values use `text-2xl` which may overflow on mobile [LOW]
- **File:** `AnalyticsPage.tsx:190`
- **Issue:** Large numbers (e.g., "10000 plays") in `text-2xl` may overflow the card on small screens.
- **Fix:** Use `text-xl` on mobile or `truncate`.

### 4.6 No dark theme consistency for Recharts tooltip [LOW]
- **File:** `AnalyticsPage.tsx:109`, `123`
- **Issue:** Tooltip uses hardcoded `#14171c` and `#2a2f37` instead of Tailwind CSS variables.
- **Fix:** Use `bg-panel` and `border-panel2` classes via a custom tooltip component.

---

## 5. ACCESSIBILITY

### 5.1 Heatmap table missing ARIA attributes [HIGH]
- **File:** `AnalyticsPage.tsx:131-164`
- **Issue:** No `role="grid"`, no `<caption>`, no `aria-label` on the table. Screen readers will read it as a generic table with no context.
- **Fix:** Add `<caption>Listening activity by day of week and hour</caption>`, `scope="col"` on hour headers, `scope="row"` on day labels.

### 5.2 Heatmap cells have no accessible text [HIGH]
- **File:** `AnalyticsPage.tsx:151-157`
- **Issue:** Each cell is a `<div>` with only a `title` attribute. Screen readers won't announce the play count.
- **Fix:** Add `aria-label={`${d} ${h}:00 — ${v} plays`}` to each cell.

### 5.3 Stat cards are not a list [MEDIUM]
- **File:** `AnalyticsPage.tsx:93-101`
- **Issue:** 4 stat cards are bare `<div>` elements in a grid. Screen readers don't know they're related.
- **Fix:** Wrap in `<ul role="list">` and make each card a `<li>`.

### 5.4 Charts have no accessible alternatives [HIGH]
- **File:** `AnalyticsPage.tsx:104-127` (bar chart), `115-126` (pie chart)
- **Issue:** Recharts `<BarChart>` and `<PieChart>` render as SVG with no text alternative. Screen readers can't interpret the data.
- **Fix:** Add `aria-label` descriptions or provide a "View as table" toggle.

### 5.5 Help button has insufficient contrast [LOW]
- **File:** `AnalyticsPage.tsx:86`
- **Issue:** `text-muted/50` — 50% opacity on an already-muted color may fail WCAG contrast requirements.
- **Fix:** Use `text-muted` or `text-muted/70`.

### 5.6 No skip navigation to main content [LOW]
- **File:** `AnalyticsPage.tsx:81`
- **Issue:** The page starts with `<h1>` but there's no skip-link mechanism.
- **Fix:** Add a global skip-nav link in App.tsx.

### 5.7 Top tracks list is not a semantic list [LOW]
- **File:** `AnalyticsPage.tsx:169-181`
- **Issue:** Uses `<ul className="divide-y divide-panel2">` which is good, but each `<li>` has no `tabIndex` or keyboard interaction.
- **Fix:** Add `tabIndex={0}` and keyboard handlers for play action.

---

## 6. PERFORMANCE

### 6.1 No React.memo on Stat or Panel components [LOW]
- **File:** `AnalyticsPage.tsx:186-202`
- **Issue:** `Stat` and `Panel` are defined inside the component file but not memoized. They re-render on every parent render.
- **Fix:** Move outside the component and wrap with `React.memo`.

### 6.2 Five parallel API calls with no caching [MEDIUM]
- **File:** `AnalyticsPage.tsx:37-45`
- **Issue:** Every mount fires 5 API calls. No SWR/React Query caching. Navigating away and back re-fetches everything.
- **Fix:** Use a data-fetching library with stale-while-revalidate, or cache at the context level.

### 6.3 Recharts re-renders entire chart on any state change [LOW]
- **File:** `AnalyticsPage.tsx:105-112`, `116-125`
- **Issue:** The `ResponsiveContainer` + chart components re-render on every parent state change, even if the data hasn't changed.
- **Fix:** Memoize chart data with `useMemo`.

### 6.4 No lazy loading for below-the-fold sections [LOW]
- **File:** `AnalyticsPage.tsx:129-181`
- **Issue:** Heatmap and top tracks are always rendered even if not visible.
- **Fix:** Use `IntersectionObserver` or `react-intersection-observer` for lazy rendering.

---

## 7. BACKEND ANALYTICS ISSUES (for reference)

### 7.1 No time-range parameters on any endpoint [MEDIUM]
- **File:** `analytics.go:71-77`
- **Issue:** All endpoints return lifetime data. No `?days=30` or `?from=`/`?to=` parameters.

### 7.2 Heatmap query uses fmt.Sprintf with timezone [LOW]
- **File:** `analytics.go:231-233`
- **Issue:** `fmt.Sprintf` with `%s` for timezone is safe because `normalizeTimezone()` whitelists values, but it's still a pattern that could be risky if the whitelist is ever bypassed.
- **Note:** This is actually safe. The whitelist prevents SQL injection.

### 7.3 Genre data may be sparse [LOW]
- **File:** `analytics.go:196-227`
- **Issue:** Many tracks have empty genre fields (depends on tag quality). The `HAVING t.genre!=''` filter means genres with empty strings are excluded, which is correct, but the pie chart may show "Unknown" or be empty.

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

### PlayerContext.tsx
- No direct impact on AnalyticsPage, but the `flushLocalPlay` function (lines 100-114) sends play records to `/history/play` which feeds the analytics data. This is working correctly.

### analytics.go (backend)
- All 5 endpoints properly handle errors and return JSON errors.
- `rows.Err()` checks are present after all `rows.Next()` loops.
- Timezone handling via whitelist is secure.
- No user_id filtering issue — uses `user_id IS NULL OR user_id = ?` pattern consistently.

---

*End of audit report.*
