# GUI Audit: PodcastsPage.tsx — REVISED

**Date:** 2026-05-22
**Reviewer:** Atlas (analyst)
**Scope:** PodcastsPage.tsx + all referenced context/providers/components
**Original:** gui-audit-PodcastsPage.md (40 findings)
**Build Status:** Backend OK, Frontend OK (both compile clean)

---

## VERIFICATION SUMMARY

| Category | Original | Confirmed | Rejected | New | Revised Severity |
|----------|----------|-----------|----------|-----|-----------------|
| Missing Features | 15 | 13 | 1 | 1 | 1 changed |
| Poor Implementations | 10 | 9 | 0 | 0 | 1 changed |
| Bugs | 7 | 6 | 0 | 1 | 0 changed |
| Visual Issues | 7 | 5 | 1 | 0 | 1 changed |
| Accessibility | 7 | 7 | 0 | 0 | 0 changed |
| Performance | 5 | 5 | 0 | 0 | 0 changed |
| Cross-Cutting | 3 | 3 | 0 | 0 | N/A |
| **TOTAL** | **54** | **48** | **2** | **2** | **3 changed** |

---

## 1. MISSING FEATURES

### 1.1 No episode sorting (CONFIRMED — MEDIUM)
- **Lines:** 341-434 (episode list rendering)
- **Verdict:** Episodes are rendered in whatever order `api.podcastEpisodes()` returns. No sort UI exists. The audit said HIGH but this is a MEDIUM — it's a convenience feature, not a broken workflow. MusicPage has column-header sorting; Podcasts has nothing.
- **Note:** The `pub_date` field exists on episodes (line 366-369) so date sorting would be trivial to add.

### 1.2 No episode filtering/search (CONFIRMED — HIGH)
- **Lines:** 341-434
- **Verdict:** Confirmed. No filter UI at all. For feeds with hundreds of episodes this is a real usability gap.

### 1.3 No pagination for episodes (CONFIRMED — MEDIUM)
- **Lines:** 341-434
- **Verdict:** Confirmed. All episodes loaded at once via `api.podcastEpisodes(feedId)`. Music page has Load More; Podcasts doesn't.

### 1.4 No bulk actions (CONFIRMED — MEDIUM)
- **Lines:** 341-434
- **Verdict:** Confirmed. Feed-level "Download all" exists (line 312-318) but no multi-select or batch operations for episodes.

### 1.5 No episode context menu (CONFIRMED — MEDIUM)
- **Lines:** 407-429
- **Verdict:** Confirmed. Each episode has only a single action button (Play OR Download). No "..." menu. TrackList.tsx has a rich context menu (lines 168-298) with add-to-playlist, create playlist, upgrade quality, delete. Podcasts has none of this.

### 1.6 No drag-and-drop reordering for feeds (CONFIRMED — LOW)
- **Lines:** 264-294
- **Verdict:** Confirmed. Feed order is whatever the DB returns. No reordering mechanism.

### 1.7 No feed-level auto-download toggle in UI (CONFIRMED — MEDIUM)
- **Lines:** api.ts line 421
- **Verdict:** Confirmed. `PodcastFeed.auto_download: boolean` exists in the type (api.ts:421) but is never rendered or toggleable in the UI.

### 1.8 No podcast search actually works (CONFIRMED — HIGH)
- **Lines:** 547-561
- **Verdict:** Confirmed. The Search tab captures `searchQuery` state (line 474) but never calls any API. The input is purely decorative. The tip text says "Use the chat on the Discover page" which confirms this is intentionally unimplemented, but the UI is still misleading — it looks functional.

### 1.9 No episode description expand (CONFIRMED — LOW)
- **Lines:** 403-405
- **Verdict:** Confirmed. `line-clamp-2` with no expand mechanism.

### 1.10 No playback speed control for podcasts (CONFIRMED — MEDIUM)
- **Verdict:** Confirmed. PlayerContext.tsx has no playback speed state or UI. The `PlayerCtx` interface (lines 41-51) has no `setPlaybackSpeed` or `playbackSpeed` field. This is a player-level gap that affects podcast UX directly since podcast listeners commonly use 1.25x-2x.

### 1.11 No "Mark as listened" / "Mark as unlistened" action (CONFIRMED — MEDIUM)
- **Lines:** 361-363
- **Verdict:** Confirmed. The "Listened" badge is display-only. No toggle action exists. The only way to mark as listened is through playback reaching the end (via `savePodcastEpisodePosition` with `completed: true`).

