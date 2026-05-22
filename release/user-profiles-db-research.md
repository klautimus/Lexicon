# Lexicon Multi-User Database Migration Strategy

> Research document for task t_1cdf3bd0 — May 21, 2026
> Author: Atlas (researcher profile)
> Source code reviewed: db.go (347 LOC), config.go (100 LOC), auth/middleware.go (51 LOC), main.go (579 LOC), scanner.go (198 LOC)

---

## 1. Current State Audit

### 1.1 Database Engine

SQLite via `modernc.org/sqlite` (pure-Go driver). Opened with:

```
?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)
```

WAL mode, 5s busy timeout, foreign keys enforced. All safe for concurrent access.

### 1.2 Current Auth Model

- `LEXICON_API_KEY` env var → Bearer token middleware (`backend/internal/auth/middleware.go`)
- Key is read once at startup via `auth.SetAPIKey()` (called after `godotenv.Load()`)
- If key is unset, middleware is a no-op — all requests pass through unauthenticated
- No user concept exists anywhere in the codebase
- The `spotify_tokens.user_id` column stores *Spotify's* user ID, not a Lexicon user ID

### 1.3 Current Tables (13 tables + 1 FTS5 virtual table)

| Table | Rows | Ownership | Notes |
|-------|------|-----------|-------|
| `tracks` | 1 per file | None | File-indexed, UNIQUE(path) |
| `tracks_fts` | 1 per track | None | FTS5 virtual, mirrors tracks |
| `plays` | 1 per listen | None | FK → tracks(id) |
| `playlists` | 1 per playlist | None | Named collection |
| `playlist_items` | 1 per track-in-list | Via playlist_id | FK → playlists(id), tracks(id) |
| `recommendations` | 1 per generation | None | Cached AI responses |
| `spotify_tokens` | 1 (CHECK id=1) | None | Single-user constraint |
| `spotify_pkce` | transient | None | OAuth state storage |
| `download_jobs` | 1 per download | None | Fire-and-forget jobs |
| `podcast_feeds` | 1 per RSS feed | None | Podcast subscriptions |
| `podcast_episodes` | 1 per episode | Via feed_id | FK → podcast_feeds(id) |
| `apple_music_config` | 1 (CHECK id=1) | None | Per-install credentials |
| `apple_music_user` | 1 (CHECK id=1) | None | Single MUT |

### 1.4 Existing Migration Pattern (from db.go lines 265-347)

```go
func Migrate(db *sql.DB) error {
    // 1. Run CREATE TABLE IF NOT EXISTS schema (idempotent)
    db.Exec(schema)
    
    // 2. Additive column migrations using columnExists()
    if !columnExists(db, "table", "column") {
        db.Exec(`ALTER TABLE table ADD COLUMN column ...`)
    }
    
    // 3. Create indexes with IF NOT EXISTS
    db.Exec(`CREATE INDEX IF NOT EXISTS ...`)
}
```

This pattern is safe, idempotent, and backwards-compatible. Every existing installation runs Migrate() on startup.

---

## 2. Core Design Decision: Shared vs. Per-User Track Library

This is the single most important architectural decision. It determines whether `tracks` gets `user_id` and cascades into FTS5, scanner, and API design.

### Option A: Shared tracks (RECOMMENDED)

**Tracks are global. All users see the same file library. Playlists, history, recommendations, downloads, podcasts are per-user.**

Rationale:
- Lexicon is a **desktop app** running on a single machine. Media files are on a shared filesystem (`MEDIA_ROOTS`). Two users on the same PC would scan the same directories. Duplicating track records per user wastes DB space and creates sync problems.
- The scanner walks `MEDIA_ROOTS` — it doesn't know about users. Making tracks per-user requires either: (a) per-user scan roots (complex, breaks the "just point at your music folder" UX), or (b) scanning once and copying track records per user (wasteful, fragile).
- FTS5 stays simple — one global index.
- Deduplication is natural: one file on disk = one track record.

### Option B: Per-user tracks

Every user has their own copy of track records. Would require per-user media roots and per-user FTS5 indexes. Rejected because:
- Doubles/triples DB size for every user (with large libraries this matters — 50K tracks × 3 users = 150K rows)
- Scanner must know which user to attribute files to
- File deduplication across users becomes a problem (same file scanned 3 times = 3 track records)
- FTS5 must be per-user (either separate virtual tables or composite with user_id column)
- UX complexity: "whose music is this?" is not a question a desktop music app should ask

### Decision: **Option A — Shared tracks, per-user everything else.**

---

## 3. Users Table Schema

```sql
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    is_admin INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);
```

