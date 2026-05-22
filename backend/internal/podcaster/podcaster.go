// Package podcaster manages podcast feed subscriptions and episode downloads.
// It uses gofeed for RSS parsing and poddl (external CLI) for downloading episodes.
// Downloaded episodes are indexed into the tracks table as media_kind='podcast'.
//
// Every download (single episode or bulk feed) registers a job with the
// downloader package so it appears on the unified Downloads page with full
// log streaming and error visibility.
package podcaster

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mmcdole/gofeed"

	"github.com/kevin/lexicon/internal/downloader"
)

// userAgent identifies HTTP requests to podcast CDNs. Some hosts (acast,
// buzzsprout, wistia) return 403 to Go's default "Go-http-client/1.1".
const userAgent = "Lexicon/1.0 (+podcast)"

// JobSink is the subset of *downloader.API used by the podcaster to record
// download progress in the unified job system. Defined as an interface so
// tests can stub it.
type JobSink interface {
	RegisterExternalJob(kind, url, output, tool string) string
	AppendExternalLog(id, line string)
	FinishExternalJob(id string, status downloader.Status, errMsg string)
}

type Config struct {
	PoddlBin     string
	OutputDir    string
	AutoDownload bool
}

type API struct {
	db             *sql.DB
	cfg            Config
	rescan         func()
	jobs           JobSink
	sema           chan struct{}     // max 3 concurrent downloads + shutdown drain
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	inflight       atomic.Int64      // in-flight download count for status API
}

// New constructs a podcaster API. `jobs` may be nil — in which case downloads
// still work but won't appear on the Downloads page. Pass *downloader.API.
func New(db *sql.DB, cfg Config, rescan func(), jobs JobSink) *API {
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	return &API{db: db, cfg: cfg, rescan: rescan, jobs: jobs, shutdownCtx: shutdownCtx, shutdownCancel: shutdownCancel, sema: make(chan struct{}, 3)}
}

func (a *API) Mount(r chi.Router) {
	r.Get("/api/podcasts/feeds", a.listFeeds)
	r.Post("/api/podcasts/subscribe", a.subscribe)
	r.Delete("/api/podcasts/feeds/{id}", a.unsubscribe)
	r.Get("/api/podcasts/feeds/{id}/episodes", a.listEpisodes)
	r.Post("/api/podcasts/feeds/{id}/sync", a.syncFeed)
	r.Post("/api/podcasts/episodes/{id}/download", a.downloadEpisode)
	r.Post("/api/podcasts/feeds/{id}/download", a.downloadFeed)
	r.Post("/api/podcasts/episodes/{id}/position", a.saveEpisodePosition)
	r.Get("/api/podcasts/episodes/{id}/position", a.getEpisodePosition)
	r.Get("/api/podcasts/status", a.status)
	r.Get("/api/podcasts/episodes/{id}/track", a.episodeTrack)
}

// Shutdown signals all in-flight podcaster goroutines to finish gracefully.
// It first waits up to 30 seconds for downloads to complete on their own,
// then cancels the shutdown context to force-terminate any remaining.
// This matches the music downloader's semaphore drain pattern (downloader.go
// Shutdown()) but adds a grace period before cancellation since podcast files
// are 10-20x larger than music files.
func (a *API) Shutdown() {
	done := make(chan struct{})
	go func() {
		// Acquire ALL semaphore slots — blocks until every in-flight goroutine
		// has released its slot (i.e., completed or errored out).
		for i := 0; i < cap(a.sema); i++ {
			a.sema <- struct{}{}
		}
		close(done)
	}()
	select {
	case <-done:
		log.Printf("[podcaster] all downloads completed before shutdown")
	case <-time.After(30 * time.Second):
		log.Printf("[podcaster] shutdown: 30s grace period expired, cancelling %d remaining downloads",
			a.inflight.Load())
		a.shutdownCancel()
		// Drain semaphore after cancellation so Shutdown() returns.
		for i := 0; i < cap(a.sema); i++ {
			a.sema <- struct{}{}
		}
	}
}

// jobContext returns the shutdown context for fire-and-forget goroutines.
// Downloads and feed syncs outlive the HTTP request, so using the request
// context would cancel them when the handler returns. Only the shutdown
// context is used so they run to completion. The reqCtx parameter is kept
// for API compatibility but is intentionally not used.
// Pass this to doSyncFeed / doDownloadEpisode / doDownloadFeed.
func (a *API) jobContext(reqCtx context.Context) context.Context {
	// Downloads and feed syncs run in fire-and-forget goroutines that outlive
	// the HTTP request. Use only the shutdown context so they aren't killed
	// when the handler returns. The reqCtx parameter is kept for API
	// compatibility but is intentionally not used.
	return a.shutdownCtx
}

// ----- Types returned to frontend -----

