# config — Development Context

> **Parent:** [backend](../development_context.md)
| **File:** `backend/internal/config/config.go` (84 LOC)
> **Last updated:** 2026-05-17

## Purpose

Loads all configuration from environment variables (`.env` file loaded by `godotenv` in main.go). Provides a single `Config` struct consumed by `main.go` to construct all sub-APIs.

## Config Struct

```go
type Config struct {
    Port               string  // default "8787"
    DBPath             string  // default "./data/lexicon.db"
    MediaRoots         string  // semicolon-separated on Windows
    Timezone           string  // default "local" (for analytics heatmap)
    DeepSeekAPIKey     string
    DeepSeekModel      string  // default "deepseek-v4-flash"
    DeepSeekThinking   string  // default "medium"
    DeepSeekBaseURL    string  // default "https://api.deepseek.com"
    SpotifyClientID     string
    SpotifyClientSecret string  // for spotDL fallback rate limit
    SpotifyRedirectURI  string
    SpotifyFrontendURL  string
    SpotiflacBin       string
    SpotiflacOutput    string
    SpotiflacFolderFmt string
    SpotdlBin          string
    SpotdlFormat       string  // default "mp3"
    SpotdlAudio        string  // default "piped,youtube,soundcloud,bandcamp"
    YtdlpBin           string
    YtdlpFormat        string  // default "mp3"
    FfmpegBin          string
    DownloadConcurrency int    // default 2
    WebSearchEnabled   bool   // default true
}
```

## Key Function

```go
func Load() Config
```

Reads `os.Getenv()` for each field, falling back to defaults. Uses helper `env(key, def string) string`.

## Working Here

- Adding a new config field: add to struct, add env read in `Load()`, update `.env.example`.
- Changing defaults: edit the second arg in the `env()` call.
