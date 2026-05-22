# GUI Audit: PlaylistsPage + PlaylistPage

**Date:** 2026-05-22
**Auditor:** Atlas (researcher)
**Scope:** PlaylistsPage.tsx, PlaylistPage.tsx, and all supporting files
**Severity scale:** Critical > High > Medium > Low > Cosmetic

---

## 1. BUGS (Broken behavior)

### B1. [CRITICAL] PlaylistPage: `error` state shown but never set on 404 from backend
**File:** `PlaylistPage.tsx:47-49`
The `catch` block sets `setError(e instanceof Error ? e.message : String(e))`, but the backend `get()` handler (playlists.go:150-158) returns HTTP 404 with body `{"error":"not found"}`. The `api` client (`j()` in api.ts:46) throws `Error('404 {"error":"not found"}')` for non-200 responses. So the error message shown to the user is the raw HTTP status + JSON body — not a friendly message. The "Playlist not found." text at line 105 is always shown when `playlist` is null, regardless of whether it was a 404 or a network error. There's no way for the user to distinguish "playlist was deleted" from "server is down."

### B2. [HIGH] PlaylistPage: `onRemove` passes `t.position ?? i` but position is 1-based from backend
**File:** `PlaylistPage.tsx:232, 258`
The `PlaylistTrackList` component calls `onRemove(t.position ?? i)`. The backend `removeTrack` handler uses the `position` column from `playlist_items`. The `position` field is 0-based (set by `addTrack` which starts at 0 and increments). However, the `i` index from `.map()` is also 0-based. So `t.position ?? i` should be correct. BUT: the backend's `removeTrack` handler (playlists.go:374) does `DELETE FROM playlist_items WHERE playlist_id=? AND position=?` — and after deletion, it recompacts positions (line 391-395). If two tracks have the same `position` value (which shouldn't happen but could after a race condition or manual DB edit), only one gets deleted. This is a minor edge case.

**Actual bug:** The `position` field on `Track` (api.ts:272) is `position?: number` — it's optional. If the backend doesn't return it (e.g., if the SELECT doesn't include `i.position`), then `t.position` is `undefined` and the fallback `i` is used. But the backend DOES include `i.position` in the SELECT (playlists.go:161). So this works, but it's fragile — if someone changes the backend query, the frontend silently falls back to index-based removal which could delete the wrong track after reordering.

### B3. [HIGH] PlaylistPage: No loading state for rename/delete operations
**File:** `PlaylistPage.tsx:70-79, 81-90`
The `saveName()` and `deletePlaylist()` functions don't set any loading state. If the API call is slow, the user can click the button multiple times, sending duplicate requests. The edit input and buttons remain enabled during the API call. Compare with PlaylistsPage which has `creating` state (line 18, 53, 63, 93).

### B4. [MEDIUM] PlaylistPage: `deletePlaylist` navigates away before confirming success
**File:** `PlaylistPage.tsx:81-90`
`deletePlaylist()` calls `navigate("/playlists")` immediately after `await api.deletePlaylist()` succeeds. But if the navigation happens before the playlists list refreshes, the user might briefly see stale data. More importantly, if the delete fails, the error is shown via toast but the user remains on the page — which is correct. However, there's no `try/catch` around the navigation itself (though navigation shouldn't throw).

### B5. [MEDIUM] PlaylistsPage: Delete confirmation is bare `confirm()` — no undo
**File:** `PlaylistsPage.tsx:40`
Uses `confirm()` which is blocking, ugly, and provides no undo. If a user accidentally confirms, the playlist is permanently gone. No toast is shown after successful deletion either — the list just silently refreshes.

### B6. [MEDIUM] PlaylistPage: `remove()` doesn't optimistically update UI
**File:** `PlaylistPage.tsx:60-68`
After removing a track, `load()` is called which re-fetches the entire playlist. This causes a full loading flash. The track should be removed from local state immediately (optimistic update), with rollback on error.

### B7. [LOW] PlaylistPage: `load()` is called on every `id` change but `id` is always a string
**File:** `PlaylistPage.tsx:55-58`
The `useEffect` depends on `id` (from `useParams`). Since `id` is a string and `Number(id)` is used in the API call, if `id` is `"abc"`, `Number(id)` is `NaN`, and the API call goes to `/api/playlists/NaN` which returns 404. The error message would be `"404 {\"error\":\"not found\"}"` — confusing. There's no validation that `id` is a valid number.

