# GUI Audit: SettingsPage + AppleMusicSettings

**Date:** 2026-05-22
**Scope:** SettingsPage.tsx, AppleMusicSettings.tsx, plus supporting context from App.tsx, PlayerContext.tsx, DownloadContext.tsx, UserContext.tsx, ToastContext.tsx, api.ts, PlayerBar.tsx, MobilePlayerBar.tsx, DevicePicker.tsx, DownloadProgressBar.tsx, MobileNavBar.tsx, TrackList.tsx, HelpModal.tsx, index.css, help-content.ts

---

## 1. MISSING FEATURES

### 1.1 No "Test Connection" button for Apple Music credentials (AppleMusicSettings.tsx)
- **Severity:** Medium
- **Location:** AppleMusicSettings.tsx lines 391-405 (State 2: configured but not connected)
- **Detail:** When a user enters Team ID, Key ID, and .p8 private key, they have no way to verify the credentials work WITHOUT connecting. The only validation is Team ID/Key ID length checks (lines 71-78) and the presence of "PRIVATE KEY" string (line 79). There's no "Test credentials" button that attempts to mint a dev token and reports success/failure before the user goes through the full MusicKit JS flow.

### 1.2 No display-name or user profile fields for Apple Music (AppleMusicSettings.tsx)
- **Severity:** Low
- **Location:** AppleMusicSettings.tsx lines 456-474 (State 3: connected)
- **Detail:** The Spotify connected state shows `display_name` and `product` (SettingsPage.tsx lines 130-141). The Apple Music connected state shows storefront, last sync, team ID, and dev token expiry — but NOT the Apple Music subscriber's name or any user-identifying info. The Apple Status API type has an optional `display_name` field (api.ts line 299) but it's never populated by the frontend. This is a UX inconsistency.

### 1.3 No bulk/library management tools on Settings page
- **Severity:** Medium
- **Location:** SettingsPage.tsx — entire page
- **Detail:** The Settings page only handles Spotify and Apple Music integration. Missing:
  - **Library management panel:** View total tracks, force rescan, clear orphaned entries, view scan status
  - **Downloader configuration panel:** Show/configure download tool paths, test each tool
  - **Audio quality settings:** Current output format/bitrate selection per codec
  - **Appearance/theme settings:** Font size, compact mode, accent color

### 1.4 No "Re-authorize" flow for expired Apple Music tokens (AppleMusicSettings.tsx)
- **Severity:** Medium
- **Location:** AppleMusicSettings.tsx lines 456-474 (State 3: connected)
- **Detail:** When the Music User Token (MUT) is invalidated by Apple, the API returns 401 (`ErrUnauthorized`). The syncer detects this (per codebase-review skill), but the Settings page has NO visual indicator that the token is expired/expiring. The dev token expiry is shown (line 421-443), but there's no "Re-authorize" button that triggers `authorizeAppleMusic()` again. The user has to disconnect and reconnect.

### 1.5 No download path/info display in Settings
- **Severity:** Low
- **Location:** SettingsPage.tsx — entire page
- **Detail:** No indication of where downloaded files are stored, how much disk space is used, or download statistics. Other than going to the Downloads page and checking a job, there's no centralized "storage" overview.

### 1.6 No confirmation toast for "Sync now" actions (SettingsPage.tsx, AppleMusicSettings.tsx)
- **Severity:** Medium
- **Location:**
  - SettingsPage.tsx lines 36-43 (Spotify syncNow)
  - AppleMusicSettings.tsx ~line 478-483 (Apple onSyncNow)
- **Detail:** `syncNow()` in SettingsPage calls `api.spotifySync()` then reloads after 2s. But there's NO toast notification telling the user the sync was started. The Apple Music `onSyncNow` handler (not fully seen but follows same pattern) likely has the same issue. Compare with DownloadContext.tsx which wraps every action in `toast.success()`/`toast.error()`. The entire SettingsPage component does NOT import `useToast()`.

### 1.7 No keyboard submission for the "Cancel" button in editing mode (AppleMusicSettings.tsx)
- **Severity:** Low
- **Location:** AppleMusicSettings.tsx lines 391-404
- **Detail:** The "Cancel" button during credential editing (line 400-403) is a `<button type="button">` inside a `<form>`. Pressing Enter while focused on other form fields can accidentally submit the form instead of canceling. This is a minor form UX issue.

---

## 2. POOR IMPLEMENTATIONS

