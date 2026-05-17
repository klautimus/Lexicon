import { useEffect, useRef, useState } from "react";
import {
  Download,
  Loader2,
  CheckCircle2,
  XCircle,
  AlertCircle,
  X,
  ChevronDown,
  ChevronRight,
} from "lucide-react";
import { api, DownloadJob, DownloadStatus } from "../lib/api";

export default function DownloadsPage() {
  const [status, setStatus] = useState<DownloadStatus | null>(null);
  const [mode, setMode] = useState<"url" | "search">("url");
  const [url, setUrl] = useState("");
  const [query, setQuery] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [jobs, setJobs] = useState<DownloadJob[]>([]);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [logs, setLogs] = useState<Record<string, string[]>>({});
  const pollRef = useRef<number | null>(null);
  const expandedRef = useRef<Record<string, boolean>>(expanded);

  // Keep expandedRef in sync with expanded state
  useEffect(() => {
    expandedRef.current = expanded;
  }, [expanded]);

  async function refresh() {
    try {
      const [s, j] = await Promise.all([
        api.downloadStatus(),
        api.downloadJobs(),
      ]);
      setStatus(s);
      setJobs(j);
      // Pull full log for any expanded job that's running
      const currentExpanded = expandedRef.current;
      for (const id of Object.keys(currentExpanded)) {
        if (currentExpanded[id]) {
          api
            .downloadJob(id)
            .then((full) =>
              setLogs((prev) => ({ ...prev, [id]: full.log || [] }))
            )
            .catch(() => {});
        }
      }
    } catch {
      // ignore
    }
  }

  useEffect(() => {
    refresh();
    pollRef.current = window.setInterval(refresh, 1500);
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      if (mode === "url") {
        if (!url.trim()) return;
        await api.download(url.trim());
        setUrl("");
      } else {
        if (!query.trim()) return;
        await api.downloadSearch(query.trim());
        setQuery("");
      }
      refresh();
    } catch (err: any) {
      setError(err?.message || "Download failed");
    } finally {
      setSubmitting(false);
    }
  }

  async function cancel(id: string) {
    await api.downloadCancel(id);
    refresh();
  }

  function toggle(id: string) {
    setExpanded((prev) => ({ ...prev, [id]: !prev[id] }));
  }

  return (
    <div className="space-y-6 max-w-4xl">
      <h1 className="text-2xl font-semibold">Downloads</h1>

      <section className="bg-panel rounded-lg p-5 border border-panel2 space-y-4">
        {!status ? (
          <p className="text-sm text-muted">Loading…</p>
        ) : !status.configured ? (
          <div className="text-sm text-muted bg-panel2/50 rounded p-3 border border-panel2">
            <p className="mb-2 flex items-center gap-2 text-yellow-400">
              <AlertCircle size={16} /> SpotiFLAC isn't configured.
            </p>
            <p>
              Set <code className="text-accent">SPOTIFLAC_BIN</code> (path to
              the spotiflac binary) in <code className="text-accent">backend/.env</code>{" "}
              and restart the server.
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
                  placeholder="https://open.spotify.com/track/... or playlist/album"
                  className="flex-1 bg-panel2 border border-panel2 rounded px-3 py-2 text-sm focus:outline-none focus:border-accent"
                  disabled={submitting}
                />
              ) : (
                <input
                  type="text"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder='Paradise By the Dashboard Light - Meat Loaf'
                  className="flex-1 bg-panel2 border border-panel2 rounded px-3 py-2 text-sm focus:outline-none focus:border-accent"
                  disabled={submitting}
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
                Download
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

      <section>
        <h2 className="text-lg font-semibold mb-3">Recent jobs</h2>
        {jobs.length === 0 ? (
          <p className="text-sm text-muted">No jobs yet.</p>
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
              />
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

function JobRow({
  job,
  expanded,
  onToggle,
  log,
  onCancel,
}: {
  job: DownloadJob;
  expanded: boolean;
  onToggle: () => void;
  log?: string[];
  onCancel: () => void;
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

  return (
    <div className="bg-panel rounded border border-panel2 overflow-hidden">
      <div className="flex items-center gap-3 px-4 py-3">
        <Icon size={18} className={`${color} ${spinning ? "animate-spin" : ""}`} />
        <div className="flex-1 min-w-0">
          <div className="text-sm truncate font-mono">{job.url}</div>
          <div className="text-xs text-muted flex items-center gap-2 flex-wrap">
            <span>{job.status}</span>
            <span>·</span>
            <span>{new Date(job.started_at * 1000).toLocaleTimeString()}</span>
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
        </div>
        {(job.status === "running" || job.status === "queued") && (
          <button
            onClick={onCancel}
            className="text-xs text-muted hover:text-red-400 px-2 py-1"
          >
            Cancel
          </button>
        )}
        <button
          onClick={onToggle}
          className="text-muted hover:text-text"
          aria-label="Toggle log"
        >
          {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
        </button>
      </div>
      {expanded && (
        <pre className="bg-black/40 text-xs text-muted px-4 py-3 max-h-64 overflow-auto whitespace-pre-wrap break-all">
          {(log && log.length > 0 ? log.join("\n") : "(no output yet)")}
        </pre>
      )}
    </div>
  );
}