Notes:
- `password_hash` = bcrypt (cost 12). The Go stdlib `golang.org/x/crypto/bcrypt` handles this.
- `is_admin` = 0/1 boolean. Admin users can manage other accounts (delete, reset password). First user created is always admin.
- No email field — this is a local desktop app, not a web service. Auth is local password only.
- No session tokens table yet — can be added later with JWT or session cookies. For v1, basic auth or API-key-per-user is sufficient.

---

## 4. Per-Table Migration Plan

### 4.1 Summary: Which tables get `user_id`

| Table | Gets user_id? | Default value | Rationale |
|-------|--------------|---------------|-----------|
| `tracks` | **NO** | N/A | Shared library — all users see same files |
| `tracks_fts` | **NO** | N/A | Mirrors tracks, stays global |
| `plays` | **YES** | 1 | Play history is per-user |
| `playlists` | **YES** | 1 | Playlists owned by specific users |
| `playlist_items` | **NO** (indirect) | Via playlist | Ownership via playlist_id → playlists.user_id |
| `recommendations` | **YES** | 1 | Generated for specific users |
| `download_jobs` | **YES** | 1 | Initiated by specific users |
| `podcast_feeds` | **YES** | 1 | Subscriptions are per-user |
| `podcast_episodes` | **NO** (indirect) | Via feed | Ownership via feed_id → podcast_feeds.user_id |
| `spotify_tokens` | **YES** | 1 (and remove CHECK) | Each user connects their own Spotify |
| `spotify_pkce` | **YES** | 1 | OAuth state must be per-user |
| `apple_music_config` | **YES** | 1 (and remove CHECK) | Each user has own Apple Music creds |
| `apple_music_user` | **NO** (indirect) | Via config | Ownership via config_id → apple_music_config.user_id |

### 4.2 Detailed ALTER TABLE Statements

All migrations use the `columnExists()` pattern. They are additive only — no columns dropped, no tables restructured.

#### plays
```sql
ALTER TABLE plays ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id);
CREATE INDEX IF NOT EXISTS idx_plays_user ON plays(user_id);
```

#### playlists
```sql
ALTER TABLE playlists ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id);
CREATE INDEX IF NOT EXISTS idx_playlists_user ON playlists(user_id);
```

#### recommendations
```sql
ALTER TABLE recommendations ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id);
CREATE INDEX IF NOT EXISTS idx_recommendations_user ON recommendations(user_id);
```

#### download_jobs
```sql
ALTER TABLE download_jobs ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id);
CREATE INDEX IF NOT EXISTS idx_download_jobs_user ON download_jobs(user_id);
```

#### podcast_feeds
```sql
ALTER TABLE podcast_feeds ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id);
CREATE INDEX IF NOT EXISTS idx_podcast_feeds_user ON podcast_feeds(user_id);
```

#### spotify_tokens
```sql
-- Remove single-user constraint (can't ALTER CONSTRAINT in SQLite, recreate approach below)
ALTER TABLE spotify_tokens ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_spotify_tokens_user ON spotify_tokens(user_id);
```

**⚠️ Special handling for spotify_tokens:** The table has `CHECK (id=1)` which enforces single-row. In SQLite you cannot ALTER a CHECK constraint. The migration must:
1. Add `user_id` column (default 1)
2. Create unique index on `user_id` (ensures one token per user going forward)
3. The CHECK constraint remains but becomes harmless — future INSERTs will use `user_id` as the unique key instead of `id=1`
4. Update Go code to query by `user_id` instead of `id=1`

#### spotify_pkce
```sql
ALTER TABLE spotify_pkce ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS idx_spotify_pkce_user ON spotify_pkce(user_id);
```

#### apple_music_config
```sql
-- Same constraint issue as spotify_tokens
ALTER TABLE apple_music_config ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_apple_config_user ON apple_music_config(user_id);
```

---

## 5. Default User Creation

### Migration Insert (in Migrate function)

```sql
INSERT OR IGNORE INTO users (id, username, password_hash, display_name, is_admin)
VALUES (1, 'default', '', 'Default User', 1);
```

`INSERT OR IGNORE` makes this idempotent — subsequent runs skip if user 1 exists.

The default user has:
- Empty password hash (cannot log in — forces account creation)
- Admin privileges (can create other users)
- Display name "Default User"

### First-Run UX

On first launch after upgrade:
1. App detects only the default user exists (id=1, empty password_hash)
2. Settings page shows "Create your account" prompt
3. User sets username + password → bcrypt hash stored, display_name updated
4. All existing data (plays, playlists, etc.) is already under user_id=1 — no migration needed
5. Optionally: "Transfer to new user" button to move data from user 1 to a newly created user 2