type FeedJSON struct {
	ID             int64  `json:"id"`
	URL            string `json:"url"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	ImageURL       string `json:"image_url"`
	Author         string `json:"author"`
	EpisodeCount   int    `json:"episode_count"`
	DownloadedCount int   `json:"downloaded_count"`
	LastFetchedAt  int64  `json:"last_fetched_at"`
	AutoDownload   bool   `json:"auto_download"`
}

type EpisodeJSON struct {
	ID                int64  `json:"id"`
	FeedID            int64  `json:"feed_id"`
	GUID              string `json:"guid"`
	Title             string `json:"title"`
	Description       string `json:"description"`
	PubDate           int64  `json:"pub_date"`
	DurationSec       int    `json:"duration_sec"`
	AudioURL          string `json:"audio_url"`
	Downloaded        bool   `json:"downloaded"`
	FilePath          string `json:"file_path"`
	DownloadError     string `json:"download_error"`
	PlaybackPositionSec int  `json:"playback_position_sec"`
	Listened          bool   `json:"listened"`
}

// ----- Helpers -----

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[podcaster] writeJSON encode: %v", err)
	}
}

// jobLog appends a line to the unified job log if a sink is configured.
// No-ops when jobID is empty (e.g. background sync).
func (a *API) jobLog(jobID, line string) {
	if a.jobs == nil || jobID == "" {
		return
	}
	a.jobs.AppendExternalLog(jobID, line)
}

// invalidFilenameChars are characters Windows disallows in filenames, plus
// path separators and any control characters.
var invalidFilenameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

// sanitizeFilename strips path separators and Windows-illegal characters
// and truncates to 80 chars to keep the full path safely under MAX_PATH.
func sanitizeFilename(s string) string {
	s = invalidFilenameChars.ReplaceAllString(s, "_")
	s = strings.TrimSpace(s)
	s = strings.Trim(s, ".") // Windows trailing-dot quirk
	if len(s) > 80 {
		s = s[:80]
	}
	if s == "" {
		s = "episode"
	}
	return s
}

// episodeFilename builds a unique on-disk filename for an episode.
// Format: <feedID>-<episodeID>-<sanitizedTitle>.<ext>
// This guarantees uniqueness across feeds and episodes, even when audio
// URLs share the same basename (e.g. cdn.example.com/audio.mp3).
func episodeFilename(feedID, episodeID int64, title, audioURL, audioType string) string {
	ext := guessAudioExt(audioURL, audioType)
	clean := sanitizeFilename(title)
	return fmt.Sprintf("%d-%d-%s%s", feedID, episodeID, clean, ext)
}

// guessAudioExt picks a sensible file extension. Prefers the URL path's
// extension, falls back to the MIME type, defaults to .mp3.
func guessAudioExt(audioURL, audioType string) string {
	if u, err := url.Parse(audioURL); err == nil {
		if ext := strings.ToLower(filepath.Ext(u.Path)); ext != "" {
			switch ext {
			case ".mp3", ".m4a", ".m4b", ".aac", ".ogg", ".opus", ".wav", ".flac", ".mp4", ".webm":
				return ext
			}
		}
	}
	switch strings.ToLower(audioType) {
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/mp4", "audio/x-m4a":
		return ".m4a"
	case "audio/aac":
		return ".aac"
	case "audio/ogg":
		return ".ogg"
	case "audio/opus":
		return ".opus"
	case "audio/flac", "audio/x-flac":
		return ".flac"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	}
	return ".mp3"
}

// isPrivateHost returns true if the hostname resolves to a private, loopback,
// or link-local IP address. Used to block SSRF via the subscribe endpoint.
func isPrivateHost(host string) bool {
	// Strip port if present.
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	// Handle literal IPv6 brackets.
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	ips, err := net.LookupIP(host)
	if err != nil {
		// If we can't resolve, check if it looks like a private literal IP.
		ip := net.ParseIP(host)
		if ip == nil {
			// Can't parse and can't resolve — block to be safe.
			return true
		}
		ips = []net.IP{ip}
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate() {
			return true
		}
	}
	return false
}

// ----- Endpoints -----

func (a *API) status(w http.ResponseWriter, _ *http.Request) {
	available := a.cfg.PoddlBin != ""
	writeJSON(w, map[string]interface{}{
		"available":          available,
		"bin":                a.cfg.PoddlBin,
		"inflight_downloads": a.inflight.Load(),
	})
}

// episodeTrack looks up the library track record for a podcast episode.
// This bridges the podcast_episodes table to the tracks table so the
// frontend player (which needs a track ID) can play downloaded episodes.
func (a *API) episodeTrack(w http.ResponseWriter, r *http.Request) {
	episodeID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if episodeID <= 0 {
		http.Error(w, "invalid id", 400)
		return
	}

	// Get the episode's file_path
	var filePath sql.NullString
	err := a.db.QueryRowContext(r.Context(),
		`SELECT file_path FROM podcast_episodes WHERE id=?`, episodeID).Scan(&filePath)
	if err != nil {
		http.Error(w, "episode not found", 404)
		return
	}
	if !filePath.Valid || filePath.String == "" {
		http.Error(w, "episode not downloaded yet", 400)
		return
	}

	// Look up the track by path
	var trackID int64
	err = a.db.QueryRowContext(r.Context(),
		`SELECT id FROM tracks WHERE path=?`, filePath.String).Scan(&trackID)
	if err != nil {
		http.Error(w, "track not found in library — rescan may be in progress", 404)
		return
	}

	writeJSON(w, map[string]interface{}{"track_id": trackID})
}

// ----- Playback position tracking -----

type positionReq struct {
	PositionSec int  `json:"position_sec"`
	Completed   bool `json:"completed"`
}

// saveEpisodePosition saves the current playback position for a podcast episode.
func (a *API) saveEpisodePosition(w http.ResponseWriter, r *http.Request) {
	episodeID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if episodeID <= 0 {
		http.Error(w, "invalid id", 400)
		return
	}
	var req positionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}

	// Auto-mark as listened if completed or position > 90% of duration
	listened := 0
	if req.Completed {
		listened = 1
	} else {
		var durationSec sql.NullInt64
		err := a.db.QueryRowContext(r.Context(),
			`SELECT duration_sec FROM podcast_episodes WHERE id=?`, episodeID).Scan(&durationSec)
		if err == nil && durationSec.Valid && durationSec.Int64 > 0 {
			if float64(req.PositionSec) >= float64(durationSec.Int64)*0.9 {
				listened = 1
			}
		}
	}

	_, err := a.db.ExecContext(r.Context(),
		`UPDATE podcast_episodes SET playback_position_sec=?, listened=? WHERE id=?`,
		req.PositionSec, listened, episodeID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// getEpisodePosition returns the saved playback position for a podcast episode.
func (a *API) getEpisodePosition(w http.ResponseWriter, r *http.Request) {
	episodeID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if episodeID <= 0 {
		http.Error(w, "invalid id", 400)
		return
	}
	var positionSec int
	var listened int
	err := a.db.QueryRowContext(r.Context(),
		`SELECT playback_position_sec, listened FROM podcast_episodes WHERE id=?`, episodeID).Scan(&positionSec, &listened)
	if err != nil {
		http.Error(w, "episode not found", 404)
		return
	}
	writeJSON(w, map[string]interface{}{
		"position_sec": positionSec,
		"listened":     listened == 1,
	})
}

type subscribeReq struct {
	URL string `json:"url"`
}

func (a *API) subscribe(w http.ResponseWriter, r *http.Request) {
	var req subscribeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	feedURL := strings.TrimSpace(req.URL)
	if feedURL == "" {
		http.Error(w, "url is required", 400)
		return
	}

	// Validate URL scheme and block private/internal IP ranges (SSRF protection).
	parsed, err := url.Parse(feedURL)
	if err != nil || !parsed.IsAbs() || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		http.Error(w, "invalid URL: must be an absolute http or https URL", 400)
		return
	}
	if isPrivateHost(parsed.Hostname()) {
		http.Error(w, "invalid URL: private/internal IP addresses are not allowed", 400)
		return
	}

	// Fetch and parse the feed using a client with timeout and redirect limit.
	fp := gofeed.NewParser()
	fp.Client = &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects (max 3)")
			}
			return nil
		},
	}
	feed, err := fp.ParseURL(feedURL)
	if err != nil {
		http.Error(w, "failed to fetch/parse RSS feed: "+err.Error(), 400)
		return
	}

	imageURL := ""
	if feed.Image != nil {
		imageURL = feed.Image.URL
	}

	// gofeed.Person is a struct — extract the name string
	authorName := ""
	if feed.Author != nil {
		authorName = feed.Author.Name
	}

	// Insert feed
	res, err := a.db.ExecContext(r.Context(),
		`INSERT OR IGNORE INTO podcast_feeds(url, title, description, image_url, author, link, language, last_fetched_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		feedURL, feed.Title, feed.Description, imageURL, authorName, feed.Link, feed.Language, time.Now().Unix())
	if err != nil {
		http.Error(w, "db error: "+err.Error(), 500)
		return
	}

	feedID, _ := res.LastInsertId()
	if feedID == 0 {
		// Already existed — fetch the ID
		var existingID int64
		err := a.db.QueryRowContext(r.Context(), `SELECT id FROM podcast_feeds WHERE url=?`, feedURL).Scan(&existingID)
		if err != nil {
			http.Error(w, "db error: "+err.Error(), 500)
			return
		}
		feedID = existingID
		// Update metadata
		a.db.ExecContext(r.Context(),
			`UPDATE podcast_feeds SET title=?, description=?, image_url=?, author=?, link=?, language=?, last_fetched_at=? WHERE id=?`,
			feed.Title, feed.Description, imageURL, authorName, feed.Link, feed.Language, time.Now().Unix(), feedID)
	}

	// Insert episodes
	now := time.Now().Unix()
	for _, item := range feed.Items {
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}
		if guid == "" {
			continue
		}

		pubDate := int64(0)
		if item.PublishedParsed != nil {
			pubDate = item.PublishedParsed.Unix()
		}

		durationSec := 0
		audioURL := ""
		audioType := ""
		audioSize := 0
		for _, enc := range item.Enclosures {
			if strings.HasPrefix(enc.Type, "audio/") {
				audioURL = enc.URL
				audioType = enc.Type
				audioSize, _ = strconv.Atoi(enc.Length)
				break
			}
		}
		// Also check for media:content
		if audioURL == "" && item.Extensions != nil {
			if media, ok := item.Extensions["media"]; ok {
				if content, ok := media["content"]; ok && len(content) > 0 {
					audioURL = content[0].Attrs["url"]
					audioType = content[0].Attrs["type"]
				}
			}
		}

		_, _ = a.db.ExecContext(r.Context(),
			`INSERT OR IGNORE INTO podcast_episodes(feed_id, guid, title, description, pub_date, duration_sec, audio_url, audio_type, audio_size, created_at)
			 VALUES(?,?,?,?,?,?,?,?,?,?)`,
			feedID, guid, item.Title, item.Description, pubDate, durationSec, audioURL, audioType, audioSize, now)
	}

	// Update last_fetched_at
	a.db.ExecContext(r.Context(), `UPDATE podcast_feeds SET last_fetched_at=? WHERE id=?`, now, feedID)

	log.Printf("[podcaster] subscribed to feed %d: %s (%d episodes)", feedID, feed.Title, len(feed.Items))
	writeJSON(w, map[string]interface{}{"id": feedID, "title": feed.Title, "episodes": len(feed.Items)})
}

