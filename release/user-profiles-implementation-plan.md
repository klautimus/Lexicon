# Lexicon Multi-User Profiles — Implementation Plan

**Date:** 2026-05-21
**Author:** Atlas (P1 synthesis)
**Inputs:** R1 (auth research), R2 (DB migration research), R3 (frontend UX research)
**Status:** Synthesis — maps all three research documents onto current codebase state, identifies gap-to-done

---

## 0. Current State Assessment

Substantial multi-user infrastructure already exists in the codebase (~60-70% complete). This plan focuses on the remaining gaps, format mismatches, and integration work needed to ship a working multi-user system.

### What's Already Done

| Layer | Component | Status |
|-------|-----------|--------|
| DB | `users` table with `id`, `username`, `password_hash`, `display_name`, `is_admin`, `role`, `created_at` | ✅ db.go:200-209 |
| DB | `user_id` columns on `tracks`, `plays`, `playlists`, `podcast_feeds`, `download_jobs`, `recommendations` | ✅ db.go:58,94,103,117,153,175 |
| DB | Indexes on all `user_id` columns | ✅ db.go:445-463 |
| DB | Default admin user creation in `Migrate()` | ✅ db.go:421-443 |
| Auth | `sessions.go` — in-memory session store (sync.Map) | ✅ sessions.go:24-96 |
| Auth | `passwords.go` — bcrypt hash/verify (DefaultCost=10) | ✅ passwords.go:1-18 |
| Auth | `middleware.go` — RequireAuth, RequireAdmin, UserFromContext, extractToken | ✅ middleware.go:1-139 |
| Auth | `handlers.go` — login (with first-run auto-create), logout, me, createUser, listUsers | ✅ handlers.go:1-253 |
| Auth | Session cleanup goroutine | ✅ sessions.go:82-97 |
| Main | `authHandler.Mount(r)`, auth middleware skip-login, session cleanup wired | ✅ main.go:260-265,348-361,393 |
| Playlists | Scoped by `user_id` with backward-compat `(user_id IS NULL OR user_id=?)` | ✅ playlists.go:83,131,140,154,252,273 |
| History | Records `user_id` on INSERT, filters recent plays by `user_id` | ✅ history.go:82,96 |
| Spotify sync | Includes `user_id` in track/play inserts | ✅ spotify/sync.go:176,203 |
| Apple sync | Includes `user_id` in track/play inserts | ✅ apple/sync.go:166,197 |
| Frontend | `UserContext.tsx` — provider with login/logout/loading/isAdmin | ✅ UserContext.tsx:1-72 |
| Frontend | `LoginPage.tsx` — full login form with error states, show/hide password | ✅ LoginPage.tsx:1-131 |
| Frontend | `AdminUsersPage.tsx` — user list, create form, delete with confirmation | ✅ AdminUsersPage.tsx:1-279 |
| Frontend | `App.tsx` — AuthGuard, routes for /login and /settings/users | ✅ App.tsx:92-109,254,193 |
| Frontend | DesktopLayout: user indicator in sidebar, admin shield, logout button | ✅ App.tsx:144-176 |
| Frontend | MobileLayout: user bar at top with admin shield, logout | ✅ App.tsx:207-225 |
| Frontend | `api.ts` — session token management, auth API methods | ✅ api.ts:1-27,216-229,450-460 |

### Known Gaps & Issues (what this plan addresses)

| # | Issue | Severity | Phase |
|---|-------|----------|-------|
| G1 | Sessions are in-memory only (sync.Map), lost on restart. No `sessions` SQLite table. | High | P1 |
| G2 | `spotify_tokens` still has `CHECK (id=1)` — no `user_id` column. Multi-user Spotify OAuth broken. | High | P1 |
| G3 | `apple_music_config` still has `CHECK (id=1)` — no `user_id` column. Multi-user Apple Music broken. | High | P1 |
| G4 | Backend `me` returns `{id, username, role}`. Frontend expects `{ user: {id, username, display_name, is_admin} }`. | Critical | P2 |
| G5 | Backend `createUserReq` has `Role` field. Frontend sends `display_name`. Missing field. | Critical | P2 |
| G6 | Backend `listUsers` returns `{id, username, role, created_at}`. Frontend expects `is_admin` boolean and `display_name`. | Critical | P2 |
| G7 | Backend missing `DELETE /api/auth/users/{id}` handler. | High | P2 |
| G8 | Provider hierarchy: `UserProvider` inside `PlayerProvider`/`DownloadProvider`/`HelpProvider`. Those mount before auth check. | High | P3 |
| G9 | `passwords.go` uses `bcrypt.DefaultCost` (10) instead of recommended cost 12. | Medium | P1 |
| G10 | `spotify_pkce` no `user_id` column. PKCE state not per-user. | Medium | P1 |
| G11 | Spotify OAuth queries use `WHERE id=1` — won't work with multi-user tokens. | High | P2 |
| G12 | Per-package user scoping not done for: recommender, downloader, podcaster, analytics. | Medium | P4 |
| G13 | `login` handler auto-creates admin on empty users table, but migration already creates default admin. Potential double-create race. | Low | P2 |
| G14 | `display_name` column exists in DB but `createUser` handler doesn't store it. | Medium | P2 |
| G15 | No `last_synced_at` column handling for multi-user spotify_tokens. | Low | P2 |