### 1.12 No empty state distinction (CONFIRMED — LOW)
- **Lines:** 336-339
- **Verdict:** Confirmed. Single message "No episodes found. Try syncing the feed." regardless of whether the feed was never synced or synced but empty.

### 1.13 No feed URL display or copy (CONFIRMED — LOW)
- **Lines:** 264-294
- **Verdict:** Confirmed. `feed.url` exists (api.ts:413) but is never rendered.

### 1.14 No last-synced timestamp display (CONFIRMED — LOW)
- **Lines:** 304-334
- **Verdict:** Confirmed. `feed.last_fetched_at` exists (api.ts:420) but is never rendered.

### 1.15 No mobile-specific layout for podcasts (CONFIRMED — HIGH)
- **Lines:** 261-441
- **Verdict:** Confirmed. The page uses `grid grid-cols-1 lg:grid-cols-4` which stacks vertically on mobile, but the episode list appears below the full feed sidebar — requiring excessive scrolling to reach episodes. TrackList.tsx has a dedicated `MobileCardList` component (lines 312-383). Podcasts has zero mobile-specific rendering. The `useIsMobile` hook is not imported or used.

### 1.16 NEW: No "Add to playlist" for downloaded podcast episodes (MEDIUM)
- **Lines:** 407-429
- **Verdict:** Downloaded podcast episodes become tracks in the library (via `podcastEpisodeTrack`), but there's no way to add them to playlists from the PodcastsPage. TrackList.tsx supports this via the context menu. Podcast episodes that are downloaded should be addable to playlists like any other track.

---

## 2. POOR IMPLEMENTATIONS

### 2.1 Silent error swallowing in loadFeeds (CONFIRMED — HIGH)
- **Lines:** 44, 55-56
- **Verdict:** Confirmed. Both `loadFeeds` (line 44) and `loadEpisodes` (line 55) have bare `catch { /* ignore */ }`. No console.log, no error state, no user feedback.

### 2.2 No error state for feed loading (CONFIRMED — HIGH)
- **Lines:** 32-49
- **Verdict:** Confirmed. If `loadFeeds` fails, `loading` becomes false and the user sees "No podcasts subscribed yet" even if they have feeds. No error state variable exists.

### 2.3 Feed selection state can desync (CONFIRMED — MEDIUM)
- **Lines:** 36-43
- **Verdict:** Confirmed. The `loadFeeds` callback re-selects by `id` (line 41-42) but if a feed is deleted by another client/tab, `selectedFeed` becomes stale. The code only handles "feed updated" and "first load" cases, not "feed deleted."

### 2.4 Download polling creates memory pressure (CONFIRMED — MEDIUM)
- **Lines:** 116-176
- **Verdict:** Confirmed. Each episode download creates its own 3-second `setInterval`. 10 concurrent downloads = 10 intervals. No batching or shared polling mechanism.

### 2.5 No deduplication of download polling (CONFIRMED — MEDIUM)
- **Lines:** 113-115
- **Verdict:** Confirmed. The code clears an existing interval before starting a new one (lines 114-115), which is good. But rapid double-clicking on Download means the first interval is cleared and the second starts — the first download's completion toast will never fire because the episode lookup happens in the second interval's closure.

### 2.6 Episode download button state is binary (CONFIRMED — LOW)
- **Lines:** 408-429
- **Verdict:** Confirmed. The button is either Play (downloaded) or Download (not downloaded). No "downloading" state distinction beyond the spinner, no progress percentage, no "queued" state.

### 2.7 Help button placement is inconsistent (CONFIRMED — LOW)
- **Lines:** 232-238
- **Verdict:** Confirmed. The help button is placed between the title and the "Add Podcast" button in the header. It's not aligned to the right side. The layout is: `[title] [help] [add podcast]` instead of `[title] [spacer] [help] [add podcast]`.

### 2.8 Modal has no loading state for URL input (CONFIRMED — LOW)
- **Lines:** 524-546
- **Verdict:** Confirmed. The URL input field remains editable while `subscribing` is true. Only the submit button is disabled (line 536). The input should be disabled during submission.

### 2.9 Search tab in Add modal is misleading (CONFIRMED — HIGH)
- **Lines:** 547-561
- **Verdict:** Confirmed. The Search tab shows a text input with placeholder "e.g. true crime, tech news..." but does nothing. The `searchQuery` state is set but never used in any API call or handler. This is a broken promise — users will type, press Enter, and nothing happens. The form has no `onSubmit` handler for the search tab.

