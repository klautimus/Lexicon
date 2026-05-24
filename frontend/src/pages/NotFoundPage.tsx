import { useNavigate } from "react-router-dom";
import { Music, Home } from "lucide-react";

export default function NotFoundPage() {
  const navigate = useNavigate();

  return (
    <div className="min-h-screen flex items-center justify-center bg-bg px-4">
      <div className="text-center">
        <div className="inline-flex items-center justify-center w-16 h-16 rounded-xl bg-panel2 mb-6">
          <Music size={32} className="text-muted" />
        </div>
        <h1 className="text-2xl font-semibold text-text mb-2">Page not found</h1>
        <p className="text-muted text-sm mb-6 max-w-xs">
          The page you're looking for doesn't exist or has been moved.
        </p>
        <button
          onClick={() => navigate("/", { replace: true })}
          className="inline-flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium bg-accent text-bg hover:bg-accent/90 transition-colors"
        >
          <Home size={16} />
          Go Home
        </button>
      </div>
    </div>
  );
}
