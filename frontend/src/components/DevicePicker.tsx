import { useState, useEffect, useRef } from "react";
import { Monitor, Smartphone, Speaker, ChevronDown, Check } from "lucide-react";
import { api, SpotifyDevice } from "../lib/api";
import { getPlayerWebSocket, fetchSpotifyDevices, transferSpotifyPlayback } from "../lib/playerws";

interface Device {
  id: string;
  name: string;
  type: "player" | "controller" | "spotify";
  active: boolean;
}

export default function DevicePicker({ currentTrack }: { currentTrack?: any }) {
  const [open, setOpen] = useState(false);
  const [devices, setDevices] = useState<Device[]>([]);
  const [spotifyDevices, setSpotifyDevices] = useState<SpotifyDevice[]>([]);
  const [activeDevice, setActiveDevice] = useState<string>("host");
  const ref = useRef<HTMLDivElement>(null);
  const ws = getPlayerWebSocket();

  // Fetch devices on open
  useEffect(() => {
    if (!open) return;

    // Get WebSocket-connected devices
    const msg = JSON.parse(localStorage.getItem("playerDevices") || '{"list":[]}');
    const wsDevices: Device[] = msg.list || [];
    setDevices(wsDevices);

    // Get Spotify Connect devices
    fetchSpotifyDevices().then((sd) => {
      setSpotifyDevices(sd);
    });

    // Listen for device updates
    const handler = (m: any) => {
      if (m.type === "devices") {
        setDevices(m.list || []);
        localStorage.setItem("playerDevices", JSON.stringify(m));
      }
    };
    ws.onDevices(handler as any);

    // Poll Spotify devices every 10s
    const interval = setInterval(() => {
      fetchSpotifyDevices().then(setSpotifyDevices);
    }, 10000);

    return () => {
      clearInterval(interval);
    };
  }, [open]);

  // Close on outside click
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  const handleTransfer = async (device: Device) => {
    if (device.type === "spotify") {
      await transferSpotifyPlayback(device.id, true);
    } else if (device.id === "host") {
      // Switch to local playback — reload page to take over as player
      sessionStorage.setItem("playerActive", "1");
      window.location.reload();
    } else {
      ws.transfer(device.id);
    }
    setActiveDevice(device.id);
    setOpen(false);
  };

  const activeName = devices.find((d) => d.id === activeDevice)?.name || "This Device";
  const activeType = devices.find((d) => d.id === activeDevice)?.type || "controller";

  const getIcon = (type: string) => {
    switch (type) {
      case "player":
        return <Monitor size={14} />;
      case "spotify":
        return <Speaker size={14} />;
      default:
        return <Smartphone size={14} />;
    }
  };

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-1 text-muted hover:text-text transition-colors text-xs p-1.5 -m-1.5 rounded hover:bg-panel2/50"
        title={`Playing on: ${activeName}`}
      >
        {getIcon(activeType)}
        <span className="hidden sm:inline max-w-24 truncate">{activeName}</span>
        <ChevronDown size={12} className={`transition-transform ${open ? "rotate-180" : ""}`} />
      </button>

      {open && (
        <div className="absolute bottom-full right-0 mb-2 w-64 max-w-[calc(100vw-1rem)] bg-panel border border-panel2 rounded-lg shadow-xl z-50 overflow-hidden">
          <div className="px-3 py-2 border-b border-panel2 text-xs text-muted uppercase tracking-wider">
            Playing On
          </div>

          {/* Scrollable device list */}
          <div className="max-h-[50vh] overflow-y-auto">
            {/* This device */}
            <button
              onClick={() => handleTransfer({ id: "self", name: "This Device", type: "controller", active: activeDevice === "self" })}
              className="w-full flex items-center gap-3 px-3 py-2.5 hover:bg-panel2 transition-colors text-left"
            >
              <Smartphone size={14} className={activeDevice === "self" ? "text-accent" : "text-muted"} />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium truncate">This Device</p>
                <p className="text-xs text-muted">Stream audio here</p>
              </div>
              {activeDevice === "self" && <Check size={14} className="text-accent" />}
            </button>

            {/* Host computer */}
            <button
              onClick={() => handleTransfer({ id: "host", name: "Host Computer", type: "player", active: activeDevice === "host" })}
              className="w-full flex items-center gap-3 px-3 py-2.5 hover:bg-panel2 transition-colors text-left"
            >
              <Monitor size={14} className={activeDevice === "host" ? "text-accent" : "text-muted"} />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium truncate">Host Computer</p>
                <p className="text-xs text-muted">
                  {currentTrack ? `Playing: ${currentTrack.title}` : "Control host playback"}
                </p>
              </div>
              {activeDevice === "host" && <Check size={14} className="text-accent" />}
            </button>

            {/* WebSocket-connected devices */}
            {devices
              .filter((d) => d.id !== "host" && d.type !== "spotify")
              .map((device) => (
                <button
                  key={device.id}
                  onClick={() => handleTransfer(device)}
                  className="w-full flex items-center gap-3 px-3 py-2.5 hover:bg-panel2 transition-colors text-left"
                >
                  {getIcon(device.type)}
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium truncate">{device.name}</p>
                    <p className="text-xs text-muted capitalize">{device.type}</p>
                  </div>
                  {device.active && <Check size={14} className="text-accent" />}
                </button>
              ))}

            {/* Spotify Connect devices */}
            {spotifyDevices.length > 0 && (
              <>
                <div className="px-3 py-1.5 border-t border-panel2 text-xs text-muted uppercase tracking-wider">
                  Spotify Connect
                </div>
                {spotifyDevices.map((device) => (
                  <button
                    key={device.id}
                    onClick={() => handleTransfer({ id: device.id, name: device.name, type: "spotify", active: device.is_active })}
                    className="w-full flex items-center gap-3 px-3 py-2.5 hover:bg-panel2 transition-colors text-left"
                  >
                    <Speaker size={14} className={device.is_active ? "text-accent" : "text-muted"} />
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium truncate">{device.name}</p>
                      <p className="text-xs text-muted">{device.type} {device.is_active ? "• Active" : ""}</p>
                    </div>
                    {device.is_active && <Check size={14} className="text-accent" />}
                  </button>
                ))}
              </>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
