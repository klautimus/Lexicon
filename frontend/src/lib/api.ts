const API = "/api";

async function j<T>(path: string, init?: RequestInit): Promise<T> {
  let r: Response;
  try {
    r = await fetch(API + path, {
      headers: {
        "Content-Type": "application/json",
        "ngrok-skip-browser-warning": "1",
        ...(init?.headers || {}),
      },
      ...init,
    });
  } catch (e: any) {
    // Network error: offline, DNS failure, timeout, CORS block, etc.
    if (e?.name === "AbortError") {
      throw new Error("Request was cancelled.");
    }
    const msg = e?.message || "";
    if (msg.includes("Failed to fetch") || msg.includes("NetworkError") || msg.includes("network")) {
      throw new Error("Unable to reach the server. Check your connection and try again.");
    }
    throw new Error(`Network error: ${msg || "connection failed"}`);
  }
  if (!r.ok) {
    const text = await r.text();
    if (text.includes("ngrok")) {
      throw new Error(
        "Request blocked by ngrok free tier. Please refresh the page and click 'Visit Site' to continue."
      );
    }
    throw new Error(`${r.status} ${text}`);
  }
  if (r.status === 204) {
    return undefined as unknown as T;
  }
  const contentType = r.headers.get("content-type") || "";
  if (contentType.includes("text/html")) {
    const text = await r.text();
    if (text.includes("ngrok")) {
      throw new Error(
        "Request blocked by ngrok free tier. Please refresh the page and click 'Visit Site' to continue."
      );
    }
    throw new Error(`Expected JSON but got HTML (HTTP ${r.status})`);
  }
  return r.json();
}

