# GUI Audit: LoginPage + AdminPages

**Date:** 2026-05-22
**Auditor:** Atlas (researcher)
**Scope:** LoginPage.tsx, AdminUsersPage.tsx + supporting auth infrastructure (UserContext, api.ts, App.tsx routing)

---

## 1. LoginPage.tsx (`frontend/src/pages/LoginPage.tsx`, 131 lines)

### 1.1 MISSING FEATURES

**M1 — No "Remember Me" / persistent session toggle** (line 6-41)
The login form has no "Remember me" checkbox. The backend already supports token-based sessions via localStorage (`lexicon_session`), but the user has no control over session persistence. Every browser restart requires re-login even though the token is stored. This is a desktop app — users expect to stay logged in.

**M2 — No password reset / forgot password flow** (line 78-103)
There's no "Forgot password?" link. If a user forgets their password, there's no self-service recovery. An admin must use the AdminUsersPage to recreate the account.

**M3 — No keyboard shortcut to submit** (line 56-127)
The form supports Enter to submit (via `<form onSubmit>`), but there's no explicit handling. This is actually fine for a single form, but the password visibility toggle button at line 93-101 has `tabIndex={-1}` which means keyboard users can't tab to it — acceptable for a decorative toggle but worth noting.

**M4 — No loading state on initial mount** (line 6-15)
When the page loads, there's no check for an existing valid session. If a user is already logged in (valid token in localStorage) and navigates to `/login`, they see the login form instead of being redirected to `/`. The `AuthGuard` in App.tsx handles the reverse (redirect to /login if not authenticated), but the login page itself doesn't check.

**M5 — No rate limiting feedback** (line 26-40)
After multiple failed login attempts, the UI doesn't implement any backoff or lockout feedback. The error message is always the same generic "Invalid username or password." — no indication of how many attempts remain or if the account is temporarily locked.

**M6 — No Caps Lock warning** (line 84-91)
No Caps Lock detection for the password field. Common UX pattern for login forms.

### 1.2 POOR IMPLEMENTATIONS

**P1 — Error clearing on every keystroke** (line 72, 89)
Both `onChange` handlers call `setError("")` on every keystroke. This causes an unnecessary re-render for each character typed. Should clear error only on submit or use a debounced approach.

**P2 — Error message truncation is arbitrary** (line 36)
`msg.length < 120 ? msg : "Login failed. Please try again."` — the 120-char threshold is arbitrary. A better approach would be to parse the error for known patterns and show user-friendly messages, or truncate with ellipsis.

**P3 — No form field validation feedback** (line 20-23)
Empty field validation only shows a generic error message. There's no per-field highlighting or individual error states. The user doesn't know which field is empty.

**P4 — Password field doesn't clear on failed login** (line 26-40)
After a failed login attempt, the password field retains its value. Security best practice is to clear the password on failed login. The username is also retained, which is fine.

### 1.3 BUGS

**B1 — Race condition on rapid submit** (line 17-41)
If the user clicks "Sign in" multiple times quickly (before `submitting` state updates), multiple `login()` calls can fire. The `submitting` flag is set in the same render cycle, but React's state batching means rapid clicks could trigger multiple requests. Should disable the button immediately via ref or use a proper async guard.

**B2 — Error state not cleared on new submit attempt** (line 24-26)
`setError("")` is called before `setSubmitting(true)`, but if the previous error was set and the user starts typing, the error is cleared (via onChange). However, if the user submits without typing (e.g., correcting password only), the error from the previous attempt is cleared only at submit time — this is actually fine.

**B3 — `usernameRef` is unused for focus management** (line 9, 66)
The `usernameRef` is assigned to the username input and `autoFocus` is also set. The ref is never used programmatically. This is dead code — either use it for focus management (e.g., focus on error) or remove it.

### 1.4 VISUAL ISSUES