### 2.10 No confirmation for "Download all episodes" (CONFIRMED — MEDIUM)
- **Lines:** 94-101, 312-318
- **Verdict:** Confirmed. `handleDownloadFeed` immediately calls `api.podcastDownloadFeed(feedId)` with no confirmation dialog. For feeds with hundreds of episodes this could trigger a massive download.

---

## 3. BUGS

### 3.1 Stale closure in loadEpisodes useEffect (CONFIRMED — MEDIUM)
- **Lines:** 64-68
- **Verdict:** Confirmed. `loadEpisodes` is wrapped in `useCallback` with `[]` deps (line 51), so it always uses the initial closure. The effect depends on `[selectedFeed, loadEpisodes]` — when `selectedFeed` changes, the effect re-runs and calls `loadEpisodes(selectedFeed.id)`. This works by accident because the effect re-runs, but if `loadEpisodes` were used elsewhere it would have a stale reference. The pattern is fragile.

### 3.2 Episode polling doesn't handle feed change correctly (CONFIRMED — MEDIUM)
- **Lines:** 116-176
- **Verdict:** Confirmed. The polling interval reads `selectedFeedRef.current` (line 120) on each tick. If the user switches feeds while a download is polling, the poll will load episodes for the NEW feed. The downloaded episode won't be found in the new feed's episodes, so it'll hit the "episode no longer exists" path (line 133-142) and stop polling — the download appears "stuck" with no completion toast.

### 3.3 Unsubscribe doesn't stop active downloads (CONFIRMED — LOW)
- **Lines:** 83-92
- **Verdict:** Confirmed. `handleUnsubscribe` sets `selectedFeed` to null and reloads feeds, but doesn't clear `downloadingIds` or stop polling intervals for that feed's episodes. The polling intervals will continue running. When they check `selectedFeedRef.current` (now null), they'll hit the `!currentFeed` path (line 121-129) and clean up — but this is incidental, not explicit cleanup.

### 3.4 handleDownloadFeed doesn't refresh episode list (CONFIRMED — LOW)
- **Lines:** 94-101
- **Verdict:** Confirmed. After calling `podcastDownloadFeed`, no episode list refresh occurs. Compare with `handleSync` (lines 70-81) which calls both `loadFeeds()` and `loadEpisodes()`.

### 3.5 Play button shown for downloaded episodes without checking track existence (CONFIRMED — LOW)
- **Lines:** 408-415
- **Verdict:** Confirmed. The Play button is shown when `ep.downloaded && ep.file_path` is truthy. If the file was deleted from disk externally, the DB record still has `downloaded=true`. Clicking Play calls `handlePlayEpisode` which calls `api.podcastEpisodeTrack(episodeId)` — this will fail if the track record is missing, showing a toast error. So it's not entirely silent, but the error message is generic.

### 3.6 No cleanup of downloadingIds on unmount (CONFIRMED — LOW)
- **Lines:** 25-30
- **Verdict:** Confirmed. The unmount cleanup (lines 25-30) clears polling intervals but doesn't clear `downloadingIds`. If the component unmounts during downloads and remounts, the downloading state is lost.

### 3.7 Feed image has no error handling (CONFIRMED — LOW)
- **Lines:** 276-280
- **Verdict:** Confirmed. The `<img>` tag at line 276 has no `onError` handler. Compare with PlayerBar.tsx line 31 which uses `onError` to hide broken covers.

### 3.8 NEW: Search tab Enter key does nothing (LOW)
- **Lines:** 547-561
- **Verdict:** The search input (line 552-556) has no `onSubmit` handler and is not wrapped in a `<form>`. Pressing Enter in the input does nothing. Even if the search were implemented, there's no submit handler wired up.

---

## 4. VISUAL ISSUES

### 4.1 Inconsistent header layout (CONFIRMED — LOW)
- **Lines:** 228-245
- **Verdict:** Confirmed. The header has `flex items-center justify-between` but the help button (lines 232-238) is placed between the title and the "Add Podcast" button. The layout reads left-to-right as: title, help, add-button. The help button should be on the far right or grouped with the add button.

### 4.2 Feed sidebar has no max-height/scroll (CONFIRMED — MEDIUM)
- **Lines:** 263-298
- **Verdict:** Confirmed. The sidebar (`lg:col-span-1 space-y-2`) has no `max-h-*` or `overflow-y-auto` classes. With many feeds, the sidebar grows indefinitely.

