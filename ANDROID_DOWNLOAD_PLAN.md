# Lexicon Android Download Feature — Implementation Plan

> **Goal:** Enable song and playlist downloads directly to an Android device's local storage, using existing Lexicon infrastructure.
> **Status:** DRAFT — pending Kevin's review
> **Date:** 2026-05-14

---

## 1. Background Research Summary

### yt-dlp
- **What it is:** Command-line downloader for YouTube and 1000+ other sites. Fork of youtube-dl. 100k+ GitHub stars.
- **GitHub:** https://github.com/yt-dlp/yt-dlp
- **Status:** Already installed at `/usr/bin/yt-dlp` (v2024.04.09). Already used by Lexicon's downloader as tier 2 of the 3-tier fallback pipeline (SpotiFLAC → yt-dlp → spotDL).
- **In Lexicon:** Called with `ytsearch1:<query> --extract-audio --audio-format mp3 --add-metadata --embed-thumbnail`
- **No changes needed to yt-dlp itself** for this feature.

### Lexicon Project (Current State)
- **Location:** `/mnt/c/Users/kevin/CascadeProjects/lexicon/`
- **Backend:** Go + chi router + SQLite (FTS5) — single binary
- **Frontend:** React + Vite + TypeScript + TailwindCSS
- **Key existing infrastructure relevant to this feature:**
  - **Streamer** (`backend/internal/streamer/streamer.go`): Already serves audio files via `GET /api/stream/{id}` with proper Content-Type, Accept-Ranges, and `http.ServeContent`
  - **Library API** (`backend/internal/library/library.go`): Full track/album/artist CRUD with FTS5 search
  - **Playlists API** (`backend/internal/playlists/playlists.go`): Full playlist CRUD with track add/remove (215 lines)
  - **Downloader** (`backend/internal/downloader/downloader.go`): 3-tier pipeline for fetching songs from Spotify URLs/free-text (765 lines)
  - **Mobile plan** (`MOBILE_IMPLEMENTATION_PLAN.md`): Already scoped out mobile browser optimization
- **Access:** Exposed to Android phone via ngrok tunnel

### Key Insight
The **existing downloader downloads to the server's MEDIA_ROOTS** (Windows filesystem). For Android, we need to **export** those files through the browser with `Content-Disposition: attachment` so the Android browser saves them to the device's Downloads folder. This is a browser-native action — no app needed.

---

## 2. Feature Specification

### What We're Building
Add the ability to download tracks and playlists from the Lexicon web UI directly to an Android phone's local storage, using the phone's browser download mechanism.

### How It Works
1. User taps a "Download" button next to a track → browser downloads the file to Android's Downloads folder
2. User taps "Download All" on a playlist → browser downloads a ZIP of all playlist tracks
3. This uses Android's native `Content-Disposition: attachment` handling — no separate app needed
4. Files are streamed from the existing Lexicon server filesystem (where the downloader already put them)

