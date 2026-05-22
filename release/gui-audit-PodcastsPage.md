# GUI Audit: PodcastsPage.tsx

**Date:** 2026-05-22
**Auditor:** Atlas (researcher)
**Scope:** PodcastsPage.tsx + all referenced context/providers/components
**File:** `frontend/src/pages/PodcastsPage.tsx` (567 lines)

---

## 1. MISSING FEATURES

### 1.1 No episode sorting (HIGH)
- **Lines:** 341-434 (episode list rendering)
- Episodes are displayed in whatever order the API returns them. No sort options (newest first, oldest first, by duration, by download status).
- Every other list page (Music, Playlists, Downloads) has sorting or at least a consistent order. Podcasts should too.

### 1.2 No episode filtering/search (HIGH)
- **Lines:** 341-434
- No way to filter episodes by downloaded/not-downloaded, listened/unlistened, or by text search within a feed's episodes.
- For podcasts with hundreds of episodes, this is a major usability gap.

### 1.3 No pagination for episodes (MEDIUM)
- **Lines:** 341-434
- All episodes are loaded at once. Music page has Load More pagination. Podcasts should too, especially for feeds with 100+ episodes.

### 1.4 No bulk actions (MEDIUM)
- **Lines:** 341-434
- No "Download all undownloaded", "Mark all as listened", "Delete all downloaded" actions. The feed-level "Download all" exists (line 313) but there's no way to select multiple episodes for batch operations.

### 1.5 No episode context menu (MEDIUM)
- **Lines:** 407-429
- Each episode only has a single action button (Play OR Download). No "..." menu with options like "Mark as listened", "Copy episode URL", "Share", "Add to playlist" (downloaded episodes should be addable to playlists like any other track).
- Compare with TrackList.tsx which has a rich "..." context menu (lines 168-302).

### 1.6 No drag-and-drop reordering for feeds (LOW)
- **Lines:** 264-294
- Feed sidebar order is fixed (DB order). No way to reorder feeds by drag-and-drop or pin favorites to top.

### 1.7 No feed-level auto-download toggle in UI (MEDIUM)
- **Lines:** 411-422 (api.ts)
- The `PodcastFeed` type has `auto_download: boolean` (api.ts line 421), but the UI never exposes this setting. Users can't toggle auto-download per feed.

### 1.8 No podcast discovery/search actually works (HIGH)
- **Lines:** 547-561 (AddPodcastModal search tab)
- The "Search" tab in the Add Podcast modal is non-functional. It shows a text input and a tip to "Use the chat on the Discover page to find podcasts by topic!" but doesn't actually call any search API. It's a dead UI — the search query state is captured (line 474) but never used.

### 1.9 No episode description expand (LOW)
- **Lines:** 403-405
- Episode descriptions are shown with `line-clamp-2` but there's no way to expand them. Some podcast episodes have lengthy show notes.

### 1.10 No playback speed control for podcasts (MEDIUM)
- PlayerContext.tsx has no playback speed control. Podcast listeners commonly want 1.25x-2x speed. This is a player-level gap but affects podcast UX directly.

### 1.11 No "Mark as listened" / "Mark as unlistened" action (MEDIUM)
- **Lines:** 361-363
- Episodes show a "Listened" badge but there's no way to toggle it manually. The only way to mark as listened is through playback reaching the end.

### 1.12 No empty state for feed with zero episodes after sync (LOW)
- **Lines:** 336-339
- The empty state just says "No episodes found. Try syncing the feed." — but if the user just synced and there are truly no episodes, this is confusing. Should distinguish between "never synced" and "synced but empty".

### 1.13 No feed URL display or copy (LOW)
- **Lines:** 264-294
- The feed URL is stored (api.ts line 413) but never shown in the UI. Users can't see or copy the URL of a subscribed feed.

### 1.14 No last-synced timestamp display (LOW)
- **Lines:** 304-334
- The feed header shows title and description but not when it was last synced. The `last_fetched_at` field exists in the API type (api.ts line 420) but is never rendered.

### 1.15 No mobile-specific layout for podcasts (HIGH)
- **Lines:** 261-441
- The page uses a `lg:grid-cols-4` grid for sidebar + episodes. On mobile, this stacks vertically (no `lg:` prefix means single-column on small screens), but the episode list then appears BELOW the feed sidebar, requiring excessive scrolling. There's no mobile-optimized view (e.g., a back-navigated detail view for episodes, or a bottom sheet for feed selection).
- Compare with how MusicPage and other pages handle mobile — TrackList.tsx has a dedicated `MobileCardList` component (lines 312-383). Podcasts has no mobile-specific rendering at all.

