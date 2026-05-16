# Lexicon Mobile Browser Optimization — Implementation Plan

> Generated via sequential analysis of detection strategies, layout architectures, and component impact. This plan prioritizes minimal new architecture while delivering a fully touch-friendly mobile experience.

---

## 1. Executive Decision Summary

| Question | Decision |
|---|---|
| Detection method | `useIsMobile()` hook via `window.matchMedia('(max-width: 768px)')` |
| Layout paradigm | Bottom tab navigation (primary) + overflow sheet (secondary) |
| Player paradigm | Collapsible mini player → full-screen expanded overlay |
| Track list paradigm | Card-based layout on mobile, table on desktop (conditional render) |
| Page strategy | Keep existing pages, add responsive classes + minor conditional rendering |
| New files | 3–4 presentational components + 1 hook |
| Desktop impact | Zero — all mobile changes are gated behind `isMobile` |

### Why This Route Wins
- **One hook, not a Context** — `useIsMobile` is derived state; no Provider boilerplate needed.
- **No duplicate business logic** — `TrackList` keeps its data logic; only rendering branches.
- **Standard mobile UX** — bottom tabs and mini players are immediately understood by users.
- **Incrementally deliverable** — Phase 2 (shell) alone makes the app usable on a phone.
- **Aligns with existing Tailwind** — leverages `md:` breakpoints already present in some pages.

---

## 2. Phased Implementation Roadmap

### Phase 1 — Foundation (Invisible Changes)
**Goal:** Enable mobile rendering without changing the visible desktop experience.

1. **Add viewport meta tag** to `frontend/index.html`
   ```html
   <meta name="viewport" content="width=device-width, initial-scale=1.0" />
   ```

2. **Create `useIsMobile` hook** (`src/hooks/useIsMobile.ts`)
   - Listens to `matchMedia('(max-width: 768px)')`
   - Reacts automatically to rotation and devtools resizing
   - Returns stable boolean for component gating

3. **Touch-friendly CSS base** (`src/index.css`)
   - `-webkit-tap-highlight-color: transparent`
   - Ensure no element relies solely on `:hover` for discoverability
   - Remove `user-scalable=no` (accessibility anti-pattern)

---

### Phase 2 — Mobile Layout Shell
**Goal:** Replace the fixed sidebar with a mobile-native navigation paradigm.

4. **Create `MobileNavBar`** (`src/components/MobileNavBar.tsx`)
   - **Bottom tab bar:** 5 primary actions (Home, Music, Search, Discover, More)
   - **Overflow sheet:** Tapping "More" slides up a sheet with secondary routes (Podcasts, Playlists, Downloads, Analytics, Settings)
   - Active state uses existing accent color (`text-accent` / `bg-accent`)
   - Icons from `lucide-react` (already a dependency)

5. **Modify `App.tsx`**
   - Desktop (`!isMobile`): keep existing `<aside>` sidebar + `<main>` layout exactly as-is
   - Mobile (`isMobile`): hide sidebar entirely, render `<MobileNavBar>` fixed at bottom
   - Main content area adds bottom padding equal to nav bar height to prevent overlap

6. **Test navigation** across all 9 routes on mobile viewport

---

### Phase 3 — Mobile Player
**Goal:** Replace the desktop player bar with a thumb-friendly mini player + expanded view.

7. **Create `MobilePlayerBar`** (`src/components/MobilePlayerBar.tsx`)
   - **Mini state:** Fixed bottom bar above nav (cover thumbnail ~40px + truncated title/artist + play/pause button + progress bar)
   - **Tap to expand:** Full-screen overlay (`ExpandedPlayer`)
   - Consumes existing `usePlayer()` context — no new state needed

8. **Create `ExpandedPlayer`** (co-located in `MobilePlayerBar.tsx` or separate file)
   - Large cover art area
   - Big play/pause, prev/next, shuffle, repeat buttons
   - Full-width scrubber with time labels
   - Close button (chevron-down) returns to mini player
   - Optional: swipe down to dismiss

9. **Modify `App.tsx` player slot**
   - Desktop: render existing `<PlayerBar />`
   - Mobile: render `<MobilePlayerBar />`

---

### Phase 4 — Content Responsiveness (The Big One)
**Goal:** Make TrackList and key pages usable on small screens.

10. **Refactor `TrackList` for mobile** (`src/components/TrackList.tsx`)
    - **Desktop:** keep existing `<table>` layout unchanged
    - **Mobile:** render card list instead
      - Each card: cover art thumbnail (if available, else placeholder), Title + Artist stacked, visible Play button (no hover), overflow actions menu (tap to open)
      - Touch target minimum 44px for all interactive elements
      - Single-tap to play (not double-click)

11. **Audit and adjust pages**
    - `HomePage`: stats grid already uses `md:grid-cols-4`, works fine. Minor padding reduction on mobile.
    - `MusicPage`: filter input is fine. TrackList cards handle the rest.
    - `SearchPage`: search form stacks vertically or stays inline. TrackList cards handle results.
    - `RecsPage`: recommendation grid already has `grid-cols-1 md:grid-cols-2`. Ensure chat input is thumb-friendly. Playlist preview items become cards.
    - `DownloadsPage` / `SettingsPage` / `AnalyticsPage`: mostly form/content pages, reduce padding and ensure buttons are tall enough.
    - `PlaylistsPage`: list view, fine with minor padding tweaks.