### 4.3 Episode cards don't show download progress (CONFIRMED — MEDIUM)
- **Lines:** 407-429
- **Verdict:** Confirmed. When downloading, the button shows a spinner but no progress percentage or label. The DownloadsPage/DownloadProgressBar shows per-job progress bars with `progress_label` (e.g., "45.2%"). PodcastsPage doesn't surface this.

### 4.4 No visual distinction between "downloaded" and "in library" (REJECTED)
- **Lines:** 376-380
- **Verdict:** REJECTED. This finding is not actionable. The `downloaded` flag on `PodcastEpisode` specifically means "this episode has been downloaded to local storage." There is no separate "in library" concept for podcast episodes — they become tracks in the library after download. The green "Downloaded" badge is the correct indicator. The audit was confused about the data model.

### 4.5 Episode error is truncated without tooltip (CONFIRMED — LOW)
- **Lines:** 381-385
- **Verdict:** Confirmed. Download errors show as just "Error" with the full message in a `title` attribute (line 382). On mobile, `title` tooltips aren't accessible.

### 4.6 Progress bar uses accent color (CONFIRMED — LOW)
- **Lines:** 391-395
- **Verdict:** Confirmed. The playback progress bar uses `bg-accent` (line 393). This is a minor visual polish issue — podcasts could use a distinct color.

### 4.7 No responsive padding adjustment (CONFIRMED — LOW)
- **Lines:** 227
- **Verdict:** Confirmed. The main container uses `space-y-4` with no responsive padding. Other pages use `p-4 md:p-6` patterns.

---

## 5. ACCESSIBILITY

### 5.1 Feed sidebar buttons lack aria-labels (CONFIRMED — MEDIUM)
- **Lines:** 265-293
- **Verdict:** Confirmed. Feed buttons have no `aria-label`. Screen readers will read the title and episode count but won't indicate it's a clickable button for navigating to that feed.

### 5.2 Episode action buttons have inconsistent aria-labels (CONFIRMED — MEDIUM)
- **Lines:** 409-428
- **Verdict:** Confirmed. The Play button (line 409) has `title` but no `aria-label`. The Download button (line 417) has no accessibility attributes at all.

### 5.3 Modal lacks focus trap (CONFIRMED — HIGH)
- **Lines:** 492-566
- **Verdict:** Confirmed. The AddPodcastModal doesn't implement focus trapping. When open, focus can move to elements behind the overlay. HelpModal.tsx (lines 13-19) has Escape handling but also no focus trap.

### 5.4 No keyboard navigation for episode list (CONFIRMED — MEDIUM)
- **Lines:** 341-434
- **Verdict:** Confirmed. Episodes are rendered as `<div>` elements (line 353), not focusable elements. No `tabIndex` or keyboard handlers.

### 5.5 Help button in header lacks visible focus indicator (CONFIRMED — LOW)
- **Lines:** 232-238
- **Verdict:** Confirmed. The help button has `p-1` padding but no `:focus-visible` styling.

### 5.6 Modal close button lacks aria-label (CONFIRMED — LOW)
- **Lines:** 497-499
- **Verdict:** Confirmed. The modal's X close button has no `aria-label`. Screen readers will just hear "button."

### 5.7 Episode progress bar is not accessible (CONFIRMED — LOW)
- **Lines:** 388-402
- **Verdict:** Confirmed. The playback progress bar is a custom div, not a semantic element. Screen readers can't convey progress information.

---

## 6. PERFORMANCE

### 6.1 No memoization of episode rendering (CONFIRMED — MEDIUM)
- **Lines:** 341-434
- **Verdict:** Confirmed. Every episode is re-rendered on any state change. No `React.memo` on episode cards.

### 6.2 Feed list re-renders entirely on selection change (CONFIRMED — LOW)
- **Lines:** 264-294
- **Verdict:** Confirmed. All feed buttons re-render when `selectedFeed` changes.

### 6.3 Polling intervals are per-episode, not batched (CONFIRMED — MEDIUM)
- **Lines:** 116-176
- **Verdict:** Confirmed. Each downloading episode creates its own 3-second interval.

### 6.4 No virtualization for long episode lists (CONFIRMED — MEDIUM)
- **Lines:** 341-434
- **Verdict:** Confirmed. All episodes are rendered in the DOM regardless of scroll position.

### 6.5 loadFeeds called after every sync (CONFIRMED — LOW)
- **Lines:** 70-81
- **Verdict:** Confirmed. `handleSync` calls `loadFeeds()` which reloads ALL feeds from the server.

---

## 7. CROSS-CUTTING CONCERNS (from referenced files)

