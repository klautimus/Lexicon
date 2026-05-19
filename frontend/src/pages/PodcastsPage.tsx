import { useEffect, useState, useCallback } from "react";
import {
  Rss, Plus, RefreshCw, Download, Play, Trash2, Check, Loader2, Search, X, MessageSquare, ChevronDown
} from "lucide-react";
import { api, PodcastFeed, PodcastEpisode, Track } from "../lib/api";
import { useToast } from "../contexts/ToastContext";
import { usePlayer } from "../player/PlayerContext";

export default function PodcastsPage() {
  const toast = useToast();
  const player = usePlayer();
  const [feeds, setFeeds] = useState<PodcastFeed[]>([]);
  const [selectedFeed, setSelectedFeed] = useState<PodcastFeed | null>(null);
  const [episodes, setEpisodes] = useState<PodcastEpisode[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAddModal, setShowAddModal] = useState(false);
  const [downloadingIds, setDownloadingIds] = useState<Set<number>>(new Set());

  const loadFeeds = useCallback(async () => {
    try {
      const f = await api.podcastFeeds();
      setFeeds(f);
      if (f.length > 0 && !selectedFeed) {
        setSelectedFeed(f[0]);
      } else if (selectedFeed) {
        const updated = f.find((x) => x.id === selectedFeed.id);
        if (updated) setSelectedFeed(updated);
      }
    } catch {
      /* ignore */
    } finally {
      setLoading(false);
    }
  }, [selectedFeed]);

  const loadEpisodes = useCallback(async (feedId: number) => {
    try {
      const eps = await api.podcastEpisodes(feedId);
      setEpisodes(eps);
    } catch {
      /* ignore */
    }
  }, []);

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
    } catch (e: any) {
      toast.error("Download failed: " + e.message);
    }
  };

  const handleDownloadEpisode = async (episode: PodcastEpisode) => {
    setDownloadingIds((prev) => new Set(prev).add(episode.id));
    try {
      await api.podcastDownloadEpisode(episode.id);
      toast.success(`Downloading "${episode.title}" — see Downloads page for progress`);
      // Poll for completion or error — check downloaded/download_error.
      // Long episodes can take a while; allow up to 30 minutes before giving up.
      let attempts = 0;
      const maxAttempts = 600; // 30 minutes at 3s interval
      const interval = setInterval(async () => {
        attempts++;
        try {
          const eps = await api.podcastEpisodes(selectedFeed!.id);
          const updated = eps.find((e) => e.id === episode.id);
          if (updated?.downloaded) {
            setDownloadingIds((prev) => {
              const next = new Set(prev);
              next.delete(episode.id);
              return next;
            });
            clearInterval(interval);
            toast.success(`Downloaded "${episode.title}"`);
          } else if (updated?.download_error) {
            setDownloadingIds((prev) => {
              const next = new Set(prev);
              next.delete(episode.id);
              return next;
            });
            clearInterval(interval);
            toast.error(`"${episode.title}" failed: ${updated.download_error}`);
          } else if (attempts >= maxAttempts) {
            setDownloadingIds((prev) => {
              const next = new Set(prev);
              next.delete(episode.id);
              return next;
            });
            clearInterval(interval);
            toast.error(`Download timed out for "${episode.title}" — check Downloads page`);
          }
        } catch {
          // ignore poll errors
        }
      }, 3000);
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
      // Fetch the latest saved position from the backend (the episodes list may be stale)
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
      // If resuming from a saved position, seek after a short delay
      // (the audio element needs time to load the source)
      if (startPositionSec > 0) {
        setTimeout(() => {
          player.seek(startPositionSec);
        }, 500);
      }
    } catch (e: any) {
      toast.error("Cannot play episode: " + (e.message || "track not found — rescan may be in progress"));
    }
  };

  if (loading) {
    return (
      <div className="space-y-4">
        <h1 className="text-2xl font-semibold">Podcasts</h1>
        <div className="flex items-center justify-center py-12">
          <Loader2 className="animate-spin text-accent" size={24} />
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold flex items-center gap-2">
          <Rss className="text-accent" /> Podcasts
        </h1>
        <button
          onClick={() => setShowAddModal(true)}
          className="px-3 py-2 bg-accent text-bg rounded-md font-medium flex items-center gap-2 text-sm"
        >
          <Plus size={14} /> Add Podcast
        </button>
      </div>

      {(!feeds || feeds.length === 0) ? (
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
        <div className="grid grid-cols-1 lg:grid-cols-4 gap-4">
          {/* Sidebar — Feed list */}
          <div className="lg:col-span-1 space-y-2">
            {feeds && feeds.map((feed) => (
              <button
                key={feed.id}
                onClick={() => setSelectedFeed(feed)}
                className={`w-full text-left rounded-lg p-3 border transition-colors ${
                  selectedFeed?.id === feed.id
                    ? "bg-panel border-accent"
                    : "bg-panel border-panel2 hover:border-accent/40"
                }`}
              >
                <div className="flex items-center gap-3">
                  {feed.image_url ? (
                    <img
                      src={feed.image_url}
                      alt=""
                      className="w-10 h-10 rounded object-cover bg-panel2"
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
                      onClick={() => handleDownloadFeed(selectedFeed.id)}
                      className="p-2 rounded-md hover:bg-panel2 transition-colors text-accent"
                      title="Download all episodes"
                    >
                      <Download size={14} />
                    </button>
                    <button
                      onClick={() => handleSync(selectedFeed.id)}
                      className="p-2 rounded-md hover:bg-panel2 transition-colors"
                      title="Sync feed"
                    >
                      <RefreshCw size={14} />
                    </button>
                    <button
                      onClick={() => handleUnsubscribe(selectedFeed.id)}
                      className="p-2 rounded-md hover:bg-panel2 transition-colors text-red-400"
                      title="Unsubscribe"
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                </div>

                {(!episodes || episodes.length === 0) ? (
                  <div className="bg-panel rounded-lg p-8 border border-panel2 text-center">
                    <p className="text-muted">No episodes found. Try syncing the feed.</p>
                  </div>
                ) : (
                  <div className="space-y-2">
                    {episodes && episodes.map((ep) => {
                      const hasProgress = ep.playback_position_sec > 0 && !ep.listened;
                      const progressPct = ep.duration_sec > 0
                        ? Math.min(100, Math.round((ep.playback_position_sec / ep.duration_sec) * 100))
                        : 0;
                      const formatTime = (s: number) => {
                        const m = Math.floor(s / 60);
                        const sec = s % 60;
                        return `${m}:${sec.toString().padStart(2, '0')}`;
                      };
                      return (
                      <div
                        key={ep.id}
                        className="bg-panel rounded-lg p-3 border border-panel2 hover:border-accent/30 transition-colors"
                      >
                        <div className="flex items-start justify-between gap-3">
                          <div className="flex-1 min-w-0">
                            <div className="flex items-center gap-2">
                              <h3 className="text-sm font-medium truncate">{ep.title}</h3>
                              {ep.listened && (
                                <span className="text-xs text-muted shrink-0">✓ Listened</span>
                              )}
                            </div>
                            <div className="flex items-center gap-2 mt-1">
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
                                  <div className="flex-1 h-1 bg-panel2 rounded-full overflow-hidden">
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
                          <div className="flex items-center gap-1">
                            {ep.downloaded && ep.file_path ? (
                              <button
                                onClick={() => handlePlayEpisode(Number(ep.id), ep.title, hasProgress ? ep.playback_position_sec : 0)}
                                className="p-2 rounded-md hover:bg-panel2 transition-colors text-accent"
                                title={hasProgress ? `Resume from ${formatTime(ep.playback_position_sec)}` : "Play"}
                              >
                                <Play size={14} />
                              </button>
                            ) : (
                              <button
                                onClick={() => handleDownloadEpisode(ep)}
                                disabled={downloadingIds.has(ep.id)}
                                className="p-2 rounded-md hover:bg-panel2 transition-colors disabled:opacity-50"
                                title="Download"
                              >
                                {downloadingIds.has(ep.id) ? (
                                  <Loader2 size={14} className="animate-spin" />
                                ) : (
                                  <Download size={14} />
                                )}
                              </button>
                            )}
                          </div>
                        </div>
                      </div>
                    );
                  })}
                  </div>
                )}
              </>
            )}
          </div>
        </div>
      )}

      {/* Add Podcast Modal */}
      {showAddModal && (
        <AddPodcastModal
          onClose={() => setShowAddModal(false)}
          onSubscribe={async (url) => {
            try {
              await api.podcastSubscribe(url);
              toast.success("Subscribed!");
              setShowAddModal(false);
              await loadFeeds();
            } catch (e: any) {
              toast.error("Failed: " + e.message);
            }
          }}
        />
      )}
    </div>
  );
}

// ----- Add Podcast Modal -----

function AddPodcastModal({
  onClose,
  onSubscribe,
}: {
  onClose: () => void;
  onSubscribe: (url: string) => Promise<void>;
}) {
  const [tab, setTab] = useState<"url" | "search">("url");
  const [url, setUrl] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const [subscribing, setSubscribing] = useState(false);
  const [error, setError] = useState("");

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!url.trim()) return;
    setSubscribing(true);
    setError("");
    try {
      await onSubscribe(url.trim());
    } catch (e: any) {
      setError(e.message || "Failed to subscribe");
    } finally {
      setSubscribing(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50 p-4">
      <div className="bg-panel rounded-lg border border-panel2 w-full max-w-md shadow-xl">
        <div className="flex items-center justify-between px-4 py-3 border-b border-panel2">
          <h2 className="text-lg font-semibold">Add Podcast</h2>
          <button onClick={onClose} className="p-1 rounded hover:bg-panel2 transition-colors">
            <X size={16} />
          </button>
        </div>

        {/* Tabs */}
        <div className="flex border-b border-panel2">
          <button
            onClick={() => setTab("url")}
            className={`flex-1 px-4 py-2 text-sm font-medium transition-colors ${
              tab === "url" ? "text-accent border-b-2 border-accent" : "text-muted"
            }`}
          >
            Paste RSS URL
          </button>
          <button
            onClick={() => setTab("search")}
            className={`flex-1 px-4 py-2 text-sm font-medium transition-colors ${
              tab === "search" ? "text-accent border-b-2 border-accent" : "text-muted"
            }`}
          >
            <Search size={14} className="inline mr-1" />
            Search
          </button>
        </div>

        <div className="p-4">
          {tab === "url" ? (
            <form onSubmit={handleSubmit} className="space-y-3">
              <input
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="https://example.com/feed.xml"
                className="w-full bg-bg border border-panel2 rounded-md px-3 py-2 outline-none focus:border-accent text-sm"
                autoFocus
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
          ) : (
            <div className="space-y-3">
              <p className="text-sm text-muted">
                Search for podcasts by name or topic. (Requires web search to be enabled.)
              </p>
              <input
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="e.g. true crime, tech news..."
                className="w-full bg-bg border border-panel2 rounded-md px-3 py-2 outline-none focus:border-accent text-sm"
              />
              <p className="text-xs text-muted">
                💡 Tip: Use the chat on the Discover page to find podcasts by topic!
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