func (a *API) unsubscribe(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 {
		http.Error(w, "invalid id", 400)
		return
	}
	// Delete episodes first (cascade should handle it, but be explicit)
	a.db.ExecContext(r.Context(), `DELETE FROM podcast_episodes WHERE feed_id=?`, id)
	_, err := a.db.ExecContext(r.Context(), `DELETE FROM podcast_feeds WHERE id=?`, id)
	if err != nil {
		http.Error(w, "db error: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *API) listFeeds(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT f.id, f.url, f.title, f.description, f.image_url, f.author,
		        COUNT(e.id) as episode_count,
		        SUM(CASE WHEN e.downloaded=1 THEN 1 ELSE 0 END) as downloaded_count,
		        f.last_fetched_at, f.auto_download
		 FROM podcast_feeds f
		 LEFT JOIN podcast_episodes e ON e.feed_id = f.id
		 GROUP BY f.id
		 ORDER BY f.created_at DESC`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var feeds = []FeedJSON{}
	for rows.Next() {
		var f FeedJSON
		var lastFetched sql.NullInt64
		if err := rows.Scan(&f.ID, &f.URL, &f.Title, &f.Description, &f.ImageURL, &f.Author,
			&f.EpisodeCount, &f.DownloadedCount, &lastFetched, &f.AutoDownload); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if lastFetched.Valid {
			f.LastFetchedAt = lastFetched.Int64
		}
		feeds = append(feeds, f)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, feeds)
}

func (a *API) listEpisodes(w http.ResponseWriter, r *http.Request) {
	feedID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if feedID <= 0 {
		http.Error(w, "invalid id", 400)
		return
	}
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT id, feed_id, guid, title, description, pub_date, duration_sec, audio_url, downloaded, file_path, download_error, playback_position_sec, listened
		 FROM podcast_episodes WHERE feed_id=? ORDER BY pub_date DESC`, feedID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var episodes = []EpisodeJSON{}
	for rows.Next() {
		var e EpisodeJSON
		var pubDate sql.NullInt64
		var durationSec sql.NullInt64
		var filePath sql.NullString
		var downloadError sql.NullString
		var playbackPositionSec sql.NullInt64
		var listened int
		if err := rows.Scan(&e.ID, &e.FeedID, &e.GUID, &e.Title, &e.Description, &pubDate, &durationSec, &e.AudioURL, &e.Downloaded, &filePath, &downloadError, &playbackPositionSec, &listened); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if pubDate.Valid {
			e.PubDate = pubDate.Int64
		}
		if durationSec.Valid {
			e.DurationSec = int(durationSec.Int64)
		}
		if filePath.Valid {
			e.FilePath = filePath.String
		}
		if downloadError.Valid {
			e.DownloadError = downloadError.String
		}
		if playbackPositionSec.Valid {
			e.PlaybackPositionSec = int(playbackPositionSec.Int64)
		}
		e.Listened = listened == 1
		episodes = append(episodes, e)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, episodes)
}

func (a *API) syncFeed(w http.ResponseWriter, r *http.Request) {
	feedID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if feedID <= 0 {
		http.Error(w, "invalid id", 400)
		return
	}

	var feedURL string
	err := a.db.QueryRowContext(r.Context(), `SELECT url FROM podcast_feeds WHERE id=?`, feedID).Scan(&feedURL)
	if err != nil {
		http.Error(w, "feed not found", 404)
		return
	}

	a.sema <- struct{}{}
	a.inflight.Add(1)
	go func() {
		defer func() { <-a.sema }()
		defer a.inflight.Add(-1)
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[podcaster] PANIC in syncFeed(feedID=%d): %v\n%s", feedID, r, debug.Stack())
			}
		}()
		if err := a.doSyncFeed(a.jobContext(r.Context()), feedID, feedURL); err != nil {
			log.Printf("[podcaster] sync feed %d error: %v", feedID, err)
		}
	}()

	writeJSON(w, map[string]bool{"ok": true})
}