---

## 2. POOR IMPLEMENTATIONS

### 2.1 Silent error swallowing in loadFeeds (HIGH)
- **Lines:** 44, 55-56
- Both `loadFeeds` and `loadEpisodes` have bare `catch { /* ignore */ }` blocks. If the API fails, the user sees an empty/broken state with no indication of what went wrong. This is the exact anti-pattern documented in the skill file's "Silent Error Swallowing" pitfall.
- **Fix:** At minimum, log to console. Better: set an error state and display it.

### 2.2 No error state for feed loading (HIGH)
- **Lines:** 32-49
- If `loadFeeds` fails, `loading` is set to `false` and the user sees the "No podcasts subscribed yet" empty state — even if they have feeds. This is misleading.
- **Fix:** Add an error state variable, display error message on failure.

### 2.3 Feed selection state can desync (MEDIUM)
- **Lines:** 36-43
- The `loadFeeds` callback uses a ref to read `selectedFeed` to avoid stale closures, but the logic for re-selecting the updated feed (line 41-42) only matches by `id`. If a feed is deleted by another client/tab, `selectedFeed` becomes stale and won't be cleared until the next loadFeeds call.
- **Fix:** After loadFeeds, if `selectedFeed` ID is not found in the new feeds array, set `selectedFeed` to null.

### 2.4 Download polling creates memory pressure (MEDIUM)
- **Lines:** 116-176
- Each episode download creates a 3-second polling interval. If a user downloads 10 episodes simultaneously, that's 10 intervals running concurrently, each making API calls. The intervals are cleaned up on unmount (lines 25-30) but there's no limit on concurrent polls.
- **Fix:** Use a single shared polling mechanism that batches episode status checks.

### 2.5 No deduplication of download polling (MEDIUM)
- **Lines:** 113-115
- The code clears an existing interval for the same episode before starting a new one, which is good. But if the user clicks "Download" twice rapidly, the first interval is cleared and the second one starts — the first download's completion toast will never fire.

### 2.6 Episode download button state is binary (LOW)
- **Lines:** 408-429
- An episode is either "downloaded" (shows Play) or "not downloaded" (shows Download). There's no visual distinction between "in library but not downloaded from this feed" vs "never downloaded". The `downloaded` flag is the only differentiator.

### 2.7 Help button placement is inconsistent (LOW)
- **Lines:** 232-238
- The help button is placed AFTER the "Add Podcast" button in the header, not aligned to the right. On other pages (e.g., MusicPage), help buttons are typically in the header area but placement varies. The help button at line 232 uses `showHelp("podcasts.feeds")` which is correct.

### 2.8 Modal has no loading state for URL input (LOW)
- **Lines:** 524-546
- The URL input field is not disabled while subscribing. A user could submit multiple times. The submit button is disabled when `subscribing || !url.trim()` (line 536) but the input itself remains editable.

### 2.9 Search tab in Add modal is misleading (HIGH)
- **Lines:** 547-561
- The "Search" tab shows a text input but does nothing with it. The placeholder says "e.g. true crime, tech news..." implying functionality that doesn't exist. This is a broken promise — users will type, press Enter, and nothing happens.
- **Fix:** Either implement search or remove the tab and keep only URL input.

### 2.10 No confirmation for "Download all episodes" (MEDIUM)
- **Lines:** 94-101, 312-318
- Clicking "Download all" immediately fires the API call with no confirmation dialog. For feeds with hundreds of episodes, this could trigger a massive download. Should at least show a confirmation with episode count.

---

## 3. BUGS

### 3.1 Stale closure in loadEpisodes useEffect (MEDIUM)
- **Lines:** 64-68
- `useEffect` depends on `[selectedFeed, loadEpisodes]`. When `selectedFeed` changes, `loadEpisodes` is called with `selectedFeed.id`. But `loadEpisodes` is wrapped in `useCallback` with `[]` deps (line 51), so it always uses the initial closure. This works because the effect re-runs when `selectedFeed` changes, but if `loadEpisodes` were used elsewhere, it would have a stale reference. The pattern is fragile.

