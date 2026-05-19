import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  useCallback,
  ReactNode,
} from "react";
import { api, Track } from "../lib/api";
import { useToast } from "../contexts/ToastContext";
import {
  spotifyPlayURI,
  spotifyToggle,
  spotifyPause,
  spotifySeek,
  spotifySetVolume,
  spotifyURIFromTrack,
  getSpotifyPlayer,
} from "../lib/spotify";
import { getPlayerWebSocket } from "../lib/playerws";

type Source = "local" | "spotify" | null;
type RepeatMode = "off" | "all" | "one";

interface PlayerState {
  current: Track | null;
  queue: Track[];
  index: number;
  playing: boolean;
  position: number;
  duration: number;
  volume: number;
  source: Source;
  error: string | null;
  shuffled: boolean;
  repeatMode: RepeatMode;
}

interface PlayerCtx extends PlayerState {
  play: (tracks: Track[], startIndex?: number) => void;
  toggle: () => void;
  next: () => void;
  prev: () => void;
  seek: (sec: number) => void;
  setVolume: (v: number) => void;
  toggleShuffle: () => void;
  toggleRepeat: () => void;
  setPodcastEpisodeId: (episodeId: number | null) => void;
}

const Ctx = createContext<PlayerCtx | null>(null);

export function PlayerProvider({ children }: { children: ReactNode }) {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const audioCtxRef = useRef<AudioContext | null>(null);
  const compressorRef = useRef<DynamicsCompressorNode | null>(null);
  const [state, setState] = useState<PlayerState>({
    current: null,
    queue: [],
    index: -1,
    playing: false,
    position: 0,
    duration: 0,
    volume: 0.9,
    source: null,
    error: null,
    shuffled: false,
    repeatMode: "off",
  });
  const playStartRef = useRef<number>(0);
  const playSecondsRef = useRef<number>(0);
  const sourceRef = useRef<Source>(null);
  const currentRef = useRef<Track | null>(null);
  const originalQueueRef = useRef<Track[]>([]);
  const shuffledRef = useRef<boolean>(false);
  const consecutiveErrorsRef = useRef<number>(0);
  const skipTimeoutRef = useRef<number | null>(null);
  const playSessionRef = useRef<number>(0);
  const podcastEpisodeIdRef = useRef<number | null>(null);
  const podcastSaveIntervalRef = useRef<number | null>(null);
  const toast = useToast();
  const wsRef = useRef<ReturnType<typeof getPlayerWebSocket> | null>(null);
  const isWsPlayerRef = useRef<boolean>(false);

  // ----- Helpers -----

  const clearSkipTimeout = useCallback(() => {
    if (skipTimeoutRef.current !== null) {
      clearTimeout(skipTimeoutRef.current);
      skipTimeoutRef.current = null;
    }
  }, []);

  const scheduleSkip = useCallback(() => {
    clearSkipTimeout();
    skipTimeoutRef.current = window.setTimeout(() => {
      skipTimeoutRef.current = null;
      goNext();
    }, 1500);
  }, [clearSkipTimeout]);

  // ----- WebSocket setup -----
  useEffect(() => {
    const ws = getPlayerWebSocket();
    wsRef.current = ws;
    isWsPlayerRef.current = ws.isPlayer();
    ws.connect();

    // If we're the player, listen for commands from controllers
    ws.onState((msg: any) => {
      if (!isWsPlayerRef.current) return;
      if (msg.type === "play" && msg.track_id) {
        // Find the track in the queue or fetch it
        api.track(msg.track_id).then((t) => {
          play([t], 0);
        }).catch(() => {});
      } else if (msg.type === "pause") {
        const a = audioRef.current;
        if (a) a.pause();
      } else if (msg.type === "resume") {
        const a = audioRef.current;
        if (a) a.play().catch(() => {});
      } else if (msg.type === "next") {
        next();
      } else if (msg.type === "prev") {
        prev();
      } else if (msg.type === "seek" && msg.position !== undefined) {
        seek(msg.position);
      } else if (msg.type === "transfer") {
        // Another device is taking over — pause local playback
        const a = audioRef.current;
        if (a) a.pause();
        isWsPlayerRef.current = false;
      }
    });

    return () => {
      ws.disconnect();
    };
  }, []);

  // Broadcast state changes to controllers when we're the player
  const broadcastState = useCallback(() => {
    if (!isWsPlayerRef.current || !wsRef.current) return;
    wsRef.current.send({
      type: "state",
      playing: state.playing,
      track: state.current,
      position: state.position,
      duration: state.duration,
      device: "host",
    });
  }, [state.playing, state.current, state.position, state.duration]);

  // ----- Local <audio> setup -----
  useEffect(() => {
    const a = new Audio();
    a.preload = "auto";
    a.crossOrigin = "anonymous";
    // playsInline / webkit-playsinline — needed for iOS Safari inline playback
    a.setAttribute("playsinline", "playsinline");
    a.setAttribute("webkit-playsinline", "webkit-playsinline");
    audioRef.current = a;
    a.volume = state.volume;
    const onTime = () =>
      setState((s) =>
        s.source === "local"
          ? { ...s, position: a.currentTime, duration: a.duration || 0 }
          : s
      );
    const onEnded = () => {
      flushLocalPlay(true);
      goNext();
    };
    const onPlay = () =>
      setState((s) => (s.source === "local" ? { ...s, playing: true } : s));
    const onPause = () =>
      setState((s) => (s.source === "local" ? { ...s, playing: false } : s));
    const onError = () => {
      const a = audioRef.current;
      const err = a?.error;
      const msg = err
        ? `Audio error (code ${err.code}): ${err.message || "unknown"}`
        : "Audio playback failed";
      console.error("[player]", msg);
      sourceRef.current = null;
      currentRef.current = null;
      playSecondsRef.current = 0;
      setState((s) => ({ ...s, source: null, error: msg, playing: false }));

      // Auto-skip to next track (with consecutive error limit)
      // Only handle if we're still in the same play session
      consecutiveErrorsRef.current++;
      if (consecutiveErrorsRef.current >= 5) {
        toast.error("Multiple tracks failed to play — stopping playback");
        consecutiveErrorsRef.current = 0;
        return;
      }
      toast.error("Playback failed — skipping to next track");
      scheduleSkip();
    };
    a.addEventListener("timeupdate", onTime);
    a.addEventListener("ended", onEnded);
    a.addEventListener("play", onPlay);
    a.addEventListener("pause", onPause);
    a.addEventListener("error", onError);
    return () => {
      a.pause();
      a.removeEventListener("timeupdate", onTime);
      a.removeEventListener("ended", onEnded);
      a.removeEventListener("play", onPlay);
      a.removeEventListener("pause", onPause);
      a.removeEventListener("error", onError);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ----- Web Audio API: Dynamics Compressor -----
  // Must be initialized after user gesture (browser autoplay policy)
  const initAudioPipeline = useCallback(() => {
    const a = audioRef.current;
    if (!a || audioCtxRef.current) return; // already initialized
    try {
      const ctx = new AudioContext();
      audioCtxRef.current = ctx;
      const source = ctx.createMediaElementSource(a);
      const compressor = ctx.createDynamicsCompressor();
      compressor.threshold.value = -24;
      compressor.knee.value = 30;
      compressor.ratio.value = 12;
      compressor.attack.value = 0.003;
      compressor.release.value = 0.25;
      compressorRef.current = compressor;
      source.connect(compressor);
      compressor.connect(ctx.destination);
    } catch (e) {
      console.warn("[player] Web Audio API init failed:", e);
    }
  }, []);

  function flushLocalPlay(completed: boolean) {
    const t = currentRef.current;
    if (!t || sourceRef.current !== "local") return;
    const dur = Math.max(0, Math.floor(playSecondsRef.current));
    if (dur < 5 && !completed) return;
    api
      .recordPlay({
        track_id: t.id,
        duration_played_sec: dur,
        completed,
        started_at: playStartRef.current,
      })
      .catch(() => {});
    playSecondsRef.current = 0;
  }

  // Broadcast state to controllers whenever it changes
  useEffect(() => {
    broadcastState();
  }, [state.playing, state.position, state.current, broadcastState]);

  // ----- Track local listening time -----
  useEffect(() => {
    const a = audioRef.current;
    if (!a) return;
    let last = a.currentTime;
    const id = setInterval(() => {
      if (sourceRef.current !== "local") {
        last = a.currentTime;
        return;
      }
      if (!a.paused) {
        const cur = a.currentTime;
        const delta = cur - last;
        if (delta > 0 && delta < 2) playSecondsRef.current += delta;
        last = cur;
      } else {
        last = a.currentTime;
      }
    }, 1000);
    return () => clearInterval(id);
  }, []);

  // ----- Spotify state poller (only active when source === 'spotify') -----
  useEffect(() => {
    let cancelled = false;
    const id = setInterval(async () => {
      if (sourceRef.current !== "spotify" || cancelled) return;
      try {
        const { player } = await getSpotifyPlayer();
        const st = await player.getCurrentState();
        if (!st) return;
        setState((s) => ({
          ...s,
          playing: !st.paused,
          position: st.position / 1000,
          duration: st.duration / 1000,
        }));
        if (st.paused && st.position === 0 && st.track_window?.current_track) {
          // possible track-end → advance
        }
      } catch {
        // ignore
      }
    }, 1000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);

  async function loadAndPlay(t: Track) {
    // Clear any pending skip timeout from a previous track
    clearSkipTimeout();
    // Increment play session so stale error handlers are ignored
    playSessionRef.current++;
    // Reset consecutive errors for the new track
    consecutiveErrorsRef.current = 0;

    flushLocalPlay(false);
    currentRef.current = t;
    setState((s) => ({ ...s, error: null }));

    const isSpotify = !!t.spotify_id;

    if (isSpotify) {
      // Stop local audio
      const a = audioRef.current;
      if (a) {
        a.pause();
        a.removeAttribute("src");
        a.load();
      }

      sourceRef.current = "spotify";
      setState((s) => ({ ...s, source: "spotify" }));

      const uri = spotifyURIFromTrack(t);
      if (!uri) {
        setState((s) => ({ ...s, error: "No Spotify URI" }));
        return;
      }
      try {
        await spotifyPlayURI(uri);
      } catch (e: any) {
        const msg = e?.message || "Spotify playback failed";
        setState((s) => ({
          ...s,
          error: msg.includes("403")
            ? "Spotify Premium required to play in-app."
            : msg,
        }));
      }
      return;
    }

    // Local file
    const a = audioRef.current!;
    // Pause and reset before changing source to prevent ghost playback
    a.pause();
    // Initialize Web Audio API compressor on first user interaction
    initAudioPipeline();
    a.src = api.streamUrl(t.id);
    a.play()
      .then(() => {
        sourceRef.current = "local";
        setState((s) => ({ ...s, source: "local", error: null }));
        playStartRef.current = Math.floor(Date.now() / 1000);
        playSecondsRef.current = 0;
        consecutiveErrorsRef.current = 0;
        // Volume normalization: adjust to -14 LUFS target
        if (t.loudness_integrated != null) {
          const targetLUFS = -14;
          const gainDB = targetLUFS - t.loudness_integrated;
          const gainLinear = Math.pow(10, gainDB / 20);
          a.volume = Math.min(1.0, state.volume * gainLinear);
        }
      })
      .catch((e: any) => {
        const msg = e?.message || "Audio playback failed";
        console.error("[player] play failed:", msg);
        sourceRef.current = null;
        currentRef.current = null;
        setState((s) => ({ ...s, source: null, error: msg, playing: false }));

        // Auto-skip — but only increment once per track (onError may also fire)
        consecutiveErrorsRef.current++;
        if (consecutiveErrorsRef.current >= 5) {
          toast.error("Multiple tracks failed to play — stopping playback");
          consecutiveErrorsRef.current = 0;
          return;
        }
        toast.error("Failed to play track — skipping to next");
        scheduleSkip();
      });
  }

  function shuffleArray<T>(arr: T[]): T[] {
    const a = [...arr];
    for (let i = a.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1));
      [a[i], a[j]] = [a[j], a[i]];
    }
    return a;
  }

  async function play(tracks: Track[], startIndex = 0) {
    if (tracks.length === 0) return;
    // Clear any pending auto-skip from previous track
    clearSkipTimeout();
    // Reset consecutive errors for manual play
    consecutiveErrorsRef.current = 0;
    // Clear podcast episode ID when playing non-podcast tracks
    // (the caller sets it explicitly for podcast episodes)
    if (podcastEpisodeIdRef.current !== null) {
      await savePodcastPosition();
      await setPodcastEpisodeId(null);
    }
    originalQueueRef.current = [...tracks];
    const chosen = tracks[startIndex];
    if (shuffledRef.current && tracks.length > 1) {
      const rest = tracks.filter((_, i) => i !== startIndex);
      const shuffled = shuffleArray(rest);
      const newQueue = [chosen, ...shuffled];
      setState((s) => ({
        ...s,
        queue: newQueue,
        index: 0,
        current: chosen,
        shuffled: true,
      }));
      loadAndPlay(chosen);
    } else {
      setState((s) => ({
        ...s,
        queue: tracks,
        index: startIndex,
        current: chosen,
        shuffled: false,
      }));
      loadAndPlay(chosen);
    }
  }

  function toggleShuffle() {
    setState((s) => {
      if (s.shuffled) {
        // Turn shuffle OFF: restore original order and find current track's index
        const restored = [...originalQueueRef.current];
        const idx = restored.findIndex((t) => t.id === s.current?.id);
        shuffledRef.current = false;
        return { ...s, queue: restored, index: Math.max(0, idx), shuffled: false };
      } else {
        // Turn shuffle ON: shuffle remaining tracks, keep current at index 0
        if (s.queue.length <= 1) {
          shuffledRef.current = true;
          return { ...s, shuffled: true };
        }
        const current = s.queue[s.index];
        const before = s.queue.slice(0, s.index);
        const after = s.queue.slice(s.index + 1);
        const rest = shuffleArray([...before, ...after]);
        shuffledRef.current = true;
        return { ...s, queue: [current, ...rest], index: 0, shuffled: true };
      }
    });
  }

  function toggleRepeat() {
    setState((s) => {
      const next: RepeatMode = s.repeatMode === "off" ? "all" : s.repeatMode === "all" ? "one" : "off";
      return { ...s, repeatMode: next };
    });
  }

  async function toggle() {
    if (sourceRef.current === "spotify") {
      try {
        await spotifyToggle();
      } catch {}
      return;
    }
    const a = audioRef.current!;
    if (a.paused) a.play();
    else a.pause();
  }

  function goNext() {
    // Clear any pending skip timeout to prevent double-skipping
    clearSkipTimeout();
    setState((s) => {
      // Repeat One: replay current track
      if (s.repeatMode === "one" && s.current) {
        loadAndPlay(s.current);
        return s;
      }

      const ni = s.index + 1;
      if (ni >= s.queue.length) {
        // Repeat All: wrap to beginning
        if (s.repeatMode === "all" && s.queue.length > 0) {
          const first = s.queue[0];
          // If shuffle is also on, reshuffle the queue
          if (s.shuffled && s.queue.length > 1) {
            const newQueue = shuffleArray(s.queue);
            loadAndPlay(newQueue[0]);
            return { ...s, queue: newQueue, index: 0, current: newQueue[0] };
          }
          loadAndPlay(first);
          return { ...s, index: 0, current: first };
        }
        // Repeat Off: stop at end
        if (sourceRef.current === "local") {
          const a = audioRef.current!;
          a.pause();
        } else if (sourceRef.current === "spotify") {
          spotifyPause().catch(() => {});
        }
        return { ...s, playing: false };
      }
      const nt = s.queue[ni];
      loadAndPlay(nt);
      return { ...s, index: ni, current: nt };
    });
  }

  function next() {
    flushLocalPlay(false);
    goNext();
  }

  function prev() {
    flushLocalPlay(false);
    setState((s) => {
      const ni = Math.max(0, s.index - 1);
      const nt = s.queue[ni];
      if (nt) loadAndPlay(nt);
      return { ...s, index: ni, current: nt };
    });
  }

  function seek(sec: number) {
    if (sourceRef.current === "spotify") {
      spotifySeek(sec * 1000).catch(() => {});
      return;
    }
    const a = audioRef.current!;
    a.currentTime = sec;
  }

  function setVolume(v: number) {
    const a = audioRef.current!;
    a.volume = v;
    spotifySetVolume(v).catch(() => {});
    setState((s) => ({ ...s, volume: v }));
  }

  // ----- Podcast playback position tracking -----

  const clearPodcastSaveInterval = useCallback(() => {
    if (podcastSaveIntervalRef.current !== null) {
      clearInterval(podcastSaveIntervalRef.current);
      podcastSaveIntervalRef.current = null;
    }
  }, []);

  const savePodcastPosition = useCallback(async () => {
    if (podcastEpisodeIdRef.current === null) return;
    const a = audioRef.current;
    if (!a) return;
    const pos = Math.floor(a.currentTime);
    if (pos <= 0) return;
    const completed = a.ended || (a.duration > 0 && a.currentTime >= a.duration - 1);
    try {
      await api.savePodcastEpisodePosition(podcastEpisodeIdRef.current, pos, completed);
    } catch {
      /* ignore */
    }
  }, []);

  const setPodcastEpisodeId = useCallback(async (episodeId: number | null) => {
    // Save position for previous episode before switching (await to ensure it completes)
    if (podcastEpisodeIdRef.current !== null && podcastEpisodeIdRef.current !== episodeId) {
      await savePodcastPosition();
    }
    clearPodcastSaveInterval();
    podcastEpisodeIdRef.current = episodeId;
    if (episodeId !== null) {
      // Save position every 5 seconds during playback
      podcastSaveIntervalRef.current = window.setInterval(() => {
        savePodcastPosition();
      }, 5000);
    }
  }, [savePodcastPosition, clearPodcastSaveInterval]);

  // Save position on page unload
  useEffect(() => {
    const handleUnload = () => {
      if (podcastEpisodeIdRef.current !== null) {
        const a = audioRef.current;
        if (a) {
          const pos = Math.floor(a.currentTime);
          if (pos > 0) {
            // Use sendBeacon for reliable delivery on page unload
            const data = JSON.stringify({ position_sec: pos, completed: a.ended });
            navigator.sendBeacon(
              `/api/podcasts/episodes/${podcastEpisodeIdRef.current}/position`,
              new Blob([data], { type: 'application/json' })
            );
          }
        }
      }
    };
    window.addEventListener('beforeunload', handleUnload);
    return () => {
      window.removeEventListener('beforeunload', handleUnload);
      clearPodcastSaveInterval();
    };
  }, [clearPodcastSaveInterval]);

  return (
    <Ctx.Provider
      value={{ ...state, play, toggle, next, prev, seek, setVolume, toggleShuffle, toggleRepeat, setPodcastEpisodeId }}
    >
      {children}
    </Ctx.Provider>
  );
}

export function usePlayer() {
  const c = useContext(Ctx);
  if (!c) throw new Error("PlayerProvider missing");
  return c;
}
