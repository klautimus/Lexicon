# GUI Audit: MusicPage — Comprehensive Analysis

**Date:** 2026-05-22
**Auditor:** Atlas (researcher)
**Scope:** MusicPage.tsx + all supporting files listed in task body
**Files read in full:** All 15 files

---

## 1. MISSING FEATURES

### 1.1 No Sorting (CRITICAL — MusicPage.tsx)
The help content at `help-content.ts:81` says "Sort — Click column headers to sort by title, artist, album, etc." but **TrackList.tsx has no sortable column headers**. The table headers at `TrackList.tsx:25-31` are static `<th>` elements with no click handlers, no sort indicators, no aria-sort attributes. This is a documented feature that doesn't exist.

### 1.2 No Column Resizing / Persistent Column Widths
TrackList columns have no resize capability. The album column is hardcoded to `max-w-48` (`TrackList.tsx:30,167`) which truncates long album names. Users can't adjust column widths.

### 1.3 No View Mode Toggle (List/Grid)
MusicPage only renders a table (desktop) or card list (mobile). No grid/album-art view option. Common in music apps.

### 1.4 No "Play All" / "Shuffle All" on MusicPage
The page shows tracks but has no "Play All" or "Shuffle All" button. Users must click the first track to start playback, then rely on queue behavior. A "Play All" button at the top of the track list would be expected.

### 1.5 No Bulk Select / Bulk Actions
No checkboxes, no multi-select, no bulk operations (bulk add to playlist, bulk delete, bulk upgrade). The `handleBulkUpgrade` function exists but operates on ALL tracks with no selection mechanism.

### 1.6 No Keyboard Shortcuts
No keyboard shortcuts for play/pause, next/prev, search focus, or navigation. The player controls exist in PlayerBar but there are no page-level shortcuts.

### 1.7 No Drag-and-Drop Reordering
No ability to drag tracks to reorder or drag into playlists.

### 1.8 No Duration Column in TrackList
The TrackList table (`TrackList.tsx:25-31`) has columns for #, Title, Artist, Album — but no Duration column. Duration data is available in the Track model (`api.ts:268: duration_sec`). This is a standard music library feature.

### 1.9 No Genre or Year Column
Track model has `genre` and `year` fields (`api.ts:266-267`) but TrackList doesn't display them. No column toggle to show/hide optional columns.

### 1.10 No Empty State for "No Tracks" When Not Loading
At `MusicPage.tsx:286`, the empty state when there are no tracks and no query is just `<p className="text-muted">No tracks.</p>` — a bare text line. The empty state when there IS a query (`MusicPage.tsx:256-284`) is much richer with an icon, message, and download button. The true empty state should have similar treatment with a "Scan Library" or "Download Music" CTA.

### 1.11 No Loading Skeleton
At `MusicPage.tsx:236`, the loading state is just `<p className="text-muted">Loading…</p>`. No skeleton placeholder for the table.

### 1.12 No Error State for Failed Track Load
In `loadInitial()` (`MusicPage.tsx:46-52`), if `api.tracks()` fails, the `.finally()` still sets `loading` to false, but there's no error state shown. The user sees "No tracks." even if the API call failed. Compare with DownloadContext which has `console.error` for failures.

### 1.13 No Total Duration Display
The page shows "X tracks in library" (`MusicPage.tsx:220`) but not total duration. Music library users expect to see "142 tracks, 8h 32m".

### 1.14 No Track Count in Page Title / Header
The header is just "Music" (`MusicPage.tsx:186`). Could show "Music (142)" for quick reference.

### 1.15 No "Add to Queue" Action
The TrackList context menu has "Add to playlist", "Upgrade Quality", "Delete" — but no "Add to Queue" action. Users can only play immediately (which replaces the queue).

### 1.16 No Filter by Genre/Year/Quality
The filter input only searches title/artist/album (`MusicPage.tsx:80-84`). No dropdown filters for genre, year range, quality/format, etc.

