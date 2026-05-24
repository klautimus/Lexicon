package db

import (
	"database/sql"
	"fmt"
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
	path TEXT NOT NULL,
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
	file_sha256 TEXT,
	user_id INTEGER REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_tracks_artist ON tracks(artist);
CREATE INDEX IF NOT EXISTS idx_tracks_album ON tracks(album);
CREATE INDEX IF NOT EXISTS idx_tracks_kind ON tracks(media_kind);
CREATE INDEX IF NOT EXISTS idx_tracks_genre ON tracks(genre);
CREATE INDEX IF NOT EXISTS idx_tracks_path ON tracks(path);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tracks_user_path ON tracks(user_id, path) WHERE user_id IS NOT NULL;

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
    download_folder TEXT,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
CREATE INDEX IF NOT EXISTS idx_podcast_feeds_url ON podcast_feeds(url);

CREATE TABLE IF NOT EXISTS podcast_subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    feed_id INTEGER NOT NULL REFERENCES podcast_feeds(id) ON DELETE CASCADE,
    auto_download INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    UNIQUE(user_id, feed_id)
);
CREATE INDEX IF NOT EXISTS idx_podcast_subs_user ON podcast_subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_podcast_subs_feed ON podcast_subscriptions(feed_id);

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
    file_path TEXT,
    file_size INTEGER,
    download_error TEXT,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    UNIQUE(feed_id, guid)
);
CREATE INDEX IF NOT EXISTS idx_podcast_episodes_feed ON podcast_episodes(feed_id);

CREATE TABLE IF NOT EXISTS podcast_episode_status (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    episode_id INTEGER NOT NULL REFERENCES podcast_episodes(id) ON DELETE CASCADE,
    downloaded INTEGER NOT NULL DEFAULT 0,
    playback_position_sec INTEGER NOT NULL DEFAULT 0,
    listened INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    UNIQUE(user_id, episode_id)
);
CREATE INDEX IF NOT EXISTS idx_podcast_ep_status_user ON podcast_episode_status(user_id);
CREATE INDEX IF NOT EXISTS idx_podcast_ep_status_ep ON podcast_episode_status(episode_id);

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
	"podcast_feeds":              true,
	"podcast_subscriptions":     true,
	"podcast_episodes":          true,
	"podcast_episode_status":    true,
	"tracks_fts":                true,
	"apple_music_config": true,
	"apple_music_user":   true,
}