---

## Phase 1: Database Foundation

**Goal:** Complete all DB schema changes — sessions table, OAuth multi-user columns, bcrypt cost fix.

### 1.1 Add `sessions` table (db.go:197-199)

**File:** `backend/internal/db/db.go`

Add to the `schema` constant, after the `users` table block (around line 209):

```sql
-- Session tokens (v3.6.0 — multi-user)
CREATE TABLE IF NOT EXISTS sessions (
    token TEXT PRIMARY KEY,              -- 64-char hex string
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
```

**Rationale:** R3 research recommended simple UUID tokens in SQLite (section 8, line 617-622). R1 research recommended SQLite sessions for restart survival (section 3, line 80-89). In-memory sync.Map is fine for dev but loses all sessions on restart — unacceptable for "remember me."

### 1.2 Replace in-memory session store with SQLite-backed store (sessions.go)

**File:** `backend/internal/auth/sessions.go`

Replace the entire file. The sync.Map store becomes DB-backed:

- `SetSession(token, u)` → `INSERT INTO sessions(token, user_id, created_at, expires_at)`
- `GetSession(token)` → `SELECT user_id, username, role FROM sessions JOIN users ... WHERE token=? AND expires_at > ?`
- `DeleteSession(token)` → `DELETE FROM sessions WHERE token=?`
- `CleanupSessions()` → `DELETE FROM sessions WHERE expires_at < ?`
- `StartSessionCleanup(interval)` → stays the same (ticker + goroutine)
- Need a DB handle. Add `InitSessionStore(db *sql.DB)` called at startup.

**Line references:**
- `sessions.go:24` — add `var sessionDB *sql.DB`
- `sessions.go:30` — add `func InitSessionStore(db *sql.DB) { sessionDB = db }`
- `sessions.go:40-44` — replace `sessionStore.Store()` with DB INSERT
- `sessions.go:50-61` — replace `sessionStore.Load()` with DB SELECT + JOIN
- `sessions.go:64-66` — replace `sessionStore.Delete()` with DB DELETE
- `sessions.go:70-77` — replace `sessionStore.Range()` with DB DELETE WHERE expired
- **main.go:261** — add `auth.InitSessionStore(database)` before `auth.NewHandler(database)`

### 1.3 Add `user_id` to spotify_tokens (db.go:121-132)

**File:** `backend/internal/db/db.go`

Currently `spotify_tokens` has `CHECK (id=1)` enforcing single-row. SQLite cannot ALTER a CHECK constraint.

**Migration in `Migrate()` (around line 419):**

```go
if !columnExists(db, "spotify_tokens", "user_id") {
    if _, err := db.Exec(`ALTER TABLE spotify_tokens ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id)`); err != nil {
        return err
    }
}
if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_spotify_tokens_user ON spotify_tokens(user_id)`); err != nil {
    return err
}
```

The `CHECK (id=1)` remains but becomes harmless — the unique index on `user_id` is the real constraint going forward.

### 1.4 Add `user_id` to spotify_pkce (db.go:134-138)

```go
if !columnExists(db, "spotify_pkce", "user_id") {
    if _, err := db.Exec(`ALTER TABLE spotify_pkce ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1`); err != nil {
        return err
    }
}
if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_spotify_pkce_user ON spotify_pkce(user_id)`); err != nil {
    return err
}
```

### 1.5 Add `user_id` to apple_music_config (db.go:214-224)

