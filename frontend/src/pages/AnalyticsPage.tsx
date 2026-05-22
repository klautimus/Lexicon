import { useEffect, useState, useMemo, useCallback, memo } from "react";
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
  Legend,
} from "recharts";
import { HelpCircle, RefreshCw, Download, Play } from "lucide-react";
import { useHelp } from "../contexts/HelpContext";

const COLORS = ["#e6b450", "#8a6d2f", "#39bae6", "#73d0ff", "#ffa759", "#d4bfff", "#95e6cb", "#f29e74"];
const DAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

const CACHE_TTL = 60_000; // 60 seconds
const CACHE_KEY = "analytics_cache_v1";

interface CachedData {
  ts: number;
  ov: Overview;
  artists: TopArtist[];
  tracks: TopTrack[];
  genres: TopGenre[];
  heat: HeatCell[];
}

function loadCache(): CachedData | null {
  try {
    const raw = localStorage.getItem(CACHE_KEY);
    if (!raw) return null;
    const d: CachedData = JSON.parse(raw);
    if (Date.now() - d.ts < CACHE_TTL) return d;
  } catch { /* ignore */ }
  return null;
}

function saveCache(d: CachedData) {
  try { localStorage.setItem(CACHE_KEY, JSON.stringify(d)); } catch { /* ignore */ }
}

function clearCache() {
  try { localStorage.removeItem(CACHE_KEY); } catch { /* ignore */ }
}

type SectionError = { overview: boolean; artists: boolean; tracks: boolean; genres: boolean; heatmap: boolean };

function formatListenTime(sec: number): string {
  if (sec < 3600) return `${Math.round(sec / 60)}m`;
  return `${Math.round(sec / 3600)}h`;
}