func (a *API) doSyncFeed(ctx context.Context, feedID int64, feedURL string) error {
	if feedURL == "" {
		return fmt.Errorf("feed URL is empty")
	}
	u, err := url.Parse(feedURL)
	if err != nil || !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("invalid feed URL: %s", feedURL)
	}

	fp := gofeed.NewParser()
	fp.Client = &http.Client{
		Timeout: 30 * time.Second,
	}
	feed, err := fp.ParseURL(feedURL)
	if err != nil {
		a.db.ExecContext(ctx, `UPDATE podcast_feeds SET last_error=? WHERE id=?`, err.Error(), feedID)
		return err
	}

	imageURL := ""
	if feed.Image != nil {
		imageURL = feed.Image.URL
	}
	now := time.Now().Unix()

	authorName := ""
	if feed.Author != nil {
		authorName = feed.Author.Name
	}

	a.db.ExecContext(ctx, `UPDATE podcast_feeds SET title=?, description=?, image_url=?, author=?, last_fetched_at=?, last_error='' WHERE id=?`,
		feed.Title, feed.Description, imageURL, authorName, now, feedID)

	for _, item := range feed.Items {
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}
		if guid == "" {
			continue
		}

		pubDate := int64(0)
		if item.PublishedParsed != nil {
			pubDate = item.PublishedParsed.Unix()
		}

		audioURL := ""
		audioType := ""
		audioSize := 0
		for _, enc := range item.Enclosures {
			if strings.HasPrefix(enc.Type, "audio/") {
				audioURL = enc.URL
				audioType = enc.Type
				audioSize, _ = strconv.Atoi(enc.Length)
				break
			}
		}

		_, _ = a.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO podcast_episodes(feed_id, guid, title, description, pub_date, audio_url, audio_type, audio_size, created_at)
			 VALUES(?,?,?,?,?,?,?,?,?)`,
			feedID, guid, item.Title, item.Description, pubDate, audioURL, audioType, audioSize, now)
	}

	return nil
}

func (a *API) downloadEpisode(w http.ResponseWriter, r *http.Request) {
	episodeID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if episodeID <= 0 {
		http.Error(w, "invalid id", 400)
		return
	}

	// Get episode info
	var feedID int64
	var audioURL, audioType, episodeTitle string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT feed_id, IFNULL(audio_url,''), IFNULL(audio_type,''), IFNULL(title,'')
		 FROM podcast_episodes WHERE id=?`, episodeID).Scan(&feedID, &audioURL, &audioType, &episodeTitle)
	if err != nil {
		http.Error(w, "episode not found", 404)
		return
	}

	var feedURL, feedTitle string
	err = a.db.QueryRowContext(r.Context(),
		`SELECT url, IFNULL(title,'') FROM podcast_feeds WHERE id=?`, feedID).Scan(&feedURL, &feedTitle)
	if err != nil {
		http.Error(w, "feed not found", 404)
		return
	}

	// poddl is only required when no direct audio URL is available.
	if audioURL == "" && a.cfg.PoddlBin == "" {
		http.Error(w, "this episode has no direct audio URL and poddl is not configured. Set PODDL_BIN in backend/.env. Download from https://github.com/freshe/poddl", 400)
		return
	}

	a.sema <- struct{}{}
	a.inflight.Add(1)
	go func() {
		defer func() { <-a.sema }()
		defer a.inflight.Add(-1)
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[podcaster] PANIC in downloadEpisode(episodeID=%d): %v\n%s", episodeID, r, debug.Stack())
			}
		}()
		a.doDownloadEpisode(a.jobContext(r.Context()), episodeID, feedID, feedURL, feedTitle, audioURL, audioType, episodeTitle)
	}()

	writeJSON(w, map[string]bool{"ok": true})
}

