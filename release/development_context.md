# release — Development Context

> **Parent:** [Lexicon root](../development_context.md)
> **Last updated:** 2026-05-20

## Purpose

Build and distribution pipeline. Produces a Windows-native InnoSetup installer containing the Go backend, embedded React frontend, and all external tool binaries.

## Files

| File | Purpose |
|------|---------|
| `build.ps1` | PowerShell build script — compiles Go binary, builds frontend, bundles tools |
| `lexicon.iss` | InnoSetup 6+ installer script |
| `lexicon-launch.ps1` | Post-install auto-launch script (starts server, polls, opens browser) |
| `lexicon.exe` | Compiled Go binary (output of build.ps1) |
| `LexiconSetup.exe` | InnoSetup installer (distributable) |
| `lexicon.ico` | Windows icon (multi-resolution, 16-256px) |
| `gen_icon.py` | Python script to generate .ico and PNG icons from SVG |
| `tools/` | Bundled external .exe files for installer |

## Build Process (`build.ps1`)

1. Builds frontend: `cd frontend && npm run build` (with npm bug workaround for rollup native modules)
2. Copies `frontend/dist/` into `backend/cmd/server/dist/` for `//go:embed` — errors if dist/ doesn't exist
3. Builds Go backend: `go build -ldflags "-s -w" -o release/lexicon.exe ./cmd/server`
4. Downloads/bundles external tools into `release/tools/`:
   - `spotiflac.exe` (from `tools/spotiflac.exe`)
   - `poddl.exe` (from `tools/poddl.exe`)
   - `yt-dlp.exe` (auto-downloaded from GitHub)
   - `spotdl.exe` (auto-downloaded from GitHub)
   - `ffmpeg.exe` + `ffprobe.exe` (from PATH)
   - `ngrok.exe` (from PATH, optional)
5. Generates icon files using `gen_icon.py` — finds Python via `python3` → `python` → `py` fallback
6. Runs InnoSetup compiler: `iscc lexicon.iss`

## InnoSetup Script (`lexicon.iss`)

- Custom wizard pages: DeepSeek API Key, Media Folders, Spotify Integration, Port Configuration
- Creates `{app}\tools\` directory with all bundled .exe files
- Creates `{app}\data\` for SQLite database (everyone-modify permissions)
- Creates `{app}\podcasts\` for podcast downloads
- Writes `.env` file from wizard page inputs
- Auto-launches via `lexicon-launch.ps1` (separate script, not inline PowerShell)
- `[UninstallRun]` stops the lexicon process before uninstall
- Uninstaller has try/except error handling for file-in-use scenarios
- Validates API key format (starts with "sk-", >20 chars)
- Validates port ranges (1-65535)
- Sets `PODDL_BIN` and `PODCAST_DIR` in generated `.env`
- Uses `lexicon.ico` as SetupIconFile
- PWA manifest with 192px/512px PNG icons

## CRITICAL CONSTRAINTS

**Windows-only distribution.** All external tools must be Windows .exe files bundled in the installer. No Python, no WSL, no Docker.

**Line endings:** `build.ps1` and `lexicon.iss` MUST have CRLF line endings only. Mixed CRLF/LF causes PowerShell parse errors.

## Working Here

- Adding a bundled tool: download it in build.ps1, add to lexicon.iss `[Files]`
- Changing installer behavior: edit lexicon.iss
- Icon updates: edit `frontend/public/icon.svg`, run `gen_icon.py`, rebuild
- Auto-launch logic: edit `lexicon-launch.ps1` (not inline in .iss anymore)
