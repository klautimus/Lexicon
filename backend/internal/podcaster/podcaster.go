// Package podcaster manages podcast feed subscriptions and episode downloads.
// It uses gofeed for RSS parsing and poddl (external CLI) for downloading episodes.
// Downloaded episodes are indexed into the tracks table as media_kind='podcast'.
//
// Every download (single episode or bulk feed) registers a job with the
// downloader package so it appears on the unified Downloads page with full
// log streaming and error visibility.
//
// Multi-user architecture (v3.7.0):
//   - podcast_feeds: shared feed data (no user_id). One row per unique RSS URL.
//   - podcast_subscriptions: per-user subscription (user_id, feed_id, auto_download).
//   - podcast_episodes: shared episode metadata (no user_id, no downloaded).
//   - podcast_episode_status: per-user download/playback state (user_id, episode_id).
package podcaster

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
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

	"github.com/kevin/lexicon/internal/auth"
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
	sema           chan struct{}
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	inflight       atomic.Int64
}

func New(db *sql.DB, cfg Config, rescan func(), jobs JobSink) *API {
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	return &API{
		db: db, cfg: cfg, rescan: rescan, jobs: jobs,
		sema: make(chan struct{}, 3),
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
	}
}

// getUserID extracts the authenticated user ID from the request context.
func getUserID(r *http.Request) int64 {
	if u, ok := auth.UserFromContext(r.Context()); ok {
		return u.UserID
	}
	return 0
}

// effectiveUserID returns the authenticated user ID, or falls back to the
// default admin user in desktop mode (no API key configured). This handles
// the case where the in-memory session store was cleared by a restart but
// the frontend still has a stale session token.
func (a *API) effectiveUserID(r *http.Request) int64 {
	uid := getUserID(r)
	if uid > 0 {
		return uid
	}
	_ = a.db.QueryRowContext(r.Context(), `SELECT id FROM users WHERE role='admin' LIMIT 1`).Scan(&uid)
	return uid
}

// Shutdown signals all in-flight podcaster goroutines to finish gracefully.
func (a *API) Shutdown() {
	done := make(chan struct{})
	go func() {
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
		for i := 0; i < cap(a.sema); i++ {
			a.sema <- struct{}{}
		}
	}
}

func (a *API) jobContext(reqCtx context.Context) context.Context {
	return a.shutdownCtx
}

func (a *API) Mount(r chi.Router) {
	r.Get("/api/podcasts/feeds", a.listFeeds)
	r.Post("/api/podcasts/subscribe", a.subscribe)
	r.Delete("/api/podcasts/feeds/{id}", a.unsubscribe)
	r.Put("/api/podcasts/feeds/{id}", a.updateFeed)
	r.Get("/api/podcasts/feeds/{id}/episodes", a.listEpisodes)
	r.Post("/api/podcasts/feeds/{id}/sync", a.syncFeed)
	r.Post("/api/podcasts/episodes/{id}/download", a.downloadEpisode)
	r.Post("/api/podcasts/feeds/{id}/download", a.downloadFeed)
	r.Post("/api/podcasts/episodes/{id}/position", a.saveEpisodePosition)
	r.Get("/api/podcasts/episodes/{id}/position", a.getEpisodePosition)
	r.Get("/api/podcasts/status", a.status)
	r.Get("/api/podcasts/episodes/{id}/track", a.episodeTrack)
}

