# GUI Audit: SearchPage.tsx — REVISED

**Date:** 2026-05-22
**Auditor:** Atlas (researcher)
**Reviewer:** Atlas (analyst)
**Scope:** Deep exploration of SearchPage.tsx and all supporting files
**Build status:** Backend ✅ (go build clean), Frontend ✅ (vite build clean, 743KB bundle)

**Files reviewed:**
- `frontend/src/pages/SearchPage.tsx` (159 lines)
- `frontend/src/App.tsx` (283 lines)
- `frontend/src/player/PlayerContext.tsx` (688 lines)
- `frontend/src/contexts/DownloadContext.tsx` (374 lines)
- `frontend/src/contexts/UserContext.tsx` (72 lines)
- `frontend/src/contexts/ToastContext.tsx` (93 lines)
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
- `backend/internal/library/library.go` (404 lines, search handler)

---

## REVIEW SUMMARY

**Original plan:** 49 findings (15 missing features, 10 poor implementations, 7 bugs, 5 visual, 6 accessibility, 3 performance, 3 cross-cutting)

**Revised plan:** 47 findings confirmed, 2 downgraded in severity, 0 false positives removed, 2 new findings added

**Key changes from original:**
- **2.6** (No memoization of TrackList): Downgraded from Low to Informational — `results` is state, not recreated per render. The real cost is TrackList's 584-line size.
- **6.1** (No memoization of search results): Downgraded from Low to Informational — same reasoning.
- **6.2** (Download polling race condition): Downgraded from Low to Informational — the `pollRef.current[job.id]` check on line 54 does prevent duplicate intervals for the same job ID.
- **3.2** (Memory leak on unmount): Clarified — interval cleanup IS handled (lines 18-22), but setState-after-unmount on line 64 is NOT guarded. Severity stays Medium.
- **New finding 3.8:** `handleDownloadSearch` doesn't check `downloading` state — user can click download button multiple times while a download is in progress, spawning duplicate downloads.
- **New finding 3.9:** `handleDownloadSearch` doesn't reset `downloading` state if `q.trim()` is empty (line 84 early return leaves `downloading` true if it was already set from a previous call).

---

## 1. MISSING FEATURES

### 1.1 No search history / recent searches
**Severity:** Medium | **Type:** Missing Feature | **Status:** CONFIRMED
**File:** `SearchPage.tsx:8-15`

The search page maintains no history of previous searches. Every visit starts with an empty input. Music apps typically offer:
- Recent searches dropdown when clicking the input
- Quick-access "trending" or "suggested" searches
- Ability to clear search history

The `searched` state (line 14) only tracks whether a search was performed, not what was searched.

### 1.2 No search filters or advanced search
**Severity:** Medium | **Type:** Missing Feature | **Status:** CONFIRMED
**File:** `SearchPage.tsx:108-118`

The search is a single free-text input with no filters for:
- Media kind (music vs podcast vs audiobook)
- Date added range
- Duration range
- Genre
- Year

The backend FTS5 search supports these fields (title, artist, album, genre), but the frontend exposes none of this.

### 1.3 No keyboard shortcut for search focus
**Severity:** Low | **Type:** Missing Feature | **Status:** CONFIRMED
**File:** `SearchPage.tsx:108-118`

No `accesskey`, no `autoFocus`, no global keyboard shortcut (e.g., `/` or `Ctrl+K`) to jump to the search input. The user must click the input manually.

### 1.4 No search result count
**Severity:** Low | **Type:** Missing Feature | **Status:** CONFIRMED
**File:** `SearchPage.tsx:124-125`

When results are found, the page shows the TrackList but no indication of how many results matched. The backend `search` endpoint returns a raw array (not a `{tracks, total}` response like the paginated `tracks` endpoint), so the count is implicit. But the UI doesn't show "42 results for 'beatles'" or similar.

### 1.5 No sorting of search results
**Severity:** Medium | **Type:** Missing Feature | **Status:** CONFIRMED
**File:** `SearchPage.tsx:124-125`

Results are displayed in whatever order the backend returns them (FTS5 rank order). The user cannot sort by title, artist, album, date added, etc.

### 1.6 No "clear search" button
**Severity:** Low | **Type:** Missing Feature | **Status:** CONFIRMED
**File:** `SearchPage.tsx:109-114`

The input has no clear (X) button. To clear a search, the user must manually select and delete the text.

### 1.7 No empty state before first search
**Severity:** Low | **Type:** Missing Feature | **Status:** CONFIRMED
**File:** `SearchPage.tsx:120-156`

