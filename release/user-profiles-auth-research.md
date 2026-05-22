# User Profiles Auth Research — Lexicon Local Desktop App

**Date:** 2026-05-21  
**Author:** Atlas (researcher)  
**Task:** R1 — evaluate auth approaches for multi-user household profiles

---

## 1. Context & Constraints

**What Lexicon is:**
- Local-first Windows desktop app (Go backend + React SPA + SQLite via `modernc.org/sqlite`)
- Single binary (`lexicon.exe`) with embedded frontend, distributed as InnoSetup installer
- MUST work fully offline — no cloud dependency, no external auth services
- Backend serves on `localhost:8787` (or LAN IP for mobile access via QR code)
- Current auth: single `LEXICON_API_KEY` env var → Bearer token checked by `auth.RequireAPIKey` middleware (no-op when unset)

**What we're building:**
- Multi-user household profiles (admin creates family accounts)
- Per-user state: playlists, playback history, podcast progress, recommendations, settings
- Optional LAN access from phones/tablets (same household — not public internet)

**Threat model:**
- Physical access = filesystem access = direct DB access. Someone sitting at the PC can read `lexicon.db` directly.
- LAN access from phones: same household WiFi, not the public internet.
- Primary concern is household privacy — keeping family members' playlists/history separate, not defending against sophisticated attackers.
- XSS via the React SPA is the most realistic attack vector.
- No external network exposure (no port forwarding, no cloud sync).

---

## 2. Password Hashing

### Recommendation: bcrypt via `golang.org/x/crypto/bcrypt`

```go
import "golang.org/x/crypto/bcrypt"

// Hash password (cost 12, ~250ms on modern hardware)
hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)

// Verify password
err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
```

**Why bcrypt:**

| Factor | bcrypt | Argon2id | scrypt | PBKDF2 |
|--------|--------|----------|--------|--------|
| Go stdlib-adjacent | ✅ `x/crypto` | ⚠️ third-party | ⚠️ third-party | ✅ `x/crypto` |
| Memory-hardness | Moderate | ✅ High | ✅ High | ❌ None |
| Maturity | ✅ 25+ years | ✅ ~10 years | ✅ 15+ years | ✅ 20+ years |
| Simplicity | ✅ 2 functions | ⚠️ many params | ⚠️ many params | ✅ simple |
| Threat-model fit | ✅ Perfect | Overkill | Overkill | Adequate |

**Why NOT Argon2id:**
- Argon2id is cryptographically superior (memory-hard → resistant to GPU/ASIC attacks)
- But: for a local desktop app where the attacker already has filesystem access, the hash algorithm doesn't matter — they can just read the SQLite DB directly
- bcrypt keeps the codebase simple with minimal dependencies (`x/crypto` is already transitive)
- If Lexicon ever adds cloud sync where the password hash could be exposed, Argon2id can be added as an upgrade migration

**Cost factor: 12**
- Takes ~250ms on 2024+ consumer hardware
- Not perceptible in a login flow (one-time, not per-request)
- OWASP recommends cost >= 10 for bcrypt
- Going higher (13-14) adds latency with diminishing returns for this threat model

**Implementation notes:**
- bcrypt automatically handles salt generation and storage (salt is embedded in the hash string)
- Hash output is 60 characters, always starts with `$2a$` (or `$2b$`/`$2y$`)
- Store as `TEXT` in SQLite
- `bcrypt.CompareHashAndPassword` is constant-time for the comparison

---

## 3. Session Management

### Recommendation: Cookie-based sessions with random hex tokens stored in SQLite

**Pattern:**

```go
type Session struct {
    Token     string    // 32-byte random hex (64 chars)
    UserID    int64
    Role      string    // "admin" | "user"
    ExpiresAt time.Time
    CreatedAt time.Time
}
```

**How it works:**
1. User logs in → backend verifies password with bcrypt
2. Backend generates a 32-byte cryptographically random token (`crypto/rand`), stores it in `sessions` table with user_id, role, and expiry
3. Backend sets the token as an HttpOnly, Secure, SameSite=Strict cookie
4. Browser sends the cookie automatically on every request
5. Middleware reads the cookie, looks up the session in SQLite, attaches user context to the request

### Comparison: Cookie Session vs JWT vs Bearer Token

