# downloader — Development Context

> **Parent:** [backend](../development_context.md)
> **File:** `backend/internal/downloader/downloader.go` (765 LOC) — 🆕 NEW in v2 (largest module)

## Purpose

Downloads audio from Spotify URLs or free-text search queries using a 3-tier fallback pipeline:
**SpotiFLAC → yt-dlp → spotDL**

Manages an in-memory job queue with log streaming, cancellation, and automatic library rescan on completion.

## Configuration

```go
type Config struct {
    Bin, Output, FolderFormat      string  // SpotiFLAC
    SpotdlBin, SpotdlFormat, SpotdlAudio string  // spotDL fallback
    YtdlpBin, YtdlpFormat          string  // yt-dlp fallback
    FfmpegBin                      string  // ffmpeg for audio extraction
    SpotifyClientID, SpotifyClientSecret string // for spotDL rate limit
    DeepSeekAPIKey, DeepSeekModel, DeepSeekThinking, DeepSeekBaseURL string
}
```

## Routes

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/download/status` | Configuration status (what's configured) |
| `POST` | `/api/download` | Enqueue Spotify URL download |
| `POST` | `/api/download/search` | Enqueue free-text search download |
| `GET` | `/api/download/jobs` | List all jobs (without logs) |
| `GET` | `/api/download/jobs/{id}` | Get single job with full log |
| `POST` | `/api/download/jobs/{id}/cancel` | Kill running process |

## Job Model

```go
type Job struct {
    ID           string   // UUID
    URL          string   // Spotify URL or search query
    Output       string   // output directory
    Status       Status   // queued → running → succeeded/failed/cancelled
    StartedAt    int64    // Unix timestamp
    FinishedAt   int64    // set on completion
    Error        string   // error message if failed
    Tool         string   // "spotiflac", "spotiflac→ytdlp", "spotiflac→ytdlp→spotdl"
    UsedFallback bool     // true if primary tool failed
    IsSearch     bool     // true for text search (not Spotify URL)
    TrackID      int64    // set when search resolves to existing library track
    Log          []string // max 500 lines per job
    cmd          *exec.Cmd // hidden from JSON
}
```

Jobs stored in-memory (`map[string]*Job`), max 50. Oldest evicted on overflow. **Lost on server restart.**

## Download Pipeline — `run()` method (line 451)

### Tier 1: SpotiFLAC (primary)
```bash
spotiflac -o <output> [-folder-format <fmt>] <spotify_url>
```
- **Critical:** SpotiFLAC always exits with code 0, even on failure!
- Success/failure detected by parsing the summary line: `Summary: X Success, Y Failed`
- Regex: `Summary:\s*(\d+)\s*Success,\s*(\d+)\s*Failed`
- If `Success == 0 && Failed > 0` → soft failure → advance to Tier 2

### Tier 2: yt-dlp (fallback)
```bash
yt-dlp "ytsearch1:<query>" --extract-audio --audio-format mp3 --audio-quality 0 --no-playlist --add-metadata --embed-thumbnail -o "<output>/%(artist)s - %(title)s.%(ext)s"
```
- Query parsed from SpotiFLAC log: "Found Track:" or "Failed:" lines
- Falls back to raw Spotify URL if parsing fails
- If successful → rescan → done

### Tier 3: spotDL (final fallback)
```bash
spotdl download <targets> --output <template> --format mp3 --threads 2 --client-id ... --client-secret ... --audio piped,youtube,soundcloud,bandcamp
```
- Uses track queries parsed from SpotiFLAC if available
- Otherwise passes original Spotify URL
- SpotDL downloads from YouTube Music (no Spotify Premium needed)
- Spotify client credentials used to avoid shared rate limits

## Search Mode — `runSearch()` (line 636)

Used when user submits free-text query (not a Spotify URL):
1. Optionally parses query via DeepSeek into structured metadata
2. Searches YouTube: `ytsearch1:<query>` with `--match-filter "duration < 600"`
3. DeepSeek prompt optimizes for YouTube audio search (appends "audio" or "- Topic")

## DeepSeek Query Parsing (line 765)

Parses user's free-text query into:
```json
{"type": "music|podcast", "artist": "...", "title": "...", "album": "...", "search_query": "optimized query"}
```
- Temperature 0.3 for deterministic output
- 30s timeout
- Gracefully degrades: if parsing fails, uses raw query

## Library Dedup — `findLibraryTrack()` (line 263)

Three strategies to avoid re-downloading existing tracks:
1. **Exact match:** title + artist (LOWER comparison)
2. **Prefix match:** title startsWith on "Artist - Title" format
3. **FTS5 match:** cleaned query tokenized and searched
4. **LIKE fallback:** wildcard match on title or artist

If found, creates a "succeeded" job immediately with the existing track_id.

## Process Management

```go
func runProcess(job *Job, logPrefix, bin string, args []string, _ string) error
```
- `cmd.StdoutPipe()` + `cmd.StderrPipe()` → streamed to job log
- `consumeOutput()` reads line-by-line with 1MB buffer, prepends tool prefix
- `appendLog()` caps at 500 lines (oldest dropped)
- Cancel: `cmd.Process.Kill()`

## Failure Detection for SpotiFLAC

Critical regex patterns:
```go
var spotiflacSummaryRE = regexp.MustCompile(`Summary:\s*(\d+)\s*Success,\s*(\d+)\s*Failed`)
var spotiflacFoundTrackRE = regexp.MustCompile(`Found Track:\s+(.+?)\r?\n`)
var spotiflacFailedTrackRE = regexp.MustCompile(`\[\d+/\d+\]\s+Failed:\s+(.+?)\s+\(`)
```

`extractFailedTrackQueries()` deduplicates track queries from SpotiFLAC output for use in fallback tiers.

## Known Issues

1. **Jobs lost on restart** — in-memory only, no persistence
2. **No rate limiting** — rapid sequential downloads fire without throttling
3. **SpotiFLAC exit code misleading** — always 0, requires log parsing
4. **No MIME verification** — downloaded files indexed by extension, not verified
5. **No cleanup on cancel** — partial downloads remain on disk
6. **yt-dlp format assumption** — assumes mp3, but actual format may differ

## Working Here

- Changing download pipeline: edit `run()` method (3-tier flow)
- Adding a new tool: add as Tier 4 in the pipeline
- Adding job persistence: serialize jobs to DB
- Adding rate limiting: add delays between downloads
- Changing SpotiFLAC parsing: update the regex patterns
