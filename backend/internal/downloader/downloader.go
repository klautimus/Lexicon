// Package downloader integrates SpotiFLAC. Users paste a Spotify URL,
// the downloader runs the spotiflac CLI, captures output, then triggers
// a library rescan so the new files appear in Lexicon.
package downloader

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Spotiflac exits with status 0 even when every track failed. We parse its
// summary line to detect "soft" failures and trigger the spotDL fallback.
//
//	Summary: 0 Success, 1 Failed. Output dir: ...
var spotiflacSummaryRE = regexp.MustCompile(`Summary:\s*(\d+)\s*Success,\s*(\d+)\s*Failed`)

// spotiflacReportedFailure returns (true, summaryLine) if the job's log shows
// a Summary line where Success == 0 and Failed > 0.
func spotiflacReportedFailure(log []string) (bool, string) {
	// Scan from the end — summary is always last.
	for i := len(log) - 1; i >= 0; i-- {
		m := spotiflacSummaryRE.FindStringSubmatch(log[i])
		if m == nil {
			continue
		}
		success, _ := strconv.Atoi(m[1])
		failed, _ := strconv.Atoi(m[2])
		if success == 0 && failed > 0 {
			return true, log[i]
		}
		return false, ""
	}
	return false, ""
}

// Spotiflac prints "Found Track: Title - Artist" (no parens, no errors) for
// single-track URLs — the cleanest source of metadata to extract.
var spotiflacFoundTrackRE = regexp.MustCompile(`Found Track:\s+(.+?)\r?\n`)

// Per-track failure line: "[1/1] Failed: Title - Artist (error_text)". The
// error_text portion can contain nested parens (e.g. context deadline
// exceeded (Client.Timeout exceeded)), so we use a lazy match terminated by
// the first " (" to capture the Title-Artist segment. This may truncate a
// track title that itself contains "(", but that's acceptable — spotDL's
// fuzzy search handles partial queries well.
var spotiflacFailedTrackRE = regexp.MustCompile(`\[\d+/\d+\]\s+Failed:\s+(.+?)\s+\(`)

// extractFailedTrackQueries returns deduped "Title - Artist" strings parsed
// from spotiflac output. It prefers "Found Track:" lines (cleanest) but
// falls back to "[N/M] Failed:" lines for albums/playlists where per-track
// "Found Track:" lines aren't printed.
func extractFailedTrackQueries(log []string) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(q string) {
		q = strings.TrimSpace(q)
		// Strip any "[spotiflac] " prefix our log wrapper added.
		q = strings.TrimPrefix(q, "[spotiflac] ")
		q = strings.TrimSpace(q)
		if q == "" || seen[q] {
			return
		}
		seen[q] = true
		out = append(out, q)
	}
	for _, line := range log {
		if m := spotiflacFoundTrackRE.FindStringSubmatch(line); m != nil {
			add(m[1])
			continue
		}
		if m := spotiflacFailedTrackRE.FindStringSubmatch(line); m != nil {
			add(m[1])
		}
	}
	return out
}

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

const maxLogLines = 1000

// maxBodySize is the maximum allowed size for HTTP request bodies (1 MB).
const maxBodySize = 1 << 20

// processTimeout is the maximum time a subprocess (spotiflac, yt-dlp, spotdl)
// is allowed to run before being killed. Hung processes permanently occupy
// semaphore slots and can deadlock all downloads.
const processTimeout = 30 * time.Minute

type Job struct {
	ID         string   `json:"id"`
	URL        string   `json:"url"`
	Output     string   `json:"output"`
	Status     Status   `json:"status"`
	StartedAt  int64    `json:"started_at"`
	FinishedAt int64    `json:"finished_at,omitempty"`
	Error      string   `json:"error,omitempty"`
	Tool       string   `json:"tool,omitempty"`        // "spotiflac", "spotdl", "ytdlp", "poddl", "http", etc.
	UsedFallback bool   `json:"used_fallback,omitempty"`
	IsSearch   bool     `json:"is_search,omitempty"`   // true when created via /download/search (no Spotify URL)
	TrackID    int64    `json:"track_id,omitempty"`    // set when search resolves to existing library track
	Kind       string   `json:"kind,omitempty"`        // "music" (default) or "podcast"; differentiates the source on the Downloads page
	Log        []string `json:"log,omitempty"`

	cmd *exec.Cmd `json:"-"`
}

type Config struct {
	Bin          string // SpotiFLAC binary
	Output       string
	FolderFormat string
	SpotdlBin           string // spotDL binary (fallback)
	SpotdlFormat        string // mp3, flac, ogg, opus, m4a, wav
	SpotdlAudio         string // comma-separated audio providers (e.g. "piped,youtube")
	SpotifyClientID     string // user's Spotify app credentials (used by spotdl to avoid shared rate limit)
	SpotifyClientSecret string
	YtdlpBin            string // yt-dlp binary (final fallback — searches YouTube directly, no Spotify)
	YtdlpFormat         string // mp3, m4a, etc.
	FfmpegBin           string // ffmpeg path for yt-dlp audio extraction
	FfprobeBin          string // ffprobe path for post-download validation (auto-detected from FfmpegBin if empty)
	DeepSeekAPIKey      string
	DeepSeekModel       string
	DeepSeekThinking    string
	DeepSeekBaseURL     string
	DownloadConcurrency int
}

// RescanFunc is called after a successful download.
type RescanFunc func()

type API struct {
	cfg            Config
	db             *sql.DB
	rescan         RescanFunc
	mu             sync.Mutex
	sema           chan struct{}
	jobs           map[string]*Job
	order          []string
	maxKeep        int
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
}

func New(cfg Config, db *sql.DB, rescan RescanFunc) *API {
	concurrency := cfg.DownloadConcurrency
	if concurrency <= 0 {
		concurrency = 2
	}
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	a := &API{
		cfg:            cfg,
		db:             db,
		rescan:         rescan,
		jobs:           map[string]*Job{},
		sema:           make(chan struct{}, concurrency),
		maxKeep:        50,
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
	}
	// Startup recovery: mark any jobs left in 'running' status as failed,
	// then load the most recent N jobs into memory.
	a.recoverJobs()
	return a
}

// Shutdown signals all in-flight download goroutines to cancel and waits
// for semaphore slots to drain. Call this before shutting down the HTTP
// server so that run() / runSearch() goroutines observe the cancelled
// context and exit promptly instead of running until processTimeout.
func (a *API) Shutdown() {
	a.shutdownCancel()
	// Wait for all semaphore slots to be released, confirming every
	// in-flight goroutine has returned.
	for i := 0; i < cap(a.sema); i++ {
		a.sema <- struct{}{}
	}
}

// jobContext returns a context that is cancelled when either the request
// context is cancelled (client disconnect) or the shutdown context is
// cancelled (server shutting down). Pass this to run/runSearch so that
// in-flight downloads are promptly cancelled on shutdown.
func (a *API) jobContext(reqCtx context.Context) context.Context {
	// Downloads run in fire-and-forget goroutines that outlive the HTTP request.
	// Use only the shutdown context so downloads aren't killed when the handler returns.
	// The reqCtx parameter is kept for API compatibility but is intentionally not used
	// to avoid canceling downloads on normal request completion.
	return a.shutdownCtx
}

