# Lexicon Audio Playback Failure — Root Cause Analysis & Fix Plan

**Date:** 2026-05-17
**Issue:** ~12% of yt-dlp downloaded files won't play. Player shows "Playback failed — file may be corrupted or inaccessible". Playback stops when corrupt file is encountered in a playlist.

---

## 1. Root Cause Analysis

### 1.1 The Smoking Gun: `isValidAudioFile()` Is Dead Code

The function `isValidAudioFile()` at `downloader.go:881` is **defined but never called anywhere in the codebase**. The `validateOutput()` function that was supposed to invoke it is a **no-op** (line 899-903):

```go
func (a *API) validateOutput(job *Job) {}
```

This means **there is zero post-download validation**. Every file that yt-dlp writes to disk is immediately rescanned and added to the library, regardless of whether it's actually a valid audio file.

### 1.2 Why yt-dlp Produces Unplayable Files (~12% of the time)

The yt-dlp command used in both `run()` (Tier 2) and `runSearch()`:

```
ytsearch1:<query> --extract-alp --audio-format mp3 --audio-quality 0
--no-playlist --add-metadata --embed-thumbnail -o "<path>/%(artist)s - %(title)s.%(ext)s"
```

The `--extract-audio --audio-format mp3` flags tell yt-dlp to:
1. Download the best available audio stream (usually webm/opus or m4a/aac from YouTube)
2. Run **ffmpeg as a post-processor** to convert it to mp3

**When ffmpeg conversion fails** (missing ffmpeg, codec incompatibility, disk space, corrupted source), yt-dlp can:
- Exit with code 0 (success) but leave the **unconverted** file (webm/opus content with `.mp3` extension)
- Leave a **partially converted** file (truncated mp3)
- Leave a file with **corrupt headers** (failed metadata embedding)

The browser's HTML5 audio element then receives a file whose content doesn't match its extension/MIME type, and fires an `error` event.

### 1.3 Why Playback Stops in Playlists

In `PlayerContext.tsx`, when a local file fails to play:

```typescript
// onError handler (line 94-106):
toast.error("Playback failed — file may be corrupted or inaccessible");
setState((s) => ({ ...s, source: null, error: msg, playing: false }));

// OR .catch() on a.play() (line 233-240):
toast.error("Failed to play track — file may be missing or corrupted");
setState((s) => ({ ...s, source: null, error: msg, playing: false }));
```

In both cases, `goNext()` is **never called**. The player just stops. The user must manually skip.

### 1.4 Contributing Factors

| Factor | Impact |
|--------|--------|
| No `--abort-on-error` in yt-dlp | Continues on errors, may produce partial files |
| No `--retries` / `--fragment-retries` | Network hiccups cause incomplete downloads |
| No ffmpeg post-processor error handling | Silent conversion failures |
| No `--postprocessor-args` for ffmpeg | ffmpeg failures don't propagate to yt-dlp exit code |
| `validateOutput()` is a no-op | No validation gate between download and library indexing |
| Scanner trusts extension-based MIME | Corrupt files get indexed with wrong MIME type |
| Player doesn't auto-skip on error | One bad file kills the entire playlist |

---

## 2. Confirmation Steps (Do First)

Before implementing the fix, confirm the root cause:

### Step 1: Check ffmpeg availability
```powershell
# Is ffmpeg in the bundled tools?
dir C:\Users\kevin\CascadeProjects\lexicon\release\tools\ffmpeg.exe

# Is it on PATH?
where ffmpeg
```

### Step 2: Inspect a failed file
Take one of the ~12% files that won't play and run:
```powershell
# Check actual file content vs extension
ffprobe -v error -show_format -show_streams "C:\path\to\failed_file.mp3"

# Check file size (corrupt files are often tiny)
dir "C:\path\to\failed_file.mp3"

# Check actual format
file "C:\path\to\failed_file.mp3"  # (if file.exe is available)
```

**Expected finding:** The "mp3" file is actually webm/opus or m4a content, or has 0 bytes, or ffprobe reports "Invalid data found."

### Step 3: Check yt-dlp logs
Look at the download job logs in the Downloads page for the failed files. Look for:
- `[ERROR] ffmpeg post-processing failed`
- `Conversion failed!`
- Any warnings about codec or format

---

## 3. Fix Plan — 3 Phases

### Phase 1: Post-Download Validation + Auto-Retry (Backend — downloader.go)

**Goal:** After yt-dlp reports success, verify the file is actually playable. If not, retry with a different YouTube URL or format.

#### 3.1.1 Add `verifyDownloadedFile()` function

