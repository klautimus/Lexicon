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
	apple_id TEXT,
	user_id INTEGER REFERENCES users(id) ON DELETE SET NULL
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
	source TEXT NOT NULL DEFAULT 'local', -- 'local'|'spotify'|'apple'|...
	user_id INTEGER REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_plays_track ON plays(track_id);
CREATE INDEX IF NOT EXISTS idx_plays_started ON plays(started_at);

CREATE TABLE IF NOT EXISTS playlists (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	description TEXT,
	cover_art_path TEXT,
	created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
	user_id INTEGER REFERENCES users(id) ON DELETE CASCADE
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
	payload TEXT NOT NULL, -- JSON: {summary, items:[{title,artist,reason,track_id?}]}
	user_id INTEGER REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_recommendations_hash ON recommendations(prompt_hash);

CREATE TABLE IF NOT EXISTS spotify_tokens (
	id INTEGER PRIMARY KEY,
	access_token TEXT NOT NULL,
	refresh_token TEXT NOT NULL,
	expires_at INTEGER NOT NULL,
	scope TEXT NOT NULL,
	user_id TEXT,
	display_name TEXT,
	product TEXT,
	last_synced_at INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
	lexicon_user_id INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS spotify_pkce (
	state TEXT PRIMARY KEY,
	code_verifier TEXT NOT NULL,
	created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
	lexicon_user_id INTEGER NOT NULL DEFAULT 1
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
	created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
	user_id INTEGER REFERENCES users(id) ON DELETE SET NULL
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
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    user_id INTEGER REFERENCES users(id) ON DELETE SET NULL
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

-- User authentication (v3.6.0)
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'user',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

-- Apple Music integration (v3.4.0)
-- Credentials are entered via the Settings GUI and stored here so users don't
-- have to edit .env. The .p8 private key is stored plaintext (local single-user app).
CREATE TABLE IF NOT EXISTS apple_music_config (
    id INTEGER PRIMARY KEY,
    team_id TEXT NOT NULL,
    key_id TEXT NOT NULL,
    private_key TEXT NOT NULL,
    storefront TEXT NOT NULL DEFAULT 'us',
    cached_dev_token TEXT,
    cached_dev_token_expires_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    lexicon_user_id INTEGER NOT NULL DEFAULT 1
);

-- Apple Music User Token + sync cursor. The MUT comes from MusicKit JS in the
-- browser; we cannot mint or refresh it server-side. If Apple invalidates it
-- (user revoked, password change), /v1/me/* calls 401 and the user must
-- re-authorize via the Settings page.
CREATE TABLE IF NOT EXISTS apple_music_user (
    id INTEGER PRIMARY KEY,
    music_user_token TEXT NOT NULL,
    storefront TEXT NOT NULL,
    display_name TEXT,
    last_synced_at INTEGER NOT NULL DEFAULT 0,
    connected_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    lexicon_user_id INTEGER NOT NULL DEFAULT 1
);
`

// validTables is the set of known database table names, used to sanitize
// the table argument in PRAGMA queries that do not support parameterization.
var validTables = map[string]bool{
	"users":             true,
	"tracks":            true,
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
	// Add download progress fields (for real-time progress bar)
	if !columnExists(db, "download_jobs", "progress") {
		if _, err := db.Exec(`ALTER TABLE download_jobs ADD COLUMN progress REAL NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if !columnExists(db, "download_jobs", "progress_label") {
		if _, err := db.Exec(`ALTER TABLE download_jobs ADD COLUMN progress_label TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
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
// Add user_id columns for multi-user support.
// Uses columnExists() pattern for idempotent migration.
// DEFAULT NULL preserves backward compatibility — existing data
// is assigned to the default admin user below.

// Ensure users table columns exist (for databases upgraded from earlier versions).
if !columnExists(db, "users", "display_name") {
if _, err := db.Exec(`ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT ''`); err != nil {
return err
}
}
if !columnExists(db, "users", "role") {
if _, err := db.Exec(`ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user'`); err != nil {
return err
}
}

if !columnExists(db, "tracks", "user_id") {
		if _, err := db.Exec(`ALTER TABLE tracks ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`); err != nil {
			return err
		}
	}
	if !columnExists(db, "plays", "user_id") {
		if _, err := db.Exec(`ALTER TABLE plays ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`); err != nil {
			return err
		}
	}
	if !columnExists(db, "playlists", "user_id") {
		if _, err := db.Exec(`ALTER TABLE playlists ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE CASCADE`); err != nil {
			return err
		}
	}
	// Playlist cover art and description fields (v3.5.2)
	if !columnExists(db, "playlists", "description") {
		if _, err := db.Exec(`ALTER TABLE playlists ADD COLUMN description TEXT`); err != nil {
			return err
		}
	}
	if !columnExists(db, "playlists", "cover_art_path") {
		if _, err := db.Exec(`ALTER TABLE playlists ADD COLUMN cover_art_path TEXT`); err != nil {
			return err
		}
	}
	if !columnExists(db, "podcast_feeds", "user_id") {
		if _, err := db.Exec(`ALTER TABLE podcast_feeds ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`); err != nil {
			return err
		}
	}
	if !columnExists(db, "podcast_feeds", "auto_download") {
		if _, err := db.Exec(`ALTER TABLE podcast_feeds ADD COLUMN auto_download INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if !columnExists(db, "download_jobs", "user_id") {
		if _, err := db.Exec(`ALTER TABLE download_jobs ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`); err != nil {
			return err
		}
	}
	if !columnExists(db, "recommendations", "user_id") {
		if _, err := db.Exec(`ALTER TABLE recommendations ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`); err != nil {
			return err
		}
	}

	// Create default admin user if no users exist yet.
	// All existing data (NULL user_id) gets assigned to this user.
	var userCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&userCount); err != nil {
		return err
	}
	if userCount == 0 {
		// Default credentials: admin / admin. The user will be prompted
		// to change this on first login (implemented in I2 auth layer).
		res, err := db.Exec(`INSERT INTO users(username, password_hash, display_name, role)
			VALUES('admin', '$2a$10$.nz9maCiy/ytbqiQzKXe4uj45W65CfdAOE4lo0mvJzO0j9f8v1LdK', 'Admin', 'admin')`)
		if err != nil {
			return err
		}
		adminID, _ := res.LastInsertId()

		// Assign all existing data to the default admin user.
		for _, table := range []string{"tracks", "plays", "playlists", "podcast_feeds", "download_jobs", "recommendations"} {
			if _, err := db.Exec(`UPDATE `+table+` SET user_id=? WHERE user_id IS NULL`, adminID); err != nil {
				return err
			}
		}
	}

	// Indexes for user_id on frequently-filtered tables
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_tracks_user ON tracks(user_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_plays_user ON plays(user_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_playlists_user ON playlists(user_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_recommendations_user ON recommendations(user_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_download_jobs_user ON download_jobs(user_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_podcast_feeds_user ON podcast_feeds(user_id)`); err != nil {
		return err
	}

	// OAuth multi-user: add lexicon_user_id columns to token/config tables.
	if !columnExists(db, "spotify_tokens", "lexicon_user_id") {
		if _, err := db.Exec(`ALTER TABLE spotify_tokens ADD COLUMN lexicon_user_id INTEGER NOT NULL DEFAULT 1`); err != nil {
			return err
		}
	}
	if !columnExists(db, "spotify_pkce", "lexicon_user_id") {
		if _, err := db.Exec(`ALTER TABLE spotify_pkce ADD COLUMN lexicon_user_id INTEGER NOT NULL DEFAULT 1`); err != nil {
			return err
		}
	}
	if !columnExists(db, "apple_music_config", "lexicon_user_id") {
		if _, err := db.Exec(`ALTER TABLE apple_music_config ADD COLUMN lexicon_user_id INTEGER NOT NULL DEFAULT 1`); err != nil {
			return err
		}
	}
	if !columnExists(db, "apple_music_user", "lexicon_user_id") {
		if _, err := db.Exec(`ALTER TABLE apple_music_user ADD COLUMN lexicon_user_id INTEGER NOT NULL DEFAULT 1`); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_spotify_tokens_luid ON spotify_tokens(lexicon_user_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_apple_config_luid ON apple_music_config(lexicon_user_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_apple_user_luid ON apple_music_user(lexicon_user_id)`); err != nil {
		return err
	}

	return nil
}
