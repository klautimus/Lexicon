package db

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"regexp"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS tracks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	path TEXT NOT NULL UNIQUE,
	title TEXT,
	artist TEXT,
	album_artist TEXT,
	album TEXT,
	track_no INTEGER,
	disc_no INTEGER,
	year INTEGER,
	genre TEXT,
	duration_sec INTEGER,
	mime TEXT,
	size_bytes INTEGER,
	media_kind TEXT NOT NULL DEFAULT 'music', -- 'music' | 'podcast'
	cover_path TEXT,
	added_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
	mtime INTEGER,
	spotify_id TEXT,
	external_url TEXT,
	apple_id TEXT
);
CREATE INDEX IF NOT EXISTS idx_tracks_artist ON tracks(artist);
CREATE INDEX IF NOT EXISTS idx_tracks_album ON tracks(album);
CREATE INDEX IF NOT EXISTS idx_tracks_kind ON tracks(media_kind);
CREATE INDEX IF NOT EXISTS idx_tracks_genre ON tracks(genre);

CREATE VIRTUAL TABLE IF NOT EXISTS tracks_fts USING fts5(
	title, artist, album, genre,
	content='tracks', content_rowid='id'
);
DROP TRIGGER IF EXISTS tracks_ai;
CREATE TRIGGER tracks_ai AFTER INSERT ON tracks BEGIN
	INSERT INTO tracks_fts(rowid, title, artist, album, genre)
	VALUES (new.id, new.title, new.artist, new.album, new.genre);
END;
DROP TRIGGER IF EXISTS tracks_ad;
CREATE TRIGGER tracks_ad AFTER DELETE ON tracks BEGIN
	INSERT INTO tracks_fts(tracks_fts, rowid, title, artist, album, genre)
	VALUES ('delete', old.id, old.title, old.artist, old.album, old.genre);
END;
DROP TRIGGER IF EXISTS tracks_au;
CREATE TRIGGER tracks_au AFTER UPDATE ON tracks BEGIN
	INSERT INTO tracks_fts(tracks_fts, rowid, title, artist, album, genre)
	VALUES ('delete', old.id, old.title, old.artist, old.album, old.genre);
	INSERT INTO tracks_fts(rowid, title, artist, album, genre)
	VALUES (new.id, new.title, new.artist, new.album, new.genre);
END;

CREATE TABLE IF NOT EXISTS plays (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	track_id INTEGER NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
	started_at INTEGER NOT NULL,
	duration_played_sec INTEGER NOT NULL DEFAULT 0,
	completed INTEGER NOT NULL DEFAULT 0,
	source TEXT NOT NULL DEFAULT 'local' -- 'local'|'spotify'|'apple'|...
);
CREATE INDEX IF NOT EXISTS idx_plays_track ON plays(track_id);
CREATE INDEX IF NOT EXISTS idx_plays_started ON plays(started_at);

CREATE TABLE IF NOT EXISTS playlists (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
CREATE TABLE IF NOT EXISTS playlist_items (
	playlist_id INTEGER NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
	track_id INTEGER NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
	position INTEGER NOT NULL,
	PRIMARY KEY (playlist_id, position)
);

CREATE TABLE IF NOT EXISTS recommendations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
	prompt_hash TEXT,
	payload TEXT NOT NULL -- JSON: {summary, items:[{title,artist,reason,track_id?}]}
);
CREATE INDEX IF NOT EXISTS idx_recommendations_hash ON recommendations(prompt_hash);