**V1 — Inconsistent spacing in login card** (line 56-127)
The form uses `space-y-4` for the card layout, but the error message (line 106-110) is outside the flow — it appears between the password field and submit button without margin adjustment. When the error appears, it pushes the submit button down abruptly.

**V2 — No mobile-specific layout** (line 44-130)
The login page uses a centered card layout with `max-w-sm` but has no mobile-specific adjustments. The `px-4` padding is fine, but on small screens the card could use more breathing room. No `useIsMobile()` check.

**V3 — Password toggle button positioning** (line 93-101)
The toggle button uses `absolute right-2 top-1/2 -translate-y-1/2` which works but has no `aria-pressed` state. The `aria-label` changes based on `showPassword`, which is good, but screen readers won't know the current state without `aria-pressed`.

### 1.5 ACCESSIBILITY

**A1 — Missing `aria-required` on form fields** (line 65-91)
Neither the username nor password input has `aria-required="true"`. The validation is only handled in JS.

**A2 — Error message not linked to form fields** (line 106-110)
The error `<p>` has no `role="alert"` and is not connected to either input via `aria-describedby`. Screen readers won't announce the error when it appears.

**A3 — No `aria-live` region for dynamic errors** (line 106-110)
When the error message appears, screen readers won't announce it because there's no `aria-live="polite"` or `role="alert"` on the error container.

**A4 — Password toggle missing `aria-pressed`** (line 93-101)
The show/hide password button should have `aria-pressed={showPassword}` to indicate its toggle state to screen readers.

**A5 — Form has no `aria-label` or `name`** (line 56-127)
The `<form>` element has no `aria-label` describing its purpose.

### 1.6 PERFORMANCE

**Perf1 — Minimal performance concerns**
The component is small (131 lines) with minimal state. No unnecessary re-renders except the error clearing on keystroke (P1). No memoization needed for this component size.

---

## 2. AdminUsersPage.tsx (`frontend/src/pages/AdminUsersPage.tsx`, 279 lines)

### 2.1 MISSING FEATURES

**M1 — No user search/filter** (line 213-276)
The user list has no search or filter capability. For a family with many accounts, this will become unwieldy.

**M2 — No user editing** (line 220-273)
There's no way to edit a user's display name, password, or admin status after creation. The only management action is deletion.

**M3 — No password change/reset for existing users** (line 52-83)
The create form has a password field, but there's no way to reset another user's password. If a family member forgets their password, the admin must delete and recreate the account.

**M4 — No admin status toggle** (line 236-241)
Admin status is displayed as a badge but there's no way to grant or revoke admin privileges from this page.

**M5 — No user activity/info display** (line 220-273)
The user list shows only display name, username, and admin badge. No last login, creation date, or play count.

**M6 — No pagination** (line 213-276)
The user list loads all users at once with no pagination. Fine for small families but could be an issue with many accounts.

**M7 — No confirmation modal for delete** (line 251-258)
Delete uses `window.confirm()` which is blocking, ugly, and inconsistent with the rest of the app's toast-based UX. Should use a proper modal or the HelpModal pattern.

**M8 — No empty state illustration** (line 215-218)
The empty state is just text: "No family accounts yet. Create one to get started." No icon or visual cue.

**M9 — No help button** (line 104-278)
Unlike every other page in the app, AdminUsersPage has no `?` help button. The help-content.ts has no entry for user management either.

**M10 — No mobile layout optimization** (line 104-278)
The page uses `max-w-2xl` but has no mobile-specific adjustments. The create form's `grid grid-cols-1 sm:grid-cols-2` is good, but the user list cards could be more touch-friendly.

### 2.2 POOR IMPLEMENTATIONS

**P1 — Redirect happens after render, not before** (line 30-34, 102)
The `useEffect` redirect at line 30-34 runs AFTER the component renders. This means non-admins briefly see the admin page before being redirected. The `if (!isAdmin) return null;` at line 102 helps, but the data loading effect at line 37-50 also fires for non-admins before the redirect.

