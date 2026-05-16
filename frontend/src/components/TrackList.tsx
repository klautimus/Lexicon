import { useState, useEffect, useRef } from "react";
import { Play, MoreHorizontal, Plus, ListMusic, Trash2 } from "lucide-react";
import { api, Track, Playlist } from "../lib/api";
import { usePlayer } from "../player/PlayerContext";
import { useIsMobile } from "../hooks/useIsMobile";

export default function TrackList({ tracks, onDelete }: { tracks: Track[]; onDelete?: (trackId: number) => void }) {
  const isMobile = useIsMobile();
  if (isMobile) {
    return <MobileCardList tracks={tracks} onDelete={onDelete} />;
  }
  return <DesktopTable tracks={tracks} onDelete={onDelete} />;
}

/* ------------------------------------------------------------------ */
/*  Desktop Table                                                       */
/* ------------------------------------------------------------------ */

function DesktopTable({ tracks, onDelete }: { tracks: Track[]; onDelete?: (trackId: number) => void }) {
  return (
    <div className="rounded-lg border border-panel2">
      <table className="w-full text-sm">
        <thead className="bg-panel2/60 text-muted">
          <tr>
            <th className="text-left px-4 py-2 w-10">#</th>
            <th className="text-left px-4 py-2">Title</th>
            <th className="text-left px-4 py-2">Artist</th>
            <th className="text-left px-4 py-2 max-w-48">Album</th>
            <th className="text-left px-4 py-2 w-10"></th>
          </tr>
        </thead>
        <tbody>
          {tracks.map((t, i) => (
            <TrackRow key={`${t.id}-${i}`} track={t} index={i} tracks={tracks} onDelete={onDelete} />
          ))}
          {tracks.length === 0 && (
            <tr>
              <td colSpan={5} className="px-4 py-8 text-center text-muted">
                No tracks.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

function TrackRow({
  track,
  index,
  tracks,
  onDelete,
}: {
  track: Track;
  index: number;
  tracks: Track[];
  onDelete?: (trackId: number) => void;
}) {
  const p = usePlayer();
  const [open, setOpen] = useState(false);
  const [playlists, setPlaylists] = useState<Playlist[]>([]);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const [addedMsg, setAddedMsg] = useState("");
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState("");
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  async function loadPlaylists() {
    try {
      const data = await api.playlists();
      setPlaylists(data);
    } catch {
      // ignore
    }
  }

  function toggle() {
    if (!open) loadPlaylists();
    setOpen(!open);
    setAddedMsg("");
    setConfirmingDelete(false);
    setDeleteError("");
  }

  async function addToPlaylist(playlistId: number, playlistName: string) {
    try {
      await api.addToPlaylist(playlistId, track.id);
      setAddedMsg(`Added to "${playlistName}"`);
      setTimeout(() => setAddedMsg(""), 2000);
    } catch {
      setAddedMsg("Failed to add");
    }
  }

  async function createPlaylist(e: React.FormEvent) {
    e.preventDefault();
    if (!newName.trim()) return;
    try {
      const pl = await api.createPlaylist(newName.trim());
      await api.addToPlaylist(pl.id, track.id);
      setNewName("");
      setCreating(false);
      setAddedMsg(`Added to "${pl.name}"`);
      setTimeout(() => setAddedMsg(""), 2000);
      loadPlaylists();
    } catch {
      setAddedMsg("Failed to create");
    }
  }

  async function handleDelete() {
    setDeleting(true);
    setDeleteError("");
    try {
      await api.deleteTrack(track.id);
      setOpen(false);
      setConfirmingDelete(false);
      onDelete?.(track.id);
    } catch {
      setDeleteError("Failed to delete");
    } finally {
      setDeleting(false);
    }
  }

  return (
    <tr
      onDoubleClick={() => p.play(tracks, index)}
      className="border-t border-panel2 hover:bg-panel2/40 cursor-pointer group"
    >
      <td className="px-4 py-2 text-muted">
        <button
          onClick={() => p.play(tracks, index)}
          className="opacity-0 group-hover:opacity-100 hover:text-accent"
        >
          <Play size={14} />
        </button>
        <span className="group-hover:hidden">{index + 1}</span>
      </td>
      <td className="px-4 py-2 truncate">{track.title}</td>
      <td className="px-4 py-2 text-muted truncate">{track.artist}</td>
      <td className="px-4 py-2 text-muted truncate max-w-48" title={track.album}>{track.album}</td>
      <td className="px-4 py-2 relative">
        <div ref={ref}>
          <button
            onClick={(e) => {
              e.stopPropagation();
              toggle();
            }}
            className="opacity-0 group-hover:opacity-100 text-muted hover:text-text p-1"
          >
            <MoreHorizontal size={14} />
          </button>
          {open && (
            <div className="absolute right-0 top-full z-20 mt-1 w-56 bg-panel border border-panel2 rounded-lg shadow-lg py-1 max-h-72 overflow-y-auto">
              {addedMsg ? (
                <div className="px-3 py-2 text-sm text-green-400">
                  {addedMsg}
                </div>
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
                        setConfirmingDelete(true);
                      }}
                      className="w-full text-left px-3 py-2 text-sm hover:bg-panel2 flex items-center gap-2 text-red-400"
                    >
                      <Trash2 size={14} />
                      Delete from Library
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
}

/* ------------------------------------------------------------------ */
/*  Mobile Card List                                                    */
/* ------------------------------------------------------------------ */

function MobileCardList({ tracks, onDelete }: { tracks: Track[]; onDelete?: (trackId: number) => void }) {
  if (tracks.length === 0) {
    return <p className="text-muted text-center py-8">No tracks.</p>;
  }
  return (
    <div className="space-y-2">
      {tracks.map((t, i) => (
        <MobileTrackCard key={`${t.id}-${i}`} track={t} index={i} tracks={tracks} onDelete={onDelete} />
      ))}
    </div>
  );
}

function MobileTrackCard({
  track,
  index,
  tracks,
  onDelete,
}: {
  track: Track;
  index: number;
  tracks: Track[];
  onDelete?: (trackId: number) => void;
}) {
  const p = usePlayer();
  const [open, setOpen] = useState(false);
  const [playlists, setPlaylists] = useState<Playlist[]>([]);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const [addedMsg, setAddedMsg] = useState("");
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState("");
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  async function loadPlaylists() {
    try {
      const data = await api.playlists();
      setPlaylists(data);
    } catch {
      // ignore
    }
  }

  function toggle(e: React.MouseEvent) {
    e.stopPropagation();
    if (!open) loadPlaylists();
    setOpen(!open);
    setAddedMsg("");
    setConfirmingDelete(false);
    setDeleteError("");
  }

  async function addToPlaylist(playlistId: number, playlistName: string) {
    try {
      await api.addToPlaylist(playlistId, track.id);
      setAddedMsg(`Added to "${playlistName}"`);
      setTimeout(() => setAddedMsg(""), 2000);
    } catch {
      setAddedMsg("Failed to add");
    }
  }

  async function createPlaylist(e: React.FormEvent) {
    e.preventDefault();
    if (!newName.trim()) return;
    try {
      const pl = await api.createPlaylist(newName.trim());
      await api.addToPlaylist(pl.id, track.id);
      setNewName("");
      setCreating(false);
      setAddedMsg(`Added to "${pl.name}"`);
      setTimeout(() => setAddedMsg(""), 2000);
      loadPlaylists();
    } catch {
      setAddedMsg("Failed to create");
    }
  }

  async function handleDelete() {
    setDeleting(true);
    setDeleteError("");
    try {
      await api.deleteTrack(track.id);
      setOpen(false);
      setConfirmingDelete(false);
      onDelete?.(track.id);
    } catch {
      setDeleteError("Failed to delete");
    } finally {
      setDeleting(false);
    }
  }

  return (
    <div
      className="bg-panel border border-panel2 rounded-lg p-3 flex items-center gap-3 active:bg-panel2/50 transition-colors relative"
      onClick={() => p.play(tracks, index)}
    >
      <img
        src={api.coverUrl(track.id)}
        alt=""
        className="w-12 h-12 rounded bg-panel2 object-cover flex-shrink-0"
        onError={(e) => ((e.target as HTMLImageElement).style.opacity = "0")}
      />
      <div className="flex-1 min-w-0">
        <div className="truncate text-sm font-medium">{track.title}</div>
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
          className="w-9 h-9 rounded-full bg-accent text-bg flex items-center justify-center"
        >
          <Play size={14} className="ml-0.5" />
        </button>
        <button
          onClick={toggle}
          className="w-9 h-9 flex items-center justify-center text-muted"
        >
          <MoreHorizontal size={18} />
        </button>

        {open && (
          <div className="absolute right-2 top-12 z-20 w-56 bg-panel border border-panel2 rounded-lg shadow-lg py-1 max-h-72 overflow-y-auto">
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
                      setConfirmingDelete(true);
                    }}
                    className="w-full text-left px-3 py-2 text-sm hover:bg-panel2 flex items-center gap-2 text-red-400"
                  >
                    <Trash2 size={14} />
                    Delete from Library
                  </button>
                </div>
              </>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

