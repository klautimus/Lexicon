# Lexicon Multi-User Profiles — Integration Review

**Reviewer:** Atlas (Kanban R4: t_51d52307)
**Date:** 2026-05-21
**Branch:** release1 (commits e457091..HEAD)
**Scope:** I1 (DB schema), I2 (auth backend), I3 (frontend auth UI)

---

## 1. Verdict

**MERGE WITH FIXES REQUIRED.** The foundational architecture is solid — auth middleware, session management, bcrypt hashing, DB migration, and route structure are all correctly implemented. However, there are **2 critical bugs** and **3 high-severity mismatches** that prevent the feature from working end-to-end. All are bounded (no cascade impact), so fixes can be applied in a single pass.

---

## 2. Verification Summary Table

| # | Test | Status | Notes |
|---|------|--------|-------|
| 1 | `go build ./internal/...` | ✅ PASS | Clean compile, no errors |
| 2 | `go build ./cmd/server` | ✅ PASS | Clean compile, no errors |
| 3 | `npm run build` (Windows) | ✅ PASS | tsc -b + vite build, 2414 modules, 0 errors |
| 4a | First-run → admin auto-created | ✅ CODE OK | `login()` handler detects empty users table, creates admin |
| 4b | Login flow | ⚠️ TYPE MISMATCH | `role` (string) sent from backend, `is_admin` (boolean) expected by frontend |
| 4c | Create family account | ✅ CODE OK | `createUser()` validates role, checks UNIQUE |
| 4d | Delete family account | 🔴 MISSING ENDPOINT | Frontend calls `DELETE /api/auth/users/:id` — no backend handler exists |
| 4e | Logout → login as family member | ✅ CODE OK | Session cleared, token removed from localStorage |
| 4f | Library isolation | ✅ BY DESIGN | Tracks are shared (filesystem-level), isolation is in logical constructs |
| 5 | Session token across API calls | ✅ CODE OK | Bearer header + cookie, j() helper injects token |
| 6 | Existing single-user data migration | ✅ CODE OK | Migrate() creates admin, backfills NULL user_id across 6 tables |
| 7a | Downloads scoped to user | ✅ CODE OK | `getUserID(r)` + `user_id` in download_jobs table |
| 7b | Playlists scoped to user | ✅ CODE OK | `user_id IS NULL OR user_id = ?` in all CRUD queries |
| 7c | Podcasts scoped to user | ✅ CODE OK | `user_id` in podcast_feeds table |
| 7d | Search scoped to user | ⚠️ NOT SCOPED | FTS search has no user_id filter — shared library by design |
| 7e | Playback/history scoped to user | ✅ CODE OK | `user_id` in plays INSERT + WHERE filter on history reads |
| 7f | Recommendations scoped to user | ✅ CODE OK | `user_id IS NULL OR user_id = ?` in cache lookup + insert |
| 7g | Analytics scoped to user | ✅ CODE OK | `user_id IS NULL OR user_id = ?` in all aggregate queries |
| 8 | Logout clears session + redirects | ✅ CODE OK | `DeleteSession(token)`, `setSessionToken(null)`, AuthGuard redirects to /login |
| 9 | Admin can create users | ✅ CODE OK | `createUser()` with role validation, UNIQUE conflict handling |
| 9b | Admin can delete users | 🔴 MISSING ENDPOINT | No `DELETE` handler registered |
| 10 | FTS search boundary | ⚠️ GLOBAL | Shared library by design — no user_id filter on FTS search |

---

## 3. Issues Found

### 🔴 Critical

#### C1: Missing DELETE /api/auth/users/:id endpoint
**File:** `backend/internal/auth/handlers.go`
**Problem:** The frontend `api.ts` (line 228) calls `DELETE /api/auth/users/${userId}` but the backend `Mount()` only registers:
```go
r.Post("/users", h.createUser)   // POST /api/auth/users
r.Get("/users", h.listUsers)     // GET /api/auth/users
```
There is no `DELETE` route. Admin users cannot delete family accounts.