### B8. [LOW] PlaylistPage: Inline edit has no keyboard handler for Enter/Escape
**File:** `PlaylistPage.tsx:124-148`
The edit input (line 126-131) has no `onKeyDown` handler. Users must click the check/X buttons to confirm/cancel. Standard UX is Enter to confirm, Escape to cancel.

### B9. [LOW] PlaylistsPage: No error state for create — just sets error string
**File:** `PlaylistsPage.tsx:50-65`
If `create()` fails, the error is displayed (line 107-111), but the form input retains the failed name and the user has to manually clear it. The `creating` state is cleared in `finally`, but the input isn't reset on error.

---

## 2. MISSING FEATURES

### F1. [HIGH] No playlist sorting/filtering on PlaylistsPage
**File:** `PlaylistsPage.tsx`
The playlists grid has no sort options (by name, by date, by track count) and no search/filter. As the number of playlists grows, finding a specific one becomes impossible. The backend `list` query orders by `created_at DESC` with no way to change it.

### F2. [HIGH] No drag-and-drop reordering in PlaylistPage
**File:** `PlaylistPage.tsx`
The help text at `help-content.ts:299` literally says "Reorder — Tracks play in the order shown (reordering coming soon)." This is a core playlist feature that's missing. Users expect to be able to reorder tracks.

### F3. [HIGH] No "Add tracks to playlist" from within PlaylistPage
**File:** `PlaylistPage.tsx`
The PlaylistPage shows tracks in the playlist but provides no way to add more tracks. Users must go to MusicPage or use the TrackList "..." menu. There's no "Add tracks" button or search-within-playlist feature.

### F4. [MEDIUM] No playlist cover art
**File:** `PlaylistsPage.tsx:132-133`
Each playlist card shows a generic Music icon. Playlists should have cover art — either auto-generated from the first 4 track covers (like Spotify) or a user-uploaded image.

### F5. [MEDIUM] No playlist description
**File:** `PlaylistsPage.tsx`, `PlaylistPage.tsx`
The `Playlist` type (api.ts:399-405) has no `description` field. The backend `playlists` table may not have a description column. Playlists should support descriptions.

### F6. [MEDIUM] No bulk actions on PlaylistsPage
**File:** `PlaylistsPage.tsx`
No way to select multiple playlists for batch delete, export, or merge.

### F7. [MEDIUM] No "Play All" shuffle mode indication
**File:** `PlaylistPage.tsx:174-181`
The "Play All" button calls `player.play(playlist.tracks, 0)` which starts from the first track. If shuffle is enabled in the player, it will shuffle — but there's no visual indication of this on the button.

### F8. [LOW] No playlist sharing/export
**File:** `PlaylistsPage.tsx`
No way to export a playlist as M3U, JSON, or share a link.

### F9. [LOW] No empty state CTA for PlaylistPage
**File:** `PlaylistPage.tsx:200-207`
The empty state says "Browse your library and add tracks to this playlist" but provides no link/button to go to the library.

### F10. [LOW] No track count / duration in PlaylistsPage card header
**File:** `PlaylistsPage.tsx:150-161`
Track count and duration are shown but only if `track_count > 0` and `total_duration > 0`. A playlist with 0 tracks shows no count at all. Should always show "0 tracks" for consistency.

---

## 3. POOR IMPLEMENTATIONS

### P1. [HIGH] PlaylistPage: Entire playlist re-fetched on every track remove
**File:** `PlaylistPage.tsx:60-68`
`remove()` calls `load()` which re-fetches the entire playlist from the server. This is wasteful and causes UI flicker. Should update local state optimistically.

### P2. [MEDIUM] PlaylistsPage: `load()` called after create AND after delete — full list refresh
**File:** `PlaylistsPage.tsx:43, 58`
After creating or deleting a playlist, the entire list is re-fetched. For create, the new playlist could just be prepended to local state. For delete, the playlist could be filtered out locally.

### P3. [MEDIUM] PlaylistPage: `usePlayer()` called inside child components
**File:** `PlaylistPage.tsx:278, 326`
Both `DesktopPlaylistTrackRow` and `MobilePlaylistTrackCard` call `usePlayer()`. This means every row/card in the playlist subscribes to player state changes. With a 100-track playlist, that's 100 components re-rendering on every player state change. The player should be passed down via props or context should be consumed at the top level.

