/**
 * Spotify Web Playback SDK loader + thin wrapper.
 * Exposes a singleton player keyed on the user's connected Spotify account.
 * Premium-only — non-Premium accounts will get a 403 from `play`.
 */

import { api } from "./api";

declare global {
  interface Window {
    Spotify?: any;
    onSpotifyWebPlaybackSDKReady?: () => void;
  }
}

let sdkLoadPromise: Promise<void> | null = null;

function loadSDK(): Promise<void> {
  if (sdkLoadPromise) return sdkLoadPromise;
  sdkLoadPromise = new Promise((resolve, reject) => {
    if (window.Spotify) return resolve();
    const s = document.createElement("script");
    s.src = "https://sdk.scdn.co/spotify-player.js";
    s.async = true;
    s.onerror = () => reject(new Error("Failed to load Spotify SDK"));
    window.onSpotifyWebPlaybackSDKReady = () => resolve();
    document.head.appendChild(s);
  });
  return sdkLoadPromise;
}

interface PlayerHandle {
  player: any;
  deviceId: string;
}

let playerPromise: Promise<PlayerHandle> | null = null;
let cachedAccess: { token: string; exp: number } | null = null;

async function getAccessToken(): Promise<string> {
  if (cachedAccess && cachedAccess.exp > Date.now()) return cachedAccess.token;
  const { access_token } = await api.spotifyToken();
  // Spotify tokens last 1h; refresh after 50 min
  cachedAccess = { token: access_token, exp: Date.now() + 50 * 60 * 1000 };
  return access_token;
}

export async function getSpotifyPlayer(): Promise<PlayerHandle> {
  if (playerPromise) return playerPromise;
  playerPromise = (async () => {
    await loadSDK();
    const player = new window.Spotify.Player({
      name: "Lexicon",
      getOAuthToken: (cb: (t: string) => void) => {
        getAccessToken().then(cb).catch(() => cb(""));
      },
      volume: 0.9,
    });

    const deviceId: string = await new Promise((resolve, reject) => {
      const onReady = ({ device_id }: { device_id: string }) => resolve(device_id);
      const onErr = (e: any) =>
        reject(new Error(e?.message || "Spotify player init failed"));
      player.addListener("ready", onReady);
      player.addListener("initialization_error", onErr);
      player.addListener("authentication_error", onErr);
      player.addListener("account_error", onErr);
      player.connect();
    });

    return { player, deviceId };
  })();
  return playerPromise;
}

export async function spotifyPlayURI(uri: string): Promise<void> {
  const { deviceId } = await getSpotifyPlayer();
  const token = await getAccessToken();
  const r = await fetch(
    `https://api.spotify.com/v1/me/player/play?device_id=${deviceId}`,
    {
      method: "PUT",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ uris: [uri] }),
    }
  );
  if (!r.ok) {
    const msg = await r.text();
    throw new Error(`Spotify play failed: ${r.status} ${msg}`);
  }
}

export async function spotifyToggle() {
  const { player } = await getSpotifyPlayer();
  await player.togglePlay();
}

export async function spotifyPause() {
  const { player } = await getSpotifyPlayer();
  await player.pause();
}

export async function spotifySeek(ms: number) {
  const { player } = await getSpotifyPlayer();
  await player.seek(ms);
}

export async function spotifySetVolume(v: number) {
  const { player } = await getSpotifyPlayer();
  await player.setVolume(v);
}

export function spotifyURIFromTrack(t: { spotify_id?: string | null }): string | null {
  return t.spotify_id ? `spotify:track:${t.spotify_id}` : null;
}