When the page first loads, the user sees only the search input and nothing else. No prompt, no suggestions, no "try searching for..." hints. Compare with Discover page which has example prompts.

### 1.8 No bulk actions on search results
**Severity:** Medium | **Type:** Missing Feature | **Status:** CONFIRMED
**File:** `SearchPage.tsx:124-125`

The TrackList component supports per-row actions (add to playlist, delete, upgrade), but there's no bulk select mode for selecting multiple search results and performing batch operations.

### 1.9 No "add all results to playlist" action
**Severity:** Medium | **Type:** Missing Feature | **Status:** CONFIRMED
**File:** `SearchPage.tsx:124-125`

A common music app pattern: search for something, then add all results to a playlist in one click. Not possible here.

### 1.10 No search within results / refinement
**Severity:** Low | **Type:** Missing Feature | **Status:** CONFIRMED
**File:** `SearchPage.tsx:108-118`

No way to refine a search without completely replacing the query. No "search within results" toggle.

### 1.11 No loading skeleton / spinner
**Severity:** Low | **Type:** Missing Feature | **Status:** CONFIRMED
**File:** `SearchPage.tsx:120-123`

The loading state shows a simple "Searching…" text in a gray box. A skeleton loader or spinner would provide better visual feedback.

### 1.12 No error state differentiation
**Severity:** Medium | **Type:** Missing Feature | **Status:** CONFIRMED
**File:** `SearchPage.tsx:32-33`

All search failures show the same generic "Search failed" toast. No distinction between:
- Network error (server unreachable)
- Server error (500)
- Timeout
- Empty query

The `api.ts` `j()` function (lines 19-68) actually produces detailed error messages, but the catch block in `go()` (line 32) discards them entirely.

### 1.13 No result highlighting
**Severity:** Low | **Type:** Missing Feature | **Status:** CONFIRMED
**File:** `SearchPage.tsx:124-125`

Search results don't highlight the matching text in the track title/artist/album. FTS5 supports `snippet()` for this, but it's not used.

### 1.14 No "play all results" button
**Severity:** Medium | **Type:** Missing Feature | **Status:** CONFIRMED
**File:** `SearchPage.tsx:124-125`

When search results are displayed, there's no "Play All" button to enqueue all results and start playing. The user must double-click the first track or manually select tracks.

### 1.15 No pagination for search results
**Severity:** Medium | **Type:** Missing Feature | **Status:** CONFIRMED
**File:** `SearchPage.tsx:124-125`

The backend search endpoint returns all matching results at once. For large libraries, this could be hundreds of tracks. The MusicPage has "Load More" pagination, but SearchPage does not.

---

## 2. POOR IMPLEMENTATIONS

### 2.1 Silent error swallowing in search handler
**Severity:** High | **Type:** Poor Implementation | **Status:** CONFIRMED
**File:** `SearchPage.tsx:32-33`

```typescript
catch {
  toast.error("Search failed");
}
```

The error object is completely discarded. The `api.ts` `j()` function produces detailed messages like "Unable to reach the server" or "500 Internal Server Error", but none of that reaches the user. Should be:

```typescript
catch (e) {
  toast.error(e instanceof Error ? e.message : "Search failed");
}
```

### 2.2 Silent error swallowing in download handler
**Severity:** High | **Type:** Poor Implementation | **Status:** CONFIRMED
**File:** `SearchPage.tsx:90-92`

Same pattern — `handleDownloadSearch` discards the error:

```typescript
catch {
  setDownloading(false);
  toast.error("Failed to start download");
}
```

### 2.3 Silent error swallowing in download polling
**Severity:** Medium | **Type:** Poor Implementation | **Status:** CONFIRMED
**File:** `SearchPage.tsx:74-78`

The polling catch block discards the error and shows a generic "Lost connection tracking download" message. No console.error for debugging. Compare with DownloadContext.tsx line 160 which does `console.error(...)`.

### 2.4 Duplicate download tracking logic
**Severity:** Medium | **Type:** Poor Implementation | **Status:** CONFIRMED
**File:** `SearchPage.tsx:39-81` vs `DownloadContext.tsx:92-174`

The `trackDownload` function in SearchPage.tsx is nearly identical to the one in DownloadContext. This is code duplication — SearchPage should use `useDownloads().trackDownload()` from the shared context instead of implementing its own polling logic.

### 2.5 Search doesn't use DownloadContext
**Severity:** Medium | **Type:** Poor Implementation | **Status:** CONFIRMED
**File:** `SearchPage.tsx:39-81`

