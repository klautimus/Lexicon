# GUI Audit: RecsPage (Discover) — Comprehensive Plan

**Date:** 2026-05-22
**Auditor:** Atlas (researcher)
**Reviewer:** Atlas (analyst) — 2026-05-22
**Scope:** `frontend/src/pages/RecsPage.tsx` + supporting contexts, API, components
**File:** 415 lines
**Build Status:** Backend ✓ (go build clean), Frontend ✓ (vite build clean)

---

## REVIEW SUMMARY

45 findings reviewed against actual source code. Results:
- **30 CONFIRMED** — accurate findings, fix as described
- **6 FALSE_POSITIVE** — inaccurate or overstated, no fix needed
- **4 NEEDS_CONTEXT** — valid concern but requires design decision or is architectural
- **5 findings merged/deduplicated** — overlapping with other findings

### Key Changes from Original Plan
1. Removed false positives (1.6, 1.9, 2.5, 2.9, 3.6, 6.3) from fix roadmap
2. Downgraded several items to NEEDS_CONTEXT (2.2, 3.4, 4.1, 4.7)
3. Merged duplicates: 1.12+4.3 (same finding), 1.2+2.1 (same root cause)
4. Added 3 new findings the auditor missed (see section 9)
5. Re-prioritized roadmap based on actual severity

---

## 1. MISSING FEATURES

### 1.1 No loading state on initial recommendations load
**Severity:** Medium
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:35-42, 43-45

The `load()` function fetches existing recommendations on mount but there's no loading spinner or skeleton state. The page renders the empty "Discover New Music" placeholder while the API call is in flight. If the API returns data, the UI abruptly switches from empty state to populated recommendations with no transition.

**Fix:** Add a `loadingInitial` state (separate from the refresh `loading`) that shows a spinner while `load()` is in flight. Only show the empty state after `load()` completes with no data.

### 1.2 No error state for failed recommendation load
**Severity:** Medium
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:43-45

`load().catch(() => {})` silently swallows all errors. If the API is unreachable or returns 500, the user sees the empty "Discover New Music" state with no indication that something went wrong. They can't distinguish between "no recommendations yet" and "server error."

**Fix:** Add an error state variable. On `load()` failure, set it to the error message. Render a distinct error UI (e.g., red alert with retry button) instead of the empty state.

### 1.3 No error state for chat failures
**Severity:** Medium
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:78-80

Chat errors are appended to the chat log as `{ role: "ai", text: "Error: " + e.message }`. This is better than silent swallowing, but the error message is rendered in the same style as normal AI responses. There's no visual distinction (color, icon) to indicate this is an error.

**Fix:** Add a distinct visual style for error messages in the chat log — e.g., red text or an error icon prefix.

### 1.4 No "stop generating" for chat
**Severity:** Medium
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:62-84, 398-411

Once a chat message is sent, there's no way to cancel the in-flight request. The `chatBusy` state disables the send button but there's no abort/cancel mechanism. If the LLM is taking a long time, the user is stuck waiting.

**Fix:** Use `AbortController` to cancel the in-flight fetch. Add a "Stop" button that appears while `chatBusy` is true, which calls `abort()`.

### 1.5 No "stop generating" for playlist generation
**Severity:** Medium
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:122-129, DownloadContext.tsx:197-215

The "Generate Playlist" button shows "Thinking…" but there's no way to cancel the generation. The `generateAiPlaylist` function in DownloadContext has no abort mechanism.

**Fix:** Add abort support to `generateAiPlaylist` in DownloadContext. Show a cancel button during generation.

### 1.6 No keyboard shortcut for sending chat
**Severity:** Low
**Status:** [FALSE_POSITIVE]

The chat form uses `onSubmit` which handles Enter correctly. The original finding claimed users might not realize Enter works, but this is standard web form behavior. No fix needed.

### 1.7 No clear/reset chat history
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:30, 382-396

Once chat messages exist, there's no way to clear the conversation. The chat log persists for the component lifetime. If the user wants to start a fresh conversation, they must navigate away and back.