// recoverJobs runs at startup to clean up stale running jobs from a
// previous crash and to load the most recent download_jobs into memory.
func (a *API) recoverJobs() {
	if a.db == nil {
		return
	}
	// Mark stale running/queued jobs as failed
	_, _ = a.db.Exec(`UPDATE download_jobs SET status='failed', error='server restarted' WHERE status IN ('running','queued')`)

	// Load the most recent maxKeep jobs into memory
	rows, err := a.db.Query(`SELECT id, url, output, status, started_at, finished_at, error, tool, used_fallback, is_search, track_id, IFNULL(kind, 'music') FROM download_jobs ORDER BY created_at DESC LIMIT ?`, a.maxKeep)
	if err != nil {
		log.Printf("[downloader] recoverJobs query: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var j Job
		var finishedAt sql.NullInt64
		var errStr, tool sql.NullString
		var usedFallback, isSearch int
		var trackID sql.NullInt64
		if err := rows.Scan(&j.ID, &j.URL, &j.Output, &j.Status, &j.StartedAt, &finishedAt, &errStr, &tool, &usedFallback, &isSearch, &trackID, &j.Kind); err != nil {
			log.Printf("[downloader] recoverJobs scan: %v", err)
			continue
		}
		if finishedAt.Valid {
			j.FinishedAt = finishedAt.Int64
		}
		if errStr.Valid {
			j.Error = errStr.String
		}
		if tool.Valid {
			j.Tool = tool.String
		}
		j.UsedFallback = usedFallback == 1
		j.IsSearch = isSearch == 1
		if trackID.Valid {
			j.TrackID = trackID.Int64
		}
		j.Log = []string{} // don't restore full log
		a.jobs[j.ID] = &j
		a.order = append(a.order, j.ID)
	}
	log.Printf("[downloader] recovered %d jobs from database", len(a.jobs))
}

func (a *API) configured() bool {
	return a.cfg.Bin != "" && a.cfg.Output != ""
}

func (a *API) Mount(r chi.Router) {
	r.Get("/api/download/status", a.status)
	r.Post("/api/download", a.enqueue)
	r.Post("/api/download/search", a.searchEnqueue)
	r.Get("/api/download/jobs", a.listJobs)
	r.Get("/api/download/jobs/{id}", a.getJob)
	r.Post("/api/download/jobs/{id}/cancel", a.cancelJob)
	// Track upgrade endpoints (re-download with new pipeline)
	r.Post("/api/library/upgrade", a.upgradeTrack)
	r.Post("/api/library/upgrade-all", a.upgradeAll)
}

// ----- HTTP -----

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[downloader] writeJSON encode: %v", err)
	}
}

type statusResponse struct {
	Configured       bool   `json:"configured"`
	Bin              string `json:"bin,omitempty"`
	Output           string `json:"output,omitempty"`
	FallbackEnabled  bool   `json:"fallback_enabled"`
	SpotdlBin        string `json:"spotdl_bin,omitempty"`
	SpotdlFormat     string `json:"spotdl_format,omitempty"`
	YtdlpBin         string `json:"ytdlp_bin,omitempty"`
	YtdlpFormat      string `json:"ytdlp_format,omitempty"`
	FfmpegBin        string `json:"ffmpeg_bin,omitempty"`
}

func (a *API) status(w http.ResponseWriter, _ *http.Request) {
	s := statusResponse{Configured: a.configured()}
	if a.configured() {
		s.Bin = a.cfg.Bin
		s.Output = a.cfg.Output
	}
	if a.cfg.SpotdlBin != "" {
		s.FallbackEnabled = true
		s.SpotdlBin = a.cfg.SpotdlBin
		s.SpotdlFormat = a.cfg.SpotdlFormat
	}
	if a.cfg.YtdlpBin != "" {
		s.YtdlpBin = a.cfg.YtdlpBin
		s.YtdlpFormat = a.cfg.YtdlpFormat
	}
	s.FfmpegBin = a.cfg.FfmpegBin
	writeJSON(w, s)
}

type enqueueReq struct {
	URL string `json:"url"`
}

func (a *API) enqueue(w http.ResponseWriter, r *http.Request) {
	if !a.configured() {
		log.Printf("[downloader] enqueue: not configured (bin=%q output=%q)", a.cfg.Bin, a.cfg.Output)
		http.Error(w, "SpotiFLAC not configured. Set SPOTIFLAC_BIN and SPOTIFLAC_OUTPUT (or MEDIA_ROOTS) in backend/.env.", 400)
		return
	}
	var req enqueueReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		log.Printf("[downloader] enqueue decode: %v", err)
		http.Error(w, err.Error(), 400)
		return
	}
	url := strings.TrimSpace(req.URL)
	if !strings.HasPrefix(url, "https://open.spotify.com/") &&
		!strings.HasPrefix(url, "http://open.spotify.com/") &&
		!strings.HasPrefix(url, "spotify:") {
		log.Printf("[downloader] enqueue invalid URL: %s", url)
		http.Error(w, "URL must be a Spotify open.spotify.com URL or spotify: URI", 400)
		return
	}
	job := &Job{
		ID:        uuid.NewString(),
		URL:       url,
		Output:    a.cfg.Output,
		Status:    StatusQueued,
		StartedAt: time.Now().Unix(),
		Kind:      "music",
		Log:       []string{},
	}
	a.mu.Lock()
	a.jobs[job.ID] = job
	a.order = append([]string{job.ID}, a.order...)
	if len(a.order) > a.maxKeep {
		for _, oldID := range a.order[a.maxKeep:] {
			delete(a.jobs, oldID)
		}
		a.order = a.order[:a.maxKeep]
	}
	a.mu.Unlock()

	// Persist to database
	if a.db != nil {
		_, _ = a.db.Exec(
			`INSERT INTO download_jobs(id, url, output, status, started_at, tool, is_search, kind) VALUES(?, ?, ?, ?, ?, '', 0, ?)`,
			job.ID, job.URL, job.Output, string(job.Status), job.StartedAt, job.Kind)
		a.evictOldJobs()
	}

	go a.run(job, a.jobContext(r.Context()))
	writeJSON(w, jobSummary(job))
}

type searchReq struct {
	Query string `json:"query"`
}

