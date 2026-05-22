# SpotiFLAC Integration Plan — Primary Download Engine for Lexicon

> **Version:** 1.0.0
> **Date:** 2026-05-20
> **Author:** Atlas (ops)
> **Status:** Draft — Ready for implementation

---

## 1. Executive Summary

The SpotiFLAC binary at `tools/spotiflac.exe` has been rebuilt from source with critical fixes: working Qobuz providers (WJHE, GDStudio, MusicDL) as the default service (replacing the dead Tidal provider), Amazon Music provider support, format detection via magic bytes for correct MP3 vs FLAC handling, and proper metadata embedding for both FLAC (Vorbis) and MP3 (ID3).

**What needs to change:**

1. **Replace the old binary** — Copy `spotiflac-cli-fixed.exe` over `tools/spotiflac.exe` and `release/tools/spotiflac.exe`
2. **Update the CLI invocation** — The new binary uses `spotiflac download --output <dir> [--service qobuz|amazon|tidal] <url>` (subcommand-based) instead of the old `spotiflac -o <dir> [-folder-format <fmt>] <url>` (flag-based)
3. **Update output parsing** — The new binary has different stdout/stderr output format (per-track progress lines like `[1/5] Track Name - Artist`, no "Summary:" line)
4. **Add `--service` config option** — Let users choose `qobuz` (default), `amazon`, or `tidal` via `SPOTIFLAC_SERVICE` env var
5. **Update fallback chain logic** — The new binary exits non-zero on failure (unlike the old one which always exited 0), so the `spotiflacReportedFailure` summary-parsing logic needs adjustment
6. **Update frontend messaging** — Downloads page and tooltips should reflect SpotiFLAC as the primary engine with Qobuz/Amazon sources
7. **Update .env.example** — Document the new `SPOTIFLAC_SERVICE` variable

**Why:** The old SpotiFLAC binary was built around a Tidal API integration that has been dead for months. Every download silently failed at the SpotiFLAC level, falling through to yt-dlp (YouTube) — meaning users were never getting FLAC-quality audio from Qobuz/Amazon. The fixed binary makes Qobuz the default, which reliably provides lossless FLAC downloads.

---

## 2. Current Architecture

### 2.1 Download Pipeline Flow

```
User submits Spotify URL
  │
  ▼
POST /api/download  (enqueue)
  │
  ├─ Validates URL (must be open.spotify.com or spotify: URI)
  ├─ Creates Job in memory + SQLite (download_jobs table)
  ├─ Returns job summary immediately
  │
  ▼
goroutine: a.run(job, ctx)
  │
  ├─ Acquire semaphore slot (max 2 concurrent)
  ├─ Set job.Status = "running", job.Tool = "spotiflac"
  │
  ├─ [TIER 1] SpotiFLAC
  │   ├─ Command: spotiflac.exe -o <output> [-folder-format <fmt>] <url>
  │   ├─ Runs via runProcess() with 30-min timeout
  │   ├─ SpotiFLAC ALWAYS exits 0 (even on failure)
  │   ├─ Parse "Summary: N Success, M Failed" line
  │   ├─ If Success > 0 → finish(succeeded) → rescan
  │   └─ If Success == 0 → fall through
  │
  ├─ [TIER 2] yt-dlp (fallback)
  │   ├─ Only if YtdlpBin is configured
  │   ├─ Parses "Found Track:" / "Failed:" lines from SpotiFLAC log
  │   ├─ Command: yt-dlp ytsearch1:<query> -f bestaudio/best --add-metadata ...
  │   ├─ On success: ffprobe validation → retry with ytsearch2 if invalid
  │   ├─ On success → finish(succeeded) → rescan
  │   └─ On failure → fall through
  │
  └─ [TIER 3] spotDL (final fallback)
      ├─ Only if SpotdlBin is configured
      ├─ Command: spotdl download <url> --output <template> --format <fmt>
      ├─ Uses parsed track queries if available, else raw Spotify URL
      └─ On success → finish(succeeded) → rescan
```

### 2.2 Search Download Flow

