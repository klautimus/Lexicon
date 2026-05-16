import { useEffect, useRef, useState } from "react";
import { Music, Download } from "lucide-react";
import { api, Track, DownloadJob } from "../lib/api";
import { useToast } from "../contexts/ToastContext";
import TrackList from "../components/TrackList";

export default function SearchPage() {
  const toast = useToast();
  const pollRef = useRef<Record<string, number>>({});
  const [q, setQ] = useState("");
  const [results, setResults] = useState<Track[]>([]);
  const [searched, setSearched] = useState(false);
  const [downloading, setDownloading] = useState(false);

  useEffect(() => {
    return () => {
      Object.values(pollRef.current).forEach(clearInterval);
    };
  }, []);

  async function go(e: React.FormEvent) {
    e.preventDefault();
    if (!q.trim()) return;
    setSearched(true);
    const r = await api.search(q.trim());
    setResults(r);
  }

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
          if (q.trim()) api.search(q.trim()).then(setResults);
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
    if (!q.trim()) return;
    setDownloading(true);
    try {
      const job = await api.downloadSearch(q.trim());
      setDownloading(false);
      trackDownload(job, q.trim());
    } catch {
      setDownloading(false);
      toast.error("Failed to start download");
    }
  }

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold">Search</h1>
      <form onSubmit={go} className="flex gap-2">
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Search title, artist, album, genre…"
          className="flex-1 bg-panel border border-panel2 rounded-md px-3 py-2 outline-none focus:border-accent"
        />
        <button className="px-4 py-2 bg-accent text-bg rounded-md font-medium">Search</button>
      </form>

      {results.length > 0 ? (
        <TrackList tracks={results} onDelete={() => api.search(q.trim()).then(setResults)} />
      ) : searched && q.trim() ? (
        <div className="bg-panel2 border border-panel2 rounded-lg p-8 text-center space-y-4">
          <Music size={32} className="mx-auto text-muted" />
          <div>
            <p className="text-muted">
              No results for "<span className="text-text font-medium">{q.trim()}</span>"
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
      ) : null}
    </div>
  );
}