### 1.17 No "Go to Artist" / "Go to Album" Navigation
Clicking a track plays it, but there's no way to navigate to an "artist view" or "album view" showing all tracks by that artist/album.

### 1.18 No Cover Art in TrackList
The desktop table has no cover art column. Mobile cards show cover art (`TrackList.tsx:431-436`). Desktop users see only text.

---

## 2. POOR IMPLEMENTATIONS

### 2.1 Client-Side Filtering Only (MusicPage.tsx:78-86)
Filtering is done client-side on `allTracks`. With `PAGE_SIZE = 200` (`MusicPage.tsx:8`), only 200 tracks are loaded initially. If the user has 5000 tracks, the filter only searches the loaded 200. The "Load More" button loads more pages, but the filter doesn't trigger a server-side search. This is misleading — the filter says "X of Y tracks match" where Y is the total count but X only matches loaded tracks.

### 2.2 Pagination State Bug (MusicPage.tsx:26-44)
The `fetchPage` function uses `offset` parameter but computes it as `off + limit` after the API call. If `loadInitial()` is called while a `handleLoadMore` is in flight, the offset can get out of sync because both functions set state independently. The `setLoadingMore(true)` guard in `handleLoadMore` helps, but there's no abort mechanism for the in-flight request.

### 2.3 Duplicate Download Polling Logic
The `trackDownload` function in MusicPage (`MusicPage.tsx:88-127`) is nearly identical to the one in DownloadContext (`DownloadContext.tsx:92-174`). Both poll `api.downloadJob()` every 2 seconds. This is code duplication that should be centralized in DownloadContext.

### 2.4 No Memoization of Filtered Results (MusicPage.tsx:78-86)
The `filtered` array is recomputed on every render. With 200+ tracks and rapid typing, this could cause jank. Should be wrapped in `useMemo`.

### 2.5 Hardcoded Page Size (MusicPage.tsx:8)
`PAGE_SIZE = 200` is hardcoded. Should be configurable or responsive to viewport height.

### 2.6 Upgrade All is Sequential with Fixed Delay (MusicPage.tsx:167-176)
`handleBulkUpgrade` iterates through ALL track IDs sequentially with a 500ms delay between each. For a library of 5000 tracks, this takes 2500 seconds (41 minutes) just in delays, plus actual download time. No concurrency, no batching, no pause/resume.

### 2.7 No Progress Tracking for Individual Upgrades (MusicPage.tsx:167-176)
The bulk upgrade calls `api.upgradeTrack()` which returns immediately with a job ID, but the code doesn't poll for job completion. It just counts "done" when the API call succeeds (which only means the job was enqueued, not completed). The progress counter is misleading.

### 2.8 Delete Doesn't Remove Track from Player Queue (TrackList.tsx:126-139)
When a track is deleted via the context menu, `onDelete?.(track.id)` is called, which in MusicPage triggers `handleRefresh()` (reloading the track list). But if the deleted track is currently playing or in the queue, PlayerContext still has a reference to it. The player will try to play a deleted file.

### 2.9 No Confirmation for Upgrade All (MusicPage.tsx:159-162)
The bulk upgrade uses `window.confirm()` which is blocking and ugly. The delete action has a nice two-step confirmation UI in the dropdown (`TrackList.tsx:188-206`), but upgrade all uses a browser confirm dialog.

### 2.10 TrackList Duplicates Desktop/Mobile Logic (TrackList.tsx)
`DesktopTable`/`TrackRow` and `MobileCardList`/`MobileTrackCard` are separate component trees with duplicated logic (playlist loading, add to playlist, create playlist, delete, upgrade). The only difference should be presentation, not logic.

### 2.11 No Error Handling in TrackList Delete (TrackList.tsx:134)
The catch block in `handleDelete` sets `setDeleteError("Failed to delete")` but doesn't log the actual error to console. Makes debugging difficult.

### 2.12 MobileTrackCard Has No Upgrade Action (TrackList.tsx:426-583)
The mobile card context menu has "Add to playlist" and "Delete" but is missing the "Upgrade Quality" action that the desktop `TrackRow` has (`TrackList.tsx:273-285`). This is a mobile/desktop parity gap.

