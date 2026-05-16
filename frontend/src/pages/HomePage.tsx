import { useEffect, useState } from "react";
import { api, Stats, RecentPlay } from "../lib/api";

export default function HomePage() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [recent, setRecent] = useState<RecentPlay[]>([]);

  useEffect(() => {
    api.stats().then(setStats).catch(() => {});
    api.recent().then(setRecent).catch(() => {});
  }, []);

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight mb-1">Welcome back</h1>
        <p className="text-muted">Your private library, intelligently organized.</p>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <Stat label="Tracks" value={stats?.tracks ?? "—"} />
        <Stat label="Albums" value={stats?.albums ?? "—"} />
        <Stat label="Artists" value={stats?.artists ?? "—"} />
        <Stat label="Podcasts" value={stats?.podcasts ?? "—"} />
      </div>

      <div>
        <h2 className="text-lg font-semibold mb-3">Recently played</h2>
        {recent.length === 0 ? (
          <p className="text-muted text-sm">
            No plays yet — head to <strong>Music</strong> and play something to start
            building your taste profile.
          </p>
        ) : (
          <ul className="divide-y divide-panel2 rounded-lg border border-panel2 overflow-hidden">
            {recent.slice(0, 10).map((r) => (
              <li key={r.id} className="px-4 py-2 text-sm flex justify-between">
                <span className="truncate">
                  <span className="font-medium">{r.title}</span>{" "}
                  <span className="text-muted">— {r.artist}</span>
                </span>
                <span className="text-muted text-xs">
                  {new Date(r.started_at * 1000).toLocaleString()}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: number | string }) {
  return (
    <div className="bg-panel rounded-lg p-4 border border-panel2">
      <div className="text-xs text-muted uppercase tracking-wide">{label}</div>
      <div className="text-2xl font-semibold mt-1">{value}</div>
    </div>
  );
}
