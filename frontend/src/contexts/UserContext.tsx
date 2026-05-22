import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from "react";
import { api, setSessionToken, type User } from "../lib/api";

interface UserContextValue {
  user: User | null;
  token: string | null;
  loading: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  isAdmin: boolean;
}

const UserContext = createContext<UserContextValue | null>(null);

export function UserProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  // Check session validity on mount
  useEffect(() => {
    let cancelled = false;
    const saved = localStorage.getItem("lexicon_session");
    if (!saved) {
      setLoading(false);
      return;
    }
    // Validate token against backend
    api.me()
      .then((data) => {
        if (cancelled) return;
        setToken(saved);
        setUser(data.user);
      })
      .catch((err) => {
        if (cancelled) return;
        const msg = err?.message || "";
        // Only clear session on 401 (invalid token), not on network errors
        if (msg.includes("401") || msg.includes("Session expired")) {
          setSessionToken(null);
          setToken(null);
          setUser(null);
        }
        // Network errors: keep the token, user can retry when back online
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, []);

  const login = useCallback(async (username: string, password: string) => {
    const data = await api.login(username, password);
    setSessionToken(data.token);
    setToken(data.token);
    setUser(data.user);
  }, []);

  const logout = useCallback(async () => {
    try { await api.logout(); } catch { /* best-effort */ }
    setSessionToken(null);
    setToken(null);
    setUser(null);
  }, []);

  const isAdmin = user?.is_admin ?? false;

  return (
    <UserContext.Provider value={{ user, token, loading, login, logout, isAdmin }}>
      {children}
    </UserContext.Provider>
  );
}

export function useUser(): UserContextValue {
  const ctx = useContext(UserContext);
  if (!ctx) throw new Error("useUser must be used within UserProvider");
  return ctx;
}