function exportCSV(ov: Overview | null, artists: TopArtist[], tracks: TopTrack[], genres: TopGenre[], heat: HeatCell[]) {
  const lines: string[] = [];
  lines.push("Metric,Value");
  lines.push(`Total plays,${ov?.total_plays ?? ""}`);
  lines.push(`Unique tracks,${ov?.unique_tracks ?? ""}`);
  lines.push(`Listen time,${ov ? formatListenTime(ov.listen_sec) : ""}`);
  lines.push(`Completed %,${ov ? ov.completed_pct + "%" : ""}`);
  lines.push("");
  lines.push("Top Artists");
  lines.push("Artist,Plays");
  for (const a of artists) lines.push(`"${a.artist}",${a.plays}`);
  lines.push("");
  lines.push("Top Tracks");
  lines.push("Title,Artist,Plays");
  for (const t of tracks) lines.push(`"${t.title}","${t.artist}",${t.plays}`);
  lines.push("");
  lines.push("Top Genres");
  lines.push("Genre,Plays");
  for (const g of genres) lines.push(`"${g.genre}",${g.plays}`);
  lines.push("");
  lines.push("Heatmap");
  lines.push("Day,Hour,Plays");
  for (const h of heat) lines.push(`${DAYS[h.dow]},${h.hour},${h.plays}`);
  const blob = new Blob([lines.join("\n")], { type: "text/csv" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "analytics.csv";
  a.click();
  URL.revokeObjectURL(url);
}

export default function AnalyticsPage() {
  const { showHelp } = useHelp();
  const [ov, setOv] = useState<Overview | null>(null);
  const [artists, setArtists] = useState<TopArtist[]>([]);
  const [tracks, setTracks] = useState<TopTrack[]>([]);
  const [genres, setGenres] = useState<TopGenre[]>([]);
  const [heat, setHeat] = useState<HeatCell[]>([]);
  const [loading, setLoading] = useState(true);
  const [sectionErrors, setSectionErrors] = useState<SectionError>({ overview: false, artists: false, tracks: false, genres: false, heatmap: false });
  const [hasAnyError, setHasAnyError] = useState(false);

  const loadData = useCallback(() => {
    setLoading(true);
    setSectionErrors({ overview: false, artists: false, tracks: false, genres: false, heatmap: false });
    setHasAnyError(false);
    setOv(null);
    setArtists([]);
    setTracks([]);
    setGenres([]);
    setHeat([]);

    let errors = 0;
    const trackErr = (key: keyof SectionError) => {
      setSectionErrors(prev => ({ ...prev, [key]: true }));
      errors++;
      if (errors >= 5) setHasAnyError(true);
    };

    Promise.all([
      api.overview().then(setOv).catch(() => trackErr("overview")),
      api.topArtists().then(setArtists).catch(() => trackErr("artists")),
      api.topTracks().then(setTracks).catch(() => trackErr("tracks")),
      api.topGenres().then(setGenres).catch(() => trackErr("genres")),
      api.heatmap().then(setHeat).catch(() => trackErr("heatmap")),
    ]).finally(() => {
      setLoading(false);
    });
  }, []);

  useEffect(() => {
    const cached = loadCache();
    if (cached) {
      setOv(cached.ov);
      setArtists(cached.artists);
      setTracks(cached.tracks);
      setGenres(cached.genres);
      setHeat(cached.heat);
      setLoading(false);
      return;
    }
    loadData();
  }, [loadData]);

  // Save to cache when data loads
  useEffect(() => {
    if (!loading && ov) {
      saveCache({ ts: Date.now(), ov, artists, tracks, genres, heat });
    }
  }, [loading, ov, artists, tracks, genres, heat]);

  const { heatLookup, heatMax } = useMemo(() => {
    const m = new Map<string, number>();
    let mx = 0;
    for (const c of heat) {
      m.set(`${c.dow}-${c.hour}`, c.plays);
      if (c.plays > mx) mx = c.plays;
    }
    return { heatLookup: m, heatMax: mx };
  }, [heat]);

  const handleRefresh = useCallback(() => {
    clearCache();
    loadData();
  }, [loadData]);

  const handleExport = useCallback(() => {
    exportCSV(ov, artists, tracks, genres, heat);
  }, [ov, artists, tracks, genres, heat]);

  const handlePlayTrack = useCallback((trackId: number) => {
    // Navigate to the track — the player will pick it up
    window.location.href = `#/music?track=${trackId}`;
  }, []);

  if (loading) {
    return (
      <div className="space-y-8">
        <div className="flex items-center gap-2">
          <h1 className="text-2xl font-semibold">Listening Analytics</h1>
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          {[1,2,3,4].map(i => (
            <div key={i} className="bg-panel rounded-lg p-4 border border-panel2 animate-pulse">
              <div className="h-3 bg-panel2 rounded w-20 mb-2" />
              <div className="h-7 bg-panel2 rounded w-16" />
            </div>
          ))}
        </div>
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {[1,2].map(i => (
            <div key={i} className="bg-panel rounded-lg p-4 border border-panel2 animate-pulse">
              <div className="h-4 bg-panel2 rounded w-24 mb-3" />
              <div className="h-[260px] bg-panel2 rounded" />
            </div>
          ))}
        </div>
        <div className="bg-panel rounded-lg p-4 border border-panel2 animate-pulse">
          <div className="h-4 bg-panel2 rounded w-32 mb-3" />
          <div className="h-40 bg-panel2 rounded" />
        </div>
        <div className="bg-panel rounded-lg p-4 border border-panel2 animate-pulse">
          <div className="h-4 bg-panel2 rounded w-28 mb-3" />
          <div className="h-48 bg-panel2 rounded" />
        </div>
      </div>
    );
  }

  if (hasAnyError) {
    return (
      <div className="space-y-8">
        <div className="flex items-center gap-2">
          <h1 className="text-2xl font-semibold">Listening Analytics</h1>
        </div>
        <div className="bg-panel rounded-lg p-8 border border-panel2 text-center">
          <p className="text-muted">Failed to load analytics. <button onClick={handleRefresh} className="text-accent hover:underline">Retry</button></p>
        </div>
      </div>
    );
  }

  const allEmpty = !ov || (ov.total_plays === 0 && artists.length === 0 && tracks.length === 0 && genres.length === 0);

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-2 flex-wrap">
        <h1 className="text-2xl font-semibold">Listening Analytics</h1>
        <button
          onClick={handleRefresh}
          className="p-1 text-muted/70 hover:text-accent transition-colors rounded hover:bg-panel2/50"
          aria-label="Refresh analytics"
          title="Refresh"
        >
          <RefreshCw size={16} />
        </button>
        <button
          onClick={handleExport}
          className="p-1 text-muted/70 hover:text-accent transition-colors rounded hover:bg-panel2/50"
          aria-label="Export analytics as CSV"
          title="Export CSV"
        >
          <Download size={16} />
        </button>
        <button
          onClick={() => showHelp("analytics.charts")}
          className="p-1 text-muted/70 hover:text-accent transition-colors rounded hover:bg-panel2/50"
          aria-label="Help: Analytics"
        >
          <HelpCircle size={16} />
        </button>
      </div>

      {allEmpty ? (
        <div className="bg-panel rounded-lg p-8 border border-panel2 text-center">
          <p className="text-muted">No data yet — start playing music to see your analytics.</p>
        </div>
      ) : (
        <>
          <ul role="list" className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <li><Stat label="Total plays" value={sectionErrors.overview ? "Error" : (ov?.total_plays ?? "—")} error={sectionErrors.overview} /></li>
            <li><Stat label="Unique tracks" value={sectionErrors.overview ? "Error" : (ov?.unique_tracks ?? "—")} error={sectionErrors.overview} /></li>
            <li><Stat label="Listen time" value={sectionErrors.overview ? "Error" : (ov ? formatListenTime(ov.listen_sec) : "—")} error={sectionErrors.overview} /></li>
            <li><Stat label="Completed %" value={sectionErrors.overview ? "Error" : (ov ? `${ov.completed_pct}%` : "—")} error={sectionErrors.overview} /></li>
          </ul>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <Panel title="Top artists" error={sectionErrors.artists}>
              {sectionErrors.artists ? (
                <p className="text-muted text-sm py-8 text-center">Failed to load artists.</p>
              ) : artists.length === 0 ? (
                <p className="text-muted text-sm py-8 text-center">No data yet — start playing music to see your analytics.</p>
              ) : (
                <ResponsiveContainer width="100%" height={260}>
                  <BarChart data={artists.slice(0, 10)} layout="vertical" margin={{ left: 20 }}>
                    <XAxis type="number" stroke="#7a8086" />
                    <YAxis type="category" dataKey="artist" stroke="#7a8086" width={120} />
                    <Tooltip contentStyle={{ background: "var(--color-panel)", border: "1px solid var(--color-panel2)" }} />
                    <Bar dataKey="plays" fill="#e6b450" />
                  </BarChart>
                </ResponsiveContainer>
              )}
            </Panel>

            <Panel title="Top genres" error={sectionErrors.genres}>
              {sectionErrors.genres ? (
                <p className="text-muted text-sm py-8 text-center">Failed to load genres.</p>
              ) : genres.length === 0 ? (
                <p className="text-muted text-sm py-8 text-center">No data yet — start playing music to see your analytics.</p>
              ) : (
                <ResponsiveContainer width="100%" height={260}>
                  <PieChart>
                    <Pie data={genres} dataKey="plays" nameKey="genre" outerRadius={100} label={false}>
                      {genres.map((_, i) => (
                        <Cell key={i} fill={COLORS[i % COLORS.length]} />
                      ))}
                    </Pie>
                    <Legend />
                    <Tooltip contentStyle={{ background: "var(--color-panel)", border: "1px solid var(--color-panel2)" }} />
                  </PieChart>
                </ResponsiveContainer>
              )}
            </Panel>
          </div>

          <Panel title="When you listen" error={sectionErrors.heatmap}>
            {sectionErrors.heatmap ? (
              <p className="text-muted text-sm py-8 text-center">Failed to load heatmap.</p>
            ) : (
              <div className="relative">
                <div className="overflow-x-auto">
                  <table className="text-xs" role="grid" aria-label="Listening activity by day of week and hour">
                    <caption className="sr-only">Listening activity by day of week and hour</caption>
                    <thead>
                      <tr>
                        <th className="px-1 py-1 text-muted" scope="col"></th>
                        {Array.from({ length: 24 }, (_, h) => (
                          <th key={h} className="px-1 py-1 text-muted font-normal" scope="col">
                            {h}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {DAYS.map((d, di) => (
                        <tr key={d}>
                          <td className="px-2 py-1 text-muted" scope="row">{d}</td>
                          {Array.from({ length: 24 }, (_, h) => {
                            const v = heatLookup.get(`${di}-${h}`) || 0;
                            const intensity = heatMax > 0 ? v / heatMax : 0;
                            return (
                              <td key={h} className="p-0">
                                <div
                                  title={`${d} ${h}:00 — ${v} plays`}
                                  aria-label={`${d} ${h}:00 — ${v} plays`}
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
                {/* Fade indicator on right edge for mobile scrollability */}
                <div className="pointer-events-none absolute top-0 right-0 bottom-0 w-6 bg-gradient-to-l from-panel to-transparent" />
                {/* Heatmap legend */}
                <div className="flex items-center gap-2 mt-2 text-xs text-muted">
                  <span>0</span>
                  <div className="flex-1 h-2 rounded" style={{ background: "linear-gradient(to right, rgba(230,180,80,0.02), rgba(230,180,80,0.87))" }} />
                  <span>{heatMax} plays</span>
                </div>
              </div>
            )}
          </Panel>

          <Panel title="Top tracks" error={sectionErrors.tracks}>
            {sectionErrors.tracks ? (
              <p className="text-muted text-sm py-8 text-center">Failed to load tracks.</p>
            ) : tracks.length === 0 ? (
              <p className="text-muted text-sm py-8 text-center">No data yet — start playing music to see your analytics.</p>
            ) : (
              <ul className="divide-y divide-panel2">
                {tracks.map((t, i) => (
                  <li key={t.id} className="px-2 py-2 flex justify-between text-sm items-center group">
                    <span>
                      <span className="text-muted w-6 inline-block">{i + 1}.</span>
                      <strong>{t.title}</strong>{" "}
                      <span className="text-muted">— {t.artist}</span>
                    </span>
                    <span className="flex items-center gap-2">
                      <span className="text-muted">{t.plays} plays</span>
                      <button
                        onClick={() => handlePlayTrack(t.id)}
                        className="p-1 text-muted/50 hover:text-accent opacity-0 group-hover:opacity-100 transition-opacity rounded hover:bg-panel2/50"
                        aria-label={`Play ${t.title}`}
                      >
                        <Play size={14} />
                      </button>
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </Panel>
        </>
      )}
    </div>
  );
}

const Stat = memo(function Stat({ label, value, error }: { label: string; value: number | string; error?: boolean }) {
  return (
    <div className={`bg-panel rounded-lg p-4 border ${error ? "border-red-500/50" : "border-panel2"}`}>
      <div className="text-xs text-muted uppercase tracking-wide">{label}</div>
      <div className="text-2xl font-semibold mt-1 truncate">{value}</div>
    </div>
  );
});

const Panel = memo(function Panel({ title, children, error }: { title: string; children: React.ReactNode; error?: boolean }) {
  return (
    <div className={`bg-panel rounded-lg p-4 border ${error ? "border-red-500/50" : "border-panel2"}`}>
      <h2 className="text-sm font-semibold mb-3 text-muted uppercase tracking-wider">{title}</h2>
      {children}
    </div>
  );
});