### P4. [MEDIUM] PlaylistPage: `useIsMobile()` called inside `PlaylistTrackList` component
**File:** `PlaylistPage.tsx:222`
The mobile/desktop switch is inside the `PlaylistTrackList` component, not at the top level. This means the decision is made late and the component tree is split unnecessarily.

### P5. [MEDIUM] PlaylistPage: No memoization of track list
**File:** `PlaylistPage.tsx:209`
`<PlaylistTrackList tracks={playlist.tracks} onRemove={(pos) => remove(pos)} />` — the `onRemove` callback is recreated on every render, causing `PlaylistTrackList` and all its children to re-render even if tracks haven't changed.

### P6. [LOW] PlaylistsPage: Help button is outside the form, separate from the create button
**File:** `PlaylistsPage.tsx:100-105`
The "How do playlists work?" help button is placed after the form, disconnected from the create action. It should be near the "Create" button or the page header help button should cover creation.

### P7. [LOW] PlaylistPage: Two help buttons — one in header, one below
**File:** `PlaylistPage.tsx:193-198`
There's a help button in the header area AND the page header has a help button via the nav. The inline help button at line 193 is redundant with the nav help button.

---

## 4. VISUAL ISSUES

### V1. [MEDIUM] PlaylistPage: Delete button is always visible, not hover-reveal
**File:** `PlaylistPage.tsx:183-189`
The delete button (trash icon) is always visible in the header. In PlaylistsPage, the delete button is hover-revealed (line 141: `opacity-100 md:opacity-0 md:group-hover:opacity-100`). This is inconsistent — the detail page should also hide delete behind a hover or menu.

### V2. [MEDIUM] PlaylistPage: No visual distinction between playing track and others
**File:** `PlaylistPage.tsx:267-313`
When a track from the playlist is playing, there's no highlight or indicator in the track list. Users can't see which track is currently playing without looking at the player bar.

### V3. [LOW] PlaylistsPage: Playlist cards have no hover animation
**File:** `PlaylistsPage.tsx:129`
The card has `hover:border-accent/50` but no scale or shadow transition. Other pages (like Music) have more polished hover effects.

### V4. [LOW] PlaylistPage: Mobile track cards don't show track number on play
**File:** `PlaylistPage.tsx:315-354`
On mobile, the track card shows the index number but doesn't have a hover-to-reveal play button like the desktop row does. The entire card is clickable, but there's no visual play indicator.

### V5. [LOW] PlaylistPage: Table has no fixed column widths
**File:** `PlaylistPage.tsx:239-264`
The table uses `truncate` for title, artist, album but has no `table-fixed` or `min-w-*` classes. On wide screens, the title column can become very long while album is squished.

---

## 5. ACCESSIBILITY

### A1. [HIGH] PlaylistPage: Edit input has no aria-label
**File:** `PlaylistPage.tsx:126-131`
The inline edit input has `autoFocus` but no `aria-label`. Screen readers won't know what this input is for.

### A2. [HIGH] PlaylistPage: Remove button has no aria-label
**File:** `PlaylistPage.tsx:300-309`
The remove button (X icon) has `title="Remove from playlist"` but no `aria-label`. The title attribute is not reliably read by screen readers.

### A3. [MEDIUM] PlaylistsPage: Delete button has `title` but no `aria-label`
**File:** `PlaylistsPage.tsx:142`
`title="Delete playlist"` but no `aria-label`.

### A4. [MEDIUM] PlaylistPage: Play All button doesn't indicate playlist context
**File:** `PlaylistPage.tsx:175-181`
The "Play All" button doesn't have an `aria-label` that includes the playlist name. Should be `aria-label="Play all tracks in ${playlist.name}"`.

### A5. [MEDIUM] PlaylistPage: Track rows are not keyboard-navigable
**File:** `PlaylistPage.tsx:267-313`
The track rows have `onDoubleClick` but no `onKeyDown` or `tabIndex`. Keyboard users can't navigate to or interact with individual tracks.

### A6. [LOW] PlaylistsPage: Empty state icon has no alt text
**File:** `PlaylistsPage.tsx:117`
`<ListMusic size={40} className="mx-auto text-muted" />` — decorative icon with no `aria-hidden="true"`.

---

## 6. PERFORMANCE