```go
// verifyDownloadedFile checks that a downloaded file is actually playable audio.
// Uses ffprobe (bundled with ffmpeg) to validate the file container and streams.
func verifyDownloadedFile(path string, ffprobeBin string) error {
    // Check file exists and has reasonable size
    info, err := os.Stat(path)
    if err != nil {
        return fmt.Errorf("file not found: %w", err)
    }
    if info.Size() < 10240 { // < 10KB is suspicious for a music file
        return fmt.Errorf("file too small (%d bytes)", info.Size())
    }

    // If ffprobe is available, do deep validation
    if ffprobeBin != "" {
        ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer cancel()
        cmd := exec.CommandContext(ctx, ffprobeBin,
            "-v", "error",
            "-show_entries", "format=duration",
            "-show_entries", "stream=codec_type",
            "-of", "default=noprint_wrappers=1",
            path,
        )
        out, err := cmd.CombinedOutput()
        if err != nil {
            return fmt.Errorf("ffprobe failed: %s", string(out))
        }
        // Verify it has an audio stream
        output := string(out)
        if !strings.Contains(output, "codec_type=audio") {
            return fmt.Errorf("no audio stream found")
        }
        // Verify duration is reasonable (> 5 seconds)
        // Parse duration from output...
    }
    return nil
}
```

#### 3.1.2 Add `findDownloadedFile()` helper

After yt-dlp runs, we need to find the actual output file. The output template uses `%(artist)s - %(title)s.%(ext)s`, so the filename depends on yt-dlp's metadata extraction. Search the output directory for recently created files:

```go
func findDownloadedFile(outputDir string, startedAt time.Time) string {
    // Find files in outputDir modified within the last 5 minutes
    // that match audio extensions
    // Return the most recently modified match
}
```

#### 3.1.3 Integrate validation into `runSearch()` and `run()`

After yt-dlp succeeds in `runSearch()` (after line 830):
```go
// After: a.finish(job, StatusSucceeded, "")
// Before finishing, verify the file
downloadedFile := findDownloadedFile(a.cfg.Output, time.Unix(job.StartedAt, 0))
if downloadedFile != "" {
    if err := verifyDownloadedFile(downloadedFile, a.cfg.FfmpegBin); err != nil {
        log.Printf("[downloader] validation failed for %s: %v", downloadedFile, err)
        // Retry with different approach
        a.appendLog(job, fmt.Sprintf("[verify] file invalid: %s", err.Error()))
        a.appendLog(job, "[verify] retrying with different source...")
        
        // Retry: use ytsearch2: (second YouTube result) with m4a format
        retryQuery := "ytsearch2:" + searchQuery
        retryArgs := buildRetryArgs(retryQuery, ytdlpFormat, outputDir, a.cfg.FfmpegBin)
        retryErr := a.runProcess(job, "ytdlp-retry", a.cfg.YtdlpBin, retryArgs, "")
        
        if retryErr != nil {
            a.finish(job, StatusFailed, fmt.Sprintf("download invalid and retry failed: %s", retryErr.Error()))
            return
        }
        
        // Verify the retry
        retryFile := findDownloadedFile(outputDir, time.Now().Add(-2*time.Minute))
        if retryFile != "" {
            if err := verifyDownloadedFile(retryFile, a.cfg.FfmpegBin); err != nil {
                a.finish(job, StatusFailed, fmt.Sprintf("download invalid, retry also invalid: %s", err.Error()))
                return
            }
        }
    }
}
```

#### 3.1.4 Add config field for ffprobe path

In `config.go`, add:
```go
FfprobeBin string  // default: "" (auto-detect from FfmpegBin directory)
```

In `Load()`, auto-detect:
```go
if cfg.FfprobeBin == "" && cfg.FfmpegBin != "" {
    cfg.FfprobeBin = strings.Replace(cfg.FfmpegBin, "ffmpeg.exe", "ffprobe.exe", 1)
}
```

---

### Phase 2: Harden yt-dlp Flags (Backend — downloader.go)

**Goal:** Make yt-dlp failures fatal and reduce the chance of bad downloads.

#### 3.2.1 Update yt-dlp arguments in both `run()` and `runSearch()`

Current flags:
```go
ytdlpArgs := []string{
    ytdlpSearch,
    "--extract-audio",
    "--audio-format", ytdlpFormat,
    "--audio-quality", "0",
    "--no-playlist",
    "--add-metadata",
    "--embed-thumbnail",
    "--newline",
    "--no-warnings",
    "-o", outputDir + "/%(artist)s - %(title)s.%(ext)s",
}
```

Replace with hardened flags:
```go
ytdlpArgs := []string{
    ytdlpSearch,
    "--extract-audio",
    "--audio-format", ytdlpFormat,
    "--audio-quality", "0",
    "--no-playlist",
    "--add-metadata",
    "--embed-thumbnail",
    "--newline",
    "--no-warnings",
    // NEW: Make failures fatal
    "--abort-on-error",
    "--retries", "3",
    "--fragment-retries", "10",
    // NEW: Ensure ffmpeg post-processor errors are fatal
    "--postprocessor-args", "ffmpeg:-abort_on_error 1 -v warning",
    // NEW: Prefer m4a container if mp3 conversion keeps failing
    // (m4a is more reliably produced by yt-dlp's ffmpeg)
    "-o", outputDir + "/%(artist)s - %(title)s.%(ext)s",
}
```

#### 3.2.2 Add `--extractor-args` for YouTube resilience

```go
"--extractor-args", "youtube:player_client=android",
```

This uses YouTube's Android client which is less likely to trigger rate limiting and format issues.

---