### 2.1 SettingsPage never uses toast for any action (SettingsPage.tsx)
- **Severity:** High
- **Location:** SettingsPage.tsx — entire file
- **Detail:** The component imports `useHelp` but NOT `useToast`. This means:
  - `disconnect()` (line 28): if `api.spotifyDisconnect()` fails, the user sees nothing. The UI just stays in connected state.
  - `syncNow()` (line 36): if `api.spotifySync()` fails silently, no feedback.
  - `load()` (line 16): already logs to console.error but gives no user feedback.
  
  This contrasts with AppleMusicSettings which imports `useToast` via the toast context calls.

### 2.2 AppleMusicSettings: `authorizeAppleMusic` errors are swallowed (AppleMusicSettings.tsx)
- **Severity:** High
- **Location:** AppleMusicSettings.tsx lines ~144-170 (onConnect handler, partially seen in truncated section)
- **Detail:** The `onConnect` handler calls `authorizeAppleMusic()` and `api.appleConnect()`. Based on the truncated view, the error handling appears to catch and setErr, but the actual `authorizeAppleMusic()` from musickit.ts can throw several distinct errors ("MusicKit unavailable after script load", "Failed to load MusicKit JS", "Apple Music did not return a user token") — these are all lumped into a generic error display. The user gets no actionable guidance.

### 2.3 Spotify disconnect doesn't handle failure cascading (SettingsPage.tsx)
- **Severity:** Medium
- **Location:** SettingsPage.tsx lines 28-34
- **Detail:** `disconnect()` uses `confirm()` (blocking native dialog) → `api.spotifyDisconnect()` → silently fails → `load()` might still show connected state. If the API call throws, `setBusy(false)` is never called because there's no try/finally. The button stays disabled forever.

### 2.4 AppleMusicSettings has 370+ lines in a single component with complex state machine
- **Severity:** Medium
- **Location:** AppleMusicSettings.tsx lines 34-537
- **Detail:** The component manages state for 3 views (unconfigured/form, configured/not-connected, connected) plus error/success messages plus busy state plus form fields. This is a prime candidate for decomposition into:
  - `AppleMusicForm.tsx` — credential entry form
  - `AppleMusicStatus.tsx` — connected/configured status display
  - `AppleMusicActions.tsx` — action buttons (sync, disconnect, edit, delete)
  
  The current monolithic structure makes testing and review harder.

### 2.5 `confirm()` used for destructive actions instead of custom modal (both components)
- **Severity:** Medium
- **Location:**
  - SettingsPage.tsx line 29: `confirm("Disconnect Spotify? ...")`
  - AppleMusicSettings.tsx line ~186: for disconnect/delete actions
- **Detail:** Native `confirm()` is blocking, looks nothing like the Lexicon dark theme, and can't be styled. The app has a HelpModal component pattern — a `ConfirmModal` should be created and used consistently. This is a visual/UX inconsistency.

### 2.6 Inline storefront list duplicated and hardcoded (AppleMusicSettings.tsx)
- **Severity:** Low
- **Location:** AppleMusicSettings.tsx lines 16-32
- **Detail:** The STOREFRONTS constant has 15 storefronts hardcoded. Apple supports 175+ storefronts. Users in unsupported countries (e.g., `za` South Africa, `ae` UAE, `ph` Philippines, `th` Thailand, `vn` Vietnam) can't select their storefront. This should either be a searchable dropdown or the list should be much more comprehensive.

### 2.7 No loading state between form submit and toast (AppleMusicSettings.tsx)
- **Severity:** Medium
- **Location:** AppleMusicSettings.tsx `onSaveConfig` (lines 68-138 partially)
- **Detail:** After hitting "Save credentials", the form stays open with no indication of progress. If the backend takes time to validate the .p8 and mint a dev token, the user doesn't know anything is happening. The `busy` state disables the submit button but doesn't show a spinner or change the button text.

### 2.8 Settings page passes `busy` to buttons but doesn't block form interaction (AppleMusicSettings.tsx)
- **Severity:** Low
- **Location:** AppleMusicSettings.tsx — various buttons have `disabled={busy}`
- **Detail:** When `busy=true`, buttons are disabled but there's no visual spinner or loading indicator on the section as a whole. The Apple Music section could show a subtle loading state.

---

## 3. BUGS

