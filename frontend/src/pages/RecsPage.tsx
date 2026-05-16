import { useEffect, useRef, useState } from "react";
import { Sparkles, Send, RefreshCw, Library, Download, Loader2, Check, ListMusic } from "lucide-react";
import { api, RecsPayload, DownloadJob, PlaylistPayload } from "../lib/api";
import { useToast } from "../contexts/ToastContext";
import { usePlayer } from "../player/PlayerContext";

export default function RecsPage() {
  const toast = useToast();
  const pollRef = useRef<Record<string, number>>({});
  const player = usePlayer();
  const [recs, setRecs] = useState<RecsPayload | null>(null);
  const [createdAt, setCreatedAt] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);
  const [chatLog, setChatLog] = useState<{ role: "user" | "ai"; text: string }[]>([]);
  const [input, setInput] = useState("");
  const [chatBusy, setChatBusy] = useState(false);
  const [downloadingIds, setDownloadingIds] = useState<Set<string>>(new Set());
  const [completedIds, setCompletedIds] = useState<Set<string>>(new Set());
  const [completedTrackIds, setCompletedTrackIds] = useState<Record<string, number>>({});

  // AI playlist generation state
  const [playlistPreview, setPlaylistPreview] = useState<PlaylistPayload | null>(null);
  const [generatingPlaylist, setGeneratingPlaylist] = useState(false);
  const [creatingPlaylist, setCreatingPlaylist] = useState(false);
  const [playlistTrackStatus, setPlaylistTrackStatus] = useState<
    Record<string, "pending" | "present" | "downloading" | "completed" | "failed">
  >({});
  const [createdPlaylistId, setCreatedPlaylistId] = useState<number | null>(null);

  async function load() {
    const r = await api.recs();
    if (!r.empty && r.data) {
      setRecs(r.data);
      setCreatedAt(r.created_at || null);
    }
  }
  useEffect(() => {
    load().catch(() => {});
  }, []);

  useEffect(() => {
    return () => {
      Object.values(pollRef.current).forEach(clearInterval);
    };
  }, []);

  async function refresh() {
    setLoading(true);
    try {
      const r = await api.refreshRecs();
      setRecs(r);
      setCreatedAt(Math.floor(Date.now() / 1000));
    } catch (e: any) {
      alert("Failed: " + e.message);
    } finally {
      setLoading(false);
    }
  }

  async function send(e: React.FormEvent) {
    e.preventDefault();
    if (!input.trim() || chatBusy) return;
    const msg = input.trim();
    setInput("");
    setChatLog((l) => [...l, { role: "user", text: msg }]);
    setChatBusy(true);
    try {
      const r = await api.chat(msg);
      if (r.playlist) {
        // Playlist mode: show conversational message in chat, then render preview
        setChatLog((l) => [...l, { role: "ai", text: r.reply }]);
        setPlaylistPreview(r.playlist);
        setCreatedPlaylistId(null);
        setPlaylistTrackStatus({});
        const initStatus: Record<string, "pending"> = {};
        r.playlist.tracks.forEach((t) => {
          initStatus[`${t.artist} - ${t.title}`] = "pending";
        });
        setPlaylistTrackStatus(initStatus);
      } else {
        // Text mode: normal chat reply
        setChatLog((l) => [...l, { role: "ai", text: r.reply }]);
      }
    } catch (e: any) {
      setChatLog((l) => [...l, { role: "ai", text: "Error: " + e.message }]);
    } finally {
      setChatBusy(false);
    }
  }

  async function playLibraryItem(trackId: number) {
    const t = await api.track(trackId);
    player.play([t], 0);
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
          setDownloadingIds((prev) => {
            const next = new Set(prev);
            next.delete(name);
            return next;
          });
          setCompletedIds((prev) => new Set(prev).add(name));
          // Try to find the newly downloaded track so we can offer a Play button
          try {
            const tracks = await api.search(name);
            if (tracks.length > 0) {
              setCompletedTrackIds((prev) => ({ ...prev, [name]: tracks[0].id }));
            }
          } catch { /* ignore */ }
        } else if (updated.status === "failed") {
          window.clearInterval(interval);
          delete pollRef.current[job.id];
          toast.error(`Download failed: ${updated.error || "Unknown error"}`);
          setDownloadingIds((prev) => {
            const next = new Set(prev);
            next.delete(name);
            return next;
          });
        } else if (updated.status === "cancelled") {
          window.clearInterval(interval);
          delete pollRef.current[job.id];
          toast.info(`Download cancelled for "${name}"`);
          setDownloadingIds((prev) => {
            const next = new Set(prev);
            next.delete(name);
            return next;
          });
        }
      } catch {
        window.clearInterval(interval);
        delete pollRef.current[job.id];
        toast.error(`Lost connection tracking download`);
        setDownloadingIds((prev) => {
          const next = new Set(prev);
          next.delete(name);
          return next;
        });
      }
    }, 2000);
    pollRef.current[job.id] = interval;
  }

  async function downloadItem(title: string, artist: string) {
    const name = `${artist} - ${title}`;
    if (downloadingIds.has(name)) return;
    setDownloadingIds((prev) => new Set(prev).add(name));
    try {
      const job = await api.downloadSearch(name);
      trackDownload(job, name);
    } catch {
      toast.error("Failed to start download");
      setDownloadingIds((prev) => {
        const next = new Set(prev);
        next.delete(name);
        return next;
      });
    }
  }

  async function generateAiPlaylist() {
    setGeneratingPlaylist(true);
    setPlaylistPreview(null);
    setCreatedPlaylistId(null);
    setPlaylistTrackStatus({});
    try {
      const data = await api.generatePlaylist();
      setPlaylistPreview(data);
      const initStatus: Record<string, "pending"> = {};
      data.tracks.forEach((t) => {
        initStatus[`${t.artist} - ${t.title}`] = "pending";
      });
      setPlaylistTrackStatus(initStatus);
    } catch (e: any) {
      toast.error("Failed to generate playlist: " + e.message);
    } finally {
      setGeneratingPlaylist(false);
    }
  }

  async function findTrackInLibrary(title: string, artist: string): Promise<number | null> {
    // Try multiple search strategies to find the track
    const queries = [
      `${artist} ${title}`,           // artist + title (no dash)
      title,                           // title only
      artist,                          // artist only
      `${artist} - ${title}`,          // full query with dash
    ];
    for (const q of queries) {
      try {
        const tracks = await api.search(q);
        if (tracks.length > 0) return tracks[0].id;
      } catch { /* continue */ }
    }
    return null;
  }

  async function createAiPlaylist() {
    if (!playlistPreview) return;
    setCreatingPlaylist(true);
    try {
      // 1. Create the playlist
      const playlist = await api.createPlaylist(playlistPreview.name);
      setCreatedPlaylistId(playlist.id);
      toast.success(`Created playlist "${playlistPreview.name}"`);

      // 2. Process each track via downloadSearch (backend checks library first)
      for (const track of playlistPreview.tracks) {
        const key = `${track.artist} - ${track.title}`;
        setPlaylistTrackStatus((prev) => ({ ...prev, [key]: "downloading" }));
        try {
          const job = await api.downloadSearch(key);

          // Case A: Backend resolved to existing track immediately
          if (job.status === "succeeded" && job.track_id) {
            await api.addToPlaylist(playlist.id, job.track_id);
            setPlaylistTrackStatus((prev) => ({ ...prev, [key]: "present" }));
            continue;
          }

          // Case B: Download in progress — poll until done
          const interval = window.setInterval(async () => {
            try {
              const updated = await api.downloadJob(job.id);
              if (updated.status === "succeeded") {
                window.clearInterval(interval);
                delete pollRef.current[job.id];

                // Sub-case B1: Backend populated track_id (resolved or after rescan)
                if (updated.track_id) {
                  await api.addToPlaylist(playlist.id, updated.track_id);
                  setPlaylistTrackStatus((prev) => ({
                    ...prev,
                    [key]: "completed",
                  }));
                  toast.success(`"${key}" added to playlist`);
                  return;
                }

                // Sub-case B2: Actual download completed — find track in library with retries
                // (rescan may still be running)
                let found = false;
                for (let attempt = 0; attempt < 15; attempt++) {
                  const trackId = await findTrackInLibrary(track.title, track.artist);
                  if (trackId) {
                    await api.addToPlaylist(playlist.id, trackId);
                    setPlaylistTrackStatus((prev) => ({
                      ...prev,
                      [key]: "completed",
                    }));
                    toast.success(`"${key}" added to playlist`);
                    found = true;
                    break;
                  }
                  await new Promise((r) => setTimeout(r, 2000));
                }
                if (!found) {
                  setPlaylistTrackStatus((prev) => ({
                    ...prev,
                    [key]: "failed",
                  }));
                }
              } else if (updated.status === "failed" || updated.status === "cancelled") {
                window.clearInterval(interval);
                delete pollRef.current[job.id];
                setPlaylistTrackStatus((prev) => ({
                  ...prev,
                  [key]: "failed",
                }));
              }
            } catch {
              window.clearInterval(interval);
              delete pollRef.current[job.id];
              setPlaylistTrackStatus((prev) => ({
                ...prev,
                [key]: "failed",
              }));
            }
          }, 2000);
          pollRef.current[job.id] = interval;
        } catch {
          setPlaylistTrackStatus((prev) => ({ ...prev, [key]: "failed" }));
        }
      }
    } catch (e: any) {
      toast.error("Failed to create playlist: " + e.message);
    } finally {
      setCreatingPlaylist(false);
    }
  }

  function statusIcon(status: string) {
    switch (status) {
      case "present":
        return <Check size={14} className="text-green-400" />;
      case "completed":
        return <Check size={14} className="text-green-400" />;
      case "downloading":
        return <Loader2 size={14} className="animate-spin text-accent" />;
      case "failed":
        return <span className="text-red-400 text-xs">Failed</span>;
      default:
        return <span className="text-muted text-xs">Pending</span>;
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold flex items-center gap-2">
          <Sparkles className="text-accent" /> Discover
        </h1>
        <div className="flex items-center gap-2">
          <button
            onClick={generateAiPlaylist}
            disabled={generatingPlaylist}
            className="px-3 py-2 bg-panel2 border border-panel2 hover:border-accent text-text rounded-md font-medium flex items-center gap-2 disabled:opacity-60 transition"
          >
            <ListMusic size={14} className={generatingPlaylist ? "animate-spin" : ""} />
            {generatingPlaylist ? "Thinking…" : "Generate Playlist"}
          </button>
          <button
            onClick={refresh}
            disabled={loading}
            className="px-3 py-2 bg-accent text-bg rounded-md font-medium flex items-center gap-2 disabled:opacity-60"
          >
            <RefreshCw size={14} className={loading ? "animate-spin" : ""} />
            {loading ? "Thinking…" : recs ? "Refresh" : "Generate"}
          </button>
        </div>
      </div>

      {createdAt && (
        <p className="text-xs text-muted">
          Last updated {new Date(createdAt * 1000).toLocaleString()}
        </p>
      )}

      {recs ? (
        <>
          <div className="bg-panel rounded-lg p-5 border border-panel2 space-y-2">
            <p className="leading-relaxed">{recs.summary}</p>
            {recs.trends && (
              <p className="text-sm text-muted leading-relaxed">{recs.trends}</p>
            )}
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {recs.items.map((it, i) => (
              <div
                key={i}
                className="bg-panel rounded-lg p-4 border border-panel2 hover:border-accent/40 transition"
              >
                <div className="flex items-center justify-between mb-1">
                  <span
                    className={`text-[10px] uppercase tracking-wider px-2 py-0.5 rounded ${
                      it.type === "library"
                        ? "bg-accent/20 text-accent"
                        : "bg-panel2 text-muted"
                    }`}
                  >
                    {it.type === "library" ? "From your library" : "Discover"}
                  </span>
                  {it.track_id ? (
                    <button
                      onClick={() => playLibraryItem(it.track_id!)}
                      className="text-xs text-accent hover:underline flex items-center gap-1"
                    >
                      <Library size={12} /> Play
                    </button>
                  ) : it.type === "discover" ? (
                    completedIds.has(`${it.artist} - ${it.title}`) ? (
                      <div className="flex items-center gap-2">
                        <span className="text-xs text-green-400 flex items-center gap-1">
                          <Check size={12} /> Downloaded
                        </span>
                        {completedTrackIds[`${it.artist} - ${it.title}`] && (
                          <button
                            onClick={() => playLibraryItem(completedTrackIds[`${it.artist} - ${it.title}`])}
                            className="text-xs text-accent hover:underline flex items-center gap-1"
                          >
                            <Library size={12} /> Play
                          </button>
                        )}
                      </div>
                    ) : (
                      <button
                        onClick={() => downloadItem(it.title, it.artist)}
                        disabled={downloadingIds.has(`${it.artist} - ${it.title}`)}
                        className="text-xs text-accent hover:underline flex items-center gap-1 disabled:opacity-50"
                      >
                        {downloadingIds.has(`${it.artist} - ${it.title}`) ? (
                          <Loader2 size={12} className="animate-spin" />
                        ) : (
                          <Download size={12} />
                        )}
                        {downloadingIds.has(`${it.artist} - ${it.title}`)
                          ? "Downloading…"
                          : "Download"}
                      </button>
                    )
                  ) : null}
                </div>
                <h3 className="font-semibold">{it.title}</h3>
                <p className="text-sm text-muted">{it.artist}</p>
                <p className="text-sm mt-2 leading-relaxed">{it.reason}</p>
              </div>
            ))}
          </div>
        </>
      ) : (
        <div className="bg-panel rounded-lg p-8 border border-panel2 text-center">
          <p className="text-muted mb-4">
            No recommendations yet. Click <strong>Generate</strong> to ask DeepSeek to
            analyze your listening history and suggest things you'll like.
          </p>
        </div>
      )}

      {playlistPreview && (
        <div className="bg-panel rounded-lg p-5 border border-panel2 space-y-4">
          <div className="flex items-start justify-between">
            <div>
              <h2 className="text-lg font-semibold flex items-center gap-2">
                <ListMusic className="text-accent" /> {playlistPreview.name}
              </h2>
              <p className="text-sm text-muted mt-1">{playlistPreview.description}</p>
            </div>
            <div className="flex items-center gap-2">
              {createdPlaylistId && (
                <a
                  href={`/playlists/${createdPlaylistId}`}
                  className="text-xs text-accent hover:underline"
                >
                  View Playlist
                </a>
              )}
              <button
                onClick={createAiPlaylist}
                disabled={creatingPlaylist || !!createdPlaylistId}
                className="px-3 py-2 bg-accent text-bg rounded-md font-medium text-sm flex items-center gap-2 disabled:opacity-60"
              >
                {creatingPlaylist ? (
                  <Loader2 size={14} className="animate-spin" />
                ) : (
                  <ListMusic size={14} />
                )}
                {creatingPlaylist ? "Creating…" : createdPlaylistId ? "Created" : "Create Playlist"}
              </button>
            </div>
          </div>

          <div className="space-y-2">
            {playlistPreview.tracks.map((t, i) => {
              const key = `${t.artist} - ${t.title}`;
              const status = playlistTrackStatus[key] || "pending";
              return (
                <div
                  key={i}
                  className="flex items-center justify-between bg-bg rounded-md px-3 py-2"
                >
                  <div className="flex items-center gap-3">
                    <span className="text-xs text-muted w-5">{i + 1}</span>
                    <div>
                      <p className="text-sm font-medium">{t.title}</p>
                      <p className="text-xs text-muted">{t.artist}</p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-muted hidden sm:block">{t.reason}</span>
                    {statusIcon(status)}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      <div className="bg-panel rounded-lg p-5 border border-panel2">
        <h2 className="text-sm uppercase tracking-wider text-muted mb-3">
          Chat about your taste
        </h2>
        <div className="space-y-3 max-h-64 overflow-y-auto mb-3">
          {chatLog.map((m, i) => (
            <div
              key={i}
              className={`text-sm ${
                m.role === "user" ? "text-text" : "text-muted"
              }`}
            >
              <strong className={m.role === "ai" ? "text-accent" : ""}>
                {m.role === "user" ? "You" : "Lexicon"}:
              </strong>{" "}
              <span className="whitespace-pre-wrap">{m.text}</span>
            </div>
          ))}
          {chatBusy && <p className="text-xs text-muted">thinking…</p>}
        </div>
        <form onSubmit={send} className="flex gap-2">
          <input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="Ask: what should I listen to right now?"
            className="flex-1 bg-bg border border-panel2 rounded-md px-3 py-2 outline-none focus:border-accent text-sm"
          />
          <button
            disabled={chatBusy}
            className="px-3 py-2 bg-accent text-bg rounded-md flex items-center gap-1 disabled:opacity-50"
          >
            <Send size={14} /> Send
          </button>
        </form>
      </div>
    </div>
  );
}