**Impact:** The delete button in AdminUsersPage will fail with 404. Users are created but can never be removed.

**Fix:** Add a `deleteUser` handler and register `r.Delete("/users/{id}", h.deleteUser)` in Mount().

---

### 🟡 High

#### H1: `/auth/me` response shape mismatch
**File:** `backend/internal/auth/handlers.go` line 161-168 vs `frontend/src/lib/api.ts` line 222

**Backend returns (`me` handler):**
```json
{"id": 1, "username": "admin", "role": "admin"}
```
(UserInfo directly — not wrapped)

**Frontend expects:**
```typescript
j<{ user: User }>('/auth/me')  // expects { user: { id, username, display_name, is_admin } }
```

**Problem:** Two mismatches: (a) response not wrapped in `{ user: ... }`, (b) field names `role` vs `is_admin`.

**Impact:** The `useUser()` effect on mount calls `api.me()` which will fail to parse the response. The saved session token will be cleared even if valid. Users will see the login page on every app start despite having a valid session.

**Fix:** Either:
- Backend: wrap `me` response as `{ user: UserInfo }` and add `is_admin`/`display_name` fields, OR
- Frontend: adjust `User` type to match `UserInfo` (`role` string instead of `is_admin` boolean)

---

#### H2: `role` (string) vs `is_admin` (boolean) type mismatch
**Files:** `backend/internal/auth/sessions.go` (UserInfo), `frontend/src/lib/api.ts` (User), `frontend/src/contexts/UserContext.tsx` (isAdmin)

**Backend UserInfo JSON tag:**
```go
Role string `json:"role"`  // "admin" or "user"
```

**Frontend User type:**
```typescript
is_admin: boolean;
```

**Impact:** `UserContext.isAdmin` is computed as `user?.is_admin ?? false`. Since the backend never sends `is_admin`, every user including admins will have `isAdmin = false`. The "Users" nav link in the sidebar will never appear. The admin routes (/settings/users) will be inaccessible. The AdminUsersPage redirect for non-admins will fire and redirect to /settings.

**Fix:** Either transform `role` → `is_admin` on the backend, or update the frontend type to use `role` string and check `user?.role === "admin"`.

---

#### H3: `display_name` not included in auth responses
**Files:** `backend/internal/auth/sessions.go` (UserInfo), `backend/internal/auth/handlers.go` (login, me, createUser, listUsers)

**Backend UserInfo:**
```go
type UserInfo struct {
    UserID   int64  `json:"id"`
    Username string `json:"username"`
    Role     string `json:"role"`
}
```

**Frontend User type:**
```typescript
display_name: string;
```

The `display_name` is stored in the `users` table but never SELECTed or returned in any auth endpoint. The login handler does `SELECT id, password_hash, role FROM users` — no `display_name`. The `me` handler returns UserInfo from context (no display_name). `listUsers` selects `id, username, role, created_at` — no `display_name`.

**Impact:** The frontend's `AdminUsersPage` displays `{u.display_name || u.username}` — display names will always be empty, falling back to username. The sidebar user menu may also show empty display names.

**Fix:** Add `display_name` to UserInfo, SELECT it in login/listUsers, and store it in session UserInfo on login.

---

### 🟡 Medium

#### M1: SQL injection surface in analytics.go (string concatenation)
**File:** `backend/internal/analytics/analytics.go` lines 100, 105, 110, 117

These use `fmt.Sprintf` to embed `getUserID(r)` directly into SQL:
```go
`SELECT COUNT(*) FROM plays WHERE user_id IS NULL OR user_id = ` + fmt.Sprintf(`%d`, getUserID(r))
```

Every other package uses parameterized `?` placeholders. `getUserID(r)` returns int64 (safe against injection), but this breaks the pattern and would be flagged by any SQL lint tool.

