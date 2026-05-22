/**
 * Centralized help content for all Lexicon features.
 * Each key maps to a { title, content } pair displayed in the HelpModal.
 * Keys are organized by page: "page.feature"
 */

export interface HelpEntry {
  title: string;
  content: string;
}

export const helpContent: Record<string, HelpEntry> = {
  // ── Navigation ──────────────────────────────────────────────
  "nav.rescan": {
    title: "Rescan Library",
    content: `**Rescan Library** tells Lexicon to scan your media folders for new or changed files.

**When to use it:**
• You've added music files directly to your media folders
• You've removed or moved files
• The library doesn't match what's on disk

**What it does:**
• Walks through all configured media folders
• Reads audio metadata (title, artist, album, etc.)
• Measures loudness for volume normalization
• Updates the database with any changes

**Note:** Downloads and podcast syncs trigger an automatic rescan, so you usually don't need to do this manually.`,
  },

  // ── Home ────────────────────────────────────────────────────
  "home.stats": {
    title: "Library Statistics",
    content: `These cards show the current state of your music library.

**Tracks** — Total number of audio files indexed by Lexicon.

**Albums** — Distinct albums found across all tracks.

**Artists** — Unique artists in your collection.

**Podcasts** — Number of podcast episodes that have been downloaded.

**Tip:** Connect Spotify or Apple Music and sync your history to enrich these stats with your streaming data.`,
  },
  "home.recent": {
    title: "Recently Played",
    content: `This list shows the last 10 tracks you've played in Lexicon.

**How it works:**
• Every track you play is recorded with a timestamp
• The list is sorted by most recent first
• Data comes from both local playback and Spotify/Apple Music sync

**Why it matters:**
Lexicon uses your listening history to generate personalized recommendations. The more you play, the better the suggestions get!`,
  },
  "home.qr": {
    title: "LAN Connection",
    content: `**Connected via LAN** means you're accessing Lexicon from another device on your local network (not the server itself).

**QR Code:** Scan this with a phone or tablet to open Lexicon on that device.

**Copy URL:** Copies the server address to your clipboard so you can paste it in another browser.

**Connection Help:** Shows network diagnostics to troubleshoot if other devices can't connect.

**Requirements:**
• Both devices must be on the same WiFi/network
• The server's firewall must allow connections on port 8787`,
  },

  // ── Music ───────────────────────────────────────────────────
  "music.library": {
    title: "Music Library",
    content: `Your complete music collection, scanned from your media folders.

**Features:**
• **Search** — Filter by title, artist, or album name
• **Sort** — Click column headers to sort by title, artist, album, etc.
• **Play** — Click any track to start playback
• **Add to Playlist** • Click the "•••" button on any row to add it to a playlist
• **Download from Web** • If a track isn't in your library, Lexicon can search for and download it

**Pagination:** Use "Load More" at the bottom to see additional tracks.`,
  },
  "music.download": {
    title: "Download from Web",
    content: `When Lexicon can't find a track in your local library, it can search the web and download it for you.

**How it works:**
1. Type the song name in the search box
2. Lexicon uses DeepSeek AI to parse your query
3. yt-dlp searches YouTube for the best audio match
4. The file is downloaded to your media folder
5. Your library is automatically rescanned

**Tips:**
• Be specific: "Artist - Title" works best
• You can also paste Spotify URLs on the Downloads page
• Downloaded files appear in your library automatically`,
  },
  "music.playlist-add": {
    title: "Add to Playlist",
    content: `Add any track to a playlist by clicking the **•••** button on the right side of each row.

**Options:**
• **Existing playlists** — Click a playlist name to add the track
• **Create new playlist** — Type a name and press Enter to create a new playlist with this track

**Tip:** You can also create and manage playlists from the Playlists page.`,
  },

  // ── Discover ────────────────────────────────────────────────
  "discover.generate": {
    title: "Generate AI Playlist",
    content: `**Generate Playlist** asks DeepSeek AI to create a personalized playlist based on your music taste.

**What it analyzes:**
• Your listening history (most-played artists, tracks, genres)
• Your library composition
• Spotify/Apple Music profile data (if connected)

**What you get:**
• A curated list of tracks with explanations for each pick
• A mix of tracks from your library and new discoveries
• The ability to download missing tracks and create a playlist in one click

**Tip:** Connect Spotify or Apple Music for better recommendations based on your full listening history.`,
  },
  "discover.track-count": {
    title: "Track Count",
    content: `Use the slider to choose how many tracks the AI should include in your generated playlist.

**Range:** 5–50 tracks

**Tip:** Start with 15–25 for a good listening session. You can always generate a new playlist with a different count.`,
  },
  "discover.recommendations": {
    title: "Recommendations",
    content: `Each card shows a track that DeepSeek thinks you'll enjoy.

**Badges:**
• **"From your library"** — You already have this track
• **"Discover"** — A new track you might like

**Actions:**
• **Play** — If the track is in your library
• **Download** — If it's a new track, download it from the web
• **Downloaded** ✓ — Successfully downloaded, ready to play

**Why this track?** Each recommendation includes a reason explaining why it was picked for you.`,
  },
  "discover.create-playlist": {
    title: "Create Playlist",
    content: `**Create Playlist** takes the AI-generated recommendations and:

1. Creates a new playlist with the suggested name
2. Adds any tracks you already have in your library
3. Downloads any missing tracks from the web
4. Adds downloaded tracks to the playlist

**Status indicators:**
• **Pending** — Waiting to be processed
• **Present** — Already in your library, added to playlist
• **Downloading** — Currently downloading from the web
• **Completed** — Downloaded and added to playlist
• **Failed** — Could not find or download the track

**Tip:** You can watch the progress in real-time. Failed tracks can be retried individually from the recommendations list.`,
  },
  "discover.chat": {
    title: "Chat with Lexicon",
    content: `**Chat with Lexicon's AI** to discover music, create playlists, and download songs — all through natural conversation.

**What you can do:**
• **"Make me a playlist for a road trip"** — Generates a themed playlist
• **"I'm in the mood for 90s grunge"** — Gets recommendations by era/genre
• **"Download that song we talked about"** — Downloads specific tracks
• **"What do you think of my taste?"** — Gets an analysis of your listening habits
• **"Find me something like [artist]"** — Discovers similar music

**How it works:**
• Lexicon uses DeepSeek AI with knowledge of your listening history
• If Spotify or Apple Music is connected, it uses that data too
• When you ask for a playlist, it generates one and shows a preview
• You can then create the playlist and download all missing tracks

**Tips:**
• Be specific about mood, genre, or activity
• Ask follow-up questions to refine recommendations
• Say "download that" to grab any track from the chat
• The more you chat, the better Lexicon understands your taste`,
  },

  // ── Search ──────────────────────────────────────────────────
  "search.main": {
    title: "Search",
    content: `Search across your entire music library.

**What it searches:**
• Track titles
• Artist names
• Album names
• Genres

**Tips:**
• Search is case-insensitive
• Partial matches work: "beat" finds "Beatles"
• If no results are found, Lexicon offers to search the web and download the track`,
  },
  "search.download": {
    title: "Search & Download from Web",
    content: `When a search has no local results, Lexicon can find and download the track from the web.

**How it works:**
1. Type the song name (e.g., "Meat Loaf - Bat Out of Hell")
2. Lexicon uses DeepSeek to parse the query
3. yt-dlp searches YouTube for the best audio match
4. The file is downloaded and added to your library

**This means you can build your entire library just by searching!**`,
  },

  // ── Downloads ───────────────────────────────────────────────
  "downloads.mode": {
    title: "Download Mode",
    content: `Choose how you want to download music:

**Spotify URL Mode:**
• Paste a Spotify track, album, or playlist URL
• Lexicon uses SpotiFLAC to download the highest quality audio
• Requires a Spotify account for some content

**Search by Name Mode:**
• Type any song or artist name
• Lexicon uses DeepSeek + yt-dlp to find and download from YouTube
• No Spotify account needed — completely free!

**Tip:** Search mode works for any song, even obscure tracks. Just be specific with "Artist - Title".`,
  },
  "downloads.jobs": {
    title: "Download Jobs",
    content: `All your download requests appear here with real-time status.

**Status icons:**
• 🔄 **Running** — Download in progress
• ⏳ **Queued** — Waiting for a free download slot
• ✅ **Succeeded** — Download complete
• ❌ **Failed** — Something went wrong
• 🚫 **Cancelled** — You cancelled the download

**Expanding a job** shows the full log output from the download tool, which is useful for debugging failures.

**Cancel** stops a running or queued download.

**Concurrency:** Lexicon downloads 2 files at a time by default. Additional downloads are queued automatically.`,
  },

  // ── Playlists ───────────────────────────────────────────────
  "playlists.grid": {
    title: "Your Playlists",
    content: `All your playlists in one place.

**Each card shows:**
• Playlist name
• Number of track
• Total duration

**Actions:**
• Click a playlist to view and edit it
• Use "Create New Playlist" to start fresh

**AI-Generated Playlists:** Playlists created from the Discover page appear here too, with all downloaded tracks included.`,
  },
  "playlists.create": {
    title: "Create New Playlist",
    content: `Create a new, empty playlist.

1. Click "Create New Playlist"
2. Type a name
3. Press Enter or click the checkmark

**After creating:**
• You'll be taken to the playlist page
• Add tracks from the Music library using the "•••" button
• Or generate an AI playlist from the Discover page`,
  },
  "playlist.detail": {
    title: "Playlist Detail",
    content: `View and manage a single playlist.

**Features:**
• **Inline Rename** — Click the playlist name to edit it
• **Play All** — Start playing from the first track
• **Track List** — See all tracks with title, artist, album, and duration
• **Remove Tracks** — Click the "×" button to remove a track
• **Reorder** — Tracks play in the order shown (reordering coming soon)

**Tip:** AI-generated playlists include the reason each track was chosen.`,
  },

  // ── Podcasts ────────────────────────────────────────────────
  "podcasts.feeds": {
    title: "Podcast Feeds",
    content: `Manage your podcast subscriptions here.

**Subscribe:**
1. Click "Add Feed"
2. Paste the RSS feed URL
3. Lexicon will fetch the latest episodes

**Feed sidebar:**
• Shows all subscribed podcasts
• Click a feed to see its episodes
• Unsubscribe with the "×" button

**Tip:** You can find RSS feed URLs on most podcast websites or directories like Apple Podcasts.`,
  },
  "podcasts.episodes": {
    title: "Episodes",
    content: `Browse episodes for the selected podcast.

**Actions:**
• **Download** — Save the episode to your library
• **Play** — Stream the episode (if downloaded)
• **Sync** — Check for new episodes

**Downloaded episodes** appear in your Music library too, so you can add them to playlists.`,
  },

  // ── Analytics ───────────────────────────────────────────────
  "analytics.charts": {
    title: "Listening Analytics",
    content: `Visualize your music listening habits.

**Available charts:**
• **Top Artists** — Your most-played artists
• **Top Genres** — Your genre distribution
• **Listening Heatmap** — When you listen most (day of week × hour)
• **Top Tracks** — Your most-played tracks

**Data sources:**
• Local playback history
• Spotify sync (if connected)
• Apple Music sync (if connected)

**Tip:** The more you listen, the richer your analytics become!`,
  },

  // ── Settings ────────────────────────────────────────────────
  "settings.spotify": {
    title: "Spotify Integration",
    content: `Connect your Spotify account to unlock powerful features.

**What it enables:**
• **Listening history sync** — Import your Spotify plays into Lexicon
• **In-app playback** — Play Spotify tracks directly (Premium required)
• **Better recommendations** — AI uses your Spotify taste profile
• **Device control** — Control Spotify on other devices

**Setup:**
1. Go to developer.spotify.com/dashboard
2. Create an app named "Lexicon"
3. Add redirect URI: http://127.0.0.1:8787/api/spotify/callback
4. Copy the Client ID to backend/.env as SPOTIFY_CLIENT_ID
5. Restart the server and click "Connect Spotify"

**Privacy:** Your Spotify data stays on your local server. Nothing is sent to third parties.`,
  },
  "settings.apple": {
    title: "Apple Music Integration",
    content: `Connect Apple Music to enrich your library and recommendations.

**What it enables:**
• **Listening history sync** — Import your Apple Music plays
• **Better recommendations** — AI uses your full listening profile
• **Library enrichment** — Combines local + streaming data

**Setup:**
1. You need an Apple Developer account
2. Generate a MusicKit private key (.p8 file)
3. Enter your Team ID, Key ID, and .p8 contents in the settings form
4. Connect via MusicKit in your browser

**Note:** Apple Music playback within Lexicon is not yet supported (data sync only).`,
  },

  // ── Player ──────────────────────────────────────────────────
  "player.controls": {
    title: "Playback Controls",
    content: `Control your music playback.

**Buttons:**
• **Play/Pause** — Toggle playback
• **Previous/Next** — Skip tracks
• **Shuffle** — Randomize play order
• **Repeat** — Loop current track or playlist

**Progress bar:**
• Shows current position in the track
• Click or drag to seek to a different position

**Volume:**
• Click the speaker icon to mute/unmute
• Drag the slider to adjust volume`,
  },
  "player.device": {
    title: "Device Picker",
    content: `Choose where to play your music.

**Available devices:**
• **This Device** — Play through Lexicon's built-in player
• **Spotify Connect** — Play on any Spotify Connect device (Premium)
• **Other Lexicon instances** — Play on another device running Lexicon

**Tip:** Use Spotify Connect to play on speakers, TVs, or other devices on your network.`,
  },
};