// findLibraryTrack tries multiple strategies to locate an existing track.
// It handles "Artist - Title" queries generated by the AI playlist feature.
func (a *API) findLibraryTrack(ctx context.Context, query string) (int64, error) {
	cleanQuery := strings.ReplaceAll(query, " - ", " ")
	cleanQuery = strings.TrimSpace(cleanQuery)

	// Strategy 1: Extract artist and title from "Artist - Title" format
	if parts := strings.SplitN(query, " - ", 2); len(parts) == 2 {
		artist := strings.TrimSpace(parts[0])
		title := strings.TrimSpace(parts[1])

		// 1a: Exact case-insensitive match on title + artist
		var id int64
		err := a.db.QueryRowContext(ctx,
			`SELECT id FROM tracks WHERE LOWER(title)=LOWER(?) AND LOWER(IFNULL(artist,''))=LOWER(?) LIMIT 1`,
			title, artist).Scan(&id)
		if err == nil {
			return id, nil
		}

		// 1b: Title starts with given title, artist matches (handles "Red Eyes (Album Version)")
		err = a.db.QueryRowContext(ctx,
			`SELECT id FROM tracks WHERE LOWER(title) LIKE LOWER(?) AND LOWER(IFNULL(artist,''))=LOWER(?) LIMIT 1`,
			title+"%", artist).Scan(&id)
		if err == nil {
			return id, nil
		}
	}

	// Strategy 2: FTS5 with cleaned query (remove dashes)
	tokens := strings.Fields(cleanQuery)
	if len(tokens) > 0 {
		for i, t := range tokens {
			tokens[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
		}
		ftsQ := strings.Join(tokens, " AND ")
		var id int64
		err := a.db.QueryRowContext(ctx,
			`SELECT t.id FROM tracks_fts f JOIN tracks t ON t.id=f.rowid
			 WHERE tracks_fts MATCH ? ORDER BY rank LIMIT 1`, ftsQ).Scan(&id)
		if err == nil {
			return id, nil
		}
	}

	// Strategy 3: LIKE on any field (most lenient)
	var id int64
	likeQ := "%" + cleanQuery + "%"
	err := a.db.QueryRowContext(ctx,
		`SELECT id FROM tracks
		 WHERE LOWER(title) LIKE LOWER(?) OR LOWER(IFNULL(artist,'')) LIKE LOWER(?)
		 LIMIT 1`,
		likeQ, likeQ).Scan(&id)
	if err == nil {
		return id, nil
	}

	return 0, sql.ErrNoRows
}

func (a *API) searchEnqueue(w http.ResponseWriter, r *http.Request) {
	if a.cfg.YtdlpBin == "" {
		log.Printf("[downloader] searchEnqueue: YtdlpBin not configured")
		http.Error(w, "yt-dlp not configured. Set YTDLP_BIN in backend/.env.", 400)
		return
	}
	var req searchReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		log.Printf("[downloader] searchEnqueue decode: %v", err)
		http.Error(w, err.Error(), 400)
		return
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		log.Printf("[downloader] searchEnqueue: empty query")
		http.Error(w, "query is required", 400)
		return
	}
	log.Printf("[downloader] searchEnqueue: query=%q ytdlp_bin=%q output=%q", query, a.cfg.YtdlpBin, a.cfg.Output)

	// Check library first to avoid re-downloading existing tracks
	if a.db != nil {
		trackID, err := a.findLibraryTrack(r.Context(), query)
		if err == nil && trackID > 0 {
			log.Printf("[downloader] searchEnqueue: query=%q resolved to existing track %d", query, trackID)
			job := &Job{
				ID:         uuid.NewString(),
				URL:        query,
				Output:     a.cfg.Output,
				Status:     StatusSucceeded,
				StartedAt:  time.Now().Unix(),
				FinishedAt: time.Now().Unix(),
				IsSearch:   true,
				TrackID:    trackID,
				Kind:       "music",
				Log:        []string{"[search] resolved to existing library track"},
			}
			a.mu.Lock()
			a.jobs[job.ID] = job
			a.order = append([]string{job.ID}, a.order...)
			if len(a.order) > a.maxKeep {
				for _, oldID := range a.order[a.maxKeep:] {
					delete(a.jobs, oldID)
				}
				a.order = a.order[:a.maxKeep]
			}
			a.mu.Unlock()

			// Persist to database
			if a.db != nil {
				_, _ = a.db.Exec(
					`INSERT INTO download_jobs(id, url, output, status, started_at, finished_at, is_search, track_id, kind) VALUES(?, ?, ?, ?, ?, ?, 1, ?, ?)`,
					job.ID, job.URL, job.Output, string(job.Status), job.StartedAt, job.FinishedAt, job.TrackID, job.Kind)
			}

			writeJSON(w, jobSummary(job))
			return
		}
	}

	job := &Job{
		ID:        uuid.NewString(),
		URL:       query, // store the search query as the URL for tracking
		Output:    a.cfg.Output,
		Status:    StatusQueued,
		StartedAt: time.Now().Unix(),
		Log:       []string{},
		IsSearch:  true,
		Kind:      "music",
	}
	a.mu.Lock()
	a.jobs[job.ID] = job
	a.order = append([]string{job.ID}, a.order...)
	if len(a.order) > a.maxKeep {
		for _, oldID := range a.order[a.maxKeep:] {
			delete(a.jobs, oldID)
		}
		a.order = a.order[:a.maxKeep]
	}
	a.mu.Unlock()

	// Persist to database
	if a.db != nil {
		_, _ = a.db.Exec(
			`INSERT INTO download_jobs(id, url, output, status, started_at, is_search, kind) VALUES(?, ?, ?, ?, ?, 1, ?)`,
			job.ID, job.URL, job.Output, string(job.Status), job.StartedAt, job.Kind)
		a.evictOldJobs()
	}

	go a.runSearch(job, a.jobContext(r.Context()))
	writeJSON(w, jobSummary(job))
}

// jobSummary returns a copy without the log array (for list views).
func jobSummary(j *Job) *Job {
	cp := *j
	cp.Log = nil
	cp.cmd = nil
	return &cp
}

// jobFull returns a deep copy of j with log included.
// The Log slice is copied to a new backing array so the returned
// Job is fully independent of the original — safe to use after
// the source is mutated by another goroutine.
func jobFull(j *Job) *Job {
	cp := *j
	cp.Log = append([]string(nil), j.Log...)
	cp.cmd = nil
	return &cp
}

func (a *API) listJobs(w http.ResponseWriter, _ *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]*Job, 0, len(a.order))
	for _, id := range a.order {
		out = append(out, jobSummary(a.jobs[id]))
	}
	writeJSON(w, out)
}

func (a *API) getJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a.mu.Lock()
	j, ok := a.jobs[id]
	if !ok {
		a.mu.Unlock()
		log.Printf("[downloader] getJob: job %s not found", id)
		http.Error(w, "not found", 404)
		return
	}
	resp := jobFull(j)
	a.mu.Unlock()
	writeJSON(w, resp)
}