| Factor | Cookie Session (SQLite) | JWT | Bearer Token (Header) |
|--------|------------------------|-----|----------------------|
| **DB lookup per request** | Yes (1 SQLite query) | No | Yes (if stored in DB) |
| **Token revocation** | ✅ Delete row | ❌ Needs blacklist | ✅ Delete row |
| **XSS resistance** | ✅ HttpOnly cookie | ❌ JS-accessible if stored in localStorage | ❌ JS-accessible |
| **CSRF risk** | ⚠️ Mitigated with SameSite | ✅ Immune (not auto-sent) | ✅ Immune |
| **Browser integration** | ✅ Automatic | Manual header | Manual header |
| **Stateless?** | ❌ | ✅ | ❌ |
| **Offline-compatible** | ✅ | ✅ | ✅ |
| **Complexity** | Low | Medium | Low |
| **Frontend effort** | None (browser handles) | Must attach header to every fetch | Must attach header to every fetch |

### Why cookie sessions win for Lexicon:

1. **HttpOnly cookie = XSS-resistant.** Even if the React app has an XSS vulnerability, JavaScript cannot read the session token. This is the single biggest security win. JWT stored in localStorage or memory is vulnerable to XSS exfiltration.

2. **Browser handles it.** No frontend code changes needed — no `Authorization: Bearer <token>` header on every `fetch()` call. The existing `api.ts` client continues to work as-is (just add `credentials: 'include'` to fetch options).

3. **Simple revocation.** Admin resets a user's password → delete all their sessions. User logs out → delete their session. No JWT blacklist to maintain.

4. **SQLite lookup is fast.** For a local desktop app with 2-6 users and < 10 concurrent requests, the extra DB query is negligible (under 1ms on SQLite WAL mode).

5. **SameSite=Strict prevents CSRF.** Modern browsers enforce SameSite=Strict by default, so the cookie won't be sent on cross-site requests. For the LAN phone use case, SameSite=Lax or None might be needed with explicit CORS.

### Session lifecycle:

| Event | Action |
|-------|--------|
| Login | Create session, set cookie, 7-day expiry |
| Every request | Middleware looks up session, refreshes expiry if < 24h remain |
| Logout | Delete session from DB, clear cookie |
| Password change | Delete ALL sessions for that user |
| Admin deletes user | Delete all sessions for that user |
| App restart | Sessions persist in DB (survives restart) |
| App reinstall | Sessions lost with DB file |

### Cookie configuration for Lexicon's use case:

```go
http.SetCookie(w, &http.Cookie{
    Name:     "lexicon_session",
    Value:    token,
    Path:     "/",
    HttpOnly: true,                      // JavaScript cannot read
    Secure:   false,                     // localhost is HTTP, not HTTPS
    SameSite: http.SameSiteLaxMode,      // allows top-level navigation from LAN
    MaxAge:   7 * 24 * 60 * 60,         // 7 days
})
```

**Note on Secure flag:** Since Lexicon runs on `http://localhost` and `http://192.168.x.x` (LAN), the `Secure` flag must be `false`. This is acceptable because the threat is household privacy, not network interception. If HTTPS is added later (via self-signed cert or ngrok), enable `Secure`.

---

## 4. Initial Setup Flow

### The "first run" detection:

```
App starts → check if any user exists in users table:
  ├─ users table empty → show setup wizard (create admin account)
  └─ users exist → show login screen
```

### Setup wizard (first run only):

```
┌─────────────────────────────────────────┐
│         Welcome to Lexicon              │
│                                         │
│  Create your admin account to get       │
│  started. This account can manage       │
│  family profiles and settings.          │
│                                         │
│  Username: [_______________]            │
│  Password: [_______________]            │
│  Confirm:  [_______________]            │
│                                         │
│         [ Create Account ]              │
└─────────────────────────────────────────┘
```

**Flow:**
1. Backend exposes `GET /api/auth/status` → returns `{setup_required: true/false}`
2. Frontend checks on app load
3. If `setup_required: true`, render Setup wizard instead of Login
4. Admin creates account → `POST /api/auth/setup {username, password}`
5. Backend creates admin user, creates session, sets cookie, returns success
6. Frontend redirects to main app

**Why a dedicated setup endpoint:** The setup endpoint (`/api/auth/setup`) only works when no users exist. This prevents anyone from creating accounts without authentication once the system is initialized.

### Admin creates family accounts:

```
┌─────────────────────────────────────────┐
│  Settings → Family Accounts             │
│                                         │
│  ┌──────────────────────────────────┐   │
│  │ Alice (admin)         [edit]     │   │
│  │ Bob                   [edit] [×] │   │
│  │ Charlie               [edit] [×] │   │
│  └──────────────────────────────────┘   │
│                                         │
│  [+ Add Family Member]                  │
└─────────────────────────────────────────┘
```

- Admin can create/edit/delete family accounts
- Admin can reset passwords (no email — this is a household app)
- Admin cannot see existing passwords (bcrypt hash is one-way)
- Admin role is permanent (first account = admin, cannot be demoted)

---

## 5. Password Policies

### Recommended policy (household-appropriate):

| Rule | Value | Rationale |
|------|-------|-----------|
| Minimum length | 6 characters | Short enough for convenience, long enough to prevent empty/trivial passwords |
| Maximum length | 128 characters | bcrypt's max input is 72 bytes, but Go's bcrypt handles truncation |
| Complexity | None required | No uppercase/number/symbol mandates — this is a household, not a bank |
| Password strength meter | Optional UI indicator | Visual feedback (weak/medium/strong) — not enforced |
| Account lockout | None | Local app, no brute-force concern (attacker has DB access anyway) |

### Why no complexity requirements:

Family members will use this app casually. Forcing "Must contain uppercase, number, and special character" on a 10-year-old or a grandparent using a shared music app is user-hostile. The password only protects against casual snooping by other household members — not external attackers.

### Password reset flow:

Since there's no email system:
1. User tells admin "I forgot my password"
2. Admin goes to Settings → Family Accounts → clicks "Reset Password" on that user
3. Admin enters a new temporary password
4. User logs in with the temporary password → prompted to change it

This is the standard pattern for offline/local-first systems where the admin is physically present.

---

## 6. Database Schema

```sql
-- Users table
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,          -- bcrypt hash, 60 chars
    role TEXT NOT NULL DEFAULT 'user',    -- 'admin' | 'user'
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

-- Sessions table
CREATE TABLE IF NOT EXISTS sessions (
    token TEXT PRIMARY KEY,               -- 64-char hex string
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    expires_at INTEGER NOT NULL           -- Unix timestamp
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
```

### Migration strategy:

Additive migration (existing pattern in `db.go`):
1. `columnExists("users", ...)` checks for each new column
2. `ALTER TABLE ... ADD COLUMN` if missing
3. No data loss — existing tracks, playlists, etc. remain untouched
4. First run after migration: users table is empty → setup wizard appears

### Per-user data ownership:

For **Phase 1 (this task):** Focus on authentication and session management. Existing data (tracks, playlists, plays) remains global/shared. User-scoping of data is a separate task (likely T2 or T3).

**Future scoping pattern (not in scope for this task):**
```sql
ALTER TABLE playlists ADD COLUMN user_id INTEGER REFERENCES users(id);
ALTER TABLE plays ADD COLUMN user_id INTEGER REFERENCES users(id);
-- etc.
```

---

## 7. API Design