### 2.13 No ARIA Live Region for Filter Results (MusicPage.tsx:210-215)
The filter results count is announced visually but not via ARIA live region. Screen reader users won't know the count changed.

### 2.14 Search Input Has No Clear Button (MusicPage.tsx:201-208)
The filter input has no "X" button to clear the query. Users must manually delete the text.

### 2.15 No Debounce on Filter Input (MusicPage.tsx:204)
Every keystroke triggers a re-render and re-filter. Should debounce or at least use `useMemo` for the filtered results.

---

## 3. BUGS

### 3.1 Filter Count Shows Total, Not Loaded Count (MusicPage.tsx:212)
`{filtered.length} of {total} track{total !== 1 ? "s" : ""}` — `total` is the server-side total count, but `filtered.length` only reflects loaded tracks. If the user has 5000 tracks but only 200 loaded, and the filter matches 50 tracks total, it shows "3 of 5000 tracks match" (only 3 in the loaded 200 match). This is misleading.

### 3.2 Load More Doesn't Preserve Filter (MusicPage.tsx:243-253)
When "Load More" is clicked, `handleLoadMore` calls `fetchPage("music", PAGE_SIZE, offset, true)` which appends to `allTracks`. But if a filter is active, the new page loads correctly — however, the "Load More" button is hidden when a filter is active (`!q` check at line 243), so this is actually correct behavior. **Not a bug** — but the UX is confusing because users can't load more results while filtering.

### 3.3 Download Search Doesn't Clear Query (MusicPage.tsx:129-140)
After `handleDownloadSearch` succeeds, the query text remains in the input. The user might accidentally re-download the same track.

### 3.4 Race Condition in loadInitial (MusicPage.tsx:46-52)
If `loadInitial` is called rapidly (e.g., double-clicking refresh), multiple in-flight requests can race. The second call resets `allTracks` to `[]` but the first call's `.then()` might still append to it. No request cancellation or sequence counter.

### 3.5 Stale Closure in handleLoadMore (MusicPage.tsx:68-76)
`handleLoadMore` reads `offset` and `hasMore` from the render closure. If `loadInitial` is called while `handleLoadMore` is in flight, the offset may be stale. The `loadingMore` flag prevents concurrent calls but doesn't prevent stale offset values.

### 3.6 Poll Ref Cleanup on Unmount (MusicPage.tsx:58-62)
The cleanup effect clears intervals in `pollRef.current`, but if the component unmounts while a download is in progress, the `trackDownload` callback's `handleRefresh()` will try to update state on an unmounted component. React will warn about this.

### 3.7 Upgrade All Fetches All Pages Sequentially (MusicPage.tsx:148-153)
The `while(true)` loop fetches tracks page by page. If the total is large (e.g., 5000 tracks = 5 API calls), this blocks the UI thread during the fetch. Should show a loading state during this phase.

### 3.8 No Error Handling for upgradeTrack API Failures (MusicPage.tsx:168-175)
The catch block increments `failed` but doesn't log the error. Users see "X failed" but can't diagnose why.

### 3.9 TrackList Key Uses Index (TrackList.tsx:35,319)
`key={${t.id}-${i}}` — if the same track appears twice in the list (unlikely but possible), the key will be unique due to index, but React's reconciliation may not properly track items if the list changes. Should use `t.id` alone as key.

### 3.10 Mobile Card Menu Doesn't Close on Action (TrackList.tsx:462-578)
In the mobile `MobileTrackCard`, after clicking "Delete" and confirming, `setOpen(false)` is called in `handleDelete` (line 407). But after "Add to playlist", the menu stays open (no `setOpen(false)`). Desktop `TrackRow` also stays open after adding. Inconsistent behavior.

---

## 4. VISUAL ISSUES

### 4.1 Inconsistent Empty State Styling (MusicPage.tsx:256-287)
The "no results" empty state has a nice card with icon, message, and button. The "no tracks" empty state is just plain text. Should be consistent.