func (a *API) cancelJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	a.mu.Lock()
	j, ok := a.jobs[id]
	if !ok {
		a.mu.Unlock()
		log.Printf("[downloader] cancelJob: job %s not found", id)
		http.Error(w, "not found", 404)
		return
	}
	if j.Status != StatusQueued && j.Status != StatusRunning {
		// Already in a terminal state — nothing to cancel.
		a.mu.Unlock()
		writeJSON(w, map[string]bool{"ok": true})
		return
	}
	// Snapshot the process handle under the lock, then set cancelled status
	// atomically before releasing. This prevents a race where another goroutine
	// could observe or modify the job between the status check and the update.
	proc := j.cmd
	j.Status = StatusCancelled
	j.FinishedAt = time.Now().Unix()
	finishedAt := j.FinishedAt
	a.mu.Unlock()

	// Kill the subprocess outside the lock (Kill may block).
	if proc != nil && proc.Process != nil {
		_ = proc.Process.Kill()
	}

	// Persist cancellation to DB
	if a.db != nil {
		_, _ = a.db.Exec(`UPDATE download_jobs SET status='cancelled', finished_at=? WHERE id=?`, finishedAt, id)
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// ----- External job API -----
// These three methods let other packages (e.g. podcaster) register their
// downloads with the unified job system so they appear on the Downloads
// page alongside SpotiFLAC/yt-dlp/spotDL music jobs. The external caller
// is responsible for actually performing the download — this just gives
// them a place to record progress, errors, and a streaming log.

// RegisterExternalJob creates a new job in 'running' state and returns the
// generated job ID. `kind` differentiates the source ("music" vs "podcast")
// and `tool` identifies the specific downloader (e.g. "poddl", "http").
// `url` is the human-readable label shown in the UI (episode title, search
// query, or actual URL — whatever is most informative).
func (a *API) RegisterExternalJob(kind, url, output, tool string) string {
	if kind == "" {
		kind = "music"
	}
	job := &Job{
		ID:        uuid.NewString(),
		URL:       url,
		Output:    output,
		Status:    StatusRunning,
		StartedAt: time.Now().Unix(),
		Tool:      tool,
		Kind:      kind,
		Log:       []string{},
	}
	a.mu.Lock()
	a.jobs[job.ID] = job
	a.order = append([]string{job.ID}, a.order...)
	if len(a.order) > a.maxKeep {
		for _, oldID := range a.order[a.maxKeep:] {
			delete(a.jobs, oldID)
		}
		a.order = a.order[:a.maxKeep]
	}
	a.mu.Unlock()
	if a.db != nil {
		_, _ = a.db.Exec(
			`INSERT INTO download_jobs(id, url, output, status, started_at, tool, kind) VALUES(?, ?, ?, ?, ?, ?, ?)`,
			job.ID, job.URL, job.Output, string(job.Status), job.StartedAt, job.Tool, job.Kind)
		a.evictOldJobs()
	}
	return job.ID
}

// AppendExternalLog appends a single line to the in-memory log of a job
// previously created via RegisterExternalJob. Safe to call from any goroutine.
// Silently no-ops if the job doesn't exist (e.g. evicted).
func (a *API) AppendExternalLog(id, line string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	job, ok := a.jobs[id]
	if !ok {
		return
	}
	job.Log = append(job.Log, line)
	if len(job.Log) > maxLogLines {
		job.Log = job.Log[len(job.Log)-maxLogLines:]
	}
}

// FinishExternalJob marks an external job as succeeded/failed/cancelled and
// persists the final state to the database. `errMsg` should be empty on success.
func (a *API) FinishExternalJob(id string, status Status, errMsg string) {
	a.mu.Lock()
	job, ok := a.jobs[id]
	if !ok {
		a.mu.Unlock()
		return
	}
	job.Status = status
	job.Error = errMsg
	job.FinishedAt = time.Now().Unix()
	a.mu.Unlock()
	if a.db != nil {
		_, _ = a.db.Exec(
			`UPDATE download_jobs SET status=?, finished_at=?, error=?, tool=? WHERE id=?`,
			string(status), job.FinishedAt, errMsg, job.Tool, id)
	}
}

// evictOldJobs deletes old download_jobs rows from the database when
// the in-memory order exceeds maxKeep.
func (a *API) evictOldJobs() {
	if a.db == nil {
		return
	}
	_, _ = a.db.Exec(`DELETE FROM download_jobs WHERE id NOT IN
		(SELECT id FROM download_jobs ORDER BY created_at DESC LIMIT ?)`,
		a.maxKeep)
}

// ----- subprocess runner -----

func (a *API) run(job *Job, ctx context.Context) {
	// Acquire semaphore slot (blocks until concurrency limit allows)
	a.sema <- struct{}{}
	defer func() { <-a.sema }()

	// Check if cancelled while waiting for slot
	a.mu.Lock()
	if job.Status == StatusCancelled {
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()

	log.Printf("[downloader] run: starting job=%s url=%q tool=spotiflac", job.ID, job.URL)

	a.mu.Lock()
	job.Status = StatusRunning
	job.Tool = "spotiflac"
	a.mu.Unlock()
	// Persist status change to DB
	if a.db != nil {
		_, _ = a.db.Exec(`UPDATE download_jobs SET status='running', tool='spotiflac' WHERE id=?`, job.ID)
	}

	a.appendLog(job, "[spotiflac] starting download")
	args := []string{"-o", a.cfg.Output}
	if a.cfg.FolderFormat != "" {
		args = append(args, "-folder-format", a.cfg.FolderFormat)
	}
	args = append(args, job.URL)

	primaryErr := a.runProcess(job, "spotiflac", a.cfg.Bin, args, ctx)

	// If user cancelled, stop here
	a.mu.Lock()
	if job.Status == StatusCancelled {
		a.mu.Unlock()
		return
	}
	logCopy := append([]string(nil), job.Log...)
	a.mu.Unlock()

	// SpotiFLAC always exits 0, so detect "soft" failures by parsing the summary.
	if primaryErr == nil {
		if soft, summary := spotiflacReportedFailure(logCopy); soft {
			primaryErr = fmt.Errorf("%s", summary)
		}
	}

	if primaryErr == nil {
		a.finish(job, StatusSucceeded, "")
		if a.rescan != nil {
			go a.rescan()
		}
		return
	}

	// SpotiFLAC failed → try yt-dlp fallback first (no Spotify API, most reliable)
	if a.cfg.YtdlpBin == "" {
		a.finish(job, StatusFailed, primaryErr.Error())
		return
	}

	a.appendLog(job, "")
	a.appendLog(job, fmt.Sprintf("[spotiflac] failed: %s", primaryErr.Error()))
	a.appendLog(job, "[ytdlp] falling back to yt-dlp (searches YouTube directly — no Spotify)")

	a.mu.Lock()
	job.Tool = "spotiflac\u2192ytdlp"
	job.UsedFallback = true
	a.mu.Unlock()

	// Build yt-dlp command. Use the first parsed query or fall back to the URL.
	ytdlpQuery := job.URL
	if queries := extractFailedTrackQueries(logCopy); len(queries) > 0 {
		ytdlpQuery = queries[0]
	}
	ytdlpSearch := "ytsearch1:" + ytdlpQuery

	ytdlpFormat := a.cfg.YtdlpFormat
	if ytdlpFormat == "" {
		ytdlpFormat = "mp3"
	}

	outputDir := strings.ReplaceAll(a.cfg.Output, "\\", "/")
	ytdlpArgs := []string{
		ytdlpSearch,
		"-f", "bestaudio/best",
		"--no-playlist",
		"--add-metadata",
		"--embed-thumbnail",
		"--convert-thumbnails", "jpg",
		"--newline",
		"--no-warnings",
		// Harden: make failures fatal and add retries
		"--abort-on-error",
		"--retries", "3",
		"--fragment-retries", "10",
		// Harden: use Android client for more reliable YouTube access
		"--extractor-args", "youtube:player_client=android",
		"-o", outputDir + "/%(artist)s - %(title)s.%(ext)s",
	}
	if a.cfg.FfmpegBin != "" {
		ytdlpArgs = append(ytdlpArgs, "--ffmpeg-location", a.cfg.FfmpegBin)
	}

	a.appendLog(job, fmt.Sprintf("[ytdlp] command: %s %s", a.cfg.YtdlpBin, strings.Join(ytdlpArgs, " ")))

	fallbackErr := a.runProcess(job, "ytdlp", a.cfg.YtdlpBin, ytdlpArgs, ctx)

	a.mu.Lock()
	cancelled := job.Status == StatusCancelled
	a.mu.Unlock()
	if cancelled {
		return
	}

	if fallbackErr == nil {
		// Validate downloaded file
		dlFile := a.findDownloadedFile(time.Unix(job.StartedAt, 0))
		if dlFile != "" {
			if verr := a.verifyDownloadedFile(ctx, dlFile); verr != nil {
				a.appendLog(job, fmt.Sprintf("[verify] file invalid: %s", verr.Error()))
				a.appendLog(job, "[verify] retrying with ytsearch2 and m4a format...")
				retrySearch := "ytsearch2:" + strings.TrimPrefix(ytdlpSearch, "ytsearch1:")
				retryOutputDir := strings.ReplaceAll(a.cfg.Output, "\\", "/")
			retryArgs := []string{
				retrySearch,
				"-f", "bestaudio/best",
				"--no-playlist", "--add-metadata",
				"--embed-thumbnail", "--convert-thumbnails", "jpg", "--newline", "--no-warnings",
				"--abort-on-error", "--retries", "3", "--fragment-retries", "10",
				"--extractor-args", "youtube:player_client=android",
				"-o", retryOutputDir+"/%(artist)s - %(title)s.%(ext)s",
			}
				if a.cfg.FfmpegBin != "" {
					retryArgs = append(retryArgs, "--ffmpeg-location", a.cfg.FfmpegBin)
				}
			retryErr := a.runProcess(job, "ytdlp-retry", a.cfg.YtdlpBin, retryArgs, ctx)
			if retryErr != nil {
				a.finish(job, StatusFailed, fmt.Sprintf("download invalid and retry failed: %s", retryErr.Error()))
				return
			}
			retryFile := a.findDownloadedFile(time.Unix(job.StartedAt, 0))
			if retryFile != "" {
				if verr := a.verifyDownloadedFile(ctx, retryFile); verr != nil {
					a.finish(job, StatusFailed, fmt.Sprintf("download invalid, retry also invalid: %s", verr.Error()))
						return
					}
				}
			}
		}
		a.finish(job, StatusSucceeded, "")
		if a.rescan != nil {
			go a.rescan()
		}
		return
	}

	// yt-dlp failed → try spotDL as last resort
	if a.cfg.SpotdlBin == "" {
		a.finish(job, StatusFailed, fmt.Sprintf("both tools failed. spotiflac: %s; yt-dlp: %s", primaryErr.Error(), fallbackErr.Error()))
		return
	}

	a.appendLog(job, "")
	a.appendLog(job, fmt.Sprintf("[ytdlp] failed: %s", fallbackErr.Error()))
	a.appendLog(job, "[spotdl] falling back to spotDL (downloads from YouTube Music)")

	a.mu.Lock()
	job.Tool = "spotiflac\u2192ytdlp\u2192spotdl"
	job.UsedFallback = true
	a.mu.Unlock()

	// spotDL output template: <output_dir>/<artist>/<album>/<title>.<ext>
	outputTemplate := strings.ReplaceAll(a.cfg.Output, "\\", "/") + "/{artist}/{album}/{title}.{output-ext}"
	format := a.cfg.SpotdlFormat
	if format == "" {
		format = "mp3"
	}
	spotdlTargets := []string{job.URL}
	if queries := extractFailedTrackQueries(logCopy); len(queries) > 0 {
		spotdlTargets = queries
		a.appendLog(job, fmt.Sprintf("[spotdl] using %d track query(s) parsed from spotiflac metadata (skipping Spotify API lookup)", len(queries)))
		for _, q := range queries {
			a.appendLog(job, fmt.Sprintf("[spotdl]   → %q", q))
		}
	} else {
		a.appendLog(job, "[spotdl] could not parse track names from spotiflac log; passing Spotify URL")
	}

	spotdlArgs := []string{"download"}
	spotdlArgs = append(spotdlArgs, spotdlTargets...)
	spotdlArgs = append(spotdlArgs,
		"--output", outputTemplate,
		"--format", format,
		"--threads", "2",
		"--print-errors",
	)
	if a.cfg.SpotifyClientID != "" && a.cfg.SpotifyClientSecret != "" {
		spotdlArgs = append(spotdlArgs,
			"--client-id", a.cfg.SpotifyClientID,
			"--client-secret", a.cfg.SpotifyClientSecret,
		)
	}
	if a.cfg.SpotdlAudio != "" {
		providers := []string{}
		for _, p := range strings.Split(a.cfg.SpotdlAudio, ",") {
			if p = strings.TrimSpace(p); p != "" {
				providers = append(providers, p)
			}
		}
		if len(providers) > 0 {
			spotdlArgs = append(spotdlArgs, "--audio")
			spotdlArgs = append(spotdlArgs, providers...)
		}
	}

	a.appendLog(job, fmt.Sprintf("[spotdl] command: %s %s", a.cfg.SpotdlBin, strings.Join(spotdlArgs, " ")))

	spotdlErr := a.runProcess(job, "spotdl", a.cfg.SpotdlBin, spotdlArgs, ctx)

	a.mu.Lock()
	cancelled = job.Status == StatusCancelled
	a.mu.Unlock()
	if cancelled {
		return
	}

	if spotdlErr != nil {
		a.finish(job, StatusFailed, fmt.Sprintf("all tools failed. spotiflac: %s; yt-dlp: %s; spotdl: %s", primaryErr.Error(), fallbackErr.Error(), spotdlErr.Error()))
		return
	}
	// Validate downloaded file
	dlFile := a.findDownloadedFile(time.Unix(job.StartedAt, 0))
	if dlFile != "" {
		if verr := a.verifyDownloadedFile(ctx, dlFile); verr != nil {
			a.finish(job, StatusFailed, fmt.Sprintf("download invalid: %s", verr.Error()))
			return
		}
	}
	a.finish(job, StatusSucceeded, "")
	if a.rescan != nil {
		go a.rescan()
	}
}

// runSearch handles jobs created via /download/search. It skips SpotiFLAC
// entirely and goes straight to yt-dlp, optionally using DeepSeek to
// refine the raw user query into a structured search term.
func (a *API) runSearch(job *Job, ctx context.Context) {
	// Acquire semaphore slot (blocks until concurrency limit allows)
	a.sema <- struct{}{}
	defer func() { <-a.sema }()

	// Check if cancelled while waiting for slot
	a.mu.Lock()
	if job.Status == StatusCancelled {
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()

	a.mu.Lock()
	job.Status = StatusRunning
	job.Tool = "ytdlp-search"
	a.mu.Unlock()
	// Persist status change to DB
	if a.db != nil {
		_, _ = a.db.Exec(`UPDATE download_jobs SET status='running', tool='ytdlp-search' WHERE id=?`, job.ID)
	}

	a.appendLog(job, fmt.Sprintf("[search] query: %s", job.URL))

	// Try to use DeepSeek to parse the query into a better search term
	searchQuery := job.URL
	if a.cfg.DeepSeekAPIKey != "" {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		parsed, err := a.deepseekParseQuery(ctx, job.URL)
		cancel()
		if err == nil && parsed.SearchQuery != "" {
			searchQuery = parsed.SearchQuery
			a.appendLog(job, fmt.Sprintf("[search] DeepSeek parsed: artist=%q title=%q type=%s", parsed.Artist, parsed.Title, parsed.Type))
		} else if err != nil {
			a.appendLog(job, fmt.Sprintf("[search] DeepSeek parse failed (%s), using raw query", err.Error()))
		}
	}

	ytdlpSearch := "ytsearch1:" + searchQuery
	ytdlpFormat := a.cfg.YtdlpFormat
	if ytdlpFormat == "" {
		ytdlpFormat = "mp3"
	}
	outputDir := strings.ReplaceAll(a.cfg.Output, "\\", "/")
	ytdlpArgs := []string{
		ytdlpSearch,
		"-f", "bestaudio/best",
		"--no-playlist",
		"--match-filter", "duration < 600",
		"--add-metadata",
		"--embed-thumbnail",
		"--convert-thumbnails", "jpg",
		"--newline",
		"--no-warnings",
		// Harden: make failures fatal and add retries
		"--abort-on-error",
		"--retries", "3",
		"--fragment-retries", "10",
		// Harden: use Android client for more reliable YouTube access
		"--extractor-args", "youtube:player_client=android",
		"-o", outputDir + "/%(artist)s - %(title)s.%(ext)s",
	}
	if a.cfg.FfmpegBin != "" {
		ytdlpArgs = append(ytdlpArgs, "--ffmpeg-location", a.cfg.FfmpegBin)
	}

	a.appendLog(job, fmt.Sprintf("[ytdlp] command: %s %s", a.cfg.YtdlpBin, strings.Join(ytdlpArgs, " ")))

	err := a.runProcess(job, "ytdlp", a.cfg.YtdlpBin, ytdlpArgs, ctx)

	a.mu.Lock()
	cancelled := job.Status == StatusCancelled
	a.mu.Unlock()
	if cancelled {
		return
	}

	if err != nil {
		log.Printf("[downloader] runSearch: yt-dlp failed for query=%q job=%s: %v", job.URL, job.ID, err)
		a.finish(job, StatusFailed, fmt.Sprintf("yt-dlp failed: %s", err.Error()))
		return
	}

	// Validate downloaded file
	dlFile := a.findDownloadedFile(time.Unix(job.StartedAt, 0))
	if dlFile == "" {
		log.Printf("[downloader] runSearch: no downloaded file found for query=%q job=%s output=%q", job.URL, job.ID, a.cfg.Output)
		a.appendLog(job, "[verify] no audio file found in output directory after download")
		a.finish(job, StatusFailed, "no audio file found in output directory")
		return
	}
	log.Printf("[downloader] runSearch: downloaded file for query=%q job=%s: %s", job.URL, job.ID, dlFile)
	if verr := a.verifyDownloadedFile(ctx, dlFile); verr != nil {
		a.appendLog(job, fmt.Sprintf("[verify] file invalid: %s", verr.Error()))
		a.appendLog(job, "[verify] retrying with ytsearch2 and m4a format...")
		retrySearch := "ytsearch2:" + strings.TrimPrefix(job.URL, "ytsearch1:")
		retryOutputDir := strings.ReplaceAll(a.cfg.Output, "\\", "/")
		retryArgs := []string{
			retrySearch,
			"-f", "bestaudio/best",
			"--no-playlist", "--add-metadata",
			"--embed-thumbnail", "--convert-thumbnails", "jpg", "--newline", "--no-warnings",
			"--abort-on-error", "--retries", "3", "--fragment-retries", "10",
			"--extractor-args", "youtube:player_client=android",
			"-o", retryOutputDir+"/%(artist)s - %(title)s.%(ext)s",
		}
		if a.cfg.FfmpegBin != "" {
			retryArgs = append(retryArgs, "--ffmpeg-location", a.cfg.FfmpegBin)
		}
		retryErr := a.runProcess(job, "ytdlp-retry", a.cfg.YtdlpBin, retryArgs, ctx)
		if retryErr != nil {
			log.Printf("[downloader] runSearch: retry also failed for query=%q job=%s: %v", job.URL, job.ID, retryErr)
			a.finish(job, StatusFailed, fmt.Sprintf("download invalid and retry failed: %s", retryErr.Error()))
			return
		}
		retryFile := a.findDownloadedFile(time.Unix(job.StartedAt, 0))
		if retryFile != "" {
			if verr := a.verifyDownloadedFile(ctx, retryFile); verr != nil {
				log.Printf("[downloader] runSearch: retry file also invalid for query=%q job=%s: %v", job.URL, job.ID, verr)
				a.finish(job, StatusFailed, fmt.Sprintf("download invalid, retry also invalid: %s", verr.Error()))
				return
			}
		}
	}

	log.Printf("[downloader] runSearch: SUCCESS query=%q job=%s file=%s", job.URL, job.ID, dlFile)
	a.finish(job, StatusSucceeded, "")

	// Try to resolve the downloaded file to a track ID so the frontend
	// doesn't have to race with the scanner. Poll the DB for up to 2 minutes
	// checking for a track whose path matches the downloaded file.
	if a.db != nil {
		go func() {
			pollCtx, pollCancel := context.WithTimeout(ctx, 2*time.Minute)
			defer pollCancel()

			dlFile := a.findDownloadedFile(time.Unix(job.StartedAt, 0))
			if dlFile == "" {
				return
			}
			for i := 0; i < 40; i++ {
				time.Sleep(3 * time.Second)
				var id int64
				err := a.db.QueryRowContext(pollCtx,
					`SELECT id FROM tracks WHERE path=? LIMIT 1`, dlFile).Scan(&id)
				if err == nil && id > 0 {
					a.mu.Lock()
					job.TrackID = id
					a.mu.Unlock()
					log.Printf("[downloader] runSearch: resolved track %d for %s", id, dlFile)
					return
				}
				if pollCtx.Err() != nil {
					log.Printf("[downloader] runSearch: track resolution cancelled for %s: %v", dlFile, pollCtx.Err())
					return
				}
			}
			log.Printf("[downloader] runSearch: could not resolve track for %s after 2min", dlFile)
		}()
	}
}

	// runProcess starts a subprocess, streams its stdout+stderr into the job log,
// and waits for it. The logPrefix is prepended to every log line so the user
// can see which tool produced what output.
func (a *API) runProcess(job *Job, logPrefix, bin string, args []string, ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, processTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	log.Printf("[downloader] runProcess: job=%s tool=%s bin=%q args=%q", job.ID, logPrefix, bin, args)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("[downloader] runProcess: job=%s tool=%s stdout pipe error: %v", job.ID, logPrefix, err)
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("[downloader] runProcess: job=%s tool=%s stderr pipe error: %v", job.ID, logPrefix, err)
		return err
	}
	a.mu.Lock()
	job.cmd = cmd
	a.mu.Unlock()
	if err := cmd.Start(); err != nil {
		log.Printf("[downloader] runProcess: job=%s tool=%s start error: %v", job.ID, logPrefix, err)
		return fmt.Errorf("failed to start: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a.consumeOutput(job, logPrefix, stdout) }()
	go func() { defer wg.Done(); a.consumeOutput(job, logPrefix, stderr) }()
	wg.Wait()
	return cmd.Wait()
}

func (a *API) consumeOutput(job *Job, prefix string, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		a.appendLog(job, fmt.Sprintf("[%s] %s", prefix, sc.Text()))
	}
}

func (a *API) appendLog(job *Job, line string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	job.Log = append(job.Log, line)
	if len(job.Log) > maxLogLines {
		job.Log = job.Log[len(job.Log)-maxLogLines:]
	}
}

func isValidAudioFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3", ".flac", ".ogg", ".m4a", ".aac", ".wav", ".wma", ".opus", ".mp4", ".webm":
		return true
	}
	// Try MIME detection as fallback
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	mime := http.DetectContentType(buf[:n])
	return strings.HasPrefix(mime, "audio/")
}

func (a *API) verifyDownloadedFile(ctx context.Context, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}
	if info.Size() < 10240 {
		return fmt.Errorf("file too small (%d bytes)", info.Size())
	}
	if a.cfg.FfprobeBin != "" {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, a.cfg.FfprobeBin,
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
		output := string(out)
		if !strings.Contains(output, "codec_type=audio") {
			return fmt.Errorf("no audio stream found")
		}
	}
	return nil
}

