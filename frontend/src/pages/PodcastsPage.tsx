import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import {
  Rss, Plus, RefreshCw, Download, Play, Trash2, Check, Loader2, Search, X, MessageSquare, ChevronDown, HelpCircle, AlertCircle, MoreHorizontal, ListMusic, ArrowUpDown, Filter, Eye, EyeOff
} from "lucide-react";
import { api, PodcastFeed, PodcastEpisode, Track, Playlist } from "../lib/api";
import { useToast } from "../contexts/ToastContext";
import { usePlayer } from "../player/PlayerContext";
import { useHelp } from "../contexts/HelpContext";
import { useIsMobile } from "../hooks/useIsMobile";

type SortField = "date" | "duration" | "title";
type SortDir = "asc" | "desc";
type FilterMode = "all" | "downloaded" | "not_downloaded" | "listened" | "not_listened";

export default function PodcastsPage() {
  const toast = useToast();
  const player = usePlayer();
  const { showHelp } = useHelp();
  const isMobile = useIsMobile();
  const [feeds, setFeeds] = useState<PodcastFeed[]>([]);
  const [selectedFeed, setSelectedFeed] = useState<PodcastFeed | null>(null);
  const [episodes, setEpisodes] = useState<PodcastEpisode[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showAddModal, setShowAddModal] = useState(false);
  const [downloadingIds, setDownloadingIds] = useState<Set<number>>(new Set());
  const [showDownloadConfirm, setShowDownloadConfirm] = useState(false);
  const [feedToDownload, setFeedToDownload] = useState<number | null>(null);
  const [subscribing, setSubscribing] = useState(false);
  const pollIntervals = useRef<Map<number, ReturnType<typeof setInterval>>>(new Map());
  const selectedFeedRef = useRef<PodcastFeed | null>(null);
  selectedFeedRef.current = selectedFeed;

  // Phase 2: Sorting, filtering, search
  const [sortField, setSortField] = useState<SortField>("date");
  const [sortDir, setSortDir] = useState<SortDir>("desc");
  const [filterMode, setFilterMode] = useState<FilterMode>("all");
  const [searchText, setSearchText] = useState("");
  const [showSortMenu, setShowSortMenu] = useState(false);
  const [showFilterMenu, setShowFilterMenu] = useState(false);

  // Cleanup all polling intervals + downloading state on unmount
  useEffect(() => {
    return () => {
      pollIntervals.current.forEach((interval) => clearInterval(interval));
      pollIntervals.current.clear();
      setDownloadingIds(new Set());
    };
  }, []);

  const loadFeeds = useCallback(async () => {
    try {
      const f = await api.podcastFeeds();
      setFeeds(f);
      setError(null);
      const current = selectedFeedRef.current;
      if (f.length > 0 && !current) {
        setSelectedFeed(f[0]);
      } else if (current) {
        const updated = f.find((x) => x.id === current.id);
        if (updated) setSelectedFeed(updated);
      }
    } catch (e: any) {
      console.error("[podcasts] loadFeeds failed:", e);
      setError(e.message || "Failed to load podcast feeds");
    } finally {
      setLoading(false);
    }
  }, []);

  const loadEpisodes = useCallback(async (feedId: number) => {
    try {
      const eps = await api.podcastEpisodes(feedId);
      setEpisodes(eps);
    } catch (e: any) {
      console.error("[podcasts] loadEpisodes failed:", e);
      toast.error(`Failed to load episodes: ${e.message || "unknown error"}`);
    }
  }, [toast]);

  useEffect(() => {
    loadFeeds();
  }, []);

  useEffect(() => {
    if (selectedFeed) {
      loadEpisodes(selectedFeed.id);
    }
  }, [selectedFeed, loadEpisodes]);

  const handleSync = async (feedId: number) => {
    try {
      await api.podcastSync(feedId);
      toast.success("Feed synced");
      await loadFeeds();
      if (selectedFeed?.id === feedId) {
        await loadEpisodes(feedId);
      }
    } catch (e: any) {
      toast.error("Sync failed: " + e.message);
    }
  };

  const handleUnsubscribe = async (feedId: number) => {
    try {
      await api.podcastUnsubscribe(feedId);
      toast.success("Unsubscribed");
      // Stop any active downloads for this feed
      setDownloadingIds((prev) => {
        const next = new Set(prev);
        next.clear();
        return next;
      });
      // Clear all polling intervals
      pollIntervals.current.forEach((interval) => clearInterval(interval));
      pollIntervals.current.clear();
      setSelectedFeed(null);
      await loadFeeds();
    } catch (e: any) {
      toast.error("Failed: " + e.message);
    }
  };

  const handleDownloadFeed = async (feedId: number) => {
    try {
      await api.podcastDownloadFeed(feedId);
      toast.success("Downloading all episodes...");
      // Refresh episode list after triggering download
      if (selectedFeed?.id === feedId) {
        await loadEpisodes(feedId);
      }
    } catch (e: any) {
      toast.error("Download failed: " + e.message);
    }
  };

  const confirmDownloadFeed = (feedId: number) => {
    setFeedToDownload(feedId);
    setShowDownloadConfirm(true);
  };

  const handleDownloadEpisode = async (episode: PodcastEpisode) => {
    if (!selectedFeed) return;
    setDownloadingIds((prev) => new Set(prev).add(episode.id));
    try {
      await api.podcastDownloadEpisode(episode.id);
      toast.success(`Downloading "${episode.title}" — see Downloads page for progress`);
      let attempts = 0;
      const maxAttempts = 600;
      const feedIdAtStart = selectedFeed.id;
      const existing = pollIntervals.current.get(episode.id);
      if (existing) clearInterval(existing);
      const interval = setInterval(async () => {
        attempts++;
        try {
          const eps = await api.podcastEpisodes(feedIdAtStart);
          const updated = eps.find((e) => e.id === episode.id);
          if (!updated) {
            setDownloadingIds((prev) => {
              const next = new Set(prev);
              next.delete(episode.id);
              return next;
            });
            clearInterval(interval);
            pollIntervals.current.delete(episode.id);
            return;
          }
          if (updated.downloaded) {
            setDownloadingIds((prev) => {
              const next = new Set(prev);
              next.delete(episode.id);
              return next;
            });
            clearInterval(interval);
            pollIntervals.current.delete(episode.id);
            toast.success(`Downloaded "${episode.title}"`);
            if (selectedFeedRef.current?.id === feedIdAtStart) {
              setEpisodes(eps);
            }
          } else if (updated.download_error) {
            setDownloadingIds((prev) => {
              const next = new Set(prev);
              next.delete(episode.id);
              return next;
            });
            clearInterval(interval);
            pollIntervals.current.delete(episode.id);
            toast.error(`"${episode.title}" failed: ${updated.download_error}`);
          } else if (attempts >= maxAttempts) {
            setDownloadingIds((prev) => {
              const next = new Set(prev);
              next.delete(episode.id);
              return next;
            });
            clearInterval(interval);
            pollIntervals.current.delete(episode.id);
            toast.error(`Download timed out for "${episode.title}" — check Downloads page`);
          }
        } catch {
          // ignore poll errors
        }
      }, 3000);
      pollIntervals.current.set(episode.id, interval);
    } catch (e: any) {
      toast.error("Download failed: " + e.message);
      setDownloadingIds((prev) => {
        const next = new Set(prev);
        next.delete(episode.id);
        return next;
      });
    }
  };

  const handlePlayEpisode = async (episodeId: number, title: string, fallbackPositionSec: number = 0) => {
    try {
      const { track_id } = await api.podcastEpisodeTrack(episodeId);
      const track = await api.track(track_id);
      let startPositionSec = fallbackPositionSec;
      try {
        const posData = await api.podcastEpisodePosition(episodeId);
        if (posData.position_sec > 0) {
          startPositionSec = posData.position_sec;
        }
      } catch {
        // ignore — use fallback
      }
      await player.setPodcastEpisodeId(episodeId);
      player.play([track], 0);
      if (startPositionSec > 0) {
        setTimeout(() => {
          player.seek(startPositionSec);
        }, 500);
      }
    } catch (e: any) {
      toast.error("Cannot play episode: " + (e.message || "track not found — rescan may be in progress"));
    }
  };

  // Phase 2: Toggle auto-download for a feed
  const handleToggleAutoDownload = async (feed: PodcastFeed) => {
    try {
      await api.updatePodcastFeed(feed.id, { auto_download: !feed.auto_download });
      toast.success(feed.auto_download ? "Auto-download disabled" : "Auto-download enabled");
      await loadFeeds();
    } catch (e: any) {
      toast.error("Failed to update feed: " + e.message);
    }
  };

  // Phase 2: Mark episode as listened/unlistened
  const handleMarkListened = async (episode: PodcastEpisode, listened: boolean) => {
    try {
      await api.savePodcastEpisodePosition(episode.id, listened ? episode.duration_sec : 0, listened);
      toast.success(listened ? "Marked as listened" : "Marked as unlistened");
      if (selectedFeed) {
        await loadEpisodes(selectedFeed.id);
      }
    } catch (e: any) {
      toast.error("Failed: " + e.message);
    }
  };

  // Phase 2: Add to playlist
  const handleAddToPlaylist = async (episode: PodcastEpisode, playlistId: number, playlistName: string) => {
    try {
      const { track_id } = await api.podcastEpisodeTrack(episode.id);
      await api.addToPlaylist(playlistId, track_id);
      toast.success(`Added to "${playlistName}"`);
    } catch (e: any) {
      toast.error("Failed to add to playlist: " + e.message);
    }
  };

  // Phase 2: Sorting + filtering
  const sortedFilteredEpisodes = useMemo(() => {
    let result = [...episodes];

    // Apply search filter
    if (searchText.trim()) {
      const q = searchText.toLowerCase();
      result = result.filter((ep) =>
        ep.title.toLowerCase().includes(q) ||
        (ep.description && ep.description.toLowerCase().includes(q))
      );
    }

    // Apply status filter
    switch (filterMode) {
      case "downloaded":
        result = result.filter((ep) => ep.downloaded);
        break;
      case "not_downloaded":
        result = result.filter((ep) => !ep.downloaded);
        break;
      case "listened":
        result = result.filter((ep) => ep.listened);
        break;
      case "not_listened":
        result = result.filter((ep) => !ep.listened);
        break;
    }

    // Apply sorting
    result.sort((a, b) => {
      let cmp = 0;
      switch (sortField) {
        case "date":
          cmp = a.pub_date - b.pub_date;
          break;
        case "duration":
          cmp = a.duration_sec - b.duration_sec;
          break;
        case "title":
          cmp = a.title.localeCompare(b.title);
          break;
      }
      return sortDir === "asc" ? cmp : -cmp;
    });

    return result;
  }, [episodes, searchText, filterMode, sortField, sortDir]);

  const formatTime = (s: number) => {
    const m = Math.floor(s / 60);
    const sec = s % 60;
    return `${m}:${sec.toString().padStart(2, "0")}`;
  };

  if (loading) {
    return (
      <div className="space-y-4 p-4 md:p-6">
        <h1 className="text-2xl font-semibold">Podcasts</h1>
        <div className="flex items-center justify-center py-12">
          <Loader2 className="animate-spin text-accent" size={24} />
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4 p-4 md:p-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold flex items-center gap-2">
          <Rss className="text-accent" /> Podcasts
        </h1>
        <div className="flex items-center gap-2">
          <button
            onClick={() => showHelp("podcasts.feeds")}
            className="p-1 text-muted/50 hover:text-accent transition-colors rounded hover:bg-panel2/50"
            aria-label="Help: Podcasts"
          >
            <HelpCircle size={16} />
          </button>
          <button
            onClick={() => setShowAddModal(true)}
            className="px-3 py-2 bg-accent text-bg rounded-md font-medium flex items-center gap-2 text-sm"
          >
            <Plus size={14} /> Add Podcast
          </button>
        </div>
      </div>

      {/* Error banner */}
      {error && (
        <div className="bg-red-900/30 border border-red-500/40 rounded-lg p-3 flex items-center gap-3">
          <AlertCircle size={18} className="text-red-400 shrink-0" />
          <p className="text-sm text-red-300 flex-1">{error}</p>
          <button
            onClick={() => { setError(null); setLoading(true); loadFeeds(); }}
            className="text-xs text-red-400 hover:text-red-300 underline shrink-0"
          >
            Retry
          </button>
        </div>
      )}

      {(!feeds || feeds.length === 0 && !error) ? (
        <div className="bg-panel rounded-lg p-8 border border-panel2 text-center">
          <Rss size={32} className="mx-auto mb-3 text-muted" />
          <p className="text-muted mb-4">
            No podcasts subscribed yet. Add an RSS feed URL to get started.
          </p>
          <button
            onClick={() => setShowAddModal(true)}
            className="px-4 py-2 bg-accent text-bg rounded-md font-medium inline-flex items-center gap-2"
          >
            <Plus size={14} /> Add Your First Podcast
          </button>
        </div>
      ) : (
        <div className={`grid grid-cols-1 ${isMobile ? "" : "lg:grid-cols-4"} gap-4`}>
          {/* Sidebar — Feed list */}
          <div className={`${isMobile ? "flex overflow-x-auto gap-2 pb-2" : "lg:col-span-1 space-y-2"} max-h-[calc(100vh-12rem)] overflow-y-auto`}>
            {feeds && feeds.map((feed) => (
              <button
                key={feed.id}
                onClick={() => setSelectedFeed(feed)}
                className={`${isMobile ? "min-w-[140px] shrink-0" : "w-full"} text-left rounded-lg p-3 border transition-colors ${
                  selectedFeed?.id === feed.id
                    ? "bg-panel border-accent"
                    : "bg-panel border-panel2 hover:border-accent/40"
                }`}
                aria-label={`${feed.title}, ${feed.episode_count} episodes`}
              >
                <div className="flex items-center gap-3">
                  {feed.image_url ? (
                    <img
                      src={feed.image_url}
                      alt=""
                      className="w-10 h-10 rounded object-cover bg-panel2"
                      onError={(e) => { (e.target as HTMLImageElement).style.display = 'none'; }}
                    />
                  ) : (
                    <div className="w-10 h-10 rounded bg-panel2 flex items-center justify-center">
                      <Rss size={16} className="text-muted" />
                    </div>
                  )}
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium truncate">{feed.title || "Untitled"}</p>
                    <p className="text-xs text-muted">
                      {feed.episode_count} episodes • {feed.downloaded_count} downloaded
                    </p>
                    {feed.auto_download && (
                      <p className="text-xs text-accent flex items-center gap-1 mt-0.5">
                        <Download size={10} /> Auto-download
                      </p>
                    )}
                  </div>
                </div>
              </button>
            ))}
            <p className="text-xs text-muted text-center pt-2">
              {feeds.length} subscription{feeds.length !== 1 ? "s" : ""}
            </p>
          </div>

          {/* Main panel — Episodes */}
          <div className="lg:col-span-3 space-y-3">
            {selectedFeed && (
              <>
                <div className="flex items-center justify-between">
                  <div>
                    <h2 className="text-lg font-semibold">{selectedFeed.title}</h2>
                    {selectedFeed.description && (
                      <p className="text-sm text-muted mt-0.5 line-clamp-2">{selectedFeed.description}</p>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => confirmDownloadFeed(selectedFeed.id)}
                      className="p-2 rounded-md hover:bg-panel2 transition-colors text-accent"
                      title="Download all episodes"
                      aria-label="Download all episodes"
                    >
                      <Download size={14} />
                    </button>
                    <button
                      onClick={() => handleSync(selectedFeed.id)}
                      className="p-2 rounded-md hover:bg-panel2 transition-colors"
                      title="Sync feed"
                      aria-label="Sync feed"
                    >
                      <RefreshCw size={14} />
                    </button>
                    <button
                      onClick={() => handleUnsubscribe(selectedFeed.id)}
                      className="p-2 rounded-md hover:bg-panel2 transition-colors text-red-400"
                      title="Unsubscribe"
                      aria-label="Unsubscribe from podcast"
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                </div>

                {/* Phase 2: Auto-download toggle */}
                <div className="flex items-center gap-3 bg-panel rounded-lg p-2 border border-panel2">
                  <button
                    onClick={() => handleToggleAutoDownload(selectedFeed)}
                    className={`flex items-center gap-2 text-sm px-3 py-1.5 rounded-md transition-colors ${
                      selectedFeed.auto_download
                        ? "bg-accent/20 text-accent"
                        : "bg-panel2 text-muted hover:text-text"
                    }`}
                    aria-label={selectedFeed.auto_download ? "Disable auto-download" : "Enable auto-download"}
                  >
                    <Download size={14} />
                    Auto-download: {selectedFeed.auto_download ? "On" : "Off"}
                  </button>
                </div>

                {/* Phase 2: Search, sort, filter bar */}
                <div className="flex items-center gap-2 flex-wrap">
                  <div className="flex-1 min-w-[200px] relative">
                    <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted" />
                    <input
                      value={searchText}
                      onChange={(e) => setSearchText(e.target.value)}
                      placeholder="Search episodes..."
                      className="w-full bg-panel border border-panel2 rounded-md pl-9 pr-3 py-2 text-sm outline-none focus:border-accent"
                      aria-label="Search episodes"
                    />
                    {searchText && (
                      <button
                        onClick={() => setSearchText("")}
                        className="absolute right-2 top-1/2 -translate-y-1/2 text-muted hover:text-text"
                        aria-label="Clear search"
                      >
                        <X size={14} />
                      </button>
                    )}
                  </div>

                  {/* Sort button */}
                  <div className="relative">
                    <button
                      onClick={() => { setShowSortMenu(!showSortMenu); setShowFilterMenu(false); }}
                      className="flex items-center gap-1 px-3 py-2 bg-panel border border-panel2 rounded-md text-sm hover:border-accent/40"
                      aria-label="Sort episodes"
                    >
                      <ArrowUpDown size={14} />
                      <span className="hidden sm:inline">{sortField === "date" ? "Date" : sortField === "duration" ? "Duration" : "Title"}</span>
                      <span className="text-xs text-muted">{sortDir === "asc" ? "↑" : "↓"}</span>
                    </button>
                    {showSortMenu && (
                      <div className="absolute right-0 top-full z-20 mt-1 w-48 bg-panel border border-panel2 rounded-lg shadow-lg py-1">
                        {(["date", "duration", "title"] as SortField[]).map((field) => (
                          <button
                            key={field}
                            onClick={() => {
                              if (sortField === field) {
                                setSortDir(sortDir === "asc" ? "desc" : "asc");
                              } else {
                                setSortField(field);
                                setSortDir(field === "date" ? "desc" : "asc");
                              }
                              setShowSortMenu(false);
                            }}
                            className={`w-full text-left px-3 py-2 text-sm hover:bg-panel2 flex items-center justify-between ${
                              sortField === field ? "text-accent" : ""
                            }`}
                          >
                            {field === "date" ? "Date" : field === "duration" ? "Duration" : "Title"}
                            {sortField === field && <span className="text-xs">{sortDir === "asc" ? "↑" : "↓"}</span>}
                          </button>
                        ))}
                      </div>
                    )}
                  </div>

                  {/* Filter button */}
                  <div className="relative">
                    <button
                      onClick={() => { setShowFilterMenu(!showFilterMenu); setShowSortMenu(false); }}
                      className={`flex items-center gap-1 px-3 py-2 bg-panel border border-panel2 rounded-md text-sm hover:border-accent/40 ${
                        filterMode !== "all" ? "border-accent/60 text-accent" : ""
                      }`}
                      aria-label="Filter episodes"
                    >
                      <Filter size={14} />
                      <span className="hidden sm:inline">{filterMode === "all" ? "All" : filterMode === "downloaded" ? "Downloaded" : filterMode === "not_downloaded" ? "Not Downloaded" : filterMode === "listened" ? "Listened" : "Not Listened"}</span>
                    </button>
                    {showFilterMenu && (
                      <div className="absolute right-0 top-full z-20 mt-1 w-48 bg-panel border border-panel2 rounded-lg shadow-lg py-1">
                        {([
                          { value: "all", label: "All" },
                          { value: "downloaded", label: "Downloaded" },
                          { value: "not_downloaded", label: "Not Downloaded" },
                          { value: "listened", label: "Listened" },
                          { value: "not_listened", label: "Not Listened" },
                        ] as { value: FilterMode; label: string }[]).map((opt) => (
                          <button
                            key={opt.value}
                            onClick={() => { setFilterMode(opt.value); setShowFilterMenu(false); }}
                            className={`w-full text-left px-3 py-2 text-sm hover:bg-panel2 ${
                              filterMode === opt.value ? "text-accent" : ""
                            }`}
                          >
                            {opt.label}
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                </div>

                {sortedFilteredEpisodes.length === 0 ? (
                  <div className="bg-panel rounded-lg p-8 border border-panel2 text-center">
                    <p className="text-muted">
                      {searchText || filterMode !== "all"
                        ? "No episodes match your search/filter."
                        : "No episodes found. Try syncing the feed."}
                    </p>
                  </div>
                ) : (
                  <div className="space-y-2">
                    {sortedFilteredEpisodes.map((ep) => {
                      const hasProgress = ep.playback_position_sec > 0 && !ep.listened;
                      const progressPct = ep.duration_sec > 0
                        ? Math.min(100, Math.round((ep.playback_position_sec / ep.duration_sec) * 100))
                        : 0;
                      return (
                        <EpisodeCard
                          key={ep.id}
                          episode={ep}
                          hasProgress={hasProgress}
                          progressPct={progressPct}
                          formatTime={formatTime}
                          downloading={downloadingIds.has(ep.id)}
                          onPlay={() => handlePlayEpisode(Number(ep.id), ep.title, hasProgress ? ep.playback_position_sec : 0)}
                          onDownload={() => handleDownloadEpisode(ep)}
                          onMarkListened={(listened) => handleMarkListened(ep, listened)}
                          onAddToPlaylist={(playlistId, playlistName) => handleAddToPlaylist(ep, playlistId, playlistName)}
                        />
                      );
                    })}
                  </div>
                )}
              </>
            )}
          </div>
        </div>
      )}

      {/* Download confirmation modal */}
      {showDownloadConfirm && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50 p-4">
          <div className="bg-panel rounded-lg border border-panel2 w-full max-w-sm shadow-xl p-4 space-y-3">
            <h3 className="text-lg font-semibold">Download all episodes?</h3>
            <p className="text-sm text-muted">
              This will download all episodes for "{feeds.find((f) => f.id === feedToDownload)?.title}". For feeds with many episodes this may take a while and use significant storage.
            </p>
            <div className="flex gap-2 justify-end">
              <button
                onClick={() => { setShowDownloadConfirm(false); setFeedToDownload(null); }}
                className="px-4 py-2 bg-panel2 rounded-md text-sm hover:bg-panel2/70"
              >
                Cancel
              </button>
              <button
                onClick={() => {
                  if (feedToDownload) handleDownloadFeed(feedToDownload);
                  setShowDownloadConfirm(false);
                  setFeedToDownload(null);
                }}
                className="px-4 py-2 bg-accent text-bg rounded-md text-sm font-medium"
              >
                Download All
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Add Podcast Modal */}
      {showAddModal && (
        <AddPodcastModal
          onClose={() => setShowAddModal(false)}
          onSubscribe={async (url) => {
            setSubscribing(true);
            try {
              await api.podcastSubscribe(url);
              toast.success("Subscribed!");
              setShowAddModal(false);
              await loadFeeds();
            } catch (e: any) {
              toast.error("Failed: " + e.message);
            } finally {
              setSubscribing(false);
            }
          }}
          subscribing={subscribing}
        />
      )}
    </div>
  );
}

// ----- Episode Card Component -----

function EpisodeCard({
  episode: ep,
  hasProgress,
  progressPct,
  formatTime,
  downloading,
  onPlay,
  onDownload,
  onMarkListened,
  onAddToPlaylist,
}: {
  episode: PodcastEpisode;
  hasProgress: boolean;
  progressPct: number;
  formatTime: (s: number) => string;
  downloading: boolean;
  onPlay: () => void;
  onDownload: () => void;
  onMarkListened: (listened: boolean) => void;
  onAddToPlaylist: (playlistId: number, playlistName: string) => void;
}) {
  const [showMenu, setShowMenu] = useState(false);
  const [playlists, setPlaylists] = useState<Playlist[]>([]);
  const [menuLoaded, setMenuLoaded] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  const toast = useToast();

  // Close menu on outside click
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setShowMenu(false);
      }
    }
    if (showMenu) {
      document.addEventListener("mousedown", handleClick);
      return () => document.removeEventListener("mousedown", handleClick);
    }
  }, [showMenu]);

  const handleOpenMenu = async () => {
    setShowMenu(true);
    if (!menuLoaded) {
      try {
        const pls = await api.playlists();
        setPlaylists(pls);
        setMenuLoaded(true);
      } catch {
        // ignore
      }
    }
  };

  return (
    <div
      className="bg-panel rounded-lg p-3 border border-panel2 hover:border-accent/30 transition-colors"
      tabIndex={0}
      role="article"
      aria-label={`Episode: ${ep.title}`}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="text-sm font-medium truncate">{ep.title}</h3>
            {ep.listened && (
              <span className="text-xs text-muted shrink-0 flex items-center gap-1">
                <Check size={10} /> Listened
              </span>
            )}
          </div>
          <div className="flex items-center gap-2 mt-1 flex-wrap">
            {ep.pub_date > 0 && (
              <span className="text-xs text-muted">
                {new Date(ep.pub_date * 1000).toLocaleDateString()}
              </span>
            )}
            {ep.duration_sec > 0 && (
              <span className="text-xs text-muted">
                {formatTime(ep.duration_sec)}
              </span>
            )}
            {ep.downloaded && (
              <span className="text-xs text-green-400 flex items-center gap-1">
                <Check size={10} /> Downloaded
              </span>
            )}
            {ep.download_error && !ep.downloaded && (
              <span className="text-xs text-red-400 flex items-center gap-1" title={ep.download_error}>
                Error
              </span>
            )}
          </div>
          {/* Progress bar for partially listened episodes */}
          {hasProgress && progressPct > 0 && (
            <div className="mt-2">
              <div className="flex items-center gap-2">
                <div className="flex-1 h-1 bg-panel2 rounded-full overflow-hidden" role="progressbar" aria-valuenow={progressPct} aria-valuemin={0} aria-valuemax={100} aria-label={`Playback progress: ${progressPct}%`}>
                  <div
                    className="h-full bg-accent rounded-full transition-all"
                    style={{ width: `${progressPct}%` }}
                  />
                </div>
                <span className="text-xs text-muted shrink-0">
                  {formatTime(ep.playback_position_sec)} / {formatTime(ep.duration_sec)}
                </span>
              </div>
            </div>
          )}
          {ep.description && (
            <p className="text-xs text-muted mt-1 line-clamp-2">{ep.description}</p>
          )}
        </div>
        <div className="flex items-center gap-1 shrink-0">
          {ep.downloaded && ep.file_path ? (
            <button
              onClick={onPlay}
              className="p-2 rounded-md hover:bg-panel2 transition-colors text-accent"
              title={hasProgress ? `Resume from ${formatTime(ep.playback_position_sec)}` : "Play"}
              aria-label={hasProgress ? `Resume "${ep.title}" from ${formatTime(ep.playback_position_sec)}` : `Play "${ep.title}"`}
            >
              <Play size={14} />
            </button>
          ) : (
            <button
              onClick={onDownload}
              disabled={downloading}
              className="p-2 rounded-md hover:bg-panel2 transition-colors disabled:opacity-50"
              title="Download"
              aria-label={`Download "${ep.title}"`}
            >
              {downloading ? (
                <Loader2 size={14} className="animate-spin" />
              ) : (
                <Download size={14} />
              )}
            </button>
          )}
          {/* Context menu button */}
          <div ref={menuRef} className="relative">
            <button
              onClick={handleOpenMenu}
              className="p-2 rounded-md hover:bg-panel2 transition-colors text-muted hover:text-text"
              aria-label={`Actions for "${ep.title}"`}
              aria-expanded={showMenu}
            >
              <MoreHorizontal size={14} />
            </button>
            {showMenu && (
              <div className="absolute right-0 top-full z-20 mt-1 w-56 bg-panel border border-panel2 rounded-lg shadow-lg py-1 max-h-72 overflow-y-auto">
                {/* Mark listened/unlistened */}
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    onMarkListened(!ep.listened);
                    setShowMenu(false);
                  }}
                  className="w-full text-left px-3 py-2 text-sm hover:bg-panel2 flex items-center gap-2"
                >
                  {ep.listened ? <EyeOff size={14} /> : <Eye size={14} />}
                  {ep.listened ? "Mark as unlistened" : "Mark as listened"}
                </button>

                {/* Add to playlist section */}
                {ep.downloaded && (
                  <>
                    <div className="px-3 py-1.5 text-xs text-muted uppercase tracking-wide border-t border-panel2">
                      Add to playlist
                    </div>
                    {menuLoaded && playlists.length === 0 ? (
                      <div className="px-3 py-2 text-sm text-muted">
                        No playlists yet.
                      </div>
                    ) : (
                      playlists.map((pl) => (
                        <button
                          key={pl.id}
                          onClick={(e) => {
                            e.stopPropagation();
                            onAddToPlaylist(pl.id, pl.name);
                            setShowMenu(false);
                          }}
                          className="w-full text-left px-3 py-2 text-sm hover:bg-panel2 flex items-center gap-2"
                        >
                          <ListMusic size={14} className="text-muted" />
                          <span className="truncate">{pl.name}</span>
                        </button>
                      ))
                    )}
                  </>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

// ----- Add Podcast Modal -----

function AddPodcastModal({
  onClose,
  onSubscribe,
  subscribing,
}: {
  onClose: () => void;
  onSubscribe: (url: string) => Promise<void>;
  subscribing: boolean;
}) {
  const [url, setUrl] = useState("");
  const [error, setError] = useState("");
  const modalRef = useRef<HTMLDivElement>(null);
  const firstFocusableRef = useRef<HTMLInputElement>(null);
  const lastFocusableRef = useRef<HTMLButtonElement>(null);

  // Focus trap
  useEffect(() => {
    firstFocusableRef.current?.focus();

    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        onClose();
        return;
      }
      if (e.key === "Tab" && modalRef.current) {
        const focusable = modalRef.current.querySelectorAll<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
        );
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (e.shiftKey) {
          if (document.activeElement === first) {
            e.preventDefault();
            last.focus();
          }
        } else {
          if (document.activeElement === last) {
            e.preventDefault();
            first.focus();
          }
        }
      }
    }
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!url.trim()) return;
    setError("");
    try {
      await onSubscribe(url.trim());
    } catch (e: any) {
      setError(e.message || "Failed to subscribe");
    }
  };

  return (
    <div
      className="fixed inset-0 bg-black/60 flex items-center justify-center z-50 p-4"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        ref={modalRef}
        className="bg-panel rounded-lg border border-panel2 w-full max-w-md shadow-xl"
        role="dialog"
        aria-modal="true"
        aria-label="Add Podcast"
      >
        <div className="flex items-center justify-between px-4 py-3 border-b border-panel2">
          <h2 className="text-lg font-semibold">Add Podcast</h2>
          <button
            onClick={onClose}
            className="p-1 rounded hover:bg-panel2 transition-colors"
            aria-label="Close"
            ref={lastFocusableRef}
          >
            <X size={16} />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-4 space-y-3">
          <input
            ref={firstFocusableRef}
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://example.com/feed.xml"
            className="w-full bg-bg border border-panel2 rounded-md px-3 py-2 outline-none focus:border-accent text-sm"
            disabled={subscribing}
          />
          {error && <p className="text-xs text-red-400">{error}</p>}
          <button
            type="submit"
            disabled={subscribing || !url.trim()}
            className="w-full px-4 py-2 bg-accent text-bg rounded-md font-medium flex items-center justify-center gap-2 disabled:opacity-50"
          >
            {subscribing ? (
              <Loader2 size={14} className="animate-spin" />
            ) : (
              <Rss size={14} />
            )}
            {subscribing ? "Subscribing..." : "Subscribe"}
          </button>
        </form>
      </div>
    </div>
  );
}