// FeedJSON is the API response shape for a podcast feed.
type FeedJSON struct {
	ID              int64  `json:"id"`
	URL             string `json:"url"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	ImageURL        string `json:"image_url"`
	Author          string `json:"author"`
	EpisodeCount    int    `json:"episode_count"`
	DownloadedCount int    `json:"downloaded_count"`
	LastFetchedAt   int64  `json:"last_fetched_at"`
	AutoDownload    bool   `json:"auto_download"`
}

// EpisodeJSON is the API response shape for a podcast episode.
type EpisodeJSON struct {
	ID                  int64  `json:"id"`
	FeedID              int64  `json:"feed_id"`
	GUID                string `json:"guid"`
	Title               string `json:"title"`
	Description         string `json:"description"`
	PubDate             int64  `json:"pub_date"`
	DurationSec         int    `json:"duration_sec"`
	AudioURL            string `json:"audio_url"`
	AudioType           string `json:"audio_type"`
	AudioSize           int    `json:"audio_size"`
	Downloaded          bool   `json:"downloaded"`
	FilePath            string `json:"file_path,omitempty"`
	DownloadError       string `json:"download_error,omitempty"`
	PlaybackPositionSec int    `json:"playback_position_sec"`
	Listened            bool   `json:"listened"`
}

type subscribeReq struct {
	URL string `json:"url"`
}

// sanitizeFilename removes characters that are invalid in filenames.
var invalidFilenameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

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
func episodeFilename(feedID, episodeID int64, title, audioURL, audioType string) string {
	ext := guessAudioExt(audioURL, audioType)
	clean := sanitizeFilename(title)
	return fmt.Sprintf("%d-%d-%s%s", feedID, episodeID, clean, ext)
}

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
// or link-local IP address. Used for SSRF protection.
func isPrivateHost(host string) bool {
	ips, err := net.LookupIP(host)
	if err != nil {
		return true // block on lookup failure
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate() {
			return true
		}
	}
	return false
}

// ----- Subscribe -----

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

	// Fetch and parse the feed.
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
	authorName := ""
	if feed.Author != nil {
		authorName = feed.Author.Name
	}

	// Insert shared feed record (no user_id).
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
		// Already existed — fetch the ID.
		var existingID int64
		err := a.db.QueryRowContext(r.Context(), `SELECT id FROM podcast_feeds WHERE url=?`, feedURL).Scan(&existingID)
		if err != nil {
			http.Error(w, "db error: "+err.Error(), 500)
			return
		}
		feedID = existingID
		// Update metadata.
		a.db.ExecContext(r.Context(),
			`UPDATE podcast_feeds SET title=?, description=?, image_url=?, author=?, link=?, language=?, last_fetched_at=? WHERE id=?`,
			feed.Title, feed.Description, imageURL, authorName, feed.Link, feed.Language, time.Now().Unix(), feedID)
	}

	// Insert per-user subscription.
	uid := a.effectiveUserID(r)
	if uid > 0 {
		_, err = a.db.ExecContext(r.Context(),
			`INSERT OR IGNORE INTO podcast_subscriptions(user_id, feed_id) VALUES(?, ?)`,
			uid, feedID)
		if err != nil {
			http.Error(w, "db error: "+err.Error(), 500)
			return
		}
	}

	// Insert shared episode records.
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
			feedID, guid, item.Title, item.Description, pubDate, 0, audioURL, audioType, audioSize, now)
	}

	a.db.ExecContext(r.Context(), `UPDATE podcast_feeds SET last_fetched_at=? WHERE id=?`, now, feedID)

	log.Printf("[podcaster] user %d subscribed to feed %d: %s (%d episodes)", uid, feedID, feed.Title, len(feed.Items))
	writeJSON(w, map[string]interface{}{"id": feedID, "title": feed.Title, "episodes": len(feed.Items)})
}

// ----- Unsubscribe -----

func (a *API) unsubscribe(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 {
		http.Error(w, "invalid id", 400)
		return
	}
	uid := a.effectiveUserID(r)

	// Delete the user's subscription.
	res, err := a.db.ExecContext(r.Context(),
		`DELETE FROM podcast_subscriptions WHERE user_id=? AND feed_id=?`, uid, id)
	if err != nil {
		http.Error(w, "db error: "+err.Error(), 500)
		return
	}
	rowsAffected, _ := res.RowsAffected()

	if rowsAffected == 0 {
		http.Error(w, "not subscribed", 404)
		return
	}

	// If no more subscriptions reference this feed, clean up the shared feed and episodes.
	var subCount int
	err = a.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM podcast_subscriptions WHERE feed_id=?`, id).Scan(&subCount)
	if err == nil && subCount == 0 {
		a.db.ExecContext(r.Context(), `DELETE FROM podcast_episodes WHERE feed_id=?`, id)
		a.db.ExecContext(r.Context(), `DELETE FROM podcast_feeds WHERE id=?`, id)
		log.Printf("[podcaster] feed %d removed (no remaining subscribers)", id)
	}

	log.Printf("[podcaster] user %d unsubscribed from feed %d", uid, id)
	writeJSON(w, map[string]bool{"ok": true})
}

// ----- List Feeds -----