func (a *API) findDownloadedFile(before time.Time) string {
	cutoff := before.Add(-5 * time.Minute)
	var best string
	bestTime := time.Time{}
	filepath.Walk(a.cfg.Output, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".mp3", ".flac", ".ogg", ".m4a", ".aac", ".wav", ".opus", ".webm", ".mp4":
			if info.ModTime().After(cutoff) && info.ModTime().Before(before.Add(5*time.Minute)) {
				if info.ModTime().After(bestTime) {
					bestTime = info.ModTime()
					best = path
				}
			}
		}
		return nil
	})
	return best
}

// validateOutput checks whether the downloaded file is a valid audio file.
// It searches the output directory for recently created audio files and
// validates them with ffprobe. Returns an error string if validation fails.
func (a *API) validateOutput(ctx context.Context, job *Job) string {
	dlFile := a.findDownloadedFile(time.Unix(job.StartedAt, 0))
	if dlFile == "" {
		return "no audio file found in output directory"
	}
	if err := a.verifyDownloadedFile(ctx, dlFile); err != nil {
		return err.Error()
	}
	return ""
}

func (a *API) finish(job *Job, status Status, errMsg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	job.Status = status
	job.Error = errMsg
	job.FinishedAt = time.Now().Unix()
	// Persist final status to database
	if a.db != nil {
		usedFallback := 0
		if job.UsedFallback {
			usedFallback = 1
		}
		isSearch := 0
		if job.IsSearch {
			isSearch = 1
		}
		_, _ = a.db.Exec(
			`UPDATE download_jobs SET status=?, finished_at=?, error=?, tool=?, used_fallback=?, is_search=?, track_id=? WHERE id=?`,
			string(status), job.FinishedAt, errMsg, job.Tool, usedFallback, isSearch, job.TrackID, job.ID)
	}
}

