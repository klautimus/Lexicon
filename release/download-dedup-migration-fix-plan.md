# Fix Plan: Lexicon Crash on Launch — columnExists Regex Bug

**Date:** 2026-05-22
**Severity:** CRITICAL — App crashes on launch for both fresh installs and upgrades
**Root Cause:** `columnExists()` regex `^[a-z_]+$` rejects column names containing digits

---

## Root Cause Analysis

The `columnExists()` function in `db.go` uses a regex `^[a-z_]+$` to validate column names before querying `PRAGMA table_info()`. This regex only allows lowercase letters and underscores — **it rejects digits**.

The `file_sha256` column name contains digits (`256`). When `columnExists("tracks", "file_sha256")` is called:
1. The regex rejects `file_sha256` as "invalid"
2. `columnExists` returns `false`
3. The migration tries `ALTER TABLE tracks ADD COLUMN file_sha256 TEXT`
4. On a **fresh install**, the column already exists (it's in the `CREATE TABLE` schema), so this fails with `duplicate column name: file_sha256`
5. On an **upgrade**, the column doesn't exist yet (old schema), so the ALTER TABLE succeeds — but the migration then fails later when `INSERT INTO tracks_v2 SELECT * FROM tracks` runs with mismatched column counts (24 vs 25)

Both paths cause `Migrate()` to return an error, which triggers `log.Fatalf("db migrate: %v", err)` in `main.go`, appearing as a crash on launch.

---

## Fix Applied

### 1. CRITICAL: Fixed `columnExists` regex to allow digits

**File:** `backend/internal/db.go`, line 264

**Before:**
```go
if matched, _ := regexp.MatchString(`^[a-z_]+$`, column); !matched {
```

**After:**
```go
if matched, _ := regexp.MatchString(`^[a-z0-9_]+$`, column); !matched {
```

This allows column names like `file_sha256` to pass validation.

### 2. LOW: Moved `file_sha256` column addition before dedup migration

**File:** `backend/internal/db.go`

Moved the `file_sha256` ALTER TABLE from after the dedup migration to before it. This ensures the old table has 25 columns (including `file_sha256`) when `INSERT INTO tracks_v2 SELECT * FROM tracks` runs during the dedup table recreation.

Also removed the now-redundant duplicate `file_sha256` block that was at the end of the migration.

### 3. LOW: Added `rows.Err()` check to `recoverJobs`

**File:** `backend/internal/downloader/downloader.go`

Added `rows.Err()` check after the scan loop to catch any scan errors that were previously silently ignored.

### 4. LOW: Added dedup columns to `enqueue()` INSERT

**File:** `backend/internal/downloader/downloader.go`

Added `dedup_source_track_id` and `dedup_method` to the INSERT statement in `enqueue()` so dedup metadata is persisted for URL-based downloads.

---

## Verification

### Tests Performed
1. **Fresh install migration:** Create new DB → `Migrate()` → verify 25 columns in tracks table ✅
2. **Upgrade migration:** Create old schema (24 columns, UNIQUE on path) → insert test data → `Migrate()` → verify 25 columns, data survived, UNIQUE removed ✅
3. **Go build:** `go build ./internal/...` and `go build ./cmd/server` both pass ✅
4. **TypeScript:** `npx tsc --noEmit` passes ✅

### Files Changed
- `backend/internal/db/db.go` — Fixed regex, reordered migration
- `backend/internal/downloader/downloader.go` — Added rows.Err() check, added dedup columns to INSERT
