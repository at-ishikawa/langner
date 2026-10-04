"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { usePathname } from "next/navigation";
import { getMe, redirectToSignIn, type AuthUser } from "@/lib/auth";
import { subscribeToken } from "@/lib/authToken";

interface SessionState {
  user: AuthUser | null;
  loading: boolean;
  // refresh re-probes /auth/me so callers (e.g. the Settings page) can pick up
  // a changed LLM-credential status without a full reload.
  refresh: () => Promise<void>;
}

const SessionContext = createContext<SessionState>({
  user: null,
  loading: true,
  refresh: async () => {},
});

// Routes that must render WITHOUT an authenticated session: the sign-in page
// and the OAuth callback landing (which is mid-way through acquiring a token).
const PUBLIC_PATHS = new Set(["/login", "/auth/callback"]);

// useSession exposes the current authenticated user (or null), whether the
// initial /auth/me probe is still in flight, and a refresh() to re-probe (e.g.
// after the Settings page saves an LLM key).
export function useSession(): SessionState {
  return useContext(SessionContext);
}

// SessionProvider probes /auth/me (with the in-memory bearer) on mount and
// whenever the token changes, and guards the app: once the probe resolves with
// no user, every route except the public ones silently re-authenticates by
// redirecting through Google (if the Google session is alive it returns with no
// user interaction). Auth is a bearer held in memory, so this guard is
// client-side only.
export function SessionProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [loading, setLoading] = useState(true);
  const pathname = usePathname();

  // probe re-reads /auth/me and updates state; returns a cancel fn so an
  // in-flight probe can be abandoned on unmount / token change.
  const probe = useCallback(() => {
    let active = true;
    getMe().then((u) => {
      if (active) {
        setUser(u);
        setLoading(false);
      }
    });
    return () => {
      active = false;
    };
  }, []);

  // refresh is the awaitable re-probe exposed to callers (e.g. the Settings
  // page, after saving an LLM key) so they can pick up the new /auth/me state.
  const refresh = useCallback(async () => {
    const u = await getMe();
    setUser(u);
  }, []);

  // Probe on mount and whenever the in-memory token changes (e.g. the OAuth
  // callback stores a freshly minted token).
  useEffect(() => {
    const cancel = probe();
    const unsubscribe = subscribeToken(() => probe());
    return () => {
      cancel();
      unsubscribe();
    };
  }, [probe]);

  useEffect(() => {
    if (!loading && !user && !PUBLIC_PATHS.has(pathname)) {
      redirectToSignIn(pathname);
    }
  }, [loading, user, pathname]);

  return (
    <SessionContext.Provider value={{ user, loading, refresh }}>
      {children}
    </SessionContext.Provider>
  );
}