```
User submits free-text query
  │
  ▼
POST /api/download/search  (searchEnqueue)
  │
  ├─ Checks library first (findLibraryTrack with exact/prefix/FTS5/LIKE strategies)
  ├─ If found → return immediately with track_id (no download needed)
  ├─ If not found → create Job, persist to DB
  │
  ▼
goroutine: a.runSearch(job, ctx)
  │
  ├─ Optionally parse query via DeepSeek (30s timeout)
  │   └─ Returns {type, artist, title, album, search_query}
  ├─ Command: yt-dlp ytsearch1:<query> -f bestaudio/best --match-filter duration < 600
  ├─ ffprobe validation → retry with ytsearch2 if invalid
  ├─ On success: poll DB for 2min to resolve file → track_id
  └─ finish(succeeded/failed) → rescan
```

### 2.3 Configuration Flow

```
.env file
  │
  ├─ SPOTIFLAC_BIN → config.SpotiflacBin → downloader.Config.Bin
  ├─ SPOTIFLAC_OUTPUT → config.SpotiflacOutput → downloader.Config.Output
  ├─ SPOTIFLAC_FOLDER_FORMAT → config.SpotiflacFolderFmt → downloader.Config.FolderFormat
  ├─ SPOTDL_BIN → config.SpotdlBin → downloader.Config.SpotdlBin
  ├─ YTDLP_BIN → config.YtdlpBin → downloader.Config.YtdlpBin
  ├─ FFMPEG_BIN → config.FfmpegBin → downloader.Config.FfmpegBin
  └─ FFPROBE_BIN → config.FfprobeBin → downloader.Config.FfprobeBin
         (auto-detected from FFMPEG_BIN if empty)

main.go wiring:
  cfg.SpotiflacOutput → dlOutput (falls back to first MEDIA_ROOTS entry)
  downloader.New(cfg{...}, database, doRescan)
```

### 2.4 Job Persistence

```
download_jobs table:
  id TEXT PK, url TEXT, output TEXT, status TEXT,
  started_at INTEGER, finished_at INTEGER, error TEXT,
  tool TEXT, used_fallback INTEGER DEFAULT 0,
  is_search INTEGER DEFAULT 0, track_id INTEGER,
  kind TEXT DEFAULT 'music', created_at INTEGER DEFAULT now

Startup recovery (recoverJobs):
  - Marks stale running/queued jobs as 'failed'
  - Loads most recent 50 jobs into memory (without logs)
```

### 2.5 Frontend Download UI

- **DownloadsPage.tsx** — Dual-mode (Spotify URL / Search), 1.5s polling, expandable logs
- **MusicPage.tsx** — Client-side filter, "Search & Download from Web" when no results
- **SearchPage.tsx** — FTS5 search, "Search & Download from Web" when no results
- **RecsPage.tsx** — AI recommendations with per-track download buttons
- **DownloadContext.tsx** — Cross-route state: downloadingIds, completedIds, playlist creation

### 2.6 AI Playlist Creation Flow

```
User clicks "Generate Playlist"
  → POST /recommendations/playlist → DeepSeek → PlaylistPayload
  → Frontend renders preview with per-track status

User clicks "Create Playlist"
  → POST /playlists {name} → get playlist ID
  → For each track:
    → POST /download/search {query}
    → If job.track_id (already in library) → addToPlaylist
    → Else poll job → trigger rescan → search library → addToPlaylist
  → Status: pending → downloading → completed/present/failed
```

---

## 3. Proposed Architecture

### 3.1 Updated Download Pipeline