SearchPage manages its own download polling via `pollRef` instead of using the `DownloadContext` that was specifically designed for cross-route download state persistence. This means:
- Downloads started on SearchPage won't show in DownloadProgressBar
- Downloads started on SearchPage won't appear on DownloadsPage
- The global download count in DownloadProgressBar won't reflect SearchPage downloads

### 2.6 No memoization of TrackList
**Severity:** Informational | **Type:** Performance | **Status:** DOWNGRADED
**File:** `SearchPage.tsx:125`

Original assessment overstated this. `results` is state — it's only set via `setResults`, not recreated per render. The TrackList re-renders when parent state changes, which is normal React behavior. The real cost is that TrackList itself is a 584-line component. Consider splitting TrackList into smaller sub-components (see 7.1).

### 2.7 Search input not controlled for Enter key
**Severity:** Low | **Type:** UX | **Status:** CONFIRMED
**File:** `SearchPage.tsx:108-118`

The form uses `onSubmit` which is fine, but the button's `disabled={loading}` state (line 115) doesn't prevent form submission — the user can still press Enter while loading, triggering another search.

### 2.8 No debounce on search
**Severity:** Low | **Type:** Performance | **Status:** CONFIRMED
**File:** `SearchPage.tsx:108-118`

Not applicable to the current form-submit pattern, but if search were ever changed to live/typeahead, there's no debounce infrastructure in place.

### 2.9 Inconsistent empty state styling
**Severity:** Low | **Type:** Visual | **Status:** CONFIRMED
**File:** `SearchPage.tsx:127-155` vs `MusicPage.tsx`

The "no results" state uses `bg-panel2 border border-panel2` while the loading state uses `bg-panel border border-panel2`. These should be consistent.

### 2.10 Download button text doesn't change on re-search
**Severity:** Low | **Type:** UX | **Status:** CONFIRMED
**File:** `SearchPage.tsx:137-148`

After downloading, if the user searches again for the same query, the "Search & Download from Web" button still appears even though the track may now be in the library. The `searched` state is reset but the results from the previous download-triggered re-search (line 64) may not include the newly downloaded track yet.

---

## 3. BUGS

### 3.1 Race condition: search results stale after download
**Severity:** High | **Type:** Bug | **Status:** CONFIRMED
**File:** `SearchPage.tsx:60-64`

When a download succeeds, the code does:
```typescript
if (q.trim()) api.search(q.trim()).then(setResults);
```

This fires immediately after download completion, but the backend rescan is asynchronous. The search may not find the newly downloaded track yet. There's no retry logic or delay. Compare with `DownloadContext.tsx:266-304` which has a 3-minute retry loop with explicit `api.scan()` trigger.

### 3.2 setState after unmount from polling
**Severity:** Medium | **Type:** Bug | **Status:** CONFIRMED (clarified)
**File:** `SearchPage.tsx:60-64`

The cleanup effect (lines 18-22) DOES properly clear all intervals on unmount. However, the `api.search(q.trim()).then(setResults)` call on line 64 can resolve AFTER unmount (the fetch is already in-flight when the interval is cleared). There is no mounted ref check or AbortController to prevent `setResults` from firing on an unmounted component. This will cause a React "setState on unmounted component" warning in development.

### 3.3 Download state not reset on new search
**Severity:** Medium | **Type:** Bug | **Status:** CONFIRMED
**File:** `SearchPage.tsx:24-37`

When the user performs a new search while a download is in progress:
- `setSearched(true)` and `setLoading(true)` are called
- But `setDownloading(false)` is NOT called
- The `downloading` state from the previous search's download button persists incorrectly

### 3.4 Search query not URL-synced
**Severity:** Medium | **Type:** Bug | **Status:** CONFIRMED
**File:** `SearchPage.tsx:8-15`

The search query `q` is component state only. If the user refreshes the page, the search is lost. If the user navigates away and back, the search is lost. Should use URL query parameters (`/search?q=beatles`).

### 3.5 No cancellation of in-flight search on unmount
**Severity:** Medium | **Type:** Bug | **Status:** CONFIRMED
**File:** `SearchPage.tsx:24-37`

The `go` function's `api.search()` call is not cancellable. If the user types a query, presses Enter, then navigates away before the response arrives, the `setResults` call will fire on an unmounted component. Should use `AbortController`.

### 3.6 TrackList onDelete callback stale closure
**Severity:** Medium | **Type:** Bug | **Status:** CONFIRMED
**File:** `SearchPage.tsx:125`

```typescript
<TrackList tracks={results} onDelete={() => api.search(q.trim()).then(setResults)} />
```

