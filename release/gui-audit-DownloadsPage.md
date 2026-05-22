# GUI Audit: DownloadsPage

**Date:** 2026-05-22
**Auditor:** Atlas (researcher)
**Scope:** DownloadsPage.tsx + all referenced context/providers/API layer
**Files reviewed:**
- `frontend/src/pages/DownloadsPage.tsx` (355 lines)
- `frontend/src/App.tsx` (283 lines)
- `frontend/src/contexts/DownloadContext.tsx` (374 lines)
- `frontend/src/contexts/PlayerContext.tsx` (688 lines)
- `frontend/src/contexts/UserContext.tsx` (72 lines)
- `frontend/src/contexts/ToastContext.tsx` (93 lines)
- `frontend/src/lib/api.ts` (460 lines)
- `frontend/src/components/PlayerBar.tsx` (154 lines)
- `frontend/src/components/MobilePlayerBar.tsx` (203 lines)
- `frontend/src/components/DevicePicker.tsx` (173 lines)
- `frontend/src/components/DownloadProgressBar.tsx` (120 lines)
- `frontend/src/components/MobileNavBar.tsx` (141 lines)
- `frontend/src/components/TrackList.tsx` (584 lines)
- `frontend/src/components/HelpModal.tsx` (79 lines)
- `frontend/src/index.css` (28 lines)
- `frontend/src/help-content.ts` (420 lines)
- `backend/internal/downloader/downloader.go` (1911 lines, partial)

---

## 1. CRITICAL BUGS

### 1.1 DownloadsPage polling continues after navigation away from page
**File:** `DownloadsPage.tsx:35-46`
**Severity:** Medium-High