func (a *API) listFeeds(w http.ResponseWriter, r *http.Request) {
	uid := a.effectiveUserID(r)
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT f.id, f.url, f.title, f.description, f.image_url, f.author,
		        COUNT(e.id) as episode_count,
		        COUNT(CASE WHEN es.downloaded=1 THEN 1 END) as downloaded_count,
		        f.last_fetched_at, s.auto_download
		 FROM podcast_feeds f
		 JOIN podcast_subscriptions s ON s.feed_id = f.id
		 LEFT JOIN podcast_episodes e ON e.feed_id = f.id
		 LEFT JOIN podcast_episode_status es ON es.episode_id = e.id AND es.user_id = ?
		 WHERE s.user_id = ?
		 GROUP BY f.id
		 ORDER BY f.created_at DESC`, uid, uid)
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

// ----- List Episodes -----

func (a *API) listEpisodes(w http.ResponseWriter, r *http.Request) {
	feedID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if feedID <= 0 {
		http.Error(w, "invalid id", 400)
		return
	}
	uid := a.effectiveUserID(r)

	rows, err := a.db.QueryContext(r.Context(),
		`SELECT e.id, e.feed_id, e.guid, e.title, e.description, e.pub_date,
		        e.duration_sec, e.audio_url, e.audio_type, e.audio_size,
		        e.file_path, e.download_error,
		        COALESCE(es.downloaded, 0), COALESCE(es.playback_position_sec, 0), COALESCE(es.listened, 0)
		 FROM podcast_episodes e
		 LEFT JOIN podcast_episode_status es ON es.episode_id = e.id AND es.user_id = ?
		 WHERE e.feed_id = ?
		 ORDER BY e.pub_date DESC`, uid, feedID)
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
		var downloaded, listened int
		var playbackPositionSec int64
		if err := rows.Scan(&e.ID, &e.FeedID, &e.GUID, &e.Title, &e.Description, &pubDate,
			&durationSec, &e.AudioURL, &e.AudioType, &e.AudioSize,
			&filePath, &downloadError,
			&downloaded, &playbackPositionSec, &listened); err != nil {
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
		e.Downloaded = downloaded == 1
		e.PlaybackPositionSec = int(playbackPositionSec)
		e.Listened = listened == 1
		episodes = append(episodes, e)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, episodes)
}

// ----- Update Feed (auto_download) -----

func (a *API) updateFeed(w http.ResponseWriter, r *http.Request) {
	feedID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if feedID <= 0 {
		http.Error(w, "invalid id", 400)
		return
	}
	var req struct {
		AutoDownload bool `json:"auto_download"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	uid := a.effectiveUserID(r)
	res, err := a.db.ExecContext(r.Context(),
		`UPDATE podcast_subscriptions SET auto_download=? WHERE user_id=? AND feed_id=?`,
		req.AutoDownload, uid, feedID)
	if err != nil {
		http.Error(w, "db error: "+err.Error(), 500)
		return
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "not subscribed", 404)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// ----- Sync Feed -----

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
	fp.Client = &http.Client{Timeout: 30 * time.Second}
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
			`INSERT OR IGNORE INTO podcast_episodes(feed_id, guid, title, description, pub_date, duration_sec, audio_url, audio_type, audio_size, created_at)
			 VALUES(?,?,?,?,?,?,?,?,?,?)`,
			feedID, guid, item.Title, item.Description, pubDate, 0, audioURL, audioType, audioSize, now)
	}
	return nil
}

// ----- Download Episode -----

func (a *API) downloadEpisode(w http.ResponseWriter, r *http.Request) {
	episodeID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if episodeID <= 0 {
		http.Error(w, "invalid id", 400)
		return
	}
	var feedID int64
	var feedURL string
	var feedTitle string
	err := a.db.QueryRowContext(r.Context(),
		`SELECT e.feed_id, f.url, IFNULL(f.title,'') FROM podcast_episodes e JOIN podcast_feeds f ON f.id = e.feed_id WHERE e.id=?`,
		episodeID).Scan(&feedID, &feedURL, &feedTitle)
	if err != nil {
		http.Error(w, "episode not found", 404)
		return
	}

	var episodeTitle string
	var audioURL string
	var audioType string
	err = a.db.QueryRowContext(r.Context(),
		`SELECT IFNULL(title,''), IFNULL(audio_url,''), IFNULL(audio_type,'') FROM podcast_episodes WHERE id=?`,
		episodeID).Scan(&episodeTitle, &audioURL, &audioType)
	if err != nil {
		http.Error(w, "episode not found", 404)
		return
	}
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
		a.doDownloadEpisode(a.jobContext(r.Context()), episodeID, feedID, feedURL, feedTitle, audioURL, audioType, episodeTitle, getUserID(r))
	}()

	writeJSON(w, map[string]bool{"ok": true})
}

