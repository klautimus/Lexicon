# tools — Development Context

> **Parent:** [Lexicon root](../development_context.md)

## Purpose

Contains external tools and their source code used by the Lexicon downloader pipeline. These are bundled into the Windows installer by `release/build.ps1`.

## Files

| File/Dir | Purpose | Size |
|----------|---------|------|
| `spotiflac.exe` | Prebuilt SpotiFLAC binary — primary download tool | ~11.8MB |
| `spotiflac-src/` | Wails desktop app source for SpotiFLAC | — |

## spotiflac.exe

Primary download tool in the 3-tier pipeline. Downloads audio from Spotify URLs.

**Usage:**
```bash
spotiflac -o <output_dir> [-folder-format <format>] <spotify_url>
```

**Critical behavior:** SpotiFLAC always exits with code 0, even when every track fails. The downloader detects failures by parsing the summary line in stdout:
```
Summary: X Success, Y Failed
```
If `Success == 0 && Failed > 0`, the downloader considers it a soft failure and falls back to yt-dlp.

**Output patterns:**
- `Found Track: Title - Artist` — single track metadata (cleanest source for fallback queries)
- `[N/M] Failed: Title - Artist (error_text)` — per-track failure with reason

## spotiflac-src/

Source code for SpotiFLAC built with [Wails](https://wails.io/) (Go + Web frontend desktop app framework). This is a **separate application** from the main Lexicon backend — it has its own Go code, frontend, and build pipeline.

The source is included for reference and potential customization. The prebuilt `spotiflac.exe` in `tools/` is the actual binary used by Lexicon.

**Files within spotiflac-src/ (from observed structure):**
- `backend/ffmpeg.go` — ffmpeg integration for audio processing
- Standard Wails project structure (frontend/, go.mod, wails.json etc.)

## Relationship to Lexicon

```
Lexicon backend (downloader.go)
  │
  ├── calls spotiflac.exe (tools/spotiflac.exe)
  │     └── built from spotiflac-src/ (separate Wails project)
  │
  ├── calls yt-dlp.exe (bundled in release/tools/)
  │
  └── calls spotdl.exe (bundled in release/tools/)
```

## Known Issues

1. **SpotiFLAC exit code misleading** — always 0, requiring log parsing for failure detection
2. **No version tracking** — manual process to update spotiflac.exe
3. **Source/build separation** — spotiflac-src is source but not part of Lexicon build pipeline; the prebuilt .exe is committed directly
4. **Large binary** — 11.8MB, contributes significantly to installer size

## Working Here

- Updating SpotiFLAC: build from spotiflac-src, replace spotiflac.exe, test download pipeline
- Customizing SpotiFLAC behavior: modify spotiflac-src source, rebuild Wails app
- Adding a new tool: add .exe to tools/, update downloader.go to call it, update build.ps1 and lexicon.iss
