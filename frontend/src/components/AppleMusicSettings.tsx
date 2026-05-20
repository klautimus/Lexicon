import { useEffect, useRef, useState } from "react";
import {
  Link2,
  Unlink,
  RefreshCw,
  Save,
  Trash2,
  ChevronDown,
  ChevronUp,
  Music,
  AlertCircle,
} from "lucide-react";
import { api, AppleStatus } from "../lib/api";
import { authorizeAppleMusic, resetMusicKit } from "../lib/musickit";

const STOREFRONTS: { id: string; name: string }[] = [
  { id: "us", name: "United States" },
  { id: "gb", name: "United Kingdom" },
  { id: "ca", name: "Canada" },
  { id: "au", name: "Australia" },
  { id: "de", name: "Germany" },
  { id: "fr", name: "France" },
  { id: "jp", name: "Japan" },
  { id: "br", name: "Brazil" },
  { id: "mx", name: "Mexico" },
  { id: "in", name: "India" },
  { id: "es", name: "Spain" },
  { id: "it", name: "Italy" },
  { id: "nl", name: "Netherlands" },
  { id: "se", name: "Sweden" },
  { id: "kr", name: "South Korea" },
];

export default function AppleMusicSettings() {
  const [status, setStatus] = useState<AppleStatus | null>(null);
  const [editing, setEditing] = useState(false);
  const [showHelp, setShowHelp] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [okMsg, setOkMsg] = useState<string | null>(null);

  const [teamId, setTeamId] = useState("");
  const [keyId, setKeyId] = useState("");
  const [privateKey, setPrivateKey] = useState("");
  const [storefront, setStorefront] = useState("us");
  const fileInputRef = useRef<HTMLInputElement>(null);

  async function load() {
    try {
      const s = await api.appleStatus();
      setStatus(s);
      if (s.configured && s.team_id) setTeamId(s.team_id);
      if (s.configured && s.key_id) setKeyId(s.key_id);
      if (s.storefront) setStorefront(s.storefront);
    } catch (e) {
      console.error("[apple-settings] load failed", e);
    }
  }
  useEffect(() => {
    load();
  }, []);

  function clearMessages() {
    setErr(null);
    setOkMsg(null);
  }

  async function onSaveConfig(e: React.FormEvent) {
    e.preventDefault();
    clearMessages();
    if (teamId.trim().length !== 10) {
      setErr("Team ID must be exactly 10 characters.");
      return;
    }
    if (keyId.trim().length !== 10) {
      setErr("Key ID must be exactly 10 characters.");
      return;
    }
    if (!privateKey.trim().includes("PRIVATE KEY")) {
      setErr(
        "Private key must include the -----BEGIN PRIVATE KEY----- header. Paste the full .p8 file contents.",
      );
      return;
    }
    setBusy(true);
    try {
      await api.appleSaveConfig({
        team_id: teamId.trim(),
        key_id: keyId.trim(),
        private_key: privateKey,
        storefront: storefront.trim().toLowerCase() || "us",
      });
      setOkMsg("Credentials saved. Click 'Connect Apple Music' to authorize.");
      setEditing(false);
      setPrivateKey(""); // don't keep the .p8 in memory after save
      resetMusicKit();
      await load();
    } catch (e: unknown) {
      console.error("[apple-settings] save config failed", e);
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function onConnect() {
    clearMessages();
    setBusy(true);
    try {
      const mut = await authorizeAppleMusic();
      await api.appleConnect(mut);
      setOkMsg("Connected to Apple Music. Syncing your recent history…");
      await load();
    } catch (e: unknown) {
      console.error("[apple-settings] connect failed", e);
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function onDisconnect() {
    if (!confirm("Disconnect Apple Music? Your synced history stays.")) return;
    clearMessages();
    setBusy(true);
    try {
      await api.appleDisconnect();
      resetMusicKit();
      setOkMsg("Disconnected.");
      await load();
    } catch (e: unknown) {
      console.error("[apple-settings] disconnect failed", e);
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function onDeleteConfig() {
    if (
      !confirm(
        "Delete Apple Music credentials? You'll need to paste them again to reconnect.",
      )
    )
      return;
    clearMessages();
    setBusy(true);
    try {
      await api.appleDeleteConfig();
      resetMusicKit();
      setTeamId("");
      setKeyId("");
      setPrivateKey("");
      setEditing(false);
      setOkMsg("Credentials deleted.");
      await load();
    } catch (e: unknown) {
      console.error("[apple-settings] delete config failed", e);
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function onSyncNow() {
    clearMessages();
    setBusy(true);
    try {
      await api.appleSync();
      setOkMsg("Sync triggered.");
      setTimeout(load, 2500);
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function onP8File(ev: React.ChangeEvent<HTMLInputElement>) {
    const file = ev.target.files?.[0];
    if (!file) return;
    try {
      const text = await file.text();
      setPrivateKey(text);
    } catch (e) {
      setErr("Could not read file: " + (e instanceof Error ? e.message : String(e)));
    }
  }

  const isConfigured = !!status?.configured;
  const isConnected = !!status?.connected;
  // Show the form when not configured, when the user explicitly clicked Edit,
  // or while the API hasn't responded yet.
  const showForm = editing || (status && !isConfigured);

  return (
    <section className="bg-panel rounded-lg p-5 border border-panel2 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold flex items-center gap-2">
            Apple Music
            {isConnected && (
              <span
                className="w-2 h-2 rounded-full bg-green-400"
                title="Connected"
              />
            )}
          </h2>
          <p className="text-sm text-muted">
            Sync your Apple Music recently-played history, heavy rotation,
            library, and Apple's personalized recommendations into Lexicon —
            feeds the AI for smarter playlist generation.
          </p>
        </div>
      </div>

      {err && (
        <div className="flex items-start gap-2 text-sm text-red-400 bg-red-400/10 border border-red-400/30 rounded px-3 py-2">
          <AlertCircle size={16} className="shrink-0 mt-0.5" />
          <span className="break-words">{err}</span>
        </div>
      )}
      {okMsg && !err && (
        <div className="text-sm text-green-400 bg-green-400/10 border border-green-400/30 rounded px-3 py-2">
          {okMsg}
        </div>
      )}

      {/* Setup help (always available) */}
      <button
        type="button"
        onClick={() => setShowHelp(!showHelp)}
        className="text-xs text-muted hover:text-text flex items-center gap-1"
      >
        {showHelp ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
        How do I get these credentials?
      </button>
      {showHelp && (
        <div className="text-sm text-muted bg-panel2/40 rounded p-3 border border-panel2 space-y-2">
          <p>
            <strong>You need an Apple Developer Program membership ($99/year)</strong>.
            Apple Music API access is gated behind it.
          </p>
          <ol className="list-decimal list-inside space-y-1">
            <li>
              Sign in to{" "}
              <a
                className="text-accent underline"
                href="https://developer.apple.com/account"
                target="_blank"
                rel="noreferrer"
              >
                developer.apple.com/account
              </a>{" "}
              and find your <strong>Team ID</strong> on the Membership page
              (10 characters, mix of letters and numbers).
            </li>
            <li>
              Create a <strong>Media identifier</strong>:{" "}
              <a
                className="text-accent underline"
                href="https://developer.apple.com/account/resources/identifiers/list/musicId"
                target="_blank"
                rel="noreferrer"
              >
                Identifiers → Media IDs
              </a>{" "}
              → New → Media IDs. Call it{" "}
              <code className="text-accent">media.lexicon</code> (or similar).
            </li>
            <li>
              Create a <strong>MusicKit private key</strong>:{" "}
              <a
                className="text-accent underline"
                href="https://developer.apple.com/account/resources/authkeys/list"
                target="_blank"
                rel="noreferrer"
              >
                Keys → New
              </a>{" "}
              → check <em>MusicKit</em> → associate it with the Media ID from
              step 2. Download the <code className="text-accent">.p8</code>{" "}
              file (you can only download it once). The 10-character{" "}
              <strong>Key ID</strong> is shown on this page.
            </li>
            <li>
              Paste the contents of the{" "}
              <code className="text-accent">.p8</code> file (including the
              -----BEGIN PRIVATE KEY----- lines) below, or use the file picker.
            </li>
          </ol>
        </div>
      )}

      {/* State 1: Not configured (or editing) — credentials form */}
      {showForm && (
        <form onSubmit={onSaveConfig} className="space-y-3">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="text-xs uppercase tracking-wide text-muted">
                Team ID
              </label>
              <input
                type="text"
                value={teamId}
                onChange={(e) => setTeamId(e.target.value)}
                placeholder="ABCDE12345"
                maxLength={10}
                className="w-full mt-1 px-3 py-2 bg-panel2 rounded text-sm font-mono"
                autoComplete="off"
                spellCheck={false}
                required
              />
            </div>
            <div>
              <label className="text-xs uppercase tracking-wide text-muted">
                Key ID
              </label>
              <input
                type="text"
                value={keyId}
                onChange={(e) => setKeyId(e.target.value)}
                placeholder="FGHIJ67890"
                maxLength={10}
                className="w-full mt-1 px-3 py-2 bg-panel2 rounded text-sm font-mono"
                autoComplete="off"
                spellCheck={false}
                required
              />
            </div>
          </div>

          <div>
            <label className="text-xs uppercase tracking-wide text-muted flex items-center justify-between">
              <span>Private Key (.p8 file contents)</span>
              <button
                type="button"
                onClick={() => fileInputRef.current?.click()}
                className="text-xs text-accent hover:underline"
              >
                Choose .p8 file…
              </button>
              <input
                ref={fileInputRef}
                type="file"
                accept=".p8,text/plain"
                onChange={onP8File}
                className="hidden"
              />
            </label>
            <textarea
              value={privateKey}
              onChange={(e) => setPrivateKey(e.target.value)}
              placeholder="-----BEGIN PRIVATE KEY-----&#10;...&#10;-----END PRIVATE KEY-----"
              rows={6}
              className="w-full mt-1 px-3 py-2 bg-panel2 rounded text-xs font-mono"
              spellCheck={false}
              required
            />
          </div>

          <div>
            <label className="text-xs uppercase tracking-wide text-muted">
              Storefront
            </label>
            <select
              value={storefront}
              onChange={(e) => setStorefront(e.target.value)}
              className="w-full mt-1 px-3 py-2 bg-panel2 rounded text-sm"
            >
              {STOREFRONTS.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name} ({s.id})
                </option>
              ))}
            </select>
            <p className="text-xs text-muted mt-1">
              Your storefront will be auto-detected when you connect; this is
              just the default for catalog lookups.
            </p>
          </div>

          <div className="flex gap-2 flex-wrap">
            <button
              type="submit"
              disabled={busy}
              className="px-4 py-2 bg-[#FA243C] hover:bg-[#fa3d51] text-white font-medium rounded-md text-sm flex items-center gap-2 disabled:opacity-50"
            >
              <Save size={14} /> Save credentials
            </button>
            {isConfigured && (
              <button
                type="button"
                onClick={() => {
                  setEditing(false);
                  setPrivateKey("");
                  clearMessages();
                }}
                disabled={busy}
                className="px-4 py-2 bg-panel2 hover:bg-panel2/70 rounded-md text-sm disabled:opacity-50"
              >
                Cancel
              </button>
            )}
          </div>
        </form>
      )}

      {/* State 2: Configured but not connected */}
      {isConfigured && !isConnected && !showForm && (
        <div className="space-y-3">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-sm">
            <Field label="Team ID">
              <code className="font-mono">{status?.team_id}</code>
            </Field>
            <Field label="Key ID">
              <code className="font-mono">{status?.key_id}</code>
            </Field>
            <Field label="Storefront">{status?.storefront || "us"}</Field>
            <Field label="Dev token expires">
              {status?.dev_token_expires_at
                ? new Date(status.dev_token_expires_at * 1000).toLocaleDateString()
                : "—"}
            </Field>
          </div>
          <div className="flex gap-2 flex-wrap">
            <button
              onClick={onConnect}
              disabled={busy}
              className="px-4 py-2 bg-[#FA243C] hover:bg-[#fa3d51] text-white font-medium rounded-md text-sm flex items-center gap-2 disabled:opacity-50"
            >
              <Music size={14} /> Connect Apple Music
            </button>
            <button
              onClick={() => {
                setEditing(true);
                clearMessages();
              }}
              disabled={busy}
              className="px-3 py-2 bg-panel2 hover:bg-panel2/70 rounded-md text-sm"
            >
              Edit credentials
            </button>
            <button
              onClick={onDeleteConfig}
              disabled={busy}
              className="px-3 py-2 border border-red-400/40 text-red-400 hover:bg-red-400/10 rounded-md text-sm flex items-center gap-2"
            >
              <Trash2 size={14} /> Delete credentials
            </button>
          </div>
        </div>
      )}

      {/* State 3: Connected */}
      {isConnected && (
        <div className="space-y-3">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-sm">
            <Field label="Storefront">{status?.storefront || "—"}</Field>
            <Field label="Last sync">
              {status?.last_synced_at
                ? new Date(status.last_synced_at * 1000).toLocaleString()
                : "never"}
            </Field>
            <Field label="Team ID">
              <code className="font-mono">{status?.team_id}</code>
            </Field>
            <Field label="Dev token expires">
              {status?.dev_token_expires_at
                ? new Date(
                    status.dev_token_expires_at * 1000,
                  ).toLocaleDateString()
                : "—"}
            </Field>
          </div>
          <div className="flex gap-2 flex-wrap">
            <button
              onClick={onSyncNow}
              disabled={busy}
              className="px-3 py-2 bg-panel2 hover:bg-panel2/70 rounded-md text-sm flex items-center gap-2 disabled:opacity-50"
            >
              <RefreshCw size={14} className={busy ? "animate-spin" : ""} />{" "}
              Sync now
            </button>
            <button
              onClick={onDisconnect}
              disabled={busy}
              className="px-3 py-2 border border-yellow-400/40 text-yellow-400 hover:bg-yellow-400/10 rounded-md text-sm flex items-center gap-2"
            >
              <Unlink size={14} /> Disconnect
            </button>
            <button
              onClick={() => {
                setEditing(true);
                clearMessages();
              }}
              disabled={busy}
              className="px-3 py-2 bg-panel2 hover:bg-panel2/70 rounded-md text-sm"
            >
              Edit credentials
            </button>
            <button
              onClick={onDeleteConfig}
              disabled={busy}
              className="px-3 py-2 border border-red-400/40 text-red-400 hover:bg-red-400/10 rounded-md text-sm flex items-center gap-2"
            >
              <Trash2 size={14} /> Delete credentials
            </button>
          </div>
        </div>
      )}

      {!status && (
        <p className="text-sm text-muted">Loading…</p>
      )}
    </section>
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

// Unused import lint guard — Link2 is intentionally exported for future use
// in connect flows that may use chained icons.
void Link2;
