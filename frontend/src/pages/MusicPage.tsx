import { useEffect, useRef, useState } from "react";
import { Search, Music, Download, RefreshCw, HelpCircle } from "lucide-react";
import { api, Track, DownloadJob } from "../lib/api";
import { useToast } from "../contexts/ToastContext";
import { useHelp } from "../contexts/HelpContext";
import TrackList from "../components/TrackList";

const PAGE_SIZE = 200;

export default function MusicPage() {
  const toast = useToast();
  const { showHelp } = useHelp();
  const pollRef = useRef<Record<string, number>>({});
  const [allTracks, setAllTracks] = useState<Track[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [query, setQuery] = useState("");
  const [downloading, setDownloading] = useState(false);
  const [upgrading, setUpgrading] = useState(false);
  const [upgradeProgress, setUpgradeProgress] = useState("");

  const hasMore = offset < total;

  function fetchPage(
    kind: string | undefined,
    limit: number,
    off: number,
    append: boolean
  ) {
    const p = append
      ? api.tracks(kind, limit, off).then((res) => {
          setAllTracks((prev) => [...prev, ...res.tracks]);
          setTotal(res.total);
          setOffset(off + limit);
        })
      : api.tracks(kind, limit, off).then((res) => {
          setAllTracks(res.tracks);
          setTotal(res.total);
          setOffset(off + limit);
        });
    return p;
  }

  function loadInitial() {
    setLoading(true);
    setAllTracks([]);
    setTotal(0);
    setOffset(0);
    fetchPage("music", PAGE_SIZE, 0, false).finally(() => setLoading(false));
  }

  useEffect(() => {
    loadInitial();
  }, []);

  useEffect(() => {
    return () => {
      Object.values(pollRef.current).forEach(clearInterval);
    };
  }, []);

  function handleRefresh() {
    loadInitial();
  }

  async function handleLoadMore() {
    if (loadingMore || !hasMore) return;
    setLoadingMore(true);
    try {
      await fetchPage("music", PAGE_SIZE, offset, true);
    } finally {
      setLoadingMore(false);
    }
  }

  const q = query.trim().toLowerCase();
  const filtered = q
    ? allTracks.filter(
        (t) =>
          (t.title ?? "").toLowerCase().includes(q) ||
          (t.artist ?? "").toLowerCase().includes(q) ||
          (t.album ?? "").toLowerCase().includes(q)
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
          handleRefresh();
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

  async function handleBulkUpgrade() {
    if (allTracks.length === 0) return;
    const confirmed = window.confirm(
      `Upgrade all ${allTracks.length} tracks to higher quality (Opus/bestaudio)?\n\nThis will re-download each track from YouTube using the new pipeline. Existing files will be replaced.`
    );
    if (!confirmed) return;
    setUpgrading(true);
    setUpgradeProgress("Starting...");
    let done = 0;
    let failed = 0;
    for (const t of allTracks) {
      try {
        await api.upgradeTrack(t.id);
        done++;
        setUpgradeProgress(`Upgrading: ${done}/${allTracks.length} (${failed} failed)`);
      } catch {
        failed++;
        setUpgradeProgress(`Upgrading: ${done}/${allTracks.length} (${failed} failed)`);
      }
      await new Promise((r) => setTimeout(r, 500));
    }
    setUpgrading(false);
    setUpgradeProgress("");
    toast.success(`Bulk upgrade complete: ${done} queued, ${failed} failed`);
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <h1 className="text-2xl font-semibold">Music</h1>
        <button
          onClick={() => showHelp("music.library")}
          className="p-1 text-muted/50 hover:text-accent transition-colors rounded hover:bg-panel2/50"
          aria-label="Help: Music Library"
        >
          <HelpCircle size={16} />
        </button>
      </div>

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
          {filtered.length} of {total} track{total !== 1 ? "s" : ""} match "
          {query.trim()}"
        </p>
      )}

      {!q && !loading && (
        <div className="flex items-center justify-between">
          <p className="text-xs text-muted">
            {total} track{total !== 1 ? "s" : ""} in library
          </p>
          {total > 0 && (
            <button
              onClick={handleBulkUpgrade}
              disabled={upgrading}
              className="flex items-center gap-1.5 px-3 py-1 text-xs bg-yellow-500/20 text-yellow-400 rounded hover:bg-yellow-500/30 disabled:opacity-50"
            >
              <RefreshCw size={12} className={upgrading ? "animate-spin" : ""} />
              {upgrading ? upgradeProgress || "Upgrading..." : "Upgrade All to Opus"}
            </button>
          )}
        </div>
      )}

      {loading ? (
        <p className="text-muted">Loading…</p>
      ) : filtered.length > 0 ? (
        <>
          <TrackList
            tracks={filtered}
            onDelete={handleRefresh}
          />
          {!q && hasMore && (
            <div className="flex justify-center pt-2">
              <button
                onClick={handleLoadMore}
                disabled={loadingMore}
                className="px-6 py-2 bg-accent hover:opacity-90 text-black font-medium rounded disabled:opacity-50"
              >
                {loadingMore ? "Loading…" : "Load More"}
              </button>
            </div>
          )}
        </>
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
          <button
            onClick={() => showHelp("music.download")}
            className="text-xs text-muted hover:text-accent flex items-center gap-1 mx-auto transition-colors"
          >
            <HelpCircle size={12} /> How does this work?
          </button>
        </div>
      ) : (
        <p className="text-muted">No tracks.</p>
      )}
    </div>
  );
}
