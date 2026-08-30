"use client";

import {
  createContext,
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
}

const SessionContext = createContext<SessionState>({
  user: null,
  loading: true,
});

// useSession exposes the current authenticated user (or null) and whether the
// initial /auth/me probe is still in flight.
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
    <SessionContext.Provider value={{ user, loading }}>
      {children}
    </SessionContext.Provider>
  );
}