export const api = {
  health: () => j<{ ok: boolean }>("/health"),
  scan: () => j<{ started: boolean }>("/scan", { method: "POST" }),
  stats: () => j<Stats>("/library/stats"),
  tracks: (kind?: string, limit = 200, offset = 0) =>
    j<TrackListResponse>(`/library/tracks?limit=${limit}&offset=${offset}${kind ? `&kind=${kind}` : ""}`),
  albums: () => j<Album[]>("/library/albums"),
  artists: () => j<Artist[]>("/library/artists"),
  podcasts: () => j<Podcast[]>("/library/podcasts"),
  search: (q: string) =>
    j<Track[]>(`/library/search?q=${encodeURIComponent(q)}`),
  track: (id: number) => j<Track>(`/library/track/${id}`),
  streamUrl: (id: number) => `${API}/stream/${id}`,
  coverUrl: (id: number) => `${API}/library/cover/${id}`,
  recordPlay: (data: PlayRecord) =>
    j<{ ok: boolean }>("/history/play", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  recent: () => j<RecentPlay[]>("/history/recent"),
  overview: () => j<Overview>("/analytics/overview"),
  topArtists: () => j<TopArtist[]>("/analytics/top-artists"),
  topTracks: () => j<TopTrack[]>("/analytics/top-tracks"),
  topGenres: () => j<TopGenre[]>("/analytics/top-genres"),
  heatmap: () => j<HeatCell[]>("/analytics/heatmap"),
  recs: () =>
    j<{ empty?: boolean; created_at?: number; data?: RecsPayload }>(
      "/recommendations"
    ),
  refreshRecs: () =>
    j<RecsPayload>("/recommendations/refresh", { method: "POST" }),
  chat: (message: string) =>
    j<{ reply: string; playlist?: PlaylistPayload }>("/recommendations/chat", {
      method: "POST",
      body: JSON.stringify({ message }),
    }),
  generatePlaylist: (force?: boolean, count?: number) => {
    let url = '/recommendations/playlist';
    const params = new URLSearchParams();
    if (force) params.set('force', 'true');
    if (count) params.set('count', count.toString());
    if (params.toString()) url += '?' + params.toString();
    return j<PlaylistPayload>(url, { method: 'POST' });
  },
  spotifyStatus: () => j<SpotifyStatus>("/spotify/status"),
  spotifyAuthURL: () => "/api/spotify/auth-url",
  spotifyDisconnect: () =>
    j<{ ok: boolean }>("/spotify/disconnect", { method: "POST" }),
  spotifySync: () =>
    j<{ started: boolean }>("/spotify/sync", { method: "POST" }),
  spotifyToken: () => j<{ access_token: string }>("/spotify/token"),

  // Apple Music
  appleStatus: () => j<AppleStatus>("/apple/status"),
  appleSaveConfig: (body: {
    team_id: string;
    key_id: string;
    private_key: string;
    storefront: string;
  }) =>
    j<{ ok: boolean; developer_token: string }>("/apple/config", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  appleDeleteConfig: () =>
    j<{ ok: boolean }>("/apple/config", { method: "DELETE" }),
  appleMusicKitConfig: () =>
    j<AppleMusicKitConfig>("/apple/musickit-config"),
  appleConnect: (musicUserToken: string) =>
    j<{ ok: boolean; storefront: string }>("/apple/connect", {
      method: "POST",
      body: JSON.stringify({ music_user_token: musicUserToken }),
    }),
  appleDisconnect: () =>
    j<{ ok: boolean }>("/apple/disconnect", { method: "POST" }),
  appleSync: () =>
    j<{ started: boolean }>("/apple/sync", { method: "POST" }),

  downloadStatus: () => j<DownloadStatus>("/download/status"),
  download: (url: string) =>
    j<DownloadJob>("/download", {
      method: "POST",
      body: JSON.stringify({ url }),
    }),
  downloadSearch: (query: string) =>
    j<DownloadJob>("/download/search", {
      method: "POST",
      body: JSON.stringify({ query }),
    }),
  downloadJobs: () => j<DownloadJob[]>("/download/jobs"),
  downloadJob: (id: string) => j<DownloadJob>(`/download/jobs/${id}`),
  downloadCancel: (id: string) =>
    j<{ ok: boolean }>(`/download/jobs/${id}/cancel`, { method: "POST" }),
  playlists: () => j<Playlist[]>('/playlists'),
  createPlaylist: (name: string) =>
    j<Playlist>('/playlists', { method: 'POST', body: JSON.stringify({ name }) }),
  playlist: (id: number) => j<PlaylistWithTracks>(`/playlists/${id}`),
  updatePlaylist: (id: number, name: string) =>
    j<{ ok: boolean }>(`/playlists/${id}`, { method: 'PUT', body: JSON.stringify({ name }) }),
  deletePlaylist: (id: number) =>
    j<void>(`/playlists/${id}`, { method: 'DELETE' }),
  addToPlaylist: (playlistId: number, trackId: number) =>
    j<{ ok: boolean }>(`/playlists/${playlistId}/tracks`, { method: 'POST', body: JSON.stringify({ track_id: trackId }) }),
  removeFromPlaylist: (playlistId: number, position: number) =>
    j<void>(`/playlists/${playlistId}/tracks/${position}`, { method: 'DELETE' }),
  deleteTrack: (trackId: number) =>
    j<void>(`/library/track/${trackId}`, { method: 'DELETE' }),

  // Track upgrade (re-download with new bestaudio/opus pipeline)
  upgradeTrack: (trackId: number) =>
    j<{ job_id: string; query: string; status: string; message: string }>(`/library/upgrade`, {
      method: 'POST',
      body: JSON.stringify({ track_id: trackId }),
    }),

  // Podcast feeds
  podcastFeeds: () => j<PodcastFeed[]>('/podcasts/feeds'),
  podcastSubscribe: (url: string) =>
    j<PodcastFeed>('/podcasts/subscribe', { method: 'POST', body: JSON.stringify({ url }) }),
  podcastUnsubscribe: (id: number) =>
    j<void>(`/podcasts/feeds/${id}`, { method: 'DELETE' }),
  podcastEpisodes: (feedId: number) =>
    j<PodcastEpisode[]>(`/podcasts/feeds/${feedId}/episodes`),
  podcastSync: (feedId: number) =>
    j<{ ok: boolean }>(`/podcasts/feeds/${feedId}/sync`, { method: 'POST' }),
  podcastDownloadEpisode: (episodeId: number) =>
    j<{ ok: boolean }>(`/podcasts/episodes/${episodeId}/download`, { method: 'POST' }),
  podcastDownloadFeed: (feedId: number) =>
    j<{ ok: boolean }>(`/podcasts/feeds/${feedId}/download`, { method: 'POST' }),
  podcastEpisodeTrack: (episodeId: number) =>
    j<{ track_id: number }>(`/podcasts/episodes/${episodeId}/track`),
  podcastEpisodePosition: (episodeId: number) =>
    j<{ position_sec: number; listened: boolean }>(`/podcasts/episodes/${episodeId}/position`),
  savePodcastEpisodePosition: (episodeId: number, positionSec: number, completed: boolean) =>
    j<{ ok: boolean }>(`/podcasts/episodes/${episodeId}/position`, {
      method: 'POST',
      body: JSON.stringify({ position_sec: positionSec, completed }),
    }),
  podcastStatus: () => j<{ available: boolean; bin?: string }>('/podcasts/status'),

  // Spotify devices
  spotifyDevices: () => j<SpotifyDevice[]>('/spotify/devices'),
  spotifyTransfer: (deviceId: string, play: boolean) =>
    j<{ ok: boolean }>('/spotify/transfer', { method: 'POST', body: JSON.stringify({ device_id: deviceId, play }) }),
};

export interface DownloadStatus {
  configured: boolean;
  bin?: string;
  output?: string;
  fallback_enabled: boolean;
  spotdl_bin?: string;
  spotdl_format?: string;
}

export interface DownloadJob {
  id: string;
  url: string;
  output: string;
  status: "queued" | "running" | "succeeded" | "failed" | "cancelled";
  started_at: number;
  finished_at?: number;
  error?: string;
  tool?: string;
  used_fallback?: boolean;
  track_id?: number;
  kind?: string; // "music" (default) or "podcast"
  log?: string[];
}

export interface Track {
  id: number;
  title: string;
  artist: string;
  album_artist: string;
  album: string;
  track_no: number;
  disc_no: number;
  year: number;
  genre: string;
  duration_sec: number;
  media_kind: string;
  mime: string;
  spotify_id?: string | null;
  external_url?: string | null;
  position?: number;
  loudness_integrated?: number | null;
  loudness_true_peak?: number | null;
  loudness_range?: number | null;
}

export interface TrackListResponse {
  tracks: Track[];
  total: number;
}

export interface SpotifyStatus {
  configured: boolean;
  connected: boolean;
  display_name?: string;
  product?: string;
  user_id?: string;
  last_synced_at?: number;
  has_playback_sdk: boolean;
}

export interface AppleStatus {
  configured: boolean;
  connected: boolean;
  team_id?: string;
  key_id?: string;
  storefront?: string;
  display_name?: string;
  last_synced_at?: number;
  dev_token_expires_at?: number;
}

export interface AppleMusicKitConfig {
  developer_token: string;
  app_name: string;
  storefront: string;
}
export interface Album {
  album: string;
  artist: string;
  year: number;
  tracks: number;
}
export interface Artist {
  artist: string;
  tracks: number;
  albums: number;
}
export interface Podcast {
  show: string;
  episodes: number;
}
export interface Stats {
  tracks: number;
  albums: number;
  artists: number;
  podcasts: number;
}
export interface PlayRecord {
  track_id: number;
  duration_played_sec: number;
  completed: boolean;
  source?: string;
  started_at?: number;
}
export interface RecentPlay {
  id: number;
  track_id: number;
  title: string;
  artist: string;
  album: string;
  started_at: number;
  duration_played_sec: number;
  completed: boolean;
  source: string;
}
export interface Overview {
  total_plays: number;
  unique_tracks: number;
  listen_sec: number;
  completed_pct: number;
}
export interface TopArtist {
  artist: string;
  plays: number;
  listen_sec: number;
}
export interface TopTrack {
  id: number;
  title: string;
  artist: string;
  plays: number;
}
export interface TopGenre {
  genre: string;
  plays: number;
}
export interface HeatCell {
  dow: number;
  hour: number;
  plays: number;
}
export interface RecItem {
  title: string;
  artist: string;
  reason: string;
  type: string;
  track_id?: number | null;
}
export interface RecsPayload {
  summary: string;
  trends: string;
  items: RecItem[];
}

export interface PlaylistTrack {
  title: string;
  artist: string;
  reason: string;
}

export interface PlaylistPayload {
  name: string;
  description: string;
  tracks: PlaylistTrack[];
}

export interface Playlist {
  id: number;
  name: string;
  track_count: number;
  total_duration: number;
  created_at: number;
}

export interface PlaylistWithTracks extends Playlist {
  tracks: Track[];
}

export interface PodcastFeed {
  id: number;
  url: string;
  title: string;
  description: string;
  image_url: string;
  author: string;
  episode_count: number;
  downloaded_count: number;
  last_fetched_at: number;
  auto_download: boolean;
}

export interface PodcastEpisode {
  id: number;
  feed_id: number;
  guid: string;
  title: string;
  description: string;
  pub_date: number;
  duration_sec: number;
  audio_url: string;
  downloaded: boolean;
  file_path: string;
  download_error: string;
  playback_position_sec: number;
  listened: boolean;
}

export interface SpotifyDevice {
  id: string;
  name: string;
  type: string;
  is_active: boolean;
  is_restricted: boolean;
  volume_percent: number;
}