// ----- DeepSeek metadata parser -----

type deepseekMetadata struct {
	Type        string `json:"type"`
	Artist      string `json:"artist"`
	Title       string `json:"title"`
	Album       string `json:"album"`
	SearchQuery string `json:"search_query"`
}

func (a *API) deepseekParseQuery(ctx context.Context, query string) (deepseekMetadata, error) {
	baseURL := a.cfg.DeepSeekBaseURL
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	prompt := fmt.Sprintf(`The user wants to download audio from this search query: %q

Parse it into structured metadata for a YouTube audio search.
Return ONLY valid JSON with this exact shape:
{
  "type": "music|podcast",
  "artist": "...",
  "title": "...",
  "album": "...",
  "search_query": "best yt-dlp search string"
}

Rules for search_query:
- For music: "Artist - Title" (e.g. "The Beatles - Hey Jude")
- For podcasts: "Show - Episode" (e.g. "Joe Rogan Experience - Elon Musk")
- CRITICAL: YouTube search returns music videos first. To find the actual AUDIO track instead of the music video, append " audio" or " - Topic" to the query. Examples:
  - "Meat Loaf - Bat Out of Hell audio"
  - "The Beatles - Hey Jude - Topic"
- For podcasts, just use the show and episode name, no extra words needed
- Keep the query concise but effective

If the query is ambiguous, make your best guess. Output ONLY JSON, no markdown.`, query)

	type dsMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type dsRequest struct {
		Model          string      `json:"model"`
		Messages       []dsMessage `json:"messages"`
		Temperature    float64    `json:"temperature"`
		ThinkingEffort string     `json:"thinking_effort,omitempty"`
	}
	type dsResponse struct {
		Choices []struct {
			Message dsMessage `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	reqBody := dsRequest{
		Model: a.cfg.DeepSeekModel,
		Messages: []dsMessage{
			{Role: "system", Content: "You are a music/podcast metadata parser. Respond with JSON only."},
			{Role: "user", Content: prompt},
		},
		Temperature:    0.3,
		ThinkingEffort: a.cfg.DeepSeekThinking,
	}
	buf, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return deepseekMetadata{}, err
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.DeepSeekAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return deepseekMetadata{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return deepseekMetadata{}, fmt.Errorf("deepseek %d: %s", resp.StatusCode, string(body))
	}
	var dr dsResponse
	if err := json.Unmarshal(body, &dr); err != nil {
		return deepseekMetadata{}, fmt.Errorf("decode: %v", err)
	}
	if dr.Error != nil {
		return deepseekMetadata{}, fmt.Errorf("deepseek error: %s", dr.Error.Message)
	}
	if len(dr.Choices) == 0 {
		return deepseekMetadata{}, fmt.Errorf("no choices")
	}

	reply := strings.TrimSpace(dr.Choices[0].Message.Content)
	reply = strings.TrimPrefix(reply, "```json")
	reply = strings.TrimPrefix(reply, "```")
	reply = strings.TrimSuffix(reply, "```")
	reply = strings.TrimSpace(reply)

	var out deepseekMetadata
	if err := json.Unmarshal([]byte(reply), &out); err != nil {
		return deepseekMetadata{}, err
	}
	return out, nil
}

// ----- Track upgrade (re-download with new pipeline) -----

type upgradeReq struct {
	TrackID int64 `json:"track_id"`
}

type upgradeAllReq struct {
	Limit int `json:"limit"` // max tracks to upgrade, 0 = all
}

// upgradeTrack re-downloads a single track using the new bestaudio pipeline.
// It deletes the old file, runs a search-based download, and updates the DB.
func (a *API) upgradeTrack(w http.ResponseWriter, r *http.Request) {
	if a.cfg.YtdlpBin == "" {
		http.Error(w, "yt-dlp not configured. Set YTDLP_BIN in backend/.env.", 400)
		return
	}
	var req upgradeReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		log.Printf("[downloader] upgradeTrack decode: %v", err)
		http.Error(w, err.Error(), 400)
		return
	}
	if req.TrackID <= 0 {
		log.Printf("[downloader] upgradeTrack: invalid track_id %d", req.TrackID)
		http.Error(w, "track_id is required", 400)
		return
	}

	// Look up the track
	var oldPath, title, artist string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT path, IFNULL(title,''), IFNULL(artist,'') FROM tracks WHERE id=? AND media_kind='music'`,
		req.TrackID).Scan(&oldPath, &title, &artist)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[downloader] upgradeTrack: track %d not found", req.TrackID)
			http.Error(w, "track not found", 404)
			return
		}
		log.Printf("[downloader] upgradeTrack: lookup track %d: %v", req.TrackID, err)
		http.Error(w, err.Error(), 500)
		return
	}

	// Build search query from title + artist
	query := strings.TrimSpace(title + " " + artist)
	if query == "" {
		query = strings.TrimSuffix(filepath.Base(oldPath), filepath.Ext(oldPath))
	}

	// Delete old file (best-effort)
	if oldPath != "" {
		if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
			log.Printf("[upgrade] failed to remove old file %s: %v", oldPath, err)
		}
	}

	// Enqueue a search-based download (uses the new bestaudio pipeline)
	job := &Job{
		ID:        uuid.NewString(),
		URL:       query,
		Output:    a.cfg.Output,
		Status:    StatusQueued,
		StartedAt: time.Now().Unix(),
		Log:       []string{},
		IsSearch:  true,
		Kind:      "music",
		TrackID:   req.TrackID,
	}
	a.mu.Lock()
	a.jobs[job.ID] = job
	a.order = append([]string{job.ID}, a.order...)
	if len(a.order) > a.maxKeep {
		for _, oldID := range a.order[a.maxKeep:] {
			delete(a.jobs, oldID)
		}
		a.order = a.order[:a.maxKeep]
	}
	a.mu.Unlock()

	if a.db != nil {
		_, _ = a.db.Exec(
			`INSERT INTO download_jobs(id, url, output, status, started_at, is_search, kind, track_id) VALUES(?, ?, ?, ?, ?, 1, ?, ?)`,
			job.ID, job.URL, job.Output, string(job.Status), job.StartedAt, job.Kind, job.TrackID)
	}

	go a.runSearchWithTrackID(job, req.TrackID, a.jobContext(r.Context()))
	writeJSON(w, map[string]interface{}{
		"job_id":  job.ID,
		"query":   query,
		"status":  "queued",
		"message": "Track upgrade queued. Poll /api/download/jobs/" + job.ID + " for status.",
	})
}

// runSearchWithTrackID is like runSearch but updates the track's path after download.
func (a *API) runSearchWithTrackID(job *Job, trackID int64, ctx context.Context) {
	// Run the standard search pipeline
	a.runSearch(job, ctx)

	// If successful, update the track's file path
	a.mu.Lock()
	status := job.Status
	a.mu.Unlock()

	if status != StatusSucceeded {
		log.Printf("[upgrade] job %s did not succeed (status=%s), skipping path update", job.ID, status)
		return
	}

	// Find the downloaded file
	dlFile := a.findDownloadedFile(time.Unix(job.StartedAt, 0))
	if dlFile == "" {
		log.Printf("[upgrade] job %s succeeded but could not find downloaded file", job.ID)
		return
	}

	// Update the track record with the new file path
	_, err := a.db.Exec(`UPDATE tracks SET path=?, mime=?, size_bytes=?, mtime=? WHERE id=?`,
		dlFile,
		audioMIME(dlFile),
		fileSize(dlFile),
		time.Now().Unix(),
		trackID)
	if err != nil {
		log.Printf("[upgrade] failed to update track %d path: %v", trackID, err)
	} else {
		log.Printf("[upgrade] track %d upgraded: %s", trackID, dlFile)
	}

	// Trigger rescan to pick up new metadata
	if a.rescan != nil {
		go a.rescan()
	}
}

// audioMIME returns the MIME type for a file based on its extension.
func audioMIME(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	m := map[string]string{
		".mp3":  "audio/mpeg",
		".flac": "audio/flac",
		".m4a":  "audio/mp4",
		".m4b":  "audio/mp4",
		".aac":  "audio/aac",
		".ogg":  "audio/ogg",
		".opus": "audio/opus",
		".wav":  "audio/wav",
		".webm": "audio/webm",
		".mp4":  "audio/mp4",
	}
	if mime, ok := m[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

// fileSize returns the file size in bytes, or 0 on error.
func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// upgradeAll enqueues all music tracks for upgrade.
func (a *API) upgradeAll(w http.ResponseWriter, r *http.Request) {
	var req upgradeAllReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		log.Printf("[downloader] upgradeAll decode: %v", err)
		http.Error(w, err.Error(), 400)
		return
	}

	// Get all music track IDs
	query := `SELECT id FROM tracks WHERE media_kind='music' ORDER BY id`
	rows, err := a.db.QueryContext(r.Context(), query)
	if err != nil {
		log.Printf("[downloader] upgradeAll query: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var trackIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			continue
		}
		trackIDs = append(trackIDs, id)
	}

	limit := req.Limit
	if limit <= 0 || limit > len(trackIDs) {
		limit = len(trackIDs)
	}

	writeJSON(w, map[string]interface{}{
		"total_tracks":   len(trackIDs),
		"upgrade_limit":  limit,
		"message":        fmt.Sprintf("Found %d tracks. Use POST /api/library/upgrade with {\"track_id\": N} for each track, or use the bulk upgrade endpoint.", len(trackIDs)),
		"track_ids":      trackIDs[:limit],
	})
}
