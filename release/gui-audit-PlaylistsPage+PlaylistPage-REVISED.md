# GUI Audit: PlaylistsPage + PlaylistPage — REVISED

**Date:** 2026-05-22
**Auditor:** Atlas (researcher)
**Reviewer:** Atlas (analyst)
**Scope:** PlaylistsPage.tsx, PlaylistPage.tsx, and all supporting files
**Severity scale:** Critical > High > Medium > Low > Cosmetic

---

## Review Summary

Original audit: 45 findings across 7 categories.
**VALID: 43** | **INVALID: 1** | **MISSED: 2** (added below)

### Invalid Findings

- **F10** — INVALID: Track count IS always shown on PlaylistPage cards (`{p.track_count} tracks` at line 153 is unconditional). Only `total_duration` is conditional.

### Missed Findings (added)

- **B10** [MEDIUM] — PlaylistPage `deletePlaylist()` also uses bare `confirm()` (line 83), same issue as B5.
- **R5** [MEDIUM] — `TrackList.tsx` also calls `usePlayer()` inside every `TrackRow` (line 61) and `MobileTrackCard` (line 336), causing the same O(n) re-render problem on MusicPage and SearchPage.

---

## 1. BUGS (Broken behavior)

### B1. [CRITICAL] PlaylistPage: `error` state shown but never set on 404 from backend
**File:** `PlaylistPage.tsx:47-49`
**Status:** VALID
The `catch` block sets `setError(e instanceof Error ? e.message : String(e))`, but the backend `get()` handler (playlists.go:150-158) returns HTTP 404 with body `{"error":"not found"}`. The `api` client (`j()` in api.ts:46) throws `Error('404 {"error":"not found"}')` for non-200 responses. So the error message shown to the user is the raw HTTP status + JSON body — not a friendly message. The "Playlist not found." text at line 105 is always shown when `playlist` is null, regardless of whether it was a 404 or a network error. There's no way for the user to distinguish "playlist was deleted" from "server is down."

### B2. [HIGH] PlaylistPage: `onRemove` passes `t.position ?? i` but position is 1-based from backend
**File:** `PlaylistPage.tsx:232, 258`
**Status:** VALID (but overstated)
The `PlaylistTrackList` component calls `onRemove(t.position ?? i)`. The backend `removeTrack` handler uses the `position` column from `playlist_items`. The `position` field is 0-based (set by `addTrack` which starts at 0 and increments). The `i` index from `.map()` is also 0-based. So `t.position ?? i` is correct. The backend DOES include `i.position` in the SELECT (playlists.go:161). The concern is valid: if someone changes the backend query, the frontend silently falls back to index-based removal which could delete the wrong track after reordering. This is a fragility issue, not an active bug.

### B3. [HIGH] PlaylistPage: No loading state for rename/delete operations
**File:** `PlaylistPage.tsx:70-79, 81-90`
**Status:** VALID
The `saveName()` and `deletePlaylist()` functions don't set any loading state. If the API call is slow, the user can click the button multiple times, sending duplicate requests. The edit input and buttons remain enabled during the API call. Compare with PlaylistsPage which has `creating` state (line 18, 53, 63, 93).

### B4. [MEDIUM] PlaylistPage: `deletePlaylist` navigates away before confirming success
**File:** `PlaylistPage.tsx:81-90`
**Status:** VALID (minor)
`deletePlaylist()` calls `navigate("/playlists")` immediately after `await api.deletePlaylist()` succeeds. The navigation happens after the await, so it's correctly sequenced. The minor concern is that the playlists page might briefly show stale data before its next load. This is a cosmetic issue, not a functional bug.

### B5. [MEDIUM] PlaylistsPage: Delete confirmation is bare `confirm()` — no undo
**File:** `PlaylistsPage.tsx:40`
**Status:** VALID
Uses `confirm()` which is blocking, provides no undo, and no toast is shown after successful deletion — the list just silently refreshes.

### B6. [MEDIUM] PlaylistPage: `remove()` doesn't optimistically update UI
**File:** `PlaylistPage.tsx:60-68`
**Status:** VALID
After removing a track, `load()` is called which re-fetches the entire playlist. This causes a full loading flash. The track should be removed from local state immediately (optimistic update), with rollback on error.