**Fix:** Use `?` placeholders: `SELECT COUNT(*) FROM plays WHERE user_id IS NULL OR user_id = ?` with `getUserID(r)` as an argument.

---

#### M2: Duplicate `CREATE TABLE IF NOT EXISTS users` in schema DDL
**File:** `backend/internal/db/db.go` lines 28-35 and 201-209

Two `CREATE TABLE IF NOT EXISTS users` statements exist in the `schema` constant:
- Line 28: creates with `is_admin INTEGER NOT NULL DEFAULT 0` (old)
- Line 201: creates with `role TEXT NOT NULL DEFAULT 'user'` (new)

On fresh install, the first wins. Then `Migrate()` adds `role` and `display_name` via ALTER TABLE. The code functions correctly but the duplicate is confusing and the `is_admin` column is never used (code uses `role`).

**Fix:** Consolidate to a single `CREATE TABLE IF NOT EXISTS users` with both columns, or remove the second declaration (the `Migrate()` ALTER TABLE handles the column additions).

---

### 🟢 Minor

#### L1: Vestigial `is_admin` column
The `users` table has `is_admin INTEGER NOT NULL DEFAULT 0` (from the first CREATE), but all auth logic uses `role TEXT`. The `Migrate()` default admin insert populates both. No code path reads `is_admin`. Safe to leave but worth cleaning up.

#### L2: Chunk size warning in frontend build
Vite warns: `index-DBIgGJ3R.js` is 743.53 kB. Not a regression — pre-existing. Consider code splitting in future.

---

## 4. Architecture Review

### What's Well Done

| Component | Assessment |
|-----------|------------|
| **Auth middleware** | Clean three-tier auth: session → API key → unauthenticated pass-through. Uses `subtle.ConstantTimeCompare` for API key. Context-based user injection. |
| **Session management** | `sync.Map` with 24h expiry + 10min cleanup goroutine. Appropriate for single-process desktop app. `crypto/rand` token generation. |
| **DB migration** | Idempotent `columnExists()` pattern. Additive ALTER TABLE. Default admin creation with bcrypt hash. All NULL rows backfilled. 6 indexes created. Well-structured. |
| **Route structure** | Clean chi grouping: public (login), auth-protected (logout, me), admin-protected (users CRUD). Auth middleware skips `/api/auth/login`, `/api/health`, and OPTIONS. |
| **Frontend auth UI** | AuthGuard redirects unauthenticated users. LoginPage with error states. AdminUsersPage with self-deletion guard. Token persistence in localStorage. |
| **Backward compatibility** | `apiKey == ""` pass-through. `user_id IS NULL` OR filtering. All existing data visible to authenticated users. |
| **User isolation** | All logical constructs (playlists, plays, recommendations, downloads, analytics) filter by user_id. Library/search remain global (shared files). |
| **go build** | Both `./internal/...` and `./cmd/server` compile cleanly. |
| **npm build** | `tsc -b && vite build` passes with 0 errors on Windows. |

### Design Decisions That Need Confirmation

1. **Tracks are shared, not user-scoped.** The library shows all tracks regardless of user. If a family member imports music, everyone sees it. This is intentional (shared filesystem) but may surprise users who expect library isolation. Consider documenting this in help-content.ts.

2. **Podcast feeds are user-scoped but episodes playable by all.** `podcast_feeds` has `user_id` but `podcast_episodes` does not. The episodes are discoverable by anyone who has the parent feed ID. This is a reasonable design (podcast content is public) but worth noting.

3. **Spotify/Apple syncs are system-level.** Both syncers insert with `user_id = nil` — all streaming history is shared. Consider scoping to the active user.

---

## 5. Files Changed (47 files, +2577/-382)

