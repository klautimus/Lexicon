# Lexicon Multi-User Profiles — Frontend UX Research

**Date:** 2026-05-21
**Author:** Atlas (R3 research task)
**Scope:** UX patterns, component architecture, and mockup descriptions for family multi-user support in Lexicon desktop app.

---

## Table of Contents

1. [Login Screen](#1-login-screen)
2. [Profile Switching](#2-profile-switching)
3. [Library Isolation](#3-library-isolation)
4. [Admin User Management](#4-admin-user-management)
5. [Visual Design](#5-visual-design)
6. [State Management](#6-state-management)
7. [Active User Indicator](#7-active-user-indicator)
8. [Implementation Notes](#8-implementation-notes)

---

## 1. Login Screen

### When & Where

**Pattern: Pre-app gate.** On startup, Lexicon shows the login screen before the main app layout. If no valid session token exists (checked via a lightweight `GET /api/auth/session` call), the user sees the login screen. If a valid token exists and "remember me" was checked, skip straight to the app.

The login screen lives OUTSIDE the normal SPA routing — it's a conditional render at the top of the component tree, NOT a `/login` route. This is important because React Router's `<Routes>` lives inside `DesktopLayout`/`MobileLayout`, and we don't want the sidebar/nav visible before auth. Reference: the current `App.tsx` provider hierarchy (line 168-182):

```tsx
// Current (single-user):
<ErrorBoundary>
  <ToastProvider>
    <PlayerProvider>
      <DownloadProvider>
        <HelpProvider>
          <AppContent />       {/* DesktopLayout or MobileLayout directly */}
        </HelpProvider>
      </DownloadProvider>
    </PlayerProvider>
  </ToastProvider>
</ErrorBoundary>

// Proposed (multi-user):
<ErrorBoundary>
  <ToastProvider>
    <UserProvider>             {/* NEW: wraps everything */}
      {!isAuthenticated ? (
        <LoginScreen />        {/* Full-screen, no sidebar, no player */}
      ) : (
        <PlayerProvider>
          <DownloadProvider>
            <HelpProvider>
              <AppContent />   {/* Same as before */}
            </HelpProvider>
          </DownloadProvider>
        </PlayerProvider>
      )}
    </UserProvider>
  </ToastProvider>
</ErrorBoundary>
```

**Why not a route?** Because when unauthenticated, we don't want ANY existing components to mount — PlayerContext fetches tracks, DownloadContext starts polling, Spotify SDK initializes, etc. The login screen is a clean, isolated view.

### Layout (Desktop)

A centered card on a dark background, no sidebar. The Lexicon logo + name at top.

```
┌─────────────────────────────────────────────────┐
│                                                 │
│                                                 │
│           ┌─────────────────────┐               │
│           │   [Lexicon Logo]    │               │
│           │   L E X I C O N     │               │
│           │                     │               │
│           │  ┌───────────────┐  │               │
│           │  │ Username      │  │               │
│           │  └───────────────┘  │               │
│           │  ┌───────────────┐  │               │
│           │  │ Password  👁   │  │               │
│           │  └───────────────┘  │               │
│           │                     │               │
│           │  ☐ Remember me      │               │
│           │                     │               │
│           │  [═══ LOG IN ═══]   │               │
│           │                     │               │
│           │  Error: Invalid...  │ (conditional) │
│           └─────────────────────┘               │
│                                                 │
│                                                 │
└─────────────────────────────────────────────────┘
```

**Specs:**
- Card: `bg-panel border border-panel2 rounded-xl p-10 w-96`
- Logo: 64x64 `icon.svg`, centered
- "Lexicon" heading: `text-2xl font-semibold tracking-wide text-center mb-6`
- Username input: `bg-inputbg border border-panel2 rounded-md px-3 py-2 text-text w-full` + `User` icon (Lucide) on left
- Password input: Same style + `Lock` icon + `Eye`/`EyeOff` toggle button on right to show/hide password
- "Remember me" checkbox: `text-sm text-muted`, styled checkbox matching dark theme
- Login button: Full-width, accent color `bg-accent hover:bg-accent/80 text-white font-medium py-2.5 rounded-md`
- Error state: Red banner `bg-red-400/10 border border-red-400/30 text-red-400 text-sm rounded p-3` above or below the button

### Error States

| State | Visual | Message |
|-------|--------|---------|
| Wrong password | Red banner, shake animation on card | "Incorrect password. Please try again." |
| No such user | Red banner | "No account found with that username." |
| Server unreachable | Red banner | "Unable to reach Lexicon server. Check that it's running." |
| Rate limited (N attempts) | Red banner + cooldown timer | "Too many attempts. Please wait 30 seconds." |

The shake animation can be CSS: `@keyframes shake { 0%,100% { transform: translateX(0) } 25% { transform: translateX(-4px) } 75% { transform: translateX(4px) } }` applied via a `login-error` class for 400ms, triggered when an error state changes.

### "Remember Me" Mechanics

- When checked: the backend returns a long-lived session token (30 days). Store in `localStorage`. On next app launch, check for token → validate with `GET /api/auth/session` → skip login if valid.
- When unchecked: the backend returns a short-lived session token (24h). Store only in React state (memory). On app close, token is lost.
- **Do NOT store the raw password.** The token is the only credential stored client-side.
- Token refresh: when a request returns 401, clear the token and show the login screen (UserContext handles this centrally).

### Mobile Layout

Same card, but full-width with padding:
- Card: `w-full max-w-sm mx-4` instead of fixed `w-96`
- Logo: 48x48
- Inputs: full width
- The card is vertically centered using `min-h-screen flex items-center justify-center`

### Keyboard Support

- `Enter` on username field → focus password
- `Enter` on password field → submit login
- `Tab` cycles through: username → password → show/hide toggle → remember me → login button

---

## 2. Profile Switching

### Two Access Points

**A) Settings page** — "Profiles" section (new, below Apple Music). Lists all family members. Clicking one prompts for password, then switches.

**B) User menu in sidebar header** — A small dropdown triggered by clicking the active user indicator (see section 7). Shows:
- Current user (highlighted, with checkmark)
- Other users (clickable)
- "Switch User..." option at bottom (opens password prompt)

```
┌─────────────────────┐
│ 👤 Kevin  ✓         │ ← current, greyed out, checkmark
│ ─────────────────── │
│ 👤 Sarah             │ ← clickable
│ 👤 Emma              │ ← clickable
│ ─────────────────── │
│ 🔒 Switch User...    │ ← opens re-auth modal
└─────────────────────┘
```

### Should Profile Switching Require Re-authentication?

**YES.** This is a security question, not a UX question. Plex and Jellyfin both require PIN/password to switch profiles. Rationale:

- Prevents a child from accidentally (or deliberately) accessing a parent's library
- Prevents playlist/data modification under the wrong user
- Simple password is enough (not full username+password — username is already known from the profile being switched to)

**Implementation:** When switching, show a small modal with:
```
┌───────────────────────────────────┐
│   Switch to Sarah's profile       │
│                                   │
│   Enter password to continue:     │
│   ┌─────────────────────────────┐ │
│   │ Password              👁    │ │
│   └─────────────────────────────┘ │
│                                   │
│   [Cancel]       [Switch]         │
└───────────────────────────────────┘
```

After successful re-auth, UserContext updates the current user and all data fetches refresh.

### What Happens on Switch

1. Pause any playing audio
2. Clear the queue (`PlayerContext.reset()`)
3. Set `loading = true` on all pages
4. Swap the session token → `UserContext.setCurrentUser(newUser, newToken)`
5. All components re-fetch data under the new user's context
6. Toast: "Switched to Sarah's library"

This is a hard context switch — no attempt to preserve state across users.

---

## 3. Library Isolation

### How the Frontend Knows Which User Is Logged In

**Session token in `Authorization` header + `UserContext` provider.**

```
Request flow:
  UserContext holds { id, username, isAdmin, token }
       │
       ▼
  api.ts j() helper attaches token:
    headers: { "Authorization": `Bearer ${token}` }
       │
       ▼
  Go backend RequireAuth middleware:
    - Parses Bearer token → validates HMAC/JWT → extracts user_id
    - Sets ctx.Value("user_id", userID) on the request context
       │
       ▼
  All handlers read user_id from context:
    db.Query("SELECT * FROM tracks WHERE user_id = ?", userID)
```

**Token storage:**
- "Remember me" enabled → `localStorage` (persists across sessions)
- "Remember me" disabled → React state only (lost on refresh/close)
- Never in sessionStorage (shared across tabs can cause conflicts)

### What Gets Isolated

| Resource | Current | With User Profiles |
|----------|---------|-------------------|
| Tracks (`/api/library/tracks`) | All tracks | Filtered by `user_id` |
| Playlists (`/api/playlists`) | All playlists | Filtered by `user_id` |
| History (`/api/history/recent`) | All plays | Filtered by `user_id` |
| Analytics (`/api/analytics/*`) | Aggregate all | Filtered by `user_id` |
| Recommendations (`/api/recommendations/*`) | Single profile | Per-user profile cache |
| Downloads (`/api/download/*`) | Shared queue | Per-user jobs |
| Spotify connection (`/api/spotify/*`) | Single account | Per-user tokens |
| Apple Music (`/api/apple/*`) | Single account | Per-user tokens |
| Podcasts (`/api/podcasts/*`) | Shared feeds | Shared feeds (system-level) or per-user? **See decision below** |

### Decision: Podcast Feeds — Shared or Per-User?

**Recommendation: Shared at the system level, but per-user playback position and download status.** Rationale: Podcast RSS feeds are infrastructure (download once, everyone listens). But each user should have their own playback position, "listened" status, and auto-download preference.

Alternative worth considering: Per-user podcast subscriptions. This adds complexity but some families may not want their feeds mixed. Implementation cost is moderate (add `user_id` to `podcast_feeds`). **Recommend starting with shared feeds + per-user state, and making per-user feeds a v2 enhancement if requested.**

### Cold Start for New Users

When a new user logs in for the first time:
- Show an empty-state Home page with a welcome message
- Optionally prompt: "Would you like to import tracks from another user's library? (shared media roots mean files are accessible)"
- Or just show empty library → they add tracks via Search & Download or connect Spotify/Apple Music

### Shared Media Roots

Since Lexicon points to shared filesystem directories (`MEDIA_ROOTS`), the actual audio files are accessible to all users. The `tracks` table has a `path` column — two users CAN see the same file. But they should have separate `tracks` records (separate `id`, separate `user_id`) for per-user metadata (play counts, playlist membership, etc.).

**This means:** If User A downloads a track, User B sees it as an available file but gets their own tracking. The scanner can be shared (it indexes files once globally), but track ownership is per-user.

**Alternative (simpler):** Tracks are global (single `tracks` table, no `user_id`), but play history, playlists, and analytics are per-user. This is simpler to implement and matches how Plex handles media libraries. **Recommend this approach for v1** — it avoids data duplication and lets everyone share a music library while having personal playlists and listening stats.

```
Simpler model (recommended for v1):
  tracks        — global, no user_id (shared media)
  plays         — user_id FK (per-user history)
  playlists     — user_id FK (per-user playlists)
  recommendations — user_id FK (per-user cache)
  spotify_tokens — user_id FK (per-user connections)
  apple_music_*  — user_id FK (per-user connections)
  download_jobs  — user_id FK (per-user queue)
```

This means when User A downloads a track, User B can also see and play it — that's the desired family behavior. Each person gets their own stats, playlists, and streaming connections.

---

## 4. Admin User Management

### Who Is Admin?

The first user created during initial setup is automatically the admin (`users.is_admin = true`). Admin status is a boolean on the `users` table. The admin can create/delete family accounts and (optionally) reset passwords.

### Where: Settings → "Family Profiles" Section

A new section in `SettingsPage.tsx`, **only visible to admin users.** Below Apple Music settings, above any future sections.

### Mockup

```
┌─────────────────────────────────────────────────────┐
│ Family Profiles                                      │
│ ─────────────────────────────────────────────────── │
│ Manage who has access to Lexicon on this computer.   │
│                                                     │
│ ┌───────────┬──────────┬────────┬─────────────────┐ │
│ │ Username  │ Role     │ Added  │ Actions         │ │
│ ├───────────┼──────────┼────────┼─────────────────┤ │
│ │ Kevin     │ Admin    │ Initial│ (you)           │ │
│ │ Sarah     │ Member   │ 3 days │ [Reset PW] [✕] │ │
│ │ Emma      │ Member   │ 1 day  │ [Reset PW] [✕] │ │
│ └───────────┴──────────┴────────┴─────────────────┘ │
│                                                     │
│ ┌──────────────────────┐                            │
│ │ [+ Add Family Member]│                            │
│ └──────────────────────┘                            │
└─────────────────────────────────────────────────────┘
```

### Add Family Member Flow

Clicking "+ Add Family Member" opens a small modal:

```
┌──────────────────────────────────────┐
│   Add Family Member                   │
│                                      │
│   Username:                          │
│   ┌────────────────────────────────┐ │
│   │ Sarah                          │ │
│   └────────────────────────────────┘ │
│                                      │
│   Password:                          │
│   ┌────────────────────────────────┐ │
│   │ ••••••••                  👁   │ │
│   └────────────────────────────────┘ │
│                                      │
│   [Cancel]              [Create]     │
└──────────────────────────────────────┘
```

**Validation:**
- Username: 3–32 chars, alphanumeric + underscores, unique
- Password: minimum 4 chars (this is a family desktop app, not a bank)
- Error states for duplicate username, empty fields

On success: user created, toast "Sarah added to Lexicon", table refreshes.

### Delete Family Member

Clicking the ✕ button → confirmation dialog:
```
┌───────────────────────────────────────────────────┐
│   Delete Sarah's profile?                          │
│                                                   │
│   This will permanently remove all of Sarah's      │
│   playlists, listening history, and settings.      │
│   Downloaded files on disk are not affected.       │
│                                                   │
│   [Cancel]               [Delete Sarah's Profile]  │
└───────────────────────────────────────────────────┘
```

Red danger button for the confirm action. After deletion: toast, table refreshes. Admin cannot delete their own account (the button is hidden for the admin row).

### Reset Password

Small inline modal: "Enter new password for Sarah" with password + confirm password fields. Submit → success toast. Simple, no email verification (desktop app, not web service).

---

## 5. Visual Design

### Principles

1. **Match Lexicon's existing dark theme exactly.**
2. **Minimal, clean, no unnecessary decoration.**
3. **Use existing component patterns** — the same input style as Apple Music Settings, the same button style as "Connect Spotify", the same `bg-panel border border-panel2 rounded-lg` card style.
4. **Lucide icons only** — Lexicon already uses Lucide React. Relevant icons:

| Purpose | Icon | Component |
|---------|------|-----------|
| Username field | `User` | LoginScreen |
| Password field | `Lock` | LoginScreen |
| Show/hide password | `Eye` / `EyeOff` | LoginScreen |
| Active user indicator | `UserCircle` or `CircleUser` | Sidebar header |
| Admin badge | `Shield` | User menu |
| Log out | `LogOut` | User menu |
| Switch user | `UserSwitch` or `ArrowLeftRight` | User menu |
| Reset password | `Key` | Admin panel |
| Delete user | `Trash2` | Admin panel |
| Add user | `UserPlus` | Admin panel |
| Family section | `Users` | Settings nav |

### Color Palette (Lexicon Existing)

- Background: `var(--color-bg)` (very dark, ~#0f0f13)
- Panel: `bg-panel` (~#1a1a24)
- Panel secondary: `bg-panel2` (~#252534)
- Text: `text-text` (~#e4e4ec)
- Muted: `text-muted` (~#8888a0)
- Accent: `text-accent` / `bg-accent` (~#7c6ff0, purple/indigo)
- Error: `text-red-400` / `border-red-400/30` / `bg-red-400/10`
- Success: `text-green-400` / `bg-green-400/10` / `border-green-400/30`
- Input background: `bg-inputbg` (~#15151e)

### Login Screen Specific

- **Background:** The full viewport uses the Lexicon bg color. No sidebar, no player bar.
- **Card:** Uses the familiar `bg-panel border border-panel2 rounded-xl` with generous padding (`p-10`).
- **Logo:** The same `icon.svg` used in the sidebar header (line 92 of App.tsx). Size: 64x64 for desktop, 48x48 for mobile.
- **Button:** Full-width, accent color. Matches the style of "Connect Spotify" but wider. `bg-accent hover:bg-accent/80 text-white font-medium py-2.5 rounded-md transition-colors`.
- **Input focus ring:** Purple glow matching accent color. `focus:ring-1 focus:ring-accent focus:border-accent outline-none`.

### User Menu (Sidebar Header)

Replaces the current static sidebar header (App.tsx lines 90-94):

```tsx
// Current:
<div className="px-5 py-4 border-b border-black/40">
  <div className="flex items-center gap-2.5">
    <img src="/icon.svg" alt="Lexicon" className="w-7 h-7 rounded" />
    <span className="text-lg font-semibold tracking-wide">Lexicon</span>
  </div>
</div>

// Proposed:
<div className="px-5 py-4 border-b border-black/40">
  <div className="flex items-center justify-between">
    <div className="flex items-center gap-2.5">
      <img src="/icon.svg" alt="Lexicon" className="w-7 h-7 rounded" />
      <span className="text-lg font-semibold tracking-wide">Lexicon</span>
    </div>
    <button onClick={toggleUserMenu} className="relative group">
      <UserCircle size={20} className="text-muted group-hover:text-text transition-colors" />
      {/* Admin badge: tiny shield icon if user.isAdmin */}
    </button>
  </div>
  {/* Subtle username below the logo */}
  <p className="text-xs text-muted mt-1 ml-9">Kevin</p>
</div>
```

### User Dropdown Menu

```
┌──────────────────────────┐
│ 👤 Kevin            ✓    │ ← current user, highlighted row
│                          │
│ ── Switch to ─────────── │
│ 👤 Sarah                  │
│ 👤 Emma                   │
│ ──────────────────────── │
│ ⚙ Settings               │
│ 📤 Log out                │
└──────────────────────────┘
```

Styled as a floating dropdown (`absolute`, `right-0`, `top-full`, `mt-2`), using `bg-panel border border-panel2 rounded-lg shadow-xl` with `z-50`. Each row: `px-4 py-2 text-sm hover:bg-panel2/50 cursor-pointer`. Current user row has `bg-accent/10`.

---

## 6. State Management — UserContext

### Context Shape

```typescript
// contexts/UserContext.tsx

interface User {
  id: number;
  username: string;
  isAdmin: boolean;
  token: string;         // session token, attached to all API requests
}

interface UserContext {
  user: User | null;
  isLoading: boolean;     // true while checking session on startup
  login: (username: string, password: string, rememberMe: boolean) => Promise<void>;
  logout: () => void;
  switchUser: (username: string, password: string) => Promise<void>;
}
```

### Provider Behavior

1. **On mount:** Check `localStorage` for a saved token. If found, call `GET /api/auth/session` with it. On success, populate `user` state and proceed to the app. On 401 (expired/invalid token), clear storage and show login screen.

2. **`login()`:** POST `/api/auth/login` → receives `{ token, user: { id, username, isAdmin } }`. If `rememberMe`, store token in `localStorage`. Update context state → app renders.

3. **`logout()`:** DELETE `/api/auth/session` (invalidate token on server). Clear `localStorage` token. Set `user = null` → login screen renders.

4. **`switchUser()`:** POST `/api/auth/switch` with target username + password → receives new token + user object. Pause audio, clear queue, swap context state.

5. **401 interceptor:** The `j()` helper in `api.ts` should check for 401 responses. If the current token is rejected, call `logout()`. This could be a callback registered via `UserContext`:

```typescript
// api.ts modification:
let onUnauthorized: (() => void) | null = null;

export function setOnUnauthorized(cb: () => void) {
  onUnauthorized = cb;
}

// In j() helper:
if (r.status === 401 && onUnauthorized) {
  onUnauthorized();
  throw new Error("Session expired. Please log in again.");
}
```

UserProvider calls `setOnUnauthorized(handleUnauthorized)` on mount.

### Provider Hierarchy

```
ErrorBoundary
  ToastProvider       ← toasts available on login screen too
    UserProvider      ← NEW: authentication gate
      {!user ? <LoginScreen /> :
        PlayerProvider ← only mounts when authenticated
          DownloadProvider
            HelpProvider
              AppContent
      }
```

This means `useUser()` is available everywhere, but `usePlayer()` only exists post-login. Clean separation.

---

## 7. Active User Indicator

### Location: Sidebar Header (Desktop) / Top Bar (Mobile)

The active user is shown subtly in the header area so the user always knows who's logged in.

### Desktop

Right side of the sidebar header bar (see section 5 mockup). The `UserCircle` icon doubles as a button that opens the user dropdown menu. Below the logo + app name, a small muted text line shows the username:

```
[icon] Lexicon           👤◂── clickable, opens user menu
       Kevin             ◂── subtle, text-muted, text-xs
```

The dot next to "Kevin" could be a green dot (`w-1.5 h-1.5 rounded-full bg-green-400 inline-block`) to indicate "active session."

### Mobile

In the mobile layout, the user indicator goes in the top area. Since mobile has no sidebar, we add a small bar at the very top:

```
┌──────────────────────────────────┐
│ 👤 Kevin                     ⚙  │  ← 40px bar, bg-panel
├──────────────────────────────────┤
│                                  │
│        [main content]            │
│                                  │
└──────────────────────────────────┘
│   [MobileNavBar at bottom]       │
└──────────────────────────────────┘
```

The user avatar opens the same dropdown menu. Settings gear icon provides quick access to the Settings page (profile switching, admin panel).

### If Admin

A tiny `Shield` icon next to the username, colored in accent (purple). `size={10}` or even `size={8}`. Subtle enough not to dominate but visible when you look for it. Hover tooltip: "Administrator."

```
       Kevin 🛡     ← shield = admin indicator
```

---

## 8. Implementation Notes

### New Files

| File | Purpose |
|------|---------|
| `frontend/src/contexts/UserContext.tsx` | User state provider (login/logout/switch) |
| `frontend/src/components/LoginScreen.tsx` | Full-screen login card |
| `frontend/src/components/UserMenu.tsx` | Dropdown for switching users, logout |
| `frontend/src/components/AdminUserPanel.tsx` | Family profile management (Settings section) |
| `frontend/src/components/SwitchUserModal.tsx` | Re-auth modal for profile switching |

### Modified Files

| File | Change |
|------|--------|
| `App.tsx` | Wrap with UserProvider, conditional LoginScreen vs AppContent |
| `api.ts` | Add `setOnUnauthorized()`, all requests attach `Authorization: Bearer` |
| `SettingsPage.tsx` | Add "Family Profiles" section (admin-only) |
| `PlayerContext.tsx` | Add `reset()` method (clear queue, pause, null current) |
| `index.css` | Add shake animation keyframes for login error |

### Backend API Endpoints Needed

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/auth/login` | Authenticate: `{username, password, remember_me}` → `{token, user}` |
| `GET` | `/api/auth/session` | Validate current token → `{user}` or 401 |
| `DELETE` | `/api/auth/session` | Invalidate token (logout) |
| `POST` | `/api/auth/switch` | Re-authenticate for profile switch: `{username, password}` → `{token, user}` |
| `POST` | `/api/users` | Admin: create family member `{username, password}` |
| `DELETE` | `/api/users/:id` | Admin: delete family member |
| `POST` | `/api/users/:id/reset-password` | Admin: reset password `{new_password}` |
| `GET` | `/api/users` | Admin: list all users |

### Token Design

**Recommendation: HMAC-SHA256 signed token, not JWT.** JWT adds complexity (key rotation, expiry embedded in token) that a desktop app doesn't need. A simple signed token pattern:

```
Token format: base64(user_id + ":" + random_bytes + ":" + expiry_timestamp + ":" + hmac_signature)
```

- Server stores the `random_bytes` portion in a `sessions` table alongside user_id, created_at, expires_at
- On validation: parse token → lookup session → verify signature → check expiry → return user
- On logout: delete session row
- Token is opaque to the frontend; it just stores and sends it

Alternative: simple random token (UUIDv4) stored in `sessions` table. Even simpler, no signing needed:
```
Token = crypto.randomUUID()
sessions table: { token TEXT PK, user_id FK, created_at, expires_at, remember_me BOOL }
```
On login: generate UUID, insert session row, return token. On auth check: `SELECT user_id FROM sessions WHERE token = ? AND expires_at > NOW()`. This is what Plex effectively does. **Recommend this approach** for simplicity — no cryptographic signing to maintain.

### Migration Path

Since Lexicon is currently single-user (no user table, no token system):

1. v3.6.0: Add `users` and `sessions` tables to DB migration. On first run after upgrade, auto-create an admin user from the existing `LEXICON_API_KEY` or prompt for initial setup. Add `user_id` columns to `plays`, `playlists`, `recommendations`, `spotify_tokens`, `apple_music_*`, `download_jobs`. Existing data gets `user_id = 1` (the auto-created admin).

2. v3.6.0: Frontend: Add UserProvider + LoginScreen. Login screen only appears if multiple users exist. If only one user exists, auto-login (preserving current behavior for single-user setups).

3. v3.7.0: Admin panel + user switching.

### Security Considerations

- **Password hashing:** bcrypt (cost 12) on the server. Never store plaintext.
- **Brute force protection:** Rate limit login attempts to 5/minute per username.
- **No password recovery:** This is a local desktop app. If someone forgets their password, the admin resets it.
- **Token never in URL:** Always in `Authorization` header, never as query param.
- **localStorage vs cookie:** `localStorage` is acceptable here because Lexicon is a desktop app, not a website. No XSS surface (no user-generated HTML, no ad networks). httpOnly cookies would require the Go backend to set them and would add CSRF concerns; not worth it for a local-only app.

### Edge Cases

| Scenario | Behavior |
|----------|----------|
| Token expires mid-session | Next API call returns 401 → logout → login screen |
| Remember-me token from deleted user | `GET /api/auth/session` returns 401 → clear localStorage → login screen |
| Admin deletes currently active user | On their next request, 401 → login screen |
| Two instances of Lexicon open with same user | Both share the same token; both work (this is fine for a desktop app) |
| Admin changes own password | Current sessions remain valid until expiry or explicit logout |

### Appendix: Prior Art References

**Plex:** "Who's watching?" screen with user cards. PIN-based switching. Admin managed via Plex Web settings → Users & Sharing. Each user has separate watch history, ratings, and "On Deck."

**Jellyfin:** Login screen with user list dropdown (or manual username entry). Quick user switch from home screen. Admin dashboard with user CRUD.

**Netflix:** Profile selection screen with colored avatars. No password per profile (just a 4-digit PIN option). Kids profile with content restrictions.

**Spotify Family:** Individual accounts managed by a plan manager. No profile switching on the same device — each family member logs into their own account on their own device.

**Apple Music Family:** Individual Apple IDs within a Family Sharing group. Each person has their own library and recommendations.

**Relevance to Lexicon:** Plex and Jellyfin are the closest analogs — local media servers with multi-user support on shared hardware. Netflix's profile model is also relevant (shared device, separate viewing histories). Lexicon should follow the Plex/Jellyfin pattern: shared media roots, per-user metadata.

---

## Summary & Recommendations

1. **Login screen** as a conditional pre-app gate (not a route)
2. **Profile switching** requires re-authentication (password), accessible from sidebar user menu or Settings
3. **Library isolation** via per-user metadata tables (plays, playlists, recommendations, connections) while sharing the track catalog
4. **Admin management** as a Settings section visible only to admin users — simple CRUD for family accounts
5. **Visual design** fully aligned with Lexicon's existing dark theme, component patterns, and Lucide icon set
6. **State management** via a new `UserContext` provider that sits above PlayerProvider and gates the entire app
7. **Active user indicator** as a subtle dropdown trigger in the sidebar header (desktop) or top bar (mobile)

**Implementation priority:** The login screen + UserContext is the foundation. Everything else builds on it. Recommend implementing in this order:
1. Backend: users table, sessions table, auth endpoints, user_id migration
2. Frontend: UserContext + LoginScreen (single-user setup auto-skips login)
3. Frontend: User menu + active indicator
4. Frontend: Admin panel + profile switching
5. Frontend: Per-user data isolation (API changes to filter by user_id)
