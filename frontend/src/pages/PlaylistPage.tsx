import { useEffect, useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
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
} from "lucide-react";
import { api, PlaylistWithTracks, Track } from "../lib/api";
import { usePlayer } from "../player/PlayerContext";
import { useIsMobile } from "../hooks/useIsMobile";

function formatDuration(sec: number) {
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}

export default function PlaylistPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const player = usePlayer();
  const [playlist, setPlaylist] = useState<PlaylistWithTracks | null>(null);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState(false);
  const [editName, setEditName] = useState("");

  async function load() {
    if (!id) return;
    try {
      const data = await api.playlist(Number(id));
      setPlaylist(data);
      setEditName(data.name);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  async function remove(pos: number) {
    if (!id) return;
    try {
      await api.removeFromPlaylist(Number(id), pos);
      load();
    } catch {
      // ignore
    }
  }

  async function saveName() {
    if (!id || !editName.trim()) return;
    try {
      await api.updatePlaylist(Number(id), editName.trim());
      setEditing(false);
      load();
    } catch {
      // ignore
    }
  }

  async function deletePlaylist() {
    if (!id) return;
    if (!confirm("Delete this playlist? This cannot be undone.")) return;
    try {
      await api.deletePlaylist(Number(id));
      navigate("/playlists");
    } catch {
      // ignore
    }
  }

  if (loading) {
    return <p className="text-muted">Loading…</p>;
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
        <p className="text-muted">Playlist not found.</p>
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

      <div className="flex items-start justify-between gap-4">
        <div className="flex-1">
          {editing ? (
            <div className="flex items-center gap-2">
              <input
                type="text"
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                className="bg-panel2 border border-panel2 rounded px-3 py-1.5 text-lg font-semibold focus:outline-none focus:border-accent"
                autoFocus
              />
              <button
                onClick={saveName}
                className="p-1.5 text-green-400 hover:bg-panel2 rounded"
              >
                <Check size={18} />
              </button>
              <button
                onClick={() => {
                  setEditing(false);
                  setEditName(playlist.name);
                }}
                className="p-1.5 text-muted hover:bg-panel2 rounded"
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
        </div>
        <div className="flex items-center gap-2">
          {playlist.tracks.length > 0 && (
            <button
              onClick={() => player.play(playlist.tracks, 0)}
              className="px-4 py-2 bg-accent hover:opacity-90 text-black font-medium rounded flex items-center gap-2"
            >
              <Play size={16} />
              Play All
            </button>
          )}
          <button
            onClick={deletePlaylist}
            className="p-2 text-red-400 hover:bg-red-400/10 rounded"
            title="Delete playlist"
          >
            <Trash2 size={16} />
          </button>
        </div>
      </div>

      {playlist.tracks.length === 0 ? (
        <div className="text-center py-12 space-y-2">
          <Music size={40} className="mx-auto text-muted" />
          <p className="text-muted">No tracks yet.</p>
          <p className="text-xs text-muted">
            Browse your library and add tracks to this playlist.
          </p>
        </div>
      ) : (
        <PlaylistTrackList tracks={playlist.tracks} onRemove={(pos) => remove(pos)} />
      )}
    </div>
  );
}

function PlaylistTrackList({
  tracks,
  onRemove,
}: {
  tracks: Track[];
  onRemove: (index: number) => void;
}) {
  const isMobile = useIsMobile();
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
          />
        ))}
      </div>
    );
  }
  return (
    <div className="rounded-lg overflow-hidden border border-panel2">
      <table className="w-full text-sm">
        <thead className="bg-panel2/60 text-muted">
          <tr>
            <th className="text-left px-4 py-2 w-10">#</th>
            <th className="text-left px-4 py-2">Title</th>
            <th className="text-left px-4 py-2">Artist</th>
            <th className="text-left px-4 py-2">Album</th>
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
}: {
  track: Track;
  index: number;
  tracks: Track[];
  onRemove: () => void;
}) {
  const player = usePlayer();
  return (
    <tr
      onDoubleClick={() => player.play(tracks, index)}
      className="border-t border-panel2 hover:bg-panel2/40 cursor-pointer group"
    >
      <td className="px-4 py-2 text-muted">
        <button
          onClick={() => player.play(tracks, index)}
          className="opacity-0 group-hover:opacity-100 hover:text-accent"
        >
          <Play size={14} />
        </button>
        <span className="group-hover:hidden">{index + 1}</span>
      </td>
      <td className="px-4 py-2 truncate">{track.title}</td>
      <td className="px-4 py-2 text-muted truncate">{track.artist}</td>
      <td className="px-4 py-2 text-muted truncate">{track.album}</td>
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
          title="Remove from playlist"
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
}: {
  track: Track;
  index: number;
  tracks: Track[];
  onRemove: () => void;
}) {
  const player = usePlayer();
  return (
    <div
      className="bg-panel border border-panel2 rounded-lg p-3 flex items-center gap-3 active:bg-panel2/50 transition-colors"
      onClick={() => player.play(tracks, index)}
    >
      <span className="text-xs text-muted w-5 flex-shrink-0">{index + 1}</span>
      <div className="flex-1 min-w-0">
        <div className="truncate text-sm font-medium">{track.title}</div>
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
        title="Remove from playlist"
      >
        <X size={18} />
      </button>
    </div>
  );
}
