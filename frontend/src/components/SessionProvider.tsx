"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { usePathname, useRouter } from "next/navigation";
import { getMe, type AuthUser } from "@/lib/auth";

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

// useSession exposes the current authenticated user (or null), whether the
// initial /auth/me probe is still in flight, and a refresh() to re-probe.
export function useSession(): SessionState {
  return useContext(SessionContext);
}

// SessionProvider probes /auth/me on mount and guards the app: once the probe
// resolves with no user, every route except /login redirects to /login. The
// cookie is HTTP-only so this guard is client-side only (no middleware.ts).
export function SessionProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [loading, setLoading] = useState(true);
  const router = useRouter();
  const pathname = usePathname();

  const refresh = useCallback(async () => {
    const u = await getMe();
    setUser(u);
  }, []);

  useEffect(() => {
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

  useEffect(() => {
    if (!loading && !user && pathname !== "/login") {
      router.replace("/login");
    }
  }, [loading, user, pathname, router]);

  return (
    <SessionContext.Provider value={{ user, loading, refresh }}>
      {children}
    </SessionContext.Provider>
  );
}