### B7. [LOW] PlaylistPage: `load()` is called on every `id` change but `id` is always a string
**File:** `PlaylistPage.tsx:55-58`
**Status:** VALID
The `useEffect` depends on `id` (from `useParams`). Since `id` is a string and `Number(id)` is used in the API call, if `id` is `"abc"`, `Number(id)` is `NaN`, and the API call goes to `/api/playlists/NaN` which returns 404. The error message would be `"404 {\"error\":\"not found\"}"` — confusing. There's no validation that `id` is a valid number.

### B8. [LOW] PlaylistPage: Inline edit has no keyboard handler for Enter/Escape
**File:** `PlaylistPage.tsx:124-148`
**Status:** VALID
The edit input (line 126-131) has no `onKeyDown` handler. Users must click the check/X buttons to confirm/cancel. Standard UX is Enter to confirm, Escape to cancel.

### B9. [LOW] PlaylistsPage: No error state reset for create — just sets error string
**File:** `PlaylistsPage.tsx:50-65`
**Status:** VALID
If `create()` fails, the error is displayed (line 107-111), but the form input retains the failed name and the user has to manually clear it. The `creating` state is cleared in `finally`, but the input isn't reset on error.

### B10. [MEDIUM] PlaylistPage: `deletePlaylist()` also uses bare `confirm()` — no undo
**File:** `PlaylistPage.tsx:83`
**Status:** VALID (MISSED in original audit)
PlaylistPage's `deletePlaylist()` uses `confirm("Delete this playlist? This cannot be undone.")` — same issue as B5. No undo, no toast feedback on success (just navigates away).

---

## 2. MISSING FEATURES

### F1. [HIGH] No playlist sorting/filtering on PlaylistsPage
**File:** `PlaylistsPage.tsx`
**Status:** VALID
The playlists grid has no sort options (by name, by date, by track count) and no search/filter. As the number of playlists grows, finding a specific one becomes impossible. The backend `list` query orders by `created_at DESC` with no way to change it.

### F2. [HIGH] No drag-and-drop reordering in PlaylistPage
**File:** `PlaylistPage.tsx`
**Status:** VALID
The help text at `help-content.ts:299` literally says "Reorder — Tracks play in the order shown (reordering coming soon)." This is a core playlist feature that's missing.

### F3. [HIGH] No "Add tracks to playlist" from within PlaylistPage
**File:** `PlaylistPage.tsx`
**Status:** VALID
The PlaylistPage shows tracks in the playlist but provides no way to add more tracks. Users must go to MusicPage or use the TrackList "..." menu.

### F4. [MEDIUM] No playlist cover art
**File:** `PlaylistsPage.tsx:132-133`
**Status:** VALID
Each playlist card shows a generic Music icon. Playlists should have cover art.

### F5. [MEDIUM] No playlist description
**File:** `PlaylistsPage.tsx`, `PlaylistPage.tsx`
**Status:** VALID
The `Playlist` type (api.ts:399-405) has no `description` field.

### F6. [MEDIUM] No bulk actions on PlaylistsPage
**File:** `PlaylistsPage.tsx`
**Status:** VALID
No way to select multiple playlists for batch delete, export, or merge.

### F7. [MEDIUM] No "Play All" shuffle mode indication
**File:** `PlaylistPage.tsx:174-181`
**Status:** VALID
The "Play All" button calls `player.play(playlist.tracks, 0)` which starts from the first track. If shuffle is enabled in the player, it will shuffle — but there's no visual indication of this on the button.

### F8. [LOW] No playlist sharing/export
**File:** `PlaylistsPage.tsx`
**Status:** VALID
No way to export a playlist as M3U, JSON, or share a link.

### F9. [LOW] No empty state CTA for PlaylistPage
**File:** `PlaylistPage.tsx:200-207`
**Status:** VALID
The empty state says "Browse your library and add tracks to this playlist" but provides no link/button to go to the library.

### F10. [LOW] No track count / duration in PlaylistsPage card header
**File:** `PlaylistsPage.tsx:150-161`
**Status:** **INVALID**
Track count IS always shown unconditionally at line 153: `{p.track_count} tracks`. Only `total_duration` is conditional (line 155: `{p.total_duration > 0 && ...}`). A playlist with 0 tracks correctly shows "0 tracks".

---

## 3. POOR IMPLEMENTATIONS

### P1. [HIGH] PlaylistPage: Entire playlist re-fetched on every track remove
**File:** `PlaylistPage.tsx:60-68`
**Status:** VALID (same as B6)
`remove()` calls `load()` which re-fetches the entire playlist from the server. Should update local state optimistically.