```
User submits Spotify URL
  │
  ▼
POST /api/download  (enqueue)  [UNCHANGED]
  │
  ▼
goroutine: a.run(job, ctx)
  │
  ├─ [TIER 1] SpotiFLAC (NEW BINARY)
  │   ├─ Command: spotiflac.exe download --output <dir> --service <svc> <url>
  │   ├─ Default service: qobuz (configurable via SPOTIFLAC_SERVICE)
  │   ├─ Requires FFmpeg for qobuz/amazon (checked at download time)
  │   ├─ NEW: Binary exits non-zero on failure (no more "always exits 0")
  │   ├─ NEW: Output format is "[1/N] Track Name - Artist" per track
  │   ├─ NEW: No "Summary:" line — use exit code + per-track line parsing
  │   ├─ On success (exit 0) → finish(succeeded) → rescan
  │   └─ On failure (exit != 0) → fall through
  │
  ├─ [TIER 2] yt-dlp (fallback)  [UNCHANGED COMMAND, UPDATED QUERY PARSING]
  │   ├─ Parse "[1/N] Track Name - Artist" lines from SpotiFLAC log
  │   │   (instead of "Found Track:" / "Failed:" lines)
  │   └─ Rest unchanged
  │
  └─ [TIER 3] spotDL (final fallback)  [UNCHANGED]
```

### 3.2 Key Behavioral Changes

| Aspect | Old Binary | New Binary |
|--------|-----------|------------|
| CLI format | `spotiflac -o <dir> <url>` | `spotiflac download --output <dir> --service <svc> <url>` |
| Default service | Tidal (dead) | Qobuz (working) |
| Exit code | Always 0 | Non-zero on failure |
| Output parsing | `Summary: N Success, M Failed` | Per-track `[1/N] Name - Artist` lines |
| FFmpeg requirement | Not required | Required for qobuz/amazon |
| Format detection | Broken (always FLAC) | Magic bytes (correct MP3 vs FLAC) |
| Metadata embedding | Partial | Full Vorbis (FLAC) + ID3 (MP3) |

---

## 4. Detailed Implementation Steps

### Step 1: Replace the Binary

**Files changed:** None (file system operation only)

1. Copy `tools/spotiflac-cli-fixed-src/spotiflac-cli-fixed.exe` → `tools/spotiflac.exe` (overwrite)
2. Copy `tools/spotiflac-cli-fixed-src/spotiflac-cli-fixed.exe` → `release/tools/spotiflac.exe` (overwrite)
3. Verify: `./spotiflac.exe download --help` shows the new subcommand format

### Step 2: Add SPOTIFLAC_SERVICE Config Field

**Files changed:**
- `backend/internal/config/config.go` — Add `SpotiflacService string` field
- `backend/.env.example` — Add `SPOTIFLAC_SERVICE=qobuz`

**config.go changes:**
```go
// Add to Config struct:
SpotiflacService string

// In Load():
SpotiflacService: env("SPOTIFLAC_SERVICE", "qobuz"),
```

### Step 3: Update downloader.go Config and run() Method

**Files changed:**
- `backend/internal/downloader/downloader.go`

**3a. Add Service field to downloader.Config:**
```go
type Config struct {
    // ... existing fields ...
    SpotiflacService string // "qobuz" (default), "amazon", "tidal"
}
```

**3b. Update main.go wiring to pass the new field:**
```go
dlAPI := downloader.New(downloader.Config{
    // ... existing fields ...
    SpotiflacService: cfg.SpotiflacService,
}, database, doRescan)
```

**3c. Update run() method — Tier 1 SpotiFLAC invocation:**

Replace the current invocation (lines 726-732):
```go
// OLD:
args := []string{"-o", a.cfg.Output}
if a.cfg.FolderFormat != "" {
    args = append(args, "-folder-format", a.cfg.FolderFormat)
}
args = append(args, job.URL)
primaryErr := a.runProcess(job, "spotiflac", a.cfg.Bin, args, ctx)
```

With:
```go
// NEW:
args := []string{"download", "--output", a.cfg.Output}
if a.cfg.SpotiflacService != "" {
    args = append(args, "--service", a.cfg.SpotiflacService)
}
args = append(args, job.URL)
primaryErr := a.runProcess(job, "spotiflac", a.cfg.Bin, args, ctx)
```

**3d. Update failure detection (lines 743-748):**

The old code relied on SpotiFLAC always exiting 0 and parsing a "Summary:" line. The new binary exits non-zero on failure, so:

