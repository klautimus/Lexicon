import { useEffect, useState, useRef } from "react";
import { Download, Loader2, CheckCircle2 } from "lucide-react";
import { api, DownloadJob } from "../lib/api";

export default function DownloadProgressBar() {
  const [items, setItems] = useState<DownloadJob[]>([]);
  const pollRef = useRef<number | null>(null);
  const [showComplete, setShowComplete] = useState(false);

  const prevJobsRef = useRef<DownloadJob[]>([]);

  useEffect(() => {
    const poll = async () => {
      try {
        const jobs = await api.downloadProgress();

        // Flash "complete" only when transitioning from active to idle
        if (jobs.length === 0 && prevJobsRef.current.length > 0) {
          setShowComplete(true);
          setTimeout(() => setShowComplete(false), 3000);
        }
        prevJobsRef.current = jobs;
        setItems(jobs);
      } catch {
        // Silently fail — progress bar is non-critical
      }
    };

    // Poll every 2 seconds
    pollRef.current = window.setInterval(poll, 2000);
    poll(); // Immediate first poll

    return () => {
      if (pollRef.current !== null) {
        window.clearInterval(pollRef.current);
        pollRef.current = null;
      }
    };
  }, []);

  // Don't render anything when no active downloads
  if (items.length === 0 && !showComplete) {
    return null;
  }

  // Show completion flash
  if (showComplete && items.length === 0) {
    return (
      <div className="bg-green-900/30 border-b border-green-800/50 px-4 py-1.5 flex items-center gap-2 text-green-400 text-xs animate-pulse">
        <CheckCircle2 size={14} />
        <span>Downloads complete</span>
      </div>
    );
  }

  const runningCount = items.filter((j) => j.status === "running").length;
  const queuedCount = items.filter((j) => j.status === "queued").length;

  return (
    <div className="bg-panel2/80 border-b border-black/20 px-4 py-1.5 space-y-1">
      {/* Summary line */}
      <div className="flex items-center gap-2 text-xs text-muted">
        <Download size={14} className="text-accent" />
        <span>
          Downloading{" "}
          {runningCount > 0 && (
            <span className="text-text font-medium">{runningCount}</span>
          )}{" "}
          {queuedCount > 0 && (
            <span>
              ({queuedCount} queued)
            </span>
          )}
          {items.length === 1 && (
            <span className="text-text/60 ml-1 truncate max-w-[300px] inline-block align-bottom">
              — {items[0].url}
            </span>
          )}
        </span>
        {runningCount > 0 && (
          <Loader2 size={12} className="animate-spin text-accent ml-auto" />
        )}
      </div>

      {/* Progress bars per job (only when 2+ jobs or single job has progress) */}
      {(items.length > 1 ||
        (items.length === 1 && (items[0].progress ?? 0) > 0)) && (
        <div className="space-y-0.5">
          {items.map((item) => (
            <div key={item.id} className="flex items-center gap-2">
              <span className="text-[10px] text-muted/60 w-5 text-right tabular-nums">
                {item.status === "queued"
                  ? "—"
                  : `${Math.round(item.progress ?? 0)}%`}
              </span>
              <div className="flex-1 h-1 bg-black/30 rounded-full overflow-hidden">
                <div
                  className={`h-full rounded-full transition-all duration-500 ${
                    item.status === "queued"
                      ? "bg-muted/30"
                      : "bg-accent"
                  }`}
                  style={{
                    width: `${
                      item.status === "queued"
                        ? 0
                        : Math.max(item.progress ?? 0, 2)
                    }%`,
                  }}
                />
              </div>
              <span className="text-[10px] text-muted/60 truncate max-w-[200px]">
                {item.progress_label && item.status === "running"
                  ? item.progress_label
                  : item.url}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
