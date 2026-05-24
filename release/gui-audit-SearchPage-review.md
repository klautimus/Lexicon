# GUI Audit: SearchPage.tsx — IMPLEMENTATION REVIEW

**Date:** 2026-05-22
**Reviewer:** Atlas (ops)
**Scope:** Verify all P0/P1/P2 audit fixes were correctly implemented
**Build status:** Backend ✅ (go build clean), Frontend ✅ (vite build clean, 793KB bundle)

---

## REVIEW SUMMARY

**All P0, P1, and P2 fixes from the audit plan are correctly implemented and building clean.**

- **P0 (Critical):** 4/4 fixed
- **P1 (High):** 9/9 fixed
- **P2 (Medium):** 8/8 fixed (per the prioritized roadmap; 3 additional P2 items from the audit were intentionally deferred — see below)
- **New bugs introduced:** 0

---

## P0 — CRITICAL (4/4)

### 2.1 ✅ Silent error swallowing in search handler
**Plan:** Pass error messages to toasts instead of discarding.
**Implementation (lines 201-211):** The `catch` block now captures the error, checks for cancellation, and differentiates network errors from server errors with specific toast messages:
- "Cannot connect to server. Check your connection." for network errors
- "Server error. Please try again later." for 500/502/503
- The actual error message for everything else

### 2.2 ✅ Silent error swallowing in download handler
**Plan:** Pass error messages to toasts.
**Implementation (lines 229-237):** The `catch` block now captures the error and differentiates network errors from other failures.

### 2.5 ✅ Use DownloadContext instead of duplicate polling
**Plan:** Replace inline `trackDownload` + `pollRef` with `useDownloads().trackDownload()`.
**Implementation (lines 36, 225-226):** `const { trackDownload } = useDownloads()` replaces the old inline polling. The `pollRef` and `DownloadJob` import are removed. Downloads now integrate with the global DownloadContext (show in DownloadProgressBar, appear on DownloadsPage).

### 3.1 ✅ Race condition: search results stale after download
**Plan:** Add retry/delay after download before re-searching.
**Implementation (lines 122-180):** `retrySearchAfterDownload()` triggers `api.scan()`, waits 3s for the scanner to start, then polls for up to 3 minutes (60 attempts x 3s). Uses `safeSetResults` with mounted guard. This mirrors the pattern from DownloadContext.tsx lines 266-304.

---

## P1 — HIGH (9/9)

### 3.2 ✅ setState after unmount from polling
**Plan:** Guard setState calls with mounted ref.
**Implementation:** `mountedRef` (line 55) is checked in:
- `safeSetResults` (line 116) — guards `setResults` and `setResultCount`
- Retry interval (lines 144, 156) — guards interval callbacks
- `go()` finally block (line 213) — guards `setLoading(false)`

### 3.3 ✅ Download state not reset on new search
**Plan:** Reset `downloading` state when a new search starts.
**Implementation (line 197):** `setDownloading(false)` is called at the start of `go()`.

### 3.5 ✅ Cancel in-flight search on unmount
**Plan:** Use AbortController to cancel in-flight search requests.
**Implementation (lines 57, 190-193, 82-84):** `abortRef` holds an `AbortController`. Each `go()` call creates a new one and aborts the previous. The cleanup effect aborts on unmount. The `j()` function in api.ts already handles `AbortError` (line 36-37) by throwing "Request was cancelled." which is caught and silently ignored (line 202).

### 3.6 ✅ Stale closure in onDelete callback
**Plan:** Use ref for query to avoid stale closure.
**Implementation (lines 56, 61-63, 241-246):** `queryRef` stays in sync with `q` via a `useEffect`. `handleDelete` reads `queryRef.current` instead of `q` from closure.

### 3.8 ✅ Guard against duplicate downloads
**Plan:** Check `downloading` state before initiating download.
**Implementation (line 220):** `if (!q.trim() || downloading) return;` — the guard check happens before `setDownloading(true)`.

### 1.15 ✅ Add pagination for search results
**Plan:** Add pagination or virtualization.
**Implementation (lines 9-11, 47-48, 51-52, 92-94, 248-250, 273, 364-373):** `PAGE_SIZE = 200`, `displayCount`/`resultCount` state, `hasMore` check, "Load More" button showing remaining count, `displayedResults = results.slice(0, displayCount)`. Pagination resets when results change.

### 5.1 ✅ Add aria-label to search input
**Plan:** Add aria-label or associated label element.
**Implementation (line 296):** `aria-label="Search"` on the input element.

### 5.3 ✅ Add aria-live for search results
**Plan:** Add aria-live region to announce result changes.
**Implementation (line 359):** `<div aria-live="polite" aria-atomic="true">` wraps the results section.