### New endpoints:

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/auth/status` | None | Returns `{setup_required, authenticated, user}` |
| POST | `/api/auth/setup` | None (only when empty) | Create first admin user |
| POST | `/api/auth/login` | None | Login, returns session cookie |
| POST | `/api/auth/logout` | Session | Clear session |
| GET | `/api/auth/me` | Session | Current user info |
| POST | `/api/users` | Admin session | Create family account |
| GET | `/api/users` | Admin session | List all users |
| PUT | `/api/users/{id}` | Admin session | Edit user (display name, role) |
| DELETE | `/api/users/{id}` | Admin session | Delete user |
| POST | `/api/users/{id}/reset-password` | Admin session | Reset password |
| POST | `/api/auth/change-password` | Session | Change own password |

### Middleware chain:

```
All requests
  │
  ├─ GET /api/auth/status         → No auth required
  ├─ POST /api/auth/setup         → No auth required (one-time)
  ├─ POST /api/auth/login         → No auth required
  │
  └─ Everything else
       │
       ├─ Read auth cookie
       ├─ Look up session in DB
       ├─ Session valid?
       │   ├─ Yes → attach user context, continue
       │   └─ No  → 401
       │
       └─ Admin-only routes (/api/users/*)
            └─ Check user.Role == "admin"
```

### Existing middleware migration:

The current `auth.RequireAPIKey` middleware (which reads `LEXICON_API_KEY` env var) should be replaced with the new session-based middleware. Two options:

**Option A: Parallel (recommended for transition)**
- Keep `LEXICON_API_KEY` as a fallback for programmatic/headless access (scripts, cron jobs)
- New session middleware runs alongside it
- Headless tools can use either: `Authorization: Bearer <api_key>` OR session cookie

**Option B: Full replacement**
- Remove `LEXICON_API_KEY` entirely
- Everything uses session cookies
- Downside: scripts and headless tools (like `upgrade-library.sh`) need to log in first

**Recommendation: Option A (parallel).** Keep the API key for headless access. The API key serves a different purpose (machine-to-machine) than user sessions (human-to-machine). Both can coexist.

---

## 8. Frontend Architecture

### New components:

```
frontend/src/
├── contexts/
│   └── AuthContext.tsx          # NEW — wraps app, manages auth state
├── components/
│   ├── LoginForm.tsx            # NEW — username + password form
│   ├── SetupWizard.tsx          # NEW — first-run admin account creation
│   ├── FamilyAccountsPanel.tsx  # NEW — admin user management (in Settings)
│   └── ProtectedRoute.tsx       # NEW — redirects to login if unauthenticated
├── pages/
│   ├── LoginPage.tsx            # NEW — login page
│   └── SetupPage.tsx            # NEW — first-run setup page
└── lib/
    └── api.ts                   # MODIFIED — add auth methods, credentials: 'include'
```

### AuthContext:

```typescript
interface AuthState {
  status: 'loading' | 'setup_required' | 'unauthenticated' | 'authenticated';
  user?: { id: number; username: string; display_name: string; role: string };
}
```

**Flow:**
1. App mounts → `AuthProvider` calls `GET /api/auth/status`
2. Status returns one of:
   - `setup_required` → render `<SetupPage />`
   - `unauthenticated` → render `<LoginPage />`
   - `authenticated` → render normal app with user context
3. On login/setup success → cookie set by backend → redirect to main app

### API client changes (`api.ts`):

```typescript
// Add to fetch options globally:
const fetchOptions: RequestInit = {
  credentials: 'include',  // Send cookies
  headers: { ... },
};

// New API methods:
async status(): Promise<AuthStatus>
async login(username: string, password: string): Promise<User>
async logout(): Promise<void>
async setup(username: string, password: string): Promise<User>
async changePassword(currentPassword: string, newPassword: string): Promise<void>
async listUsers(): Promise<User[]>
async createUser(username: string, password: string, displayName: string): Promise<User>
async updateUser(id: number, data: Partial<User>): Promise<User>
async deleteUser(id: number): Promise<void>
async resetUserPassword(id: number, newPassword: string): Promise<void>
```

---

## 9. Security Considerations

### What bcrypt protects against:

| Scenario | Protected? |
|----------|------------|
| DB file stolen/copied → passwords readable? | ✅ Hashes are one-way |
| Family member tries other member's password | ✅ bcrypt comparison |
| Session token leaked via XSS | ✅ HttpOnly cookie |
| Session token stolen from DB | ✅ Random, session-scoped |
| Brute force login attempts | ⚠️ No lockout (acceptable for local) |

### What bcrypt CANNOT protect against:

| Scenario | Why |
|----------|-----|
| Attacker has filesystem access | Can read DB directly, bypass auth entirely |
| Attacker modifies DB | SQLite has no access control — anyone with FS write can add users |
| Attacker reads audio files | Files are on disk, unencrypted |
| Malware on the PC | Keylogger can capture password at OS level |

### The honest threat-model assessment:

**The password protects against:**
1. Household members casually accessing each other's profiles through the app
2. Someone on the same LAN opening the web UI without permission
3. Making it slightly harder for someone who steals the DB file to impersonate users (they'd need to crack bcrypt or add their own hash)

**The password does NOT protect against:**
1. Anyone with physical access to the PC who can open the DB file directly
2. Anyone who can run SQLite commands on the DB file
3. Malware, keyloggers, or compromised OS

**This is the correct tradeoff for a household music app.** Adding more security (encrypted DB, TPM, biometrics) would add complexity disproportionate to the threat.

### Recommendations for hardening (future):

1. **TLS for LAN access** (if phones connect over WiFi): Generate a self-signed certificate at install time, serve HTTPS. Then `Secure: true` on cookies.
2. **Session cleanup goroutine**: Delete expired sessions every hour (simple `DELETE FROM sessions WHERE expires_at < strftime('%s','now')`).
3. **Audit log**: Record login attempts (success/failure) in a `auth_log` table. Useful for "who used the app when?"
4. **Per-user encryption** (very future): If Lexicon ever gets cloud sync, encrypt each user's data with a key derived from their password. This way the server can't read it.

---

## 10. Comparison of Auth Patterns

### What Jellyfin does (media server, similar use case):
- Supports local users with passwords (no external auth required)
- Quick Connect for devices (show code on TV, enter on phone)
- Admin dashboard for user management
- No email/password reset — admin handles it

### What Plex does:
- Plex account (cloud-based) with local server linking
- NOT applicable — requires internet, Plex cloud dependency

### What Home Assistant does:
- Local users with Owner/Admin/User roles
- Supports "Trusted Networks" (no password from certain IPs)
- Session tokens in localStorage (not HttpOnly cookies)

### What we should do (Lexicon-specific):
- Jellyfin-like: local users, admin management, no cloud dependency
- Cookie-based sessions (better than Home Assistant's localStorage approach)
- Optional API key fallback for headless/script access
- Simple setup wizard for first run

---

## 11. Implementation Plan

### Phase 1: Auth foundation (this task's scope)
1. Add `users` and `sessions` tables to `db.go`
2. Create `backend/internal/auth/` package:
   - `users.go` — user CRUD (create, list, update, delete, change password)
   - `sessions.go` — session create, validate, delete, cleanup
   - `middleware.go` — replaced: session-based auth middleware
   - `handlers.go` — HTTP handlers for auth endpoints
3. Update `main.go`:
   - Mount new auth routes
   - Replace `RequireAPIKey` with session middleware
   - Keep API key middleware as parallel fallback
4. Frontend:
   - `AuthContext.tsx`, `LoginPage.tsx`, `SetupPage.tsx`
   - Update `api.ts` with auth methods
   - Add `ProtectedRoute` wrapper

### Phase 2: Per-user data (future task)
1. Add `user_id` columns to: `playlists`, `plays`, `recommendations`, `podcast_episodes`
2. Update all queries to filter by `user_id`
3. Migration: assign all existing data to admin user

### Phase 3: Profile features (future task)
1. User-specific settings (theme, default media root, etc.)
2. Profile switching (fast user switching without logout)
3. User avatars

---

## 12. Summary & Recommendation

| Component | Recommendation | Rationale |
|-----------|---------------|-----------|
| Password hashing | **bcrypt, cost 12** | Simple, Go-native (`x/crypto`), appropriate for threat model |
| Session transport | **HttpOnly cookie** | XSS-resistant, browser-handled, no frontend changes needed |
| Session storage | **SQLite `sessions` table** | Fast local lookup, survives restart, easy revocation |
| Session token | **32-byte crypto/rand hex** | 64-char string, 256 bits of entropy, URL-safe |
| Initial setup | **First-run wizard** | Detects empty users table, prompts for admin account |
| User management | **Admin-only CRUD** | Admin creates/deletes family accounts, resets passwords |
| Password policy | **Min 6 chars, no complexity** | Household-appropriate, not a corporate policy |
| API key fallback | **Keep for headless access** | Scripts and cron jobs can use Bearer token |
| Frontend auth state | **AuthContext provider** | Wraps app, redirects to login/setup as needed |

### What NOT to do:

- ❌ **JWT** — adds complexity (signing keys, revocation blacklist) with no benefit over DB sessions
- ❌ **Argon2id** — memory-hardness is pointless when attacker has filesystem access to the DB
- ❌ **OAuth/OIDC** — requires internet, defeats offline requirement
- ❌ **localStorage for tokens** — vulnerable to XSS exfiltration
- ❌ **Complex password rules** — user-hostile for a household music app
- ❌ **Account lockout** — admin can physically talk to the person; brute-force isn't the threat
- ❌ **Email-based password reset** — requires SMTP config, internet; admin handles it instead
- ❌ **SSO / LDAP / Active Directory** — enterprise features for a household app

---

## 13. References

- Go bcrypt: `golang.org/x/crypto/bcrypt` — https://pkg.go.dev/golang.org/x/crypto/bcrypt
- OWASP Session Management Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html
- OWASP Authentication Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html
- Jellyfin local user model (similar household media server auth): https://jellyfin.org/docs/general/server/users/
- Home Assistant authentication: https://www.home-assistant.io/docs/authentication/
- Go `crypto/rand` for secure token generation: https://pkg.go.dev/crypto/rand
- SameSite cookie attribute: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Set-Cookie#samesitesamesite-value
