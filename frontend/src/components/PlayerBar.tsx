import { Play, Pause, SkipBack, SkipForward, Volume2, Shuffle, Repeat, Repeat1 } from "lucide-react";
import { usePlayer } from "../player/PlayerContext";
import { api } from "../lib/api";

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
    <div className="h-20 bg-panel border-t border-black/40 px-4 flex items-center gap-4">
      <div className="flex items-center gap-3 w-72 min-w-0">
        {t ? (
          <>
            <img
              src={api.coverUrl(t.id)}
              alt=""
              className="w-12 h-12 rounded bg-panel2 object-cover"
              onError={(e) => ((e.target as HTMLImageElement).style.opacity = "0")}
            />
            <div className="min-w-0">
              <div className="truncate text-sm font-medium flex items-center gap-1">
                {t.title}
                {p.source === "spotify" && (
                  <span className="text-[9px] px-1 py-0.5 rounded bg-[#1DB954]/20 text-[#1DB954]">
                    SPOTIFY
                  </span>
                )}
              </div>
              <div className="truncate text-xs text-muted">
                {t.artist} {t.album && `— ${t.album}`}
              </div>
              {p.error && (
                <div className="truncate text-xs text-red-400">{p.error}</div>
              )}
            </div>
          </>
        ) : (
          <div className="text-sm text-muted">Nothing playing</div>
        )}
      </div>

      <div className="flex-1 flex flex-col items-center gap-1">
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

      <div className="flex items-center gap-2 w-32">
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
