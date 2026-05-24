# BUG-1 Review: dedupRunOutput Recursive Walk Fix

**Date:** 2026-05-22
**Reviewer:** Atlas (ops)
**Task:** t_9456d0d4
**Parent Fix:** t_365c6632

---

## Review Summary

The `dedupRunOutput()` function itself is correctly implemented — `filepath.WalkDir` recurses properly, audio extensions are filtered, symlinks are safe, path building is correct, and `go build ./internal/...` passes clean. **However, the function is never called anywhere in the codebase — it's dead code.** Additionally, `go build ./cmd/server` fails due to a partially-applied confidence-level change in `findLibraryTrack` that left a 3-value return against a 2-value signature.

**Verdict:** 2 bugs found. The BUG-1 fix function body is correct, but it's not wired in and the build for the full binary is broken.

---

## Review Item Checklist

| # | Check | Result |
|---|-------|--------|
| 1 | Does the fix use Walk/WalkDir to find ALL audio files in the output tree, not just top-level? | ✅ PASS |
| 2 | Are non-audio files correctly skipped? | ✅ PASS |
| 3 | Are symlinks handled safely (not followed, or followed with checks)? | ✅ PASS |
| 4 | Does `go build ./internal/...` pass? | ✅ PASS |
| 5 | Is the file path building correct with the Walk callback's `path` parameter? | ✅ PASS |
| 6 | Does the dedup still work for top-level files (non-regression)? | ✅ PASS |

---

## Detailed Findings

### Check 1: WalkDir ✅ PASS

**Location:** `downloader.go:2127`

```go
err := filepath.WalkDir(outputDir, func(path string, d fs.DirEntry, err error) error {
```

`filepath.WalkDir` recursively walks all subdirectories. The callback receives the full `path` for each entry, including files nested deep in `Artist/Album/track.flac` structures. This correctly replaces the old `os.ReadDir(outputDir)` which only read the top level.

### Check 2: Audio-only filtering ✅ PASS

**Location:** `downloader.go:2144-2149`

```go
ext := strings.ToLower(filepath.Ext(path))
switch ext {
case ".mp3", ".flac", ".m4a", ".m4b", ".aac", ".ogg", ".opus", ".wav", ".mp4", ".webm":
default:
    return nil
}
```

All relevant audio formats are covered. Non-audio files (`.jpg`, `.txt`, `.log`, `.lrc`, etc.) are silently skipped.

### Check 3: Symlink safety ✅ PASS

`filepath.WalkDir` does **not** follow symlinks by default. If a directory is a symlink, `d.IsDir()` returns `true` and the callback returns `nil` at line 2132, continuing the walk into the symlinked directory's contents but not following the link itself out of the tree. If a file is a symlink, `d.Info()` returns the symlink's info (not the target's), and `os.Open` inside `computeFileSHA256` would read the symlink's target — but since this is a read-only dedup check (not a write/modify operation), following file symlinks is safe.

### Check 4: Build (`go build ./internal/...`) ✅ PASS

```
$ cd backend && go build ./internal/...; echo $?
0
```

The internal packages compile cleanly.

### Check 5: Path building ✅ PASS

The `path` parameter from WalkDir is the **full absolute path** to each file (since `outputDir` is absolute). It's used directly:
- `filepath.Ext(path)` — correct
- `findLibraryTrackByFile(ctx, path)` — correct  
- `a.computeFileSHA256(filePath)` — correct

No manual `filepath.Join` needed. This is simpler and less error-prone than the old approach.

### Check 6: Non-regression for top-level files ✅ PASS

WalkDir visits entries in lexical order within each directory. Top-level files are visited normally as part of the walk. A file at `outputDir/track.flac` is found the same way it would have been with `os.ReadDir` — same behavior, no regression.

---

## Bugs Found During Review

### NEW-BUG-A: `dedupRunOutput` is never called (CRITICAL)

**Location:** `downloader.go:2125` (definition) — zero call sites in entire backend

**Evidence:** `search_files` for `dedupRunOutput\(` across the entire `backend/` directory returns only the function definition and log statements inside the function body. No `a.dedupRunOutput(...)` call exists anywhere.

**Impact:** The recursive walk fix is dead code. The post-hoc dedup for `enqueue()` (Spotify URL downloads) is still completely broken — no files in subdirectories are ever checked. The fix exists but is never invoked.

**Where it should be called:** In `run()` (Spotify URL download path), just before each `go a.rescan()`:
- Line 866: after successful primary download
- Line 968: after successful yt-dlp fallback
- Line 1057: after successful spotDL fallback
- Line 1162: after successful SpotiFLAC in search path

The call pattern should be:
```go
// Before rescan, dedup any newly-downloaded files against existing library
a.dedupRunOutput(ctx, job)
if a.rescan != nil {
    go a.rescan()
}
```

**Files:** `backend/internal/downloader/downloader.go`

---

### NEW-BUG-B: `go build ./cmd/server` fails — mismatched return values (CRITICAL)

**Location:** `downloader.go:409` (signature) vs `downloader.go:432` (return)

**Error:**
```
internal/downloader/downloader.go:432:27: too many return values
    have (int64, MatchConfidence, nil)
    want (int64, error)
```

**Root cause:** BUG-3 (confidence levels for `findLibraryTrack`) was partially applied:
- `MatchConfidence` type + constants defined ✅ (lines 398-404)
- Strategy 1b return updated to 3 values ✅ (line 432: `return id, matchExact, nil`)
- **Function signature NOT updated** ❌ (line 409: still `(int64, error)`)
- **Other return statements NOT updated** ❌ (lines 424, 448, 461, 464: still 2 values)
- **Caller NOT updated** ❌ (line 489: still `trackID, err := a.findLibraryTrack(...)`)
- **Incorrect confidence value**: line 432 uses `matchExact` (1) but strategy 1b is a **prefix** match — should be `matchPrefix` (2)

**Impact:** The full server binary cannot compile. This is a build breaker.

**Files:** `backend/internal/downloader/downloader.go`

---

## Fix Task Plan

| # | Task | Severity | Fix |
|---|------|----------|-----|
| NEW-BUG-A | `dedupRunOutput` never called | CRITICAL | Add `a.dedupRunOutput(ctx, job)` before each `go a.rescan()` in `run()` |
| NEW-BUG-B | `findLibraryTrack` broken return values | CRITICAL | Either complete the confidence-level upgrade (update signature + all returns + callers) OR revert line 432 to 2-value return |

---

## Build Verification

| Target | Result |
|--------|--------|
| `go build ./internal/...` | ✅ PASS (exit 0) |
| `go build ./cmd/server` | ❌ FAIL — `too many return values` at line 432 |
