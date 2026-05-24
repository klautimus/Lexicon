import { useState, useEffect, useRef, useCallback, memo } from "react";
import { Play, MoreHorizontal, Plus, ListMusic, Trash2, Download, ListPlus } from "lucide-react";
import { api, Track, Playlist } from "../lib/api";
import { usePlayer } from "../player/PlayerContext";
import { useIsMobile } from "../hooks/useIsMobile";
import { useToast } from "../contexts/ToastContext";

type SortField = "title" | "artist" | "album" | "duration";
type SortDir = "asc" | "desc";

interface TrackListProps {
  tracks: Track[];
  onDelete?: (trackId: number) => void;
  sortField?: SortField | null;
  sortDir?: SortDir;
  onSort?: (field: SortField) => void;
  player?: ReturnType<typeof usePlayer>;
}

// Module-level playlist cache to avoid redundant API calls when
// multiple TrackRow menus are opened (30s TTL).
let playlistCache: { data: Playlist[]; ts: number } | null = null;
async function getCachedPlaylists(): Promise<Playlist[]> {
  if (playlistCache && Date.now() - playlistCache.ts < 30_000) {
    return playlistCache.data;
  }
  const data = await api.playlists();
  playlistCache = { data, ts: Date.now() };
  return data;
}

export default function TrackList({ tracks, onDelete, sortField, sortDir, onSort, player }: TrackListProps) {
  const isMobile = useIsMobile();
  if (isMobile) {
    return <MobileCardList tracks={tracks} onDelete={onDelete} sortField={sortField} sortDir={sortDir} onSort={onSort} player={player} />;
  }
  return <DesktopTable tracks={tracks} onDelete={onDelete} sortField={sortField} sortDir={sortDir} onSort={onSort} player={player} />;
}

function SortHeader({ field, label, sortField, sortDir, onSort }: {
  field: SortField; label: string; sortField: SortField | null | undefined;
  sortDir: SortDir | undefined; onSort?: (f: SortField) => void;
}) {
  const ariaSort = sortField !== field ? "none" : sortDir === "asc" ? "ascending" : "descending";
  return (
    <th
      className="text-left px-4 py-2 cursor-pointer select-none hover:text-text"
      onClick={() => onSort?.(field)}
      aria-sort={ariaSort}
    >
      {label} {sortField === field ? (sortDir === "asc" ? "↑" : "↓") : ""}
    </th>
  );
}

/* ------------------------------------------------------------------ */
/*  Desktop Table                                                       */
/* ------------------------------------------------------------------ */

