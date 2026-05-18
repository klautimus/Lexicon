import { api, SpotifyDevice } from "./api";

export interface PlayerCommand {
  type: string;
  track_id?: number;
  position?: number;
  target?: string;
  playing?: boolean;
  track?: any;
  duration?: number;
  device?: string;
}

export interface PlayerState {
  type: "state";
  playing: boolean;
  track: any;
  position: number;
  duration: number;
  device: string;
}

export interface DeviceList {
  type: "devices";
  list: Array<{
    id: string;
    name: string;
    type: string;
    active: boolean;
  }>;
}

type MessageHandler = (msg: PlayerState | DeviceList) => void;

const WS_RETRY_DELAY = 3000;
const WS_MAX_RETRIES = 10;

export class PlayerWebSocket {
  private ws: WebSocket | null = null;
  private deviceID: string;
  private role: "player" | "controller";
  private name: string;
  private url: string;
  private retries = 0;
  private handlers: MessageHandler[] = [];
  private stateHandlers: MessageHandler[] = [];
  private deviceHandlers: MessageHandler[] = [];
  private intentionalClose = false;

  constructor() {
    this.deviceID = localStorage.getItem("playerDeviceID") || this.generateID();
    localStorage.setItem("playerDeviceID", this.deviceID);

    // First device to connect becomes the player
    const isFirst = !sessionStorage.getItem("playerActive");
    this.role = isFirst ? "player" : "controller";
    if (isFirst) sessionStorage.setItem("playerActive", "1");

    this.name = this.detectName();

    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    this.url = `${proto}//${location.host}/api/ws/player?deviceID=${this.deviceID}&role=${this.role}&name=${encodeURIComponent(this.name)}`;
  }

  private generateID(): string {
    return "dev-" + Math.random().toString(36).substring(2, 10);
  }

  private detectName(): string {
    const ua = navigator.userAgent;
    if (/iPhone/.test(ua)) return "iPhone";
    if (/iPad/.test(ua)) return "iPad";
    if (/Android/.test(ua)) return "Android Device";
    if (/Windows/.test(ua)) return "Windows PC";
    if (/Mac/.test(ua)) return "Mac";
    return "Browser";
  }

  connect(): void {
    this.intentionalClose = false;
    try {
      this.ws = new WebSocket(this.url);
      this.ws.onopen = () => {
        console.log("[playerws] connected as", this.role);
        this.retries = 0;
      };
      this.ws.onmessage = (ev) => {
        try {
          const msg = JSON.parse(ev.data);
          if (msg.type === "state") {
            this.stateHandlers.forEach((h) => h(msg));
          } else if (msg.type === "devices") {
            this.deviceHandlers.forEach((h) => h(msg));
          }
          this.handlers.forEach((h) => h(msg));
        } catch {
          /* ignore parse errors */
        }
      };
      this.ws.onclose = () => {
        if (!this.intentionalClose && this.retries < WS_MAX_RETRIES) {
          this.retries++;
          setTimeout(() => this.connect(), WS_RETRY_DELAY);
        }
      };
      this.ws.onerror = () => {
        /* onclose will fire next */;
      };
    } catch {
      /* WebSocket not available */
    }
  }

  disconnect(): void {
    this.intentionalClose = true;
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  send(cmd: PlayerCommand): void {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(cmd));
    }
  }

  play(trackId: number): void {
    this.send({ type: "play", track_id: trackId, position: 0 });
  }

  pause(): void {
    this.send({ type: "pause" });
  }

  resume(): void {
    this.send({ type: "resume" });
  }

  next(): void {
    this.send({ type: "next" });
  }

  prev(): void {
    this.send({ type: "prev" });
  }

  seek(position: number): void {
    this.send({ type: "seek", position });
  }

  transfer(target: string): void {
    this.send({ type: "transfer", target });
  }

  onState(handler: MessageHandler): void {
    this.stateHandlers.push(handler);
  }

  onDevices(handler: MessageHandler): void {
    this.deviceHandlers.push(handler);
  }

  getDeviceID(): string {
    return this.deviceID;
  }

  getRole(): string {
    return this.role;
  }

  isPlayer(): boolean {
    return this.role === "player";
  }
}

// Singleton
let instance: PlayerWebSocket | null = null;

export function getPlayerWebSocket(): PlayerWebSocket {
  if (!instance) {
    instance = new PlayerWebSocket();
  }
  return instance;
}

// Fetch Spotify Connect devices
export async function fetchSpotifyDevices(): Promise<SpotifyDevice[]> {
  try {
    return await api.spotifyDevices();
  } catch {
    return [];
  }
}

// Transfer Spotify playback to a device
export async function transferSpotifyPlayback(deviceId: string, play: boolean): Promise<boolean> {
  try {
    await api.spotifyTransfer(deviceId, play);
    return true;
  } catch {
    return false;
  }
}
