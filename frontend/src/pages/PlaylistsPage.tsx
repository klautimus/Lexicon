import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Plus, ListMusic, Clock, Music, Trash2, HelpCircle, ArrowUpDown, Search } from "lucide-react";
import { api, Playlist } from "../lib/api";
import { useHelp } from "../contexts/HelpContext";
import { useToast } from "../contexts/ToastContext";

function formatDuration(sec: number) {
  const m = Math.floor(sec / 60);
  const h = Math.floor(m / 60);
  if (h > 0) return `${h}h ${m % 60}m`;
  return `${m}m`;
}

type SortMode = "newest" | "oldest" | "name" | "tracks";

export default function PlaylistsPage() {
  const { showHelp } = useHelp();
  const toast = useToast();
  const [playlists, setPlaylists] = useState<Playlist[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [deletingIds, setDeletingIds] = useState<Set<number>>(new Set());
  const [sortMode, setSortMode] = useState<SortMode>("newest");
  const [filterText, setFilterText] = useState("");

  async function load() {
    try {
      const data = await api.playlists();
      setPlaylists(data);
      setError(null);
    } catch (e) {
      console.error("[PlaylistsPage] failed to load playlists", e);
      setError(e instanceof Error ? e.message : "Failed to load playlists");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
  }, []);

  async function handleDeletePlaylist(id: number, name: string) {
    if (deletingIds.has(id)) return;
    setDeletingIds((prev) => new Set(prev).add(id));
    try {
      await api.deletePlaylist(id);
      setPlaylists((prev) => prev.filter((p) => p.id !== id));
      toast.success(`Deleted "${name}"`);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to delete playlist");
    } finally {
      setDeletingIds((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    }
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (!newName.trim()) return;
    setCreating(true);
    setError(null);
    try {
      const pl = await api.createPlaylist(newName.trim());
      setPlaylists((prev) => [pl, ...prev]);
      setNewName("");
      toast.success(`Created "${pl.name}"`);
    } catch (e) {
      console.error("[PlaylistsPage] failed to create playlist", e);
      const msg = e instanceof Error ? e.message : "Failed to create playlist";
      // C4: Handle 409 duplicate name
      if (msg.includes("409") || msg.includes("already exists")) {
        setError("A playlist with this name already exists.");
      } else {
        setError(msg);
      }
    } finally {
      setCreating(false);
    }
  }

  // F1: Sort and filter playlists
  const filteredPlaylists = playlists
    .filter((p) => p.name.toLowerCase().includes(filterText.toLowerCase()))
    .sort((a, b) => {
      switch (sortMode) {
        case "newest":
          return b.created_at - a.created_at;
        case "oldest":
          return a.created_at - b.created_at;
        case "name":
          return a.name.localeCompare(b.name);
        case "tracks":
          return b.track_count - a.track_count;
        default:
          return 0;
      }
    });

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

      {error && (
        <div className="text-sm text-red-400 bg-red-400/10 border border-red-400/30 rounded px-3 py-2">
          {error}
        </div>
      )}

      {loading ? (
        <p className="text-muted">Loading…</p>
      ) : playlists.length === 0 ? (
        <div className="text-center py-12 space-y-2">
          <ListMusic size={40} className="mx-auto text-muted" aria-hidden="true" />
          <p className="text-muted">No playlists yet.</p>
          <p className="text-xs text-muted">
            Create one above, or generate an AI playlist from the Discover page.
          </p>
        </div>
      ) : (
        <>
          {/* F1: Sort and filter controls */}
          <div className="flex gap-2 items-center">
            <div className="relative flex-1 max-w-xs">
              <Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted" />
              <input
                type="text"
                value={filterText}
                onChange={(e) => setFilterText(e.target.value)}
                placeholder="Filter playlists..."
                className="w-full bg-panel2 border border-panel2 rounded pl-8 pr-3 py-1.5 text-sm focus:outline-none focus:border-accent"
                aria-label="Filter playlists"
              />
            </div>
            <div className="flex items-center gap-1 text-xs text-muted">
              <ArrowUpDown size={14} />
              <select
                value={sortMode}
                onChange={(e) => setSortMode(e.target.value as SortMode)}
                className="bg-panel2 border border-panel2 rounded px-2 py-1.5 text-sm focus:outline-none focus:border-accent"
                aria-label="Sort playlists"
              >
                <option value="newest">Newest first</option>
                <option value="oldest">Oldest first</option>
                <option value="name">Name A–Z</option>
                <option value="tracks">Most tracks</option>
              </select>
            </div>
          </div>

          {filteredPlaylists.length === 0 ? (
            <p className="text-muted text-sm">No playlists match "{filterText}"</p>
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
              {filteredPlaylists.map((p) => (
                <div key={p.id} className="group relative">
                  <Link
                    to={`/playlists/${p.id}`}
                    className="bg-panel2 border border-panel2 rounded-lg p-4 hover:border-accent/50 transition block"
                  >
                    <div className="flex items-start justify-between mb-3">
                      <div className="w-10 h-10 rounded bg-panel flex items-center justify-center">
                        <Music size={20} className="text-accent" />
                      </div>
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
                  <button
                    onClick={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      handleDeletePlaylist(p.id, p.name);
                    }}
                    disabled={deletingIds.has(p.id)}
                    className="absolute top-3 right-3 text-red-400 hover:bg-red-400/10 rounded p-1.5 opacity-0 group-hover:opacity-100 transition disabled:opacity-50"
                    aria-label={`Delete ${p.name}`}
                    title="Delete playlist"
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}
