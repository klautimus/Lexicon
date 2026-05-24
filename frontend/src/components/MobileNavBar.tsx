import { useState } from "react";
import { NavLink, useLocation } from "react-router-dom";
import {
  Library,
  Music,
  Search,
  Sparkles,
  Menu,
  X,
  Mic,
  ListMusic,
  Download,
  BarChart3,
  Settings,
  Shield,
} from "lucide-react";
import { useUser } from "../contexts/UserContext";

const primaryTabs = [
  { to: "/", label: "Home", icon: Library },
  { to: "/music", label: "Music", icon: Music },
  { to: "/search", label: "Search", icon: Search },
  { to: "/discover", label: "Discover", icon: Sparkles },
];

const overflowItems = [
  { to: "/podcasts", label: "Podcasts", icon: Mic },
  { to: "/playlists", label: "Playlists", icon: ListMusic },
  { to: "/downloads", label: "Downloads", icon: Download },
  { to: "/analytics", label: "Analytics", icon: BarChart3 },
  { to: "/settings", label: "Settings", icon: Settings },
];

// Admin-only overflow items
const adminOverflowItems = [
  { to: "/settings/users", label: "Users", icon: Shield },
];

export default function MobileNavBar() {
  const [sheetOpen, setSheetOpen] = useState(false);
  const location = useLocation();
  const { isAdmin } = useUser();

  const allOverflowItems = isAdmin
    ? [...overflowItems.slice(0, -1), ...adminOverflowItems, overflowItems[overflowItems.length - 1]]
    : overflowItems;

  const isOverflowActive = allOverflowItems.some((i) =>
    location.pathname.startsWith(i.to)
  );

  return (
    <>
      {/* Bottom Tab Bar */}
      <nav className="fixed bottom-0 left-0 right-0 z-40 bg-panel border-t border-black/40 flex justify-around items-center h-14 pb-safe">
        {primaryTabs.map((item) => {
          const active =
            item.to === "/"
              ? location.pathname === "/"
              : location.pathname.startsWith(item.to);
          return (
            <NavLink
              key={item.to}
              to={item.to}
              className="flex flex-col items-center justify-center flex-1 h-full"
              aria-label={item.label}
            >
              <item.icon
                size={20}
                className={active ? "text-accent" : "text-muted"}
              />
              <span
                className={`text-[10px] mt-0.5 ${
                  active ? "text-accent font-medium" : "text-muted"
                }`}
              >
                {item.label}
              </span>
            </NavLink>
          );
        })}

        {/* More button */}
        <button
          onClick={() => setSheetOpen(true)}
          className="flex flex-col items-center justify-center flex-1 h-full"
          aria-label="More navigation options"
        >
          <Menu
            size={20}
            className={sheetOpen || isOverflowActive ? "text-accent" : "text-muted"}
          />
          <span
            className={`text-[10px] mt-0.5 ${
              sheetOpen || isOverflowActive
                ? "text-accent font-medium"
                : "text-muted"
            }`}
          >
            More
          </span>
        </button>
      </nav>

      {/* Overflow Sheet */}
      {sheetOpen && (
        <>
          <div
            className="fixed inset-0 z-50 bg-black/60"
            onClick={() => setSheetOpen(false)}
          />
          <div className="fixed bottom-14 left-0 right-0 z-50 bg-panel border-t border-panel2 rounded-t-xl p-4" style={{ animation: "slideUp 0.2s ease-out" }}>
            <div className="flex items-center justify-between mb-3">
              <div className="flex items-center gap-2">
                <img src="/icon.svg" alt="Lexicon" className="w-5 h-5 rounded" />
                <span className="text-sm font-semibold text-muted uppercase tracking-wide">
                  More
                </span>
              </div>
              <button
                onClick={() => setSheetOpen(false)}
                className="p-1 text-muted hover:text-text"
              >
                <X size={18} />
              </button>
            </div>
            <div className="grid grid-cols-3 gap-2">
              {allOverflowItems.map((item) => {
                const active = location.pathname.startsWith(item.to);
                return (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    onClick={() => setSheetOpen(false)}
                    className={`flex flex-col items-center gap-1.5 p-3 rounded-lg transition ${
                      active
                        ? "bg-panel2 text-accent"
                        : "text-muted hover:bg-panel2/50 hover:text-text"
                    }`}
                    aria-label={item.label}
                  >
                    <item.icon size={22} />
                    <span className="text-xs">{item.label}</span>
                  </NavLink>
                );
              })}
            </div>
          </div>
        </>
      )}
    </>
  );
}
