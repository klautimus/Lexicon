import { useEffect, useRef, useState } from "react";
import { Search, Music, Download } from "lucide-react";
import { api, Track, DownloadJob } from "../lib/api";
import { useToast } from "../contexts/ToastContext";
import TrackList from "../components/TrackList";

export default function MusicPage() {
  const toast = useToast();
  const pollRef = useRef<Record<string, number>>({});
  const [allTracks, setAllTracks] = useState<Track[]>([]);
  const [loading, setLoading] = useState(true);
  const [query, setQuery] = useState("");
  const [downloading, setDownloading] = useState(false);

  useEffect(() => {
    api
      .tracks("music", 500)
      .then(setAllTracks)
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    return () => {
      Object.values(pollRef.current).forEach(clearInterval);
    };
  }, []);

  const q = query.trim().toLowerCase();
  const filtered = q
    ? allTracks.filter(
        (t) =>
          t.title.toLowerCase().includes(q) ||
          t.artist.toLowerCase().includes(q) ||
          t.album.toLowerCase().includes(q)
      )
    : allTracks;

  function trackDownload(job: DownloadJob, name: string) {
    if (job.status === "succeeded") {
      toast.success(`"${name}" downloaded successfully!`);
      return;
    }
    if (job.status === "failed") {
      toast.error(`Download failed: ${job.error || "Unknown error"}`);
      return;
    }
    if (job.status === "cancelled") {
      toast.info(`Download cancelled for "${name}"`);
      return;
    }

    toast.info(`Download started for "${name}"`);
    const interval = window.setInterval(async () => {
      try {
        const updated = await api.downloadJob(job.id);
        if (updated.status === "succeeded") {
          window.clearInterval(interval);
          delete pollRef.current[job.id];
          toast.success(`"${name}" downloaded successfully!`);
          api.tracks("music", 500).then(setAllTracks);
        } else if (updated.status === "failed") {
          window.clearInterval(interval);
          delete pollRef.current[job.id];
          toast.error(`Download failed: ${updated.error || "Unknown error"}`);
        } else if (updated.status === "cancelled") {
          window.clearInterval(interval);
          delete pollRef.current[job.id];
          toast.info(`Download cancelled for "${name}"`);
        }
      } catch {
        window.clearInterval(interval);
        delete pollRef.current[job.id];
        toast.error(`Lost connection tracking download`);
      }
    }, 2000);
    pollRef.current[job.id] = interval;
  }

  async function handleDownloadSearch() {
    if (!query.trim()) return;
    setDownloading(true);
    try {
      const job = await api.downloadSearch(query.trim());
      setDownloading(false);
      trackDownload(job, query.trim());
    } catch {
      setDownloading(false);
      toast.error("Failed to start download");
    }
  }

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold">Music</h1>

      <div className="relative">
        <Search
          size={16}
          className="absolute left-3 top-1/2 -translate-y-1/2 text-muted"
        />
        <input
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Filter by title, artist, or album…"
          className="w-full bg-panel2 border border-panel2 rounded-md pl-9 pr-3 py-2 text-sm focus:outline-none focus:border-accent"
        />
      </div>

      {q && (
        <p className="text-xs text-muted">
          {filtered.length} result{filtered.length !== 1 ? "s" : ""} for "{query.trim()}"
        </p>
      )}

      {loading ? (
        <p className="text-muted">Loading…</p>
      ) : filtered.length > 0 ? (
        <TrackList tracks={filtered} onDelete={() => api.tracks("music", 500).then(setAllTracks)} />
      ) : q ? (
        <div className="bg-panel2 border border-panel2 rounded-lg p-8 text-center space-y-4">
          <Music size={32} className="mx-auto text-muted" />
          <div>
            <p className="text-muted">
              No local results for "<span className="text-text font-medium">{query.trim()}</span>"
            </p>
            <p className="text-xs text-muted mt-1">
              This track isn't in your library yet.
            </p>
          </div>
          <button
            onClick={handleDownloadSearch}
            disabled={downloading}
            className="px-4 py-2 bg-accent hover:opacity-90 text-black font-medium rounded flex items-center gap-2 mx-auto disabled:opacity-50"
          >
            {downloading ? (
              <span className="animate-spin">⟳</span>
            ) : (
              <Download size={16} />
            )}
            Search & Download from Web
          </button>
          <p className="text-xs text-muted">
            DeepSeek finds metadata → yt-dlp downloads audio
          </p>
        </div>
      ) : (
        <p className="text-muted">No tracks.</p>
      )}
    </div>
  );
}