### R1. [HIGH] PlaylistPage: `usePlayer()` in every row causes O(n) re-renders
**File:** `PlaylistPage.tsx:278, 326`
Each `DesktopPlaylistTrackRow` and `MobilePlaylistTrackCard` calls `usePlayer()`. When player state changes (every second during playback), ALL rows re-render. For a 50-track playlist, that's 50 unnecessary re-renders per second.

### R2. [MEDIUM] PlaylistPage: `onRemove` callback recreated every render
**File:** `PlaylistPage.tsx:209`
`onRemove={(pos) => remove(pos)}` is a new function on every render, preventing `PlaylistTrackList` from memoization.

### R3. [MEDIUM] PlaylistsPage: No virtualization for large playlists
**File:** `PlaylistsPage.tsx:124-164`
The grid renders all playlist cards at once. For users with 100+ playlists, this could cause jank. Consider virtualization or pagination.

### R4. [LOW] PlaylistPage: `loadPlaylists()` in TrackList fetches all playlists on every row menu open
**File:** `TrackList.tsx:83-90`
Every time a user opens the "..." menu on a track, `loadPlaylists()` fetches ALL playlists. This is in the shared `TrackList` component used across Music, Search, and Playlist pages. The playlists should be cached or fetched once at app level.

---

## 7. CROSS-CUTTING CONCERNS

### C1. [HIGH] Backend: `list()` query doesn't filter by user_id properly for multi-user
**File:** `playlists.go:83`
The query uses `WHERE p.user_id IS NULL OR p.user_id = ?` — this means playlists with `user_id = 0` (unauthenticated) are visible to all users. In a multi-user setup, User A could see User B's playlists if they were created without a user_id. The `getUserID()` function returns 0 when unauthenticated (playlists.go:63), so all unauthenticated requests see all unauthenticated playlists.

### C2. [MEDIUM] Backend: `removeTrack` recompaction is not transactional
**File:** `playlists.go:391-395`
The position recompaction (`UPDATE playlist_items SET position = position - 1`) happens AFTER the `DELETE` but is not wrapped in the same transaction. If the recompaction fails, positions have a gap. The DELETE already committed (no transaction wrapper around both).

### C3. [MEDIUM] Frontend: No toast feedback for track removal from playlist
**File:** `PlaylistPage.tsx:60-68`
When a track is removed, there's no toast notification. The track just disappears from the list. Users get no confirmation or undo option.

### C4. [LOW] Backend: `create` returns 409 for duplicate names but frontend doesn't handle it specially
**File:** `playlists.go:132-134`, `PlaylistsPage.tsx:59-61`
The backend returns 409 for duplicate playlist names, but the frontend catch block treats it the same as any other error — shows the raw error message. Should show a friendly "A playlist with this name already exists" message.

---

## PRIORITIZED FIX ROADMAP

### Phase 1: Critical Bugs (fix first)
1. **B1** — Handle 404 gracefully in PlaylistPage, show friendly "Playlist not found" vs network error
2. **B3** — Add loading state to rename/delete operations in PlaylistPage
3. **B7** — Validate `id` param is a valid number before API call
4. **C1** — Fix user_id filtering in playlist list query for multi-user safety

### Phase 2: High-Impact Features
5. **F2** — Implement drag-and-drop track reordering in PlaylistPage
6. **F3** — Add "Add tracks to playlist" functionality within PlaylistPage
7. **F1** — Add playlist sorting/filtering on PlaylistsPage
8. **R1** — Fix O(n) re-render issue by lifting player state consumption

### Phase 3: Medium Improvements
9. **B5** — Replace bare `confirm()` with proper modal + toast feedback
10. **B6** — Optimistic UI updates for track removal
11. **P1** — Stop re-fetching entire playlist on track remove
12. **P3** — Pass player as prop instead of usePlayer() in every row
13. **C3** — Add toast feedback for track removal
14. **C4** — Handle 409 duplicate name error with friendly message
15. **C2** — Make position recompaction transactional

### Phase 4: Low Priority / Polish
15. **B8** — Add Enter/Escape keyboard handlers for inline edit
16. **B9** — Reset form on create error
17. **F4** — Playlist cover art
18. **F5** — Playlist descriptions
19. **F9** — Empty state CTA with link to library
20. **V1** — Consistent delete button visibility (hover-reveal)
21. **V2** — Currently playing track highlight
22. **A1-A6** — Accessibility fixes (aria-labels, keyboard nav)
23. **R4** — Cache playlists at app level instead of fetching per-row