---

## 6. Media Files: Shared Storage Strategy

### Design

**Files on disk are shared. Track records are shared (no user_id on tracks). Per-user metadata references shared tracks by ID.**

```
                    ┌─────────────────────┐
                    │   MEDIA_ROOTS disk   │
                    │  /mnt/music/         │
                    │    song1.flac        │
                    │    song2.mp3         │
                    └──────┬──────────────┘
                           │ scanner indexes once
                           ▼
                    ┌─────────────────────┐
                    │   tracks table       │
                    │   id=1: song1.flac   │
                    │   id=2: song2.mp3    │
                    └──────┬──────────────┘
                           │ referenced by
              ┌────────────┼────────────┐
              ▼            ▼            ▼
        user 1 plays   user 2 plays   shared playlists
        (user_id=1)    (user_id=2)    reference track ids
```

### Benefits

- **Deduplication**: 50K files = 50K track records regardless of user count
- **Simplicity**: Scanner doesn't change. FTS5 doesn't change.
- **Consistency**: All users always see the same library state
- **Performance**: Queries like "top tracks" can aggregate across users without JOINing user-filtered track tables

### Trade-offs

- Users cannot have private tracks (files one user can see but others can't)
- If private tracks are needed later, add a `tracks.owner_user_id` column (nullable, NULL = shared)
- This is unlikely for a desktop app on a shared machine

---

## 7. FTS5 Strategy: Global Index

### Decision: Keep global FTS5

Since tracks are shared, keeping a single `tracks_fts` virtual table is the correct choice. The triggers (INSERT/UPDATE/DELETE on tracks → sync to FTS5) remain unchanged.

If per-user search filtering is needed (e.g., "search only my playlists"), that's handled by JOINing against the relevant per-user table:

```sql
-- Search across all tracks
SELECT t.* FROM tracks t
JOIN tracks_fts fts ON t.id = fts.rowid
WHERE tracks_fts MATCH 'beatles';

-- Search only tracks in user's playlists
SELECT DISTINCT t.* FROM tracks t
JOIN tracks_fts fts ON t.id = fts.rowid
JOIN playlist_items pi ON pi.track_id = t.id
JOIN playlists p ON p.id = pi.playlist_id
WHERE tracks_fts MATCH 'beatles' AND p.user_id = ?;
```

No changes needed to FTS5 triggers or virtual table definition.

---

## 8. Auth Flow Changes

### Current: Bearer token (LEXICON_API_KEY)

Single static key. Middleware is a no-op when unset.

### Proposed: Username/password → JWT or session token

For a desktop app, there are two viable approaches:

**Approach A: JWT tokens (recommended)**
- User logs in → server returns signed JWT
- JWT contains `{user_id, username, is_admin, exp}`
- Middleware validates JWT, extracts user_id into request context
- Benefits: stateless, no session table, standard

**Approach B: Session tokens**
- User logs in → server generates random token, stores in `sessions` table
- Middleware looks up token → gets user_id
- Benefits: revocable, simpler to implement
- Downsides: needs sessions table, token cleanup

### New middleware: `RequireAuth` (replaces `RequireAPIKey`)

```go
func RequireAuth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := extractBearerToken(r)
        claims, err := validateJWT(token)
        if err != nil {
            http.Error(w, `{"error":"unauthorized"}`, 401)
            return
        }
        ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### Login endpoint (new)

```
POST /api/auth/login
Body: {"username": "...", "password": "..."}
Response: {"token": "eyJ...", "user": {"id": 1, "username": "...", "display_name": "..."}}
```

### Backward compatibility

During transition, `RequireAuth` falls back to `RequireAPIKey` behavior:
- If no user accounts exist (only default user with empty password), all requests pass through as user_id=1
- If `LEXICON_API_KEY` is set and no auth header present, fall back to API key auth with user_id=1
- This means existing single-user installations continue working without any login UI

---

## 9. Migration Script Implementation

### Complete `Migrate()` additions (in order)

```go
// 1. Create users table
if !tableExists(db, "users") {
    db.Exec(`CREATE TABLE IF NOT EXISTS users (...)`);
}

// 2. Insert default user (idempotent)
db.Exec(`INSERT OR IGNORE INTO users (id, username, password_hash, display_name, is_admin) 
         VALUES (1, 'default', '', 'Default User', 1)`);

// 3. Add user_id to each per-user table (using columnExists pattern)

// plays
if !columnExists(db, "plays", "user_id") {
    db.Exec(`ALTER TABLE plays ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id)`);
    db.Exec(`CREATE INDEX IF NOT EXISTS idx_plays_user ON plays(user_id)`);
}

// playlists
if !columnExists(db, "playlists", "user_id") {
    db.Exec(`ALTER TABLE playlists ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id)`);
    db.Exec(`CREATE INDEX IF NOT EXISTS idx_playlists_user ON playlists(user_id)`);
}

// recommendations
if !columnExists(db, "recommendations", "user_id") {
    db.Exec(`ALTER TABLE recommendations ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id)`);
    db.Exec(`CREATE INDEX IF NOT EXISTS idx_recommendations_user ON recommendations(user_id)`);
}

// download_jobs
if !columnExists(db, "download_jobs", "user_id") {
    db.Exec(`ALTER TABLE download_jobs ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id)`);
    db.Exec(`CREATE INDEX IF NOT EXISTS idx_download_jobs_user ON download_jobs(user_id)`);
}

// podcast_feeds
if !columnExists(db, "podcast_feeds", "user_id") {
    db.Exec(`ALTER TABLE podcast_feeds ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id)`);
    db.Exec(`CREATE INDEX IF NOT EXISTS idx_podcast_feeds_user ON podcast_feeds(user_id)`);
}

// spotify_tokens
if !columnExists(db, "spotify_tokens", "user_id") {
    db.Exec(`ALTER TABLE spotify_tokens ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id)`);
    db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_spotify_tokens_user ON spotify_tokens(user_id)`);
}

// spotify_pkce
if !columnExists(db, "spotify_pkce", "user_id") {
    db.Exec(`ALTER TABLE spotify_pkce ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1`);
    db.Exec(`CREATE INDEX IF NOT EXISTS idx_spotify_pkce_user ON spotify_pkce(user_id)`);
}

// apple_music_config
if !columnExists(db, "apple_music_config", "user_id") {
    db.Exec(`ALTER TABLE apple_music_config ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id)`);
    db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_apple_config_user ON apple_music_config(user_id)`);
}
```

### New helper: `tableExists()`

```go
func tableExists(db *sql.DB, table string) bool {
    var count int
    db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
    return count > 0
}
```

Add to `validTables` map: `"users": true`.

### Safety Properties

- **Idempotent**: Every statement is guarded by `columnExists` or `tableExists` or uses `IF NOT EXISTS`/`OR IGNORE`
- **WAL mode**: Already active, no change
- **Foreign keys**: Already ON, no change
- **Backward compatible**: All new columns have `DEFAULT 1` — existing code that doesn't filter by user_id continues to see all data (under user 1)
- **No data loss**: No columns dropped, no tables restructured
- **Rollback-safe**: All statements are individual ALTER TABLEs — if one fails, previous ones persist (acceptable for additive migrations)

---

## 10. Code Changes Required (Scope Estimate)

### Backend packages that need modification

| Package | Change | Effort |
|---------|--------|--------|
| `db/db.go` | Add users table, user_id migrations, tableExists helper | Medium |
| `auth/middleware.go` | Replace RequireAPIKey with RequireAuth, add JWT logic | Large |
| `auth/` (new) | Add login handler, JWT sign/verify, password hashing | Large |
| `config/config.go` | Add JWT_SECRET, optional SESSION_DURATION | Small |
| `playlists/playlists.go` | Filter by user_id from context, set user_id on create | Medium |
| `recommender/recommender.go` | Filter/scope by user_id | Medium |
| `downloader/downloader.go` | Set user_id on job creation | Small |
| `podcaster/podcaster.go` | Filter feeds/episodes by user_id | Medium |
| `spotify/spotify.go` | Multi-user tokens, query by user_id | Large |
| `spotify/oauth.go` | Pass user_id through OAuth flow | Medium |
| `apple/apple.go` | Multi-user config, query by user_id | Medium |
| `history/history.go` | Set user_id on play recording, filter queries | Medium |
| `analytics/analytics.go` | Add user_id filter (optional, admin sees all) | Small |
| `cmd/server/main.go` | Wire new auth middleware, mount login route | Small |

### Frontend changes

| Area | Change | Effort |
|------|--------|--------|
| Login page (new) | Username/password form, token storage | Medium |
| Settings page | User management (admin: create/delete users) | Large |
| App.tsx | Auth guard, redirect to login if unauthenticated | Medium |
| API client | Attach JWT to all requests | Small |
| All pages | No changes needed (user_id is transparent via API) | None |

### Total estimated scope: ~1500-2000 lines of new/changed code across 15+ files.

---

## 11. Recommended Implementation Order

### Phase 1: Database foundation (this task's output)
- [x] Research complete (this document)
- [ ] Implement users table + all user_id migrations in db.go
- [ ] Write migration tests

### Phase 2: Auth system
- [ ] JWT sign/verify in auth package
- [ ] Login endpoint
- [ ] Replace middleware
- [ ] Frontend login page

### Phase 3: Per-package user scoping
- [ ] playlists: filter by user_id
- [ ] plays/history: record + filter by user_id
- [ ] recommendations: scope by user_id
- [ ] podcast_feeds: filter by user_id

### Phase 4: Multi-user OAuth (Spotify + Apple)
- [ ] spotify_tokens: remove single-row constraint, query by user_id
- [ ] spotify_pkce: associate with user
- [ ] apple_music_config: remove single-row constraint, query by user_id

### Phase 5: UI + polish
- [ ] Settings: user management for admins
- [ ] First-run account creation flow
- [ ] Backward compatibility testing

---

## 12. Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|-----------|
| spotify_tokens CHECK(id=1) blocks multi-user inserts | Spotify OAuth breaks for user 2+ | Update all INSERTs to use user_id as unique key; CHECK becomes harmless (id can be any value) |
| apple_music_config CHECK(id=1) same issue | Apple Music breaks for user 2+ | Same approach as spotify_tokens |
| Existing code doesn't filter by user_id → all users see each other's data | Privacy leak | Add user_id WHERE clauses incrementally (Phase 3); until then, all data visible to all users (acceptable for family desktop app) |
| JWT secret management | Token forgery if weak | Generate random 256-bit secret on first run, store in DB. Desktop app doesn't need env var. |
| Large DB with many users | Slow queries without indexes | Every user_id FK gets an index. SQLite handles thousands of users fine for a desktop app. |
| Backward compat: old frontend with new backend | API changes break existing install | Login is additive — old frontend ignores it. API responses include user_id but old clients ignore unknown fields. |

---

## 13. Open Questions for Kevin

1. **Auth method preference**: JWT (stateless, standard) vs. session tokens (simpler, revocable)? JWT recommended for desktop app.

2. **Admin vs. regular users**: What can admins do that regular users can't? Proposed: create/delete accounts, view all users' analytics. Regular users: only their own data.

3. **"Private tracks" future**: Should we reserve a nullable `tracks.owner_user_id` column now (NULL = shared), or add it later if needed? Recommend: add later — don't pre-optimize.

4. **First-run flow**: Auto-create account prompt, or manual setup via Settings? Recommend: Settings page banner that won't dismiss until account created.

5. **API key backward compat**: Keep `LEXICON_API_KEY` as a fallback for headless/debug mode? Recommend: yes — if API key is set and no JWT provided, authenticate as user_id=1.

---

## Appendix A: Complete Migration SQL Reference

```sql
-- ============================================================
-- Lexicon Multi-User Migration — SQL Reference
-- All statements are idempotent (guarded by IF NOT EXISTS / OR IGNORE)
-- ============================================================

