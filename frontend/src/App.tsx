import { lazy, Suspense } from "react";
import { Routes, Route, NavLink, Navigate } from "react-router-dom";
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
  User,
  LogOut,
  Shield,
  Loader2,
} from "lucide-react";
import { PlayerProvider } from "./player/PlayerContext";
import { ToastProvider } from "./contexts/ToastContext";
import { DownloadProvider } from "./contexts/DownloadContext";
import { HelpProvider, useHelp } from "./contexts/HelpContext";
import { UserProvider, useUser } from "./contexts/UserContext";
import PlayerBar from "./components/PlayerBar";
import MobileNavBar from "./components/MobileNavBar";
import MobilePlayerBar from "./components/MobilePlayerBar";
import DownloadProgressBar from "./components/DownloadProgressBar";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { useIsMobile } from "./hooks/useIsMobile";
import HomePage from "./pages/HomePage";
import PodcastsPage from "./pages/PodcastsPage";
import AnalyticsPage from "./pages/AnalyticsPage";
import RecsPage from "./pages/RecsPage";
import SearchPage from "./pages/SearchPage";
import SettingsPage from "./pages/SettingsPage";
import DownloadsPage from "./pages/DownloadsPage";
import PlaylistsPage from "./pages/PlaylistsPage";
import PlaylistPage from "./pages/PlaylistPage";
import LoginPage from "./pages/LoginPage";
import AdminUsersPage from "./pages/AdminUsersPage";
import NotFoundPage from "./pages/NotFoundPage";
import { api } from "./lib/api";

// 6.8: Code split MusicPage with React.lazy
const MusicPage = lazy(() => import("./pages/MusicPage"));

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

// Auth guard: redirects to /login if no valid session
function AuthGuard({ children }: { children: React.ReactNode }) {
  const { user, loading } = useUser();

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen bg-bg">
        <Loader2 size={24} className="animate-spin text-muted" />
      </div>
    );
  }

  if (!user) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
}

// Admin guard: redirects to / if not admin
function AdminGuard({ children }: { children: React.ReactNode }) {
  const { user, isAdmin, loading } = useUser();

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen bg-bg">
        <Loader2 size={24} className="animate-spin text-muted" />
      </div>
    );
  }

  if (!user) {
    return <Navigate to="/login" replace />;
  }

  if (!isAdmin) {
    return <Navigate to="/" replace />;
  }

  return <>{children}</>;
}

function DesktopLayout() {
  const { showHelp } = useHelp();
  const { user, logout, isAdmin } = useUser();

  return (
    <div className="flex h-screen">
      <aside className="w-56 bg-panel border-r border-black/40 flex flex-col">
        <div className="px-5 py-4 border-b border-black/40">
          <div className="flex items-center gap-2.5">
            <img src="/icon.svg" alt="Lexicon" className="w-7 h-7 rounded" />
            <span className="text-lg font-semibold tracking-wide">Lexicon</span>
          </div>
        </div>
        <nav className="flex-1 p-2 space-y-1" aria-label="Main navigation">
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

        {/* User section */}
        {user && (
          <div className="p-2 border-t border-black/40 space-y-1">
            <div className="flex items-center gap-2 px-2 py-1.5">
              <User size={14} className="text-muted flex-shrink-0" />
              <span className="text-sm text-text truncate">
                {user.display_name || user.username}
              </span>
              {isAdmin && (
                <span title="Admin">
                  <Shield size={12} className="text-accent/70 flex-shrink-0" />
                </span>
              )}
            </div>
            <div className="flex gap-1 px-1">
              {isAdmin && (
                <NavLink
                  to="/settings/users"
                  className="text-xs text-muted hover:text-text px-2 py-1 rounded transition-colors hover:bg-panel2/50"
                >
                  Users
                </NavLink>
              )}
              <button
                onClick={logout}
                className="text-xs text-muted hover:text-red-400 px-2 py-1 rounded transition-colors hover:bg-panel2/50 flex items-center gap-1"
              >
                <LogOut size={11} />
                Logout
              </button>
            </div>
          </div>
        )}
      </aside>

      <main className="flex-1 flex flex-col overflow-hidden">
        <DownloadProgressBar />
        <div className="flex-1 overflow-auto p-6">
          <Routes>
            <Route path="/" element={<HomePage />} />
            <Route path="/music" element={<Suspense fallback={<div className="flex items-center justify-center h-full"><Loader2 size={24} className="animate-spin text-muted" /></div>}><MusicPage /></Suspense>} />
            <Route path="/podcasts" element={<PodcastsPage />} />
            <Route path="/analytics" element={<AnalyticsPage />} />
            <Route path="/discover" element={<RecsPage />} />
            <Route path="/downloads" element={<DownloadsPage />} />
            <Route path="/playlists" element={<PlaylistsPage />} />
            <Route path="/playlists/:id" element={<PlaylistPage />} />
            <Route path="/search" element={<SearchPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/settings/users" element={<AdminGuard><AdminUsersPage /></AdminGuard>} />
            <Route path="*" element={<NotFoundPage />} />
          </Routes>
        </div>
        <PlayerBar />
      </main>
    </div>
  );
}

function MobileLayout() {
  const { user, logout, isAdmin } = useUser();

  return (
    <div className="flex flex-col h-screen pb-14">
      {/* User bar (top) */}
      {user && (
        <div className="flex items-center justify-between px-4 py-1.5 bg-panel border-b border-black/40 text-xs flex-shrink-0">
          <span className="text-muted flex items-center gap-1.5 truncate">
            <User size={12} className="flex-shrink-0" />
            {user.display_name || user.username}
            {isAdmin && (
              <Shield size={10} className="text-accent/70 flex-shrink-0" />
            )}
          </span>
          <button
            onClick={logout}
            className="text-muted hover:text-red-400 flex items-center gap-1 transition-colors"
          >
            <LogOut size={11} />
            Logout
          </button>
        </div>
      )}

      <DownloadProgressBar />
      <main className="flex-1 overflow-auto p-4 pb-28">
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/music" element={<Suspense fallback={<div className="flex items-center justify-center h-full"><Loader2 size={24} className="animate-spin text-muted" /></div>}><MusicPage /></Suspense>} />
          <Route path="/podcasts" element={<PodcastsPage />} />
          <Route path="/analytics" element={<AnalyticsPage />} />
          <Route path="/discover" element={<RecsPage />} />
          <Route path="/downloads" element={<DownloadsPage />} />
          <Route path="/playlists" element={<PlaylistsPage />} />
          <Route path="/playlists/:id" element={<PlaylistPage />} />
          <Route path="/search" element={<SearchPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/settings/users" element={<AdminGuard><AdminUsersPage /></AdminGuard>} />
          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </main>
      <MobilePlayerBar />
      <MobileNavBar />
    </div>
  );
}

function AppContent() {
  const isMobile = useIsMobile();

  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="*"
        element={
          <AuthGuard>
            {isMobile ? <MobileLayout /> : <DesktopLayout />}
          </AuthGuard>
        }
      />
    </Routes>
  );
}

export default function App() {
  return (
    <ErrorBoundary>
      <ToastProvider>
        <PlayerProvider>
          <DownloadProvider>
            <HelpProvider>
              <UserProvider>
                <AppContent />
              </UserProvider>
            </HelpProvider>
          </DownloadProvider>
        </PlayerProvider>
      </ToastProvider>
    </ErrorBoundary>
  );
}
