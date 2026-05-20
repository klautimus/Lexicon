import { useEffect, useState } from "react";
import {
  api,
  Overview,
  TopArtist,
  TopTrack,
  TopGenre,
  HeatCell,
} from "../lib/api";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
} from "recharts";

const COLORS = ["#e6b450", "#8a6d2f", "#39bae6", "#73d0ff", "#ffa759", "#d4bfff", "#95e6cb", "#f29e74"];
const DAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

export default function AnalyticsPage() {
  const [ov, setOv] = useState<Overview | null>(null);
  const [artists, setArtists] = useState<TopArtist[]>([]);
  const [tracks, setTracks] = useState<TopTrack[]>([]);
  const [genres, setGenres] = useState<TopGenre[]>([]);
  const [heat, setHeat] = useState<HeatCell[]>([]);

  useEffect(() => {
    api.overview().then(setOv).catch(() => {});
    api.topArtists().then(setArtists).catch(() => {});
    api.topTracks().then(setTracks).catch(() => {});
    api.topGenres().then(setGenres).catch(() => {});
    api.heatmap().then(setHeat).catch(() => {});
  }, []);

  const heatLookup = new Map<string, number>();
  let heatMax = 0;
  for (const c of heat) {
    heatLookup.set(`${c.dow}-${c.hour}`, c.plays);
    if (c.plays > heatMax) heatMax = c.plays;
  }

  return (
    <div className="space-y-8">
      <h1 className="text-2xl font-semibold">Listening Analytics</h1>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <Stat label="Total plays" value={ov?.total_plays ?? "—"} />
        <Stat label="Unique tracks" value={ov?.unique_tracks ?? "—"} />
        <Stat
          label="Listen time"
          value={ov ? `${Math.round(ov.listen_sec / 3600)}h` : "—"}
        />
        <Stat label="Completed %" value={ov ? `${ov.completed_pct}%` : "—"} />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Panel title="Top artists">
          <ResponsiveContainer width="100%" height={260}>
            <BarChart data={artists.slice(0, 10)} layout="vertical" margin={{ left: 20 }}>
              <XAxis type="number" stroke="#7a8086" />
              <YAxis type="category" dataKey="artist" stroke="#7a8086" width={100} />
              <Tooltip contentStyle={{ background: "#14171c", border: "1px solid #2a2f37" }} />
              <Bar dataKey="plays" fill="#e6b450" />
            </BarChart>
          </ResponsiveContainer>
        </Panel>

        <Panel title="Top genres">
          <ResponsiveContainer width="100%" height={260}>
            <PieChart>
              <Pie data={genres} dataKey="plays" nameKey="genre" outerRadius={100} label>
                {genres.map((_, i) => (
                  <Cell key={i} fill={COLORS[i % COLORS.length]} />
                ))}
              </Pie>
              <Tooltip contentStyle={{ background: "#14171c", border: "1px solid #2a2f37" }} />
            </PieChart>
          </ResponsiveContainer>
        </Panel>
      </div>

      <Panel title="When you listen">
        <div className="overflow-x-auto">
          <table className="text-xs">
            <thead>
              <tr>
                <th className="px-1 py-1 text-muted"></th>
                {Array.from({ length: 24 }, (_, h) => (
                  <th key={h} className="px-1 py-1 text-muted font-normal">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {DAYS.map((d, di) => (
                <tr key={d}>
                  <td className="px-2 py-1 text-muted">{d}</td>
                  {Array.from({ length: 24 }, (_, h) => {
                    const v = heatLookup.get(`${di}-${h}`) || 0;
                    const intensity = heatMax > 0 ? v / heatMax : 0;
                    return (
                      <td key={h} className="p-0">
                        <div
                          title={`${d} ${h}:00 — ${v} plays`}
                          className="w-5 h-5 m-px rounded"
                          style={{
                            background: `rgba(230, 180, 80, ${intensity * 0.85 + (v ? 0.1 : 0.02)})`,
                          }}
                        />
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Panel>

      <Panel title="Top tracks">
        <ul className="divide-y divide-panel2">
          {tracks.map((t, i) => (
            <li key={t.id} className="px-2 py-2 flex justify-between text-sm">
              <span>
                <span className="text-muted w-6 inline-block">{i + 1}.</span>
                <strong>{t.title}</strong>{" "}
                <span className="text-muted">— {t.artist}</span>
              </span>
              <span className="text-muted">{t.plays} plays</span>
            </li>
          ))}
        </ul>
      </Panel>
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

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-panel rounded-lg p-4 border border-panel2">
      <h2 className="text-sm font-semibold mb-3 text-muted uppercase tracking-wider">{title}</h2>
      {children}
    </div>
  );
}