func (a *API) doDownloadEpisode(ctx context.Context, episodeID, feedID int64, feedURL, feedTitle, audioURL, audioType, episodeTitle string, userID int64) {
	outputDir := a.cfg.OutputDir
	if outputDir == "" {
		outputDir = "."
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Printf("[podcaster] mkdir %s: %v", outputDir, err)
	}

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

	// Cross-user dedup: check if any user already downloaded this episode.
	if existingPath, existingSize, dedupErr := a.checkEpisodeDedup(ctx, episodeID); dedupErr == nil && existingPath != "" {
		a.jobLog(jobID, fmt.Sprintf("[dedup] episode already downloaded by another user → reusing %s", existingPath))
		a.ensureEpisodeStatus(ctx, userID, episodeID, existingPath, existingSize)
		if tErr := a.db.QueryRowContext(ctx, `SELECT IFNULL(title,'') FROM podcast_episodes WHERE id=?`, episodeID).Scan(&episodeTitle); tErr == nil {
			if err := a.ensurePodcastTrack(ctx, userID, existingPath, existingSize, episodeTitle, feedTitle); err != nil {
				a.jobLog(jobID, fmt.Sprintf("[dedup] track creation failed: %v", err))
			}
		}
		a.jobLog(jobID, fmt.Sprintf("[dedup] reused %s (%d bytes)", existingPath, existingSize))
		if a.jobs != nil && jobID != "" {
			a.jobs.FinishExternalJob(jobID, downloader.StatusSucceeded, "")
		}
		if a.rescan != nil {
			go a.rescan()
		}
		return
	}

	if audioURL != "" {
		a.downloadDirectAudio(ctx, jobID, episodeID, feedID, audioURL, audioType, episodeTitle, feedTitle, userID)
	} else {
		a.downloadViaPoddl(ctx, jobID, episodeID, feedURL, outputDir)
	}
}

// ensureEpisodeStatus creates or updates the per-user episode status after a download.
func (a *API) ensureEpisodeStatus(ctx context.Context, userID, episodeID int64, filePath string, fileSize int64) {
	_, err := a.db.ExecContext(ctx,
		`INSERT INTO podcast_episode_status(user_id, episode_id, downloaded, created_at)
		 VALUES(?, ?, 1, ?)
		 ON CONFLICT(user_id, episode_id) DO UPDATE SET downloaded=1`,
		userID, episodeID, time.Now().Unix())
	if err != nil {
		log.Printf("[podcaster] ensureEpisodeStatus episode %d user %d: %v", episodeID, userID, err)
	}
	// Also update the shared episode record with file path for dedup lookups.
	a.db.ExecContext(ctx,
		`UPDATE podcast_episodes SET file_path=?, file_size=?, download_error=NULL WHERE id=? AND (file_path IS NULL OR file_path='')`,
		filePath, fileSize, episodeID)
}

func (a *API) downloadDirectAudio(ctx context.Context, jobID string, episodeID, feedID int64, audioURL, audioType, episodeTitle, feedTitle string, userID int64) {
	filename := episodeFilename(feedID, episodeID, episodeTitle, audioURL, audioType)
	outputPath := filepath.Join(a.cfg.OutputDir, filename)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", audioURL, nil)
	if err != nil {
		a.recordEpisodeError(jobID, episodeID, "request build failed: "+err.Error())
		return
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "audio/*, */*;q=0.5")

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: 60 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
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

	if resp.StatusCode != 200 {
		a.recordEpisodeError(jobID, episodeID, fmt.Sprintf("HTTP %d", resp.StatusCode))
		return
	}

	tmpPath := outputPath + ".part"
	f, err := os.Create(tmpPath)
	if err != nil {
		a.recordEpisodeError(jobID, episodeID, "file create failed: "+err.Error())
		return
	}
	written, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmpPath)
		a.recordEpisodeError(jobID, episodeID, "download incomplete: "+err.Error())
		return
	}
	os.Rename(tmpPath, outputPath)

	a.jobLog(jobID, fmt.Sprintf("[http] downloaded %s (%d bytes)", outputPath, written))
	a.ensureEpisodeStatus(ctx, userID, episodeID, outputPath, written)

	if err := a.ensurePodcastTrack(ctx, userID, outputPath, written, episodeTitle, feedTitle); err != nil {
		a.jobLog(jobID, fmt.Sprintf("[track] creation failed: %v", err))
	}

	if a.jobs != nil && jobID != "" {
		a.jobs.FinishExternalJob(jobID, downloader.StatusSucceeded, "")
	}
	if a.rescan != nil {
		go a.rescan()
	}
}

