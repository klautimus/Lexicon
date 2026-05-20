import { useEffect, useRef, useState } from "react";
import { Sparkles, Send, RefreshCw, Library, Download, Loader2, Check, ListMusic, HelpCircle, MessageSquare, Wand2 } from "lucide-react";
import { api, RecsPayload, PlaylistPayload } from "../lib/api";
import { useToast } from "../contexts/ToastContext";
import { usePlayer } from "../player/PlayerContext";
import { useDownloads } from "../contexts/DownloadContext";
import { useHelp } from "../contexts/HelpContext";

const EXAMPLE_PROMPTS = [
  "Make me a playlist for a road trip",
  "I'm in the mood for 90s grunge",
  "Something chill for studying",
  "Find me artists similar to my top plays",
  "Create a workout playlist with high energy tracks",
];

export default function RecsPage() {
  const toast = useToast();
  const player = usePlayer();
  const downloads = useDownloads();
  const { showHelp } = useHelp();
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; };
  }, []);
  const [recs, setRecs] = useState<RecsPayload | null>(null);
  const [createdAt, setCreatedAt] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);
  const [chatLog, setChatLog] = useState<{ role: "user" | "ai"; text: string }[]>([]);
  const [input, setInput] = useState("");
  const [chatBusy, setChatBusy] = useState(false);
  const [trackCount, setTrackCount] = useState(25);

  async function load() {
    const r = await api.recs();
    if (!mounted.current) return;
    if (!r.empty && r.data) {
      setRecs(r.data);
      setCreatedAt(r.created_at || null);
    }
  }
  useEffect(() => {
    load().catch(() => {});
  }, []);

  async function refresh() {
    setLoading(true);
    try {
      const r = await api.refreshRecs();
      if (!mounted.current) return;
      setRecs(r);
      setCreatedAt(Math.floor(Date.now() / 1000));
    } catch (e: any) {
      if (!mounted.current) return;
      toast.error("Failed: " + e.message);
    } finally {
      if (mounted.current) setLoading(false);
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
      if (!mounted.current) return;
      if (r.playlist) {
        setChatLog((l) => [...l, { role: "ai", text: r.reply }]);
        downloads.adoptPlaylistPreview(r.playlist);
      } else {
        setChatLog((l) => [...l, { role: "ai", text: r.reply }]);
      }
    } catch (e: any) {
      if (!mounted.current) return;
      setChatLog((l) => [...l, { role: "ai", text: "Error: " + e.message }]);
    } finally {
      if (mounted.current) setChatBusy(false);
    }
  }

  async function playLibraryItem(trackId: number) {
    const t = await api.track(trackId);
    player.play([t], 0);
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
        <div className="flex items-center gap-2">
          <h1 className="text-2xl font-semibold flex items-center gap-2">
            <Sparkles className="text-accent" /> Discover
          </h1>
          <button
            onClick={() => showHelp("discover.generate")}
            className="p-1 text-muted/50 hover:text-accent transition-colors rounded hover:bg-panel2/50"
            aria-label="Help: Discover"
          >
            <HelpCircle size={16} />
          </button>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => downloads.generateAiPlaylist(false, trackCount)}
            disabled={downloads.generatingPlaylist}
            className="px-3 py-2 bg-panel2 border border-panel2 hover:border-accent text-text rounded-md font-medium flex items-center gap-2 disabled:opacity-60 transition"
          >
            <ListMusic size={14} className={downloads.generatingPlaylist ? "animate-spin" : ""} />
            {downloads.generatingPlaylist ? "Thinking…" : "Generate Playlist"}
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

      {/* Track count slider */}
      <div className="flex items-center gap-3 bg-panel rounded-lg px-4 py-3 border border-panel2">
        <ListMusic size={14} className="text-accent" />
        <span className="text-sm text-muted">Tracks:</span>
        <input
          type="range"
          min={5}
          max={50}
          value={trackCount}
          onChange={(e) => setTrackCount(Number(e.target.value))}
          className="flex-1 accent-accent h-1"
        />
        <span className="text-sm font-medium w-8 text-right">{trackCount}</span>
        <button
          onClick={() => showHelp("discover.track-count")}
          className="p-0.5 text-muted/50 hover:text-accent transition-colors"
          aria-label="Help: Track Count"
        >
          <HelpCircle size={12} />
        </button>
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

          <div>
            <div className="flex items-center gap-2 mb-3">
              <h2 className="text-lg font-semibold">Recommended for You</h2>
              <button
                onClick={() => showHelp("discover.recommendations")}
                className="p-0.5 text-muted/50 hover:text-accent transition-colors"
                aria-label="Help: Recommendations"
              >
                <HelpCircle size={14} />
              </button>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {recs.items && recs.items.map((it, i) => (
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
                      downloads.completedIds.has(`${it.artist} - ${it.title}`) ? (
                        <div className="flex items-center gap-2">
                          <span className="text-xs text-green-400 flex items-center gap-1">
                            <Check size={12} /> Downloaded
                          </span>
                          {downloads.completedTrackIds[`${it.artist} - ${it.title}`] && (
                            <button
                              onClick={() => playLibraryItem(downloads.completedTrackIds[`${it.artist} - ${it.title}`])}
                              className="text-xs text-accent hover:underline flex items-center gap-1"
                            >
                              <Library size={12} /> Play
                            </button>
                          )}
                        </div>
                      ) : (
                        <button
                          onClick={() => downloads.downloadItem(it.title, it.artist)}
                          disabled={downloads.downloadingIds.has(`${it.artist} - ${it.title}`)}
                          className="text-xs text-accent hover:underline flex items-center gap-1 disabled:opacity-50"
                        >
                          {downloads.downloadingIds.has(`${it.artist} - ${it.title}`) ? (
                            <Loader2 size={12} className="animate-spin" />
                          ) : (
                            <Download size={12} />
                          )}
                          {downloads.downloadingIds.has(`${it.artist} - ${it.title}`)
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
          </div>
        </>
      ) : (
        <div className="bg-panel rounded-lg p-8 border border-panel2 text-center">
          <Sparkles size={32} className="mx-auto text-accent mb-3" />
          <h2 className="text-lg font-semibold mb-2">Discover New Music</h2>
          <p className="text-muted mb-4 max-w-md mx-auto">
            Click <strong>Generate</strong> to ask AI to analyze your listening history
            and suggest music you'll love. Connect Spotify or Apple Music for even
            better recommendations.
          </p>
          <p className="text-xs text-muted">
            You can also use the chat below to have a conversation about your taste,
            create custom playlists, and download any song for free.
          </p>
        </div>
      )}

      {downloads.playlistPreview && (
        <div className="bg-panel rounded-lg p-5 border border-panel2 space-y-4">
          <div className="flex items-start justify-between">
            <div>
              <h2 className="text-lg font-semibold flex items-center gap-2">
                <ListMusic className="text-accent" /> {downloads.playlistPreview.name}
              </h2>
              <p className="text-sm text-muted mt-1">{downloads.playlistPreview.description}</p>
            </div>
            <div className="flex items-center gap-2">
              {downloads.createdPlaylistId && (
                <a
                  href={`/playlists/${downloads.createdPlaylistId}`}
                  className="text-xs text-accent hover:underline"
                >
                  View Playlist
                </a>
              )}
              <button
                onClick={() => { downloads.generateAiPlaylist(true, trackCount); }}
                className="text-xs text-gray-400 hover:text-white transition-colors"
                disabled={downloads.generatingPlaylist}
              >
                {downloads.generatingPlaylist ? 'Generating...' : '\u{1F504} Regenerate'}
              </button>
              <button
                onClick={downloads.createAiPlaylist}
                disabled={downloads.creatingPlaylist || !!downloads.createdPlaylistId}
                className="px-3 py-2 bg-accent text-bg rounded-md font-medium text-sm flex items-center gap-2 disabled:opacity-60"
              >
                {downloads.creatingPlaylist ? (
                  <Loader2 size={14} className="animate-spin" />
                ) : (
                  <ListMusic size={14} />
                )}
                {downloads.creatingPlaylist ? "Creating…" : downloads.createdPlaylistId ? "Created" : "Create Playlist"}
              </button>
            </div>
          </div>

          <div className="space-y-2">
            {downloads.playlistPreview.tracks.map((t, i) => {
              const key = `${t.artist} - ${t.title}`;
              const status = downloads.playlistTrackStatus[key] || "pending";
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

      {/* Chat Section — Overhauled */}
      <div className="bg-panel rounded-lg p-5 border border-panel2">
        <div className="flex items-center gap-2 mb-2">
          <MessageSquare size={18} className="text-accent" />
          <h2 className="text-lg font-semibold">Chat with Lexicon</h2>
          <button
            onClick={() => showHelp("discover.chat")}
            className="p-0.5 text-muted/50 hover:text-accent transition-colors"
            aria-label="Help: Chat"
          >
            <HelpCircle size={14} />
          </button>
        </div>

        <div className="bg-bg/50 rounded-lg p-4 mb-4 border border-panel2">
          <p className="text-sm text-text leading-relaxed mb-2">
            <strong>Chat with Lexicon's AI</strong> to discover music, create playlists, and download songs — all through natural conversation.
          </p>
          <p className="text-xs text-muted leading-relaxed">
            Lexicon uses DeepSeek AI with knowledge of your listening history
            {status?.connected ? " and Spotify data" : ""}.
            {status?.appleConnected ? " and Apple Music data" : ""} Ask for playlists by mood, genre, or activity — then download any track for free.
          </p>
        </div>

        {/* Example prompts */}
        {chatLog.length === 0 && (
          <div className="mb-4">
            <p className="text-xs text-muted mb-2">Try one of these:</p>
            <div className="flex flex-wrap gap-2">
              {EXAMPLE_PROMPTS.map((prompt, i) => (
                <button
                  key={i}
                  onClick={() => {
                    setInput(prompt);
                  }}
                  className="text-xs px-3 py-1.5 bg-panel2 hover:bg-accent/20 hover:text-accent text-muted rounded-full transition-colors border border-panel2 hover:border-accent/30"
                >
                  <Wand2 size={10} className="inline mr-1" />
                  {prompt}
                </button>
              ))}
            </div>
          </div>
        )}

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
            placeholder="Ask for a playlist, recommendations, or say 'download that'…"
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
