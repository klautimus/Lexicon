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
  HelpCircle,
} from "lucide-react";
import { PlayerProvider } from "./player/PlayerContext";
import { ToastProvider } from "./contexts/ToastContext";
import { DownloadProvider } from "./contexts/DownloadContext";
import { HelpProvider, useHelp } from "./contexts/HelpContext";
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
  { to: "/", label: "Home", icon: Library, end: true, helpKey: "home.stats" },
  { to: "/music", label: "Music", icon: Music, helpKey: "music.library" },
  { to: "/podcasts", label: "Podcasts", icon: Mic, helpKey: "podcasts.feeds" },
  { to: "/playlists", label: "Playlists", icon: ListMusic, helpKey: "playlists.grid" },
  { to: "/downloads", label: "Downloads", icon: Download, helpKey: "downloads.mode" },
  { to: "/analytics", label: "Analytics", icon: BarChart3, helpKey: "analytics.charts" },
  { to: "/discover", label: "Discover", icon: Sparkles, helpKey: "discover.generate" },
  { to: "/search", label: "Search", icon: Search, helpKey: "search.main" },
  { to: "/settings", label: "Settings", icon: SettingsIcon, helpKey: "settings.spotify" },
];

function NavItem({ item, onClick }: { item: typeof navItems[0]; onClick?: () => void }) {
  const { showHelp } = useHelp();
  return (
    <div className="flex items-center gap-1">
      <NavLink
        to={item.to}
        end={item.end}
        onClick={onClick}
        className={({ isActive }) =>
          `flex items-center gap-3 px-3 py-2 rounded-md text-sm transition flex-1 ${
            isActive
              ? "bg-panel2 text-accent"
              : "text-muted hover:text-text hover:bg-panel2/50"
          }`
        }
      >
        <item.icon size={16} />
        {item.label}
      </NavLink>
      {item.helpKey && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            showHelp(item.helpKey!);
          }}
          className="p-1 text-muted/50 hover:text-accent transition-colors rounded hover:bg-panel2/50"
          aria-label={`Help: ${item.label}`}
          title={`Learn about ${item.label}`}
        >
          <HelpCircle size={12} />
        </button>
      )}
    </div>
  );
}

function DesktopLayout() {
  const { showHelp } = useHelp();
  return (
    <div className="flex h-screen">
      <aside className="w-56 bg-panel border-r border-black/40 flex flex-col">
        <div className="px-5 py-4 border-b border-black/40">
          <div className="flex items-center gap-2.5">
            <img src="/icon.svg" alt="Lexicon" className="w-7 h-7 rounded" />
            <span className="text-lg font-semibold tracking-wide">Lexicon</span>
          </div>
        </div>
        <nav className="flex-1 p-2 space-y-1">
          {navItems.map((item) => (
            <NavItem key={item.to} item={item} />
          ))}
        </nav>
        <div className="p-2 space-y-1">
          <button
            onClick={() => api.scan()}
            className="w-full px-3 py-2 text-xs text-muted hover:text-text border border-panel2 rounded-md flex items-center justify-center gap-2 transition-colors"
          >
            <RefreshCw size={12} /> Rescan library
          </button>
          <button
            onClick={() => showHelp("nav.rescan")}
            className="w-full px-3 py-2 text-xs text-muted/50 hover:text-accent flex items-center justify-center gap-2 transition-colors"
          >
            <HelpCircle size={12} /> Help
          </button>
        </div>
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

function AppContent() {
  const isMobile = useIsMobile();
  return isMobile ? <MobileLayout /> : <DesktopLayout />;
}

export default function App() {
  return (
    <ErrorBoundary>
      <ToastProvider>
        <PlayerProvider>
          <DownloadProvider>
            <HelpProvider>
              <AppContent />
            </HelpProvider>
          </DownloadProvider>
        </PlayerProvider>
      </ToastProvider>
    </ErrorBoundary>
  );
}