func (a *API) doDownloadEpisode(ctx context.Context, episodeID, feedID int64, feedURL, feedTitle, audioURL, audioType, episodeTitle string) {
	outputDir := a.cfg.OutputDir
	if outputDir == "" {
		outputDir = "."
	}

	// Ensure the output directory exists. os.Create() fails if any parent
	// dir is missing, so this MkdirAll is essential — the installer creates
	// the dir but nothing creates per-feed subdirs if we ever add them.
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Printf("[podcaster] mkdir %s: %v", outputDir, err)
	}

	// Register a job for the unified Downloads page.
	jobLabel := episodeTitle
	if feedTitle != "" {
		jobLabel = feedTitle + " — " + episodeTitle
	}
	if jobLabel == "" {
		jobLabel = fmt.Sprintf("episode #%d", episodeID)
	}
	tool := "http"
	if audioURL == "" {
		tool = "poddl"
	}
	jobID := ""
	if a.jobs != nil {
		jobID = a.jobs.RegisterExternalJob("podcast", jobLabel, outputDir, tool)
	}

	// poddl expects an RSS feed URL — when we have a direct audio URL (from
	// the episode enclosure), use HTTP. Only fall back to poddl when no
	// direct URL is available.
	if audioURL != "" {
		a.downloadDirectAudio(ctx, jobID, episodeID, feedID, audioURL, audioType, episodeTitle, outputDir)
	} else {
		a.downloadViaPoddl(ctx, jobID, episodeID, feedURL, outputDir)
	}
}