### P2. [MEDIUM] PlaylistsPage: `load()` called after create AND after delete — full list refresh
**File:** `PlaylistsPage.tsx:43, 58`
**Status:** VALID
After creating or deleting a playlist, the entire list is re-fetched. For create, the new playlist could just be prepended to local state. For delete, the playlist could be filtered out locally.

### P3. [MEDIUM] PlaylistPage: `usePlayer()` called inside child components
**File:** `PlaylistPage.tsx:278, 326`
**Status:** VALID
Both `DesktopPlaylistTrackRow` and `MobilePlaylistTrackCard` call `usePlayer()`. With a 100-track playlist, that's 100 components re-rendering on every player state change.

### P4. [MEDIUM] PlaylistPage: `useIsMobile()` called inside `PlaylistTrackList` component
**File:** `PlaylistPage.tsx:222`
**Status:** VALID
The mobile/desktop switch is inside the `PlaylistTrackList` component, not at the top level.

### P5. [MEDIUM] PlaylistPage: No memoization of track list
**File:** `PlaylistPage.tsx:209`
**Status:** VALID
`<PlaylistTrackList tracks={playlist.tracks} onRemove={(pos) => remove(pos)} />` — the `onRemove` callback is recreated on every render.

### P6. [LOW] PlaylistsPage: Help button is outside the form, separate from the create button
**File:** `PlaylistsPage.tsx:100-105`
**Status:** VALID
The "How do playlists work?" help button is placed after the form, disconnected from the create action.

### P7. [LOW] PlaylistPage: Two help buttons — one in header, one below
**File:** `PlaylistPage.tsx:193-198`
**Status:** VALID
There's a help button in the header area AND the page header has a help button via the nav. The inline help button at line 193 is redundant.

---

## 4. VISUAL ISSUES

### V1. [MEDIUM] PlaylistPage: Delete button is always visible, not hover-reveal
**File:** `PlaylistPage.tsx:183-189`
**Status:** VALID
The delete button (trash icon) is always visible in the header. In PlaylistsPage, the delete button is hover-revealed (line 141: `opacity-100 md:opacity-0 md:group-hover:opacity-100`). Inconsistent.

### V2. [MEDIUM] PlaylistPage: No visual distinction between playing track and others
**File:** `PlaylistPage.tsx:267-313`
**Status:** VALID
When a track from the playlist is playing, there's no highlight or indicator in the track list.

### V3. [LOW] PlaylistsPage: Playlist cards have no hover animation
**File:** `PlaylistsPage.tsx:129`
**Status:** VALID
The card has `hover:border-accent/50` but no scale or shadow transition.

### V4. [LOW] PlaylistPage: Mobile track cards don't show track number on play
**File:** `PlaylistPage.tsx:315-354`
**Status:** VALID
On mobile, the track card shows the index number but doesn't have a hover-to-revealed play button like the desktop row does.

### V5. [LOW] PlaylistPage: Table has no fixed column widths
**File:** `PlaylistPage.tsx:239-264`
**Status:** VALID
The table uses `truncate` for title, artist, album but has no `table-fixed` or `min-w-*` classes.

---

## 5. ACCESSIBILITY

### A1. [HIGH] PlaylistPage: Edit input has no aria-label
**File:** `PlaylistPage.tsx:126-131`
**Status:** VALID
The inline edit input has `autoFocus` but no `aria-label`.

### A2. [HIGH] PlaylistPage: Remove button has no aria-label
**File:** `PlaylistPage.tsx:300-309`
**Status:** VALID
The remove button (X icon) has `title="Remove from playlist"` but no `aria-label`.

### A3. [MEDIUM] PlaylistsPage: Delete button has `title` but no `aria-label`
**File:** `PlaylistsPage.tsx:142`
**Status:** VALID
`title="Delete playlist"` but no `aria-label`.

### A4. [MEDIUM] PlaylistPage: Play All button doesn't indicate playlist context
**File:** `PlaylistPage.tsx:175-181`
**Status:** VALID
The "Play All" button doesn't have an `aria-label` that includes the playlist name.

### A5. [MEDIUM] PlaylistPage: Track rows are not keyboard-navigable
**File:** `PlaylistPage.tsx:267-313`
**Status:** VALID
The track rows have `onDoubleClick` but no `onKeyDown` or `tabIndex`.

