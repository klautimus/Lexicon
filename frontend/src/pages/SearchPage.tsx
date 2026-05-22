import { useEffect, useRef, useState, useCallback } from "react";
import { Music, Download, HelpCircle, Loader2, X, History } from "lucide-react";
import { api, Track } from "../lib/api";
import { useToast } from "../contexts/ToastContext";
import { useHelp } from "../contexts/HelpContext";
import { useDownloads } from "../contexts/DownloadContext";
import TrackList from "../components/TrackList";

const PAGE_SIZE = 200;
const SEARCH_HISTORY_KEY = "lexicon_search_history";
const MAX_HISTORY = 10;

function getSearchHistory(): string[] {
  try {
    const raw = localStorage.getItem(SEARCH_HISTORY_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
}

function addSearchHistory(query: string) {
  const q = query.trim();
  if (!q) return;
  const history = getSearchHistory().filter((h) => h !== q);
  history.unshift(q);
  localStorage.setItem(
    SEARCH_HISTORY_KEY,
    JSON.stringify(history.slice(0, MAX_HISTORY))
  );
}

export default function SearchPage() {
  const toast = useToast();
  const { showHelp } = useHelp();
  const { trackDownload } = useDownloads();

  // URL-synced query param
  const [q, setQ] = useState(() => {
    const params = new URLSearchParams(window.location.search);
    return params.get("q") || "";
  });
  const [results, setResults] = useState<Track[]>([]);
  const [searched, setSearched] = useState(false);
  const [loading, setLoading] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [resultCount, setResultCount] = useState(0);
  const [showHistory, setShowHistory] = useState(false);

  // Pagination state
  const [displayCount, setDisplayCount] = useState(PAGE_SIZE);
  const hasMore = displayCount < resultCount;

  // Refs for guarding against stale closures and unmount
  const mountedRef = useRef(true);
  const queryRef = useRef(q);
  const abortRef = useRef<AbortController | null>(null);
  const downloadRetryRef = useRef<number | null>(null);

  // Keep queryRef in sync
  useEffect(() => {
    queryRef.current = q;
  }, [q]);

  // Sync query to URL
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (q) {
      params.set("q", q);
    } else {
      params.delete("q");
    }
    const newUrl = `${window.location.pathname}${params.toString() ? "?" + params.toString() : ""}`;
    window.history.replaceState(null, "", newUrl);
  }, [q]);

  // Cleanup on unmount
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (abortRef.current) {
        abortRef.current.abort();
      }
      if (downloadRetryRef.current) {
        window.clearInterval(downloadRetryRef.current);
      }
    };
  }, []);

  // Reset pagination when results change
  useEffect(() => {
    setDisplayCount(PAGE_SIZE);
  }, [results]);

  // Hide history dropdown on outside click
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      const target = e.target as Node;
      const historyEl = document.getElementById("search-history-dropdown");
      const inputEl = document.getElementById("search-input");
      if (
        historyEl &&
        !historyEl.contains(target) &&
        inputEl &&
        !inputEl.contains(target)
      ) {
        setShowHistory(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  const safeSetResults = useCallback((tracks: Track[]) => {
    if (mountedRef.current) {
      setResults(tracks);
      setResultCount(tracks.length);
    }
  }, []);

  // Retry search after download: trigger scan, then poll for results
  const retrySearchAfterDownload = useCallback(
    (query: string) => {
      // Clear any existing retry interval
      if (downloadRetryRef.current) {
        window.clearInterval(downloadRetryRef.current);
        downloadRetryRef.current = null;
      }

      // Trigger a rescan so the backend indexes the new file
      api.scan().catch(() => {
        /* ignore scan trigger failure */
      });

      // Wait for scanner to start processing
      const startDelay = setTimeout(async () => {
        if (!mountedRef.current) return;

        let found = false;
        // Retry for up to 3 minutes (60 attempts × 3s)
        let attempt = 0;
        const interval = window.setInterval(async () => {
          if (!mountedRef.current) {
            window.clearInterval(interval);
            return;
          }
          attempt++;
          if (attempt > 60) {
            window.clearInterval(interval);
            downloadRetryRef.current = null;
            return;
          }
          try {
            const tracks = await api.search(query);
            if (tracks.length > 0 && mountedRef.current) {
              window.clearInterval(interval);
              downloadRetryRef.current = null;
              safeSetResults(tracks);
              setSearched(true);
              found = true;
            }
          } catch {
            /* continue retrying */
          }
        }, 3000);

        downloadRetryRef.current = interval;
      }, 3000);

      // Store timeout so we can clean up
      const timeoutId = window.setTimeout(async () => {
        // The interval above handles the actual retry; this is just the initial delay
      }, 0);

      // We don't need the timeout ref since the interval handles cleanup
      void timeoutId;
    },
    [safeSetResults]
  );

  async function go(e: React.FormEvent) {
    e.preventDefault();
    if (!q.trim() || loading) return;

    setShowHistory(false);
    addSearchHistory(q.trim());

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
      // Differentiate error types
      const msg = e instanceof Error ? e.message : "Search failed";
      if (msg.includes("Unable to reach the server") || msg.includes("Network error")) {
        toast.error("Cannot connect to server. Check your connection.");
      } else if (msg.includes("500") || msg.includes("502") || msg.includes("503")) {
        toast.error("Server error. Please try again later.");
      } else {
        toast.error(msg);
      }
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
      // Start retry search after download to pick up the new track
      retrySearchAfterDownload(q.trim());
    } catch (e) {
      setDownloading(false);
      const msg = e instanceof Error ? e.message : "Failed to start download";
      if (msg.includes("Unable to reach the server") || msg.includes("Network error")) {
        toast.error("Cannot connect to server. Check your connection.");
      } else {
        toast.error(msg);
      }
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

  const handleClear = () => {
    setQ("");
    setResults([]);
    setSearched(false);
    setResultCount(0);
    setDisplayCount(PAGE_SIZE);
  };

  const handleHistorySelect = (query: string) => {
    setQ(query);
    setShowHistory(false);
  };

  const handleHistoryClear = () => {
    localStorage.removeItem(SEARCH_HISTORY_KEY);
    setShowHistory(false);
  };

  const history = getSearchHistory();
  const showHistoryDropdown = showHistory && history.length > 0;

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
        <div className="flex-1 relative">
          <input
            id="search-input"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onFocus={() => setShowHistory(true)}
            placeholder="Search title, artist, album, genre…"
            className="w-full bg-panel border border-panel2 rounded-md px-3 py-2 pr-16 outline-none focus:border-accent"
            aria-label="Search"
            autoComplete="off"
          />
          {q && (
            <button
              type="button"
              onClick={handleClear}
              className="absolute right-10 top-1/2 -translate-y-1/2 p-1 text-muted hover:text-text"
              aria-label="Clear search"
            >
              <X size={14} />
            </button>
          )}
          {history.length > 0 && (
            <button
              type="button"
              onClick={() => setShowHistory(!showHistory)}
              className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-muted hover:text-text"
              aria-label="Search history"
            >
              <History size={14} />
            </button>
          )}
          {showHistoryDropdown && (
            <div
              id="search-history-dropdown"
              className="absolute top-full left-0 right-0 mt-1 bg-panel border border-panel2 rounded-lg shadow-lg z-30 max-h-60 overflow-y-auto"
            >
              <div className="flex items-center justify-between px-3 py-2 border-b border-panel2">
                <span className="text-xs text-muted uppercase tracking-wide">
                  Recent Searches
                </span>
                <button
                  onClick={handleHistoryClear}
                  className="text-xs text-muted hover:text-red-400"
                >
                  Clear
                </button>
              </div>
              {history.map((h, i) => (
                <button
                  key={`${h}-${i}`}
                  type="button"
                  onClick={() => handleHistorySelect(h)}
                  className="w-full text-left px-3 py-2 text-sm hover:bg-panel2 flex items-center gap-2"
                >
                  <History size={12} className="text-muted" />
                  <span className="truncate">{h}</span>
                </button>
              ))}
            </div>
          )}
        </div>
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