**Fix:** Add a "Clear chat" button that appears when `chatLog.length > 0`.

### 1.8 No empty state for recommendations with zero items
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:169-251

If `recs.items` is an empty array (`[]`), the code renders `<div className="grid grid-cols-1 md:grid-cols-2 gap-4">` with no children — an empty grid. The user sees a blank area with no indication of what happened.

**Fix:** Add a check: if `recs.items.length === 0`, show a message like "No recommendations available. Try refreshing."

### 1.9 No confirmation before overwriting playlist
**Severity:** Medium
**Status:** [FALSE_POSITIVE]

The original finding claimed that clicking "Regenerate" when a playlist exists would silently overwrite. However, the code shows:
- The "Create Playlist" button is disabled when `createdPlaylistId` exists (`disabled={downloads.creatingPlaylist || !!downloads.createdPlaylistId}`)
- The "Regenerate" button calls `generateAiPlaylist(true, trackCount)` which creates a new preview, not overwrite the existing playlist
- The existing playlist is safe

No fix needed.

### 1.10 No bulk download for all discover items
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:189-250

Each "Discover" item has its own Download button, but there's no "Download All" button. If the user wants to download all missing tracks, they must click each one individually.

**Fix:** Add a "Download All Missing" button that iterates through all discover items and triggers downloads for each.

### 1.11 No indication of which service generated recommendations
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:169-176

The recommendations panel shows a summary and trends but doesn't indicate whether Spotify/Apple Music data was used. Users might not understand why recommendations are generic (no connected services).

**Fix:** Add a small indicator like "Using Spotify + library data" or "Connect Spotify for better recommendations" below the summary.

### 1.12 Chat doesn't show typing/streaming indicator properly
**Severity:** Low
**Status:** [CONFIRMED] — MERGED with 4.3
**Location:** RecsPage.tsx:396

The "thinking…" text is appended below the chat log. It's not animated and doesn't look like a typing indicator. It's easy to miss.

**Fix:** Use an animated typing indicator (three pulsing dots) styled like modern chat apps.

---

## 2. POOR IMPLEMENTATIONS

### 2.1 `load()` called without error handling on mount
**Severity:** Medium
**Status:** [CONFIRMED] — MERGED with 1.2
**Location:** RecsPage.tsx:43-45

```tsx
useEffect(() => {
    load().catch(() => {});
}, []);
```

The empty catch swallows all errors. This is the most critical data-fetching path on the page and it has zero error visibility.

**Fix:** See 1.2 — add error state and UI.

### 2.2 `playLibraryItem` creates a new API call per play
**Severity:** Low
**Status:** [NEEDS_CONTEXT]
**Location:** RecsPage.tsx:86-89

```tsx
async function playLibraryItem(trackId: number) {
    const t = await api.track(trackId);
    player.play([t], 0);
}
```

The player's `play()` function requires `Track[]` objects. The recommendation items only have `track_id` (number), not the full Track object. So the API call IS necessary unless the RecItem type is extended to include full track data, or the player is modified to accept track IDs. This is an architectural consideration, not a simple fix.

**Fix:** Either extend RecItem to include full track data, or modify player to accept track IDs and resolve internally. Requires coordination between RecsPage and PlayerContext.

### 2.3 `statusIcon` function defined inside component body
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:91-104

`statusIcon` is a pure function that returns JSX based on a string. It's recreated on every render. While not a performance bottleneck for this page, it's a code smell.

**Fix:** Move `statusIcon` outside the component or extract to a shared utility.

### 2.4 Duplicate download status tracking keys
**Severity:** Medium
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:213, 218, 220, 228-230, DownloadContext.tsx:178

The download tracking uses `${artist} - ${title}` as the key in multiple places:
- `downloads.completedIds.has(\`${it.artist} - ${it.title}\`)` (RecsPage:213)
- `downloads.completedTrackIds[\`${it.artist} - ${it.title}\`]` (RecsPage:218)
- `downloads.downloadingIds.has(\`${it.artist} - ${it.title}\`)` (RecsPage:230)
- `downloadItem(title, artist)` uses `${artist} - ${title}` (DownloadContext:178)

