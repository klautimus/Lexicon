import { useEffect, useRef, useState, useCallback } from "react";
import { Music, Download, HelpCircle, Loader2 } from "lucide-react";
import { api, Track } from "../lib/api";
import { useToast } from "../contexts/ToastContext";
import { useHelp } from "../contexts/HelpContext";
import { useDownloads } from "../contexts/DownloadContext";
import TrackList from "../components/TrackList";

const PAGE_SIZE = 200;

export default function SearchPage() {
  const toast = useToast();
  const { showHelp } = useHelp();
  const { trackDownload } = useDownloads();

  const [q, setQ] = useState("");
  const [results, setResults] = useState<Track[]>([]);
  const [searched, setSearched] = useState(false);
  const [loading, setLoading] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [resultCount, setResultCount] = useState(0);

  // Pagination state
  const [displayCount, setDisplayCount] = useState(PAGE_SIZE);
  const hasMore = displayCount < resultCount;

  // Refs for guarding against stale closures and unmount
  const mountedRef = useRef(true);
  const queryRef = useRef(q);
  const abortRef = useRef<AbortController | null>(null);

  // Keep queryRef in sync
  useEffect(() => {
    queryRef.current = q;
  }, [q]);

  // Cleanup on unmount
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (abortRef.current) {
        abortRef.current.abort();
      }
    };
  }, []);

  // Reset pagination when results change
  useEffect(() => {
    setDisplayCount(PAGE_SIZE);
  }, [results]);

  const safeSetResults = useCallback((tracks: Track[]) => {
    if (mountedRef.current) {
      setResults(tracks);
      setResultCount(tracks.length);
    }
  }, []);

  async function go(e: React.FormEvent) {
    e.preventDefault();
    if (!q.trim() || loading) return;

    // Cancel any in-flight search
    if (abortRef.current) {
      abortRef.current.abort();
    }
    abortRef.current = new AbortController();

    setSearched(true);
    setLoading(true);
    setDownloading(false);
    try {
      const r = await api.search(q.trim(), { signal: abortRef.current.signal });
      safeSetResults(r);
    } catch (e: any) {
      if (e?.message === "Request was cancelled.") return;
      toast.error(e instanceof Error ? e.message : "Search failed");
    } finally {
      if (mountedRef.current) {
        setLoading(false);
      }
    }
  }

  async function handleDownloadSearch() {
    if (!q.trim() || downloading) return;
    setDownloading(true);
    try {
      const job = await api.downloadSearch(q.trim());
      setDownloading(false);
      // Use DownloadContext's trackDownload which includes retry logic
      trackDownload(job, q.trim());
    } catch (e) {
      setDownloading(false);
      toast.error(e instanceof Error ? e.message : "Failed to start download");
    }
  }

  // Stale closure fix: read query from ref
  const handleDelete = useCallback(() => {
    const currentQuery = queryRef.current;
    if (currentQuery.trim()) {
      api.search(currentQuery.trim()).then(safeSetResults);
    }
  }, [safeSetResults]);

  const handleLoadMore = () => {
    setDisplayCount((prev) => prev + PAGE_SIZE);
  };

  const displayedResults = results.slice(0, displayCount);

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <h1 className="text-2xl font-semibold">Search</h1>
        <button
          onClick={() => showHelp("search.main")}
          className="p-1 text-muted/50 hover:text-accent transition-colors rounded hover:bg-panel2/50"
          aria-label="Help: Search"
        >
          <HelpCircle size={16} />
        </button>
      </div>
      <form onSubmit={go} className="flex gap-2" role="search">
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Search title, artist, album, genre…"
          className="flex-1 bg-panel border border-panel2 rounded-md px-3 py-2 outline-none focus:border-accent"
          aria-label="Search"
        />
        <button className="px-4 py-2 bg-accent text-bg rounded-md font-medium" disabled={loading}>
          {loading ? "Searching…" : "Search"}
        </button>
      </form>

      {loading ? (
        <div className="bg-panel rounded-lg p-8 border border-panel2 text-center">
          <p className="text-muted">Searching…</p>
        </div>
      ) : results.length > 0 ? (
        <div aria-live="polite" aria-atomic="true">
          <p className="text-sm text-muted mb-2">
            {resultCount} result{resultCount !== 1 ? "s" : ""} for "{q.trim()}"
          </p>
          <TrackList tracks={displayedResults} onDelete={handleDelete} />
          {hasMore && (
            <div className="mt-4 text-center">
              <button
                onClick={handleLoadMore}
                className="px-4 py-2 bg-panel2 hover:bg-panel text-text rounded-md font-medium transition-colors"
              >
                Load More ({resultCount - displayCount} remaining)
              </button>
            </div>
          )}
        </div>
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
              <Loader2 size={16} className="animate-spin" />
            ) : (
              <Download size={16} />
            )}
            Search & Download from Web
          </button>
          <button
            onClick={() => showHelp("search.download")}
            className="text-xs text-muted hover:text-accent flex items-center gap-1 mx-auto transition-colors"
          >
            <HelpCircle size={12} /> How does this work?
          </button>
        </div>
      ) : null}
    </div>
  );
}