```go
if !columnExists(db, "apple_music_config", "user_id") {
    if _, err := db.Exec(`ALTER TABLE apple_music_config ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id)`); err != nil {
        return err
    }
}
if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_apple_config_user ON apple_music_config(user_id)`); err != nil {
    return err
}
```

### 1.6 Fix bcrypt cost from DefaultCost(10) → 12 (passwords.go:7)

**File:** `backend/internal/auth/passwords.go`, line 7

Change:
```go
bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
```
To:
```go
bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
```

**Rationale:** R1 research section 2 recommended cost 12 (~250ms, OWASP-compliant). DefaultCost is 10 (too fast).

### 1.7 Add `validTables` entry for `sessions` (db.go:242-257)

Add `"sessions": true` to the `validTables` map.

### Phase 1 Estimated Effort: ~3 hours
### Phase 1 Files Changed:
- `backend/internal/db/db.go` — schema + migration additions
- `backend/internal/auth/sessions.go` — rewrite to DB-backed
- `backend/internal/auth/passwords.go` — cost 10→12
- `backend/cmd/server/main.go` — add InitSessionStore call at line 261

---

## Phase 2: Auth Backend Completion

**Goal:** Fix all format mismatches between backend handlers and frontend API client. Add missing endpoints.

### 2.1 Fix `me` handler response format (handlers.go:161-168)

**Problem:** `me()` returns `UserInfo` directly (`{id, username, role}`). Frontend `api.me()` expects `{ user: {id, username, display_name, is_admin} }`.

**Fix in handlers.go:161-168:**
```go
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
    u, ok := UserFromContext(r.Context())
    if !ok {
        writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
        return
    }
    // Fetch display_name and is_admin from DB
    var displayName string
    var isAdmin bool
    h.db.QueryRowContext(r.Context(),
        `SELECT display_name, is_admin FROM users WHERE id=?`, u.UserID,
    ).Scan(&displayName, &isAdmin)
    writeJSON(w, http.StatusOK, map[string]interface{}{
        "user": map[string]interface{}{
            "id":           u.UserID,
            "username":     u.Username,
            "display_name": displayName,
            "is_admin":     isAdmin || u.Role == "admin",
        },
    })
}
```

### 2.2 Fix `createUser` handler to accept `display_name` (handlers.go:58-62,170-213)

**Problem:** `createUserReq` has `Role` field. Frontend sends `{username, password, display_name}`. Missing `display_name`.

**Fix:**
1. **Line 58-62:** Add `DisplayName` to `createUserReq`:
```go
type createUserReq struct {
    Username    string `json:"username"`
    Password    string `json:"password"`
    DisplayName string `json:"display_name"`
    Role        string `json:"role"`
}
```

2. **Line 180-182:** Add `DisplayName` default:
```go
if req.DisplayName == "" {
    req.DisplayName = req.Username
}
```

3. **Line 195-197:** Include `display_name` in INSERT:
```go
res, err := h.db.ExecContext(r.Context(),
    `INSERT INTO users (username, password_hash, display_name, role) VALUES (?, ?, ?, ?)`,
    req.Username, hash, req.DisplayName, req.Role)
```

4. **Line 208-212:** Return `display_name` and `is_admin` in response:
```go
writeJSON(w, http.StatusCreated, map[string]interface{}{
    "user": map[string]interface{}{
        "id":           id,
        "username":     req.Username,
        "display_name": req.DisplayName,
        "is_admin":     req.Role == "admin",
    },
})
```

### 2.3 Fix `listUsers` to return `display_name` and `is_admin` (handlers.go:215-243)

**Problem:** Returns `{id, username, role, created_at}`. Frontend `User` type expects `is_admin: boolean` and `display_name`.

**Fix in handlers.go:216-217:**
```go
rows, err := h.db.QueryContext(r.Context(),
    `SELECT id, username, display_name, is_admin, role, created_at FROM users ORDER BY id`)