### Phase 3: Player Resilience (Frontend — PlayerContext.tsx)

**Goal:** When a track fails to play, auto-skip to the next track instead of stopping the entire playlist.

#### 3.3.1 Add auto-skip on error

In the `onError` handler (line 94-106), add auto-skip:

```typescript
const onError = () => {
  const a = audioRef.current;
  const err = a?.error;
  const msg = err
    ? `Audio error (code ${err.code}): ${err.message || "unknown"}`
    : "Audio playback failed";
  console.error("[player]", msg);
  toast.error("Playback failed — file may be corrupted or inaccessible");
  sourceRef.current = null;
  currentRef.current = null;
  playSecondsRef.current = 0;
  setState((s) => ({ ...s, source: null, error: msg, playing: false }));

  // NEW: Auto-skip to next track after a brief delay
  setTimeout(() => {
    goNext();
  }, 1500);
};
```

#### 3.3.2 Add auto-skip on play() rejection

In `loadAndPlay()` (line 233-240), add auto-skip:

```typescript
.catch((e: any) => {
  const msg = e?.message || "Audio playback failed";
  console.error("[player] play failed:", msg);
  toast.error("Failed to play track — file may be missing or corrupted");
  sourceRef.current = null;
  currentRef.current = null;
  setState((s) => ({ ...s, source: null, error: msg, playing: false }));

  // NEW: Auto-skip to next track
  setTimeout(() => {
    goNext();
  }, 1500);
});
```

#### 3.3.3 Track consecutive failures to prevent infinite skip loops

Add a ref to track consecutive playback failures:

```typescript
const consecutiveErrorsRef = useRef<number>(0);
const MAX_CONSECUTIVE_ERRORS = 5;

// In onError / .catch():
consecutiveErrorsRef.current++;
if (consecutiveErrorsRef.current >= MAX_CONSECUTIVE_ERRORS) {
  toast.error("Multiple tracks failed to play — stopping playback");
  consecutiveErrorsRef.current = 0;
  return; // Don't auto-skip, let the user investigate
}

// In loadAndPlay() success path:
consecutiveErrorsRef.current = 0;
```

---

### Phase 4: Scanner-Side Validation (Backend — scanner.go) [Optional but Recommended]

**Goal:** Catch corrupt files that were downloaded before the validation was added (existing library).

#### 3.4.1 Add optional ffprobe validation during scan

In `indexFile()`, after extracting metadata with `tag.ReadFrom()`:

```go
// After tag extraction, optionally validate with ffprobe
if ffprobeBin != "" {
    if err := quickValidate(path, ffprobeBin); err != nil {
        log.Printf("[scanner] skipping corrupt file %s: %v", path, err)
        return nil // Skip this file — don't index it
    }
}
```

This ensures that when the library is rescanned, previously-downloaded corrupt files get flagged.

---

## 4. Files Changed Summary

| File | Change | Phase |
|------|--------|-------|
| `backend/internal/downloader/downloader.go` | Add `verifyDownloadedFile()`, `findDownloadedFile()`, integrate validation + retry into `run()` and `runSearch()` | 1 |
| `backend/internal/downloader/downloader.go` | Harden yt-dlp flags (`--abort-on-error`, `--retries`, `--postprocessor-args`, `--extractor-args`) | 2 |
| `backend/internal/config/config.go` | Add `FfprobeBin` field with auto-detection from `FfmpegBin` | 1 |
| `backend/internal/config/config.go` | Add `FfprobeBin` to `.env.example` | 1 |
| `backend/cmd/server/main.go` | Pass `FfprobeBin` to downloader Config | 1 |
| `frontend/src/player/PlayerContext.tsx` | Add auto-skip on playback error + consecutive error tracking | 3 |
| `backend/internal/scanner/scanner.go` | Add optional ffprobe validation during scan | 4 |

---

## 5. Rollout Order

1. **Phase 2 first** (hardened yt-dlp flags) — lowest risk, immediately reduces bad downloads
2. **Phase 1 next** (validation + retry) — catches remaining failures, auto-recovers
3. **Phase 3** (player resilience) — UX improvement, prevents playlist stoppage
4. **Phase 4** (scanner validation) — cleanup of existing corrupt files

---

## 6. Success Metrics

After implementation:
- **Target:** < 1% unplayable files (down from ~12%)
- **Auto-retry success rate:** Track how often retry produces a valid file
- **Playlist continuity:** Verify playback continues past corrupt files
- **Log visibility:** All validation failures and retries are logged with `[verify]` prefix

---

## 7. Alternative: Nuclear Option

If the above doesn't reduce failures enough, consider switching the yt-dlp download strategy:

**Instead of:** Download webm → ffmpeg convert to mp3
**Use:** Download best m4a directly (no conversion needed)

```go
// Change from:
"--extract-audio", "--audio-format", "mp3",
// To:
"--format", "bestaudio[ext=m4a]/bestaudio",
// Remove: --extract-audio, --audio-format
```

This avoids the ffmpeg conversion step entirely. m4a/aac is natively playable in all modern browsers. The trade-off is slightly larger file sizes vs mp3, but zero conversion failures.
