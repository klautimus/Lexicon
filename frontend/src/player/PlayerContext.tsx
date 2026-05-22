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
  loading: boolean;
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
    loading: false,
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
  const volumeRef = useRef<number>(0.9);
  const toast = useToast();
  const wsRef = useRef<ReturnType<typeof getPlayerWebSocket> | null>(null);
  const isWsPlayerRef = useRef<boolean>(false);
  // Cache the Spotify player promise so we don't re-init on every poll tick
  const spotifyPlayerPromiseRef = useRef<ReturnType<typeof getSpotifyPlayer> | null>(null);

  // ----- Helpers -----

  const clearSkipTimeout = useCallback(() => {
    if (skipTimeoutRef.current !== null) {
      clearTimeout(skipTimeoutRef.current);
      skipTimeoutRef.current = null;
    }
  }, []);

  const flushLocalPlay = useCallback((completed: boolean) => {
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
  }, []);

  // ----- Web Audio API: Dynamics Compressor -----
  const initAudioPipeline = useCallback(() => {
    const a = audioRef.current;
    if (!a || audioCtxRef.current) return;
    try {
      const ctx = new AudioContext();
      ctx.resume(); // Required: AudioContext starts suspended due to autoplay policy
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

  // ----- Core playback logic (useCallback so effects get stable refs) -----

  const loadAndPlay = useCallback(async (t: Track) => {
    clearSkipTimeout();
    const session = ++playSessionRef.current;
    consecutiveErrorsRef.current = 0;

    flushLocalPlay(false);
    currentRef.current = t;
    setState((s) => ({ ...s, error: null, loading: true }));

    const isSpotify = !!t.spotify_id;

    if (isSpotify) {
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
        setState((s) => ({ ...s, error: "No Spotify URI", loading: false }));
        return;
      }
      try {
        await spotifyPlayURI(uri);
        // Guard: if a newer loadAndPlay was called, discard this result
        if (playSessionRef.current !== session) return;
        setState((s) => ({ ...s, loading: false }));
      } catch (e: any) {
        if (playSessionRef.current !== session) return;
        const msg = e?.message || "Spotify playback failed";
        setState((s) => ({
          ...s,
          error: msg.includes("403")
            ? "Spotify Premium required to play in-app."
            : msg,
          loading: false,
        }));
      }
      return;
    }

    // Local file
    const a = audioRef.current!;
    a.pause();
    initAudioPipeline();
    // Ensure AudioContext is running (browsers may suspend it)
    if (audioCtxRef.current?.state === 'suspended') {
      audioCtxRef.current.resume();
    }
    a.src = api.streamUrl(t.id);
    a.play()
      .then(() => {
        // Guard: if a newer loadAndPlay was called, discard this result
        if (playSessionRef.current !== session) return;
        sourceRef.current = "local";
        setState((s) => ({ ...s, source: "local", error: null, loading: false }));
        playStartRef.current = Math.floor(Date.now() / 1000);
        playSecondsRef.current = 0;
        consecutiveErrorsRef.current = 0;
        if (t.loudness_integrated != null) {
          const targetLUFS = -14;
          const gainDB = targetLUFS - t.loudness_integrated;
          const gainLinear = Math.pow(10, gainDB / 20);
          a.volume = Math.min(1.0, volumeRef.current * gainLinear);
        }
      })
      .catch((e: any) => {
        // Guard: if a newer loadAndPlay was called, discard this result
        if (playSessionRef.current !== session) return;
        const msg = e?.message || "Audio playback failed";
        console.error("[player] play failed:", msg);
        sourceRef.current = null;
        currentRef.current = null;
        setState((s) => ({ ...s, source: null, error: msg, playing: false, loading: false }));
        consecutiveErrorsRef.current++;
        if (consecutiveErrorsRef.current >= 5) {
          toast.error("Multiple tracks failed to play — stopping playback");
          consecutiveErrorsRef.current = 0;
          return;
        }
        toast.error("Failed to play track — skipping to next");
        scheduleSkip();
      });
  }, [clearSkipTimeout, flushLocalPlay, initAudioPipeline]);

  const goNext = useCallback(() => {
    clearSkipTimeout();
    // Read current state via ref to avoid nested setState
    const s = stateRef.current;
    if (s.repeatMode === "one" && s.current) {
      loadAndPlay(s.current);
      return;
    }

    const ni = s.index + 1;
    if (ni >= s.queue.length) {
      if (s.repeatMode === "all" && s.queue.length > 0) {
        const first = s.queue[0];
        if (s.shuffled && s.queue.length > 1) {
          const newQueue = shuffleArray(s.queue);
          loadAndPlay(newQueue[0]);
          setState((prev) => ({ ...prev, queue: newQueue, index: 0, current: newQueue[0] }));
          return;
        }
        loadAndPlay(first);
        setState((prev) => ({ ...prev, index: 0, current: first }));
        return;
      }
      if (sourceRef.current === "local") {
        const a = audioRef.current;
        if (a) a.pause();
      } else if (sourceRef.current === "spotify") {
        spotifyPause().catch(() => {});
      }
      setState((prev) => ({ ...prev, playing: false }));
      return;
    }
    const nt = s.queue[ni];
    loadAndPlay(nt);
    setState((prev) => ({ ...prev, index: ni, current: nt }));
  }, [clearSkipTimeout, loadAndPlay]);

  const scheduleSkip = useCallback(() => {
    clearSkipTimeout();
    skipTimeoutRef.current = window.setTimeout(() => {
      skipTimeoutRef.current = null;
      goNext();
    }, 1500);
  }, [clearSkipTimeout, goNext]);

  const next = useCallback(() => {
    flushLocalPlay(false);
    goNext();
  }, [flushLocalPlay, goNext]);

  const prev = useCallback(() => {
    flushLocalPlay(false);
    setState((s) => {
      const ni = Math.max(0, s.index - 1);
      const nt = s.queue[ni];
      if (nt) loadAndPlay(nt);
      return { ...s, index: ni, current: nt };
    });
  }, [flushLocalPlay, loadAndPlay]);

  const seekFn = useCallback((sec: number) => {
    if (sourceRef.current === "spotify") {
      spotifySeek(sec * 1000).catch(() => {});
      return;
    }
    const a = audioRef.current;
    if (a) a.currentTime = sec;
  }, []);

  const toggleFn = useCallback(async () => {
    if (sourceRef.current === "spotify") {
      try {
        await spotifyToggle();
      } catch {}
      return;
    }
    const a = audioRef.current;
    if (!a) return;
    if (a.paused) a.play();
    else a.pause();
  }, []);

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

  const setPodcastEpisodeIdCb = useCallback(async (episodeId: number | null) => {
    if (podcastEpisodeIdRef.current !== null && podcastEpisodeIdRef.current !== episodeId) {
      await savePodcastPosition();
    }
    clearPodcastSaveInterval();
    podcastEpisodeIdRef.current = episodeId;
    if (episodeId !== null) {
      podcastSaveIntervalRef.current = window.setInterval(() => {
        savePodcastPosition();
      }, 5000);
    }
  }, [savePodcastPosition, clearPodcastSaveInterval]);

  const playFn = useCallback(async (tracks: Track[], startIndex = 0) => {
    if (tracks.length === 0) return;
    clearSkipTimeout();
    consecutiveErrorsRef.current = 0;
    if (podcastEpisodeIdRef.current !== null) {
      await savePodcastPosition();
      await setPodcastEpisodeIdCb(null);
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
  }, [clearSkipTimeout, loadAndPlay, setPodcastEpisodeIdCb]);

  // ----- WebSocket setup (stable callbacks, proper deps) -----
  useEffect(() => {
    const ws = getPlayerWebSocket();
    wsRef.current = ws;
    isWsPlayerRef.current = ws.isPlayer();
    ws.connect();

    ws.onState((msg: any) => {
      if (!isWsPlayerRef.current) return;
      if (msg.type === "play" && msg.track_id) {
        api.track(msg.track_id).then((t) => {
          playFn([t], 0);
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
        seekFn(msg.position);
      }
    });

    // Handle role reassignment: this device becomes the audio player
    ws.onPromoted((msg) => {
      isWsPlayerRef.current = true;
      // Accept queue + track from the transferring device
      const tracks = msg.queue || msg.tracks;
      const track = msg.currentTrack || msg.track;
      if (tracks && track && tracks.length > 0) {
        clearSkipTimeout();
        consecutiveErrorsRef.current = 0;
        originalQueueRef.current = [...tracks];
        shuffledRef.current = false;
        setState((s) => ({
          ...s,
          queue: tracks,
          index: 0,
          current: track,
          shuffled: false,
        }));
        loadAndPlay(track);
      }
    });

    // Handle role reassignment: this device becomes a passive controller
    ws.onDemoted(() => {
      const a = audioRef.current;
      if (a) a.pause();
      isWsPlayerRef.current = false;
      setState((s) => ({ ...s, playing: false }));
    });

    // DEAD CODE (T8 fix): onTransfer was never sent by the hub.
    // The hub now sends the queue + currentTrack directly in the transfer
    // message passed to handleTransfer, which builds the promoted message.
    // Queue continuity is handled upstream — no per-client transfer handshake.
    // ws.onTransfer((msg: any) => { ... });


    return () => {
      ws.disconnect();
    };
  }, [playFn, next, prev, seekFn, loadAndPlay, clearSkipTimeout]);

  // Keep a ref to the latest state so broadcastState doesn't need to be
  // recreated on every tick of the Spotify poller (which sets state every 1s).
  const stateRef = useRef(state);
  stateRef.current = state;

  // Broadcast state changes to controllers when we're the player.
  // Stable callback (empty deps) — reads latest state from ref.
  const broadcastState = useCallback(() => {
    if (!isWsPlayerRef.current || !wsRef.current) return;
    const s = stateRef.current;
    wsRef.current.send({
      type: "state",
      playing: s.playing,
      track: s.current,
      position: s.position,
      duration: s.duration,
      device: "host",
      tracks: s.queue,
      start_index: s.index,
    });
  }, []);

  useEffect(() => {
    broadcastState();
  }, [broadcastState]);

  // ----- Local <audio> setup -----
  useEffect(() => {
    const a = new Audio();
    a.preload = "auto";
    a.crossOrigin = "anonymous";
    a.setAttribute("playsinline", "playsinline");
    a.setAttribute("webkit-playsinline", "webkit-playsinline");
    audioRef.current = a;
    a.volume = volumeRef.current;
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
  }, [flushLocalPlay, goNext, scheduleSkip]);

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
      if (cancelled || sourceRef.current !== "spotify") return;
      try {
        // Cache the player promise so we don't re-init SDK on every tick
        if (!spotifyPlayerPromiseRef.current) {
          spotifyPlayerPromiseRef.current = getSpotifyPlayer();
        }
        const { player } = await spotifyPlayerPromiseRef.current;
        if (cancelled) return;
        const st = await player.getCurrentState();
        if (cancelled) return;
        if (!st) return;
        setState((s) => ({
          ...s,
          playing: !st.paused,
          position: st.position / 1000,
          duration: st.duration / 1000,
        }));
      } catch {
        // ignore
      }
    }, 1000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);

  function shuffleArray<T>(arr: T[]): T[] {
    const a = [...arr];
    for (let i = a.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1));
      [a[i], a[j]] = [a[j], a[i]];
    }
    return a;
  }

  const toggleShuffle = useCallback(() => {
    setState((s) => {
      if (s.shuffled) {
        const restored = [...originalQueueRef.current];
        const idx = restored.findIndex((t) => t.id === s.current?.id);
        shuffledRef.current = false;
        return { ...s, queue: restored, index: Math.max(0, idx), shuffled: false };
      } else {
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
  }, []);

  const toggleRepeat = useCallback(() => {
    setState((s) => {
      const next: RepeatMode = s.repeatMode === "off" ? "all" : s.repeatMode === "all" ? "one" : "off";
      return { ...s, repeatMode: next };
    });
  }, []);

  const setVolume = useCallback((v: number) => {
    volumeRef.current = v;
    const a = audioRef.current;
    if (a) a.volume = v;
    // Only call Spotify API when Spotify is the active source
    if (sourceRef.current === "spotify") {
      spotifySetVolume(v).catch(() => {});
    }
    setState((s) => ({ ...s, volume: v }));
  }, []);

  // Save position on page unload
  useEffect(() => {
    const handleUnload = () => {
      if (podcastEpisodeIdRef.current !== null) {
        const a = audioRef.current;
        if (a) {
          const pos = Math.floor(a.currentTime);
          if (pos > 0) {
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
      value={{
        ...state,
        play: playFn,
        toggle: toggleFn,
        next,
        prev,
        seek: seekFn,
        setVolume,
        toggleShuffle,
        toggleRepeat,
        setPodcastEpisodeId: setPodcastEpisodeIdCb,
      }}
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