// columnExists returns true if the given column already exists on the table.
func columnExists(db *sql.DB, table, column string) bool {
	if !validTables[table] {
		return false
	}
	if matched, _ := regexp.MatchString(`^[a-z0-9_]+$`, column); !matched {
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
	log.Printf("[db] starting migration...")
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	log.Printf("[db] schema executed successfully")
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
	// Podcast playback position/listened moved to podcast_episode_status in v3.7.0.
	// The schema constant above creates podcast_episode_status with these columns.
	// No migration needed — existing data will be handled by the podcast multi-user migration below.
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
	// Podcast feeds user_id/auto_download removed in v3.7.0 (moved to podcast_subscriptions).
	// The schema constant above creates podcast_feeds without these columns.
	// The migration block below handles upgrading existing installations.

	// Podcast multi-user migration (v3.7.0): separate shared feed data from
	// per-user subscriptions. Creates podcast_subscriptions and podcast_episode_status
	// tables, migrates existing data, then recreates podcast_feeds and podcast_episodes
	// without per-user columns.
	{
		var podcastMigrated int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='podcast_subscriptions'`).Scan(&podcastMigrated); err != nil {
			return fmt.Errorf("podcast migration: check subscriptions table: %w", err)
		}
		log.Printf("[db] podcast migration: podcast_subscriptions exists=%d", podcastMigrated)
		if podcastMigrated == 0 {
			log.Printf("[db] podcast migration: RUNNING migration block")
			log.Printf("[db] podcast multi-user migration: creating subscription tables...")

			// Create subscription tables (new installs get these from schema constant,
			// but existing installs need them created here before data migration)
			if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS podcast_subscriptions (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				feed_id INTEGER NOT NULL REFERENCES podcast_feeds(id) ON DELETE CASCADE,
				auto_download INTEGER NOT NULL DEFAULT 0,
				created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
				UNIQUE(user_id, feed_id)
			)`); err != nil {
				return fmt.Errorf("podcast migration: create subscriptions: %w", err)
			}
			if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_podcast_subs_user ON podcast_subscriptions(user_id)`); err != nil {
				return err
			}
			if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_podcast_subs_feed ON podcast_subscriptions(feed_id)`); err != nil {
				return err
			}

			if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS podcast_episode_status (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				episode_id INTEGER NOT NULL REFERENCES podcast_episodes(id) ON DELETE CASCADE,
				downloaded INTEGER NOT NULL DEFAULT 0,
				playback_position_sec INTEGER NOT NULL DEFAULT 0,
				listened INTEGER NOT NULL DEFAULT 0,
				created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
				UNIQUE(user_id, episode_id)
			)`); err != nil {
				return fmt.Errorf("podcast migration: create episode_status: %w", err)
			}
			if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_podcast_ep_status_user ON podcast_episode_status(user_id)`); err != nil {
				return err
			}
			if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_podcast_ep_status_ep ON podcast_episode_status(episode_id)`); err != nil {
				return err
			}

			// Migrate existing feeds: create subscriptions for feeds with user_id
			_, err := db.Exec(`INSERT OR IGNORE INTO podcast_subscriptions(user_id, feed_id, auto_download)
				SELECT user_id, id, auto_download FROM podcast_feeds WHERE user_id IS NOT NULL`)
			if err != nil {
				return fmt.Errorf("podcast migration: migrate feed subscriptions: %w", err)
			}

			// Migrate existing episodes: create status for downloaded episodes
			// Join with podcast_feeds to get the user_id
			_, err = db.Exec(`INSERT OR IGNORE INTO podcast_episode_status(user_id, episode_id, downloaded)
				SELECT f.user_id, e.id, e.downloaded
				FROM podcast_episodes e
				JOIN podcast_feeds f ON f.id = e.feed_id
				WHERE e.downloaded = 1 AND f.user_id IS NOT NULL`)
			if err != nil {
				return fmt.Errorf("podcast migration: migrate episode status: %w", err)
			}

			// Recreate podcast_feeds without user_id and auto_download
			if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
				return err
			}
			if _, err := db.Exec(`
				CREATE TABLE podcast_feeds_v2 (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					url TEXT NOT NULL UNIQUE,
					title TEXT, description TEXT, image_url TEXT, author TEXT,
					link TEXT, language TEXT, last_fetched_at INTEGER, last_error TEXT,
					download_folder TEXT,
					created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
				)
			`); err != nil {
				return fmt.Errorf("podcast migration: create feeds_v2: %w", err)
			}
			if _, err := db.Exec(`INSERT INTO podcast_feeds_v2 SELECT id, url, title, description, image_url, author, link, language, last_fetched_at, last_error, download_folder, created_at FROM podcast_feeds`); err != nil {
				return fmt.Errorf("podcast migration: copy feeds: %w", err)
			}
			if _, err := db.Exec(`DROP TABLE podcast_feeds`); err != nil {
				return fmt.Errorf("podcast migration: drop old feeds: %w", err)
			}
			if _, err := db.Exec(`ALTER TABLE podcast_feeds_v2 RENAME TO podcast_feeds`); err != nil {
				return fmt.Errorf("podcast migration: rename feeds: %w", err)
			}
			if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_podcast_feeds_url ON podcast_feeds(url)`); err != nil {
				return err
			}

			// Recreate podcast_episodes without downloaded column
			if _, err := db.Exec(`
				CREATE TABLE podcast_episodes_v2 (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					feed_id INTEGER NOT NULL REFERENCES podcast_feeds(id) ON DELETE CASCADE,
					guid TEXT NOT NULL,
					title TEXT, description TEXT, pub_date INTEGER,
					duration_sec INTEGER, audio_url TEXT, audio_type TEXT, audio_size INTEGER,
					file_path TEXT, file_size INTEGER, download_error TEXT,
					created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
					UNIQUE(feed_id, guid)
				)
			`); err != nil {
				return fmt.Errorf("podcast migration: create episodes_v2: %w", err)
			}
			if _, err := db.Exec(`INSERT INTO podcast_episodes_v2 SELECT id, feed_id, guid, title, description, pub_date, duration_sec, audio_url, audio_type, audio_size, file_path, file_size, download_error, created_at FROM podcast_episodes`); err != nil {
				return fmt.Errorf("podcast migration: copy episodes: %w", err)
			}
			if _, err := db.Exec(`DROP TABLE podcast_episodes`); err != nil {
				return fmt.Errorf("podcast migration: drop old episodes: %w", err)
			}
			if _, err := db.Exec(`ALTER TABLE podcast_episodes_v2 RENAME TO podcast_episodes`); err != nil {
				return fmt.Errorf("podcast migration: rename episodes: %w", err)
			}
			if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_podcast_episodes_feed ON podcast_episodes(feed_id)`); err != nil {
				return err
			}

			// Update subscription foreign key to point to new feeds table
			// (SQLite doesn't enforce FK after table recreation, but the data is intact)

			if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
				return err
			}

			log.Printf("[db] podcast multi-user migration: complete")
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

		// Assign existing data to the default admin user.
		// Playlists are NOT assigned — they remain with user_id IS NULL
		// so all users can see legacy playlists (household sharing model).
		for _, table := range []string{"tracks", "plays", "download_jobs", "recommendations"} {
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

	// file_sha256 column for file-level dedup — must be added BEFORE the dedup
	// migration below, so that the old table has the same column count as tracks_v2
	// when INSERT INTO tracks_v2 SELECT * FROM tracks runs.
	if !columnExists(db, "tracks", "file_sha256") {
		if _, err := db.Exec(`ALTER TABLE tracks ADD COLUMN file_sha256 TEXT`); err != nil {
			return fmt.Errorf("dedup: add file_sha256: %w", err)
		}
	}

	// Cross-user download dedup (v3.7.0): remove UNIQUE constraint on tracks.path,
	// add UNIQUE(user_id, path) for cross-user file sharing.
	// For existing databases, recreate the tracks table without the UNIQUE.
	// For fresh installs, the updated const schema already has the correct structure.
	{
		var dedupDone int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='dedup_migration_done'`).Scan(&dedupDone); err != nil {
			return fmt.Errorf("dedup migration: check marker: %w", err)
		}
		if dedupDone == 0 {
			var trackCount int
			if err := db.QueryRow(`SELECT COUNT(*) FROM tracks`).Scan(&trackCount); err != nil {
				return fmt.Errorf("dedup migration: count tracks: %w", err)
			}
			if trackCount > 0 {
				log.Printf("[db] dedup migration: recreating tracks table (%d rows) to remove UNIQUE on path...", trackCount)
				// Disable foreign keys during migration
				if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
					return fmt.Errorf("dedup migration: disable FK: %w", err)
				}
				// Create new table without UNIQUE on path
				if _, err := db.Exec(`
					CREATE TABLE tracks_v2 (
						id INTEGER PRIMARY KEY AUTOINCREMENT,
						path TEXT NOT NULL,
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
						media_kind TEXT NOT NULL DEFAULT 'music',
						cover_path TEXT,
						added_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
						mtime INTEGER,
						spotify_id TEXT,
						external_url TEXT,
						apple_id TEXT,
						file_sha256 TEXT,
						loudness_integrated REAL,
						loudness_true_peak REAL,
						loudness_range REAL,
						user_id INTEGER REFERENCES users(id) ON DELETE SET NULL
					)
				`); err != nil {
					return fmt.Errorf("dedup migration: create tracks_v2: %w", err)
				}
				// Copy all data
				if _, err := db.Exec(`INSERT INTO tracks_v2 SELECT * FROM tracks`); err != nil {
					return fmt.Errorf("dedup migration: copy data: %w", err)
				}
				// Drop old table
				if _, err := db.Exec(`DROP TABLE tracks`); err != nil {
					return fmt.Errorf("dedup migration: drop old tracks: %w", err)
				}
				// Rename
				if _, err := db.Exec(`ALTER TABLE tracks_v2 RENAME TO tracks`); err != nil {
					return fmt.Errorf("dedup migration: rename: %w", err)
				}
				// Rebuild indexes
				if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_tracks_artist ON tracks(artist)`); err != nil {
					return err
				}
				if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_tracks_album ON tracks(album)`); err != nil {
					return err
				}
				if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_tracks_kind ON tracks(media_kind)`); err != nil {
					return err
				}
				if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_tracks_genre ON tracks(genre)`); err != nil {
					return err
				}
				if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_tracks_path ON tracks(path)`); err != nil {
					return err
				}
				if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_tracks_user_path ON tracks(user_id, path) WHERE user_id IS NOT NULL`); err != nil {
					return err
				}
				// Rebuild FTS5 triggers (dropped when old table was dropped)
				if _, err := db.Exec(`DROP TRIGGER IF EXISTS tracks_ai`); err != nil {
					return err
				}
				if _, err := db.Exec(`CREATE TRIGGER tracks_ai AFTER INSERT ON tracks BEGIN
					INSERT INTO tracks_fts(rowid, title, artist, album, genre)
					VALUES (new.id, new.title, new.artist, new.album, new.genre);
				END`); err != nil {
					return err
				}
				if _, err := db.Exec(`DROP TRIGGER IF EXISTS tracks_ad`); err != nil {
					return err
				}
				if _, err := db.Exec(`CREATE TRIGGER tracks_ad AFTER DELETE ON tracks BEGIN
					INSERT INTO tracks_fts(tracks_fts, rowid, title, artist, album, genre)
					VALUES ('delete', old.id, old.title, old.artist, old.album, old.genre);
				END`); err != nil {
					return err
				}
				if _, err := db.Exec(`DROP TRIGGER IF EXISTS tracks_au`); err != nil {
					return err
				}
				if _, err := db.Exec(`CREATE TRIGGER tracks_au AFTER UPDATE ON tracks BEGIN
					INSERT INTO tracks_fts(tracks_fts, rowid, title, artist, album, genre)
					VALUES ('delete', old.id, old.title, old.artist, old.album, old.genre);
					INSERT INTO tracks_fts(rowid, title, artist, album, genre)
					VALUES (new.id, new.title, new.artist, new.album, new.genre);
				END`); err != nil {
					return err
				}
				// Rebuild FTS5 index
				if _, err := db.Exec(`INSERT INTO tracks_fts(tracks_fts) VALUES('rebuild')`); err != nil {
					return fmt.Errorf("dedup migration: rebuild FTS5: %w", err)
				}
				// Re-enable foreign keys
				if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
					return fmt.Errorf("dedup migration: enable FK: %w", err)
				}
				log.Printf("[db] dedup migration: complete. tracks table recreated with UNIQUE(user_id, path).")
			}
			// Mark migration as done
			if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS dedup_migration_done (done_at INTEGER NOT NULL)`); err != nil {
				return fmt.Errorf("dedup migration: create marker: %w", err)
			}
			if _, err := db.Exec(`INSERT INTO dedup_migration_done VALUES (strftime('%s','now'))`); err != nil {
				return fmt.Errorf("dedup migration: insert marker: %w", err)
			}
		}
	}

	// dedup columns on download_jobs for tracking which track was used as source
	if !columnExists(db, "download_jobs", "dedup_source_track_id") {
		if _, err := db.Exec(`ALTER TABLE download_jobs ADD COLUMN dedup_source_track_id INTEGER`); err != nil {
			return fmt.Errorf("dedup: add dedup_source_track_id: %w", err)
		}
	}
	if !columnExists(db, "download_jobs", "dedup_method") {
		if _, err := db.Exec(`ALTER TABLE download_jobs ADD COLUMN dedup_method TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("dedup: add dedup_method: %w", err)
		}
	}

	return nil
}
