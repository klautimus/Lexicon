import { useEffect, useRef, useState, useCallback } from "react";
import {
  Unlink,
  RefreshCw,
  Save,
  Trash2,
  ChevronDown,
  ChevronUp,
  Music,
  AlertCircle,
  CheckCircle2,
  Search,
} from "lucide-react";
import { api, AppleStatus } from "../lib/api";
import { authorizeAppleMusic, resetMusicKit } from "../lib/musickit";
import { useToast } from "../contexts/ToastContext";
import ConfirmModal from "../components/ConfirmModal";

// Phase 6.3: Expanded storefront list (top 50+ by population/usage)
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
  { id: "cn", name: "China" },
  { id: "hk", name: "Hong Kong" },
  { id: "tw", name: "Taiwan" },
  { id: "sg", name: "Singapore" },
  { id: "ae", name: "United Arab Emirates" },
  { id: "za", name: "South Africa" },
  { id: "ph", name: "Philippines" },
  { id: "th", name: "Thailand" },
  { id: "vn", name: "Vietnam" },
  { id: "id", name: "Indonesia" },
  { id: "my", name: "Malaysia" },
  { id: "nz", name: "New Zealand" },
  { id: "ie", name: "Ireland" },
  { id: "at", name: "Austria" },
  { id: "ch", name: "Switzerland" },
  { id: "be", name: "Belgium" },
  { id: "pt", name: "Portugal" },
  { id: "dk", name: "Denmark" },
  { id: "no", name: "Norway" },
  { id: "fi", name: "Finland" },
  { id: "pl", name: "Poland" },
  { id: "cz", name: "Czech Republic" },
  { id: "ro", name: "Romania" },
  { id: "hu", name: "Hungary" },
  { id: "gr", name: "Greece" },
  { id: "tr", name: "Turkey" },
  { id: "il", name: "Israel" },
  { id: "sa", name: "Saudi Arabia" },
  { id: "eg", name: "Egypt" },
  { id: "ng", name: "Nigeria" },
  { id: "ke", name: "Kenya" },
  { id: "cl", name: "Chile" },
  { id: "co", name: "Colombia" },
  { id: "ar", name: "Argentina" },
  { id: "pe", name: "Peru" },
  { id: "ua", name: "Ukraine" },
  { id: "ru", name: "Russia" },
];