This string concatenation pattern is repeated 6+ times across files. If the format ever changes, it must be updated everywhere. Worse, if an artist or title contains " - ", the key could collide.

**Fix:** Create a helper function `trackKey(artist, title)` and use it consistently. Consider using a more collision-resistant format.

### 2.5 Chat example prompts don't work on Enter key
**Severity:** Low
**Status:** [FALSE_POSITIVE]

The original finding claimed clicking an example prompt should auto-submit. However, the current behavior (set input text, user presses Enter) is correct and standard. The form's onSubmit handles Enter natively. No fix needed.

### 2.6 No memoization of recommendation cards
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:189-250

The recommendation grid maps over `recs.items` and creates card components inline. If the parent re-renders (e.g., due to chat state change), all cards re-render even though their content hasn't changed.

**Fix:** Extract the card to a memoized component: `const RecCard = React.memo(({ item }) => { ... })`.

### 2.7 Track count slider state is local, not persisted
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:33, 145-153

The `trackCount` state defaults to 25 and is not persisted. If the user changes it to 50, navigates away, and comes back, it resets to 25.

**Fix:** Persist to `localStorage` or lift to a user preference.

### 2.8 Playlist preview regenerate button has no loading state
**Severity:** Medium
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:287-293

```tsx
onClick={() => { downloads.generateAiPlaylist(true, trackCount); }}
disabled={downloads.generatingPlaylist}
```

The regenerate button shows "Generating..." text but the unicode refresh icon `\u{1F504}` doesn't animate. The button is disabled during generation but the text change is the only indicator.

**Fix:** Add `animate-spin` to the refresh icon during generation, matching the pattern used elsewhere.

### 2.9 Chat input not cleared on error
**Severity:** Low
**Status:** [FALSE_POSITIVE]

The original finding acknowledged "This is actually fine for the happy path." The input is cleared on send, which is correct behavior. No fix needed.

---

## 3. BUGS

### 3.1 Race condition: `load()` and `refresh()` can conflict
**Severity:** Medium
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:35-42, 47-60

Both `load()` and `refresh()` set `recs` and `createdAt` state. If `load()` is still in flight (from mount) and the user clicks "Generate" (which calls `refresh()`), the `load()` response could overwrite the fresh recommendations with stale data. The `mounted` ref prevents setting state on unmount, but doesn't prevent this race.

**Fix:** Use an `AbortController` or a request counter to invalidate stale responses.

### 3.2 `createdAt` timestamp inconsistency
**Severity:** Medium
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:38-40, 52-53

In `load()`, `createdAt` comes from `r.created_at` (server timestamp). In `refresh()`, it's set to `Math.floor(Date.now() / 1000)` (client timestamp). If the client clock is wrong, the "Last updated" time will be inconsistent between initial load and refresh.

**Fix:** Always use the server timestamp. Have the refresh endpoint return the created_at value.

### 3.3 Chat scroll-to-bottom not implemented
**Severity:** Medium
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:382-396

The chat container has `max-h-64 overflow-y-auto` but there's no `scrollToBottom` effect when new messages are added. If the chat log exceeds the max height, new messages appear below the visible area and the user doesn't see them without manually scrolling.

**Fix:** Add a `useEffect` that scrolls the chat container to the bottom when `chatLog` changes:
```tsx
const chatEndRef = useRef<HTMLDivElement>(null);
useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' });
}, [chatLog]);
```

### 3.4 Playlist track status doesn't update on RecsPage after creation
**Severity:** Medium
**Status:** [NEEDS_CONTEXT]
**Location:** RecsPage.tsx:309-332, DownloadContext.tsx:227-338

The `statusIcon` function is local to RecsPage but renders status strings from DownloadContext. This creates a coupling: RecsPage must know the exact status strings ("present", "completed", "downloading", "failed", "pending"). However, this currently works correctly because both files use the same string constants.

**Fix:** Extract `statusIcon` to a shared utility. Document the status string contract. This is a code quality issue, not a runtime bug.