// downloadDirectAudio downloads a direct audio URL via HTTP.
// poddl only accepts RSS feed URLs, so we handle direct downloads ourselves.
func (a *API) downloadDirectAudio(ctx context.Context, jobID string, episodeID, feedID int64, audioURL, audioType, episodeTitle, outputDir string) {
	a.jobLog(jobID, fmt.Sprintf("[http] GET %s", audioURL))

	parsed, err := url.Parse(audioURL)
	if err != nil {
		msg := "invalid URL: " + err.Error()
		a.recordEpisodeError(jobID, episodeID, msg)
		return
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		msg := fmt.Sprintf("invalid URL scheme %q: must be http or https", parsed.Scheme)
		a.recordEpisodeError(jobID, episodeID, msg)
		return
	}

	filename := episodeFilename(feedID, episodeID, episodeTitle, audioURL, audioType)
	outputPath := filepath.Join(outputDir, filename)

	// Build request with proper User-Agent. Some podcast CDNs (acast,
	// buzzsprout) return 403 to Go's default UA.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", audioURL, nil)
	if err != nil {
		a.recordEpisodeError(jobID, episodeID, "request build failed: "+err.Error())
		return
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "audio/*, */*;q=0.5")

	// Context already has 30-min timeout (line 825) — no need for Client.Timeout.
	// Go's setRequestCancel skips nested context creation when deadlines match.
	// Dedicated transport isolates podcast downloads from DefaultTransport pooling.
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   60 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   30 * time.Second,
			ResponseHeaderTimeout: 5 * time.Minute,
			MaxIdleConns:          10,
			IdleConnTimeout:       30 * time.Second,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		a.recordEpisodeError(jobID, episodeID, "download failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	a.jobLog(jobID, fmt.Sprintf("[http] HTTP %d %s — Content-Type=%q Content-Length=%s",
		resp.StatusCode, resp.Status, resp.Header.Get("Content-Type"), resp.Header.Get("Content-Length")))

	if resp.StatusCode != http.StatusOK {
		// Capture a snippet of the body so the user can see what the server
		// actually said (often "access denied", a JSON error, or HTML).
		bodyPreview, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		preview := strings.TrimSpace(string(bodyPreview))
		preview = strings.ReplaceAll(preview, "\n", " ")
		preview = strings.ReplaceAll(preview, "\r", " ")
		if preview == "" {
			preview = "(empty body)"
		}
		msg := fmt.Sprintf("HTTP %d %s: %s", resp.StatusCode, resp.Status, preview)
		a.recordEpisodeError(jobID, episodeID, msg)
		return
	}

	f, err := os.Create(outputPath)
	if err != nil {
		a.recordEpisodeError(jobID, episodeID, "file create failed: "+err.Error())
		return
	}
	// Cap response body at 2 GB — generous for any real podcast, prevents
	// disk exhaustion from malicious/misconfigured CDNs.
	const maxBodySize = 2 << 30 // 2 GB
	written, copyErr := io.Copy(f, io.LimitReader(resp.Body, maxBodySize))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(outputPath)
		a.recordEpisodeError(jobID, episodeID, "write failed: "+copyErr.Error())
		return
	}
	if written >= maxBodySize {
		_ = os.Remove(outputPath)
		a.recordEpisodeError(jobID, episodeID,
			fmt.Sprintf("response body exceeded %d byte limit — download aborted", maxBodySize))
		return
	}
	if closeErr != nil {
		a.jobLog(jobID, "[http] warning: close error: "+closeErr.Error())
	}

	if written < 10*1024 {
		_ = os.Remove(outputPath)
		a.recordEpisodeError(jobID, episodeID, fmt.Sprintf("downloaded file too small (%d bytes) — likely an error page", written))
		return
	}

	a.jobLog(jobID, fmt.Sprintf("[http] wrote %s (%d bytes)", outputPath, written))
	log.Printf("[podcaster] downloaded episode %d: %s (%d bytes)", episodeID, outputPath, written)

	// Success: clear any stale error, mark as downloaded.
	if _, err := a.db.Exec(`UPDATE podcast_episodes SET downloaded=1, file_path=?, file_size=?, download_error=NULL WHERE id=?`,
		outputPath, written, episodeID); err != nil {
		a.jobLog(jobID, "[db] update failed: "+err.Error())
	}
	if a.jobs != nil && jobID != "" {
		a.jobs.FinishExternalJob(jobID, downloader.StatusSucceeded, "")
	}
	if a.rescan != nil {
		go a.rescan()
	}
}

// recordEpisodeError persists an error to both the unified job log and the
// per-episode `download_error` column, then finishes the job as failed.
func (a *API) recordEpisodeError(jobID string, episodeID int64, msg string) {
	log.Printf("[podcaster] episode %d failed: %s", episodeID, msg)
	a.jobLog(jobID, "[error] "+msg)
	if _, err := a.db.Exec(`UPDATE podcast_episodes SET download_error=? WHERE id=?`, msg, episodeID); err != nil {
		log.Printf("[podcaster] episode %d: db update failed: %v", episodeID, err)
	}
	if a.jobs != nil && jobID != "" {
		a.jobs.FinishExternalJob(jobID, downloader.StatusFailed, msg)
	}
}