**P2 — Data loading effect has no dependency array guard** (line 37-50)
The `useEffect` for loading users has `[]` dependency array, which is correct. However, it fires even for non-admin users (before the redirect effect). The `cancelled` flag prevents state updates but the API call still fires.

**P3 — No error state for user creation** (line 78-82)
The create error is displayed inline but cleared only when the user starts typing in the form fields. If the user dismisses the form and reopens it, the error persists until they type.

**P4 — Toast for self-delete prevention** (line 86-90)
When an admin tries to delete their own account, a toast error is shown. But the user already clicked the delete button and saw the confirm dialog — the flow should prevent showing the confirm dialog in the first place.

**P5 — Delete confirmation uses window.confirm** (line 254)
`window.confirm()` is blocking, looks native/out-of-place, and can't be styled. Inconsistent with the app's toast/modal pattern.

**P6 — No loading state for individual user actions** (line 250-270)
The delete button shows a spinner via `isDeleting` state, but there's no visual indication of which user is being processed when multiple users exist. The `deletingId` state is used correctly but the UX could be clearer.

**P7 — Form state not reset on cancel** (line 186-193)
Clicking "Cancel" hides the form but doesn't clear the form fields. If the user opens the form again, the previous values are still there.

### 2.3 BUGS

**B1 — Race condition: load fires before admin check** (line 30-50)
The `useEffect` for loading users (line 37-50) and the redirect effect (line 30-34) both fire on mount. The user load effect doesn't check `isAdmin` before firing the API call. Non-admin users trigger a `/auth/users` API call that will likely 403.

**B2 — Stale user list after creation** (line 71-73)
After creating a user, the new user is appended to the local state with `setUsers((prev) => [...prev, data.user])`. This works, but if the backend returns a different user structure than expected (e.g., missing fields), the UI could show incomplete data. A full refetch would be more robust.

**B3 — No error handling for users() API failure** (line 37-50)
The catch block sets error state but doesn't show a toast. The user only sees an inline error message. For a critical failure, a toast would be more visible.

**B4 — `isAdmin` can be stale** (line 9, 30-34)
The `isAdmin` value is destructured at render time from `useUser()`. If the admin status changes (e.g., another admin revokes privileges), the component won't re-render until the next navigation. This is a minor issue for a desktop app.

### 2.4 VISUAL ISSUES

**V1 — Inconsistent header sizing** (line 108)
The page uses `text-xl` for the heading, which is consistent with other pages. Good.

**V2 — User card layout could be tighter** (line 224-271)
The user cards use `px-4 py-3` which is fine, but the admin badge and "you" badge use different sizing (`text-[10px]`). The visual hierarchy is clear but could be more polished.

**V3 — No visual distinction between admin and regular users** (line 224-271)
Admin users only have a small badge. No color coding or icon distinction beyond the shield badge.

### 2.5 ACCESSIBILITY

**A1 — Delete button has no `aria-label`** (line 250-270)
The delete button has `title` but no `aria-label`. Screen readers will just hear "button".

**A2 — No `role="alert"` on error messages** (line 172-176, 198-202)
Error messages in both the create form and the loading error have no ARIA roles for screen readers.

**A3 — Form fields missing `aria-required`** (line 131-170)
The username and password fields in the create form have no `aria-required` attribute.

**A4 — No `aria-live` region for dynamic updates** (line 71-73, 92-94)
When a user is created or deleted, the list updates without any screen reader announcement.

**A5 — `window.confirm` is not accessible** (line 254)
Native confirm dialogs are accessible but can't be styled and are inconsistent with the app's design system.

### 2.6 PERFORMANCE

**Perf1 — Minimal performance concerns**
The component is small with limited state. The user list could benefit from virtualization for large lists, but this is unlikely for a family app.

**Perf2 — `loadPlaylists` called on every dropdown open** (line 83-90 in TrackList.tsx — not in scope but relevant)
Not directly in scope, but the pattern of loading data on dropdown open is used elsewhere and could be optimized.