```

**Fix in handlers.go:228:**
```go
if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.IsAdmin, &u.Role, &u.CreatedAt); err != nil {
```

**Update `userRow` struct (handlers.go:64-69):**
```go
type userRow struct {
    ID          int64  `json:"id"`
    Username    string `json:"username"`
    DisplayName string `json:"display_name"`
    IsAdmin     bool   `json:"is_admin"`
    Role        string `json:"role"`
    CreatedAt   int64  `json:"created_at"`
}
```

### 2.4 Add `deleteUser` handler (handlers.go: after line 243)

**Problem:** Frontend calls `DELETE /api/auth/users/{id}` but no backend handler exists.

**Add to `Mount()` (handlers.go:36-43):**
```go
r.Delete("/users/{id}", h.deleteUser)
```

**New handler:**
```go
func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
    id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
    if err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
        return
    }
    // Prevent admin from deleting themselves
    currentUser, _ := UserFromContext(r.Context())
    if currentUser != nil && currentUser.UserID == id {
        writeJSON(w, http.StatusForbidden, map[string]string{"error": "cannot delete yourself"})
        return
    }
    res, err := h.db.ExecContext(r.Context(), `DELETE FROM users WHERE id=?`, id)
    if err != nil {
        log.Printf("[auth] deleteUser: %v", err)
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
        return
    }
    if n, _ := res.RowsAffected(); n == 0 {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
        return
    }
    writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
```

Add `strconv` and `chi` imports to handlers.go if not already present (chi already imported line 10).

### 2.5 Fix `login` handler first-run logic (handlers.go:86-120)

**Problem:** Migration at db.go:421-443 creates a default admin user (`admin` / hardcoded bcrypt hash). But `login` handler at line 86-120 has a "first run" path that auto-creates an admin when `SELECT COUNT(*) FROM users` returns 0. These conflict — after migration, the count is 1, so the first-run path never fires. But what if migration didn't run?

**Fix:** Remove the first-run auto-create logic from `login` handler. It's cleaner to have migration handle admin creation. Users log in with the default admin credentials, then change them.

**Remove lines 86-120** and replace with a simple check:
```go
// Check if default admin exists with empty/weak password
var isDefault bool
h.db.QueryRowContext(r.Context(),
    `SELECT password_hash='' OR password_hash='$2a$10$.nz9maCiy/ytbqiQzKXe4uj45W65CfdAOE4lo0mvJzO0j9f8v1LdK' FROM users WHERE id=1`,
).Scan(&isDefault)
```

If `isDefault`, the frontend should prompt password change. (This can be a follow-up — not blocking.)

**Simplest fix for now:** Keep the first-run auto-create BUT guard it with a check for whether the migration's default admin already exists. If default admin exists and `userCount == 1`, skip auto-create and proceed to normal login.

### 2.6 Add `strconv` import to handlers.go

Line 7 currently imports:
```go
import (
    "database/sql"
    "encoding/json"
    "log"
    "net/http"
    "strings"

    "github.com/go-chi/chi/v5"
)
```

Add `"strconv"` to the stdlib imports.

### Phase 2 Estimated Effort: ~2 hours
### Phase 2 Files Changed:
- `backend/internal/auth/handlers.go` — response format fixes, deleteUser, createUser display_name, listUsers fields
- `backend/internal/auth/sessions.go` — (Phase 1 already rewrites this)

---

## Phase 3: Frontend Integration

**Goal:** Fix provider hierarchy, wire up session persistence, verify end-to-end login flow.

### 3.1 Fix provider hierarchy (App.tsx:267-283)

**Problem:** `UserProvider` is inside `PlayerProvider`, `DownloadProvider`, `HelpProvider` (line 274). This means those contexts mount before auth is checked — PlayerContext starts fetching tracks before the user logs in. The UX research (R3, section 1, lines 44-62) recommended `UserProvider` at the top, gating everything.

**Current (broken):**
```tsx
<ErrorBoundary>
  <ToastProvider>
    <PlayerProvider>       // ← mounts before auth!
      <DownloadProvider>   // ← mounts before auth!
        <HelpProvider>     // ← mounts before auth!
          <UserProvider>
            <AppContent />
          </UserProvider>
        </HelpProvider>
      </DownloadProvider>
    </PlayerProvider>
  </ToastProvider>
</ErrorBoundary>
```

**Fixed (App.tsx:267-283):**
```tsx
<ErrorBoundary>
  <ToastProvider>
    <UserProvider>           // ← FIRST — gates everything
      <AppContent />
    </UserProvider>
  </ToastProvider>
