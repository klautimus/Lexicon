# Fix Plan: Podcast Multi-User Subscription — Complete Implementation

**Date:** 2026-05-23
**Status:** IMPLEMENTED

---

## Root Cause

The `podcast_feeds` table had a UNIQUE constraint on `url` and a `user_id` column. When user 1 subscribed to a feed, the row was created with `user_id=1`. When user 2 tried to subscribe to the same URL, `INSERT OR IGNORE` silently skipped (URL already existed). The `listFeeds` query filtered by `f.user_id IS NULL OR f.user_id=?`, so user 2 couldn't see the feed.

---

## Solution: Separate Shared Feed Data from Per-User Subscriptions

### New Schema

```
podcast_feeds: id, url(UNIQUE), title, description, image_url, author, link, language, last_fetched_at, last_error, download_folder, created_at
  — Shared feed data, NO user_id. One row per unique RSS URL.

podcast_subscriptions: id, user_id, feed_id, auto_download, created_at
  — Per-user subscription. UNIQUE(user_id, feed_id).

podcast_episodes: id, feed_id, guid, title, description, pub_date, duration_sec, audio_url, audio_type, audio_size, file_path, file_size, download_error, created_at
  — Shared episode metadata, NO user_id, NO downloaded.

podcast_episode_status: id, user_id, episode_id, downloaded, playback_position_sec, listened, created_at
  — Per-user download/playback state. UNIQUE(user_id, episode_id).
```

### Migration Changes (db.go)

1. Updated `schema` constant:
   - `podcast_feeds`: removed `user_id` and `auto_download` columns
   - Added `podcast_subscriptions` table
   - `podcast_episodes`: removed `downloaded` column
   - Added `podcast_episode_status` table

2. Added migration block:
   - Creates `podcast_subscriptions` and `podcast_episode_status` tables
   - Migrates existing feed subscriptions: `INSERT INTO podcast_subscriptions SELECT user_id, id, auto_download FROM podcast_feeds WHERE user_id IS NULL`
   - Migrates existing episode downloads: `INSERT INTO podcast_episode_status SELECT f.user_id, e.id, e.downloaded FROM podcast_episodes e JOIN podcast_feeds f ON f.id = e.feed_id WHERE e.downloaded=1 AND f.user_id IS NOT NULL`
   - Recreates `podcast_feeds` without `user_id` and `auto_download`
   - Recreates `podcast_episodes` without `downloaded`

3. Removed old migrations:
   - Removed `ALTER TABLE podcast_feeds ADD COLUMN user_id` (line 438-441)
   - Removed `ALTER TABLE podcast_feeds ADD COLUMN auto_download` (line 443-446)
   - Removed `ALTER TABLE podcast_episodes ADD COLUMN playback_position_sec` (line 377-380)
   - Removed `ALTER TABLE podcast_episodes ADD COLUMN listened` (line 382-385)
   - Removed `CREATE INDEX idx_podcast_feeds_user` (line 629-631)
   - Removed `"podcast_feeds"` from the `user_id` assignment loop (line 601)

4. Fixed `columnExists` regex: `^[a-z_]+$` → `^[a-z0-9_]+$` to allow digits in column names (e.g., `file_sha256`)

5. Moved `file_sha256` column addition before the dedup migration to ensure matching column counts during table recreation.

### Code Changes (podcaster.go)

Complete rewrite of all handler functions:

- `subscribe()`: Insert shared feed (no user_id), then insert per-user subscription
- `unsubscribe()`: Delete from subscriptions; if no more subscribers, clean up feed and episodes
- `listFeeds()`: JOIN with subscriptions, use episode_status for per-user download counts
- `listEpisodes()`: LEFT JOIN with episode_status for per-user download state
- `updateFeed()`: Update subscriptions.auto_download instead of podcast_feeds.auto_download
- `doDownloadEpisode()`: Use episode_status for per-user download tracking
- `doDownloadFeed()`: Use episode_status for per-user download tracking
- `checkEpisodeDedup()`: Use episode_status for cross-user dedup
- `ensureEpisodeStatus()`: New helper for per-user episode status
- `saveEpisodePosition()`: Use episode_status table
- `getEpisodePosition()`: Use episode_status table
- `episodeTrack()`: Unchanged (already per-user via tracks table)
- `recordEpisodeError()`: Removed downloaded=1 update (now in episode_status)

### Files Changed

1. `backend/internal/db/db.go` — Schema + migration
2. `backend/internal/podcaster/podcaster.go` — Complete rewrite
3. `backend/internal/scanner/scanner.go` — ffmpeg path from config + ffmpegAvailable check

---

## Verification

- Fresh install migration: ✅ All tables created with correct schema
- Build: ✅ Go + TypeScript compile clean
- Per-user subscriptions: Each user can independently subscribe/unsubscribe to the same feed
- Shared feed data: Feed metadata and episodes are shared across users
- Per-user download state: Each user has independent download/playback status