### 3.2 Episode polling doesn't handle feed change correctly (MEDIUM)
- **Lines:** 116-176
- When polling for download completion, the interval callback reads `selectedFeedRef.current` (line 120). If the user switches feeds while a download is polling, the poll will try to load episodes for the NEW feed, not the one the download was started from. This could cause the download to appear "stuck" (never found in the new feed's episodes).

### 3.3 Unsubscribe doesn't stop active downloads (LOW)
- **Lines:** 83-92
- If the user unsubscribes from a feed while episodes are being downloaded, the download polling intervals continue running. The `handleUnsubscribe` function sets `selectedFeed` to null and reloads feeds, but doesn't clear `downloadingIds` or stop polling intervals for that feed's episodes.

### 3.4 handleDownloadFeed doesn't refresh episode list (LOW)
- **Lines:** 94-101
- After calling `podcastDownloadFeed`, the episode list is not refreshed. The user has to manually sync to see the download progress. Compare with `handleSync` (lines 70-81) which does call `loadEpisodes`.

### 3.5 Play button shown for downloaded episodes without checking track existence (LOW)
- **Lines:** 408-415
- The Play button is shown when `ep.downloaded && ep.file_path` is truthy. But if the file was deleted from disk (e.g., external cleanup), the track record still exists in the DB with `downloaded=true`. Clicking Play would fail silently or with a generic error.

### 3.6 No cleanup of downloadingIds on unmount (LOW)
- **Lines:** 25-30
- The unmount cleanup clears polling intervals but doesn't clear `downloadingIds`. If the component unmounts during downloads and remounts, the downloading state is lost (intervals are already cleared, so downloads appear stuck).

### 3.7 Feed image has no error handling (LOW)
- **Lines:** 276-280
- Feed images use `<img>` with no `onError` handler. Broken images show as broken icons. Compare with PlayerBar.tsx line 31 which uses `onError` to hide broken covers.

---

## 4. VISUAL ISSUES

### 4.1 Inconsistent header layout (LOW)
- **Lines:** 228-245
- The header has `flex items-center justify-between` but the help button (lines 232-238) is placed between the title and the "Add Podcast" button, not on the right side. This makes the layout look unbalanced.

### 4.2 Feed sidebar has no max-height/scroll (MEDIUM)
- **Lines:** 263-298
- The feed sidebar (`lg:col-span-1 space-y-2`) has no max-height or overflow scrolling. With many subscribed feeds, the sidebar grows indefinitely, pushing the episode list below the fold.

### 4.3 Episode cards don't show download progress (MEDIUM)
- **Lines:** 407-429
- When an episode is downloading, the button shows a spinner but there's no progress indicator. The DownloadsPage shows per-job progress bars. Podcasts should at least show the progress_label from the download job.

### 4.4 No visual distinction between "downloaded" and "in library" (LOW)
- **Lines:** 376-380
- Downloaded episodes show a green "Downloaded" badge. But episodes that were already in the library (added via scan from another source) don't show any indicator. Users can't tell what's available locally vs what needs downloading.

### 4.5 Episode error is truncated without tooltip (LOW)
- **Lines:** 381-385
- Download errors show as just "Error" with the full message in a `title` attribute. On mobile, `title` tooltips aren't accessible. Should show at least a truncated error message or an expandable error detail.

### 4.6 Progress bar uses accent color instead of distinct podcast color (LOW)
- **Lines:** 391-395
- The playback progress bar uses `bg-accent` which is the same color as everything else. Podcasts could use a distinct color (e.g., purple) to differentiate from music playback.

### 4.7 No responsive padding adjustment (LOW)
- **Lines:** 227
- The main container uses `space-y-4` but doesn't adjust padding for mobile. Compare with other pages that use `p-4` vs `p-6` responsive padding.

---

## 5. ACCESSIBILITY

### 5.1 Feed sidebar buttons lack aria-labels (MEDIUM)
- **Lines:** 265-293
- Feed buttons in the sidebar have no `aria-label`. Screen readers will read the title and episode count but won't indicate it's a clickable button for navigating to that feed.

### 5.2 Episode action buttons have inconsistent aria-labels (MEDIUM)
- **Lines:** 409-428
- The Play button has `title` but no `aria-label`. The Download button has no accessibility attributes at all.

### 5.3 Modal lacks focus trap (HIGH)
- **Lines:** 492-566
- The AddPodcastModal doesn't implement focus trapping. When open, focus can move to elements behind the overlay. The HelpModal (HelpModal.tsx lines 13-19) has Escape handling but also no focus trap.

### 5.4 No keyboard navigation for episode list (MEDIUM)
- **Lines:** 341-434
- Episodes are rendered as `<div>` elements, not focusable elements. Keyboard users can't navigate between episodes. Should use `<button>` or add `tabIndex` and keyboard handlers.

### 5.5 Help button in header lacks visible focus indicator (LOW)
- **Lines:** 232-238
- The help button has `p-1` padding but no `:focus-visible` styling. Keyboard users can't see when it's focused.

### 5.6 Modal close button lacks aria-label (LOW)
- **Lines:** 497-499
- The modal's X close button has no `aria-label`. Screen readers will just hear "button".

### 5.7 Episode progress bar is not accessible (LOW)
- **Lines:** 388-402
- The playback progress bar is a custom div, not a semantic element. Screen readers can't convey progress information.

---

## 6. PERFORMANCE

### 6.1 No memoization of episode rendering (MEDIUM)
- **Lines:** 341-434
- Every episode is re-rendered on any state change. With 100+ episodes, this causes unnecessary re-renders. Should use `React.memo` for episode cards.

### 6.2 Feed list re-renders entirely on selection change (LOW)
- **Lines:** 264-294
- All feed buttons re-render when `selectedFeed` changes because the `selectedFeed?.id === feed.id` check runs for every item. Should memoize feed items.

### 6.3 Polling intervals are per-episode, not batched (MEDIUM)
- **Lines:** 116-176
- Each downloading episode creates its own 3-second interval. A single interval that polls all active downloads would be more efficient.

### 6.4 No virtualization for long episode lists (MEDIUM)
- **Lines:** 341-434
- All episodes are rendered in the DOM regardless of scroll position. For feeds with 500+ episodes, this causes significant DOM bloat. Should use virtualization (e.g., `react-virtuoso` or similar).

### 6.5 loadFeeds called after every sync (LOW)
- **Lines:** 70-81
- `handleSync` calls `loadFeeds()` which reloads ALL feeds from the server, even though only the current feed's episodes changed. This is wasteful for accounts with many subscriptions.

---

## 7. CROSS-CUTTING CONCERNS (from referenced files)

### 7.1 PlayerContext podcast position save interval (INFO)
- **PlayerContext.tsx lines 342-346:** Podcast position is saved every 5 seconds during playback. This is reasonable but could be batched with the download polling to reduce API calls.

### 7.2 DownloadContext doesn't track podcast downloads separately (INFO)
- **DownloadContext.tsx:** The download tracking is generic (works for both music and podcast downloads). The `kind` field in DownloadJob (api.ts line 251) distinguishes them, but the PodcastsPage doesn't use the DownloadContext — it has its own polling mechanism.

### 7.3 No shared podcast state between pages (INFO)
- Podcast episode state is local to PodcastsPage. If a podcast episode is playing and the user navigates away, the episode context is lost. Consider lifting podcast playback state to a context provider.

---

## PRIORITY SUMMARY

### Critical (fix first):
1. **1.8** — Search tab in Add modal is non-functional dead UI
2. **2.1 / 2.2** — Silent error swallowing, no error state for feed loading
3. **1.15** — No mobile-specific layout
4. **5.3** — Modal lacks focus trap

### High:
1. **1.1** — No episode sorting
2. **1.2** — No episode filtering/search
3. **1.4** — No bulk actions
4. **2.9** — Search tab misleading
5. **3.2** — Episode polling breaks on feed change

### Medium:
1. **1.3** — No pagination
2. **1.5** — No episode context menu
3. **1.7** — No auto-download toggle in UI
4. **1.11** — No mark as listened/unlistened
5. **2.4 / 2.5** — Download polling memory/perf
6. **3.3** — Unsubscribe doesn't stop downloads
7. **4.2** — Feed sidebar no max-height
8. **6.1 / 6.4** — No memoization/virtualization

### Low:
1. **1.6, 1.9, 1.12, 1.13, 1.14** — Minor missing features
2. **2.6, 2.7, 2.8, 2.10** — Minor implementation issues
3. **3.4, 3.5, 3.6, 3.7** — Minor bugs
4. **4.1, 4.3, 4.4, 4.5, 4.6, 4.7** — Visual polish
5. **5.1, 5.2, 5.4, 5.5, 5.6, 5.7** — Accessibility polish
6. **6.2, 6.3, 6.5** — Minor perf issues
