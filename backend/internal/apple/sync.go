package apple

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

const (
	syncInterval = 30 * time.Minute
	minSyncGap   = 25 * time.Minute
)

// Syncer periodically pulls the user's recently-played Apple Music history
// into the plays table with source='apple'. Mirrors spotify.Syncer.
type Syncer struct {
	db  *sql.DB
	api *API
	mu  sync.Mutex
}

func NewSyncer(db *sql.DB, api *API) *Syncer {
	return &Syncer{db: db, api: api}
}

// Start launches the background ticker. Safe to call once at startup.
func (s *Syncer) Start(ctx context.Context) {
	go func() {
		// Grace period so app finishes booting before first sync attempt.
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return
		}
		_ = s.RunOnce(ctx)
		t := time.NewTicker(syncInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = s.RunOnce(ctx)
			}
		}
	}()
}

// RunOnce performs one sync pass. Returns nil if not configured / not
// connected / nothing to do.
func (s *Syncer) RunOnce(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT lexicon_user_id FROM apple_music_user WHERE music_user_token IS NOT NULL AND music_user_token != ''`)
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

// syncOneUser runs a single sync pass for one Apple Music user.
func (s *Syncer) syncOneUser(ctx context.Context, uid int64) {
	devTok, mut, err := s.api.CurrentTokens(ctx, uid)
	if err != nil {
		if errors.Is(err, ErrNotConfigured) {
			return
		}
		log.Printf("[apple] sync user %d: token: %v", uid, err)
		return
	}
	if mut == "" {
		// Not connected yet — silent no-op.
		return
	}

	var lastSyncedAt int64
	var storefront string
	if err := s.db.QueryRowContext(ctx,
		`SELECT last_synced_at, storefront FROM apple_music_user WHERE lexicon_user_id=?`, uid).Scan(&lastSyncedAt, &storefront); err != nil {
		return
	}
	if storefront == "" {
		storefront = "us"
	}
	if lastSyncedAt > 0 && time.Now().Unix()-lastSyncedAt < int64(minSyncGap.Seconds()) {
		return
	}

	rp, err := FetchRecentlyPlayed(ctx, devTok, mut, 30)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			log.Printf("[apple] sync user %d: MUT rejected by Apple — user must re-authorize", uid)
		} else {
			log.Printf("[apple] sync user %d: recently-played: %v", uid, err)
		}
		return
	}
	if len(rp.Data) == 0 {
		_, _ = s.db.ExecContext(ctx, `UPDATE apple_music_user SET last_synced_at=? WHERE lexicon_user_id=?`, time.Now().Unix(), uid)
		return
	}

	// Apple does not provide play timestamps on /me/recent/played/tracks —
	// it just returns "the most recently played N", newest first. To map
	// onto the existing plays table, we assign synthetic started_at values
	// spaced by the track durations relative to now. This is good enough
	// for ordering analytics; exact play time is not guaranteed by Apple.
	imported := 0
	cursor := time.Now().Unix()
	for _, song := range rp.Data {
		dur := song.Attributes.DurationInMillis / 1000
		if dur <= 0 {
			dur = 180
		}
		startedAt := cursor - dur
		cursor = startedAt
		// Avoid re-ingesting items we already have for the same approximate slot.
		if lastSyncedAt > 0 && startedAt < lastSyncedAt-int64(48*time.Hour.Seconds()) {
			// Item is older than 48h before our last cursor — skip.
			continue
		}
		if err := s.ingest(ctx, song, startedAt); err != nil {
			log.Printf("[apple] ingest %s: %v", song.ID, err)
			continue
		}
		imported++
	}

	_, _ = s.db.ExecContext(ctx, `UPDATE apple_music_user SET last_synced_at=? WHERE lexicon_user_id=?`, time.Now().Unix(), uid)
	if imported > 0 {
		log.Printf("[apple] user %d: synced %d play(s)", uid, imported)
	}
}

// ingest upserts a track row (by apple_id) and records a play entry.
func (s *Syncer) ingest(ctx context.Context, song SongResource, startedAt int64) error {
	if song.ID == "" {
		return nil
	}
	attr := song.Attributes
	genre := ""
	if len(attr.GenreNames) > 0 {
		genre = attr.GenreNames[0]
	}
	durSec := attr.DurationInMillis / 1000
	year := 0
	if attr.ReleaseDate != "" {
		if y, err := parseYear(attr.ReleaseDate); err == nil {
			year = y
		}
	}

	// Catalog ID is what we'd actually use for playback later; library
	// songs have "i.XXX" ids while catalog songs are numeric. Prefer
	// PlayParams.CatalogID when present.
	canonicalID := song.ID
	if attr.PlayParams.CatalogID != "" {
		canonicalID = attr.PlayParams.CatalogID
	}

	var trackID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM tracks WHERE apple_id=?`, canonicalID).Scan(&trackID)
	if errors.Is(err, sql.ErrNoRows) {
		now := time.Now().Unix()
		res, err := s.db.ExecContext(ctx, `
			INSERT INTO tracks(path, title, artist, album_artist, album, year, genre,
			                  duration_sec, mime, media_kind, size_bytes, cover_path,
			                  added_at, apple_id, external_url, user_id)
			VALUES('apple:' || ?, ?, ?, ?, ?, ?, ?, ?, '', 'music', 0, '', ?, ?, ?, ?)
		`, canonicalID, attr.Name, attr.ArtistName, attr.ArtistName, attr.AlbumName,
			year, genre, durSec, now, canonicalID, attr.URL, nil)
		if err != nil {
			return fmt.Errorf("insert track: %w", err)
		}
		trackID, _ = res.LastInsertId()
	} else if err != nil {
		return err
	} else {
		_, err := s.db.ExecContext(ctx, `
			UPDATE tracks SET title=?, artist=?, album_artist=?, album=?, year=?, genre=?,
			                 duration_sec=?, external_url=?
			WHERE apple_id=?`,
			attr.Name, attr.ArtistName, attr.ArtistName, attr.AlbumName,
			year, genre, durSec, attr.URL, canonicalID)
		if err != nil {
			return fmt.Errorf("update track: %w", err)
		}
	}

	// Avoid duplicate plays at the same (track, started_at, source) slot.
	var exists int
	_ = s.db.QueryRowContext(ctx,
		`SELECT 1 FROM plays WHERE track_id=? AND started_at=? AND source='apple' LIMIT 1`,
		trackID, startedAt).Scan(&exists)
	if exists == 1 {
		return nil
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO plays(track_id, started_at, duration_played_sec, completed, source, user_id)
		 VALUES(?, ?, ?, 1, 'apple', ?)`,
		trackID, startedAt, durSec, nil)
	return err
}

// parseYear extracts the leading 4-digit year from a date like "2022-05-06".
func parseYear(s string) (int, error) {
	s = strings.TrimSpace(s)
	if len(s) < 4 {
		return 0, fmt.Errorf("too short")
	}
	var y int
	for i := 0; i < 4; i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("non-digit at %d", i)
		}
		y = y*10 + int(c-'0')
	}
	return y, nil
}