The `setInterval(refresh, 1500)` on line 38 keeps running even when the user navigates away from the Downloads page. The `mountedRef` check on line 54 prevents state updates after unmount, but the interval itself is never cleared when navigating away — it only clears on full unmount (which doesn't happen during route changes since the component stays mounted in the router). This means:

1. API calls to `/api/download/status` and `/api/download/jobs` continue every 1.5s on every page
2. For each expanded job, an additional `api.downloadJob(id)` call fires every 1.5s
3. This wastes bandwidth and server resources

**Fix:** The polling should pause when the page is not visible (using `document.visibilityChange` or a focus-based approach), or the interval should be tied to route visibility.

### 1.2 Expanded job log polling has no error backoff
**File:** `DownloadsPage.tsx:58-68`
**Severity:** Medium

When a job is expanded, the `refresh()` function fetches full job details including logs. If the server returns an error (e.g., job was cleaned up), the `catch () {}` on line 66 silently swallows it, and the poll continues every 1.5s indefinitely. There's no backoff or stop condition.

### 1.3 `submit()` doesn't trim URL/query before validation
**File:** `DownloadsPage.tsx:74-94`
**Severity:** Low-Medium

On line 80, `if (!url.trim()) return;` checks for empty input but the actual API call on line 81 uses `url.trim()`. However, the `setSubmitting(true)` on line 77 happens before the trim check, so if the user enters only whitespace, `submitting` gets set to `true` but then the function returns early without setting it back to `false`. The submit button stays in a loading/spinner state until the user types something.

**Fix:** Move `setSubmitting(true)` after the empty check, or add `setSubmitting(false)` in the early return path.

### 1.4 No duplicate submission prevention
**File:** `DownloadsPage.tsx:74-94`
**Severity:** Medium

The `submitting` state disables the submit button, but there's no protection against double-clicks that occur before React re-renders the disabled state. If a user double-clicks quickly, two API calls can fire. The `setSubmitting(true)` on line 77 and the API call on line 81 are not atomic.

### 1.5 Job list has no stable sort — order depends on backend
**File:** `DownloadsPage.tsx:247-256`
**Severity:** Low-Medium

Jobs are rendered in whatever order the backend returns them. If the backend returns jobs in insertion order (newest first), this is fine. But if the order changes between polls (e.g., due to DB query ordering), jobs could jump around in the UI, confusing users. There's no client-side sorting by status/time.

### 1.6 `DownloadProgressBar` and `DownloadsPage` poll the same endpoint independently
**File:** `DownloadProgressBar.tsx:27` and `DownloadsPage.tsx:38`
**Severity:** Low

Both components poll `/api/download/progress` and `/api/download/jobs` independently every 1.5-2s. When the user is on the Downloads page, this results in 4 API calls every 2 seconds (2 from DownloadsPage, 2 from DownloadProgressBar). These should be consolidated into the DownloadContext.

### 1.7 `DownloadContext.trackDownload` creates intervals that are never cleaned up on unmount
**File:** `DownloadContext.tsx:92-174`
**Severity:** Medium

The `trackDownload` function creates polling intervals for each download job. These are stored in `pollRef.current`. While the cleanup effect on line 67-71 clears all intervals on unmount, the `DownloadProvider` never unmounts (it wraps the entire app). This means intervals accumulate over time. If a download fails or is cancelled, the interval is cleaned up (lines 117-118, 139-140, 150-151), but if the component loses track of a job ID (e.g., due to a race), the interval leaks.

### 1.8 `createAiPlaylist` polling interval leak on error
**File:** `DownloadContext.tsx:246-333`
**Severity:** Medium

In the `createAiPlaylist` function, when polling for download completion, if the `api.downloadJob(job.id)` call throws (line 248), the catch block on line 324-331 clears the interval and deletes the ref. However, if the error happens during the `api.addToPlaylist` or `api.search` calls (lines 255, 285-286), the interval is NOT cleared — it keeps polling a job that may already be complete.

---

## 2. MISSING FEATURES

### 2.1 No bulk/clear completed jobs
**File:** `DownloadsPage.tsx:232-259`
**Severity:** Medium

There's no way to clear completed/failed jobs from the list. Over time, the job list grows indefinitely. Users should be able to:
- Clear all completed jobs
- Clear all failed jobs
- Remove individual jobs from the list

### 2.2 No download progress percentage
**File:** `DownloadsPage.tsx:264-355` (JobRow component)
**Severity:** Medium

The `DownloadProgressBar` component (line 120) shows per-job progress bars with percentage, but the `JobRow` component in `DownloadsPage` shows no progress information at all — just a spinning icon for running jobs. Users can't see:
- Download percentage
- Download speed
- ETA
- File size

The backend `DownloadJob` type supports `progress` (0-100) and `progress_label` fields (api.ts:252-253), but `JobRow` doesn't display them.

### 2.3 No retry failed downloads
**File:** `DownloadsPage.tsx:264-355`
**Severity:** Medium

Failed jobs show an error message but there's no "Retry" button. Users have to re-enter the URL/query and submit again.

### 2.4 No download queue management
**File:** `DownloadsPage.tsx:232-259`
**Severity:** Low

Users can't:
- Reorder queued downloads
- Pause/resume the queue
- Set priority for specific downloads
- See estimated queue wait time

### 2.5 No keyboard shortcuts
**File:** `DownloadsPage.tsx` (entire file)
**Severity:** Low

No keyboard navigation support:
- Enter to submit (works via form, but no explicit handling)
- Escape to clear input
- Arrow keys to navigate job list
- Space to expand/collapse job logs

### 2.6 No drag-and-drop for Spotify URLs
**File:** `DownloadsPage.tsx:178-210`
**Severity:** Low

Users must paste URLs manually. Drag-and-drop from a browser or file manager would improve UX.

### 2.7 No download history persistence info
**File:** `DownloadsPage.tsx:232-259`
**Severity:** Low

The page shows "Recent jobs" but doesn't indicate:
- Total downloads ever
- Total data downloaded
- Success/failure rate
- When the list was last cleared

### 2.8 No indication of which download tool is being used before starting
**File:** `DownloadsPage.tsx:136-161`
**Severity:** Low

The config status section shows SpotiFLAC and spotDL status, but doesn't show:
- yt-dlp availability
- Current download concurrency setting
- Which tool will be used for the current mode

### 2.9 No toast notifications for download events on DownloadsPage
**File:** `DownloadsPage.tsx` (entire file)
**Severity:** Medium

The page doesn't use `useToast()` at all. When a download completes, fails, or is cancelled, there's no toast notification — the user has to be looking at the page to see the status change. Compare with `DownloadContext.tsx:92-174` which does use toasts for the Discover/Music page download flows.

### 2.10 No mobile-specific layout for DownloadsPage
**File:** `DownloadsPage.tsx` (entire file)
**Severity:** Low

The page uses `max-w-4xl` and `sm:` breakpoints but has no dedicated mobile layout. The job rows with expandable logs and the dual input mode (URL/search) could be difficult to use on small screens. The URL input placeholder is very long and will overflow on mobile.

---

## 3. POOR IMPLEMENTATIONS

### 3.1 Silent error swallowing everywhere
**File:** `DownloadsPage.tsx:69-71`, `DownloadsPage.tsx:89-90`, `DownloadContext.tsx:159-163`

Multiple `catch {}` and `catch { /* ignore */ }` blocks silently swallow errors. This makes debugging extremely difficult. At minimum, errors should be logged to console.

**Affected locations:**
- `DownloadsPage.tsx:69` — `refresh()` swallows all errors
- `DownloadsPage.tsx:89` — submit error is shown to user but not logged
- `DownloadContext.tsx:135` — search after download silently fails
- `DownloadContext.tsx:160` — poll failure logged but interval cleanup is correct
- `DownloadContext.tsx:267-269` — scan trigger failure silently ignored
- `DownloadContext.tsx:299-301` — search retry failures silently ignored

### 3.2 `expandedRef` sync pattern is fragile
**File:** `DownloadsPage.tsx:28,31-33,57-68`

The `expandedRef` is kept in sync with `expanded` state via a `useEffect`. This is a common React anti-pattern — the ref can be stale if the effect hasn't run yet. A better approach would be to read the current expanded state from a ref that's updated synchronously, or to restructure the polling to not need the expanded state.

### 3.3 Job log display uses `<pre>` with no line numbers or filtering
**File:** `DownloadsPage.tsx:349-351`

The log output is displayed in a `<pre>` tag with `max-h-64 overflow-auto`. For long logs (up to 500 lines per the backend), this is hard to navigate. No:
- Line numbers
- Search/filter within log
- "Copy log" button
- "Scroll to bottom" button
- Error/warning highlighting

### 3.4 No loading state for initial job list
**File:** `DownloadsPage.tsx:48-72`

The `refresh()` function is called on mount (line 37), but there's no loading state for the job list. The "Recent jobs" section immediately shows "No jobs yet." (line 244) which flashes briefly before the first poll completes.

### 3.5 `DownloadJob.url` displayed as primary identifier
**File:** `DownloadsPage.tsx:300`

The job's `url` field is displayed as the main identifier in the job row. For search-mode downloads, this is the raw search query (e.g., "Paradise By the Dashboard Light - Meat Loaf"), which is fine. But for Spotify URLs, it shows the full URL which is ugly and not user-friendly. There's no display of the track title/artist that was downloaded.

### 3.6 No connection between DownloadsPage and DownloadContext
**File:** `DownloadsPage.tsx` (entire file)

The DownloadsPage manages its own polling and state independently from `DownloadContext`. This means:
- Downloads started on the DownloadsPage don't show up in the DownloadProgressBar
- Downloads started on Music/Search/Discover pages don't sync with the DownloadsPage in real-time
- The `completedIds` and `completedTrackIds` in DownloadContext are never populated by DownloadsPage

### 3.7 `DownloadProgressBar` uses `downloadProgress()` API which may be redundant
**File:** `DownloadProgressBar.tsx:13`

The component calls `api.downloadProgress()` which hits `GET /api/download/progress`. But the DownloadsPage calls `api.downloadJobs()` which hits `GET /api/download/jobs`. If these return the same data, the progress endpoint is redundant. If they differ, the semantics are confusing.

---

## 4. VISUAL/UI ISSUES

### 4.1 Inconsistent help button sizing
**File:** `DownloadsPage.tsx:109-115` vs `DownloadsPage.tsx:235-241`

The "Downloads" title help button uses `HelpCircle size={16}` while the "Recent jobs" help button uses `HelpCircle size={14}`. This is inconsistent.

### 4.2 Job status text is not capitalized consistently
**File:** `DownloadsPage.tsx:302`

The job status is displayed as `{job.status}` which renders as lowercase "running", "queued", "succeeded", "failed", "cancelled". Other pages may capitalize these. Should be "Running", "Queued", etc.

### 4.3 No visual distinction between job types
**File:** `DownloadsPage.tsx:296-354`

Music downloads and podcast downloads look identical except for the optional `kind` badge. For a music-focused page, podcast downloads might be unexpected. The `kind` badge only appears when `job.kind !== "music"` (line 305-311), but there's no filter to show only music or only podcast downloads.

### 4.4 Cancel button is too small and unstyled
**File:** `DownloadsPage.tsx:332-338`

The cancel button is just `text-xs text-muted hover:text-red-400 px-2 py-1` — very easy to miss and hard to tap on mobile. Should be a more prominent button.

### 4.5 Log area has no visual hierarchy
**File:** `DownloadsPage.tsx:349-351`

The `<pre>` tag uses `bg-black/40 text-xs text-muted` — dark text on dark background with small font. Error lines are not highlighted. The `(no output yet)` placeholder looks like actual log output.

### 4.6 URL input placeholder is too long for mobile
**File:** `DownloadsPage.tsx:184`

Placeholder text `"https://open.spotify.com/track/... or playlist/album"` will overflow on mobile screens. Should be shortened or hidden on small screens.

### 4.7 No empty state illustration or guidance
**File:** `DownloadsPage.tsx:243-244`

The "No jobs yet." text is bare. For first-time users, this should include a call-to-action like "Paste a Spotify URL above to start downloading" or "Try searching for a song by name".

---

## 5. ACCESSIBILITY ISSUES

### 5.1 Missing ARIA labels on job rows
**File:** `DownloadsPage.tsx:296-354`

Job rows have no `aria-label` or `role` attributes. Screen readers will just read the URL and status without context. Should have `aria-label="Download job for [track name]: [status]"`.

### 5.2 Log toggle button has generic label
**File:** `DownloadsPage.tsx:340-346`

The toggle button has `aria-label="Toggle log"` but doesn't indicate whether it's expanding or collapsing. Should be `aria-label="Show log"` / `aria-label="Hide log"` or use `aria-expanded`.

### 5.3 No live region for job status changes
**File:** `DownloadsPage.tsx` (entire file)

When a job status changes (e.g., from "running" to "succeeded"), there's no `aria-live` region to announce the change to screen readers.

### 5.4 Form inputs lack proper labeling
**File:** `DownloadsPage.tsx:180-197`

The URL and search inputs have `placeholder` but no associated `<label>` element. Screen readers won't know what the input is for.

### 5.5 Cancel button lacks confirmation
**File:** `DownloadsPage.tsx:332-338`

The cancel button immediately cancels the download with no confirmation dialog. This is destructive and irreversible. Should at least have `aria-label="Cancel download for [track]"`.

---

## 6. PERFORMANCE ISSUES

### 6.1 No memoization of JobRow components
**File:** `DownloadsPage.tsx:247-256`

Each `JobRow` is re-rendered on every poll cycle (1.5s) even if the job data hasn't changed. The `JobRow` component should be wrapped in `React.memo()` and the `onToggle`/`onCancel` callbacks should be stable (useCallback).

### 6.2 `refresh()` creates new state objects even when data is unchanged
**File:** `DownloadsPage.tsx:48-72`

Every 1.5s, `setJobs(j)` and `setStatus(s)` are called with new objects even if the data hasn't changed. This triggers re-renders of the entire page. Should compare with previous state before setting.

### 6.3 Logs are stored per-job in state
**File:** `DownloadsPage.tsx:26,58-68`

Logs for expanded jobs are stored in a `Record<string, string[]>` state. For jobs with large logs (up to 500 lines), this creates significant memory pressure when multiple jobs are expanded. Logs should be fetched on expand and discarded on collapse.

### 6.4 `DownloadContext` creates new Set objects on every state update
**File:** `DownloadContext.tsx:120-125, 145-148, 153-157`

In the `trackDownload` polling callback, new `Set` objects are created for `downloadingIds` and `completedIds` on every state update. This causes unnecessary re-renders of all consumers.

---

## 7. SUMMARY OF FINDINGS

| Category | Critical | Medium | Low | Total |
|----------|----------|--------|-----|-------|
| Bugs | 0 | 5 | 2 | 7 |
| Missing Features | 0 | 5 | 5 | 10 |
| Poor Implementations | 0 | 4 | 3 | 7 |
| Visual/UI | 0 | 2 | 5 | 7 |
| Accessibility | 0 | 2 | 3 | 5 |
| Performance | 0 | 2 | 2 | 4 |
| **TOTAL** | **0** | **20** | **20** | **40** |

---

## 8. PRIORITIZED FIX ROADMAP

### Phase 1: Critical Fixes (should do first)
1. Fix `submitting` state bug when entering whitespace (1.3)
2. Add toast notifications to DownloadsPage (2.9)
3. Consolidate DownloadsPage polling with DownloadContext (3.6)
4. Add error logging to all silent catch blocks (3.1)

### Phase 2: Important Improvements
5. Add download progress display to JobRow (2.2)
6. Add retry button for failed downloads (2.3)
7. Add clear/remove job functionality (2.1)
8. Fix cancel button styling and add confirmation (4.4)
9. Add proper ARIA labels and live regions (5.1, 5.2, 5.3)

### Phase 3: Nice-to-Have
10. Add loading state for initial job list (3.4)
11. Add log search/filter/line numbers (3.3)
12. Add keyboard shortcuts (2.5)
13. Add mobile-specific layout improvements (2.10)
14. Add memoization for JobRow components (6.1)
15. Add empty state guidance (4.7)
