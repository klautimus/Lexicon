# config — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/config/config.go` (58 LOC)

## Purpose

Loads all configuration from environment variables (`.env` file loaded by `godotenv` in main.go). Provides a single `Config` struct consumed by `main.go` to construct all sub-APIs.

## Config Struct

```go
type Config struct {
    Port               string  // default "8787"
    DBPath             string  // default "./data/lexicon.db"
    MediaRoots         string  // semicolon-separated on Windows
    DeepSeekAPIKey     string
    DeepSeekModel      string  // default "deepseek-v4-flash"
    DeepSeekThinking   string  // default "medium"
    DeepSeekBaseURL    string
    SpotifyClientID    string
    SpotifyClientSecret string  // NEW v2
    SpotifyRedirectURI string
    SpotifyFrontendURL string
    SpotiflacBin       string  // NEW v2
    SpotiflacOutput    string  // NEW v2
    SpotiflacFolderFmt string  // NEW v2
    SpotdlBin          string  // NEW v2
    SpotdlFormat       string  // NEW v2 (default "mp3")
    SpotdlAudio        string  // NEW v2
    YtdlpBin           string  // NEW v2
    YtdlpFormat        string  // NEW v2 (default "mp3")
    FfmpegBin          string  // NEW v2
}
```

## Key Function

```go
func Load() Config
```

Reads `os.Getenv()` for each field, falling back to defaults. Uses helper `env(key, def string) string`.

## Known Issues

- No validation of required fields at load time — missing `DEEPSEEK_API_KEY` causes runtime 400 errors.
- `MediaRoots` split by `;` — Windows-specific, not portable to Unix.
- No config file reload — server restart required for any env var changes.

## Working Here

- Adding a new config field: add to struct, add env read in `Load()`, update `.env.example`.
- Changing defaults: edit the second arg in the `env()` call.