### 2.3 ✅ Add console.error to polling catch block
**Plan:** Add console.error for debugging in polling catch.
**Implementation:** SearchPage no longer has its own polling — it delegates to `DownloadContext.trackDownload()` which already has `console.error` at line 162. Fixed by elimination.

---

## P2 — MEDIUM (8/8)

### 1.1 ✅ Add search history
**Implementation (lines 10-31, 48, 187, 260-268, 270-271, 309-347):** Full search history system with localStorage persistence (`lexicon_search_history` key, max 10 entries). History dropdown appears on input focus with History icon toggle. Each entry is clickable to re-search. Clear button removes all history.

### 1.4 ✅ Show search result count
**Implementation (lines 360-362):** `{resultCount} result{resultCount !== 1 ? "s" : ""} for "{q.trim()}"` displayed above results.

### 1.6 ✅ Add clear search button
**Implementation (lines 252-258, 299-307):** X button appears when input has text. `handleClear()` resets query, results, searched state, result count, and display count.

### 3.4 ✅ Sync search query to URL
**Implementation (lines 39-42, 65-75):** `useState` initializer reads from `window.location.search`. `useEffect` syncs `q` back via `history.replaceState`. URL format: `/search?q=query`.

### 3.7 ✅ Disable form submission while loading
**Implementation (lines 184, 349):** `go()` returns early if `loading` is true. Search button has `disabled={loading}` and shows "Searching…" text.

### 3.9 ✅ Reset downloading state on early return
**Implementation (line 220):** The guard `if (!q.trim() || downloading) return;` is placed BEFORE `setDownloading(true)`, so an empty query returns without touching the downloading state.

### 4.3 ✅ Replace emoji spinner with Lucide Loader2
**Implementation (line 392):** `<Loader2 size={16} className="animate-spin" />` replaces the old `<span className="animate-spin">⟳</span>`.

### 1.12 ✅ Differentiate error types in toast messages
**Implementation (lines 203-211, 231-235):** Both search and download handlers differentiate:
- Network errors: "Cannot connect to server. Check your connection."
- Server errors (500/502/503): "Server error. Please try again later."
- Other errors: the actual error message

---

## INTENTIONALLY DEFERRED (P2 — Nice to Have)

These items from the audit plan were not included in the prioritized fix roadmap:

| Finding | Description | Reason |
|---------|-------------|--------|
| 1.2 | Search filters (media kind, genre, year) | Requires backend API changes + new UI components |
| 1.7 | Empty state with suggestions before first search | UX enhancement, not a bug |
| 1.14 | "Play All" button for search results | Feature addition, not a bug fix |

---

## NEW BUGS FOUND: 0

No new bugs, logic errors, race conditions, missing error handling, broken imports, or regressions were introduced by these changes.

### Specific checks performed:

1. **AbortController signal propagation:** Verified that `api.search(q, { signal })` correctly passes the `AbortSignal` through `RequestInit` to the `j()` function, which spreads `...init` into the `fetch()` call. The `j()` function also correctly catches `AbortError` and throws "Request was cancelled." which is handled in the `catch` block.

2. **DownloadContext delegation:** Verified that `trackDownload(job, q.trim())` from `useDownloads()` properly handles all job statuses (succeeded/failed/cancelled/queued) and includes `console.error` in its polling catch block.

3. **Mounted ref lifecycle:** Verified that `mountedRef` is set to `true` on mount (line 79), `false` on unmount (line 81), and checked in all async callbacks (`safeSetResults`, retry interval, `go()` finally block).

4. **URL sync:** Verified that the URL sync `useEffect` uses `replaceState` (not `pushState`) to avoid polluting browser history, and correctly handles empty query by removing the param.

5. **Query ref sync:** Verified that `queryRef.current` is updated in a `useEffect` that depends on `q`, ensuring it's always current when `handleDelete` reads it.

6. **Build verification:** Both `go build ./internal/...` and `npm run build` pass with zero errors.

7. **Import cleanup:** Verified that `DownloadJob` was removed from imports (no longer needed) and `Loader2` was added. All imports resolve correctly (confirmed by clean build).

8. **Visual consistency:** All new UI elements use the existing Tailwind classes (`bg-panel`, `border-panel2`, `text-muted`, `text-accent`, `rounded-md`, etc.) consistent with the dark theme. The Lucide `Loader2` icon replaces the emoji spinner for consistency.

---

## CONCLUSION

**All P0, P1, and P2 fixes from the audit plan are correctly implemented. Both builds pass clean. No new bugs introduced. The implementation is ready for merge.**