-- 1. Users table
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    is_admin INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

-- 2. Default user (migration seed)
INSERT OR IGNORE INTO users (id, username, password_hash, display_name, is_admin)
VALUES (1, 'default', '', 'Default User', 1);

-- 3. Per-user columns (each guarded by columnExists in Go)
ALTER TABLE plays ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id);
CREATE INDEX IF NOT EXISTS idx_plays_user ON plays(user_id);

ALTER TABLE playlists ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id);
CREATE INDEX IF NOT EXISTS idx_playlists_user ON playlists(user_id);

ALTER TABLE recommendations ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id);
CREATE INDEX IF NOT EXISTS idx_recommendations_user ON recommendations(user_id);

ALTER TABLE download_jobs ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id);
CREATE INDEX IF NOT EXISTS idx_download_jobs_user ON download_jobs(user_id);

ALTER TABLE podcast_feeds ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id);
CREATE INDEX IF NOT EXISTS idx_podcast_feeds_user ON podcast_feeds(user_id);

ALTER TABLE spotify_tokens ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_spotify_tokens_user ON spotify_tokens(user_id);

ALTER TABLE spotify_pkce ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS idx_spotify_pkce_user ON spotify_pkce(user_id);

ALTER TABLE apple_music_config ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_apple_config_user ON apple_music_config(user_id);
```

---

## Appendix B: Tables NOT Modified

| Table | Why not |
|-------|---------|
| `tracks` | Shared library — all users see same files |
| `tracks_fts` | Mirrors tracks, stays global |
| `playlist_items` | Ownership via playlist_id FK chain |
| `podcast_episodes` | Ownership via feed_id FK chain |
| `apple_music_user` | Ownership via config_id → apple_music_config FK chain |

---

*End of research document. Ready for implementation in db.go Migrate() function.*