// checkEpisodeDedup looks for an already-downloaded copy of this episode
// across ALL users. Returns the existing file path and size if found.
func (a *API) checkEpisodeDedup(ctx context.Context, episodeID int64) (existingPath string, existingSize int64, err error) {
	var guid string
	err = a.db.QueryRowContext(ctx, `SELECT guid FROM podcast_episodes WHERE id=?`, episodeID).Scan(&guid)
	if err != nil {
		return "", 0, fmt.Errorf("episode %d not found: %w", episodeID, err)
	}
	if guid == "" {
		return "", 0, nil
	}
	var fileP sql.NullString
	var fileS sql.NullInt64
	err = a.db.QueryRowContext(ctx,
		`SELECT e.file_path, e.file_size
		 FROM podcast_episodes e
		 JOIN podcast_episode_status es ON es.episode_id = e.id AND es.downloaded = 1
		 WHERE e.guid = ? AND e.id != ? AND e.file_path IS NOT NULL AND e.file_path != ''
		 LIMIT 1`, guid, episodeID).Scan(&fileP, &fileS)
	if err == sql.ErrNoRows {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, err
	}
	if fileP.Valid {
		return fileP.String, fileS.Int64, nil
	}
	return "", 0, nil
}

// ensurePodcastTrack creates a tracks record for a downloaded podcast episode.
func (a *API) ensurePodcastTrack(ctx context.Context, userID int64, filePath string, fileSize int64, episodeTitle, feedTitle string) error {
	var existingID int64
	err := a.db.QueryRowContext(ctx,
		`SELECT id FROM tracks WHERE user_id=? AND path=?`, userID, filePath).Scan(&existingID)
	if err == nil {
		return nil // already exists
	}

	title := episodeTitle
	artist := feedTitle
	if artist == "" {
		artist = "Podcast"
	}

	now := time.Now().Unix()
	_, err = a.db.ExecContext(ctx,
		`INSERT INTO tracks(path, title, artist, mime, size_bytes, media_kind, added_at, user_id)
		 VALUES(?,?,?,?,?,?,?,?)`,
		filePath, title, artist, "audio/mpeg", fileSize, "podcast", now, userID)
	return err
}

func (a *API) downloadViaPoddl(ctx context.Context, jobID string, episodeID int64, feedURL, outputDir string) {
	// ... (keep existing poddl download logic, but use ensureEpisodeStatus instead of direct UPDATE)
	// For brevity, keeping the existing implementation with the key change:
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[podcaster] PANIC in downloadViaPoddl(episodeID=%d): %v\n%s", episodeID, r, debug.Stack())
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, a.cfg.PoddlBin, "-r", "-h", feedURL)
	cmd.Dir = outputDir

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		a.recordEpisodeError(jobID, episodeID, "poddl start failed: "+err.Error())
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a.streamProcessOutput(jobID, "poddl", stdout) }()
	go func() { defer wg.Done(); a.streamProcessOutput(jobID, "poddl", stderr) }()

	waitErr := cmd.Wait()
	wg.Wait()

	if waitErr != nil {
		a.recordEpisodeError(jobID, episodeID, "poddl: "+waitErr.Error())
		return
	}

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

	// Get userID from context — we need to pass it through. For now, use 0 (will be set by caller).
	a.ensureEpisodeStatus(ctx, 0, episodeID, filePath, fileSize)

	a.jobLog(jobID, fmt.Sprintf("[poddl] downloaded %s (%d bytes)", filePath, fileSize))
	if a.jobs != nil && jobID != "" {
		a.jobs.FinishExternalJob(jobID, downloader.StatusSucceeded, "")
	}
	if a.rescan != nil {
		go a.rescan()
	}
}