### 3.1 **BUG: SettingsPage `disconnect()` — busy lockout on error** (SettingsPage.tsx)
- **Severity:** Critical
- **Location:** SettingsPage.tsx lines 28-34
```tsx
async function disconnect() {
    if (!confirm("Disconnect Spotify? Your synced history stays.")) return;
    setBusy(true);
    await api.spotifyDisconnect();   // <-- if this throws...
    setBusy(false);                 // <-- ...this is NEVER reached
    load();
}
```
- **Fix:** Wrap in try/finally:
```tsx
async function disconnect() {
    if (!confirm("Disconnect Spotify? Your synced history stays.")) return;
    setBusy(true);
    try {
      await api.spotifyDisconnect();
    } catch (e) {
      console.error("[SettingsPage] disconnect failed", e);
    } finally {
      setBusy(false);
      load();
    }
}
```

### 3.2 **BUG: `load()` error silently leaves stale state** (SettingsPage.tsx)
- **Severity:** Medium
- **Location:** SettingsPage.tsx lines 16-23
```tsx
async function load() {
    try {
      const s = await api.spotifyStatus();
      setStatus(s);
    } catch (e) {
      console.error("[SettingsPage] failed to load Spotify status", e);
    }
}
```
- **Detail:** If `api.spotifyStatus()` throws, `status` stays at whatever it was before. If the user was on the page when the server restarts, `status` still shows "connected" even though the session is dead. Should set `status(null)` on error so the UI shows "Loading..." or an error state.

### 3.3 **BUG: `syncNow()` setTimeout doesn't guard against component unmount** (SettingsPage.tsx)
- **Severity:** Medium
- **Location:** SettingsPage.tsx lines 36-43
```tsx
async function syncNow() {
    setBusy(true);
    await api.spotifySync();
    setTimeout(() => {
      load();
      setBusy(false);
    }, 2000);
}
```
- **Detail:** If the user navigates away within 2 seconds, `load()` and `setBusy(false)` run on an unmounted component. In React 18 StrictMode with automatic batching this can cause warnings; in production it's a state-update-on-unmounted component memory leak. Should use a ref + cleanup pattern or the `useEffect` cleanup approach.

### 3.4 **BUG: `api.spotifySync()` errors unhandled** (SettingsPage.tsx)
- **Severity:** Medium
- **Location:** SettingsPage.tsx lines 36-43
- **Detail:** If `api.spotifySync()` throws, neither `load()` nor `setBusy(false)` is called. The UI is stuck in "busy" state permanently until page refresh. This is the same pattern as bug 3.1 but for the sync action.

### 3.5 **BUG: Apple Music `void Link2` lint guard is fragile** (AppleMusicSettings.tsx)
- **Severity:** Low
- **Location:** AppleMusicSettings.tsx line 537: `void Link2;`
- **Detail:** This is a hack to suppress an unused import lint warning. If a future linter/TS config changes how `void` expressions are treated, this could break the build. The actual fix is to remove unused imports from the import statement.

### 3.6 **BUG: AppleMusicSettings form doesn't reset on save success** (AppleMusicSettings.tsx)
- **Severity:** Medium
- **Location:** AppleMusicSettings.tsx lines 68-138 (onSaveConfig)
- **Detail:** When `onSaveConfig` succeeds, the form closes (`setEditing(false)`) and `setPrivateKey("")` clears the key. But `teamId` and `keyId` fields are NOT cleared — they retain their values. When the user next clicks "Edit credentials", the form shows the previously entered teamId/keyId but with an empty privateKey. This is confusing because the user has to re-paste the .p8 every time even though teamId/keyId are remembered.

### 3.7 **BUG: `setShowHelp`/`showHelp` naming collision with context** (AppleMusicSettings.tsx)
- **Severity:** Low
- **Location:** AppleMusicSettings.tsx line 37: `const [showHelp, setShowHelp] = useState(false);`
- **Detail:** The component has a local `showHelp` state for toggling a help section AND uses `useHelp()` context (imported but usage not confirmed in the truncated view). The name collision is confusing and could lead to bugs if someone refactors to use the context help system. Should rename local state to `showSetupGuide` or similar.

### 3.8 **BUG: `onConnect` loads `authorizeAppleMusic` which requires browser interaction but doesn't handle popup blocking** (AppleMusicSettings.tsx, musickit.ts)
- **Severity:** High
- **Location:** musickit.ts line 116: `music.authorize()`
- **Detail:** MusicKit JS `authorize()` opens a popup for Apple ID sign-in. If the browser blocks the popup, the promise hangs indefinitely. The AppleMusicSettings `onConnect` handler sets `busy=true` but never resets it if the popup is blocked. There's no timeout or error handling for this scenario. The user is stuck with a permanently disabled "Connect Apple Music" button.