### 7.1 PlayerContext podcast position save interval (CONFIRMED — INFO)
- **PlayerContext.tsx lines 342-346:** Podcast position is saved every 5 seconds during playback. This is reasonable.

### 7.2 DownloadContext doesn't track podcast downloads separately (CONFIRMED — INFO)
- **DownloadContext.tsx:** The download tracking is generic. The `kind` field in DownloadJob (api.ts line 251) distinguishes them, but the PodcastsPage doesn't use the DownloadContext — it has its own polling mechanism. This is a design inconsistency.

### 7.3 No shared podcast state between pages (CONFIRMED — INFO)
- Podcast episode state is local to PodcastsPage. If a podcast episode is playing and the user navigates away, the episode context is lost.

---

## PRIORITIZED IMPLEMENTATION ROADMAP

### Phase 1: Critical Fixes (break user workflows)
1. **2.1/2.2** — Add error handling to loadFeeds/loadEpisodes (error state + user feedback)
2. **2.9/1.8** — Remove or implement the Search tab in AddPodcastModal
3. **3.2** — Fix episode polling to track the feed it was started from, not selectedFeedRef
4. **5.3** — Add focus trap to AddPodcastModal

### Phase 2: High-Impact UX (major usability gaps)
5. **1.15** — Add mobile-specific layout (useIsMobile + MobileCardList pattern from TrackList)
6. **1.2** — Add episode filtering (downloaded/not, listened/unlistened, text search)
7. **1.1** — Add episode sorting (by date, duration, download status)
8. **1.5** — Add episode context menu (add to playlist, mark listened, delete)
9. **1.16** — Add "Add to playlist" for downloaded podcast episodes
10. **2.10** — Add confirmation dialog for "Download all episodes"
11. **1.7** — Add auto-download toggle per feed

### Phase 3: Medium Polish (noticeable improvements)
12. **1.3** — Add pagination for episodes (Load More pattern)
13. **1.4** — Add bulk actions (download all undownloaded, mark all listened)
14. **1.11** — Add "Mark as listened/unlistened" toggle
15. **1.10** — Add playback speed control to PlayerContext
16. **2.4/2.5/6.3** — Refactor download polling to use a single shared interval
17. **3.3** — Explicitly stop downloads on unsubscribe
18. **3.4** — Refresh episode list after handleDownloadFeed
19. **4.2** — Add max-height + overflow scrolling to feed sidebar
20. **4.3** — Show download progress percentage on episode cards
21. **6.1/6.4** — Add memoization and virtualization for episode list
22. **5.1/5.2/5.4** — Add aria-labels and keyboard navigation

### Phase 4: Low Polish (nice-to-have)
23. **1.6** — Feed reordering (drag-and-drop or pin)
24. **1.9** — Episode description expand/collapse
25. **1.12** — Distinguish "never synced" vs "synced but empty" states
26. **1.13** — Show/copy feed URL
27. **1.14** — Show last-synced timestamp
28. **2.6** — Richer download button states
29. **2.7** — Fix help button placement in header
30. **2.8** — Disable URL input while subscribing
31. **3.1** — Fix stale closure pattern in loadEpisodes
32. **3.5** — Validate track existence before showing Play button
33. **3.6** — Clear downloadingIds on unmount
34. **3.7** — Add onError handler to feed images
35. **3.8** — Wire up search tab Enter key (when search is implemented)
36. **4.1** — Fix header layout
37. **4.5** — Show expandable error details
38. **4.6** — Use distinct color for podcast progress bar
39. **4.7** — Add responsive padding
40. **5.5/5.6/5.7** — Accessibility polish (focus indicators, aria-labels, semantic progress)
41. **6.2/6.5** — Minor perf optimizations (memoize feed items, avoid full feed reload on sync)

---

## SEVERITY CHANGES FROM ORIGINAL

| Finding | Original | Revised | Reason |
|---------|----------|---------|--------|
| 1.1 No sorting | HIGH | MEDIUM | Convenience feature, not broken workflow |
| 4.4 Downloaded vs in-library | LOW | REJECTED | Not a real distinction in the data model |
| 2.7 Help button placement | LOW | LOW (unchanged) | Confirmed but very minor |

---

## REJECTED FINDINGS

1. **4.4** — "No visual distinction between downloaded and in library" — The `downloaded` flag IS the "in library" indicator for podcast episodes. There's no separate concept. The finding reflects a misunderstanding of the data model.

---

## NEW FINDINGS

1. **1.16** — No "Add to playlist" for downloaded podcast episodes (MEDIUM)
2. **3.8** — Search tab Enter key does nothing (LOW)