```go
// OLD (lines 743-748):
if primaryErr == nil {
    if soft, summary := spotiflacReportedFailure(logCopy); soft {
        primaryErr = fmt.Errorf("%s", summary)
    }
}

// NEW:
// The new binary exits non-zero on failure, so primaryErr is already set.
// No need for summary-line parsing. But keep it as a safety net for
// edge cases where the binary exits 0 but reports failures in output.
if primaryErr == nil {
    if soft, summary := spotiflacReportedFailure(logCopy); soft {
        primaryErr = fmt.Errorf("%s", summary)
    }
}
```

Actually, the existing code already handles both cases correctly — if `primaryErr != nil` (non-zero exit), it falls through. If `primaryErr == nil` (exit 0), it checks for soft failures. This works for both old and new binaries. **No change needed here.**

**3e. Update extractFailedTrackQueries for new output format:**

The new binary outputs `[1/N] Track Name - Artist` per track instead of `Found Track: Name - Artist`. Update the regex:

```go
// OLD:
var spotiflacFoundTrackRE = regexp.MustCompile(`Found Track:\s+(.+?)\r?\n`)

// NEW: Match "[1/5] Track Name - Artist" lines
var spotiflacFoundTrackRE = regexp.MustCompile(`\[\d+/\d+\]\s+(.+?)\s*$`)
```

Also update the `spotiflacFailedTrackRE` — the new binary may output failure differently. Since the new binary exits non-zero on failure, the fallback parsing is less critical, but we should still handle per-track failures for the yt-dlp fallback query building:

```go
// OLD:
var spotiflacFailedTrackRE = regexp.MustCompile(`\[\d+/\d+\]\s+Failed:\s+(.+?)\s+\(`)

// NEW: The new binary doesn't print "Failed:" lines — it just exits non-zero.
// The per-track "[1/N] Name - Artist" lines are the success indicators.
// For fallback, we can still extract track names from the success lines
// (tracks that were attempted before failure).
// Keep the regex but it won't match anything in the new output — that's fine.
var spotiflacFailedTrackRE = regexp.MustCompile(`\[\d+/\d+\]\s+Failed:\s+(.+?)\s+\(`)
```

**3f. Remove the `-folder-format` flag handling:**

The new binary doesn't support `-folder-format`. Instead, it uses `--output` only. The folder structure is determined by the binary's internal logic (typically `{artist}/{album}/{title}.{ext}` for Qobuz). Remove the `FolderFormat` from the args:

```go
// REMOVE:
if a.cfg.FolderFormat != "" {
    args = append(args, "-folder-format", a.cfg.FolderFormat)
}
```

Note: We should keep `FolderFormat` in the config structs for backward compatibility (so existing .env files don't break config loading), but simply not use it in the command invocation. Alternatively, we can remove it entirely — but that's a breaking change for anyone who has it set. **Recommendation: Keep the config field but ignore it with a deprecation log message.**

### Step 4: Update Frontend Messaging

**Files changed:**
- `frontend/src/pages/DownloadsPage.tsx`

**4a. Update the "not configured" message (line 124):**
```tsx
// OLD:
<p className="mb-2 flex items-center gap-2 text-yellow-400">
  <AlertCircle size={16} /> SpotiFLAC isn't configured.
</p>
<p>
  Set <code className="text-accent">SPOTIFLAC_BIN</code> (path to
  the spotiflac binary) in <code className="text-accent">backend/.env</code>{" "}
  and restart the server.
</p>

// NEW:
<p className="mb-2 flex items-center gap-2 text-yellow-400">
  <AlertCircle size={16} /> SpotiFLAC isn't configured.
</p>
<p>
  Set <code className="text-accent">SPOTIFLAC_BIN</code> (path to
  the spotiflac binary) and <code className="text-accent">SPOTIFLAC_OUTPUT</code>{" "}
  in <code className="text-accent">backend/.env</code> and restart the server.
  Optional: set <code className="text-accent">SPOTIFLAC_SERVICE</code> to{" "}
  <code className="text-accent">qobuz</code> (default),{" "}
  <code className="text-accent">amazon</code>, or{" "}
  <code className="text-accent">tidal</code>.
</p>
```

**4b. Update the URL mode help text (line 217):**
```tsx
// OLD:
<p className="text-xs text-muted">
  Paste a Spotify <strong>track</strong>, <strong>album</strong>, or{" "}
  <strong>playlist</strong> URL. Files are downloaded as FLAC and
  your library is rescanned automatically when the job finishes.
</p>

// NEW:
<p className="text-xs text-muted">
  Paste a Spotify <strong>track</strong>, <strong>album</strong>, or{" "}
  <strong>playlist</strong> URL. Files are downloaded from Qobuz (or your
  configured service) as FLAC/MP3 with full metadata, and your library is
  rescanned automatically when the job finishes.
</p>
```

**4c. Update the search mode help text (line 221):**
```tsx
// OLD:
<p className="text-xs text-muted">
  Type a song or podcast name. DeepSeek parses the query and yt-dlp
  searches YouTube directly — no Spotify account needed.
</p>

// NEW:
<p className="text-xs text-muted">
  Type a song or podcast name. DeepSeek parses the query and yt-dlp
  searches YouTube directly — no Spotify account needed. This is the
  fallback path when a Spotify URL isn't available.
</p>
```

### Step 5: Update .env.example

**Files changed:**
- `backend/.env.example`

```env
# SpotiFLAC (downloads tracks/albums/playlists from a Spotify URL into your library)
SPOTIFLAC_BIN=
SPOTIFLAC_OUTPUT=
SPOTIFLAC_SERVICE=qobuz    # qobuz (default), amazon, or tidal. FFmpeg required for qobuz/amazon.
```

### Step 6: Update build.ps1 (if needed)

**Files changed:** None expected

The build.ps1 already bundles `spotiflac.exe` from `tools/` (line 84-90). Since we're replacing the file in-place, no build script changes needed. Just verify the new binary gets picked up.

### Step 7: Update status endpoint response (optional enhancement)

**Files changed:**
- `backend/internal/downloader/downloader.go` — `statusResponse` struct and `status()` handler
- `frontend/src/lib/api.ts` — `DownloadStatus` interface
- `frontend/src/pages/DownloadsPage.tsx` — Status display

Add service info to the status response so the frontend can show which service is configured:

```go
// In statusResponse struct:
Service string `json:"service,omitempty"`

// In status():
s := statusResponse{
    Configured: a.configured(),
    Service:    a.cfg.SpotiflacService,
}
if a.cfg.SpotdlBin != "" {
    s.FallbackEnabled = true
}
```

```tsx
// In DownloadStatus interface:
service?: string;

// In DownloadsPage status display, add:
<div>
  <div className="text-xs uppercase tracking-wide text-muted mb-1">Source</div>
  <div className="text-sm">
    {status.service ? (
      <span className="text-green-400">SpotiFLAC ({status.service})</span>
    ) : (
      <span className="text-muted">Not configured</span>
    )}
  </div>
</div>
```

---

## 5. Backend Changes — File-by-File

### 5.1 `backend/internal/config/config.go`

| Change | Type | Lines |
|--------|------|-------|
| Add `SpotiflacService string` to Config struct | Add | After line 24 |
| Add `SpotiflacService: env("SPOTIFLAC_SERVICE", "qobuz")` in Load() | Add | After line 56 |

### 5.2 `backend/internal/downloader/downloader.go`

| Change | Type | Lines |
|--------|------|-------|
| Add `SpotiflacService string` to Config struct | Add | After line 139 |
| Update `spotiflacFoundTrackRE` regex for `[1/N] Name - Artist` format | Modify | Line 58 |
| Update Tier 1 args: `download --output <dir> --service <svc> <url>` | Modify | Lines 726-730 |
| Remove `-folder-format` flag from args | Remove | Lines 727-729 |
| Add deprecation log if FolderFormat is set | Add | After line 725 |
| (Optional) Add Service to statusResponse | Add | Line 299-302 |

### 5.3 `backend/cmd/server/main.go`

| Change | Type | Lines |
|--------|------|-------|
| Pass `SpotiflacService: cfg.SpotiflacService` to downloader.Config | Add | In downloader.New() call, line 224 |

### 5.4 `backend/.env.example`

| Change | Type | Lines |
|--------|------|-------|
| Add `SPOTIFLAC_SERVICE=qobuz` with comment | Add | After line 22 |
| Remove `SPOTIFLAC_FOLDER_FORMAT` (deprecated) | Remove/Comment | Line 22 |

---

## 6. Frontend Changes — File-by-File

### 6.1 `frontend/src/pages/DownloadsPage.tsx`

| Change | Type | Lines |
|--------|------|-------|
| Update "not configured" message to mention SPOTIFLAC_SERVICE | Modify | Lines 122-131 |
| Update URL mode help text to mention Qobuz/service | Modify | Lines 214-219 |
| Update search mode help text | Modify | Lines 220-225 |
| (Optional) Add service display to status section | Add | After line 158 |

### 6.2 `frontend/src/lib/api.ts` (optional)

| Change | Type | Lines |
|--------|------|-------|
| Add `service?: string` to DownloadStatus interface | Add | Line 196 |

---

## 7. Configuration Changes

### 7.1 New Environment Variable

| Variable | Default | Description |
|----------|---------|-------------|
| `SPOTIFLAC_SERVICE` | `qobuz` | Download service: `qobuz` (default, lossless FLAC), `amazon` (requires FFmpeg), `tidal` (requires self-hosted Tidal API instance) |

### 7.2 Deprecated Environment Variable

| Variable | Status | Notes |
|----------|--------|-------|
| `SPOTIFLAC_FOLDER_FORMAT` | Deprecated | No longer used by the new binary. Log a deprecation warning if set. |

### 7.3 Existing Variables (unchanged)

| Variable | Purpose |
|----------|---------|
| `SPOTIFLAC_BIN` | Path to spotiflac.exe |
| `SPOTIFLAC_OUTPUT` | Output directory (falls back to first MEDIA_ROOTS entry) |
| `SPOTDL_BIN` | Path to spotdl.exe (fallback tier 3) |
| `YTDLP_BIN` | Path to yt-dlp.exe (fallback tier 2) |
| `FFMPEG_BIN` | Path to ffmpeg.exe (required for qobuz/amazon in new binary) |
| `FFPROBE_BIN` | Path to ffprobe.exe (auto-detected from FFMPEG_BIN) |

---

## 8. Testing Plan

### 8.1 Binary Verification

```bash
# Verify new binary works
cd tools/
./spotiflac.exe --help
# Expected: Shows "download" and "metadata" subcommands

./spotiflac.exe download --help
# Expected: Shows --output and --service flags

# Test actual download (requires Spotify URL)
./spotiflac.exe download --output /tmp/test --service qobuz "https://open.spotify.com/track/..."
# Expected: Downloads FLAC file, exits 0

# Test with invalid URL
./spotiflac.exe download --output /tmp/test --service qobuz "invalid"
# Expected: Error message, exits non-zero
```

### 8.2 Backend Unit Testing

1. **Config loading:** Verify `SPOTIFLAC_SERVICE` loads correctly with default `qobuz`
2. **Command construction:** Verify the `run()` method builds the correct command: `spotiflac.exe download --output <dir> --service qobuz <url>`
3. **Failure detection:** Verify non-zero exit codes from SpotiFLAC trigger fallback to yt-dlp
4. **Output parsing:** Verify `[1/N] Name - Artist` lines are parsed correctly for yt-dlp fallback queries

### 8.3 Integration Testing

1. **Full pipeline — Spotify URL:**
   - Start backend with new binary configured
   - POST /api/download with a Spotify track URL
   - Verify job succeeds with `tool: "spotiflac"`
   - Verify file appears in output directory
   - Verify scanner picks up the file
   - Verify track appears in library

2. **Full pipeline — fallback:**
   - Configure an invalid SpotiFLAC binary path
   - POST /api/download with a Spotify URL
   - Verify job falls through to yt-dlp
   - Verify `used_fallback: true` and `tool: "spotiflac→ytdlp"`

3. **Search download:**
   - POST /api/download/search with a query
   - Verify library check happens first
   - Verify yt-dlp search download works

4. **AI playlist creation:**
   - Generate a playlist via Discover page
   - Click "Create Playlist"
   - Verify each track is resolved/downloaded/added correctly

### 8.4 Frontend Testing

1. **Downloads page — URL mode:** Paste Spotify URL, verify download starts
2. **Downloads page — Search mode:** Type query, verify download starts
3. **Music page — no results:** Verify "Search & Download from Web" button appears
4. **Search page — no results:** Verify "Search & Download from Web" button appears
5. **Discover page — download item:** Verify per-track download buttons work
6. **Status display:** Verify SpotiFLAC service name shows in status section

### 8.5 Regression Testing

1. Existing yt-dlp fallback still works when SpotiFLAC is not configured
2. Existing spotDL fallback still works
3. Existing search download (via /download/search) still works
4. Existing track upgrade (/api/library/upgrade) still works
5. Existing podcast download still works
6. Existing AI playlist creation still works
7. Scanner still picks up files from all download sources

---

## 9. Risk Assessment

### 9.1 High Risk

| Risk | Impact | Mitigation |
|------|--------|------------|
| New binary requires FFmpeg for qobuz/amazon | Downloads fail if FFmpeg not installed | The binary checks for FFmpeg and returns a clear error. The `runProcess()` error will be caught and trigger yt-dlp fallback. Add a startup check in main.go that warns if FFmpeg is not found when SpotiFLAC is configured. |
| New binary has different output format | `extractFailedTrackQueries` may not parse track names for yt-dlp fallback | Updated regex handles `[1/N] Name - Artist` format. Even if parsing fails, yt-dlp falls back to raw URL/query. |
| Folder structure changes | Files may appear in different directory structure than expected | The new binary uses its own folder structure (typically `{artist}/{album}/`). The scanner walks recursively, so files will still be found. |

### 9.2 Medium Risk

| Risk | Impact | Mitigation |
|------|--------|------------|
| `SPOTIFLAC_FOLDER_FORMAT` is silently ignored | Users who set it may be confused | Log a deprecation warning at startup if the field is set. Update .env.example to mark it deprecated. |
| New binary may have different rate limiting behavior | Qobuz may rate-limit aggressive downloads | The existing semaphore (max 2 concurrent) provides throttling. Monitor logs for rate limit errors. |
| Binary file size may be different | Build.ps1 bundles by filename, so no issue | Verify the new binary is roughly the same size (~11MB) |

### 9.3 Low Risk

| Risk | Impact | Mitigation |
|------|--------|------------|
| Frontend text changes | Minimal — just help text updates | Standard text changes, no logic changes |
| .env.example changes | Minimal — just documentation | Add new variable, deprecate old one |

### 9.4 Rollback Plan

If the new binary causes issues:
1. Restore the old `spotiflac.exe` from git history
2. Revert the config.go and downloader.go changes
3. The yt-dlp fallback will continue to work regardless

---

## 10. Kanban Task Breakdown

### Phase 1: Binary Replacement and Core Backend Changes

| Task | Assignee | Description | Dependencies |
|------|----------|-------------|--------------|
| T1.1 | ops | Replace spotiflac.exe binary in tools/ and release/tools/ | None |
| T1.2 | ops | Add SPOTIFLAC_SERVICE to config.go and .env.example | None |
| T1.3 | ops | Update downloader.go: new CLI invocation, updated regex, pass service config | T1.2 |
| T1.4 | ops | Update main.go: wire SpotiflacService to downloader config | T1.2, T1.3 |

### Phase 2: Frontend Updates

| Task | Assignee | Description | Dependencies |
|------|----------|-------------|--------------|
| T2.1 | ops | Update DownloadsPage.tsx: help text, status display, config instructions | T1.4 |
| T2.2 | ops | (Optional) Update api.ts DownloadStatus interface with service field | T1.4 |

### Phase 3: Testing and Verification

| Task | Assignee | Description | Dependencies |
|------|----------|-------------|--------------|
| T3.1 | ops | Binary verification: test new spotiflac.exe CLI directly | T1.1 |
| T3.2 | ops | Integration test: full download pipeline with new binary | T1.4, T3.1 |
| T3.3 | ops | Regression test: fallback chain, search download, AI playlist | T1.4 |
| T3.4 | ops | Frontend verification: all download UI paths work | T2.1, T3.2 |

### Phase 4: Documentation

| Task | Assignee | Description | Dependencies |
|------|----------|-------------|--------------|
| T4.1 | ops | Update development_context.md files for changed packages | T1.4 |
| T4.2 | ops | Update lexicon-codebase-review skill with new SpotiFLAC details | T1.4 |
| T4.3 | ops | Update help-content.ts with new download descriptions | T2.1 |

---

## Appendix A: New SpotiFLAC Binary CLI Reference

```
NAME:
   spotiflac-cli - Spotify downloader with playlist sync in mind.

USAGE:
   spotiflac-cli [global options] [command [command options]]

COMMANDS:
   download, d  download a song/playlist
   metadata, m  view song metadata
   help, h      Shows a list of commands or help for one command

DOWNLOAD OPTIONS:
   --output string, -o string   set output folder
   --service string, -s string  set service to tidal/amazon/qobuz (FFmpeg is required for amazon and qobuz)

EXIT CODES:
   0 = Success (all tracks downloaded)
   Non-zero = Failure (partial or complete)

OUTPUT FORMAT:
   Per-track: [1/N] Track Name - Artist
   On success: Download completed successfully
   On failure: Error message

DEFAULT SERVICE: qobuz (when --service is not specified)
```

## Appendix B: Migration Guide for Users

1. **Update the binary:** The new `spotiflac.exe` replaces the old one. Download from [rebuild source] or use the one in `tools/spotiflac-cli-fixed-src/`.
2. **Ensure FFmpeg is installed:** The new binary requires FFmpeg for Qobuz and Amazon downloads. FFmpeg is already bundled with the Lexicon installer.
3. **Optional: Set SPOTIFLAC_SERVICE:** Add `SPOTIFLAC_SERVICE=qobuz` to `.env` (this is the default if not set).
4. **Remove SPOTIFLAC_FOLDER_FORMAT:** This variable is no longer used. You can leave it set — it will be ignored with a deprecation warning.
5. **Restart the server:** After making changes, restart Lexicon.

## Appendix C: Key Code Locations Reference

| File | Purpose | Key Lines |
|------|---------|-----------|
| `backend/internal/config/config.go` | Config struct and env loading | Lines 9-98 |
| `backend/internal/downloader/downloader.go` | Download pipeline | Lines 1-1659 |
| `backend/internal/downloader/downloader.go` | SpotiFLAC invocation | Lines 701-756 |
| `backend/internal/downloader/downloader.go` | yt-dlp fallback | Lines 758-858 |
| `backend/internal/downloader/downloader.go` | spotDL fallback | Lines 860-946 |
| `backend/internal/downloader/downloader.go` | runSearch (search downloads) | Lines 948-1117 |
| `backend/internal/downloader/downloader.go` | Output parsing regexes | Lines 31-96 |
| `backend/internal/downloader/downloader.go` | DeepSeek query parsing | Lines 1302-1395 |
| `backend/cmd/server/main.go` | Wiring and config | Lines 208-242 |
| `frontend/src/pages/DownloadsPage.tsx` | Download UI | Lines 1-353 |
| `frontend/src/lib/api.ts` | API client types | Lines 196-218 |
| `frontend/src/contexts/DownloadContext.tsx` | Cross-route state | Lines 1-374 |
| `backend/.env` | Current configuration | Lines 1-38 |
| `backend/.env.example` | Config template | Lines 1-41 |
