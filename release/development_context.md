# release — Development Context

> **Parent:** [Lexicon root](../development_context.md)

## Purpose

Build and distribution pipeline. Produces a Windows-native InnoSetup installer containing the Go backend, embedded React frontend, and all external tool binaries.

## Files

| File | Purpose |
|------|---------|
| `build.ps1` | PowerShell build script — compiles Go binary, builds frontend, bundles tools |
| `lexicon.iss` | InnoSetup 6+ installer script |
| `lexicon.exe` | Compiled Go binary (output of build.ps1) |
| `LexiconSetup.exe` | InnoSetup installer (distributable) |
| `LexiconSetup.zip` | Zipped installer |
| `data/` | Runtime data directory (created by installer) |
| `tools/` | Bundled external .exe files for installer |

## Build Process (`build.ps1`)

1. Builds Go backend: `go build -o release/lexicon.exe ./cmd/server`
2. Builds frontend: `cd frontend && npm run build`
3. Frontend `dist/` is embedded in Go binary via `//go:embed`
4. Downloads/bundles external tools into `release/tools/`:
   - `spotiflac.exe`
   - `yt-dlp.exe`
   - `spotdl.exe`
   - `ffmpeg.exe` + `ffprobe.exe`
   - `ngrok.exe` (optional)
5. Runs InnoSetup compiler: `iscc lexicon.iss`

## InnoSetup Script (`lexicon.iss`)

- Creates `{app}\tools\` directory with all bundled .exe files
- Creates `{app}\data\` for SQLite database
- Adds start menu shortcut
- Sets file associations (optional)
- Uninstaller included

## CRITICAL CONSTRAINTS

**Windows-only distribution.** All external tools must be Windows .exe files bundled in the installer. No Python, no WSL, no Docker.

Python-based tools (SearXNG, Trafilatura) would require bundling embedded Python:
- `{app}\tools\python\` — embedded Python distribution
- `pip install searxng trafilatura` during install
- Go backend calls SearXNG via HTTP, Trafilatura via Python subprocess

## Known Issues

1. **Build.ps1 must run on Windows** — Go cross-compilation targets Windows, InnoSetup is Windows-only
2. **Tool binaries must be manually updated** — no automated version tracking for spotiflac/yt-dlp/spotdl
3. **No CI/CD** — builds are manual
4. **Installer size** — spotiflac.exe alone is 11.8MB, full installer can be large

## Working Here

- Adding a bundled tool: download it in build.ps1, add to lexicon.iss
- Changing installer behavior: edit lexicon.iss
- Adding Python dependency: bundle embedded Python, install packages, test subprocess paths