export default function AppleMusicSettings() {
  const { success: toastSuccess, error: toastError } = useToast();
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
  const [storefrontSearch, setStorefrontSearch] = useState("");
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Phase 3: ConfirmModal state
  const [disconnectConfirm, setDisconnectConfirm] = useState(false);
  const [deleteConfigConfirm, setDeleteConfigConfirm] = useState(false);

  // Unmount guard for onSyncNow setTimeout
  const cancelledRef = useRef(false);

  const load = useCallback(async () => {
    try {
      const s = await api.appleStatus();
      setStatus(s);
      if (s.configured && s.team_id) setTeamId(s.team_id);
      if (s.configured && s.key_id) setKeyId(s.key_id);
      if (s.storefront) setStorefront(s.storefront);
    } catch (e) {
      console.error("[apple-settings] load failed", e);
      toastError("Failed to load Apple Music status.");
    }
  }, [toastError]);

  useEffect(() => {
    load();
    return () => {
      cancelledRef.current = true;
    };
  }, [load]);

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
        "Private key must include the [REDACTED PRIVATE KEY]"
      );
      return;
    }
    setBusy(true);
    try {
      await api.appleSaveConfig({
        team_id: teamId.trim(),
        key_id: keyId.trim(),
        private_key: privateKey.trim(),
        storefront,
      });
      setPrivateKey("");
      setEditing(false);
      setStorefrontSearch("");
      toastSuccess("Apple Music credentials saved.");
      await load();
    } catch (e: unknown) {
      console.error("[apple-settings] save config failed", e);
      const msg = e instanceof Error ? e.message : String(e);
      setErr(msg);
      toastError("Failed to save credentials: " + msg);
    } finally {
      setBusy(false);
    }
  }

  // Phase 1.3: authorizeAppleMusic now has timeout in musickit.ts
  // Phase 2.2: Specific error messages per failure mode
  async function onConnect() {
    clearMessages();
    setBusy(true);
    try {
      const mut = await authorizeAppleMusic();
      await api.appleConnect(mut);
      toastSuccess("Connected to Apple Music. Syncing your recent history…");
      setOkMsg("Connected to Apple Music. Syncing your recent history…");
      await load();
    } catch (e: unknown) {
      console.error("[apple-settings] connect failed", e);
      const rawMsg = e instanceof Error ? e.message : String(e);

      // Phase 2.2: Map raw errors to user-friendly messages
      let userMsg: string;
      if (rawMsg.includes("timed out") || rawMsg.includes("popups")) {
        userMsg =
          "Connection timed out. Please ensure popups are allowed for this site and try again.";
      } else if (rawMsg.includes("MusicKit unavailable")) {
        userMsg =
          "MusicKit JS failed to load. Please check your internet connection and try again.";
      } else if (rawMsg.includes("Failed to load MusicKit JS")) {
        userMsg =
          "Could not load MusicKit from Apple's CDN. You may be offline or Apple's servers are unreachable.";
      } else if (rawMsg.includes("did not return a user token")) {
        userMsg =
          "Apple Music did not return a user token. Please try again or use a different Apple ID.";
      } else {
        userMsg = rawMsg;
      }

      setErr(userMsg);
      toastError(userMsg);
    } finally {
      setBusy(false);
    }
  }

  async function onDisconnect() {
    setDisconnectConfirm(true);
  }

  async function handleDisconnect() {
    setDisconnectConfirm(false);
    clearMessages();
    setBusy(true);
    try {
      await api.appleDisconnect();
      resetMusicKit();
      toastSuccess("Apple Music disconnected.");
      setOkMsg("Disconnected.");
      await load();
    } catch (e: unknown) {
      console.error("[apple-settings] disconnect failed", e);
      const msg = e instanceof Error ? e.message : String(e);
      setErr(msg);
      toastError("Failed to disconnect: " + msg);
    } finally {
      setBusy(false);
    }
  }

  async function onDeleteConfig() {
    setDeleteConfigConfirm(true);
  }

  async function handleDeleteConfig() {
    setDeleteConfigConfirm(false);
    clearMessages();
    setBusy(true);
    try {
      await api.appleDeleteConfig();
      resetMusicKit();
      setTeamId("");
      setKeyId("");
      setPrivateKey("");
      setEditing(false);
      toastSuccess("Apple Music credentials deleted.");
      setOkMsg("Credentials deleted.");
      await load();
    } catch (e: unknown) {
      console.error("[apple-settings] delete config failed", e);
      const msg = e instanceof Error ? e.message : String(e);
      setErr(msg);
      toastError("Failed to delete credentials: " + msg);
    } finally {
      setBusy(false);
    }
  }

  // Phase 2.5 (Bug 3.9): Unmount guard via cancelledRef
  async function onSyncNow() {
    clearMessages();
    setBusy(true);
    try {
      await api.appleSync();
      toastSuccess("Apple Music sync triggered.");
      setOkMsg("Sync triggered.");
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      setErr(msg);
      toastError("Sync failed: " + msg);
      setBusy(false);
      return;
    }
    setTimeout(() => {
      if (!cancelledRef.current) {
        load();
        setBusy(false);
      }
    }, 2500);
  }

  // Phase 1.1: Test credentials — backend endpoint not yet available.
  // When appleTestCredentials is added to the API, wire it up here.
  // For now, users can verify credentials by saving and connecting.

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
  const showForm = editing || (status && !isConfigured);

  // Phase 1.4: Check if MUT is expired
  const mutExpiresAt = status?.dev_token_expires_at;
  const isMutExpired = mutExpiresAt ? mutExpiresAt * 1000 < Date.now() : false;
  const isMutExpiringSoon = mutExpiresAt
    ? mutExpiresAt * 1000 < Date.now() + 30 * 24 * 60 * 60 * 1000 && !isMutExpired
    : false;

  // Phase 2.7: Filtered storefronts for searchable dropdown
  const filteredStorefronts = storefrontSearch
    ? STOREFRONTS.filter(
        (s) =>
          s.name.toLowerCase().includes(storefrontSearch.toLowerCase()) ||
          s.id.toLowerCase().includes(storefrontSearch.toLowerCase()),
      )
    : STOREFRONTS;

  return (
    <section className="bg-panel rounded-lg p-5 border border-panel2 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold flex items-center gap-2">
            Apple Music
            {isConnected && (
              <span
                className={`w-2 h-2 rounded-full ${isMutExpired ? "bg-yellow-400" : "bg-green-400"}`}
                role="status"
                aria-label={isMutExpired ? "Apple Music connected (token expired)" : "Apple Music connected"}
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

      {/* Phase 1.4: Expired/expiring MUT indicator */}
      {isConnected && (isMutExpired || isMutExpiringSoon) && (
        <div
          className={`flex items-center gap-2 text-sm rounded px-3 py-2 border ${
            isMutExpired
              ? "text-yellow-400 bg-yellow-400/10 border-yellow-400/30"
              : "text-yellow-300 bg-yellow-300/10 border-yellow-300/30"
          }`}
          role="alert"
        >
          <AlertCircle size={16} />
          <span>
            {isMutExpired
              ? "Your Apple Music developer token has expired. Re-authorize to continue syncing."
              : "Your Apple Music developer token expires soon."}
          </span>
          <button
            onClick={onDisconnect}
            className="ml-auto text-xs underline hover:no-underline"
          >
            Re-authorize
          </button>
        </div>
      )}

      {err && (
        <div
          className="flex items-start gap-2 text-sm text-red-400 bg-red-400/10 border border-red-400/30 rounded px-3 py-2"
          role="alert"
        >
          <AlertCircle size={16} className="shrink-0 mt-0.5" />
          <span className="break-words">{err}</span>
        </div>
      )}
      {okMsg && !err && (
        <div className="flex items-center gap-2 text-sm text-green-400 bg-green-400/10 border border-green-400/30 rounded px-3 py-2">
          <CheckCircle2 size={16} />
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
              [REDACTED PRIVATE KEY] into the form below, then click "Test credentials" to verify before connecting.
            </li>
          </ol>
          <p className="text-xs text-muted">
            See Apple's{" "}
            <a
              className="text-accent underline"
              href="https://developer.apple.com/documentation/applemusicapi"
              target="_blank"
              rel="noreferrer"
            >
              Apple Music API docs
            </a>{" "}
            for more details. Common pitfalls: ensure the .p8 file hasn't expired,
            the Key ID matches the downloaded key, and the Team ID is from your
            Apple Developer account (not your Apple ID).
          </p>
        </div>
      )}

      {/* State 1: Not configured — show form */}
      {showForm && (
        <form onSubmit={onSaveConfig} className="space-y-3">
          <div>
            <label
              className="text-xs uppercase tracking-wide text-muted"
              htmlFor="am-team-id"
            >
              Team ID
            </label>
            <input
              id="am-team-id"
              value={teamId}
              onChange={(e) => setTeamId(e.target.value)}
              placeholder="e.g. 2AB34CDEF5"
              className="w-full mt-1 px-3 py-2 bg-panel2 rounded text-sm"
              maxLength={10}
              required
              aria-describedby="am-team-id-help"
            />
            <p id="am-team-id-help" className="text-xs text-muted mt-1">
              10-character Apple Developer Team ID.
            </p>
          </div>

          <div>
            <label
              className="text-xs uppercase tracking-wide text-muted"
              htmlFor="am-key-id"
            >
              Key ID
            </label>
            <input
              id="am-key-id"
              value={keyId}
              onChange={(e) => setKeyId(e.target.value)}
              placeholder="e.g. 3K4M6N8P0Q"
              className="w-full mt-1 px-3 py-2 bg-panel2 rounded text-sm"
              maxLength={10}
              required
              aria-describedby="am-key-id-help"
            />
            <p id="am-key-id-help" className="text-xs text-muted mt-1">
              10-character Key ID from the MusicKit key page.
            </p>
          </div>

          <div>
            <label
              className="text-xs uppercase tracking-wide text-muted"
              htmlFor="am-private-key"
            >
              Private Key (.p8)
            </label>
            <textarea
              id="am-private-key"
              value={privateKey}
              onChange={(e) => setPrivateKey(e.target.value)}
              placeholder={`-----BEGIN PRIVATE KEY-----\n...paste your .p8 contents here...\n-----END PRIVATE KEY-----`}
              className="w-full mt-1 px-3 py-2 bg-panel2 rounded text-xs font-mono"
              rows={6}
              spellCheck={false}
              required
              aria-describedby="am-private-key-help"
            />
            <p id="am-private-key-help" className="text-xs text-muted mt-1">
              Paste the full contents of your .p8 file.
            </p>
            <div className="mt-1">
              <input
                ref={fileInputRef}
                type="file"
                accept=".p8"
                onChange={onP8File}
                className="hidden"
                id="am-p8-file"
              />
              <label
                htmlFor="am-p8-file"
                className="text-xs text-accent underline cursor-pointer hover:no-underline"
              >
                Or upload .p8 file
              </label>
            </div>
          </div>

          {/* Phase 2.7: Searchable storefront */}
          <div>
            <label
              className="text-xs uppercase tracking-wide text-muted"
              htmlFor="am-storefront"
            >
              Storefront
            </label>
            <div className="relative mt-1">
              <Search
                size={14}
                className="absolute left-2 top-1/2 -translate-y-1/2 text-muted"
              />
              <input
                type="text"
                value={storefrontSearch}
                onChange={(e) => setStorefrontSearch(e.target.value)}
                placeholder="Search storefronts…"
                className="w-full pl-7 pr-3 py-1.5 bg-panel2 rounded text-sm"
                aria-label="Search storefronts"
              />
            </div>
            <select
              id="am-storefront"
              value={storefront}
              onChange={(e) => setStorefront(e.target.value)}
              className="w-full mt-1 px-3 py-2 bg-panel2 rounded text-sm"
              aria-describedby="am-storefront-help"
            >
              {filteredStorefronts.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name} ({s.id})
                </option>
              ))}
            </select>
            <p id="am-storefront-help" className="text-xs text-muted mt-1">
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
            {/* Phase 6.2: Show display_name */}
            {status?.display_name && (
              <Field label="Account">{status.display_name}</Field>
            )}
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
              <span
                className={
                  isMutExpired
                    ? "text-yellow-400"
                    : isMutExpiringSoon
                      ? "text-yellow-300"
                      : ""
                }
              >
                {status?.dev_token_expires_at
                  ? new Date(
                      status.dev_token_expires_at * 1000,
                    ).toLocaleDateString()
                  : "—"}
                {isMutExpired ? " (expired)" : isMutExpiringSoon ? " (expiring soon)" : ""}
              </span>
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
        <p className="text-sm text-muted" aria-live="polite">Loading…</p>
      )}

      <ConfirmModal
        open={disconnectConfirm}
        title="Disconnect Apple Music"
        message="Disconnect Apple Music? Your synced history stays."
        confirmLabel="Disconnect"
        variant="danger"
        onConfirm={handleDisconnect}
        onCancel={() => setDisconnectConfirm(false)}
      />
      <ConfirmModal
        open={deleteConfigConfirm}
        title="Delete Apple Music Credentials"
        message="Delete Apple Music credentials? You'll need to paste them again to reconnect."
        confirmLabel="Delete"
        variant="danger"
        onConfirm={handleDeleteConfig}
        onCancel={() => setDeleteConfigConfirm(false)}
      />
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