function DesktopTable({ tracks, onDelete, sortField, sortDir, onSort, player }: TrackListProps) {
  return (
    <div className="rounded-lg border border-panel2">
      <table className="w-full text-sm">
        <caption className="sr-only">Track list</caption>
        <thead className="bg-panel2/60 text-muted sticky top-0">
          <tr>
            <th className="text-left px-4 py-2 w-10">#</th>
            <SortHeader field="title" label="Title" sortField={sortField} sortDir={sortDir} onSort={onSort} />
            <SortHeader field="artist" label="Artist" sortField={sortField} sortDir={sortDir} onSort={onSort} />
            <SortHeader field="album" label="Album" sortField={sortField} sortDir={sortDir} onSort={onSort} />
            <SortHeader field="duration" label="Duration" sortField={sortField} sortDir={sortDir} onSort={onSort} />
            <th className="text-left px-4 py-2 w-10"></th>
          </tr>
        </thead>
        <tbody>
          {tracks.map((t, i) => (
            <TrackRow key={t.id} track={t} index={i} tracks={tracks} onDelete={onDelete} player={player} />
          ))}
          {tracks.length === 0 && (
            <tr>
              <td colSpan={6} className="px-4 py-8 text-center text-muted">
                No tracks.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

function formatDuration(sec: number): string {
  if (!sec || sec <= 0) return "—";
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}

const TrackRow = memo(function TrackRow({
  track,
  index,
  tracks,
  onDelete,
  player,
}: {
  track: Track;
  index: number;
  tracks: Track[];
  onDelete?: (trackId: number) => void;
  player?: ReturnType<typeof usePlayer>;
}) {
  const p = player ?? usePlayer();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [playlists, setPlaylists] = useState<Playlist[]>([]);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const [addedMsg, setAddedMsg] = useState("");
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState("");
  const ref = useRef<HTMLDivElement>(null);

  const isPlaying = p.current?.id === track.id;

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  const loadPlaylists = useCallback(async () => {
    try {
      const data = await getCachedPlaylists();
      setPlaylists(data);
    } catch {
      // ignore
    }
  }, []);

  const toggle = useCallback(() => {
    if (!open) loadPlaylists();
    setOpen((v) => !v);
    setAddedMsg("");
    setConfirmingDelete(false);
    setDeleteError("");
  }, [open, loadPlaylists]);

  const addToPlaylist = useCallback(async (playlistId: number, playlistName: string) => {
    try {
      await api.addToPlaylist(playlistId, track.id);
      setAddedMsg(`Added to "${playlistName}"`);
      setTimeout(() => setAddedMsg(""), 2000);
    } catch {
      setAddedMsg("Failed to add");
    }
  }, [track.id]);

  const createPlaylist = useCallback(async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newName.trim()) return;
    try {
      const pl = await api.createPlaylist(newName.trim());
      await api.addToPlaylist(pl.id, track.id);
      playlistCache = null; // invalidate so next load fetches fresh data
      setNewName("");
      setCreating(false);
      setAddedMsg(`Added to "${pl.name}"`);
      setTimeout(() => setAddedMsg(""), 2000);
      loadPlaylists();
    } catch {
      setAddedMsg("Failed to create");
    }
  }, [newName, track.id, loadPlaylists]);

  const handleDelete = useCallback(async () => {
    setDeleting(true);
    setDeleteError("");
    try {
      await api.deleteTrack(track.id);
      setOpen(false);
      setConfirmingDelete(false);
      onDelete?.(track.id);
    } catch (e) {
      console.error(`[TrackRow] delete failed for track ${track.id}:`, e);
      setDeleteError("Failed to delete");
    } finally {
      setDeleting(false);
    }
  }, [track.id, onDelete]);

  const handleUpgrade = useCallback(async () => {
    try {
      const result = await api.upgradeTrack(track.id);
      toast.info(`Upgrading "${track.title}" — ${result.message || "queued"}`);
    } catch {
      toast.error(`Failed to upgrade "${track.title}"`);
    }
  }, [track.id, track.title, toast]);

  const handleAddToQueue = useCallback(() => {
    p.addToQueue?.(track);
    setOpen(false);
    toast.info(`Added "${track.title}" to queue`);
  }, [p, track, toast]);

  return (
    <tr
      onDoubleClick={() => p.play(tracks, index)}
      className={`border-t border-panel2 hover:bg-panel2/50 cursor-pointer group${isPlaying ? " bg-accent/10" : ""}`}
    >
      <td className="px-4 py-2 text-muted">
        <button
          onClick={() => p.play(tracks, index)}
          className={isPlaying ? "text-accent" : "opacity-0 group-hover:opacity-100 hover:text-accent"}
          aria-label={`Play ${track.title}`}
        >
          <Play size={14} />
        </button>
        <span className={isPlaying ? "hidden" : "group-hover:hidden"}>{index + 1}</span>
        {isPlaying && <span className="text-accent text-xs">▶</span>}
      </td>
      <td className="px-4 py-2 truncate">{track.title}</td>
      <td className="px-4 py-2 text-muted truncate">{track.artist}</td>
      <td className="px-4 py-2 text-muted truncate max-w-48" title={track.album}>{track.album}</td>
      <td className="px-4 py-2 text-muted tabular-nums">{formatDuration(track.duration_sec)}</td>
      <td className="px-4 py-2 relative">
        <div ref={ref}>
          <button
            onClick={(e) => {
              e.stopPropagation();
              toggle();
            }}
            className="opacity-0 group-hover:opacity-100 text-muted hover:text-text p-1"
            aria-label="Track actions menu"
          >
            <MoreHorizontal size={14} />
          </button>
          {open && (
            <div className="absolute right-0 top-full z-20 mt-1 w-56 bg-panel border border-panel2 rounded-lg shadow-lg py-1 max-h-72 overflow-y-auto">
              {addedMsg ? (
                <div className="px-3 py-2 text-sm text-green-400">{addedMsg}</div>
              ) : deleteError ? (
                <div className="px-3 py-2 text-sm text-red-400">{deleteError}</div>
              ) : confirmingDelete ? (
                <div className="px-3 py-2 space-y-2" onClick={(e) => e.stopPropagation()}>
                  <p className="text-sm text-muted">Delete "{track.title}"?</p>
                  <div className="flex gap-2">
                    <button
                      onClick={handleDelete}
                      disabled={deleting}
                      className="flex-1 px-2 py-1 bg-red-500/20 text-red-400 text-xs rounded font-medium hover:bg-red-500/30 disabled:opacity-50"
                    >
                      {deleting ? "Deleting..." : "Confirm"}
                    </button>
                    <button
                      onClick={() => setConfirmingDelete(false)}
                      className="px-2 py-1 text-xs text-muted hover:text-text"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              ) : creating ? (
                <form
                  onSubmit={createPlaylist}
                  className="px-3 py-2 space-y-2"
                  onClick={(e) => e.stopPropagation()}
                >
                  <input
                    type="text"
                    value={newName}
                    onChange={(e) => setNewName(e.target.value)}
                    placeholder="Playlist name..."
                    autoFocus
                    className="w-full bg-panel2 border border-panel2 rounded px-2 py-1 text-sm focus:outline-none focus:border-accent"
                  />
                  <div className="flex gap-2">
                    <button
                      type="submit"
                      className="flex-1 px-2 py-1 bg-accent text-black text-xs rounded font-medium"
                    >
                      Create & Add
                    </button>
                    <button
                      type="button"
                      onClick={() => setCreating(false)}
                      className="px-2 py-1 text-xs text-muted hover:text-text"
                    >
                      Cancel
                    </button>
                  </div>
                </form>
              ) : (
                <>
                  <div className="px-3 py-1.5 text-xs text-muted uppercase tracking-wide">
                    Add to playlist
                  </div>
                  {playlists.length === 0 ? (
                    <div className="px-3 py-2 text-sm text-muted">
                      No playlists yet.
                    </div>
                  ) : (
                    playlists.map((pl) => (
                      <button
                        key={pl.id}
                        onClick={(e) => {
                          e.stopPropagation();
                          addToPlaylist(pl.id, pl.name);
                        }}
                        className="w-full text-left px-3 py-2 text-sm hover:bg-panel2 flex items-center gap-2"
                      >
                        <ListMusic size={14} className="text-muted" />
                        <span className="truncate">{pl.name}</span>
                      </button>
                    ))
                  )}
                  <div className="border-t border-panel2 mt-1 pt-1">
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        setCreating(true);
                      }}
                      className="w-full text-left px-3 py-2 text-sm hover:bg-panel2 flex items-center gap-2 text-accent"
                    >
                      <Plus size={14} />
                      Create new playlist...
                    </button>
                  </div>
                  <div className="border-t border-panel2 mt-1 pt-1">
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        handleAddToQueue();
                      }}
                      className="w-full text-left px-3 py-2 text-sm hover:bg-panel2 flex items-center gap-2 text-blue-400"
                    >
                      <ListPlus size={14} />
                      Add to Queue
                    </button>
                  </div>
                  <div className="border-t border-panel2 mt-1 pt-1">
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        setOpen(false);
                        handleUpgrade();
                      }}
                      className="w-full text-left px-3 py-2 text-sm hover:bg-panel2 flex items-center gap-2 text-yellow-400"
                    >
                      <Download size={14} />
                      Upgrade Quality
                    </button>
                  </div>
                  <div className="border-t border-panel2 mt-1 pt-1">
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        setConfirmingDelete(true);
                      }}
                      className="w-full text-left px-3 py-2 text-sm hover:bg-panel2 flex items-center gap-2 text-red-400"
                    >
                      <Trash2 size={14} />
                      Delete
                    </button>
                  </div>
                </>
              )}
            </div>
          )}
        </div>
      </td>
    </tr>
  );
});