// downloadViaPoddl uses poddl to download from an RSS feed URL.
// Used as fallback when no direct audio URL is available.
func (a *API) downloadViaPoddl(ctx context.Context, jobID string, episodeID int64, feedURL, outputDir string) {
	if a.cfg.PoddlBin == "" {
		a.recordEpisodeError(jobID, episodeID, "poddl is not configured")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	args := []string{feedURL, "-o", outputDir, "-r", "-t", "1"}
	a.jobLog(jobID, fmt.Sprintf("[poddl] %s %s", a.cfg.PoddlBin, strings.Join(args, " ")))

	// Capture stdout+stderr line-by-line into the job log so the user sees
	// poddl's actual messages instead of just "exit status 0xffffffff".
	cmd := exec.CommandContext(ctx, a.cfg.PoddlBin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		a.recordEpisodeError(jobID, episodeID, "poddl stdout pipe: "+err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		a.recordEpisodeError(jobID, episodeID, "poddl stderr pipe: "+err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		a.recordEpisodeError(jobID, episodeID, "poddl start: "+err.Error())
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a.streamProcessOutput(jobID, "poddl", stdout) }()
	go func() { defer wg.Done(); a.streamProcessOutput(jobID, "poddl", stderr) }()
	wg.Wait()
	waitErr := cmd.Wait()

	if waitErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			a.recordEpisodeError(jobID, episodeID, "poddl timed out after 5 minutes")
		} else {
			a.recordEpisodeError(jobID, episodeID, "poddl: "+waitErr.Error())
		}
		return
	}

	// Find the downloaded file
	filePath := a.findLatestAudioFile(outputDir)
	if filePath == "" {
		a.recordEpisodeError(jobID, episodeID, "poddl exited cleanly but no audio file was found")
		return
	}
	info, _ := os.Stat(filePath)
	fileSize := int64(0)
	if info != nil {
		fileSize = info.Size()
	}
	if _, err := a.db.Exec(`UPDATE podcast_episodes SET downloaded=1, file_path=?, file_size=?, download_error=NULL WHERE id=?`,
		filePath, fileSize, episodeID); err != nil {
		a.jobLog(jobID, "[db] update failed: "+err.Error())
	}
	a.jobLog(jobID, fmt.Sprintf("[poddl] downloaded %s (%d bytes)", filePath, fileSize))
	log.Printf("[podcaster] downloaded episode %d via poddl: %s", episodeID, filePath)

	if a.jobs != nil && jobID != "" {
		a.jobs.FinishExternalJob(jobID, downloader.StatusSucceeded, "")
	}
	if a.rescan != nil {
		go a.rescan()
	}
}

// streamProcessOutput consumes a subprocess's pipe and forwards each line
// into the unified job log with a prefix so the user sees real-time output.
func (a *API) streamProcessOutput(jobID, prefix string, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		a.jobLog(jobID, fmt.Sprintf("[%s] %s", prefix, sc.Text()))
	}
}