### 3.5 Download tracking doesn't clean up on page navigation
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** DownloadContext.tsx:67-71, 107-171

The `pollRef` cleanup only runs on unmount. Since `DownloadProvider` wraps the entire app (App.tsx:272), it never unmounts during normal navigation. If a download is started on RecsPage and the user navigates away, the polling interval continues running in the background. This is mostly fine (it's global state), but the toast notifications will fire on whatever page the user is on, which could be confusing.

**Fix:** Consider scoping download tracking to the page that initiated it, or suppress toast notifications when not on the originating page.

### 3.6 `handleUpgrade` in TrackList has no error message display
**Severity:** Low
**Status:** [FALSE_POSITIVE]

The code shows `toast.error` is called on failure (TrackList.tsx:146, 422). The original finding acknowledged "This is acceptable for now." No fix needed.

### 3.7 Chat busy state doesn't prevent navigation
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:32, 406

If the user navigates away while `chatBusy` is true, the in-flight request continues but the response is discarded (the `mounted` ref check at line 71 prevents setting state). This is correct behavior, but the request still completes on the server side, wasting resources.

**Fix:** Use `AbortController` to cancel the request on unmount.

---

## 4. VISUAL ISSUES

### 4.1 Inconsistent button styling between Generate and Refresh
**Severity:** Low
**Status:** [NEEDS_CONTEXT]
**Location:** RecsPage.tsx:122-138

"Generate Playlist" uses `bg-panel2 border border-panel2 hover:border-accent` while "Refresh" uses `bg-accent text-bg`. This appears intentional — Refresh/Generate is the primary action, Generate Playlist is secondary. This is a design preference, not a defect.

**Fix:** Consider making "Generate Playlist" use a more prominent style if product decides it should be equally prominent.

### 4.2 Track count slider help button is tiny
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:154-160

The help button for track count uses `p-0.5` and `text-muted/50` making it very hard to see and click, especially on mobile.

**Fix:** Increase to `p-1` and `text-muted/70` minimum.

### 4.3 Chat "thinking…" text is unstyled
**Severity:** Low
**Status:** [CONFIRMED] — MERGED with 1.12
**Location:** RecsPage.tsx:396

`<p className="text-xs text-muted">thinking…</p>` — no animation, no icon. Looks like a bug or leftover text.

**Fix:** Use an animated typing indicator with a pulsing animation.

### 4.4 Playlist preview regenerate button uses unicode icon
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:292

`'\u{1F504} Regenerate'` uses a raw unicode emoji instead of a Lucide icon. This is inconsistent with the rest of the UI which uses Lucide icons exclusively. The emoji may not render on all systems.

**Fix:** Replace with `<RefreshCw size={12} />` from Lucide.

### 4.5 Recommendation cards don't show album art
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:191-248

The recommendation cards show title, artist, reason, and type badge, but no album art. For a music discovery page, visual appeal matters. The "From your library" items could show cover art since they have `track_id`.

**Fix:** For items with `track_id`, show the cover art thumbnail using `api.coverUrl(track_id)`.

### 4.6 Chat section info box takes up space permanently
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:350-359

The info box explaining chat capabilities is always visible, even after the user has used chat multiple times. It takes up valuable vertical space.

**Fix:** Collapse the info box after the first chat interaction, or make it dismissible.

### 4.7 No visual separator between recommendations and playlist preview
**Severity:** Low
**Status:** [NEEDS_CONTEXT]
**Location:** RecsPage.tsx:251-269

The playlist preview section appears directly below the recommendations grid with no visual separation beyond the natural `space-y-6` on the parent. This is a subjective design preference.

**Fix:** Add a heading or visual separator between the recommendations grid and the playlist preview if product decides it's needed.

---

## 5. ACCESSIBILITY

### 5.1 Chat messages lack ARIA live region
**Severity:** Medium
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:382-396

New chat messages appear in the DOM but screen readers won't announce them because there's no `aria-live` region. Users relying on assistive technology won't know when the AI responds.

**Fix:** Add `aria-live="polite"` to the chat messages container.

### 5.2 Chat input lacks proper label
**Severity:** Medium
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:399-404

The chat input has a `placeholder` but no associated `<label>` element or `aria-label`. Screen readers will announce it as "edit text" without context.

**Fix:** Add `aria-label="Chat message"` to the input.

### 5.3 Example prompt buttons lack context
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:366-378

The example prompt buttons have no `aria-label` beyond their text content. This is acceptable since the text is descriptive, but the `Wand2` icon is decorative and should be hidden from screen readers.

**Fix:** Add `aria-hidden="true"` to the Wand2 icon.

### 5.4 Recommendation cards are not keyboard-navigable as a group
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:189-250

The recommendation cards are plain `<div>` elements. The Play and Download buttons inside them are keyboard-accessible, but the cards themselves have no semantic meaning.

**Fix:** Consider using `<article>` or adding `role="listitem"` with a parent `role="list"`.

### 5.5 Track count slider lacks ARIA attributes
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:145-153

The range input has `className` but no `aria-label` or `aria-valuetext`. Screen readers will announce it as a slider with min/max but no context.

**Fix:** Add `aria-label="Number of tracks"` and `aria-valuetext` showing the current value.

### 5.6 Playlist preview track status icons lack text alternatives
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:310-331

The `statusIcon` function returns icons (Check, Loader2, "Failed" text) but these have no `aria-label` or `title` attribute. Screen readers won't convey the status.

**Fix:** Add `aria-label={status}` to the status icon container.

---

## 6. PERFORMANCE

### 6.1 `downloadItem` in DownloadContext has stale closure over `downloadingIds`
**Severity:** Medium
**Status:** [CONFIRMED]
**Location:** DownloadContext.tsx:176-195

```tsx
const downloadItem = useCallback(
    async (title: string, artist: string) => {
      const name = `${artist} - ${title}`;
      if (downloadingIds.has(name)) return;
      ...
    },
    [downloadingIds, toast, trackDownload]
  );
```

`downloadingIds` is a `Set` in state. The `useCallback` depends on it, so every time `downloadingIds` changes, the callback is recreated. This causes `RecsPage` to re-render because `downloads.downloadItem` is a new function reference. Since downloads can change frequently (every time any download starts/stops), this triggers unnecessary re-renders of the entire RecsPage.

**Fix:** Use a ref for `downloadingIds` in the check, or restructure to avoid the dependency.

### 6.2 `createAiPlaylist` processes tracks sequentially with polling
**Severity:** Medium
**Status:** [CONFIRMED]
**Location:** DownloadContext.tsx:227-338

The `for...of` loop with `await` inside means tracks are processed one at a time. Each track can take up to 3 minutes (60 retries × 3s). For a 25-track playlist, this could take 75 minutes. The backend supports concurrent downloads (semaphore default 2), but the frontend serializes them.

**Fix:** Process tracks in batches of 2-3 concurrently using `Promise.all` with a concurrency limit.

### 6.3 No virtualization for large recommendation lists
**Severity:** Low
**Status:** [FALSE_POSITIVE]

50 cards in a grid is not a performance issue. This is premature optimization. No fix needed.

### 6.4 `api.track()` called on every play click
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:86-89

Each play click triggers `api.track(trackId)` which is a network request. The track data is likely already available in the player's queue or could be passed directly.

**Fix:** Pass the track data directly if available, or cache track lookups.

---

## 7. NEW FINDINGS (Missed by Original Auditor)

### 7.1 Download polling interval is 2s but scanner wait is 3s
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** DownloadContext.tsx:170, 272

The `trackDownload` function polls every 2 seconds (line 170), but after a download succeeds, the `createAiPlaylist` function triggers a scan and then waits 3 seconds (line 272) before starting to search for the track. The first poll at 2s will likely fail because the scanner hasn't finished yet. This is a minor timing inefficiency.

**Fix:** Either increase the initial wait to 4s or start polling after the 3s wait completes.

### 7.2 `adoptPlaylistPreview` doesn't check for existing `createdPlaylistId`
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** DownloadContext.tsx:79-90

When `adoptPlaylistPreview` is called (from chat), it resets `createdPlaylistId` to null and reinitializes track status. If the user had already created a playlist from a previous chat response, adopting a new preview will lose the reference to the created playlist.

**Fix:** Consider preserving `createdPlaylistId` or warning the user when adopting a new preview.

### 7.3 Chat error messages expose raw API errors to users
**Severity:** Low
**Status:** [CONFIRMED]
**Location:** RecsPage.tsx:80

`setChatLog((l) => [...l, { role: "ai", text: "Error: " + e.message }])` — raw error messages from the API (which may include internal details like status codes, URLs, or stack traces) are displayed directly to users.

**Fix:** Sanitize error messages before displaying. Show a user-friendly message like "Something went wrong. Please try again." and log the actual error to console.

---

## 8. PRIORITIZED FIX ROADMAP (REVISED)

### Phase 1: Critical Bugs & Error Handling (fix first)
1. **1.2/2.1** — No error state for failed recommendation load (merged)
2. **1.4** — No "stop generating" for chat (AbortController)
3. **3.1** — Race condition between `load()` and `refresh()`
4. **3.2** — `createdAt` timestamp inconsistency
5. **3.3** — Chat scroll-to-bottom not implemented
6. **7.3** — Chat error messages expose raw API errors

### Phase 2: Missing Features (high value)
7. **1.1** — Loading state on initial recommendations load
8. **1.3** — Distinct visual style for chat error messages
9. **1.5** — No "stop generating" for playlist generation
10. **1.7** — Clear/reset chat history button
11. **1.8** — Empty state for zero recommendation items
2. **1.10** — Bulk download for all discover items
3. **1.12/4.3** — Animated typing indicator for chat (merged)

### Phase 3: Poor Implementations & Performance
4. **2.4** — Extract `trackKey()` helper to avoid string duplication
5. **2.6** — Memoize recommendation cards
6. **2.8** — Playlist preview regenerate button loading animation
7. **4.4** — Replace unicode emoji with Lucide icon
8. **6.1** — Stale closure over `downloadingIds` causing re-renders
9. **6.2** — Sequential track processing in `createAiPlaylist`
10. **6.4** — `api.track()` called on every play click

### Phase 4: Accessibility
11. **5.1** — ARIA live region for chat messages
12. **5.2** — Chat input label
13. **5.3** — Wand2 icon aria-hidden
14. **5.4** — Recommendation cards semantic markup
15. **5.5** — Track count slider ARIA attributes
16. **5.6** — Playlist track status text alternatives

### Phase 5: Visual Polish
17. **4.2** — Track count slider help button size
18. **4.5** — Show album art on recommendation cards
19. **4.6** — Collapsible chat info box
20. **1.11** — Service indicator for recommendation source
21. **2.3** — Extract `statusIcon` from component body
22. **2.7** — Persist track count slider
23. **3.5** — Download tracking toast suppression on navigation
24. **3.7** — AbortController for chat on unmount
25. **7.1** — Download polling/scanner timing alignment
26. **7.2** — adoptPlaylistPreview createdPlaylistId handling

### Phase 6: Nice to Have / Design Decisions Required
27. **2.2** — playLibraryItem unnecessary API call (needs architecture decision)
28. **3.4** — statusIcon coupling (needs shared utility)
29. **4.1** — Button styling consistency (design preference)
30. **4.7** — Visual separator between sections (design preference)

---

## 9. CROSS-REFERENCE: RELATED FILES

| File | Lines | Relevance |
|------|-------|-----------|
| `RecsPage.tsx` | 415 | Primary audit target |
| `DownloadContext.tsx` | 374 | AI playlist generation, download tracking |
| `PlayerContext.tsx` | 688 | Play functionality from recommendations |
| `api.ts` | 460 | API types and methods |
| `ToastContext.tsx` | 93 | Error/success feedback |
| `HelpContext.tsx` | ~40 | Help system integration |
| `help-content.ts` | 420 | Discover page help entries |
| `TrackList.tsx` | 584 | Shared track list (not used by RecsPage but referenced) |
| `PlayerBar.tsx` | 154 | Player controls |
