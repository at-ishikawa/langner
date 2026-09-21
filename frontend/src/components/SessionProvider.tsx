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
}

const SessionContext = createContext<SessionState>({
  user: null,
  loading: true,
});

// Routes that must render WITHOUT an authenticated session: the sign-in page
// and the OAuth callback landing (which is mid-way through acquiring a token).
const PUBLIC_PATHS = new Set(["/login", "/auth/callback"]);

// useSession exposes the current authenticated user (or null) and whether the
// initial /auth/me probe is still in flight.
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

  const refresh = useCallback(() => {
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

  // Probe on mount and whenever the in-memory token changes (e.g. the OAuth
  // callback stores a freshly minted token).
  useEffect(() => {
    const cancel = refresh();
    const unsubscribe = subscribeToken(() => refresh());
    return () => {
      cancel();
      unsubscribe();
    };
  }, [refresh]);

  useEffect(() => {
    if (!loading && !user && !PUBLIC_PATHS.has(pathname)) {
      redirectToSignIn(pathname);
    }
  }, [loading, user, pathname]);

  return (
    <SessionContext.Provider value={{ user, loading }}>
      {children}
    </SessionContext.Provider>
  );
}
