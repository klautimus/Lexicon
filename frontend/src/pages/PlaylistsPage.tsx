import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Plus, ListMusic, Clock, Music, Trash2, HelpCircle } from "lucide-react";
import { api, Playlist } from "../lib/api";
import { useHelp } from "../contexts/HelpContext";

function formatDuration(sec: number) {
  const m = Math.floor(sec / 60);
  const h = Math.floor(m / 60);
  if (h > 0) return `${h}h ${m % 60}m`;
  return `${m}m`;
}

export default function PlaylistsPage() {
  const { showHelp } = useHelp();
  const [playlists, setPlaylists] = useState<Playlist[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");

  async function load() {
    try {
      const data = await api.playlists();
      setPlaylists(data);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
  }, []);

  async function handleDeletePlaylist(id: number, name: string) {
    if (!confirm(`Delete "${name}"?`)) return;
    try {
      await api.deletePlaylist(id);
      load();
    } catch {
      // ignore
    }
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (!newName.trim()) return;
    setCreating(true);
    try {
      await api.createPlaylist(newName.trim());
      setNewName("");
      load();
    } catch {
      // ignore
    } finally {
      setCreating(false);
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <h1 className="text-2xl font-semibold">Playlists</h1>
          <button
            onClick={() => showHelp("playlists.grid")}
            className="p-1 text-muted/50 hover:text-accent transition-colors rounded hover:bg-panel2/50"
            aria-label="Help: Playlists"
          >
            <HelpCircle size={16} />
          </button>
        </div>
      </div>

      <form onSubmit={create} className="flex gap-2">
        <input
          type="text"
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
          placeholder="New playlist name..."
          className="flex-1 bg-panel2 border border-panel2 rounded px-3 py-2 text-sm focus:outline-none focus:border-accent"
          disabled={creating}
        />
        <button
          type="submit"
          disabled={creating || !newName.trim()}
          className="px-4 py-2 bg-accent hover:opacity-90 text-black font-medium rounded flex items-center gap-2 disabled:opacity-50"
        >
          <Plus size={16} />
          Create
        </button>
      </form>
      <button
        onClick={() => showHelp("playlists.create")}
        className="text-xs text-muted hover:text-accent flex items-center gap-1 transition-colors"
      >
        <HelpCircle size={12} /> How do playlists work?
      </button>

      {loading ? (
        <p className="text-muted">Loading…</p>
      ) : playlists.length === 0 ? (
        <div className="text-center py-12 space-y-2">
          <ListMusic size={40} className="mx-auto text-muted" />
          <p className="text-muted">No playlists yet.</p>
          <p className="text-xs text-muted">
            Create one above, or generate an AI playlist from the Discover page.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {playlists.map((p) => (
            <Link
              key={p.id}
              to={`/playlists/${p.id}`}
              className="bg-panel2 border border-panel2 rounded-lg p-4 hover:border-accent/50 transition group"
            >
              <div className="flex items-start justify-between mb-3">
                <div className="w-10 h-10 rounded bg-panel flex items-center justify-center">
                  <Music size={20} className="text-accent" />
                </div>
                <button
                  onClick={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    handleDeletePlaylist(p.id, p.name);
                  }}
                  className="text-red-400 hover:bg-red-400/10 rounded p-1.5 opacity-100 md:opacity-0 md:group-hover:opacity-100 transition"
                  title="Delete playlist"
                >
                  <Trash2 size={16} />
                </button>
              </div>
              <h3 className="font-medium truncate group-hover:text-accent transition">
                {p.name}
              </h3>
              <div className="flex items-center gap-3 mt-2 text-xs text-muted">
                <span className="flex items-center gap-1">
                  <ListMusic size={12} />
                  {p.track_count} tracks
                </span>
                {p.total_duration > 0 && (
                  <span className="flex items-center gap-1">
                    <Clock size={12} />
                    {formatDuration(p.total_duration)}
                  </span>
                )}
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