The `onDelete` callback captures `q` from the closure. If `q` changes between when the callback is created and when it fires (after a track is deleted), the re-search will use the new query, not the original one. This is a stale closure bug.

### 3.7 Download button disabled state doesn't prevent form submission
**Severity:** Low | **Type:** Bug | **Status:** CONFIRMED
**File:** `SearchPage.tsx:115`

The search button has `disabled={loading}`, but this only affects the button. The form can still be submitted via Enter key while loading, triggering duplicate searches.

### 3.8 Duplicate download on rapid clicks
**Severity:** Medium | **Type:** Bug | **Status:** NEW
**File:** `SearchPage.tsx:83-94`

`handleDownloadSearch` doesn't check the `downloading` state before initiating a new download. If the user clicks the "Search & Download from Web" button multiple times rapidly, multiple download jobs will be spawned for the same query. The `disabled={downloading}` on the button (line 139) helps but doesn't prevent the first click from racing with the state update.

### 3.9 Download state stuck if query becomes empty
**Severity:** Low | **Type:** Bug | **Status:** NEW
**File:** `SearchPage.tsx:83-94`

If `handleDownloadSearch` is called when `q.trim()` is empty (line 84 early return), and `downloading` was previously set to `true`, the function returns without resetting `downloading` to `false`. This is unlikely in practice (the button is only shown when `q.trim()` is truthy), but it's a defensive gap.

---

## 4. VISUAL ISSUES

### 4.1 Search input placeholder text is too long
**Severity:** Low | **Type:** Visual | **Status:** CONFIRMED
**File:** `SearchPage.tsx:113`

`placeholder="Search title, artist, album, genre…"` — this is fine but could be more inviting. Compare with Spotify's "What do you want to listen to?"

### 4.2 No visual distinction between search page and music page
**Severity:** Low | **Type:** Visual | **Status:** CONFIRMED
**File:** `SearchPage.tsx:96-157`

Both pages show a TrackList in the same style. The search page could benefit from a more distinct visual identity — perhaps showing the search query as a heading above results, or showing result counts.

### 4.3 Download spinner uses emoji instead of Lucide icon
**Severity:** Low | **Type:** Visual | **Status:** CONFIRMED
**File:** `SearchPage.tsx:143`

```typescript
<span className="animate-spin">⟳</span>
```

This uses a raw Unicode character with CSS animation. Inconsistent with the rest of the app which uses Lucide icons. Should use `<Loader2 size={16} className="animate-spin" />` from lucide-react.

### 4.4 Empty state icon size inconsistent
**Severity:** Low | **Type:** Visual | **Status:** CONFIRMED
**File:** `SearchPage.tsx:128`

The `Music` icon is `size={32}` in the empty state. Compare with other empty states in the app which may use different sizes.

### 4.5 Help button positioning
**Severity:** Low | **Type:** Visual | **Status:** CONFIRMED
**File:** `SearchPage.tsx:98-107`

The help button is inline with the heading, which is fine, but the spacing `gap-2` between the h1 and the help button feels tight compared to other pages.

---

## 5. ACCESSIBILITY

### 5.1 Search input missing aria-label
**Severity:** Medium | **Type:** Accessibility | **Status:** CONFIRMED
**File:** `SearchPage.tsx:109-114`

The search input has a `placeholder` but no `aria-label` or associated `<label>` element. Screen readers will not have context for what this input does.

### 5.2 Search button text changes to "Searching…" but no aria-live region
**Severity:** Low | **Type:** Accessibility | **Status:** CONFIRMED
**File:** `SearchPage.tsx:115-117`

When loading, the button text changes to "Searching…" but there's no `aria-live` region or `aria-busy` state to announce this to screen readers.

### 5.3 No aria-live for search results
**Severity:** Medium | **Type:** Accessibility | **Status:** CONFIRMED
**File:** `SearchPage.tsx:120-156`

When search results appear or the "no results" state shows, there's no `aria-live` region to announce the change to screen readers.

### 5.4 Download button in empty state lacks aria-label
**Severity:** Low | **Type:** Accessibility | **Status:** CONFIRMED
**File:** `SearchPage.tsx:137-148`

The "Search & Download from Web" button has no `aria-label`. The text is descriptive enough, but an explicit label would be better.

### 5.5 No skip-to-content link
**Severity:** Low | **Type:** Accessibility | **Status:** CONFIRMED
**File:** `SearchPage.tsx:96`

The page has no skip navigation link for keyboard users.

### 5.6 Form lacks aria-label
**Severity:** Low | **Type:** Accessibility | **Status:** CONFIRMED
**File:** `SearchPage.tsx:108`

The `<form>` element has no `aria-label` or `role="search"`.