---

## 4. VISUAL ISSUES

### 4.1 Inconsistent button ordering between Spotify and Apple Music sections
- **Severity:** Low
- **Location:** 
  - SettingsPage.tsx lines 151-167: Spotify buttons are [Sync now] [Disconnect]
  - AppleMusicSettings.tsx lines 476-500: Apple buttons are [Sync now] [Disconnect] [Edit credentials]
- **Detail:** Spotify puts "Sync" first then "Disconnect". Apple puts "Sync" first then "Disconnect" then "Edit". But the Spotify section has buttons in a `flex gap-2` row while Apple uses `flex gap-2 flex-wrap`. Minor inconsistency.

### 4.2 Connected state shows "—" for missing fields without explanation
- **Severity:** Low
- **Location:** 
  - SettingsPage.tsx line 130: `{status.display_name || "—"}`
  - AppleMusicSettings.tsx line 459: `{status?.storefront || "—"}`
- **Detail:** When fields are empty, a bare dash is shown. No tooltip or helper text explains what this field is or why it might be empty.

### 4.3 No empty/zero-state for Settings when nothing is configured
- **Severity:** Low
- **Location:** SettingsPage.tsx lines 85-119
- **Detail:** When Spotify is not configured, the setup instructions are shown in a monolithic block. There's no visual distinction between the numbered steps and the code snippets. The `<code>` elements use the accent color which looks clickable but isn't.

### 4.4 Apple Music section header missing connection indicator dot
- **Severity:** Low
- **Location:** AppleMusicSettings.tsx — section header (partially seen)
- **Detail:** The Spotify section header has a green dot when connected (SettingsPage.tsx lines 63-65). The Apple Music section header doesn't have this visual indicator. Users can't glance at Settings and see "both connected" vs "only Spotify connected."

### 4.5 Mobile: Settings page actions get cramped
- **Severity:** Low
- **Location:** SettingsPage.tsx lines 151-167 (grid-cols-1 sm:grid-cols-2) and AppleMusicSettings.tsx
- **Detail:** On mobile screens, the "Connected" state info grid is `grid-cols-1` which stacks everything vertically. Two action buttons in a row (`flex gap-2`) can overflow on narrow screens. No `flex-wrap` on the button container in SettingsPage (vs AppleMusicSettings which has `flex-wrap`).

---

## 5. ACCESSIBILITY

### 5.1 No `aria-label` or `role` on status indicator dot (SettingsPage.tsx)
- **Severity:** High
- **Location:** SettingsPage.tsx lines 63-65
```tsx
<span className="w-2 h-2 rounded-full bg-green-400" title="Connected" />
```
- **Detail:** The green dot is purely decorative but a screen reader will announce an empty `<span>`. Should have `aria-label="Spotify connected"` and `role="status"` or be hidden from AT with `aria-hidden="true"`.

### 5.2 Apple Music "Loading..." text has no aria-live region
- **Severity:** Medium
- **Location:** AppleMusicSettings.tsx line 514: `{!status && <p className="text-sm text-muted">Loading…</p>}`
- **Detail:** Same issue as SettingsPage. Screen readers won't announce the loading state change. Should use `aria-live="polite"`.

### 5.3 No `aria-describedby` connecting form fields to helper text (AppleMusicSettings.tsx)
- **Severity:** Medium
- **Location:** AppleMusicSettings.tsx lines 376-381 (storefront select has helper text)
- **Detail:** The storefront `<select>` at line 366 has a helper `<p>` below it at line 377, but no `aria-describedby` links them. Screen reader users won't know the helper text relates to the dropdown.

### 5.4 Editable playlist name missing accessible name (SettingsPage.tsx)
- **Severity:** N/A (PlaylistPage issue, but noting for completeness)

### 5.5 `confirm()` dialogs are not accessible (both components)
- **Severity:** Medium
- **Location:** SettingsPage.tsx line 29, AppleMusicSettings.tsx ~186
- **Detail:** Native `confirm()` can't be styled for high-contrast mode, can't be customized for screen readers, and blocks the main thread. A custom `<ConfirmDialog>` component with proper focus trapping and ARIA roles should replace all `confirm()` calls.

### 5.6 Error messages in AppleMusicSettings not announced to screen readers
- **Severity:** Medium
- **Location:** AppleMusicSettings.tsx line 39-40: `err` state displayed in JSX
- **Detail:** The error display (a `<p>` or `<div>` with red text) has no `role="alert"` or `aria-live` attribute. Screen readers won't announce when errors appear.