---

### Phase 5 — Polish & PWA Primer
**Goal:** Native-app feel without building a full PWA yet.

12. **Mobile meta tags** in `index.html`
    - `<meta name="theme-color" content="#14171c" />`
    - `<meta name="apple-mobile-web-app-capable" content="yes" />`
    - `<meta name="apple-mobile-web-app-status-bar-style" content="black-translucent" />`

13. **Prevent iOS zoom on input focus**
    - Set font-size to 16px on all `<input>` and `<select>` elements (iOS zooms `<16px`)

14. **Remove hover-only dependencies**
    - TrackRow: play button hidden via `opacity-0 group-hover:opacity-100` must be always-visible on mobile
    - TrackRow: `MoreHorizontal` menu trigger must be always-visible on mobile
    - Any tooltip-only hints need mobile-visible alternatives

15. **Test checklist**
    - All 9 routes reachable via bottom nav + overflow sheet
    - Track can be played with single tap
    - Track actions (add to playlist, delete) accessible via touch
    - Player mini bar doesn't obscure content
    - Expanded player scrubs correctly and closes cleanly
    - No horizontal scroll on any page (except intentional swipes)

---

## 3. New File Inventory

| File | Purpose | Lines (est.) |
|---|---|---|
| `src/hooks/useIsMobile.ts` | Viewport detection hook | ~15 |
| `src/components/MobileNavBar.tsx` | Bottom tab bar + overflow sheet | ~120 |
| `src/components/MobilePlayerBar.tsx` | Mini player + Expanded overlay | ~200 |
| *(optional)* `src/components/TrackCard.tsx` | Single track card for mobile list | ~60 |

**Total new code:** ~400 lines, all presentational, zero new dependencies.

---

## 4. File Modification Inventory

| File | Changes |
|---|---|
| `frontend/index.html` | Add viewport, theme-color, apple-mobile-web-app meta tags |
| `src/index.css` | Add tap-highlight override, ensure 16px input font-size |
| `src/App.tsx` | Conditional layout: sidebar vs bottom nav; conditional player bar |
| `src/components/TrackList.tsx` | Branch: table (desktop) vs card list (mobile) |
| `src/components/PlayerBar.tsx` | (Optional) Minor responsive tweaks if not fully replaced |
| `src/pages/RecsPage.tsx` | Visible action buttons on mobile, playlist card layout |
| `src/pages/*.tsx` | Minor padding/font-size audits for touch screens |

---

## 5. Design Decisions & Rationales

### Why not a separate mobile route tree?
Creating `/m/` routes or `MobileMusicPage` duplicates business logic. The existing pages work; they just need layout adjustments. Conditional rendering inside shared components prevents drift.

### Why not a Context for mobile state?
`useIsMobile` is derived from window state, not application state. Only 3–4 top-level components need it. A Context adds boilerplate with no benefit over direct hook usage.

### Why 768px breakpoint?
- Aligns with Tailwind's `md:` prefix, already used in `HomePage` and `RecsPage`
- iPad portrait (768px) gets desktop layout, which is correct
- Phones (generally <430px) get mobile layout with comfortable margins

### Why bottom tabs instead of a hamburger drawer?
Hamburger drawers hide navigation behind an extra tap. Bottom tabs are the standard for media apps (Spotify, Apple Music, YouTube Music) because the primary actions are one tap away. The "More" overflow sheet handles secondary routes without cluttering the bar.

### Why cards instead of a responsive table?
Tables on mobile require horizontal scrolling, which breaks vertical rhythm and hides columns. Card layouts are the industry standard for list data on phones. The `TrackList` component can branch internally without creating a separate `MobileTrackList`.

### Why mini + expanded player instead of a compact persistent bar?
A persistent desktop-style bar on mobile leaves ~80px of usable screen. A mini player (~50px) preserves space, and the expanded view gives full controls when needed. This mirrors every major music streaming app.

---

## 6. Risk Mitigation

| Risk | Mitigation |
|---|---|
| Desktop layout breaks | All mobile changes are `isMobile`-gated; desktop code paths untouched |
| Hydration mismatch | Not applicable — SPA with no SSR |
| Rotation doesn't update | `matchMedia` listener fires on orientation change automatically |
| Touch targets too small | Audit all interactive elements for `min-h-[44px]` / `min-w-[44px]` |
| Player state out of sync | Mobile player consumes `PlayerContext`; no new state introduced |
| iOS Safari quirks | Test input zoom (16px font), safe-area-inset for notch devices |

---

## 7. Success Criteria

- [ ] App is fully navigable on a 375px-wide viewport without horizontal scrolling
- [ ] Any track can be played with a single tap
- [ ] Any track action (add to playlist, delete) is accessible via touch
- [ ] Player is controllable (play/pause, skip, scrub) without zooming or pinching
- [ ] Desktop layout is pixel-identical to pre-mobile state
- [ ] No new runtime dependencies added

---

## 8. Out of Scope (Future Work)

- Service worker / offline support
- Native app wrapper (Capacitor / React Native)
- Swipe gestures on queue (next/prev track via swipe)
- Pull-to-refresh on lists
- Push notifications
- Home screen icon / full PWA manifest

These are valuable but represent a separate project. This plan focuses strictly on the mobile-optimized *view*.
