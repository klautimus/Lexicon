import { Routes, Route, NavLink } from "react-router-dom";
import {
  Library,
  Music,
  Mic,
  BarChart3,
  Sparkles,
  Search,
  RefreshCw,
  Download,
  ListMusic,
  Settings as SettingsIcon,
} from "lucide-react";
import { PlayerProvider } from "./player/PlayerContext";
import { ToastProvider } from "./contexts/ToastContext";
import { DownloadProvider } from "./contexts/DownloadContext";
import PlayerBar from "./components/PlayerBar";
import MobileNavBar from "./components/MobileNavBar";
import MobilePlayerBar from "./components/MobilePlayerBar";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { useIsMobile } from "./hooks/useIsMobile";
import HomePage from "./pages/HomePage";
import MusicPage from "./pages/MusicPage";
import PodcastsPage from "./pages/PodcastsPage";
import AnalyticsPage from "./pages/AnalyticsPage";
import RecsPage from "./pages/RecsPage";
import SearchPage from "./pages/SearchPage";
import SettingsPage from "./pages/SettingsPage";
import DownloadsPage from "./pages/DownloadsPage";
import PlaylistsPage from "./pages/PlaylistsPage";
import PlaylistPage from "./pages/PlaylistPage";
import { api } from "./lib/api";

const navItems = [
  { to: "/", label: "Home", icon: Library, end: true },
  { to: "/music", label: "Music", icon: Music },
  { to: "/podcasts", label: "Podcasts", icon: Mic },
  { to: "/playlists", label: "Playlists", icon: ListMusic },
  { to: "/downloads", label: "Downloads", icon: Download },
  { to: "/analytics", label: "Analytics", icon: BarChart3 },
  { to: "/discover", label: "Discover", icon: Sparkles },
  { to: "/search", label: "Search", icon: Search },
  { to: "/settings", label: "Settings", icon: SettingsIcon },
];

function DesktopLayout() {
  return (
    <div className="flex h-screen">
      <aside className="w-56 bg-panel border-r border-black/40 flex flex-col">
        <div className="px-5 py-4 border-b border-black/40">
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-full border-2 border-accent flex items-center justify-center">
              <div className="w-2 h-2 rounded-full bg-accent" />
            </div>
            <span className="text-lg font-semibold tracking-wide">Lexicon</span>
          </div>
        </div>
        <nav className="flex-1 p-2 space-y-1">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2 rounded-md text-sm transition ${
                  isActive
                    ? "bg-panel2 text-accent"
                    : "text-muted hover:text-text hover:bg-panel2/50"
                }`
              }
            >
              <item.icon size={16} />
              {item.label}
            </NavLink>
          ))}
        </nav>
        <button
          onClick={() => api.scan()}
          className="m-2 px-3 py-2 text-xs text-muted hover:text-text border border-panel2 rounded-md flex items-center justify-center gap-2"
        >
          <RefreshCw size={12} /> Rescan library
        </button>
      </aside>

      <main className="flex-1 flex flex-col overflow-hidden">
        <div className="flex-1 overflow-auto p-6">
          <Routes>
            <Route path="/" element={<HomePage />} />
            <Route path="/music" element={<MusicPage />} />
            <Route path="/podcasts" element={<PodcastsPage />} />
            <Route path="/analytics" element={<AnalyticsPage />} />
            <Route path="/discover" element={<RecsPage />} />
            <Route path="/downloads" element={<DownloadsPage />} />
            <Route path="/playlists" element={<PlaylistsPage />} />
            <Route path="/playlists/:id" element={<PlaylistPage />} />
            <Route path="/search" element={<SearchPage />} />
            <Route path="/settings" element={<SettingsPage />} />
          </Routes>
        </div>
        <PlayerBar />
      </main>
    </div>
  );
}

function MobileLayout() {
  return (
    <div className="flex flex-col h-screen pb-14">
      <main className="flex-1 overflow-auto p-4 pb-28">
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/music" element={<MusicPage />} />
          <Route path="/podcasts" element={<PodcastsPage />} />
          <Route path="/analytics" element={<AnalyticsPage />} />
          <Route path="/discover" element={<RecsPage />} />
          <Route path="/downloads" element={<DownloadsPage />} />
          <Route path="/playlists" element={<PlaylistsPage />} />
          <Route path="/playlists/:id" element={<PlaylistPage />} />
          <Route path="/search" element={<SearchPage />} />
          <Route path="/settings" element={<SettingsPage />} />
        </Routes>
      </main>
      <MobilePlayerBar />
      <MobileNavBar />
    </div>
  );
}

export default function App() {
  const isMobile = useIsMobile();
  return (
    <ErrorBoundary>
      <ToastProvider>
        <PlayerProvider>
          <DownloadProvider>
            {isMobile ? <MobileLayout /> : <DesktopLayout />}
          </DownloadProvider>
        </PlayerProvider>
      </ToastProvider>
    </ErrorBoundary>
  );
}