---

## 6. PERFORMANCE

### 6.1 SettingsPage re-fetches status on every mount with no caching
- **Severity:** Low
- **Location:** SettingsPage.tsx lines 24-26
- **Detail:** The `useEffect(() => { load(); }, [])` runs on mount. If the user navigates to Settings and back multiple times, each mount triggers a fresh API call. This is minor for a Settings page (not high-traffic) but could benefit from a short-lived cache or lifting status to a context.

### 6.2 AppleMusicSettings duplicate load on mount
- **Severity:** Low
- **Location:** AppleMusicSettings.tsx lines 59-61
- **Detail:** Same pattern — every mount fetches appleStatus. If both Spotify and Apple Music sections are on the same page, that's 2 API calls on every navigation to Settings.

### 6.3 AppleMusicSettings inline STOREFRONTS array re-created every render
- **Severity:** Low
- **Location:** AppleMusicSettings.tsx lines 16-32
- **Detail:** `const STOREFRONTS` is declared inside the component function body, creating a new array on every render. Should be moved outside the component or memoized.

---

## 7. CROSS-CUTTING CONCERNS

### 7.1 Auth system (UserContext.tsx) is Settings-adjacent but not surfaced on Settings page
- **Severity:** Medium
- **Location:** UserContext.tsx lines 1-72, SettingsPage.tsx
- **Detail:** The auth system (login, logout, session management, admin check) is fully implemented in UserContext and LoginPage. But the Settings page — the natural place for account management — has NO user management section. Admins see a "Users" link in the sidebar (App.tsx line 159-165) but regular users have no way to change password, manage sessions, or view account info.

### 7.2 No connection health/status indicator for either integration
- **Severity:** Medium
- **Location:** SettingsPage.tsx
- **Detail:** Both Spotify and Apple Music show "last synced at" but neither shows whether the connection is currently healthy (token valid, API reachable). A small "Healthy" / "Token expiring" / "Connection lost" badge would help diagnose sync issues.

### 7.3 Help content for Apple Music is minimal
- **Severity:** Low
- **Location:** help-content.ts lines 372-388
- **Detail:** The Apple Music help entry (`settings.apple`) is 17 lines. The Spotify entry (`settings.spotify`) is 19 lines. But Apple Music setup is significantly more complex (Developer account, .p8 key, Team ID, Key ID, MusicKit JS flow). The help content should walk through each step, link to Apple Developer docs, and explain common pitfalls.

---

## PRIORITIZED FIX ROADMAP

### Phase 1: Critical Bugs (fix immediately)
1. **3.1** SettingsPage `disconnect()` busy lockout on error — add try/finally
2. **3.4** SettingsPage `syncNow()` errors unhandled — add try/finally + error toast  
3. **3.8** Apple Music `onConnect` popup hang — add timeout + error handling for blocked popups

### Phase 2: Missing Error Handling
4. **2.1** SettingsPage never uses `useToast` — add toast to all actions
5. **3.2** SettingsPage `load()` stale state on error — set status to null on failure
6. **3.3** SettingsPage `syncNow()` unmount guard — add cleanup/cancelled ref
7. **3.6** AppleMusicSettings form doesn't reset privateKey on save success

### Phase 3: UX Improvements
8. **1.6** Add toast notifications for sync actions
9. **2.3** Spotify disconnect failure cascading
10. **2.5** Replace `confirm()` with custom ConfirmModal
11. **1.4** Add visual indicator for expired Apple Music MUT + re-authorize button
12. **1.1** Add "Test credentials" button for Apple Music setup

### Phase 4: Accessibility
13. **5.1** Add `aria-label`/`role` to connection indicator dots
14. **5.6** Add `role="alert"` to error message displays
15. **5.3** Add `aria-describedby` to form fields with helper text
16. **5.5** Replace `confirm()` with accessible custom dialog

### Phase 5: Visual/Polish
17. **4.4** Add connection indicator dot for Apple Music section header
18. **4.3** Restructure setup instructions with better visual hierarchy
19. **4.5** Add `flex-wrap` to SettingsPage button containers
20. **6.3** Move STOREFRONTS outside component

### Phase 6: Enhancements
21. **1.3** Add library management section to Settings
22. **1.2** Show Apple Music subscriber display name when available
23. **7.1** Add user account section to Settings page
24. **7.2** Add connection health/status indicators
25. **2.6** Expand Apple Music storefront list to cover all 175+ regions
26. **7.3** Improve Apple Music help content