### What This Is NOT
- NOT a change to the downloader pipeline (yt-dlp, SpotiFLAC, spotDL remain untouched)
- NOT a PWA/service worker offline feature (that's separate work)
- NOT a native Android app

---

## 3. Implementation Plan

### Phase 1 — Backend: Individual Track Download

**File:** `backend/internal/streamer/streamer.go` (modify existing)

Add a new route and handler method:

```
GET /api/stream/{id}/download
```

**Handler logic:**
1. Query track by ID from SQLite (same query as existing `stream` handler)
2. Open the file
3. Set headers:
   - `Content-Disposition: attachment; filename="Artist - Title.ext"`
   - `Content-Type: <MIME from DB>`
4. Use `http.ServeContent()` to stream the file (reuses existing pattern)

**Why add to streamer package:** The streamer already handles file serving with proper range request support. Adding download alongside streaming keeps related file-serving logic together.

**Estimated additions:** ~30 lines
- New route: `r.Get("/api/stream/{id}/download", s.download)`
- New method: `func (s *Streamer) download(...)`

---

### Phase 2 — Backend: Playlist Download (ZIP)

**File:** `backend/internal/playlists/playlists.go` (modify existing)

Add a new route and handler:

```
GET /api/playlists/{id}/download
```

**Handler logic:**
1. Fetch playlist tracks from SQLite (reuses existing `playlistWithTracks` query)
2. Create an in-memory ZIP archive using Go's `archive/zip` + `bytes.Buffer`
3. For each track:
   - Read file from disk
   - Add to ZIP with filename "Artist - Title.ext"
4. Set headers:
   - `Content-Disposition: attachment; filename="Playlist Name.zip"`
   - `Content-Type: application/zip`
5. Stream the ZIP buffer

**Stdlib only:** `archive/zip`, `io`, `bytes`, `os` — all in Go standard library. Zero new dependencies.

**Estimated additions:** ~60 lines
- New route: `r.Get("/api/playlists/{id}/download", s.downloadZip)`
- New method: `func (s *API) downloadZip(...)`

---

### Phase 3 — Backend: Route Mounting

**File:** `backend/cmd/server/main.go` (modify)

Add new routes to the existing mount structure:
- Streamer already mounts. If download routes are in streamer, they're auto-mounted.
- Playlist download route needs to be mounted if added to playlists API.

**Estimated changes:** ~3 lines

---

### Phase 4 — Frontend: API Client

**File:** `frontend/src/lib/api.ts` (modify)

Add helper functions that construct download URLs:

```typescript
// Return the URL for downloading a single track
export function downloadTrackUrl(id: number): string {
  return `/api/stream/${id}/download`;
}

// Return the URL for downloading a playlist as ZIP
export function downloadPlaylistUrl(id: number): string {
  return `/api/playlists/${id}/download`;
}
```

These return URLs, not fetch responses — the browser handles the download natively when the URL is used as an `<a>` tag `href` or `window.open()`.

**Estimated additions:** ~12 lines

---

### Phase 5 — Frontend: TrackRow Download Button

**File:** `frontend/src/components/TrackList.tsx` (modify)

Add a download button to each track row in the existing action buttons area:
- Use `Download` icon from lucide-react (already a dependency)
- Add a small download button with the same styling as the existing "..." menu button
- On click: programmatically trigger download via `<a>` element or `window.open()`

**Mobile consideration:** The download button should also be visible on mobile (not hover-dependent), matching the MOBILE_IMPLEMENTATION_PLAN's requirement for always-visible action buttons.

**Estimated additions:** ~15 lines

---

### Phase 6 — Frontend: Playlist Download All Button

**File:** `frontend/src/pages/PlaylistPage.tsx` (modify)

Add a "Download All" button to the playlist page header area, next to the existing "Play All" button:
- Label: "Download All" with a Download icon
- Downloads the entire playlist as a single ZIP file
- The same button should work on mobile (always visible, touch-friendly)

**Estimated additions:** ~10 lines

---

## 4. File Change Summary

| # | File | Change Type | Estimated Lines Added | Complexity |
|---|------|-------------|----------------------|------------|
| 1 | `backend/internal/streamer/streamer.go` | Modify (add route + handler) | ~30 | Low |
| 2 | `backend/internal/playlists/playlists.go` | Modify (add route + handler) | ~60 | Medium |
| 3 | `backend/cmd/server/main.go` | Modify (mount new routes) | ~3 | Low |
| 4 | `frontend/src/lib/api.ts` | Modify (add URL helpers) | ~12 | Low |
| 5 | `frontend/src/components/TrackList.tsx` | Modify (add download button) | ~15 | Low |
| 6 | `frontend/src/pages/PlaylistPage.tsx` | Modify (add Download All button) | ~10 | Low |

**Total new code:** ~130 lines (all in existing files, no new files)
**New dependencies:** Zero (all stdlib + existing dependencies)

---

## 5. Design Decisions

### Why `Content-Disposition: attachment` instead of built-in download manager?
Android browsers handle `Content-Disposition: attachment` natively — the file is saved to the Downloads folder automatically. This requires zero Android-side code and works with the existing ngrok tunnel.

### Why ZIP for playlists instead of individual downloads?
A playlist with 10-20 tracks would be tedious to download one by one. A single ZIP preserves the playlist structure, keeps download history clean, and matches user expectations from other music services.

### Why add to the streamer package instead of creating a new one?
The streamer already owns file-serving logic (`http.ServeContent`, MIME detection, range requests). Adding download alongside streaming keeps related concerns together and avoids creating a new package for ~30 lines of code.

### Why not add `?dl=1` parameter to the existing stream endpoint?
Could work, but a separate `/download` path is:
- More explicit for future caching/CDN considerations
- Easier to maintain and debug
- More REST-idiomatic (different resource action → different URL)

---

## 6. Potential Pitfalls

1. **Large ZIP files for big playlists** — A 50-track playlist could be 500MB+. In-memory ZIP creation will consume server RAM. Mitigation: Use `archive/zip` with `io.Pipe` to stream files through without buffering the entire archive in memory.
2. **Filename collisions** — Two tracks with "Artist - Title.mp3" names will collide in the ZIP. Mitigation: Include track number or album name in the ZIP path.
3. **Ngrok bandwidth limits** — Free ngrok has bandwidth caps. Large file downloads count against this. Not a code issue, but worth noting.
4. **Android browser download interruptions** — If the user switches apps mid-download, Android may kill the download. This is browser behavior, not something we can fix on the server side.

---

## 7. Success Criteria

- [ ] A single track can be downloaded from the Lexicon web UI to an Android device's Downloads folder
- [ ] A complete playlist can be downloaded as a single ZIP file
- [ ] Downloaded files have correct filenames (Artist - Title.mp3)
- [ ] ZIP file extracts correctly with proper filenames
- [ ] Download button is visible on both desktop and mobile (not hover-only)
- [ ] No new runtime dependencies added
- [ ] Existing streaming functionality is completely unaffected

---

## 8. Out of Scope (Future Work)

- **Progressive download with resume** — would require a native app
- **Background download manager** — Android ServiceWorker approach
- **Offline playback within Lexicon web UI** — would need service worker + cache API
- **PWA install prompt** — separate feature, covered in MOBILE_IMPLEMENTATION_PLAN Phase 5

---

*Plan created by Atlas for Kevin's review. No implementation starts until Kevin approves.*
