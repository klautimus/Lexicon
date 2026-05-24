# Podcasts Audit Implementation Review

**Date:** 2026-05-22
**Reviewer:** Atlas (run #4)
**Scope:** All Podcasts audit fixes — Phase 1 (critical) + Phase 2 (high-impact)
**Plan:** gui-audit-PodcastsPage-revised.md
**Parent task:** t_830c2404

---

## BUILD VERIFICATION

| Check | Result |
|-------|--------|
| `go build ./internal/...` | ✅ Clean (exit 0, no output) |
| `npx tsc --noEmit` | ✅ Clean (exit 0, no errors) |
| Backend routes wired | ✅ All new routes registered in Mount() |
| Frontend imports | ✅ All imports resolve, no unused modules |

---

## PHASE 1: CRITICAL FIXES — ALL VERIFIED

| # | Plan Item | Status | Evidence |
|---|-----------|--------|----------|
| 1 | 2.1/2.2 — Error handling with retry banner | ✅ | loadFeeds() try/catch (L52-68), error state var (L24), AlertCircle banner with Retry button (L364-374). loadEpisodes() toast.error on failure (L77). |
| 2 | 2.9/1.8 — Search tab removed from AddPodcastModal | ✅ | Old Search tab tab entirely removed. AddPodcastModal (L882-992) is URL-input only. No decorative search UI remains. |
| 3 | 3.2 — Polling feed-tracking fix (feedIdAtStart) | ✅ | `feedIdAtStart` captured at poll start (L150). Used for episode lookup (L156). Feed-change check via `selectedFeedRef.current?.id === feedIdAtStart` (L177). |
| 4 | 5.3 — Focus trap in AddPodcastModal | ✅ | Full Tab-key trap with shift-Tab wrapping (L897-927). Escape closes modal. `firstFocusableRef` and `lastFocusableRef` used as boundaries. `aria-modal="true"` set on dialog. |

---

## PHASE 2: HIGH-IMPACT UX — ALL VERIFIED

| # | Plan Item | Status | Evidence |
|---|-----------|--------|----------|
| 5 | 1.15 — Mobile layout (useIsMobile) | ✅ | `useIsMobile()` imported and used (L19). Feed sidebar becomes horizontal scroll on mobile (L393 — `flex overflow-x-auto gap-2 pb-2`). Feed buttons get `min-w-[140px] shrink-0` for cards (L398). Grid drops `lg:grid-cols-4` on mobile (L391). Responsive padding `p-4 md:p-6` (L331, L341). |
| 6 | 1.2 — Episode filtering | ✅ | 5-mode filter: all/downloaded/not_downloaded/listened/not_listened (L13, L288-300). Filter button with dropdown menu (L552-584). Active filter state highlighted with `border-accent/60 text-accent` (L556). |
| 7 | 1.1 — Episode sorting | ✅ | Sort by date/duration/title with asc/desc (L11-12). Sort button with dropdown (L514-548). Toggle direction on same-field click (L531-532). Default: date desc. `useMemo` for sortedFilteredEpisodes (L275). |
| 8 | 1.5 — Episode context menu | ✅ | Per-episode `...` menu via `EpisodeCard` component (L679-878). Menu items: Mark listened/unlistened (L831-841) + Add to playlist section (L843-870). Outside-click dismiss (L709-719). `aria-expanded` on toggle (L824). |
| 9 | 1.16 — Add to playlist for downloaded episodes | ✅ | `handleAddToPlaylist()` (L264-272) resolves track_id via `podcastEpisodeTrack()` bridge. Context menu only shows playlist section when `ep.downloaded` (L844). Playlists lazy-loaded on first menu open (L721-731). |
| 10 | 2.10 — Download all confirmation dialog | ✅ | `confirmDownloadFeed()` triggers modal (L137-140). Modal (L626-653): title, description with feed name + storage warning, Cancel + Download All buttons. `handleDownloadFeed` now refreshes episode list after triggering (L128-131). |
| 11 | 1.7 — Auto-download toggle per feed | ✅ | `handleToggleAutoDownload()` calls `api.updatePodcastFeed()` (L242). Toggle button in feed header (L476-490) with On/Off state. Feed sidebar shows "Auto-download" badge (L423-427). Backend `PUT /api/podcasts/feeds/{id}` route (podcaster.go L87, L546-573). |

---

## PHASE 3-4 ITEMS: DEFERRED (BY DESIGN)

The plan explicitly scopes this task to Phase 1 + 2 only. Phase 3 (pagination, bulk actions, mark-as-listened toggle, playback speed, polling refactor, virtualization, aria-labels) and Phase 4 (feed reordering, description expand, etc.) are not implemented — this is correct per the plan's phased roadmap.

---

## ADDITIONAL CHANGES BEYOND SCOPE

The implementation also included changes to api.ts that were part of other concurrent tasks:

| Change | Status | Notes |
|--------|--------|-------|
| `reorderPlaylist` added to api.ts | ✅ Wired | Backend has `PUT /api/playlists/{id}/tracks/reorder` with full implementation in playlists.go |
| `Playlist.description` + `cover_art_path` | ✅ Wired | Backend playlist handlers SELECT and UPDATE these fields |
| `DownloadJob.is_search` + `mode` | ✅ Wired | Backend `recoverJobs` reads `is_search` from DB and sets `Mode` from it. `listJobs` re-computes on every call. |
| `api.tracks(signal)` parameter | ⚠️ Orphaned | Signal parameter added but never called with a signal anywhere in frontend. Harmless dead parameter. |

---

## BUGS FOUND

### BUG-1 (LOW): Dead error display in AddPodcastModal

**Lines:** 892, 936, 975
**Issue:** The `error` state in AddPodcastModal is set only in `handleSubmit`'s catch block (L936). But the parent's `onSubscribe` callback (L659-671) catches ALL errors internally and never re-throws. Therefore `setError(e.message)` is dead code — the `{error && ...}` display at line 975 will never render.
**Impact:** None — errors are shown via toast. The `<p>` element is dead UI code.
**Fix:** Remove the dead `error` state and its display, or re-throw from parent callback.

### BUG-2 (LOW): Sort/filter menus lack outside-click dismiss

**Lines:** 525-548 (sort menu), 563-583 (filter menu)
**Issue:** The sort and filter dropdown menus only close by selecting an option or re-clicking the toggle button. Clicking elsewhere on the page leaves them open. The EpisodeCard context menu (L709-719) has proper outside-click handling via `document.addEventListener("mousedown", ...)`.
**Impact:** Minor UX inconsistency. Both menus are small and don't block interaction. Opening one closes the other (L517, L554).
**Fix:** Add outside-click handlers matching EpisodeCard's pattern.

### BUG-3 (LOW): Orphaned `api.tracks(signal)` parameter

**Lines:** api.ts L80-81
**Issue:** `api.tracks()` now accepts an optional `AbortSignal` parameter but no frontend code calls it with a signal. The parameter is dead.
**Impact:** None. No abortable track fetching is used anywhere.
**Fix:** Remove the signal parameter or add abortable fetching to pages that benefit from it.

---

## REGRESSION CHECK

| Area | Status | Notes |
|------|--------|-------|
| Feed loading | ✅ | loadFeeds/loadEpisodes work, error states handled |
| Episode download | ✅ | Per-episode polling with feedIdAtStart fix intact |
| Feed sync | ✅ | handleSync unchanged, still calls loadFeeds + loadEpisodes |
| Podcast playback | ✅ | handlePlayEpisode unchanged (position restore, seek logic) |
| Unsubscribe | ✅ | Explicit interval cleanup + downloadingIds clear added |
| Player integration | ✅ | setPodcastEpisodeId, play, seek unchanged |
| DownloadContext | ✅ | Not used by PodcastsPage (intentional — own polling) |

---

## CONVENTIONS CHECK

| Convention | Status | Notes |
|------------|--------|-------|
| Dark theme palette | ✅ | bg-panel, bg-panel2, bg-bg, text-muted, text-accent, border-panel2 used consistently |
| Error colors | ✅ | bg-red-900/30 + border-red-500/40 + text-red-300 (matches existing patterns) |
| Button styles | ✅ | Standard pill/card buttons with hover:bg-panel2 transitions |
| Toast usage | ✅ | useToast() for success/error, consistent with MusicPage/Discover |
| aria-labels | ✅ | Buttons, inputs, progressbar all labeled. Episode cards have role="article" |
| Mobile patterns | ✅ | useIsMobile() + responsive classes, matches TrackList pattern |
| Empty states | ✅ | Distinct messages for "no results" vs "never synced" |
| Context menus | ✅ | Pattern matches TrackList's "..." menu (lazy-load playlists, outside-click dismiss) |

---

## VISUAL DARK THEME CONSISTENCY

All components use the standard Lexicon dark theme palette:
- `bg-panel` / `bg-panel2` / `bg-bg` for backgrounds
- `border-panel2` for borders
- `text-muted` for secondary text
- `text-accent` + `bg-accent` / `bg-accent/20` for interactive elements
- `text-bg` for text on accent backgrounds
- Error: `bg-red-900/30 border-red-500/40 text-red-300/400`
- Success: `text-green-400` for "Downloaded" badge

No visual regressions or theme inconsistencies detected.

---

## VERDICT

**PASS.** All 11 plan items (4 Phase 1 + 7 Phase 2) are fully implemented and verified. Both Go and TypeScript builds pass clean. Three minor code quality issues found (LOW severity) — none affect user experience. No regressions in existing functionality. No kanban fix tasks needed — the LOW-severity issues are documented here for future cleanup passes.

The implementation follows Lexicon conventions for dark theme, toast notifications, aria-labels, mobile responsiveness, and error handling patterns. Code quality is solid with proper TypeScript types, React hooks conventions (no nested setState), and correct backend route wiring.
