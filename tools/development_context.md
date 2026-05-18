# tools — Development Context

> **Parent:** [Lexicon root](../development_context.md)
> **Last updated:** 2026-05-17

## Purpose

Contains external tools and their source code used by the Lexicon downloader pipeline. These are bundled into the Windows installer by `release/build.ps1`.

## Files

| File/Dir | Purpose | Size |
|----------|---------|------|
| `spotiflac.exe` | Prebuilt SpotiFLAC binary — primary download tool | ~11.3MB |
| `poddl.exe` | Prebuilt poddl binary — podcast episode downloader | ~1.3MB |
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
- `Found Track: Title - Artist` — single track metadata
- `[N/M] Failed: Title - Artist (error_text)` — per-track failure

## poddl.exe

Podcast episode downloader (https://github.com/freshe/poddl). Downloads episodes from RSS feeds.

**Usage:**
```bash
# Download all episodes from feed
poddl.exe "https://example.com/feed.xml" -o C:\podcasts

# Download latest episode only
poddl.exe "https://example.com/feed.xml" -o C:\podcasts -r -t 1

# Download specific episode (direct audio URL)
poddl.exe "https://example.com/episode.mp3" -o C:\podcasts
```

**CRITICAL:** Argument order is `poddl <url> -o <output> [flags]`. The `-o` flag comes AFTER the URL.

**Exit codes:** 0 = success, 255 = error (check stderr for details)

## spotiflac-src/

Source code for SpotiFLAC built with Wails (Go + Web frontend desktop app framework). This is a **separate application** from the main Lexicon backend.

## Relationship to Lexicon

```
Lexicon backend
  ├── downloader.go → calls spotiflac.exe → built from spotiflac-src/
  ├── downloader.go → calls yt-dlp.exe (bundled in release/tools/)
  ├── downloader.go → calls spotdl.exe (bundled in release/tools/)
  └── podcaster.go → calls poddl.exe
```

## Known Issues

1. **SpotiFLAC exit code misleading** — always 0, requiring log parsing for failure detection
2. **poddl argument order** — URL must come before `-o` flag (unlike other tools)
3. **No version tracking** — manual process to update binaries
4. **Large binaries** — spotiflac.exe is 11.3MB, contributes significantly to installer size

## Working Here

- Updating a tool: build from source or download new binary, replace .exe, test pipeline
- Adding a new tool: add .exe to tools/, update relevant Go code, update build.ps1 and lexicon.iss