---

## 6. PERFORMANCE

### 6.1 No memoization of search results
**Severity:** Informational | **Type:** Performance | **Status:** DOWNGRADED
**File:** `SearchPage.tsx:13`

Original assessment overstated this. `results` is state — only set via `setResults`, not recreated per render. The real cost is TrackList being a 584-line component that re-renders on parent state changes.

### 6.2 Download polling creates new interval on each call
**Severity:** Informational | **Type:** Performance | **Status:** DOWNGRADED
**File:** `SearchPage.tsx:57-80`

The `pollRef.current[job.id]` check on line 54 does prevent duplicate intervals for the same job. Different jobs getting their own interval is correct behavior. Minor concern: no upper bound on total concurrent polls.

### 6.3 No virtualization for large result sets
**Severity:** Medium | **Type:** Performance | **Status:** CONFIRMED
**File:** `SearchPage.tsx:124-125`

All search results are rendered at once in TrackList. For large libraries with common search terms, this could be hundreds of DOM nodes. The MusicPage has pagination (Load More), but SearchPage doesn't.

---

## 7. CROSS-CUTTING CONCERNS (from supporting files)

### 7.1 TrackList is 584 lines with dual layout
**File:** `TrackList.tsx:1-584` | **Status:** CONFIRMED

TrackList is a massive component that handles both desktop table and mobile card views. It contains duplicated logic for:
- Playlist dropdown (DesktopTable lines 238-298 vs MobileCardList lines 518-576)
- Delete confirmation (DesktopTable lines 188-206 vs MobileCardList lines 469-485)
- Upgrade quality (DesktopTable lines 273-285 vs MobileCardList lines 551-563)

This should be refactored into shared sub-components.

### 7.2 PlayerContext is 688 lines
**File:** `PlayerContext.tsx:1-688` | **Status:** CONFIRMED

The player context is doing too much: local audio, Spotify SDK, WebSocket, podcast position tracking, loudness normalization, auto-skip, shuffle, repeat. Should be split into smaller contexts or hooks.

### 7.3 DownloadContext duplicates SearchPage download logic
**File:** `DownloadContext.tsx:92-174` vs `SearchPage.tsx:39-81` | **Status:** CONFIRMED

The `trackDownload` function exists in both places with nearly identical logic. SearchPage should use the shared context.

---

## PRIORITIZED FIX ROADMAP

### P0 — Critical (fix immediately)
1. **2.1** Fix silent error swallowing in search — pass error messages to toasts
2. **2.2** Fix silent error swallowing in download handler
3. **2.5** Use DownloadContext instead of duplicate polling logic (also fixes 2.4, 7.3)
4. **3.1** Fix race condition: add retry/delay after download before re-searching

### P1 — High (fix soon)
5. **3.2** Guard setState-after-unmount in polling (add mounted ref or AbortController)
6. **3.3** Reset download state on new search
7. **3.5** Cancel in-flight search requests on unmount (AbortController)
8. **3.6** Fix stale closure in onDelete callback (use ref for query)
9. **3.8** Guard against duplicate downloads in handleDownloadSearch
10. **1.15** Add pagination or virtualization for search results (also addresses 6.3)
11. **5.1** Add aria-label to search input
12. **5.3** Add aria-live for search results
13. **2.3** Add console.error to polling catch block

### P2 — Medium (fix when convenient)
14. **1.1** Add search history
15. **1.2** Add search filters (media kind, genre, year)
16. **1.4** Show search result count
17. **1.6** Add clear search button
18. **1.7** Add empty state with suggestions before first search
19. **1.14** Add "Play All" button for search results
20. **3.4** Sync search query to URL
21. **3.7** Disable form submission while loading
22. **3.9** Reset downloading state on early return
23. **4.3** Replace emoji spinner with Lucide Loader2 icon
24. **1.12** Differentiate error types in toast messages

### P3 — Low (nice to have)
25. **1.3** Add keyboard shortcut for search focus
26. **1.5** Add sorting of search results
27. **1.8** Add bulk actions on search results
28. **1.9** Add "add all to playlist" action
29. **1.10** Add search refinement
30. **1.11** Add loading skeleton
31. **1.13** Add result highlighting
32. **2.7** Improve Enter key handling during loading
33. **2.9** Consistent empty state styling
34. **2.10** Update download button state after re-search
35. **4.1-4.5** Visual polish
36. **5.2, 5.4, 5.5, 5.6** Additional accessibility improvements
37. **7.1** Refactor TrackList into sub-components
38. **7.2** Split PlayerContext into smaller contexts/hooks
