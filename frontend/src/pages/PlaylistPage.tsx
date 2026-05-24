import { useEffect, useState, useCallback } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import {
  Play,
  Trash2,
  ArrowLeft,
  Edit2,
  Check,
  X,
  Music,
  Clock,
  ListMusic,
  HelpCircle,
  Plus,
  Search,
  XCircle,
} from "lucide-react";
import { api, PlaylistWithTracks, Track } from "../lib/api";
import { usePlayer } from "../player/PlayerContext";
import { useToast } from "../contexts/ToastContext";
import { useHelp } from "../contexts/HelpContext";
import { useIsMobile } from "../hooks/useIsMobile";

function formatDuration(sec: number | null | undefined) {
  if (!sec || sec <= 0) return "0:00";
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}

/** Parse a user-friendly error message from the API error */
function parseApiError(e: unknown): string {
  if (e instanceof Error) {
    const msg = e.message;
    // Match "404 {"error":"not found"}" pattern
    const notFoundMatch = msg.match(/^404\s*\{/);
    if (notFoundMatch) {
      return "Playlist not found. It may have been deleted.";
    }
    // Match "409 {"error":"playlist with this name already exists"}" pattern
    const conflictMatch = msg.match(/^409\s*\{/);
    if (conflictMatch) {
      try {
        const jsonStr = msg.slice(msg.indexOf("{"));
        const parsed = JSON.parse(jsonStr);
        return parsed.error || "A playlist with this name already exists.";
      } catch {
        return "A playlist with this name already exists.";
      }
    }
    // Match other HTTP error codes
    const httpMatch = msg.match(/^(\d+)\s/);
    if (httpMatch) {
      return `Server error (${httpMatch[1]}): ${msg}`;
    }
    return msg;
  }
  return "An unexpected error occurred.";
}

export default function PlaylistPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const player = usePlayer();
  const toast = useToast();
  const { showHelp } = useHelp();
  const [playlist, setPlaylist] = useState<PlaylistWithTracks | null>(null);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState(false);
  const [editName, setEditName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notFound, setNotFound] = useState(false);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [showAddTracks, setShowAddTracks] = useState(false);

  // B7: Validate id param is a valid number
  const playlistId = id && /^\d+$/.test(id) ? Number(id) : null;

  async function load() {
    if (!playlistId) {
      setNotFound(true);
      setLoading(false);
      return;
    }
    try {
      const data = await api.playlist(playlistId);
      setPlaylist(data);
      setEditName(data.name);
      setError(null);
      setNotFound(false);
    } catch (e) {
      console.error("[PlaylistPage] failed to load playlist", id, e);
      const msg = parseApiError(e);
      if (msg.includes("not found")) {
        setNotFound(true);
        setError(null);
      } else {
        setError(msg);
      }
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  const remove = useCallback(async (pos: number) => {
    if (!playlistId) return;
    // B6/P1: Optimistic UI update
    const prevPlaylist = playlist;
    if (playlist) {
      const newTracks = playlist.tracks.filter((_, i) => i !== pos);
      setPlaylist({ ...playlist, tracks: newTracks, track_count: newTracks.length });
    }
    try {
      await api.removeFromPlaylist(playlistId, pos);
      toast.success("Track removed from playlist");
    } catch (e) {
      // Rollback on error
      if (prevPlaylist) setPlaylist(prevPlaylist);
      toast.error(parseApiError(e));
    }
  }, [playlistId, playlist, toast]);

  // F2: Drag-and-drop reorder
  const handleReorder = useCallback(async (from: number, to: number) => {
    if (!playlistId || from === to) return;
    // Optimistic update
    const prevPlaylist = playlist;
    if (playlist) {
      const newTracks = [...playlist.tracks];
      const [moved] = newTracks.splice(from, 1);
      newTracks.splice(to, 0, moved);
      setPlaylist({ ...playlist, tracks: newTracks });
    }
    try {
      await api.reorderPlaylist(playlistId, from, to);
    } catch (e) {
      // Rollback on error
      if (prevPlaylist) setPlaylist(prevPlaylist);
      toast.error(parseApiError(e));
    }
  }, [playlistId, playlist, toast]);

  async function saveName() {
    if (!playlistId || !editName.trim()) return;
    setSaving(true);
    try {
      await api.updatePlaylist(playlistId, editName.trim(), playlist?.description, playlist?.cover_art_path);
      setEditing(false);
      if (playlist) {
        setPlaylist({ ...playlist, name: editName.trim() });
      }
      toast.success("Playlist renamed");
    } catch (e) {
      toast.error(parseApiError(e));
    } finally {
      setSaving(false);
    }
  }

  async function deletePlaylist() {
    if (!playlistId) return;
    if (!window.confirm(`Delete "${playlist?.name || 'this playlist'}"? This cannot be undone.`)) return;
    setDeleting(true);
    try {
      await api.deletePlaylist(playlistId);
      toast.success("Playlist deleted");
      navigate("/playlists");
    } catch (e) {
      toast.error(parseApiError(e));
      setDeleting(false);
    }
  }

  // B8: Keyboard handler for inline edit
  function handleEditKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter") {
      e.preventDefault();
      saveName();
    } else if (e.key === "Escape") {
      setEditing(false);
      if (playlist) setEditName(playlist.name);
    }
  }

  if (loading) {
    return <p className="text-muted">Loading…</p>;
  }

  // B1: Friendly 404 state
  if (notFound || (!playlist && !error)) {
    return (
      <div className="space-y-4">
        <button
          onClick={() => navigate("/playlists")}
          className="flex items-center gap-1 text-sm text-muted hover:text-text"
        >
          <ArrowLeft size={14} /> Back to playlists
        </button>
        <div className="text-center py-12 space-y-3">
          <Music size={40} className="mx-auto text-muted" />
          <p className="text-lg font-medium">Playlist not found</p>
          <p className="text-sm text-muted">
            This playlist may have been deleted or the link is incorrect.
          </p>
          <button
            onClick={() => navigate("/playlists")}
            className="text-sm text-accent hover:underline"
          >
            Go to all playlists
          </button>
        </div>
      </div>
    );
  }

  if (!playlist) {
    return (
      <div className="space-y-4">
        <button
          onClick={() => navigate("/playlists")}
          className="flex items-center gap-1 text-sm text-muted hover:text-text"
        >
          <ArrowLeft size={14} /> Back to playlists
        </button>
        <p className="text-muted">Unable to load playlist.</p>
        {error && (
          <p className="text-sm text-red-400 bg-red-400/10 border border-red-400/30 rounded px-3 py-2">
            {error}
          </p>
        )}
        <button
          onClick={() => navigate("/playlists")}
          className="text-sm text-accent hover:underline"
        >
          Go to all playlists
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <button
        onClick={() => navigate("/playlists")}
        className="flex items-center gap-1 text-sm text-muted hover:text-text"
      >
        <ArrowLeft size={14} /> Back to playlists
      </button>

      <div className="flex items-start justify-between gap-4 group">
        <div className="flex items-start gap-4 flex-1">
          {playlist.cover_art_path ? (
            <img
              src={`/api/library/cover-by-path?path=${encodeURIComponent(playlist.cover_art_path)}`}
              alt=""
              className="w-20 h-20 rounded-lg object-cover flex-shrink-0"
              onError={(e) => ((e.target as HTMLImageElement).style.display = "none")}
            />
          ) : (
            <div className="w-20 h-20 rounded-lg bg-panel2 flex items-center justify-center flex-shrink-0">
              <Music size={32} className="text-accent" />
            </div>
          )}
          <div className="flex-1 min-w-0">
            {editing ? (
            <div className="flex items-center gap-2">
              <input
                type="text"
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                onKeyDown={handleEditKeyDown}
                className="bg-panel2 border border-panel2 rounded px-3 py-1.5 text-lg font-semibold focus:outline-none focus:border-accent"
                autoFocus
                aria-label="Playlist name"
                disabled={saving}
              />
              <button
                onClick={saveName}
                disabled={saving || !editName.trim()}
                className="p-1.5 text-green-400 hover:bg-panel2 rounded disabled:opacity-50"
                aria-label="Save playlist name"
              >
                <Check size={18} />
              </button>
              <button
                onClick={() => {
                  setEditing(false);
                  setEditName(playlist.name);
                }}
                disabled={saving}
                className="p-1.5 text-muted hover:bg-panel2 rounded disabled:opacity-50"
                aria-label="Cancel editing"
              >
                <X size={18} />
              </button>
            </div>
          ) : (
            <div className="flex items-center gap-2">
              <h1 className="text-2xl font-semibold">{playlist.name}</h1>
              <button
                onClick={() => setEditing(true)}
                className="p-1 text-muted hover:text-text"
                aria-label="Rename playlist"
              >
                <Edit2 size={14} />
              </button>
            </div>
          )}
          <div className="flex items-center gap-3 mt-1 text-sm text-muted">
            <span className="flex items-center gap-1">
              <ListMusic size={14} />
              {playlist.track_count} tracks
            </span>
            {playlist.total_duration > 0 && (
              <span className="flex items-center gap-1">
                <Clock size={14} />
                {formatDuration(playlist.total_duration)}
              </span>
            )}
          </div>
          {playlist.description && (
            <p className="text-sm text-muted mt-2">{playlist.description}</p>
          )}
        </div>
        </div>
        <div className="flex items-center gap-2">
          {playlist.tracks.length > 0 && (
            <button
              onClick={() => player.play(playlist.tracks, 0)}
              className="px-4 py-2 bg-accent hover:opacity-90 text-black font-medium rounded flex items-center gap-2"
              aria-label={`Play all tracks in ${playlist.name}`}
            >
              <Play size={16} />
              Play All
            </button>
          )}
          <button
            onClick={deletePlaylist}
            disabled={deleting}
            className="p-2 text-red-400 hover:bg-red-400/10 rounded opacity-100 md:opacity-0 md:group-hover:opacity-100 transition disabled:opacity-50"
            aria-label="Delete playlist"
            title="Delete playlist"
          >
            <Trash2 size={16} />
          </button>
        </div>
      </div>

      <button
        onClick={() => showHelp("playlist.detail")}
        className="text-xs text-muted hover:text-accent flex items-center gap-1 transition-colors"
      >
        <HelpCircle size={12} /> How to manage this playlist
      </button>

      {/* F3: Add tracks to playlist */}
      <div>
        <button
          onClick={() => setShowAddTracks(true)}
          className="flex items-center gap-2 px-4 py-2 bg-panel2 hover:bg-panel2/80 text-sm rounded transition-colors"
        >
          <Plus size={16} />
          Add tracks
        </button>
      </div>

      {showAddTracks && (
        <AddTracksModal
          playlistId={playlistId}
          existingTrackIds={new Set(playlist.tracks.map((t) => t.id))}
          onClose={() => setShowAddTracks(false)}
          onAdded={() => {
            load();
          }}
        />
      )}

      {playlist.tracks.length === 0 ? (
        <div className="text-center py-12 space-y-2">
          <Music size={40} className="mx-auto text-muted" />
          <p className="text-muted">No tracks yet.</p>
          <p className="text-xs text-muted">
            <Link to="/library" className="text-accent hover:underline">
              Browse your library
            </Link>{" "}
            and add tracks to this playlist.
          </p>
        </div>
      ) : (
        <PlaylistTrackList tracks={playlist.tracks} onRemove={(pos) => remove(pos)} player={player} onReorder={handleReorder} />
      )}
    </div>
  );
}

// F3: Add tracks modal
function AddTracksModal({
  playlistId,
  existingTrackIds,
  onClose,
  onAdded,
}: {
  playlistId: number | null;
  existingTrackIds: Set<number>;
  onClose: () => void;
  onAdded: () => void;
}) {
  const toast = useToast();
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Track[]>([]);
  const [searching, setSearching] = useState(false);
  const [addingId, setAddingId] = useState<number | null>(null);

  async function search() {
    if (!query.trim()) {
      setResults([]);
      return;
    }
    setSearching(true);
    try {
      const data = await api.search(query.trim());
      setResults(data.filter((t) => !existingTrackIds.has(t.id)));
    } catch {
      toast.error("Search failed");
    } finally {
      setSearching(false);
    }
  }

  async function addTrack(track: Track) {
    if (!playlistId) return;
    setAddingId(track.id);
    try {
      await api.addToPlaylist(playlistId, track.id);
      setResults(prev => prev.filter(t => t.id !== track.id));
      toast.success(`Added "${track.title}" to playlist`);
      onAdded();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to add track");
    } finally {
      setAddingId(null);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60" onClick={onClose}>
      <div
        className="bg-panel border border-panel2 rounded-lg w-full max-w-lg max-h-[80vh] flex flex-col shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between px-4 py-3 border-b border-panel2">
          <h2 className="text-lg font-medium">Add tracks to playlist</h2>
          <button onClick={onClose} className="p-1 text-muted hover:text-text" aria-label="Close">
            <XCircle size={20} />
          </button>
        </div>
        <div className="px-4 py-3 flex gap-2">
          <div className="relative flex-1">
            <Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted" />
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && search()}
              placeholder="Search tracks..."
              className="w-full bg-panel2 border border-panel2 rounded pl-8 pr-3 py-2 text-sm focus:outline-none focus:border-accent"
              autoFocus
              aria-label="Search tracks to add"
            />
          </div>
          <button
            onClick={search}
            disabled={searching || !query.trim()}
            className="px-4 py-2 bg-accent hover:opacity-90 text-black font-medium rounded text-sm disabled:opacity-50"
          >
            {searching ? "Searching..." : "Search"}
          </button>
        </div>
        <div className="flex-1 overflow-y-auto px-4 pb-4">
          {results.length === 0 && query.trim() && !searching && (
            <p className="text-muted text-sm text-center py-8">No tracks found</p>
          )}
          {results.length === 0 && !query.trim() && (
            <p className="text-muted text-sm text-center py-8">Search for tracks to add to this playlist</p>
          )}
          {searching && <p className="text-muted text-sm text-center py-8">Searching...</p>}
          <div className="space-y-1">
            {results.map((track) => (
              <div
                key={track.id}
                className="flex items-center gap-3 px-2 py-2 rounded hover:bg-panel2/50 group"
              >
                <div className="flex-1 min-w-0">
                  <div className="text-sm truncate">{track.title}</div>
                  <div className="text-xs text-muted truncate">{track.artist}</div>
                </div>
                <button
                  onClick={() => addTrack(track)}
                  disabled={addingId === track.id}
                  className="flex items-center gap-1 px-3 py-1 text-xs bg-accent/10 text-accent hover:bg-accent/20 rounded disabled:opacity-50"
                >
                  <Plus size={12} />
                  {addingId === track.id ? "Adding..." : "Add"}
                </button>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

function PlaylistTrackList({
  tracks,
  onRemove,
  player,
  onReorder,
}: {
  tracks: Track[];
  onRemove: (index: number) => void;
  player: ReturnType<typeof usePlayer>;
  onReorder?: (from: number, to: number) => void;
}) {
  const isMobile = useIsMobile();
  const [dragIndex, setDragIndex] = useState<number | null>(null);
  const [dropIndex, setDropIndex] = useState<number | null>(null);

  if (isMobile) {
    return (
      <div className="space-y-2">
        {tracks.map((t, i) => (
          <MobilePlaylistTrackCard
            key={`${t.id}-${i}`}
            track={t}
            index={i}
            tracks={tracks}
            onRemove={() => onRemove(t.position ?? i)}
            player={player}
            onReorder={onReorder}
            dragIndex={dragIndex}
            setDragIndex={setDragIndex}
            dropIndex={dropIndex}
            setDropIndex={setDropIndex}
          />
        ))}
      </div>
    );
  }
  return (
    <div className="rounded-lg overflow-hidden border border-panel2 group">
      <table className="w-full text-sm">
        <thead className="bg-panel2/60 text-muted">
          <tr>
            <th className="text-left px-4 py-2 w-10">#</th>
            <th className="text-left px-4 py-2">Title</th>
            <th className="text-left px-4 py-2">Artist</th>
            <th className="text-left px-4 py-2 hidden md:table-cell">Album</th>
            <th className="text-left px-4 py-2 w-16">Time</th>
            <th className="text-left px-4 py-2 w-10"></th>
          </tr>
        </thead>
        <tbody>
          {tracks.map((t, i) => (
            <DesktopPlaylistTrackRow
              key={`${t.id}-${i}`}
              track={t}
              index={i}
              tracks={tracks}
              onRemove={() => onRemove(t.position ?? i)}
              player={player}
              onReorder={onReorder}
              dragIndex={dragIndex}
              setDragIndex={setDragIndex}
              dropIndex={dropIndex}
              setDropIndex={setDropIndex}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function DesktopPlaylistTrackRow({
  track,
  index,
  tracks,
  onRemove,
  player,
  onReorder,
  dragIndex,
  setDragIndex,
  dropIndex,
  setDropIndex,
}: {
  track: Track;
  index: number;
  tracks: Track[];
  onRemove: () => void;
  player: ReturnType<typeof usePlayer>;
  onReorder?: (from: number, to: number) => void;
  dragIndex: number | null;
  setDragIndex: (i: number | null) => void;
  dropIndex: number | null;
  setDropIndex: (i: number | null) => void;
}) {
  const isCurrentTrack = player.current?.id === track.id;
  const isDragging = dragIndex === index;
  const isDropTarget = dropIndex === index;
  return (
    <tr
      draggable={!!onReorder}
      onDragStart={(e) => {
        if (!onReorder) return;
        setDragIndex(index);
        e.dataTransfer.effectAllowed = "move";
      }}
      onDragOver={(e) => {
        if (!onReorder || dragIndex === null) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
        setDropIndex(index);
      }}
      onDragLeave={() => {
        if (dropIndex === index) setDropIndex(null);
      }}
      onDrop={(e) => {
        e.preventDefault();
        if (onReorder && dragIndex !== null && dragIndex !== index) {
          onReorder(dragIndex, index);
        }
        setDragIndex(null);
        setDropIndex(null);
      }}
      onDragEnd={() => {
        setDragIndex(null);
        setDropIndex(null);
      }}
      onDoubleClick={() => player.play(tracks, index)}
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === "Enter") player.play(tracks, index);
      }}
      className={`border-t border-panel2 hover:bg-panel2/40 cursor-pointer group ${isCurrentTrack ? "bg-accent/10" : ""} ${isDragging ? "opacity-50" : ""} ${isDropTarget ? "border-t-2 border-t-accent" : ""}`}
    >
      <td className="px-4 py-2 text-muted">
        <button
          onClick={() => player.play(tracks, index)}
          className="opacity-0 group-hover:opacity-100 hover:text-accent"
          aria-label={`Play ${track.title}`}
        >
          <Play size={14} />
        </button>
        <span className="group-hover:hidden">{index + 1}</span>
      </td>
      <td className={`px-4 py-2 truncate ${isCurrentTrack ? "text-accent font-medium" : ""}`}>{track.title}</td>
      <td className="px-4 py-2 text-muted truncate">{track.artist}</td>
      <td className="px-4 py-2 text-muted truncate hidden md:table-cell">{track.album}</td>
      <td className="px-4 py-2 text-muted text-xs">
        {formatDuration(track.duration_sec)}
      </td>
      <td className="px-4 py-2">
        <button
          onClick={(e) => {
            e.stopPropagation();
            onRemove();
          }}
          className="opacity-0 group-hover:opacity-100 text-red-400 hover:text-red-300"
          aria-label={`Remove ${track.title} from playlist`}
        >
          <X size={14} />
        </button>
      </td>
    </tr>
  );
}

function MobilePlaylistTrackCard({
  track,
  index,
  tracks,
  onRemove,
  player,
  onReorder,
  dragIndex,
  setDragIndex,
  dropIndex,
  setDropIndex,
}: {
  track: Track;
  index: number;
  tracks: Track[];
  onRemove: () => void;
  player: ReturnType<typeof usePlayer>;
  onReorder?: (from: number, to: number) => void;
  dragIndex: number | null;
  setDragIndex: (i: number | null) => void;
  dropIndex: number | null;
  setDropIndex: (i: number | null) => void;
}) {
  const isCurrentTrack = player.current?.id === track.id;
  const isDragging = dragIndex === index;
  const isDropTarget = dropIndex === index;
  return (
    <div
      draggable={!!onReorder}
      onDragStart={(e) => {
        if (!onReorder) return;
        setDragIndex(index);
        e.dataTransfer.effectAllowed = "move";
      }}
      onDragOver={(e) => {
        if (!onReorder || dragIndex === null) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
        setDropIndex(index);
      }}
      onDragLeave={() => {
        if (dropIndex === index) setDropIndex(null);
      }}
      onDrop={(e) => {
        e.preventDefault();
        if (onReorder && dragIndex !== null && dragIndex !== index) {
          onReorder(dragIndex, index);
        }
        setDragIndex(null);
        setDropIndex(null);
      }}
      onDragEnd={() => {
        setDragIndex(null);
        setDropIndex(null);
      }}
      className={`bg-panel border border-panel2 rounded-lg p-3 flex items-center gap-3 active:bg-panel2/50 transition-colors ${isCurrentTrack ? "border-accent/50 bg-accent/5" : ""} ${isDragging ? "opacity-50" : ""} ${isDropTarget ? "border-accent border-2" : ""}`}
      onClick={() => player.play(tracks, index)}
    >
      <span className={`text-xs w-5 flex-shrink-0 ${isCurrentTrack ? "text-accent" : "text-muted"}`}>{index + 1}</span>
      <div className="flex-1 min-w-0">
        <div className={`truncate text-sm font-medium ${isCurrentTrack ? "text-accent" : ""}`}>{track.title}</div>
        <div className="truncate text-xs text-muted">
          {track.artist} {track.album && `— ${track.album}`}
        </div>
      </div>
      <span className="text-xs text-muted flex-shrink-0">
        {formatDuration(track.duration_sec)}
      </span>
      <button
        onClick={(e) => {
          e.stopPropagation();
          onRemove();
        }}
        className="w-9 h-9 flex items-center justify-center text-red-400 flex-shrink-0"
        aria-label={`Remove ${track.title} from playlist`}
      >
        <X size={18} />
      </button>
    </div>
  );
}
