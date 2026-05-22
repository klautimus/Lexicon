import { useEffect, useRef, useState, useMemo, useCallback } from "react";
import { Search, Music, Download, RefreshCw, HelpCircle, X, Play, Shuffle, Clock } from "lucide-react";
import { api, Track } from "../lib/api";
import { useToast } from "../contexts/ToastContext";
import { useHelp } from "../contexts/HelpContext";
import { useDownloads } from "../contexts/DownloadContext";
import { usePlayer } from "../player/PlayerContext";
import TrackList from "../components/TrackList";

const DEFAULT_PAGE_SIZE = 200;

type SortField = "title" | "artist" | "album" | "duration";
type SortDir = "asc" | "desc";

export default function MusicPage() {
  const toast = useToast();
  const { showHelp } = useHelp();
  const player = usePlayer();
  const { trackDownload } = useDownloads();
  const reqSeqRef = useRef(0);
  const [allTracks, setAllTracks] = useState<Track[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [downloading, setDownloading] = useState(false);
  const [upgrading, setUpgrading] = useState(false);
  const [upgradeProgress, setUpgradeProgress] = useState("");
  const [sortField, setSortField] = useState<SortField | null>(null);
  const [sortDir, setSortDir] = useState<SortDir>("asc");
  const [pageSize] = useState(DEFAULT_PAGE_SIZE);

  const hasMore = offset < total;

  const totalDuration = useMemo(() => {
    if (allTracks.length === 0) return null;
    const secs = allTracks.reduce((sum, t) => sum + (t.duration_sec || 0), 0);
    const h = Math.floor(secs / 3600);
    const m = Math.floor((secs % 3600) / 60);
    if (h > 0) return `${h}h ${m}m`;
    return `${m}m`;
  }, [allTracks]);

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
    setLoadError(null);
    setAllTracks([]);
    setTotal(0);
    setOffset(0);
    const seq = ++reqSeqRef.current;
    fetchPage("music", pageSize, 0, false)
      .catch((e: any) => {
        if (seq === reqSeqRef.current) {
          setLoadError(e?.message || "Failed to load tracks");
        }
      })
      .finally(() => {
        if (seq === reqSeqRef.current) {
          setLoading(false);
        }
      });
  }

  useEffect(() => {
    loadInitial();
  }, [pageSize]);

  function handleRefresh() {
    loadInitial();
  }

  async function handleLoadMore() {
    if (loadingMore || !hasMore) return;
    setLoadingMore(true);
    try {
      await fetchPage("music", pageSize, offset, true);
    } finally {
      setLoadingMore(false);
    }
  }

  const q = query.trim().toLowerCase();
  const filtered = useMemo(() => {
    const base = q
      ? allTracks.filter(
          (t) =>
            (t.title ?? "").toLowerCase().includes(q) ||
            (t.artist ?? "").toLowerCase().includes(q) ||
            (t.album ?? "").toLowerCase().includes(q)
        )
      : allTracks;

    if (sortField) {
      const sorted = [...base].sort((a, b) => {
        let va: string | number = "";
        let vb: string | number = "";
        switch (sortField) {
          case "title": va = a.title ?? ""; vb = b.title ?? ""; break;
          case "artist": va = a.artist ?? ""; vb = b.artist ?? ""; break;
          case "album": va = a.album ?? ""; vb = b.album ?? ""; break;
          case "duration": va = a.duration_sec || 0; vb = b.duration_sec || 0; break;
        }
        if (va < vb) return sortDir === "asc" ? -1 : 1;
        if (va > vb) return sortDir === "asc" ? 1 : -1;
        return 0;
      });
      return sorted;
    }
    return base;
  }, [allTracks, q, sortField, sortDir]);

  const handleSort = useCallback((field: SortField) => {
    const prevField = sortField;
    setSortField(field);
    if (prevField === field) {
      setSortDir((prevDir) => (prevDir === "asc" ? "desc" : "asc"));
    } else {
      setSortDir("asc");
    }
  }, [sortField]);

  async function handleDownloadSearch() {
    if (!query.trim()) return;
    setDownloading(true);
    const searchQuery = query.trim();
    try {
      const job = await api.downloadSearch(searchQuery);
      setDownloading(false);
      setQuery("");
      trackDownload(job, searchQuery);
    } catch {
      setDownloading(false);
      toast.error("Failed to start download");
    }
  }

  async function handleBulkUpgrade() {
    let allIds: number[] = [];
    let offset = 0;
    const batchSize = 1000;
    try {
      while (true) {
        const res = await api.tracks("music", batchSize, offset);
        allIds = allIds.concat(res.tracks.map((t) => t.id));
        if (res.tracks.length < batchSize) break;
        offset += batchSize;
      }
    } catch (e: any) {
      toast.error("Failed to fetch track list: " + (e.message || "unknown error"));
      return;
    }
    if (allIds.length === 0) return;
    if (!window.confirm(`Upgrade all ${allIds.length} tracks to Opus? This will take a long time.`)) return;
    setUpgrading(true);
    setUpgradeProgress("Starting...");
    let done = 0;
    let failed = 0;
    for (const id of allIds) {
      try {
        await api.upgradeTrack(id);
        done++;
        setUpgradeProgress(`Upgrading: ${done}/${allIds.length} (${failed} failed)`);
      } catch (e) {
        console.error(`[MusicPage] upgradeTrack ${id} failed:`, e);
        failed++;
        setUpgradeProgress(`Upgrading: ${done}/${allIds.length} (${failed} failed)`);
      }
      await new Promise((r) => setTimeout(r, 500));
    }
    setUpgrading(false);
    setUpgradeProgress("");
    toast.success(`Bulk upgrade complete: ${done} queued, ${failed} failed`);
  }

  function handlePlayAll() {
    if (filtered.length > 0) {
      player.play(filtered, 0);
    }
  }

  function handleShuffleAll() {
    if (filtered.length > 0) {
      const shuffled = [...filtered].sort(() => Math.random() - 0.5);
      player.play(shuffled, 0);
    }
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
          className="w-full bg-panel2 border border-panel2 rounded-md pl-9 pr-9 py-2 text-sm focus:outline-none focus:border-accent"
          aria-label="Filter tracks"
        />
        {query && (
          <button
            onClick={() => setQuery("")}
            className="absolute right-3 top-1/2 -translate-y-1/2 text-muted hover:text-text"
            aria-label="Clear filter"
          >
            <X size={14} />
          </button>
        )}
      </div>

      {q && (
        <p className="text-xs text-muted" role="status" aria-live="polite">
          {filtered.length} of {allTracks.length} loaded track{allTracks.length !== 1 ? "s" : ""} match "
          {query.trim()}"
        </p>
      )}

      {!q && !loading && !loadError && (
        <div className="flex items-center justify-between flex-wrap gap-2">
          <p className="text-xs text-muted">
            {total} track{total !== 1 ? "s" : ""} in library
            {totalDuration && (
              <span className="ml-1 inline-flex items-center gap-1">
                <Clock size={10} /> {totalDuration}
              </span>
            )}
          </p>
          <div className="flex items-center gap-2">
            <button
              onClick={handlePlayAll}
              disabled={filtered.length === 0}
              className="flex items-center gap-1.5 px-3 py-1 text-xs bg-accent/20 text-accent rounded hover:bg-accent/30 disabled:opacity-50"
            >
              <Play size={12} /> Play All
            </button>
            <button
              onClick={handleShuffleAll}
              disabled={filtered.length === 0}
              className="flex items-center gap-1.5 px-3 py-1 text-xs bg-accent/20 text-accent rounded hover:bg-accent/30 disabled:opacity-50"
            >
              <Shuffle size={12} /> Shuffle
            </button>
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
        </div>
      )}

      {loading ? (
        <div className="space-y-3" aria-busy="true">
          <div className="h-8 bg-panel2/40 rounded animate-pulse" />
          <div className="h-8 bg-panel2/40 rounded animate-pulse" />
          <div className="h-8 bg-panel2/40 rounded animate-pulse" />
          <div className="h-8 bg-panel2/40 rounded animate-pulse" />
          <div className="h-8 bg-panel2/40 rounded animate-pulse" />
        </div>
      ) : loadError ? (
        <div className="bg-panel2 border border-panel2 rounded-lg p-8 text-center space-y-4">
          <p className="text-red-400">{loadError}</p>
          <button
            onClick={handleRefresh}
            className="px-4 py-2 bg-accent hover:opacity-90 text-black font-medium rounded"
          >
            Retry
          </button>
        </div>
      ) : filtered.length > 0 ? (
        <>
          <TrackList
            tracks={filtered}
            onDelete={handleRefresh}
            sortField={sortField}
            sortDir={sortDir}
            onSort={handleSort}
            player={player}
          />
          {!q && hasMore && (
            <div className="flex justify-center pt-2">
              <button
                onClick={handleLoadMore}
                disabled={loadingMore}
                className="px-6 py-2 bg-accent hover:opacity-90 text-black font-medium rounded disabled:opacity-50"
                aria-busy={loadingMore}
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
        <div className="bg-panel2 border border-panel2 rounded-lg p-8 text-center space-y-4">
          <Music size={32} className="mx-auto text-muted" />
          <p className="text-muted">No tracks in your library yet.</p>
          <div className="flex items-center justify-center gap-3">
            <button
              onClick={() => showHelp("music.download")}
              className="px-4 py-2 bg-accent hover:opacity-90 text-black font-medium rounded flex items-center gap-2"
            >
              <Download size={16} /> Scan Library
            </button>
            <button
              onClick={() => showHelp("music.download")}
              className="text-xs text-muted hover:text-accent flex items-center gap-1 transition-colors"
            >
              <HelpCircle size={12} /> How does this work?
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