### A6. [LOW] PlaylistsPage: Empty state icon has no alt text
**File:** `PlaylistsPage.tsx:117`
**Status:** VALID
`<ListMusic size={40} className="mx-auto text-muted" />` — decorative icon with no `aria-hidden="true"`.

---

## 6. PERFORMANCE

### R1. [HIGH] PlaylistPage: `usePlayer()` in every row causes O(n) re-renders
**File:** `PlaylistPage.tsx:278, 326`
**Status:** VALID
Each `DesktopPlaylistTrackRow` and `MobilePlaylistTrackCard` calls `usePlayer()`. When player state changes (every second during playback), ALL rows re-render.

### R2. [MEDIUM] PlaylistPage: `onRemove` callback recreated every render
**File:** `PlaylistPage.tsx:209`
**Status:** VALID (same as P5)
`onRemove={(pos) => remove(pos)}` is a new function on every render.

### R3. [MEDIUM] PlaylistsPage: No virtualization for large playlists
**File:** `PlaylistsPage.tsx:124-164`
**Status:** VALID
The grid renders all playlist cards at once. For users with 100+ playlists, this could cause jank.

### R4. [LOW] PlaylistPage: `loadPlaylists()` in TrackList fetches all playlists on every row menu open
**File:** `TrackList.tsx:83-90`
**Status:** VALID
Every time a user opens the "..." menu on a track, `loadPlaylists()` fetches ALL playlists. The playlists should be cached or fetched once at app level.

### R5. [MEDIUM] TrackList: `usePlayer()` in every TrackRow causes O(n) re-renders on Music/Search pages
**File:** `TrackList.tsx:61, 336`
**Status:** VALID (MISSED in original audit)
`TrackRow` (line 61) and `MobileTrackCard` (line 336) in the shared `TrackList` component both call `usePlayer()`. This causes the same O(n) re-render problem on MusicPage and SearchPage, not just PlaylistPage.

---

## 7. CROSS-CUTTING CONCERNS

### C1. [HIGH] Backend: `list()` query doesn't filter by user_id properly for multi-user
**File:** `playlists.go:83`
**Status:** VALID
The query uses `WHERE p.user_id IS NULL OR p.user_id = ?` — playlists with `user_id IS NULL` (legacy/unauthenticated) are visible to all users. In a multi-user setup, this could leak playlists between users.

### C2. [MEDIUM] Backend: `removeTrack` recompaction is not transactional
**File:** `playlists.go:391-395`
**Status:** VALID
The position recompaction (`UPDATE playlist_items SET position = position - 1`) happens AFTER the `DELETE` as a separate `ExecContext` call. If the recompaction fails, positions have a gap.

### C3. [MEDIUM] Frontend: No toast feedback for track removal from playlist
**File:** `PlaylistPage.tsx:60-68`
**Status:** VALID
When a track is removed, there's no toast notification. The track just disappears from the list.

### C4. [LOW] Backend: `create` returns 409 for duplicate names but frontend doesn't handle it specially
**File:** `playlists.go:132-134`, `PlaylistsPage.tsx:59-61`
**Status:** VALID
The backend returns 409 for duplicate playlist names, but the frontend catch block treats it the same as any other error — shows the raw error message like `409 {"error":"playlist with this name already exists"}`.

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
8. **R1** — Fix O(n) re-render issue by lifting player state consumption in PlaylistPage
9. **R5** — Fix O(n) re-render issue in shared TrackList component

### Phase 3: Medium Improvements
10. **B5** — Replace bare `confirm()` with proper modal + toast feedback on PlaylistsPage
11. **B10** — Replace bare `confirm()` with proper modal on PlaylistPage
12. **B6/P1** — Optimistic UI updates for track removal (stop re-fetching entire playlist)
13. **P3** — Pass player as prop instead of usePlayer() in every row
14. **C3** — Add toast feedback for track removal
15. **C4** — Handle 409 duplicate name error with friendly message
16. **C2** — Make position recompaction transactional

### Phase 4: Low Priority / Polish
17. **B8** — Add Enter/Escape keyboard handlers for inline edit
18. **B9** — Reset form on create error
19. **F4** — Playlist cover art
20. **F5** — Playlist descriptions
21. **F9** — Empty state CTA with link to library
22. **V1** — Consistent delete button visibility (hover-reveal)
23. **V2** — Currently playing track highlight
24. **A1-A6** — Accessibility fixes (aria-labels, keyboard nav)
25. **R4** — Cache playlists at app level instead of fetching per-row