CREATE TABLE IF NOT EXISTS spotify_tokens (
	id INTEGER PRIMARY KEY CHECK (id=1),
	access_token TEXT NOT NULL,
	refresh_token TEXT NOT NULL,
	expires_at INTEGER NOT NULL,
	scope TEXT NOT NULL,
	user_id TEXT,
	display_name TEXT,
	product TEXT,
	last_synced_at INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

CREATE TABLE IF NOT EXISTS spotify_pkce (
	state TEXT PRIMARY KEY,
	code_verifier TEXT NOT NULL,
	created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

CREATE TABLE IF NOT EXISTS download_jobs (
	id TEXT PRIMARY KEY,
	url TEXT NOT NULL,
	output TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'queued',
	started_at INTEGER NOT NULL,
	finished_at INTEGER,
	error TEXT,
	tool TEXT,
	used_fallback INTEGER NOT NULL DEFAULT 0,
	is_search INTEGER NOT NULL DEFAULT 0,
	track_id INTEGER,
	created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
CREATE INDEX IF NOT EXISTS idx_download_jobs_status ON download_jobs(status);
-- idx_download_jobs_kind is created in Migrate() AFTER the additive ALTER TABLE
-- because existing installations' download_jobs table doesn't have the kind
-- column yet when this schema block runs (CREATE TABLE IF NOT EXISTS is a no-op
-- when the table exists, so the column from line 130 never lands).

CREATE TABLE IF NOT EXISTS podcast_feeds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    title TEXT,
    description TEXT,
    image_url TEXT,
    author TEXT,
    link TEXT,
    language TEXT,
    last_fetched_at INTEGER,
    last_error TEXT,
    auto_download INTEGER NOT NULL DEFAULT 0,
    download_folder TEXT,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
CREATE INDEX IF NOT EXISTS idx_podcast_feeds_url ON podcast_feeds(url);

CREATE TABLE IF NOT EXISTS podcast_episodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_id INTEGER NOT NULL REFERENCES podcast_feeds(id) ON DELETE CASCADE,
    guid TEXT NOT NULL,
    title TEXT,
    description TEXT,
    pub_date INTEGER,
    duration_sec INTEGER,
    audio_url TEXT,
    audio_type TEXT,
    audio_size INTEGER,
    downloaded INTEGER NOT NULL DEFAULT 0,
    file_path TEXT,
    file_size INTEGER,
    download_error TEXT,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    UNIQUE(feed_id, guid)
);
CREATE INDEX IF NOT EXISTS idx_podcast_episodes_feed ON podcast_episodes(feed_id);
CREATE INDEX IF NOT EXISTS idx_podcast_episodes_downloaded ON podcast_episodes(downloaded);

-- Apple Music integration (v3.4.0)
-- Credentials are entered via the Settings GUI and stored here so users don't
-- have to edit .env. The .p8 private key is stored plaintext (local single-user app).
CREATE TABLE IF NOT EXISTS apple_music_config (
    id INTEGER PRIMARY KEY CHECK (id=1),
    team_id TEXT NOT NULL,
    key_id TEXT NOT NULL,
    private_key TEXT NOT NULL,
    storefront TEXT NOT NULL DEFAULT 'us',
    cached_dev_token TEXT,
    cached_dev_token_expires_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

-- Apple Music User Token + sync cursor. The MUT comes from MusicKit JS in the
-- browser; we cannot mint or refresh it server-side. If Apple invalidates it
-- (user revoked, password change), /v1/me/* calls 401 and the user must
-- re-authorize via the Settings page.
CREATE TABLE IF NOT EXISTS apple_music_user (
    id INTEGER PRIMARY KEY CHECK (id=1),
    music_user_token TEXT NOT NULL,
    storefront TEXT NOT NULL,
    display_name TEXT,
    last_synced_at INTEGER NOT NULL DEFAULT 0,
    connected_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
`

// validTables is the set of known database table names, used to sanitize
// the table argument in PRAGMA queries that do not support parameterization.
var validTables = map[string]bool{
	"tracks":           true,
	"plays":            true,
	"playlists":        true,
	"playlist_items":   true,
	"recommendations":  true,
	"spotify_tokens":   true,
	"spotify_pkce":     true,
	"download_jobs":    true,
	"podcast_feeds":    true,
	"podcast_episodes": true,
	"tracks_fts":       true,
	"apple_music_config": true,
	"apple_music_user":   true,
}

// columnExists returns true if the given column already exists on the table.
func columnExists(db *sql.DB, table, column string) bool {
	if !validTables[table] {
		return false
	}
	if matched, _ := regexp.MatchString(`^[a-z_]+$`, column); !matched {
		log.Printf("[db] columnExists: invalid column name %q", column)
		return false
	}
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			log.Printf("[db] columnExists scan %s.%s: %v", table, column, err)
			return false
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		return false
	}
	return false
}

func Migrate(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	// Additive column migrations (idempotent)
	// Add type column to recommendations (for playlist cache differentiation)
	if !columnExists(db, "recommendations", "type") {
		if _, err := db.Exec(`ALTER TABLE recommendations ADD COLUMN type TEXT NOT NULL DEFAULT 'general'`); err != nil {
			return err
		}
	}

	if !columnExists(db, "tracks", "spotify_id") {
		if _, err := db.Exec(`ALTER TABLE tracks ADD COLUMN spotify_id TEXT`); err != nil {
			return err
		}
	}
	if !columnExists(db, "tracks", "external_url") {
		if _, err := db.Exec(`ALTER TABLE tracks ADD COLUMN external_url TEXT`); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_tracks_spotify ON tracks(spotify_id) WHERE spotify_id IS NOT NULL`); err != nil {
		return err
	}
	// Add kind column to download_jobs (existing rows default to 'music')
	if !columnExists(db, "download_jobs", "kind") {
		if _, err := db.Exec(`ALTER TABLE download_jobs ADD COLUMN kind TEXT NOT NULL DEFAULT 'music'`); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_download_jobs_kind ON download_jobs(kind)`); err != nil {
		return err
	}
	// Add loudness measurement columns
	if !columnExists(db, "tracks", "loudness_integrated") {
		if _, err := db.Exec(`ALTER TABLE tracks ADD COLUMN loudness_integrated REAL`); err != nil {
			return err
		}
	}
	if !columnExists(db, "tracks", "loudness_true_peak") {
		if _, err := db.Exec(`ALTER TABLE tracks ADD COLUMN loudness_true_peak REAL`); err != nil {
			return err
		}
	}
	if !columnExists(db, "tracks", "loudness_range") {
		if _, err := db.Exec(`ALTER TABLE tracks ADD COLUMN loudness_range REAL`); err != nil {
			return err
		}
	}
	// Add playback position tracking to podcast_episodes
	if !columnExists(db, "podcast_episodes", "playback_position_sec") {
		if _, err := db.Exec(`ALTER TABLE podcast_episodes ADD COLUMN playback_position_sec INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if !columnExists(db, "podcast_episodes", "listened") {
		if _, err := db.Exec(`ALTER TABLE podcast_episodes ADD COLUMN listened INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	// Apple Music: per-track id column + unique partial index (mirrors spotify_id pattern)
	if !columnExists(db, "tracks", "apple_id") {
		if _, err := db.Exec(`ALTER TABLE tracks ADD COLUMN apple_id TEXT`); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_tracks_apple ON tracks(apple_id) WHERE apple_id IS NOT NULL`); err != nil {
		return err
	}
	return nil
}
