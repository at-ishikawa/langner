// Thin client for the backend's plain-HTTP auth endpoints.
//
// The access token is a bearer held in memory (see authToken.ts) and attached
// as `Authorization: Bearer <token>` to API calls; auth state is discovered via
// /auth/me. The long-lived refresh token is an HttpOnly cookie the browser
// sends automatically to the same-origin (reverse-proxied) /auth/token/*
// endpoints — JS never sees it.

import { getAccessToken, clearAccessToken } from "./authToken";
import { tryRefreshSession } from "./refresh";

export const API_BASE =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export interface AuthUser {
  username: string;
}

interface MeResponse {
  authenticated: boolean;
  username?: string;
}

// getMe returns the signed-in user, or null when unauthenticated. It restores a
// persisted session across reloads: with no in-memory access token (a fresh
// load) — or on a 401 — it attempts one silent refresh (the HttpOnly cookie
// rides along) and retries, so a reload keeps the user signed in without a
// Google round-trip. No email/name is exposed — only the auto-generated username.
export async function getMe(): Promise<AuthUser | null> {
  let token = getAccessToken();
  if (!token) {
    if (await tryRefreshSession()) token = getAccessToken();
    if (!token) return null;
  }

  const probe = (t: string): Promise<Response> =>
    fetch(`${API_BASE}/auth/me`, { headers: { Authorization: `Bearer ${t}` } });

  let res: Response;
  try {
    res = await probe(token);
    if (res.status === 401 && (await tryRefreshSession())) {
      const refreshed = getAccessToken();
      if (refreshed) res = await probe(refreshed);
    }
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

// redirectToSignIn sends the browser to Google sign-in, folding the current
// location into `next` so the OAuth callback returns the user here. The path is
// SAME-ORIGIN (reverse-proxied to the backend) so the OAuth state cookie and the
// refresh cookie the callback sets are first-party to the app. When the Google
// session is still alive this round-trips with no user interaction (silent SSO).
export function redirectToSignIn(next?: string): void {
  if (typeof window === "undefined") return;
  const target = next ?? window.location.href;
  window.location.href = `/auth/google/login?next=${encodeURIComponent(target)}`;
}

// login starts the Google OAuth flow (the explicit "Sign in" button).
export function login(next?: string): void {
  redirectToSignIn(next ?? "/");
}

// logout revokes the refresh token server-side (the cookie is sent via
// credentials:"include") so a leaked copy can't be used after sign-out, drops
// the in-memory access token, and returns to /login.
export function logout(): void {
  if (typeof window !== "undefined") {
    // Fire-and-forget; keepalive lets it complete during navigation.
    void fetch("/auth/token/revoke", {
      method: "POST",
      credentials: "include",
      keepalive: true,
    }).catch(() => {});
  }
  clearAccessToken();
  if (typeof window !== "undefined") {
    window.location.href = "/login";
  }
}
