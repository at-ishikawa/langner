// Thin client for the backend's plain-HTTP auth endpoints. Auth is a bearer
// access token held in memory (see authToken.ts) — there is no cookie — so
// every call attaches `Authorization: Bearer <token>` and authentication state
// is discovered by calling /auth/me.

import { getAccessToken, clearAccessToken } from "./authToken";

export const API_BASE =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export interface AuthUser {
  username: string;
}

interface MeResponse {
  authenticated: boolean;
  username?: string;
}

// getMe returns the signed-in user, or null when unauthenticated (no/expired
// token → 401). No email/name is exposed — only the auto-generated username.
export async function getMe(): Promise<AuthUser | null> {
  const token = getAccessToken();
  if (!token) {
    return null;
  }
  let res: Response;
  try {
    res = await fetch(`${API_BASE}/auth/me`, {
      headers: { Authorization: `Bearer ${token}` },
    });
  } catch {
    return null;
  }
  if (!res.ok) {
    return null;
  }
  const data = (await res.json()) as MeResponse;
  if (!data.authenticated || !data.username) {
    return null;
  }
  return { username: data.username };
}

// redirectToSignIn sends the browser to the backend's Google sign-in, folding
// the current location into `next` so the OAuth callback can return the user
// here after minting a fresh access token. When the Google session is still
// alive this round-trips with no user interaction (silent SSO).
export function redirectToSignIn(next?: string): void {
  if (typeof window === "undefined") return;
  const target = next ?? window.location.href;
  window.location.href = `${API_BASE}/auth/google/login?next=${encodeURIComponent(target)}`;
}

// login starts the Google OAuth flow (the explicit "Sign in" button).
export function login(next?: string): void {
  redirectToSignIn(next ?? "/");
}

// logout drops the in-memory access token and returns to /login. There is no
// web refresh token or server session to revoke — the access token simply
// stops being sent and expires on its own.
export function logout(): void {
  clearAccessToken();
  if (typeof window !== "undefined") {
    window.location.href = "/login";
  }
}