### 4.2 No Visual Indicator for Currently Playing Track
TrackList doesn't highlight the currently playing track. In a large library, users can't see what's playing without looking at the PlayerBar.

### 4.3 Upgrade All Button Styling (MusicPage.tsx:226-229)
The "Upgrade All to Opus" button uses `bg-yellow-500/20 text-yellow-400` which doesn't match the accent color scheme. Uses raw Tailwind colors instead of the theme tokens (`bg-accent`, `text-accent`).

### 4.4 Filter Input Icon Color (MusicPage.tsx:197-199)
The Search icon uses `text-muted` which may be too low-contrast against `bg-panel2`.

### 4.5 Track List Row Hover State (TrackList.tsx:153)
`hover:bg-panel2/40` is very subtle. Hard to see which row is being hovered, especially on lower-contrast displays.

### 4.6 No Visual Separator Between TrackList Pages
When "Load More" loads additional tracks, there's no visual indicator of where the previous page ended.

### 4.7 Mobile Card Action Button Sizing (TrackList.tsx:454-460)
The "..." button on mobile cards is `w-9 h-9` but the play button is also `w-9 h-9`. The touch target is small for mobile. Should be at least 44x44px per accessibility guidelines.

### 4.8 Desktop Table Has No Fixed Header (TrackList.tsx:22-47)
When scrolling through many tracks, the column headers scroll out of view. No sticky header.

---

## 5. ACCESSIBILITY

### 5.1 No aria-label on Search Input (MusicPage.tsx:201-208)
The filter input has `placeholder` but no `aria-label` or associated `<label>` element.

### 5.2 No aria-sort on Table Headers (TrackList.tsx:25-31)
Table headers don't indicate sort state (even though sorting doesn't exist yet, the headers should still have `aria-sort="none"`).

### 5.3 No Role="status" for Filter Results (MusicPage.tsx:210-215)
The filter results count should be a live region for screen readers.

### 5.4 No Keyboard Navigation in TrackList (TrackList.tsx)
Track rows are not focusable via keyboard. No `tabIndex`, no keyboard event handlers. Users can't navigate the track list without a mouse.

### 5.5 Context Menu Not Keyboard Accessible (TrackList.tsx:170-179)
The "..." button and dropdown menu have no keyboard support. Can't open the menu with Enter/Space, can't navigate menu items with arrow keys.

### 5.6 No Skip Navigation Link
No "skip to main content" link for keyboard users.

### 5.7 Load More Button Has No aria-busy (MusicPage.tsx:246-251)
When loading more, the button text changes to "Loading…" but there's no `aria-busy="true"` on the container.

### 5.8 No aria-describedby on Upgrade Button (MusicPage.tsx:223-231)
The "Upgrade All to Opus" button has no description explaining what it does. The help button is on the page header, not near the upgrade button.

### 5.9 Table Has No Caption (TrackList.tsx:23)
The `<table>` has no `<caption>` element describing its purpose.

### 5.10 Mobile Card Has No aria-label on Play Button (TrackList.tsx:449-453)
The play button on mobile cards has no `aria-label` indicating which track it plays.

---

## 6. PERFORMANCE

### 6.1 No Memoization of Filtered Tracks (MusicPage.tsx:78-86)
`filtered` is recomputed on every render. Should be `useMemo` with `[allTracks, q]` deps.

### 6.2 No Virtualization for Large Lists
All tracks are rendered in the DOM at once. With 200+ tracks, this creates 200+ `<tr>` elements. Should use virtualization (e.g., `react-virtuoso` or `react-window`) for large lists.

### 6.3 TrackRow Creates New Functions on Every Render (TrackList.tsx:50-306)
`TrackRow` defines `loadPlaylists`, `toggle`, `addToPlaylist`, `createPlaylist`, `handleDelete`, `handleUpgrade` as inline functions. These are recreated on every render. Should be `useCallback` wrapped.

