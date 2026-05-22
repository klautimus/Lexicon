package spotify

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/kevin/lexicon/internal/auth"
)

const (
	syncInterval = 30 * time.Minute
	minSyncGap   = 25 * time.Minute
)

type Syncer struct {
	db  *sql.DB
	cfg Config
	mu  sync.Mutex
}

func NewSyncer(db *sql.DB, cfg Config) *Syncer {
	return &Syncer{db: db, cfg: cfg}
}

// Start launches the background ticker. Safe to call once at startup.
func (s *Syncer) Start(ctx context.Context) {
	go func() {
		// Initial sync after 5s grace period (lets server fully boot)
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return
		}
		s.RunOnce(ctx)
		t := time.NewTicker(syncInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.RunOnce(ctx)
			}
		}
	}()
}

// RunOnce performs one sync cycle. Returns nil if not connected or nothing to do.
func (s *Syncer) RunOnce(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cfg.ClientID == "" {
		return nil
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT lexicon_user_id FROM spotify_tokens WHERE access_token IS NOT NULL`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var userIDs []int64
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			continue
		}
		userIDs = append(userIDs, uid)
	}

	for _, uid := range userIDs {
		s.syncOneUser(ctx, uid)
	}
	return nil
}

// syncOneUser runs a single sync pass for one user.
func (s *Syncer) syncOneUser(ctx context.Context, uid int64) {
	// Embed the user in the context so ensureToken resolves the right row.
	ctx = auth.ContextWithUser(ctx, &auth.UserInfo{UserID: uid})

	var lastSyncedAt int64
	err := s.db.QueryRowContext(ctx,
		`SELECT last_synced_at FROM spotify_tokens WHERE lexicon_user_id=?`, uid).Scan(&lastSyncedAt)
	if err != nil {
		// Not connected — silently skip.
		return
	}
	if lastSyncedAt > 0 && time.Now().Unix()-lastSyncedAt < int64(minSyncGap.Seconds()) {
		return
	}

	access, err := ensureToken(ctx, s.db, s.cfg.ClientID, s.cfg.ClientSecret)
	if err != nil {
		log.Printf("[spotify] token for user %d: %v", uid, err)
		return
	}

	// Spotify cursor uses ms-since-epoch; we store seconds. Use last_synced_at*1000.
	afterMs := lastSyncedAt * 1000
	rp, err := recentlyPlayed(ctx, access, afterMs)
	if err != nil {
		log.Printf("[spotify] recently-played for user %d: %v", uid, err)
		return
	}
	if len(rp.Items) == 0 {
		s.db.ExecContext(ctx, `UPDATE spotify_tokens SET last_synced_at=? WHERE lexicon_user_id=?`, time.Now().Unix(), uid)
		return
	}

	// Collect unique artist IDs
	artistIDs := make(map[string]bool)
	for _, item := range rp.Items {
		for _, artist := range item.Track.Artists {
			if artist.ID != "" {
				artistIDs[artist.ID] = true
			}
		}
	}

	// Fetch genres for all artists
	genreMap := make(map[string]string)
	if len(artistIDs) > 0 {
		ids := make([]string, 0, len(artistIDs))
		for id := range artistIDs {
			ids = append(ids, id)
		}
		var err error
		genreMap, err = fetchArtistGenres(ctx, access, ids)
		if err != nil {
			log.Printf("[spotify] fetch artist genres for user %d: %v", uid, err)
		}
	}

	imported := 0
	for _, item := range rp.Items {
		if err := s.ingestPlay(ctx, item, genreMap); err != nil {
			log.Printf("[spotify] ingest %s: %v", item.Track.ID, err)
			continue
		}
		imported++
	}

	// Cursor: use the most-recent played_at we saw (rp items come newest-first)
	newestMs := afterMs
	for _, it := range rp.Items {
		if t, err := time.Parse(time.RFC3339, it.PlayedAt); err == nil {
			ms := t.UnixMilli()
			if ms > newestMs {
				newestMs = ms
			}
		}
	}
	s.db.ExecContext(ctx, `UPDATE spotify_tokens SET last_synced_at=? WHERE lexicon_user_id=?`, newestMs/1000, uid)
	log.Printf("[spotify] user %d: synced %d play(s)", uid, imported)
}

func (s *Syncer) ingestPlay(ctx context.Context, item RecentItem, genreMap map[string]string) error {
	t := item.Track
	if t.ID == "" {
		return nil
	}
	playedAt, err := time.Parse(time.RFC3339, item.PlayedAt)
	if err != nil {
		return err
	}
	startedAt := playedAt.Unix()

	artist := artistsString(t)
	album := t.Album.Name
	year := releaseYear(t.Album.ReleaseDate)
	durSec := t.DurationMs / 1000
	externalURL := ""
	if t.ExternalURL != nil {
		externalURL = t.ExternalURL["spotify"]
	}
	// Resolve genre from the first artist that has genres
	genre := ""
	for _, a := range t.Artists {
		if g, ok := genreMap[a.ID]; ok && g != "" {
			genre = g
			break
		}
	}

	// Upsert by spotify_id — explicit SELECT/UPDATE/INSERT avoids SQLite partial-index ON CONFLICT limitation
	// Note: loudness_integrated, loudness_true_peak, and loudness_range are left NULL for Spotify
	// tracks. These columns measure local file loudness via ffmpeg during scanning. Spotify
	// streams are already loudness-normalized server-side, so local measurement is not applicable.
	// The Spotify Audio Features API provides integrated loudness but not true peak or range,
	// and calling it per-track would significantly slow sync.
	var trackID int64
	err = s.db.QueryRowContext(ctx, `SELECT id FROM tracks WHERE spotify_id=?`, t.ID).Scan(&trackID)
	if err == sql.ErrNoRows {
		now := time.Now().Unix()
		res, err := s.db.ExecContext(ctx, `
			INSERT INTO tracks(path, title, artist, album_artist, album, year, genre, duration_sec, mime, media_kind, size_bytes, cover_path, added_at, spotify_id, external_url, user_id)
			VALUES('spotify:' || ?, ?, ?, ?, ?, ?, ?, ?, '', 'music', 0, '', ?, ?, ?, ?)
		`, t.ID, t.Name, artist, artist, album, year, genre, durSec, now, t.ID, externalURL, nil)
		if err != nil {
			return err
		}
		trackID, _ = res.LastInsertId()
	} else if err != nil {
		return err
	} else {
		_, err = s.db.ExecContext(ctx, `
			UPDATE tracks SET title=?, artist=?, album_artist=?, album=?, year=?, genre=?, duration_sec=?, external_url=?
			WHERE spotify_id=?`, t.Name, artist, artist, album, year, genre, durSec, externalURL, t.ID)
		if err != nil {
			return err
		}
	}

	// Avoid duplicate plays with same (track, started_at)
	var exists int
	s.db.QueryRowContext(ctx,
		`SELECT 1 FROM plays WHERE track_id=? AND started_at=? AND source='spotify' LIMIT 1`,
		trackID, startedAt).Scan(&exists)
	if exists == 1 {
		return nil
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO plays(track_id, started_at, duration_played_sec, completed, source, user_id)
		VALUES(?, ?, ?, 1, 'spotify', ?)`, trackID, startedAt, durSec, nil)
	return err
}

// Manual sync HTTP handler is in spotify.go; helper here.
func (a *API) manualSync(w http.ResponseWriter, r *http.Request) {
	go a.sync.RunOnce(r.Context())
	writeJSON(w, map[string]bool{"started": true})
}

// SDK token mint
func (a *API) sdkToken(w http.ResponseWriter, r *http.Request) {
	access, err := a.ValidAccessToken(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]string{"access_token": access})
}

// silence "declared and not used" if json import unused
var _ = json.Marshal
