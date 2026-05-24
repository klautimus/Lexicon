import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { CheckCircle2, AlertCircle, RefreshCw, Link2, Unlink, HelpCircle } from "lucide-react";
import { api, SpotifyStatus } from "../lib/api";
import AppleMusicSettings from "../components/AppleMusicSettings";
import { useHelp } from "../contexts/HelpContext";
import { useToast } from "../contexts/ToastContext";
import ConfirmModal from "../components/ConfirmModal";

export default function SettingsPage() {
  const { showHelp } = useHelp();
  const { success: toastSuccess, error: toastError } = useToast();
  const [status, setStatus] = useState<SpotifyStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [params] = useSearchParams();
  const justConnected = params.get("spotify") === "ok";
  const errorReason = params.get("spotify") === "error" ? params.get("reason") : null;

  // Phase 3: ConfirmModal state
  const [disconnectConfirm, setDisconnectConfirm] = useState(false);

  // Phase 3.3: Unmount guard for syncNow setTimeout
  const cancelledRef = useRef(false);

  async function load() {
    try {
      const s = await api.spotifyStatus();
      setStatus(s);
    } catch (e) {
      console.error("[SettingsPage] failed to load Spotify status", e);
      setStatus(null);
      toastError("Failed to load Spotify status.");
    }
  }

  useEffect(() => {
    load();
    return () => {
      cancelledRef.current = true;
    };
  }, []);

  // Phase 3.1: try/finally + Phase 3: ConfirmModal
  async function disconnect() {
    setDisconnectConfirm(true);
  }

  async function handleDisconnect() {
    setDisconnectConfirm(false);
    setBusy(true);
    try {
      await api.spotifyDisconnect();
      toastSuccess("Spotify disconnected.");
    } catch (e) {
      console.error("[SettingsPage] disconnect failed", e);
      toastError("Failed to disconnect Spotify.");
    } finally {
      setBusy(false);
      load();
    }
  }

  // Phase 3.4: try/finally + Phase 3.3: unmount guard
  async function syncNow() {
    setBusy(true);
    try {
      await api.spotifySync();
      toastSuccess("Spotify sync triggered.");
    } catch (e) {
      console.error("[SettingsPage] sync failed", e);
      toastError("Failed to sync Spotify.");
      setBusy(false);
      return;
    }
    setTimeout(() => {
      if (!cancelledRef.current) {
        load();
        setBusy(false);
      }
    }, 2000);
  }

  return (
    <div className="space-y-6 max-w-3xl">
      <div className="flex items-center gap-2">
        <h1 className="text-2xl font-semibold">Settings</h1>
        <button
          onClick={() => showHelp("settings.spotify")}
          className="p-1 text-muted/50 hover:text-accent transition-colors rounded hover:bg-panel2/50"
          aria-label="Help: Settings"
        >
          <HelpCircle size={16} />
        </button>
      </div>

      <section className="bg-panel rounded-lg p-5 border border-panel2 space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold flex items-center gap-2">
              Spotify
              {status?.connected && (
                <span
                  className="w-2 h-2 rounded-full bg-green-400"
                  role="status"
                  aria-label="Spotify connected"
                />
              )}
            </h2>
            <p className="text-sm text-muted">
              Pull listening history every 30 minutes and play Spotify tracks
              directly in Lexicon (Premium).
            </p>
          </div>
        </div>

        {justConnected && (
          <div className="flex items-center gap-2 text-sm text-green-400 bg-green-400/10 border border-green-400/30 rounded px-3 py-2">
            <CheckCircle2 size={16} /> Connected successfully.
          </div>
        )}
        {errorReason && (
          <div
            className="flex items-center gap-2 text-sm text-red-400 bg-red-400/10 border border-red-400/30 rounded px-3 py-2"
            role="alert"
          >
            <AlertCircle size={16} /> Spotify returned an error: {errorReason}
          </div>
        )}

        {!status ? (
          <p className="text-sm text-muted" aria-live="polite">Loading…</p>
        ) : !status.configured ? (
          <div className="text-sm text-muted bg-panel2/50 rounded p-3 border border-panel2">
            <p className="mb-2">
              <strong>Setup required.</strong> Add a Spotify Client ID to{" "}
              <code className="text-accent">backend/.env</code>:
            </p>
            <ol className="list-decimal list-inside space-y-1">
              <li>
                Go to{" "}
                <a
                  className="text-accent underline"
                  href="https://developer.spotify.com/dashboard"
                  target="_blank"
                  rel="noreferrer"
                >
                  developer.spotify.com/dashboard
                </a>{" "}
                and create an app named "Lexicon".
              </li>
              <li>
                Add redirect URI:{" "}
                <code className="text-accent">
                  http://127.0.0.1:8787/api/spotify/callback
                </code>
              </li>
              <li>
                Copy the Client ID into{" "}
                <code className="text-accent">SPOTIFY_CLIENT_ID</code> in{" "}
                <code className="text-accent">backend/.env</code>, then restart
                the server.
              </li>
            </ol>
          </div>
        ) : !status.connected ? (
          <a
            href={api.spotifyAuthURL()}
            className="inline-flex items-center gap-2 px-4 py-2 bg-[#1DB954] hover:bg-[#1ed760] text-black font-medium rounded-md transition"
          >
            <Link2 size={16} /> Connect Spotify
          </a>
        ) : (
          <div className="space-y-3">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-sm">
              <Field label="Account">{status.display_name || "—"}</Field>
              <Field label="Plan">
                <span
                  className={
                    status.product === "premium"
                      ? "text-green-400"
                      : "text-yellow-400"
                  }
                >
                  {status.product || "—"}
                </span>
              </Field>
              <Field label="Last sync">
                {status.last_synced_at
                  ? new Date(status.last_synced_at * 1000).toLocaleString()
                  : "never"}
              </Field>
              <Field label="In-app playback">
                {status.has_playback_sdk ? "Enabled" : "Premium required"}
              </Field>
            </div>
            <div className="flex gap-2 flex-wrap">
              <button
                onClick={syncNow}
                disabled={busy}
                className="px-3 py-2 bg-panel2 hover:bg-panel2/70 rounded-md text-sm flex items-center gap-2 disabled:opacity-50"
              >
                <RefreshCw size={14} className={busy ? "animate-spin" : ""} />{" "}
                Sync now
              </button>
              <button
                onClick={disconnect}
                disabled={busy}
                className="px-3 py-2 border border-red-400/40 text-red-400 hover:bg-red-400/10 rounded-md text-sm flex items-center gap-2 disabled:opacity-50"
              >
                <Unlink size={14} /> Disconnect
              </button>
            </div>
          </div>
        )}
      </section>

      <AppleMusicSettings />

      <ConfirmModal
        open={disconnectConfirm}
        title="Disconnect Spotify"
        message="Disconnect Spotify? Your synced history stays."
        confirmLabel="Disconnect"
        variant="danger"
        onConfirm={handleDisconnect}
        onCancel={() => setDisconnectConfirm(false)}
      />
    </div>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <div className="text-xs uppercase tracking-wide text-muted">{label}</div>
      <div>{children}</div>
    </div>
  );
}