### 6.4 No React.memo on TrackRow (TrackList.tsx:50)
`TrackRow` is not wrapped in `React.memo`. When the parent re-renders (e.g., typing in filter), every TrackRow re-renders even though only the filtered list changed.

### 6.5 Poll Ref Uses Record<string, number> (MusicPage.tsx:13)
`pollRef` stores interval IDs keyed by job ID. If many downloads are tracked, this grows without bound. The cleanup on unmount clears them, but during the component lifetime, completed job intervals are cleaned up individually (lines 107-108, 112-113, 116-117) which is correct.

### 6.6 DownloadContext Polls Every 2 Seconds (DownloadContext.tsx:113-170)
The `trackDownload` function in DownloadContext polls every 2 seconds. With many concurrent downloads, this creates many polling intervals. Should use a single centralized poller.

### 6.7 Spotify Player Polls Every 1 Second (PlayerContext.tsx:561-589)
The Spotify state poller runs every 1 second even when Spotify is not the active source (it checks `sourceRef.current !== "spotify"` inside the interval). This is wasteful — the interval should be created/cancelled based on source changes.

### 6.8 No Code Splitting for MusicPage
MusicPage is imported directly in App.tsx (`App.tsx:31`). Should use `React.lazy` + `Suspense` for code splitting.

---

## PRIORITIZED FIX ROADMAP

### P0 — Critical (Broken / Misleading)
1. **Filter count shows wrong total** (3.1) — Fix to show "X of Y loaded tracks" or switch to server-side search
2. **No error state for failed track load** (1.12) — Add try/catch with error state in loadInitial
3. **Race condition in loadInitial** (3.4) — Add request sequence counter or abort controller
4. **Delete doesn't update player queue** (2.8) — Emit event or call player.removeFromQueue()

### P1 — High (Missing Core Features)
5. **Add sorting to TrackList** (1.1) — Implement column header click sorting with aria-sort
6. **Add "Play All" / "Shuffle All" buttons** (1.4) — Add to page header
7. **Add Duration column** (1.8) — Display duration_sec in TrackList
8. **Fix mobile/desktop parity for Upgrade** (2.12) — Add Upgrade to mobile card menu
9. **Memoize filtered results** (6.1) — Wrap in useMemo
10. **Add React.memo to TrackRow** (6.4) — Prevent unnecessary re-renders

### P2 — Medium (UX Improvements)
11. **Add loading skeleton** (1.11) — Replace "Loading…" with skeleton
12. **Improve empty state** (1.10) — Add icon + CTA for empty library
13. **Add clear button to search input** (2.14) — X button to clear filter
14. **Add currently playing indicator** (4.2) — Highlight playing track in list
15. **Add total duration display** (1.13) — Show "142 tracks, 8h 32m"
16. **Add view mode toggle** (1.3) — List/grid option
17. **Add genre/year columns** (1.9) — Optional columns
18. **Add "Add to Queue" action** (1.15) — Context menu item

### P3 — Lower (Nice to Have)
19. **Virtualize track list** (6.2) — For large libraries
20. **Add keyboard shortcuts** (1.6) — Play/pause, search focus
21. **Add bulk select/actions** (1.5) — Checkboxes + bulk operations
22. **Add drag-and-drop** (1.7) — Reorder, drag to playlist
23. **Add artist/album navigation** (1.17) — Click artist name to filter
24. **Add cover art column** (1.18) — Desktop table
25. **Code split MusicPage** (6.8) — React.lazy
26. **Centralize download polling** (2.3) — Use DownloadContext only
27. **Add column resizing** (1.2) — Resizable table columns
28. **Add filter by genre/year** (1.16) — Dropdown filters

### Accessibility Fixes (Should be done with each feature)
- Add aria-label to search input (5.1)
- Add aria-sort to table headers (5.2)
- Add role="status" for filter results (5.3)
- Make TrackList keyboard navigable (5.4)
- Make context menu keyboard accessible (5.5)
- Add skip navigation link (5.6)
- Add aria-busy to Load More (5.7)
- Add table caption (5.9)
- Add aria-label to mobile play buttons (5.10)
