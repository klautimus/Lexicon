import { useEffect, useRef, useState, useCallback, memo } from "react";
import {
  Download,
  Loader2,
  CheckCircle2,
  XCircle,
  AlertCircle,
  X,
  ChevronDown,
  ChevronRight,
  HelpCircle,
  RotateCcw,
  Trash2,
} from "lucide-react";
import { api, DownloadJob, DownloadStatus } from "../lib/api";
import { useHelp } from "../contexts/HelpContext";
import { useToast } from "../contexts/ToastContext";

function capitalize(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

export default function DownloadsPage() {
  const { showHelp } = useHelp();
  const toast = useToast();
  const [status, setStatus] = useState<DownloadStatus | null>(null);
  const [mode, setMode] = useState<"url" | "search">("url");
  const [url, setUrl] = useState("");
  const [query, setQuery] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [jobs, setJobs] = useState<DownloadJob[]>([]);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [logs, setLogs] = useState<Record<string, string[]>>({});
  const [loading, setLoading] = useState(true);
  const pollRef = useRef<number | null>(null);
  const expandedRef = useRef<Record<string, boolean>>(expanded);
  const mountedRef = useRef(true);

  useEffect(() => {
    expandedRef.current = expanded;
  }, [expanded]);

  useEffect(() => {
    mountedRef.current = true;
    refresh();
    const id = window.setInterval(refresh, 1500);
    pollRef.current = id;
    return () => {
      mountedRef.current = false;
      clearInterval(id);
      pollRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function refresh() {
    try {
      const [s, j] = await Promise.all([
        api.downloadStatus(),
        api.downloadJobs(),
      ]);
      if (!mountedRef.current) return;
      setStatus(s);
      setJobs(j);
      setLoading(false);
      const currentExpanded = expandedRef.current;
      for (const id of Object.keys(currentExpanded)) {
        if (currentExpanded[id]) {
          api
            .downloadJob(id)
            .then((full) => {
              if (!mountedRef.current) return;
              setLogs((prev) => ({ ...prev, [id]: full.log || [] }));
            })
            .catch((err) => {
              console.error(`[DownloadsPage] Failed to fetch job ${id}:`, err);
            });
        }
      }
    } catch (err) {
      console.error("[DownloadsPage] refresh failed:", err);
    }
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const trimmedUrl = mode === "url" ? url.trim() : "";
    const trimmedQuery = mode === "search" ? query.trim() : "";
    if (mode === "url" && !trimmedUrl) {
      setSubmitting(false);
      return;
    }
    if (mode === "search" && !trimmedQuery) {
      setSubmitting(false);
      return;
    }
    setSubmitting(true);
    try {
      if (mode === "url") {
        await api.download(trimmedUrl);
        setUrl("");
        toast.success("Download started");
      } else {
        await api.downloadSearch(trimmedQuery);
        setQuery("");
        toast.success("Download started");
      }
      refresh();
    } catch (err: any) {
      console.error("[DownloadsPage] submit failed:", err);
      const msg = err?.message || "Download failed";
      setError(msg);
      toast.error(msg);
    } finally {
      setSubmitting(false);
    }
  }

  async function cancel(id: string) {
    if (!window.confirm("Cancel this download?")) return;
    try {
      await api.downloadCancel(id);
      toast.info("Download cancelled");
      refresh();
    } catch (err: any) {
      console.error(`[DownloadsPage] cancel failed for ${id}:`, err);
      toast.error("Failed to cancel download");
    }
  }

  async function retry(job: DownloadJob) {
    try {
      if (job.mode === "search" || job.is_search) {
        await api.downloadSearch(job.url);
      } else {
        await api.download(job.url);
      }
      toast.success("Retrying download");
      refresh();
    } catch (err: any) {
      console.error(`[DownloadsPage] retry failed for ${job.id}:`, err);
      toast.error("Failed to retry download");
    }
  }

  const removeJob = useCallback((id: string) => {
    setJobs((prev) => prev.filter((j) => j.id !== id));
    setLogs((prev) => {
      const next = { ...prev };
      delete next[id];
      return next;
    });
    setExpanded((prev) => {
      const next = { ...prev };
      delete next[id];
      return next;
    });
  }, []);

  const clearCompleted = useCallback(() => {
    setJobs((prev) => prev.filter((j) => j.status !== "succeeded"));
  }, []);

  const clearFailed = useCallback(() => {
    setJobs((prev) => prev.filter((j) => j.status !== "failed" && j.status !== "cancelled"));
  }, []);

  const toggle = useCallback((id: string) => {
    setExpanded((prev) => ({ ...prev, [id]: !prev[id] }));
  }, []);

  const completedCount = jobs.filter((j) => j.status === "succeeded").length;
  const failedCount = jobs.filter((j) => j.status === "failed" || j.status === "cancelled").length;

  return (
    <div className="space-y-6 max-w-4xl">
      <div className="flex items-center gap-2">
        <h1 className="text-2xl font-semibold">Downloads</h1>
        <button
          onClick={() => showHelp("downloads.mode")}
          className="p-1 text-muted/50 hover:text-accent transition-colors rounded hover:bg-panel2/50"
          aria-label="Help: Downloads"
        >
          <HelpCircle size={16} />
        </button>
      </div>

      <section className="bg-panel rounded-lg p-5 border border-panel2 space-y-4">
        {!status ? (
          <p className="text-sm text-muted">Loading…</p>
        ) : !status.configured ? (
          <div className="text-sm text-muted bg-panel2/50 rounded p-3 border border-panel2">
            <p className="mb-2 flex items-center gap-2 text-yellow-400">
              <AlertCircle size={16} /> SpotiFLAC isn't configured.
            </p>
            <p>
              In <code className="text-accent">backend/.env</code> set{" "}
              <code className="text-accent">SPOTIFLAC_BIN</code> (path to binary),{" "}
              <code className="text-accent">SPOTIFLAC_OUTPUT</code> (download directory), and optionally{" "}
              <code className="text-accent">SPOTIFLAC_SERVICE</code> (qobuz, amazon, or tidal).
              Restart the server after configuring.
            </p>
          </div>
        ) : (
          <>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div>
                <div className="text-xs uppercase tracking-wide text-muted mb-1">
                  Output directory
                </div>
                <div className="font-mono text-sm break-all">
                  {status.output}
                </div>
              </div>
              <div>
                <div className="text-xs uppercase tracking-wide text-muted mb-1">
                  Fallback (spotDL)
                </div>
                <div className="text-sm">
                  {status.fallback_enabled ? (
                    <span className="text-green-400">
                      Enabled ({status.spotdl_format || "mp3"})
                    </span>
                  ) : (
                    <span className="text-muted">
                      Disabled — set SPOTDL_BIN in .env
                    </span>
                  )}
                </div>
              </div>
            </div>
            <div className="flex gap-2 text-sm">
              <button
                type="button"
                onClick={() => setMode("url")}
                className={`px-3 py-1.5 rounded ${mode === "url" ? "bg-accent text-black" : "bg-panel2 text-muted hover:text-text"}`}
              >
                Spotify URL
              </button>
              <button
                type="button"
                onClick={() => setMode("search")}
                className={`px-3 py-1.5 rounded ${mode === "search" ? "bg-accent text-black" : "bg-panel2 text-muted hover:text-text"}`}
              >
                Search by name
              </button>
            </div>
            <form onSubmit={submit} className="flex gap-2">
              {mode === "url" ? (
                <input
                  type="url"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  placeholder="https://open.spotify.com/track/..."
                  className="flex-1 bg-panel2 border border-panel2 rounded px-3 py-2 text-sm focus:outline-none focus:border-accent"
                  disabled={submitting}
                  aria-label="Spotify URL"
                />
              ) : (
                <input
                  type="text"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder="Song or podcast name"
                  className="flex-1 bg-panel2 border border-panel2 rounded px-3 py-2 text-sm focus:outline-none focus:border-accent"
                  disabled={submitting}
                  aria-label="Search query"
                />
              )}
              <button
                type="submit"
                disabled={submitting || (mode === "url" ? !url.trim() : !query.trim())}
                className="px-4 py-2 bg-accent hover:opacity-90 text-black font-medium rounded flex items-center gap-2 disabled:opacity-50"
              >
                {submitting ? (
                  <Loader2 size={16} className="animate-spin" />
                ) : (
                  <Download size={16} />
                )}
                <span className="hidden sm:inline">Download</span>
              </button>
            </form>
            {error && (
              <div className="text-sm text-red-400 bg-red-400/10 border border-red-400/30 rounded px-3 py-2">
                {error}
              </div>
            )}
            {mode === "url" ? (
              <p className="text-xs text-muted">
                Paste a Spotify <strong>track</strong>, <strong>album</strong>, or{" "}
                <strong>playlist</strong> URL. Files are downloaded as FLAC and
                your library is rescanned automatically when the job finishes.
              </p>
            ) : (
              <p className="text-xs text-muted">
                Type a song or podcast name. DeepSeek parses the query and yt-dlp
                searches YouTube directly — no Spotify account needed.
              </p>
            )}
          </>
        )}
      </section>

      <section aria-live="polite" aria-label="Download jobs">
        <div className="flex items-center justify-between mb-3">
          <div className="flex items-center gap-2">
            <h2 className="text-lg font-semibold">Recent jobs</h2>
            <button
              onClick={() => showHelp("downloads.jobs")}
              className="p-1 text-muted/50 hover:text-accent transition-colors rounded hover:bg-panel2/50"
              aria-label="Help: Download Jobs"
            >
              <HelpCircle size={16} />
            </button>
          </div>
          <div className="flex gap-2">
            {completedCount > 0 && (
              <button
                onClick={clearCompleted}
                className="text-xs text-muted hover:text-text px-2 py-1 rounded bg-panel2/50 hover:bg-panel2 transition-colors"
              >
                Clear completed ({completedCount})
              </button>
            )}
            {failedCount > 0 && (
              <button
                onClick={clearFailed}
                className="text-xs text-muted hover:text-text px-2 py-1 rounded bg-panel2/50 hover:bg-panel2 transition-colors"
              >
                Clear failed ({failedCount})
              </button>
            )}
          </div>
        </div>
        {loading ? (
          <p className="text-sm text-muted">Loading jobs…</p>
        ) : jobs.length === 0 ? (
          <div className="text-sm text-muted bg-panel rounded-lg p-6 border border-panel2 text-center">
            <p className="mb-1">No jobs yet.</p>
            <p className="text-xs text-muted/60">Paste a Spotify URL above to start downloading, or try searching for a song by name.</p>
          </div>
        ) : (
          <div className="space-y-2">
            {jobs.map((job) => (
              <JobRow
                key={job.id}
                job={job}
                expanded={!!expanded[job.id]}
                onToggle={() => toggle(job.id)}
                log={logs[job.id]}
                onCancel={() => cancel(job.id)}
                onRetry={() => retry(job)}
                onRemove={() => removeJob(job.id)}
              />
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

const JobRow = memo(function JobRow({
  job,
  expanded,
  onToggle,
  log,
  onCancel,
  onRetry,
  onRemove,
}: {
  job: DownloadJob;
  expanded: boolean;
  onToggle: () => void;
  log?: string[];
  onCancel: () => void;
  onRetry: () => void;
  onRemove: () => void;
}) {
  const Icon =
    job.status === "succeeded"
      ? CheckCircle2
      : job.status === "failed"
      ? XCircle
      : job.status === "cancelled"
      ? X
      : Loader2;
  const color =
    job.status === "succeeded"
      ? "text-green-400"
      : job.status === "failed"
      ? "text-red-400"
      : job.status === "cancelled"
      ? "text-muted"
      : "text-accent";
  const spinning = job.status === "running" || job.status === "queued";

  const hasProgress = (job.progress ?? 0) > 0;

  return (
    <div
      className="bg-panel rounded border border-panel2 overflow-hidden"
      aria-label={`Download job: ${job.url} — ${capitalize(job.status)}`}
    >
      <div className="flex items-center gap-3 px-4 py-3">
        <Icon size={18} className={`${color} ${spinning ? "animate-spin" : ""}`} />
        <div className="flex-1 min-w-0">
          <div className="text-sm truncate font-mono">{job.url}</div>
          <div className="text-xs text-muted flex items-center gap-2 flex-wrap">
            <span>{capitalize(job.status)}</span>
            <span>·</span>
            <span>{new Date(job.started_at * 1000).toLocaleTimeString()}</span>
            {job.kind && job.kind !== "music" && (
              <>
                <span>·</span>
                <span className="px-1.5 rounded bg-purple-400/20 text-purple-300">
                  {job.kind}
                </span>
              </>
            )}
            {job.tool && (
              <>
                <span>·</span>
                <span
                  className={
                    job.used_fallback
                      ? "px-1.5 rounded bg-yellow-400/20 text-yellow-400"
                      : "px-1.5 rounded bg-panel2 text-muted"
                  }
                >
                  {job.tool}
                </span>
              </>
            )}
            {job.error && (
              <span className="text-red-400 truncate"> · {job.error}</span>
            )}
          </div>
          {/* Progress bar */}
          {hasProgress && (job.status === "running" || job.status === "queued") && (
            <div className="mt-1.5 flex items-center gap-2">
              <div className="flex-1 h-1 bg-black/30 rounded-full overflow-hidden">
                <div
                  className="h-full rounded-full bg-accent transition-all duration-500"
                  style={{ width: `${Math.max(job.progress ?? 0, 2)}%` }}
                />
              </div>
              <span className="text-[10px] text-muted tabular-nums w-10 text-right">
                {job.progress_label || `${Math.round(job.progress ?? 0)}%`}
              </span>
            </div>
          )}
        </div>
        <div className="flex items-center gap-1">
          {(job.status === "running" || job.status === "queued") && (
            <button
              onClick={onCancel}
              className="text-xs text-muted hover:text-red-400 px-2 py-1.5 rounded bg-panel2/50 hover:bg-red-400/10 transition-colors"
              aria-label={`Cancel download: ${job.url}`}
            >
              Cancel
            </button>
          )}
          {job.status === "failed" && (
            <button
              onClick={onRetry}
              className="text-xs text-muted hover:text-accent px-2 py-1.5 rounded bg-panel2/50 hover:bg-accent/10 transition-colors flex items-center gap-1"
              aria-label={`Retry download: ${job.url}`}
            >
              <RotateCcw size={12} />
              Retry
            </button>
          )}
          {(job.status === "succeeded" || job.status === "cancelled") && (
            <button
              onClick={onRemove}
              className="text-xs text-muted hover:text-red-400 px-2 py-1.5 rounded bg-panel2/50 hover:bg-red-400/10 transition-colors"
              aria-label={`Remove job: ${job.url}`}
            >
              <Trash2 size={12} />
            </button>
          )}
          <button
            onClick={onToggle}
            className="text-muted hover:text-text p-1.5"
            aria-label={expanded ? "Hide log" : "Show log"}
            aria-expanded={expanded}
          >
            {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          </button>
        </div>
      </div>
      {expanded && (
        <pre className="bg-black/40 text-xs text-muted px-4 py-3 max-h-64 overflow-auto whitespace-pre-wrap break-all">
          {(log && log.length > 0 ? log.join("\n") : "(no output yet)")}
        </pre>
      )}
    </div>
  );
});