func (a *API) downloadFeed(w http.ResponseWriter, r *http.Request) {
	feedID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if feedID <= 0 {
		http.Error(w, "invalid id", 400)
		return
	}

	if a.cfg.PoddlBin == "" {
		http.Error(w, "poddl not configured. Set PODDL_BIN in backend/.env to the path of poddl.exe. Download from https://github.com/freshe/poddl", 400)
		return
	}

	var feedURL, feedTitle string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT url, IFNULL(title,'') FROM podcast_feeds WHERE id=?`, feedID).Scan(&feedURL, &feedTitle)
	if err != nil {
		http.Error(w, "feed not found", 404)
		return
	}

	a.sema <- struct{}{}
	a.inflight.Add(1)
	go func() {
		defer func() { <-a.sema }()
		defer a.inflight.Add(-1)
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[podcaster] PANIC in downloadFeed(feedID=%d): %v\n%s", feedID, r, debug.Stack())
			}
		}()
		a.doDownloadFeed(a.jobContext(r.Context()), feedID, feedURL, feedTitle)
	}()

	writeJSON(w, map[string]bool{"ok": true})
}

func (a *API) doDownloadFeed(ctx context.Context, feedID int64, feedURL, feedTitle string) {
	outputDir := a.cfg.OutputDir
	if outputDir == "" {
		outputDir = "."
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Printf("[podcaster] mkdir %s: %v", outputDir, err)
	}

	// Register a unified job for the bulk feed download.
	jobLabel := feedTitle
	if jobLabel == "" {
		jobLabel = fmt.Sprintf("feed #%d (bulk)", feedID)
	} else {
		jobLabel += " (bulk)"
	}
	jobID := ""
	if a.jobs != nil {
		jobID = a.jobs.RegisterExternalJob("podcast", jobLabel, outputDir, "poddl")
	}
	a.jobLog(jobID, fmt.Sprintf("[poddl] bulk download for feed %d (%s)", feedID, feedURL))

	finishFail := func(msg string) {
		log.Printf("[podcaster] feed %d: %s", feedID, msg)
		a.jobLog(jobID, "[error] "+msg)
		a.db.Exec(`UPDATE podcast_feeds SET last_error=? WHERE id=?`, msg, feedID)
		if a.jobs != nil && jobID != "" {
			a.jobs.FinishExternalJob(jobID, downloader.StatusFailed, msg)
		}
	}

	// Record timestamp before download so we can identify which files are new
	snapshotTime := time.Now()

	// Download all episodes from the feed, newest first.
	// -h flag: quit when first existing file is found (efficient incremental downloads).
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	args := []string{feedURL, "-o", outputDir, "-r", "-h"}
	a.jobLog(jobID, fmt.Sprintf("[poddl] %s %s", a.cfg.PoddlBin, strings.Join(args, " ")))

	cmd := exec.CommandContext(ctx, a.cfg.PoddlBin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		finishFail("poddl stdout pipe: " + err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		finishFail("poddl stderr pipe: " + err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		finishFail("poddl start: " + err.Error())
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a.streamProcessOutput(jobID, "poddl", stdout) }()
	go func() { defer wg.Done(); a.streamProcessOutput(jobID, "poddl", stderr) }()
	wg.Wait()
	waitErr := cmd.Wait()

	if waitErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			finishFail("poddl timed out after 30 minutes")
		} else {
			finishFail("poddl: " + waitErr.Error())
		}
		return
	}

	// Find all audio files created since the snapshot, sorted by mtime DESC (newest first).
	// poddl downloads newest-first (-r), so mtime DESC matches the download order.
	newFiles := a.findNewAudioFiles(outputDir, snapshotTime)
	if len(newFiles) == 0 {
		// Not a hard failure — could mean every episode was already downloaded
		// (the -h flag tells poddl to stop on the first existing file).
		a.jobLog(jobID, "[poddl] no new files (everything already downloaded?)")
		a.db.Exec(`UPDATE podcast_feeds SET last_error='' WHERE id=?`, feedID)
		if a.jobs != nil && jobID != "" {
			a.jobs.FinishExternalJob(jobID, downloader.StatusSucceeded, "")
		}
		return
	}

	// Get undownloaded episodes sorted by pub_date DESC (newest first) to match poddl's -r order.
	rows, err := a.db.Query(`SELECT id FROM podcast_episodes WHERE feed_id=? AND downloaded=0 ORDER BY pub_date DESC`, feedID)
	if err != nil {
		finishFail("query error: " + err.Error())
		return
	}
	defer rows.Close()

	var episodeIDs []int64
	for rows.Next() {
		var epID int64
		if err := rows.Scan(&epID); err != nil {
			continue
		}
		episodeIDs = append(episodeIDs, epID)
	}

	// Match files to episodes positionally: newest file → newest episode.
	n := len(newFiles)
	if len(episodeIDs) < n {
		n = len(episodeIDs)
	}
	for i := 0; i < n; i++ {
		filePath := newFiles[i].path
		fileSize := newFiles[i].size
		epID := episodeIDs[i]
		a.db.Exec(`UPDATE podcast_episodes SET downloaded=1, file_path=?, file_size=?, download_error=NULL WHERE id=?`,
			filePath, fileSize, epID)
		a.jobLog(jobID, fmt.Sprintf("[match] episode %d → %s", epID, filepath.Base(filePath)))
	}

	if a.rescan != nil {
		go a.rescan()
	}

	msg := fmt.Sprintf("downloaded %d episodes", n)
	a.jobLog(jobID, "[poddl] "+msg)
	log.Printf("[podcaster] feed %d: %s", feedID, msg)
	a.db.Exec(`UPDATE podcast_feeds SET last_error='' WHERE id=?`, feedID)
	if a.jobs != nil && jobID != "" {
		a.jobs.FinishExternalJob(jobID, downloader.StatusSucceeded, "")
	}
}

// fileInfo holds a downloaded file's path, size, and mtime.
type fileInfo struct {
	path    string
	size    int64
	modTime time.Time
}

// findNewAudioFiles returns all audio files in dir created at or after snapshotTime,
// sorted by modification time descending (newest first).
func (a *API) findNewAudioFiles(dir string, snapshotTime time.Time) []fileInfo {
	var files []fileInfo
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3", ".m4a", ".m4b", ".ogg", ".opus", ".flac", ".aac", ".wav", ".mp4", ".webm":
		if !info.ModTime().Before(snapshotTime) {
				files = append(files, fileInfo{path: path, size: info.Size(), modTime: info.ModTime()})
			}
		}
		return nil
	})
	// Sort by mtime descending (newest first) to match poddl's -r download order
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	return files
}

// findLatestAudioFile returns the most recently modified audio file in dir.
// Used by downloadViaPoddl for single-episode fallback downloads.
func (a *API) findLatestAudioFile(dir string) string {
	var best string
	var bestTime time.Time
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".mp3", ".m4a", ".ogg", ".opus", ".flac", ".aac", ".wav":
			if info.ModTime().After(bestTime) {
				bestTime = info.ModTime()
				best = path
			}
		}
		return nil
	})
	return best
}

// SyncAllFeeds syncs all subscribed feeds (called by background goroutine).
func (a *API) SyncAllFeeds(ctx context.Context) {
	rows, err := a.db.QueryContext(ctx, `SELECT id, url FROM podcast_feeds`)
	if err != nil {
		log.Printf("[podcaster] SyncAllFeeds query error: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var url string
		if err := rows.Scan(&id, &url); err != nil {
			continue
		}
		if err := a.doSyncFeed(ctx, id, url); err != nil {
			log.Printf("[podcaster] background sync feed %d error: %v", id, err)
		}
	}
}
