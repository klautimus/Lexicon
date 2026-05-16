import { useState } from "react";
import {
  Play,
  Pause,
  SkipBack,
  SkipForward,
  Shuffle,
  Repeat,
  Repeat1,
  ChevronDown,
  Volume2,
} from "lucide-react";
import { usePlayer } from "../player/PlayerContext";
import { api } from "../lib/api";

function fmt(s: number) {
  if (!isFinite(s) || s < 0) return "0:00";
  const m = Math.floor(s / 60);
  const r = Math.floor(s % 60);
  return `${m}:${r.toString().padStart(2, "0")}`;
}

export default function MobilePlayerBar() {
  const p = usePlayer();
  const t = p.current;
  const [expanded, setExpanded] = useState(false);

  if (!t) {
    return (
      <div className="fixed bottom-14 left-0 right-0 z-30 h-14 bg-panel border-t border-black/40 flex items-center px-4">
        <span className="text-sm text-muted">Nothing playing</span>
      </div>
    );
  }

  return (
    <>
      {/* Mini Player */}
      <div
        className="fixed bottom-14 left-0 right-0 z-30 h-14 bg-panel border-t border-black/40 flex items-center gap-3 px-3 active:bg-panel2/50 transition-colors"
        onClick={() => setExpanded(true)}
      >
        <img
          src={api.coverUrl(t.id)}
          alt=""
          className="w-10 h-10 rounded bg-panel2 object-cover flex-shrink-0"
          onError={(e) => ((e.target as HTMLImageElement).style.opacity = "0")}
        />
        <div className="flex-1 min-w-0">
          <div className="truncate text-sm font-medium">{t.title}</div>
          <div className="truncate text-xs text-muted">{t.artist}</div>
        </div>
        <button
          onClick={(e) => {
            e.stopPropagation();
            p.toggle();
          }}
          className="w-9 h-9 rounded-full bg-accent text-bg flex items-center justify-center flex-shrink-0"
        >
          {p.playing ? <Pause size={16} /> : <Play size={16} className="ml-0.5" />}
        </button>

        {/* Thin progress bar */}
        <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-panel2">
          <div
            className="h-full bg-accent"
            style={{
              width: `${p.duration ? (p.position / p.duration) * 100 : 0}%`,
            }}
          />
        </div>
      </div>

      {/* Expanded Player Overlay */}
      {expanded && (
        <div className="fixed inset-0 z-50 bg-bg flex flex-col" style={{ animation: "fadeIn 0.2s ease-out" }}>
          {/* Header */}
          <div className="flex items-center justify-between px-4 pt-4 pb-2">
            <button
              onClick={() => setExpanded(false)}
              className="p-2 text-muted hover:text-text"
            >
              <ChevronDown size={24} />
            </button>
            <span className="text-xs text-muted uppercase tracking-wide">
              Now Playing
            </span>
            <div className="w-10" />
          </div>

          {/* Cover Art */}
          <div className="flex-1 flex items-center justify-center px-8">
            <img
              src={api.coverUrl(t.id)}
              alt=""
              className="w-full max-w-xs aspect-square rounded-xl bg-panel2 object-cover shadow-2xl"
              onError={(e) =>
                ((e.target as HTMLImageElement).style.opacity = "0")
              }
            />
          </div>

          {/* Track Info */}
          <div className="px-6 pb-4">
            <div className="truncate text-xl font-semibold">{t.title}</div>
            <div className="truncate text-muted mt-1">
              {t.artist} {t.album && `— ${t.album}`}
            </div>
            {p.source === "spotify" && (
              <span className="inline-block mt-1 text-[9px] px-1.5 py-0.5 rounded bg-[#1DB954]/20 text-[#1DB954]">
                SPOTIFY
              </span>
            )}
            {p.error && (
              <div className="text-xs text-red-400 mt-1">{p.error}</div>
            )}
          </div>

          {/* Scrubber */}
          <div className="px-6 pb-2">
            <input
              type="range"
              min={0}
              max={p.duration || 0}
              value={p.position}
              onChange={(e) => p.seek(Number(e.target.value))}
              className="w-full accent-accent h-1.5"
            />
            <div className="flex justify-between text-xs text-muted mt-1">
              <span>{fmt(p.position)}</span>
              <span>{fmt(p.duration)}</span>
            </div>
          </div>

          {/* Controls */}
          <div className="flex items-center justify-center gap-6 pb-4">
            <button
              onClick={p.toggleShuffle}
              className={`p-2 ${p.shuffled ? "text-accent" : "text-muted"}`}
            >
              <Shuffle size={22} />
            </button>
            <button onClick={p.prev} className="p-2 text-text">
              <SkipBack size={28} />
            </button>
            <button
              onClick={p.toggle}
              className="w-16 h-16 rounded-full bg-accent text-bg flex items-center justify-center"
            >
              {p.playing ? (
                <Pause size={28} />
              ) : (
                <Play size={28} className="ml-1" />
              )}
            </button>
            <button onClick={p.next} className="p-2 text-text">
              <SkipForward size={28} />
            </button>
            <button
              onClick={p.toggleRepeat}
              className={`p-2 ${
                p.repeatMode !== "off" ? "text-accent" : "text-muted"
              }`}
              title={
                p.repeatMode === "off"
                  ? "Repeat Off"
                  : p.repeatMode === "all"
                  ? "Repeat All"
                  : "Repeat One"
              }
            >
              {p.repeatMode === "one" ? (
                <Repeat1 size={22} />
              ) : (
                <Repeat size={22} />
              )}
            </button>
          </div>

          {/* Volume */}
          <div className="flex items-center gap-3 px-6 pb-6">
            <Volume2 size={16} className="text-muted flex-shrink-0" />
            <input
              type="range"
              min={0}
              max={1}
              step={0.01}
              value={p.volume}
              onChange={(e) => p.setVolume(Number(e.target.value))}
              className="flex-1 accent-accent"
            />
          </div>
        </div>
      )}
    </>
  );
}
