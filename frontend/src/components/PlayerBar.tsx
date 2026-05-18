import { Play, Pause, SkipBack, SkipForward, Volume2, Shuffle, Repeat, Repeat1 } from "lucide-react";
import { usePlayer } from "../player/PlayerContext";
import { api } from "../lib/api";
import DevicePicker from "./DevicePicker";

function fmt(s: number) {
  if (!isFinite(s) || s < 0) return "0:00";
  const m = Math.floor(s / 60);
  const r = Math.floor(s % 60);
  return `${m}:${r.toString().padStart(2, "0")}`;
}

export default function PlayerBar() {
  const p = usePlayer();
  const t = p.current;

  return (
    <div className="bg-panel border-t border-black/40 px-3 py-2 md:px-4 md:h-20 md:py-0 md:flex md:items-center md:gap-4 flex flex-col gap-1.5">
      {/* ── Mobile: Row 1 ── */}
      <div className="flex items-center gap-2 w-full">
        {/* Track info — narrower on mobile */}
        <div className="flex items-center gap-2 min-w-0 flex-1">
          {t ? (
            <>
              <img
                src={api.coverUrl(t.id)}
                alt=""
                className="w-9 h-9 md:w-12 md:h-12 rounded bg-panel2 object-cover flex-shrink-0"
                onError={(e) => ((e.target as HTMLImageElement).style.opacity = "0")}
              />
              <div className="min-w-0 flex-1">
                <div className="truncate text-xs md:text-sm font-medium flex items-center gap-1">
                  {t.title}
                  {p.source === "spotify" && (
                    <span className="text-[8px] md:text-[9px] px-1 py-0.5 rounded bg-[#1DB954]/20 text-[#1DB954]">
                      SPOTIFY
                    </span>
                  )}
                </div>
                <div className="truncate text-[10px] md:text-xs text-muted">
                  {t.artist} {t.album && `— ${t.album}`}
                </div>
                {p.error && (
                  <div className="truncate text-[10px] md:text-xs text-red-400">{p.error}</div>
                )}
              </div>
            </>
          ) : (
            <div className="text-xs md:text-sm text-muted">Nothing playing</div>
          )}
        </div>

        {/* Play/Pause — always visible, larger touch target on mobile */}
        <button
          onClick={p.toggle}
          disabled={!t}
          className="md:hidden w-10 h-10 rounded-full bg-accent text-bg flex items-center justify-center hover:scale-105 transition disabled:opacity-30 flex-shrink-0"
        >
          {p.playing ? <Pause size={18} /> : <Play size={18} className="ml-0.5" />}
        </button>

        {/* DevicePicker — always visible, never shrinks */}
        <div className="flex-shrink-0">
          <DevicePicker currentTrack={t} />
        </div>
      </div>

      {/* ── Mobile: Row 2 — Progress bar ── */}
      <div className="flex items-center gap-2 w-full md:hidden">
        <span className="w-8 text-right text-[10px] text-muted">{fmt(p.position)}</span>
        <input
          type="range"
          min={0}
          max={p.duration || 0}
          value={p.position}
          onChange={(e) => p.seek(Number(e.target.value))}
          className="flex-1 accent-accent"
          disabled={!t}
        />
        <span className="w-8 text-[10px] text-muted">{fmt(p.duration)}</span>
      </div>

      {/* ── Desktop: Center controls ── */}
      <div className="hidden md:flex flex-1 flex-col items-center gap-1">
        <div className="flex items-center gap-3">
          <button
            onClick={p.toggleShuffle}
            disabled={!t}
            className={`hover:text-text transition disabled:opacity-30 ${
              p.shuffled ? "text-accent" : "text-muted"
            }`}
            title="Shuffle"
          >
            <Shuffle size={18} />
          </button>
          <button onClick={p.prev} className="text-muted hover:text-text" disabled={!t}>
            <SkipBack size={18} />
          </button>
          <button
            onClick={p.toggle}
            disabled={!t}
            className="w-9 h-9 rounded-full bg-accent text-bg flex items-center justify-center hover:scale-105 transition disabled:opacity-30"
          >
            {p.playing ? <Pause size={16} /> : <Play size={16} className="ml-0.5" />}
          </button>
          <button onClick={p.next} className="text-muted hover:text-text" disabled={!t}>
            <SkipForward size={18} />
          </button>
          <button
            onClick={p.toggleRepeat}
            disabled={!t}
            className={`hover:text-text transition disabled:opacity-30 ${
              p.repeatMode !== "off" ? "text-accent" : "text-muted"
            }`}
            title={
              p.repeatMode === "off" ? "Repeat Off" : p.repeatMode === "all" ? "Repeat All" : "Repeat One"
            }
          >
            {p.repeatMode === "one" ? <Repeat1 size={18} /> : <Repeat size={18} />}
          </button>
        </div>
        <div className="flex items-center gap-2 w-full max-w-xl text-xs text-muted">
          <span className="w-10 text-right">{fmt(p.position)}</span>
          <input
            type="range"
            min={0}
            max={p.duration || 0}
            value={p.position}
            onChange={(e) => p.seek(Number(e.target.value))}
            className="flex-1 accent-accent"
            disabled={!t}
          />
          <span className="w-10">{fmt(p.duration)}</span>
        </div>
      </div>

      {/* ── Desktop: Volume ── */}
      <div className="hidden md:flex items-center gap-2 w-32">
        <Volume2 size={14} className="text-muted" />
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
  );
}
