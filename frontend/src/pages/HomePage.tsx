import { useEffect, useState } from "react";
import { api, Stats, RecentPlay } from "../lib/api";
import { QrCode, Wifi, X, Copy, CheckCircle2, AlertTriangle, HelpCircle } from "lucide-react";
import { useHelp } from "../contexts/HelpContext";

export default function HomePage() {
  const { showHelp } = useHelp();
  const [stats, setStats] = useState<Stats | null>(null);
  const [recent, setRecent] = useState<RecentPlay[]>([]);
  const [statsError, setStatsError] = useState(false);
  const [recentError, setRecentError] = useState(false);
  const [showQr, setShowQr] = useState(false);
  const [localUrl, setLocalUrl] = useState("");
  const [networkInfo, setNetworkInfo] = useState<{local_ip: string; all_ips: string[]; private_ips: string[]} | null>(null);
  const [copied, setCopied] = useState(false);
  const [showNetworkHelp, setShowNetworkHelp] = useState(false);

  useEffect(() => {
    api.stats().then(setStats).catch(() => setStatsError(true));
    api.recent().then(setRecent).catch(() => setRecentError(true));
    const host = window.location.hostname;
    if (host !== "localhost" && host !== "127.0.0.1" && host !== "::1") {
      setLocalUrl(window.location.origin);
      setShowQr(true);
    }
    fetch("/api/network")
      .then(r => r.json())
      .then(setNetworkInfo)
      .catch(() => {});
  }, []);

  function copyUrl(url: string) {
    navigator.clipboard.writeText(url).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }).catch(() => {
      const ta = document.createElement("textarea");
      ta.value = url;
      ta.style.position = "fixed";
      ta.style.opacity = "0";
      document.body.appendChild(ta);
      ta.select();
      try { document.execCommand("copy"); } catch {}
      document.body.removeChild(ta);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <div className="space-y-8">
      {showQr && localUrl && (
        <div className="bg-panel border border-accent/30 rounded-lg p-4 space-y-3">
          <div className="flex items-center gap-4">
            <div className="flex-shrink-0">
              <img
                src={`${localUrl}/api/qr`}
                alt="QR code for mobile connection"
                className="w-20 h-20 rounded"
              />
            </div>
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2 text-sm font-medium">
                <Wifi size={14} className="text-accent" />
                <span>Connected via LAN</span>
              </div>
              <p className="text-xs text-muted mt-1">
                Scan this QR code with another device on your network, or visit{" "}
                <code className="text-accent">{localUrl}</code>
              </p>
              <div className="flex items-center gap-2 mt-2">
                <button
                  onClick={() => copyUrl(localUrl)}
                  className="text-xs text-muted hover:text-text flex items-center gap-1"
                >
                  {copied ? <CheckCircle2 size={12} className="text-green-400" /> : <Copy size={12} />}
                  {copied ? "Copied!" : "Copy URL"}
                </button>
                <button
                  onClick={() => setShowNetworkHelp(!showNetworkHelp)}
                  className="text-xs text-muted hover:text-text flex items-center gap-1"
                >
                  <AlertTriangle size={12} />
                  Connection help
                </button>
              </div>
            </div>
            <button
              onClick={() => setShowQr(false)}
              className="flex-shrink-0 p-1 text-muted hover:text-text"
              aria-label="Dismiss connection banner"
              title="Dismiss"
            >
              <X size={16} />
            </button>
          </div>
          {showNetworkHelp && networkInfo && (
            <div className="bg-bg/50 rounded-md p-3 text-xs space-y-2 border border-panel2">
              <p className="font-medium text-text">Server Network Info</p>
              <div className="grid grid-cols-1 gap-1">
                <div className="flex gap-2">
                  <span className="text-muted w-24 flex-shrink-0">Server IP:</span>
                  <code className="text-accent">{networkInfo.local_ip}</code>
                </div>
                {networkInfo.private_ips.length > 0 && (
                  <div className="flex gap-2">
                    <span className="text-muted w-24 flex-shrink-0">Private IPs:</span>
                    <span>{networkInfo.private_ips.join(", ")}</span>
                  </div>
                )}
                <div className="flex gap-2">
                  <span className="text-muted w-24 flex-shrink-0">Your IP:</span>
                  <code className="text-accent">{window.location.hostname}</code>
                </div>
              </div>
              {networkInfo.private_ips.length === 0 && (
                <p className="text-yellow-400">
                  ⚠ No private LAN IPs detected. Other devices may not be able to connect.
                </p>
              )}
              {window.location.hostname === networkInfo.local_ip && (
                <p className="text-green-400">
                  ✓ You're connecting directly to the server's LAN IP.
                </p>
              )}
              {window.location.hostname !== networkInfo.local_ip && !window.location.hostname.startsWith("192.") && !window.location.hostname.startsWith("10.") && !isRfc1918(window.location.hostname) && window.location.hostname !== "localhost" && window.location.hostname !== "127.0.0.1" && (
                <div className="text-yellow-400 space-y-1">
                  <p>⚠ Your IP ({window.location.hostname}) doesn't look like a local network address.</p>
                  <p className="text-muted">Make sure your device is connected to the same WiFi network as the server ({networkInfo.local_ip}).</p>
                </div>
              )}
            </div>
          )}
        </div>
      )}

      <div>
        <div className="flex items-center gap-3 mb-2">
          <img src="/icon.svg" alt="Lexicon" className="w-10 h-10 rounded-lg" />
          <div>
            <h1 className="text-3xl font-semibold tracking-tight mb-0">Welcome back</h1>
            <p className="text-muted">Your private library, intelligently organized.</p>
          </div>
        </div>
      </div>

      <div>
        <div className="flex items-center gap-2 mb-4">
          <h2 className="text-lg font-semibold">Library Stats</h2>
          <button
            onClick={() => showHelp("home.stats")}
            className="p-2 text-muted/50 hover:text-accent transition-colors rounded hover:bg-panel2/50 min-w-[44px] min-h-[44px] flex items-center justify-center"
            aria-label="Help: Library Stats"
          >
            <HelpCircle size={16} />
          </button>
        </div>
        {statsError ? (
          <div className="bg-panel rounded-lg p-4 border border-panel2 text-center">
            <p className="text-muted text-sm">Failed to load stats. <button onClick={() => { setStatsError(false); api.stats().then(setStats).catch(() => setStatsError(true)); }} className="text-accent hover:underline">Retry</button></p>
          </div>
        ) : (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          <Stat label="Tracks" value={stats?.tracks ?? "—"} />
          <Stat label="Albums" value={stats?.albums ?? "—"} />
          <Stat label="Artists" value={stats?.artists ?? "—"} />
          <Stat label="Podcasts" value={stats?.podcasts ?? "—"} />
        </div>
        )}
      </div>

      <div>
        <div className="flex items-center gap-2 mb-3">
          <h2 className="text-lg font-semibold">Recently Played</h2>
          <button
            onClick={() => showHelp("home.recent")}
            className="p-2 text-muted/50 hover:text-accent transition-colors rounded hover:bg-panel2/50 min-w-[44px] min-h-[44px] flex items-center justify-center"
            aria-label="Help: Recently Played"
          >
            <HelpCircle size={16} />
          </button>
        </div>
        {recent.length === 0 && !recentError ? (
          <div className="bg-panel rounded-lg p-6 border border-panel2 text-center">
            <p className="text-muted text-sm">
              No plays yet — head to <strong>Music</strong> and play something to start
              building your taste profile.
            </p>
          </div>
        ) : recentError ? (
          <div className="bg-panel rounded-lg p-4 border border-panel2 text-center">
            <p className="text-muted text-sm">Failed to load recent plays. <button onClick={() => { setRecentError(false); api.recent().then(setRecent).catch(() => setRecentError(true)); }} className="text-accent hover:underline">Retry</button></p>
          </div>
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

function isRfc1918(host: string): boolean {
  const parts = host.split(".");
  if (parts.length !== 4) return false;
  const [a, b] = [parseInt(parts[0], 10), parseInt(parts[1], 10)];
  if (a === 10) return true;
  if (a === 172 && b >= 16 && b <= 31) return true;
  if (a === 192 && b === 168) return true;
  return false;
}

function Stat({ label, value }: { label: string; value: number | string }) {
  return (
    <div className="bg-panel rounded-lg p-4 border border-panel2">
      <div className="text-xs text-muted uppercase tracking-wide">{label}</div>
      <div className="text-2xl font-semibold mt-1">{value}</div>
    </div>
  );
}
