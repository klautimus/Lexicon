# BUG-3 Review: findLibraryTrack Confidence Levels Fix

**Date:** 2026-05-22
**Reviewer:** Atlas (ops)
**Task:** t_e8c47811
**Parent Fix:** t_ef53d294

---

## Review Summary: ❌ FAIL — Fix not applied correctly

The parent task claimed to add `MatchConfidence` to `findLibraryTrack()` and gate cross-user sharing on confidence levels. In reality, **none of the required changes were applied correctly.** The `MatchConfidence` type and constants were never added, `findLibraryTrack()` was never updated, and `searchEnqueue` contains a malformed duplicate code block. The downloader package **does not compile**.

---

## Review Item Checklist

| # | Check | Result |
|---|-------|--------|
| 1 | Does `findLibraryTrack()` return a confidence level alongside trackID? | ❌ FAIL |
| 2 | Are all callers updated for the new return signature? | ❌ FAIL |
| 3 | In `searchEnqueue`, is sharing only triggered for confidence <= matchPrefix? | ❌ FAIL |
| 4 | Are lower-confidence matches (FTS5, LIKE) still resolved but NOT auto-shared? | ❌ FAIL |
| 5 | Does `go build ./internal/...` pass? | ❌ FAIL |
| 6 | Are there any callers that depend on the old 2-return-value signature? | ❌ FAIL |
| 7 | Does the confidence enum ordering make sense (lower = more confident)? | ❌ N/A |

---

## Findings

### F-1: `findLibraryTrack` never updated (BLOCKING)

**Location:** `downloader.go:422`

The function signature is still the original:

```go
func (a *API) findLibraryTrack(ctx context.Context, query string) (int64, error) {
```

It returns `(int64, error)` — **not** `(int64, MatchConfidence, error)`. All four strategies still return `(id, nil)` on success with no confidence information. The final fallback still returns `(0, sql.ErrNoRows)`.

### F-2: `MatchConfidence` type and constants never defined (BLOCKING)

**Location:** Nowhere in the codebase

A grep for `MatchConfidence`, `matchExact`, `matchFTS`, and `matchLike` across the entire `backend/` directory returns **zero results**. Only `matchPrefix` appears (as a bare undefined identifier at lines 636 and 668).

Without this type and const block:
```go
type MatchConfidence int
const (
    matchExact  MatchConfidence = 1
    matchPrefix MatchConfidence = 2
    matchFTS    MatchConfidence = 3
    matchLike   MatchConfidence = 4
)
```
…nothing else can work.

### F-3: Callers destructure 3 values from a 2-value function (BLOCKING)

**Location:** `downloader.go:629` and `downloader.go:664`

Both call sites:
```go
trackID, confidence, err := a.findLibraryTrack(r.Context(), query)
```

…expect 3 return values. But `findLibraryTrack` returns only 2. This is a compile error.

Additionally, line 664 is inside a **duplicate code block** (see F-4), meaning `trackID`, `confidence`, and `err` are redeclared in the same scope — another compile error.

### F-4: Duplicate/partial code block in `searchEnqueue` (BLOCKING)

**Location:** `downloader.go:662-711`

The `searchEnqueue` function contains two conflicting dedup blocks:

- **Lines 628-661**: Cross-user dedup with `shareTrack()`. Handles the case where a track exists in another user's library — shares it and creates a completed job. Includes BUG-2 fix (own-ID lookup).

- **Lines 662-708**: A SECOND dedup block, starting at `// Check library first to avoid re-downloading existing tracks`. This block does **auto-resolve** (creates a succeeded job without sharing). It's structurally broken — it sits inside the first dedup block's scope but has its own `if a.db != nil` guard.

- **Lines 709-711**: Three orphaned closing braces (`}\n\t}\n}`) that don't match any opening braces.

This is clearly a merge/patch failure — the fix attempted to add a new dedup path but pasted it as a duplicate rather than integrating it with the existing code.

**Effect on compilation:** The brace mismatch cascades into subsequent functions. The Go parser sees `jobSummary` (line ~750) as appearing inside a broken function body, producing:
```
downloader.go:750:6: syntax error: unexpected name jobSummary, expected (
downloader.go:761:6: syntax error: unexpected name jobFull, expected (
downloader.go:768:26: syntax error: unexpected name http in argument list
```

### F-5: Build verification

```
$ go build ./internal/downloader/
# github.com/kevin/lexicon/internal/downloader
internal/downloader/downloader.go:750:6: syntax error: unexpected name jobSummary
internal/downloader/downloader.go:761:6: syntax error: unexpected name jobFull
internal/downloader/downloader.go:768:26: syntax error: unexpected name http
```

Note: `go build ./internal/...` may appear to pass due to build cache — the downloader package must be built explicitly with `./internal/downloader/` or after a `go clean -cache`.

### F-6: Design observation — duplicate dedup logic

Even if the code compiled, having two different dedup blocks (cross-user sharing vs. auto-resolve) with different behaviors is confusing. The fix should integrate confidence gating into the **existing** cross-user sharing block (lines 628-661) rather than adding a parallel code path. The current duplication creates maintenance risk — future changes to one block won't propagate to the other.

---

## What the fix SHOULD look like

1. Add the `MatchConfidence` type and constants before `findLibraryTrack`
2. Update `findLibraryTrack` signature to `(int64, MatchConfidence, error)` and return appropriate confidence for each strategy
3. Update the single call site in `searchEnqueue` (line 629) to capture confidence
4. Gate the share logic on `confidence <= matchPrefix`
5. Remove the duplicate block at lines 662-711
6. Remove the orphaned braces at lines 709-711
7. Ensure `go build ./internal/downloader/` passes

---

## Severity: CRITICAL (does not compile)

The parent task claimed all checks passed and the build was verified. This is incorrect — the downloader package has 3 distinct compile errors and structural corruption from a failed merge.

---

## Fix Task Required

A new fix task is needed to correctly implement the confidence-level changes. The approach is straightforward (see "What the fix SHOULD look like" above) but must be done from the clean pre-fix code, not by patching the current broken state.
