# Scanner ON CONFLICT(path) Fix — Review

**Reviewer:** ops (Atlas)  
**Date:** 2026-05-22  
**Parent task:** t_0f1613e0 — "FIX: scanner ON CONFLICT(path) broken after dedup migration"

## Fix Summary

The parent task replaced the broken `INSERT ... ON CONFLICT(path) DO UPDATE` with explicit `SELECT`-then-`UPDATE`/`INSERT`. The old ON CONFLICT was a silent no-op after the dedup migration removed the UNIQUE constraint from `tracks.path` (replaced with a partial unique index on `(user_id, path) WHERE user_id IS NOT NULL`).

**New logic** (scanner.go lines 124-183):
1. `SELECT id, mtime FROM tracks WHERE path=? AND user_id IS NULL LIMIT 1` — find existing scanner-owned track
2. If found and mtime unchanged → skip (incremental scan)
3. If found (mtime changed) → `UPDATE` by `id`, preserving `added_at`
4. If not found → `INSERT` with `added_at = time.Now().Unix()`, `user_id = NULL`

---

## Verification Results

### 1. Build Checks ✅

```
go build ./internal/... — EXIT 0
go build ./cmd/server   — EXIT 0
```

Both targets compile cleanly.

### 2. Upsert Logic — NULL vs non-NULL user_id ✅

The scanner only queries for `WHERE user_id IS NULL`. This is correct:
- Scanner-created tracks always have `user_id IS NULL`
- Per-user copies (`user_id IS NOT NULL`) are never touched by the scanner
- The `UPDATE` path targets by `id` (the specific row found), not by path — so even if there were somehow duplicates, only the row identified in step 1 would be updated
- The `INSERT` path explicitly sets `user_id` to `nil`

**Edge case considered:** If a per-user track exists for the same path alongside the scanner track, the scanner correctly ignores the user track and only manages the NULL-user_id copy. This is the intended dedup architecture.

### 3. Duplicate Verification ✅

```
SELECT COUNT(*) FROM (
  SELECT path, COUNT(*) as cnt FROM tracks
  WHERE user_id IS NULL
  GROUP BY path HAVING cnt > 1
) → 0
```

Zero duplicate tracks with `user_id IS NULL` for the same path. The `LIMIT 1` in the SELECT query is safe — there's no risk of missing duplicates because none exist.

DB snapshot: 414 total tracks, 133 with `user_id IS NULL`.

### 4. `added_at` Timestamps ✅ (with caveat)

**Code:** `time.Now().Unix()` on INSERT (line 181). Correct.

**DB state:**
- 84 existing tracks have `added_at = 0` — all scanner-owned (`user_id IS NULL`), all pre-fix legacy
- 0 tracks have `added_at IS NULL`
- New tracks inserted after the fix will get correct timestamps
- The `UPDATE` path (line 166-175) does NOT touch `added_at` — this is correct; `added_at` should reflect the original import time, not subsequent metadata refreshes

**Recommendation:** The 84 legacy tracks with `added_at = 0` could be backfilled with a one-time migration (`UPDATE tracks SET added_at = mtime WHERE added_at = 0 AND user_id IS NULL`), but this is cosmetic and not required for correctness.

### 5. Other Scanner Behavior — Preserved ✅

The fix only touched the upsert block in `indexFile()` (lines 163-183). No other logic was changed:

- **Loudness measurement** (lines 188-205): Async goroutine, semaphore-bounded, `context.WithTimeout` before `exec.CommandContext` — all preserved
- **Loudness UPDATE** (line 198-200): Still uses `WHERE path=?` which updates ALL rows with that path (including per-user copies). This is intentional — loudness is a file property, identical for all records.
- **Podcast detection** (lines 157-161): Unchanged — genre/path-based classification
- **File size validation** (lines 118-122): Unchanged — sub-10KB files skipped
- **mtime skip** (lines 128-130): Unchanged — incremental scan optimization
- **Metadata extraction** (lines 132-154): Unchanged — `dhowden/tag` library
- **Format support** (lines 80-91): Unchanged — all 10 formats preserved

### 6. Development Context — Needs Update ❌

**File:** `backend/internal/scanner/development_context.md`

Two issues found:

1. **LOC count is stale:** Reports 184 LOC, actual file is 208 lines (+24 from the fix's SELECT/UPDATE/INSERT expansion)

2. **Key function description is wrong:** Line 40 still says:
   ```
   7. `INSERT ... ON CONFLICT(path) DO UPDATE` — upsert by path
   ```
   Should say something like:
   ```
   7. `SELECT WHERE path=? AND user_id IS NULL` → `UPDATE by id` if found, `INSERT` if not (ON CONFLICT no longer valid after dedup migration removed UNIQUE on path)
   ```

Both issues should be fixed.

### 7. Schema Alignment ✅

The new upsert correctly maps to the current schema:

| Schema column | INSERT value | UPDATE value | Notes |
|---|---|---|---|
| `path` | `path` | — (WHERE id) | Not updated on existing rows |
| `added_at` | `time.Now().Unix()` | — (preserved) | Correct: not overwritten |
| `user_id` | `nil` | — (preserved) | Scanner always creates NULL user_id |
| `mtime` | `mtime` | `mtime` | Updated on both paths |
| `loudness_*` | `0.0, 0.0, 0.0` | `0.0, 0.0, 0.0` | Zeroed on upsert, then async-filled |
| `duration_sec` | — | — | Never set by scanner (known limitation) |

All 19 INSERT columns and 15 UPDATE columns match the schema. No missing or extra columns.

---

## Overall Verdict: ✅ APPROVED

The fix is correct. The SELECT-then-UPDATE/INSERT pattern properly handles the post-dedup schema where `path` is no longer UNIQUE. All build checks pass. No duplicate scanner tracks exist in the current DB. `added_at` is set correctly on INSERTs. The UPDATE path correctly preserves `added_at`. No other scanner behavior was broken.

**Required follow-up:** Update `backend/internal/scanner/development_context.md` — correct the LOC count (184→208) and the upsert description (line 40).