</ErrorBoundary>
```

And inside `AppContent()` (currently line 249-265), wrap the authenticated portion:
```tsx
function AppContent() {
  const { user, loading } = useUser();
  const isMobile = useIsMobile();

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen bg-bg">
        <Loader2 size={24} className="animate-spin text-muted" />
      </div>
    );
  }

  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="*"
        element={
          user ? (
            // Only mount these AFTER auth
            <PlayerProvider>
              <DownloadProvider>
                <HelpProvider>
                  {isMobile ? <MobileLayout /> : <DesktopLayout />}
                </HelpProvider>
              </DownloadProvider>
            </PlayerProvider>
          ) : (
            <Navigate to="/login" replace />
          )
        }
      />
    </Routes>
  );
}
```

And simplify the top-level `App()` to:
```tsx
export default function App() {
  return (
    <ErrorBoundary>
      <ToastProvider>
        <UserProvider>
          <AppContent />
        </UserProvider>
      </ToastProvider>
    </ErrorBoundary>
  );
}
```

**Remove `AuthGuard`** (App.tsx:92-109) — it's replaced by the conditional render in AppContent.

### 3.2 Fix `api.me()` response handling (UserContext.tsx:29-33)

**Problem:** `api.me()` calls `j<{ user: User }>('/auth/me')`. Currently backend returns `UserInfo` directly. After Phase 2.1 fix, backend will return `{ user: {...} }`. The frontend code line 32 does `setUser(data.user)` — this is correct IF backend returns the wrapped format. Verify after Phase 2.1 is done.

**Line 32 is already correct:**
```typescript
setUser(data.user);
```

### 3.3 Fix `UserContext` isAdmin check (UserContext.tsx:59)

**Current:** `const isAdmin = user?.is_admin ?? false;`

The backend `login` response has `user.role` not `user.is_admin`. After Phase 2.1 fixes, `me()` will return `user.is_admin`. But `login()` returns `UserInfo` directly — need to ensure it also returns `is_admin`.

**Fix:** After Phase 2 backend changes, verify `loginResp.User` includes `is_admin`. The backend `loginResp` type at handlers.go:53-56 should be updated:

```go
type loginResp struct {
    Token string   `json:"token"`
    User  struct {
        ID          int64  `json:"id"`
        Username    string `json:"username"`
        DisplayName string `json:"display_name"`
        IsAdmin     bool   `json:"is_admin"`
        Role        string `json:"role"`
    } `json:"user"`
}
```

### 3.4 Session persistence ("Remember Me") (api.ts:4-13, UserContext.tsx:21-43)

**Current state:** Token is saved to `localStorage` on login (api.ts:9). On mount, UserContext reads it and validates with `api.me()` (UserContext.tsx:23-34). This IS the "remember me" pattern — no explicit checkbox, just always persistent.

**Verdict:** This works. The R3 research's "Remember Me" checkbox (section 1, lines 117-122) is a nice-to-have but current behavior (always persistent) is acceptable for a desktop app. Defer the checkbox to v3.7.

### 3.5 Verify `api.users()` response handling (AdminUsersPage.tsx:39-40,71-73)

**After Phase 2.3 fix:** Backend will return `User[]` with `is_admin` boolean and `display_name`. Frontend line 40 does `setUsers(data)` and line 71-72 does `setUsers((prev) => [...prev, data.user])`. These should work IF:

1. `api.users()` returns `User[]` directly (currently typed as `j<User[]>('/auth/users')` at line 224). ✅
2. `api.createUser()` returns `{ user: User }` (currently typed as `j<{ user: User }>('/auth/users', ...)` at line 226). ✅

### 3.6 Verify `api.deleteUser()` works (AdminUsersPage.tsx:92)

Frontend calls `api.deleteUser(userId)` at line 92. After Phase 2.4 fix, backend handles `DELETE /api/auth/users/{id}`. Response is `{ ok: true }` — frontend expects `j<{ ok: boolean }>` at api.ts:228. ✅

### Phase 3 Estimated Effort: ~2 hours
### Phase 3 Files Changed:
- `frontend/src/App.tsx` — fix provider hierarchy, remove AuthGuard
- `frontend/src/contexts/UserContext.tsx` — verify after backend fixes
- (No changes needed to LoginPage.tsx, AdminUsersPage.tsx if backend responses match)

---

## Phase 4: Multi-User OAuth (Spotify + Apple Music)

**Goal:** Each user gets their own Spotify/Apple Music connection. Remove single-row constraints.

### 4.1 Spotify OAuth: Multi-user tokens (spotify/oauth.go, spotify/client.go)

**Problem:** All queries use `WHERE id=1` for the single-row spotify_tokens table.
- `spotify/oauth.go:221` — status check: `SELECT ... FROM spotify_tokens WHERE id=1`
- `spotify/oauth.go:125-132` — token save: `INSERT INTO spotify_tokens(id, ...` with `ON CONFLICT(id) DO UPDATE`

**Fix pattern:** Replace `id=1` queries with `user_id=?` queries.

**spotify/oauth.go changes:**

1. **Line 125-133:** Change INSERT from `id=1` to use `user_id`:
```go
_, err = a.db.ExecContext(ctx,
    `INSERT INTO spotify_tokens(user_id, access_token, refresh_token, expires_at, scope, user_id_spotify, display_name, product, last_synced_at)
     VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
     ON CONFLICT(user_id) DO UPDATE SET
        access_token=excluded.access_token,
        refresh_token=excluded.refresh_token,
        expires_at=excluded.expires_at,
        scope=excluded.scope,
        user_id=excluded.user_id,
        display_name=excluded.display_name,
        product=excluded.product,
        last_synced_at=excluded.last_synced_at`,
    userID,  // from context
    ...)
```
Note: `spotify_tokens.user_id` conflicts with the Spotify API's `user_id` field. Rename the Spotify field to `spotify_user_id` or use a different column name. The existing column `spotify_tokens.user_id` on line 127 of db.go is TEXT — this stores Spotify's user ID, not the Lexicon user ID. The new migration column `user_id` (INTEGER FK) is the Lexicon user. Need to disambiguate.

**Resolution:** The existing `user_id TEXT` column on spotify_tokens (db.go:127) stores the Spotify user ID. Keep it as-is. The new migration column `lexicon_user_id` would be clearer, but the migration in Phase 1.3 already uses `user_id` for the FK. To avoid ambiguity in Go code, rename the TEXT column — but we can't rename in SQLite without a complex migration.

**Simplest fix:** The Spotify user_id column is rarely used (only for display). Rename the Go struct field and handle the ambiguity in queries:
- Keep DB column `user_id` (TEXT) for Spotify's user ID
- Add DB column `lexicon_user_id` (INTEGER FK) for the Lexicon user
- Update all queries to use `lexicon_user_id` for auth scoping

**Actually — the Phase 1.3 migration already adds `user_id INTEGER` as the FK column.** But the schema already has `user_id TEXT` for Spotify's user ID. This is a column name collision.

**Critical fix needed in Phase 1.3:** Use `lexicon_user_id` instead of `user_id` for the spotify_tokens FK column:

```go
if !columnExists(db, "spotify_tokens", "lexicon_user_id") {
    db.Exec(`ALTER TABLE spotify_tokens ADD COLUMN lexicon_user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id)`)
}
CREATE UNIQUE INDEX IF NOT EXISTS idx_spotify_tokens_lexicon_user ON spotify_tokens(lexicon_user_id);
```

**Update spotify/oauth.go queries:**
- Line 125: `INSERT INTO spotify_tokens(lexicon_user_id, access_token, ...)`
- Line 132: `ON CONFLICT(lexicon_user_id) DO UPDATE`
- Line 221: `WHERE lexicon_user_id=?` instead of `WHERE id=1`

**spotify/client.go:**
- `ValidAccessToken()` and `getAccessToken()` — change `WHERE id=1` to `WHERE lexicon_user_id=?`
- Pass `userID int64` parameter through to these functions

**spotify/spotify.go:**
- `Status()` handler — change query to `WHERE lexicon_user_id=?`
- `Disconnect()` handler — change DELETE to `WHERE lexicon_user_id=?`

### 4.2 Spotify PKCE: Associate with user (spotify/oauth.go:134-138)

**spotify/oauth.go line 134-138:** PKCE state INSERT currently has no user association.

**Fix:** Add `lexicon_user_id` to the INSERT:
```go
db.ExecContext(ctx,
    `INSERT INTO spotify_pkce(state, code_verifier, created_at, lexicon_user_id) VALUES(?,?,?,?)`,
    state, verifier, time.Now().Unix(), userID)
```

**spotify/oauth.go (callback handler):** When looking up PKCE state, filter by user_id:
```go
db.QueryRowContext(ctx,
    `SELECT code_verifier FROM spotify_pkce WHERE state=? AND lexicon_user_id=?`,
    state, userID)
```

### 4.3 Apple Music: Multi-user config (apple/apple.go, apple/token.go)

**Problem:** Same as Spotify — `CHECK (id=1)` on `apple_music_config` and `apple_music_user`.

**apple/apple.go — SaveConfig handler:**
Current INSERT uses `ON CONFLICT(id) DO UPDATE`. Change to `ON CONFLICT(lexicon_user_id) DO UPDATE` and include `lexicon_user_id` in the INSERT.

**apple/apple.go — Status handler:**
Change query from `WHERE id=1` to `WHERE lexicon_user_id=?`.

**apple/token.go — developer token generation:**
Reads config from `WHERE id=1`. Change to accept `userID` parameter and filter by `lexicon_user_id`.

**apple/apple.go — Connect handler:**
`apple_music_user` needs `lexicon_user_id` column too. Add migration in Phase 1:
```go
if !columnExists(db, "apple_music_user", "lexicon_user_id") {
    db.Exec(`ALTER TABLE apple_music_user ADD COLUMN lexicon_user_id INTEGER NOT NULL DEFAULT 1 REFERENCES users(id)`)
}
CREATE UNIQUE INDEX IF NOT EXISTS idx_apple_music_user_lexicon ON apple_music_user(lexicon_user_id);
```

### Phase 4 Estimated Effort: ~4 hours
### Phase 4 Files Changed:
- `backend/internal/db/db.go` — fix spotify/apple column naming (lexicon_user_id)
- `backend/internal/spotify/oauth.go` — multi-user queries
- `backend/internal/spotify/client.go` — multi-user token access
- `backend/internal/spotify/spotify.go` — multi-user status/disconnect
- `backend/internal/apple/apple.go` — multi-user config/status/connect
- `backend/internal/apple/token.go` — user-scoped config reads

---

## Phase 5: Per-Package User Scoping

**Goal:** All remaining packages filter data by user_id from auth context.

### 5.1 Recommender (recommender.go)

**Current state:** No user_id filtering. Recommendations are global.

**Changes:**
- `buildProfile()` — filter plays query by `user_id`
- `loadCached()` / `saveCached()` — include `user_id` in prompt_hash
- Playlist generation — filter by `user_id`
- Chat — filter by `user_id`

### 5.2 Downloader (downloader.go)

**Current state:** `Job.UserID` field exists (line 142) but is never set to the auth context user. All jobs are global.

**Changes:**
- `enqueue()` — set `job.UserID` from `auth.UserFromContext(r.Context())`
- `searchEnqueue()` — same
- Job listing endpoints — filter by `user_id` from context
- `upgradeTrack()` — set `user_id`

### 5.3 Podcaster (podcaster.go)

**Current state:** `podcast_feeds` has `user_id` column (db.go:175) but podcaster doesn't filter by it.

**Changes:**
- `listFeeds()` — filter by `user_id`
- `subscribe()` — save `user_id`
- `unsubscribe()` — check `user_id` ownership
- `syncPodcast()` — respect `user_id`

### 5.4 Analytics (analytics.go)

**Current state:** No user_id filtering. All stats are global aggregate.

**Changes:**
- `overview()` — filter plays by `user_id`
- `topArtists()` — filter by `user_id`
- `topTracks()` — filter by `user_id`
- `topGenres()` — filter by `user_id`
- `heatmap()` — filter by `user_id`
- Admin users can optionally see aggregate (all users) via a query param `?all=true`

### 5.5 History (history.go)

**Already done:** `user_id` recorded on INSERT (line 82) and filtered on SELECT (line 96). ✅

### 5.6 Playlists (playlists.go)

**Already done:** All queries use `(user_id IS NULL OR user_id=?)` pattern. ✅

### Phase 5 Estimated Effort: ~3 hours
### Phase 5 Files Changed:
- `backend/internal/recommender/recommender.go`
- `backend/internal/downloader/downloader.go`
- `backend/internal/podcaster/podcaster.go`
- `backend/internal/analytics/analytics.go`

---

## Integration Testing: End-to-End Flow

### Test Scenario: First Run → Admin Setup → Family Account → Login → Private Library

```
1. Fresh install / delete lexicon.db
2. Start Lexicon
3. DB migration runs → creates users table, default admin account
4. Login screen appears (no valid session token)
5. Log in as "admin" / "admin"
6. Backend validates bcrypt hash → creates session → returns token
7. Frontend stores token, sets UserContext
8. Main app renders with admin user indicator
9. Navigate to Settings → Family Accounts (admin-only)
10. Create family account "sarah" / password "sarah123"
11. Verify sarah appears in user list with "Member" role
12. Logout
13. Login page shown
14. Log in as "sarah" / "sarah123"
15. Verify Sarah sees the library (shared tracks)
16. Sarah creates a playlist → should be scoped to sarah's user_id
17. Logout, login as admin
18. Verify admin can't see sarah's playlist (user_id scoping)
19. Admin deletes sarah → playlist CASCADE deleted
```

### Test Scenario: Spotify Multi-User

```
1. Admin connects Spotify (Settings → Connect Spotify)
2. Admin disconnects
3. Sarah logs in → connects her Spotify account
4. Admin logs in → connects a different Spotify account
5. Verify each user has their own Spotify tokens in spotify_tokens table
6. Verify each user's sync pulls to their own plays/playlists
```

---

## Rollback Plan

### If migration fails:
- All `ALTER TABLE` statements are guarded by `columnExists()` — if one fails, previous ones persist (additive migration)
- No columns are dropped, no tables restructured
- `CREATE TABLE IF NOT EXISTS` for new tables (users, sessions) — safe to re-run
- Worst case: restore `lexicon.db` from backup (InnoSetup installer can backup on upgrade)

### If auth is broken:
- `RequireAuth` middleware falls through to unauthenticated if no API key is set (line 119 of middleware.go) — desktop backward compat preserved
- `LEXICON_API_KEY` still works as parallel auth (API key fallback in middleware.go:103-115)
- Can disable session auth by removing the middleware block from main.go:348-361

### If frontend login is broken:
- Login page is a route (`/login`) that can be bypassed by navigating directly to `/` with a valid session token
- `localStorage` token persists — clearing it forces login screen

---

## Dependency Order Summary

```
Phase 1 (DB foundation) ──── must complete first
    │
    ├── Phase 2 (auth backend) ──── depends on Phase 1 sessions table
    │       │
    │       ├── Phase 3 (frontend) ──── depends on Phase 2 response format fixes
    │       │
    │       └── Phase 4 (OAuth multi-user) ──── depends on Phase 1 DB columns
    │               │
    │               └── Phase 5 (per-package scoping) ──── depends on Phase 2 auth context
    │
    └── Integration testing ──── after all phases
```

### Can run in parallel:
- Phase 2 and Phase 4 can be done in parallel (different files, different concerns)
- Phase 3 and Phase 5 can be done in parallel (frontend vs backend packages)

---

## Estimated Total Effort: ~14 hours across 5 phases

| Phase | Hours | Files Changed | Risk |
|-------|-------|--------------|------|
| P1: DB Foundation | 3 | 4 | Low — additive migrations |
| P2: Auth Backend | 2 | 2 | Medium — response format changes |
| P3: Frontend Integration | 2 | 2 | Medium — provider hierarchy restructure |
| P4: OAuth Multi-User | 4 | 6 | High — complex query changes in spotify/apple |
| P5: Per-Package Scoping | 3 | 4 | Low — adding WHERE clauses |

---

## Appendix: Key File Locations

| File | Lines | Role |
|------|-------|------|
| `backend/internal/db/db.go` | 466 | Schema (lines 27-238), Migration (lines 292-466) |
| `backend/internal/auth/sessions.go` | 97 | In-memory session store (needs DB rewrite) |
| `backend/internal/auth/passwords.go` | 18 | bcrypt hash/verify |
| `backend/internal/auth/middleware.go` | 139 | RequireAuth, RequireAdmin, context helpers |
| `backend/internal/auth/handlers.go` | 253 | login, logout, me, createUser, listUsers (+ missing deleteUser) |
| `backend/cmd/server/main.go` | 590 | Wiring: auth middleware lines 348-361, authHandler.Mount line 393 |
| `backend/internal/spotify/oauth.go` | ~221 | PKCE flow, token storage (id=1 queries need fixing) |
| `backend/internal/spotify/client.go` | ~439 | Token access, API helpers (id=1 queries need fixing) |
| `backend/internal/spotify/spotify.go` | ~122 | Status/disconnect handlers |
| `backend/internal/apple/apple.go` | ~310 | Config/status/connect (id=1 queries need fixing) |
| `backend/internal/apple/token.go` | ~190 | Developer token (config read needs user_id param) |
| `frontend/src/App.tsx` | 283 | Provider hierarchy (lines 267-283 need reorder) |
| `frontend/src/contexts/UserContext.tsx` | 72 | Auth state provider |
| `frontend/src/pages/LoginPage.tsx` | 131 | Login form |
| `frontend/src/pages/AdminUsersPage.tsx` | 279 | Admin user management |
| `frontend/src/lib/api.ts` | 460 | API client, auth types lines 450-460 |
