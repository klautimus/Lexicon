import { useEffect, useState } from "react";
import { api, Stats, RecentPlay } from "../lib/api";
import { QrCode, Wifi, X, Copy, CheckCircle2, AlertTriangle } from "lucide-react";

export default function HomePage() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [recent, setRecent] = useState<RecentPlay[]>([]);
  const [showQr, setShowQr] = useState(false);
  const [localUrl, setLocalUrl] = useState("");
  const [networkInfo, setNetworkInfo] = useState<{local_ip: string; all_ips: string[]; private_ips: string[]} | null>(null);
  const [copied, setCopied] = useState(false);
  const [showNetworkHelp, setShowNetworkHelp] = useState(false);

  useEffect(() => {
    api.stats().then(setStats).catch(() => {});
    api.recent().then(setRecent).catch(() => {});
    // Detect if accessed from LAN (non-localhost)
    const host = window.location.hostname;
    if (host !== "localhost" && host !== "127.0.0.1" && host !== "::1") {
      setLocalUrl(window.location.origin);
      setShowQr(true);
    }
    // Fetch server network info for debugging
    fetch("/api/network")
      .then(r => r.json())
      .then(setNetworkInfo)
      .catch(() => {});
  }, []);

  function copyUrl(url: string) {
    navigator.clipboard.writeText(url).then(() => {
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
              {window.location.hostname !== networkInfo.local_ip && !window.location.hostname.startsWith("192.") && !window.location.hostname.startsWith("10.") && !window.location.hostname.startsWith("172.") && window.location.hostname !== "localhost" && window.location.hostname !== "127.0.0.1" && (
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