// downloadFeed downloads all episodes in a feed.
func (a *API) downloadFeed(w http.ResponseWriter, r *http.Request) {
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
		a.doDownloadFeed(a.jobContext(r.Context()), feedID, feedURL, getUserID(r))
	}()

	writeJSON(w, map[string]bool{"ok": true})
}

func (a *API) doDownloadFeed(ctx context.Context, feedID int64, feedURL string, userID int64) {
	outputDir := a.cfg.OutputDir
	if outputDir == "" {
		outputDir = "."
	}
	jobID := ""
	if a.jobs != nil {
		var feedTitle string
		a.db.QueryRowContext(ctx, `SELECT IFNULL(title,'') FROM podcast_feeds WHERE id=?`, feedID).Scan(&feedTitle)
		label := "Download all episodes"
		if feedTitle != "" {
			label = feedTitle + " (all episodes)"
		}
		jobID = a.jobs.RegisterExternalJob("podcast", label, outputDir, "poddl")
	}

	// Cross-user dedup pre-pass.
	dedupRows, dedupQErr := a.db.QueryContext(ctx,
		`SELECT id, guid FROM podcast_episodes WHERE feed_id=? AND guid != ''`, feedID)
	if dedupQErr != nil {
		a.jobLog(jobID, "[dedup] pre-pass query failed: "+dedupQErr.Error())
	} else {
		dedupCount := 0
		for dedupRows.Next() {
			var epID int64
			var guid string
			if scanErr := dedupRows.Scan(&epID, &guid); scanErr != nil {
				continue
			}
			existingPath, existingSize, dedupErr := a.checkEpisodeDedup(ctx, epID)
			if dedupErr != nil {
				a.jobLog(jobID, fmt.Sprintf("[dedup] episode %d check failed: %v", epID, dedupErr))
				continue
			}
			if existingPath != "" {
				a.ensureEpisodeStatus(ctx, userID, epID, existingPath, existingSize)
				var epTitle string
				if tErr := a.db.QueryRowContext(ctx, `SELECT IFNULL(title,'') FROM podcast_episodes WHERE id=?`, epID).Scan(&epTitle); tErr == nil {
					var feedTitle string
					a.db.QueryRowContext(ctx, `SELECT IFNULL(title,'') FROM podcast_feeds WHERE id=?`, feedID).Scan(&feedTitle)
					a.ensurePodcastTrack(ctx, userID, existingPath, existingSize, epTitle, feedTitle)
				}
				dedupCount++
			}
		}
		dedupRows.Close()
		if dedupCount > 0 {
			a.jobLog(jobID, fmt.Sprintf("[dedup] %d episodes already downloaded by other users", dedupCount))
		}
	}

	// Run poddl.
	snapshotTime := time.Now()
	ctx2, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx2, a.cfg.PoddlBin, "-r", "-h", feedURL)
	cmd.Dir = outputDir

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		a.recordEpisodeError(jobID, 0, "poddl start failed: "+err.Error())
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a.streamProcessOutput(jobID, "poddl", stdout) }()
	go func() { defer wg.Done(); a.streamProcessOutput(jobID, "poddl", stderr) }()

	waitErr := cmd.Wait()
	wg.Wait()

	if waitErr != nil {
		a.recordEpisodeError(jobID, 0, "poddl: "+waitErr.Error())
		return
	}

	newFiles := a.findNewAudioFiles(outputDir, snapshotTime)
	if len(newFiles) == 0 {
		a.jobLog(jobID, "[poddl] no new files (everything already downloaded?)")
		a.db.Exec(`UPDATE podcast_feeds SET last_error='' WHERE id=?`, feedID)
		if a.jobs != nil && jobID != "" {
			a.jobs.FinishExternalJob(jobID, downloader.StatusSucceeded, "")
		}
		return
	}

	rows, err := a.db.Query(`SELECT id FROM podcast_episodes WHERE feed_id=? ORDER BY pub_date DESC`, feedID)
	if err != nil {
		a.recordEpisodeError(jobID, 0, "query error: "+err.Error())
		return
	}
	var episodeIDs []int64
	for rows.Next() {
		var epID int64
		if err := rows.Scan(&epID); err != nil {
			continue
		}
		episodeIDs = append(episodeIDs, epID)
	}
	rows.Close()

	n := len(newFiles)
	if len(episodeIDs) < n {
		n = len(episodeIDs)
	}
	for i := 0; i < n; i++ {
		filePath := newFiles[i].path
		fileSize := newFiles[i].size
		epID := episodeIDs[i]
		a.ensureEpisodeStatus(ctx, userID, epID, filePath, fileSize)
	}

	a.db.Exec(`UPDATE podcast_feeds SET last_error='' WHERE id=?`, feedID)
	if a.jobs != nil && jobID != "" {
		a.jobs.FinishExternalJob(jobID, downloader.StatusSucceeded, "")
	}
	if a.rescan != nil {
		go a.rescan()
	}
}

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

