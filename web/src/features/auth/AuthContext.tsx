import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { api, configureClient, json } from "@/shared/api/client";
import type { Principal, Role, Session } from "@/shared/api/types";

const STORAGE_KEY = "plantorq.session";

interface AuthState {
  session: Session | null;
  login(email: string, password: string): Promise<void>;
  logout(): void;
  can(role: Role): boolean;
}

const AuthContext = createContext<AuthState | null>(null);

const rank: Record<Role, number> = { viewer: 1, operator: 2, admin: 3 };

function loadSession(): Session | null {
  try {
    const s = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "null") as Session | null;
    return s && new Date(s.expires_at) > new Date() ? s : null;
  } catch {
    return null;
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(loadSession);

  const logout = useCallback(() => {
    try {
      localStorage.removeItem(STORAGE_KEY);
    } catch {
      /* storage unavailable */
    }
    setSession(null);
  }, []);

  // Wire the HTTP client to the current token; a 401 logs the user out.
  useEffect(() => configureClient(() => session?.token ?? null, logout), [session, logout]);

  const login = useCallback(async (email: string, password: string) => {
    const s = await api<Session>("/auth/login", { method: "POST", ...json({ email, password }) });
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(s));
    } catch {
      /* storage unavailable: session lasts for this tab only */
    }
    setSession(s);
  }, []);

  const value = useMemo<AuthState>(
    () => ({ session, login, logout, can: (role) => !!session && rank[session.user.role] >= rank[role] }),
    [session, login, logout],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside <AuthProvider>");
  return ctx;
}

export function useUser(): Principal {
  const { session } = useAuth();
  if (!session) throw new Error("useUser requires an authenticated session");
  return session.user;
}
