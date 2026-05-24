import { useState, useRef, useEffect, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { Eye, EyeOff, Loader2, Music } from "lucide-react";
import { useUser } from "../contexts/UserContext";

export default function LoginPage() {
  const { login, user, loading: sessionLoading } = useUser();
  const navigate = useNavigate();
  const usernameRef = useRef<HTMLInputElement>(null);

  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  // M4: Redirect to / if already authenticated
  useEffect(() => {
    if (!sessionLoading && user) {
      navigate("/", { replace: true });
    }
  }, [sessionLoading, user, navigate]);

  // B3 fix: Use usernameRef for focus management on error
  useEffect(() => {
    if (error && usernameRef.current) {
      usernameRef.current.focus();
    }
  }, [error]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    const u = username.trim();
    if (!u || !password) {
      setError("Please enter both username and password.");
      return;
    }
    // B1 fix: Guard against rapid submit — disable immediately
    if (submitting) return;
    setError("");
    setSubmitting(true);
    try {
      await login(u, password);
      navigate("/", { replace: true });
    } catch (err: any) {
      const msg = err?.message || "Login failed";
      if (msg.includes("401") || msg.includes("invalid") || msg.includes("wrong")) {
        setError("Invalid username or password.");
      } else if (msg.includes("Unable to reach") || msg.includes("Network")) {
        setError("Unable to reach the server. Make sure Lexicon is running.");
      } else {
        setError(msg.length < 120 ? msg : "Login failed. Please try again.");
      }
      // P4 fix: Clear password on failed login
      setPassword("");
    } finally {
      setSubmitting(false);
    }
  }

  // Show loading spinner while checking session
  if (sessionLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-bg px-4">
        <Loader2 size={24} className="animate-spin text-muted" />
      </div>
    );
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-bg px-4">
      <div className="w-full max-w-sm">
        {/* Logo + brand */}
        <div className="text-center mb-8">
          <div className="inline-flex items-center justify-center w-14 h-14 rounded-xl bg-panel2 mb-4">
            <Music size={28} className="text-accent" />
          </div>
          <h1 className="text-2xl font-semibold text-text tracking-wide">Lexicon</h1>
          <p className="text-muted text-sm mt-1">Your personal music library</p>
        </div>

        {/* Login card */}
        <form
          onSubmit={handleSubmit}
          className="bg-panel border border-panel2 rounded-xl p-6 space-y-4"
          aria-label="Sign in to Lexicon"
          name="login"
        >
          {/* Username */}
          <div>
            <label htmlFor="login-username" className="block text-xs text-muted mb-1.5">
              Username
            </label>
            <input
              ref={usernameRef}
              id="login-username"
              type="text"
              autoFocus
              autoComplete="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="w-full bg-bg border border-panel2 rounded-md px-3 py-2 text-sm text-text placeholder:text-muted/50 focus:outline-none focus:border-accent/40 transition-colors"
              placeholder="Enter username"
              aria-required="true"
              aria-describedby={error ? "login-error" : undefined}
            />
          </div>

          {/* Password */}
          <div>
            <label htmlFor="login-password" className="block text-xs text-muted mb-1.5">
              Password
            </label>
            <div className="relative">
              <input
                id="login-password"
                type={showPassword ? "text" : "password"}
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="w-full bg-bg border border-panel2 rounded-md px-3 py-2 pr-9 text-sm text-text placeholder:text-muted/50 focus:outline-none focus:border-accent/40 transition-colors"
                placeholder="Enter password"
                aria-required="true"
                aria-describedby={error ? "login-error" : undefined}
              />
              <button
                type="button"
                onClick={() => setShowPassword((p) => !p)}
                className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-muted hover:text-text transition-colors"
                tabIndex={-1}
                aria-label={showPassword ? "Hide password" : "Show password"}
                aria-pressed={showPassword}
              >
                {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
              </button>
            </div>
          </div>

          {/* Error — V1 fix: proper spacing with mt, A2/A3 fix: role="alert" + aria-live */}
          {error && (
            <p
              id="login-error"
              className="text-xs text-red-400 bg-red-400/10 border border-red-400/20 rounded-md px-3 py-2 mt-2"
              role="alert"
              aria-live="polite"
            >
              {error}
            </p>
          )}

          {/* Submit */}
          <button
            type="submit"
            disabled={submitting}
            className="w-full bg-accent text-bg font-medium rounded-md px-4 py-2.5 text-sm hover:bg-accent/90 disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center justify-center gap-2"
          >
            {submitting ? (
              <>
                <Loader2 size={16} className="animate-spin" />
                Signing in…
              </>
            ) : (
              "Sign in"
            )}
          </button>
        </form>
      </div>
    </div>
  );
}