### Backend (auth-specific)
| File | Changes |
|------|---------|
| `backend/internal/auth/passwords.go` | **NEW** — bcrypt hash + compare |
| `backend/internal/auth/sessions.go` | **NEW** — sync.Map session store, 24h expiry, cleanup |
| `backend/internal/auth/handlers.go` | **NEW** — login, logout, me, createUser, listUsers |
| `backend/internal/auth/middleware.go` | **UPDATED** — RequireAuth (session + API key), RequireAdmin, UserFromContext |
| `backend/internal/db/db.go` | **UPDATED** — users table, user_id on 6 tables, default admin backfill, indexes |
| `backend/internal/models/models.go` | **UPDATED** — UserID field, TrackCols 23→24, ScanTrack updated |
| `backend/cmd/server/main.go` | **UPDATED** — authHandler mount, session auth middleware, cleanup goroutine |
| `backend/internal/playlists/playlists.go` | **UPDATED** — user_id filtering in all CRUD |
| `backend/internal/downloader/downloader.go` | **UPDATED** — getUserID helper, user_id in jobs |
| `backend/internal/history/history.go` | **UPDATED** — user_id in INSERT + WHERE filter |
| `backend/internal/recommender/recommender.go` | **UPDATED** — user_id in cache + playlist generation |
| `backend/internal/analytics/analytics.go` | **UPDATED** — user_id in all aggregate queries |
| `backend/internal/scanner/scanner.go` | **UPDATED** — user_id=NULL in track INSERT |
| `backend/internal/spotify/sync.go` | **UPDATED** — user_id=NULL in sync INSERTs |
| `backend/internal/apple/sync.go` | **UPDATED** — user_id=NULL in sync INSERTs |
| `backend/internal/podcaster/podcaster.go` | **UPDATED** — user_id=NULL in feed INSERT |
| `backend/go.mod` | **UPDATED** — added golang.org/x/crypto v0.49.0 |

### Frontend (auth-specific)
| File | Changes |
|------|---------|
| `frontend/src/contexts/UserContext.tsx` | **NEW** — auth state, login/logout, session validation |
| `frontend/src/pages/LoginPage.tsx` | **NEW** — login form with error states, auto-redirect |
| `frontend/src/pages/AdminUsersPage.tsx` | **NEW** — create/delete users, admin-only guard |
| `frontend/src/lib/api.ts` | **UPDATED** — auth methods + types (login, logout, me, users, createUser, deleteUser) |
| `frontend/src/App.tsx` | **UPDATED** — UserProvider wrap, AuthGuard, login route, users route |

---

## 6. Required Fixes Before Merge

### Must-Fix (merge blocker)

1. **Add DELETE /api/auth/users/:id handler** (C1)
   - Add `deleteUser` handler to `handlers.go`
   - Register `r.Delete("/users/{id}", h.deleteUser)` in `Mount()`
   - Prevent self-deletion (check userID != authenticated user)
   - Cascading delete: playlists ON DELETE CASCADE; tracks/plays/download_jobs/recommendations/podcast_feeds ON DELETE SET NULL

2. **Fix `/auth/me` response shape** (H1)
   - Either wrap the response as `{"user": {...}}` or update the frontend `j<...>()` type
   - Preferred: backend wraps it — consistent with login response

3. **Align `role` / `is_admin` type** (H2)
   - Either: add `is_admin` bool to backend UserInfo, computed from `role == "admin"`
   - Or: update frontend User type to use `role: string` and check `user.role === "admin"`
   - Preferred: backend approach — cleaner separation

4. **Add `display_name` to auth responses** (H3)
   - Add `DisplayName` field to `UserInfo`
   - SELECT display_name in login query
   - Include in session UserInfo
   - Include in listUsers response

### Should-Fix (not a merge blocker but important)

5. **Parameterize analytics.go SQL** (M1) — use `?` instead of `fmt.Sprintf`

6. **Clean up duplicate `CREATE TABLE` for users** (M2) — remove or consolidate

### Could-Fix (nice to have)

7. **Document library-sharing behavior** in help-content.ts

---

## 7. End-to-End Flow Walkthrough