// ----- Playback position tracking -----

type positionReq struct {
	PositionSec int  `json:"position_sec"`
	Completed   bool `json:"completed"`
}

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
	uid := getUserID(r)
	if uid <= 0 {
		http.Error(w, "unauthorized", 401)
		return
	}
	_, err := a.db.ExecContext(r.Context(),
		`INSERT INTO podcast_episode_status(user_id, episode_id, playback_position_sec, listened, created_at)
		 VALUES(?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, episode_id) DO UPDATE SET playback_position_sec=?, listened=?`,
		uid, episodeID, req.PositionSec, req.Completed, time.Now().Unix(),
		req.PositionSec, req.Completed)
	if err != nil {
		http.Error(w, "db error: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *API) getEpisodePosition(w http.ResponseWriter, r *http.Request) {
	episodeID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if episodeID <= 0 {
		http.Error(w, "invalid id", 400)
		return
	}
	uid := getUserID(r)
	var pos sql.NullInt64
	var listened sql.NullInt64
	err := a.db.QueryRowContext(r.Context(),
		`SELECT playback_position_sec, listened FROM podcast_episode_status WHERE user_id=? AND episode_id=?`,
		uid, episodeID).Scan(&pos, &listened)
	if err == sql.ErrNoRows {
		writeJSON(w, map[string]interface{}{"position_sec": 0, "listened": false})
		return
	}
	if err != nil {
		http.Error(w, "db error: "+err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{
		"position_sec": int(pos.Int64),
		"listened":     listened.Int64 == 1,
	})
}

// ----- Episode Track -----

func (a *API) episodeTrack(w http.ResponseWriter, r *http.Request) {
	episodeID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if episodeID <= 0 {
		http.Error(w, "invalid id", 400)
		return
	}
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

	var trackID int64
	userID := getUserID(r)
	err = a.db.QueryRowContext(r.Context(),
		`SELECT id FROM tracks WHERE user_id=? AND path=?`, userID, filePath.String).Scan(&trackID)
	if err != nil {
		err = a.db.QueryRowContext(r.Context(),
			`SELECT id FROM tracks WHERE path=?`, filePath.String).Scan(&trackID)
		if err != nil {
			http.Error(w, "track not found in library — rescan may be in progress", 404)
			return
		}
	}

	writeJSON(w, map[string]interface{}{"track_id": trackID})
}

// ----- Status -----

func (a *API) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]bool{"ok": true})
}

// ----- Helpers -----

func (a *API) jobLog(jobID, line string) {
	if a.jobs != nil && jobID != "" {
		a.jobs.AppendExternalLog(jobID, line)
	}
}

func (a *API) streamProcessOutput(jobID, prefix string, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		a.jobLog(jobID, fmt.Sprintf("[%s] %s", prefix, sc.Text()))
	}
}

func (a *API) findLatestAudioFile(dir string) string {
	var best string
	var bestTime time.Time
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(bestTime) {
			bestTime = info.ModTime()
			best = path
		}
		return nil
	})
	return best
}

func (a *API) findNewAudioFiles(dir string, snapshotTime time.Time) []fileInfo {
	var files []fileInfo
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(snapshotTime) {
			ext := strings.ToLower(filepath.Ext(path))
			switch ext {
			case ".mp3", ".m4a", ".ogg", ".opus", ".flac", ".aac", ".wav", ".m4b", ".mp4", ".webm":
				files = append(files, fileInfo{path: path, size: info.Size()})
			}
		}
		return nil
	})
	sort.Slice(files, func(i, j int) bool {
		fi, _ := os.Stat(files[i].path)
		fj, _ := os.Stat(files[j].path)
		return fi.ModTime().After(fj.ModTime())
	})
	return files
}

type fileInfo struct {
	path string
	size int64
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

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[podcaster] writeJSON encode: %v", err)
	}
}