---

## 3. Supporting Infrastructure Issues

### 3.1 UserContext.tsx (`frontend/src/contexts/UserContext.tsx`, 72 lines)

**I1 — Session token stored in localStorage without expiration check** (line 23-29)
The token is stored in `localStorage` under `lexicon_session` but there's no expiration check. If the token expires, the user must manually clear storage or re-login. The `api.me()` call validates the token, but there's no proactive expiration handling.

**I2 — No session refresh mechanism** (line 45-50)
The `login` function sets the token but there's no refresh token mechanism. Once the token expires, the user must log in again.

**I3 — Logout doesn't redirect to login** (line 52-57)
The `logout` function clears state but doesn't navigate to `/login`. The `AuthGuard` will redirect, but the user sees a brief flash of the authenticated UI before the redirect.

**I4 — No error handling for `api.me()` network failure** (line 29-43)
If the server is unreachable on mount, the `catch` block clears the session token. This means a temporary network issue logs the user out. Should distinguish between 401 (invalid token) and network errors.

### 3.2 App.tsx Routing (`frontend/src/App.tsx`, 283 lines)

**R1 — Login route is outside AuthGuard** (line 253-254)
Correctly placed outside the `AuthGuard`, but there's no reverse guard — authenticated users can still navigate to `/login` and see the login form.

**R2 — AdminUsersPage route has no admin guard** (line 193, 240)
The route `/settings/users` is inside `AuthGuard` but has no admin check. The `AdminUsersPage` component handles this internally with a redirect, but the route should be protected at the router level.

**R3 — No 404/not-found route** (line 182-194, 229-241)
All routes are defined but there's no catch-all 404 route. Unknown paths just render nothing.

### 3.3 api.ts Auth Methods (`frontend/src/lib/api.ts`, lines 216-228)

**API1 — Auth endpoints don't handle 401 specially** (line 216-228)
The `j()` function (line 19-68) handles errors generically. A 401 from `/auth/me` or `/auth/users` should trigger an automatic logout/redirect to login, but currently it just throws a generic error.

**API2 — No token refresh logic** (line 216-228)
The API client has no token refresh mechanism. Once the token expires, all subsequent requests fail.

---

## 4. Prioritized Fix Roadmap

### Critical (fix first)
1. **B1** — Guard user loading with admin check to prevent unnecessary API calls
2. **I4** — Distinguish 401 from network errors in UserContext session validation
3. **R2** — Add admin guard at router level for `/settings/users`
4. **API1** — Handle 401 globally with automatic logout/redirect

### High priority
5. **M9** — Add help button and help-content entry for AdminUsersPage
6. **M7** — Replace `window.confirm()` with proper modal
7. **P7** — Clear form state on cancel in AdminUsersPage
8. **A1-A5** — Add ARIA attributes to both pages
9. **M4** — Add admin status display with visual distinction

### Medium priority
10. **M2** — Add password reset flow
11. **M3** — Add user editing (display name, password)
12. **P1** — Fix redirect-before-render for non-admins
13. **V1** — Fix error message spacing in LoginPage
14. **M1** — Add "Remember Me" toggle (or auto-redirect if session valid)

### Low priority
15. **M5** — Add rate limiting feedback
16. **M6** — Add Caps Lock warning
17. **M8** — Add empty state illustration
18. **B3** — Full refetch after user creation
19. **R3** — Add 404 route
20. **I2** — Add session refresh mechanism

---

## 5. Summary

Both pages are functional but lack polish. The LoginPage is a basic login form with good structure but missing accessibility and session management features. The AdminUsersPage is a competent CRUD page but needs better UX patterns (modals instead of window.confirm, form state management, help system integration).

The biggest architectural gap is the auth infrastructure: no token refresh, no global 401 handling, and no proactive session management. These should be addressed before adding more auth-dependent features.