### First Run
1. App starts → `godotenv.Load()` → `auth.SetAPIKey()` → `config.Load()` → `db.Migrate()`
2. Migrate() runs schema + column migrations → `users` table created with `is_admin` column
3. Migrate() adds `display_name` and `role` via ALTER TABLE
4. No users exist yet → Migrate() creates default admin (bcrypt hash) ⚠️ 
5. Wait — the Migrate() creates a default admin with hardcoded bcrypt hash. But the login handler ALSO has first-run logic (lines 86-120). **Both Migrate() and login() create an admin on first run.** This is a double-creation scenario:
   - Migrate() creates admin/admin with hardcoded hash
   - If user uses admin/admin → bcrypt compare succeeds → normal login path (lines 122-151)
   - If Migrate() already created the user, login() `SELECT COUNT(*) FROM users` returns 1 → first-run path is skipped
   - ✅ This works correctly — Migrate() creates the default admin, login() first-run logic is dead code on the first real run BUT would fire if Migrate() somehow failed to create the user (safety net)
6. User visits app → AuthGuard checks session → localStorage empty → redirects to /login
7. User enters admin/admin → login handler queries users table → bcrypt compare → session created → token returned
8. Frontend stores token in localStorage, sets in memory → UserContext updates → AuthGuard passes → app renders

### Adding Family Member
1. Admin navigates to /settings/users → AdminUsersPage loads
2. ⚠️ `isAdmin` is `false` because of type mismatch → user gets redirected to /settings immediately → **DEAD END**
3. **If type mismatch is fixed:** admin sees the Users page → clicks "Add Account"
4. Fills form → POST /api/auth/users → createUser validates role → INSERT → returns user
5. Card appears in list

### Logout + Login as Family Member
1. User clicks logout in sidebar → POST /api/auth/logout → session deleted → token cleared from localStorage
2. AuthGuard redirects to /login
3. User enters family member credentials → login handler queries users table → bcrypt compare → session created
4. User is now authenticated as family member

### Library Isolation
1. Family member browses library → `GET /api/library/tracks` → returns all tracks (shared)
2. Family member views playlists → `GET /api/playlists` → only family member's playlists returned
3. Family member views history → only their plays returned
4. Family member views analytics → only their stats returned
5. Family member creates playlist → INSERT with `user_id` set to their ID
6. Admin logs in → cannot see family member's playlist
7. Family member downloads music → download_job has their `user_id` → only visible to them

---

## 8. Regression Risk Assessment

| Area | Risk | Notes |
|------|------|-------|
| Library browsing | **Low** | No user_id filtering added — unchanged behavior |
| Music playback | **Low** | History now records user_id, but playback itself unchanged |
| Downloads | **Medium** | user_id added to jobs — tested on downloads page view |
| Playlists | **Medium** | user_id filtering in all CRUD — query changes are additive |
| Podcasts | **Medium** | user_id added to feeds — subscribe/list endpoints changed |
| Recommendations | **Low** | user_id added to cache — additive change |
| Analytics | **Low** | user_id added to all aggregate queries — additive WHERE clause |
| Spotify sync | **Low** | user_id=NULL in INSERT — unchanged behavior |
| Apple Music sync | **Low** | user_id=NULL in INSERT — unchanged behavior |
| FTS search | **None** | No changes — global search preserved |

---

## 9. Overall Assessment

The implementation is **structurally sound** — the architecture is clean, the DB migration is well-structured, the auth middleware is properly layered, and the user isolation is correctly applied to all logical constructs. The `go build` and `npm run build` both pass cleanly.

The bugs are **bounded and fixable** — they're type mismatches between frontend and backend, not architectural flaws. None require re-architecting any subsystem. A single pass of targeted fixes (add DELETE handler, align response types, add display_name) will resolve all critical and high issues.

**Estimated fix time:** 30-60 minutes for all critical + high issues.

**Recommendation:** Fix C1 + H1 + H2 + H3, then merge. M1 and M2 can be fixed in a follow-up PR.