/* ------------------------------------------------------------------ */
/*  Mobile Card List                                                    */
/* ------------------------------------------------------------------ */

function MobileCardList({ tracks, onDelete, sortField, sortDir, onSort, player }: TrackListProps) {
  if (tracks.length === 0) {
    return <p className="text-muted text-center py-8">No tracks.</p>;
  }
  return (
    <div className="space-y-2">
      {tracks.map((t, i) => (
        <MobileTrackCard key={t.id} track={t} index={i} tracks={tracks} onDelete={onDelete} player={player} />
      ))}
    </div>
  );
}

const MobileTrackCard = memo(function MobileTrackCard({
  track,
  index,
  tracks,
  onDelete,
  player,
}: {
  track: Track;
  index: number;
  tracks: Track[];
  onDelete?: (trackId: number) => void;
  player?: ReturnType<typeof usePlayer>;
}) {
  const p = player ?? usePlayer();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [playlists, setPlaylists] = useState<Playlist[]>([]);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const [addedMsg, setAddedMsg] = useState("");
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState("");
  const ref = useRef<HTMLDivElement>(null);

  const isPlaying = p.current?.id === track.id;

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  const loadPlaylists = useCallback(async () => {
    try {
      const data = await getCachedPlaylists();
      setPlaylists(data);
    } catch {
      // ignore
    }
  }, []);

  const toggle = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    if (!open) loadPlaylists();
    setOpen((v) => !v);
    setAddedMsg("");
    setConfirmingDelete(false);
    setDeleteError("");
  }, [open, loadPlaylists]);

  const addToPlaylist = useCallback(async (playlistId: number, playlistName: string) => {
    try {
      await api.addToPlaylist(playlistId, track.id);
      setAddedMsg(`Added to "${playlistName}"`);
      setTimeout(() => setAddedMsg(""), 2000);
    } catch {
      setAddedMsg("Failed to add");
    }
  }, [track.id]);

  const createPlaylist = useCallback(async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newName.trim()) return;
    try {
      const pl = await api.createPlaylist(newName.trim());
      await api.addToPlaylist(pl.id, track.id);
      playlistCache = null; // invalidate so next load fetches fresh data
      setNewName("");
      setCreating(false);
      setAddedMsg(`Added to "${pl.name}"`);
      setTimeout(() => setAddedMsg(""), 2000);
      loadPlaylists();
    } catch {
      setAddedMsg("Failed to create");
    }
  }, [newName, track.id, loadPlaylists]);

  const handleDelete = useCallback(async () => {
    setDeleting(true);
    setDeleteError("");
    try {
      await api.deleteTrack(track.id);
      setOpen(false);
      setConfirmingDelete(false);
      onDelete?.(track.id);
    } catch (e) {
      console.error(`[MobileTrackCard] delete failed for track ${track.id}:`, e);
      setDeleteError("Failed to delete");
    } finally {
      setDeleting(false);
    }
  }, [track.id, onDelete]);

  const handleUpgrade = useCallback(async () => {
    try {
      const result = await api.upgradeTrack(track.id);
      toast.info(`Upgrading "${track.title}" — ${result.message || "queued"}`);
    } catch {
      toast.error(`Failed to upgrade "${track.title}"`);
    }
  }, [track.id, track.title, toast]);

  const handleAddToQueue = useCallback(() => {
    p.addToQueue?.(track);
    setOpen(false);
    toast.info(`Added "${track.title}" to queue`);
  }, [p, track, toast]);

  return (
    <div
      className={`bg-panel border border-panel2 rounded-lg p-3 flex items-center gap-3 active:bg-panel2/50 transition-colors relative${isPlaying ? " ring-1 ring-accent/50" : ""}`}
      onClick={() => p.play(tracks, index)}
    >
      <img
        src={api.coverUrl(track.id)}
        alt=""
        className="w-12 h-12 rounded bg-panel2 object-cover flex-shrink-0"
        onError={(e) => ((e.target as HTMLImageElement).style.opacity = "0")}
      />
      <div className="flex-1 min-w-0">
        <div className={`truncate text-sm font-medium${isPlaying ? " text-accent" : ""}`}>{track.title}</div>
        <div className="truncate text-xs text-muted">
          {track.artist} {track.album && `— ${track.album}`}
        </div>
      </div>

      <div className="flex items-center gap-1 flex-shrink-0" ref={ref}>
        <button
          onClick={(e) => {
            e.stopPropagation();
            p.play(tracks, index);
          }}
          className="w-11 h-11 rounded-full bg-accent text-bg flex items-center justify-center"
          aria-label={`Play ${track.title}`}
        >
          <Play size={16} className="ml-0.5" />
        </button>
        <button
          onClick={toggle}
          className="w-11 h-11 flex items-center justify-center text-muted"
          aria-label="Track actions menu"
        >
          <MoreHorizontal size={20} />
        </button>

        {open && (
          <div className="absolute right-2 top-14 z-20 w-56 bg-panel border border-panel2 rounded-lg shadow-lg py-1 max-h-72 overflow-y-auto">
            {addedMsg ? (
              <div className="px-3 py-2 text-sm text-green-400">{addedMsg}</div>
            ) : deleteError ? (
              <div className="px-3 py-2 text-sm text-red-400">{deleteError}</div>
            ) : confirmingDelete ? (
              <div className="px-3 py-2 space-y-2" onClick={(e) => e.stopPropagation()}>
                <p className="text-sm text-muted">Delete "{track.title}"?</p>
                <div className="flex gap-2">
                  <button
                    onClick={handleDelete}
                    disabled={deleting}
                    className="flex-1 px-2 py-1 bg-red-500/20 text-red-400 text-xs rounded font-medium hover:bg-red-500/30 disabled:opacity-50"
                  >
                    {deleting ? "Deleting..." : "Confirm"}
                  </button>
                  <button
                    onClick={() => setConfirmingDelete(false)}
                    className="px-2 py-1 text-xs text-muted hover:text-text"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            ) : creating ? (
              <form
                onSubmit={createPlaylist}
                className="px-3 py-2 space-y-2"
                onClick={(e) => e.stopPropagation()}
              >
                <input
                  type="text"
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  placeholder="Playlist name..."
                  autoFocus
                  className="w-full bg-panel2 border border-panel2 rounded px-2 py-1 text-sm focus:outline-none focus:border-accent"
                />
                <div className="flex gap-2">
                  <button
                    type="submit"
                    className="flex-1 px-2 py-1 bg-accent text-black text-xs rounded font-medium"
                  >
                    Create & Add
                  </button>
                  <button
                    type="button"
                    onClick={() => setCreating(false)}
                    className="px-2 py-1 text-xs text-muted hover:text-text"
                  >
                    Cancel
                  </button>
                </div>
              </form>
            ) : (
              <>
                <div className="px-3 py-1.5 text-xs text-muted uppercase tracking-wide">
                  Add to playlist
                </div>
                {playlists.length === 0 ? (
                  <div className="px-3 py-2 text-sm text-muted">No playlists yet.</div>
                ) : (
                  playlists.map((pl) => (
                    <button
                      key={pl.id}
                      onClick={(e) => {
                        e.stopPropagation();
                        addToPlaylist(pl.id, pl.name);
                      }}
                      className="w-full text-left px-3 py-2 text-sm hover:bg-panel2 flex items-center gap-2"
                    >
                      <ListMusic size={14} className="text-muted" />
                      <span className="truncate">{pl.name}</span>
                    </button>
                  ))
                )}
                <div className="border-t border-panel2 mt-1 pt-1">
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      setCreating(true);
                    }}
                    className="w-full text-left px-3 py-2 text-sm hover:bg-panel2 flex items-center gap-2 text-accent"
                  >
                    <Plus size={14} />
                    Create new playlist...
                  </button>
                </div>
                <div className="border-t border-panel2 mt-1 pt-1">
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      handleAddToQueue();
                    }}
                    className="w-full text-left px-3 py-2 text-sm hover:bg-panel2 flex items-center gap-2 text-blue-400"
                  >
                    <ListPlus size={14} />
                    Add to Queue
                  </button>
                </div>
                <div className="border-t border-panel2 mt-1 pt-1">
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      setOpen(false);
                      handleUpgrade();
                    }}
                    className="w-full text-left px-3 py-2 text-sm hover:bg-panel2 flex items-center gap-2 text-yellow-400"
                  >
                    <Download size={14} />
                    Upgrade Quality
                  </button>
                </div>
                <div className="border-t border-panel2 mt-1 pt-1">
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      setConfirmingDelete(true);
                    }}
                    className="w-full text-left px-3 py-2 text-sm hover:bg-panel2 flex items-center gap-2 text-red-400"
                  >
                    <Trash2 size={14} />
                    Delete
                  </button>
                </div>
              </>
            )}
          </div>
        )}
      </div>
    </div>
  );
});
