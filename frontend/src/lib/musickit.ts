/**
 * MusicKit JS wrapper.
 *
 * Loads MusicKit JS v1 lazily from Apple's CDN, configures it with a
 * developer token minted by our backend, and exposes a thin promise-based
 * API for the Settings page to authorize/unauthorize the user.
 *
 * The Music User Token (MUT) returned from authorize() must be POSTed to
 * /api/apple/connect so the backend can use it for /v1/me/* calls.
 *
 * MusicKit JS docs: https://js-cdn.music.apple.com/musickit/v1/index.html
 */

import { api } from "./api";

const MUSICKIT_SCRIPT_URL = "https://js-cdn.music.apple.com/musickit/v1/musickit.js";

// Phase 3.8: Timeout for authorize popup (30 seconds)
const AUTHORIZE_TIMEOUT_MS = 30000;

// Global stub typing for MusicKit JS — kept minimal to avoid a full d.ts.
declare global {
  interface Window {
    MusicKit?: {
      configure: (opts: {
        developerToken: string;
        app: { name: string; build: string };
        storefrontId?: string;
        suppressErrorDialog?: boolean;
      }) => unknown;
      getInstance: () => MusicKitInstance;
    };
  }
}

interface MusicKitInstance {
  isAuthorized: boolean;
  musicUserToken: string;
  authorize: () => Promise<string>;
  unauthorize: () => Promise<void> | void;
}

let scriptPromise: Promise<void> | null = null;
let configured = false;

/** Load the MusicKit JS script tag exactly once. */
function loadScript(): Promise<void> {
  if (typeof window === "undefined") {
    return Promise.reject(new Error("MusicKit JS can only load in a browser"));
  }
  if (window.MusicKit) {
    return Promise.resolve();
  }
  if (scriptPromise) return scriptPromise;

  scriptPromise = new Promise<void>((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>(
      `script[src="${MUSICKIT_SCRIPT_URL}"]`,
    );
    if (existing) {
      // Wait for musickitloaded if the script tag was added but not yet ready.
      if (window.MusicKit) {
        resolve();
        return;
      }
      document.addEventListener("musickitloaded", () => resolve(), {
        once: true,
      });
      return;
    }
    const s = document.createElement("script");
    s.src = MUSICKIT_SCRIPT_URL;
    s.async = true;
    s.onload = () => {
      // Apple emits a 'musickitloaded' event after the script self-initializes.
      if (window.MusicKit) {
        resolve();
        return;
      }
      document.addEventListener(
        "musickitloaded",
        () => resolve(),
        { once: true },
      );
    };
    s.onerror = () => reject(new Error("Failed to load MusicKit JS from Apple CDN"));
    document.head.appendChild(s);
  });
  return scriptPromise;
}

/** Configure MusicKit with the developer token from the backend. */
async function configure(): Promise<MusicKitInstance> {
  await loadScript();
  if (!window.MusicKit) {
    throw new Error("MusicKit unavailable after script load");
  }
  if (!configured) {
    const cfg = await api.appleMusicKitConfig();
    window.MusicKit.configure({
      developerToken: cfg.developer_token,
      app: { name: cfg.app_name || "Lexicon", build: "1.0" },
      storefrontId: cfg.storefront || "us",
      suppressErrorDialog: true,
    });
    configured = true;
  }
  return window.MusicKit.getInstance();
}

/**
 * Prompt the user to sign in with their Apple ID and authorize Lexicon.
 * Returns the Music User Token on success.
 *
 * Phase 3.8: Adds a timeout so the popup doesn't hang indefinitely if
 * the browser blocks it. Also detects if the popup was blocked.
 */
export async function authorizeAppleMusic(): Promise<string> {
  const music = await configure();

  // Phase 3.8: Race authorize() against a timeout
  let popupBlocked = false;
  setTimeout(() => {
    // If the window didn't blur, the popup was likely blocked
    // This is a heuristic — not 100% reliable but catches most cases
    if (document.hasFocus()) {
      popupBlocked = true;
    }
  }, 1000);

  const authorizePromise = music.authorize().then((result) => {
    const mut = (typeof result === "string" && result) || music.musicUserToken;
    if (!mut) {
      throw new Error("Apple Music did not return a user token");
    }
    return mut;
  });

  const timeoutPromise = new Promise<never>((_, reject) =>
    setTimeout(
      () => reject(new Error(`Authorization timed out after ${AUTHORIZE_TIMEOUT_MS / 1000}s. ${popupBlocked ? "Popups may be blocked — please allow popups for this site and try again." : "Please check your internet connection and try again."}`)),
      AUTHORIZE_TIMEOUT_MS,
    )
  );

  return Promise.race([authorizePromise, timeoutPromise]);
}

/** Sign the user out of Apple Music within this browser context. */
export async function unauthorizeAppleMusic(): Promise<void> {
  const music = await configure();
  await music.unauthorize();
}

/** True if MusicKit reports an authorized subscriber in this browser. */
export function isAppleMusicAuthorized(): boolean {
  if (typeof window === "undefined" || !window.MusicKit) return false;
  try {
    return window.MusicKit.getInstance().isAuthorized;
  } catch {
    return false;
  }
}

/** Reset internal config flag — used after the backend deletes credentials. */
export function resetMusicKit(): void {
  configured = false;
}
