// Silent access-token refresh for the web session.
//
// The access token is short-lived (24h) and in-memory; the refresh token lives
// for weeks in an HttpOnly cookie the browser sends automatically. When a
// request 401s (or on a fresh page load with no in-memory access token), we POST
// to /auth/token/refresh — the cookie rides along via `credentials:"include"`,
// so JS never reads or holds the refresh token — and the server returns a new
// access token (and rotates the cookie). The refresh endpoint is same-origin
// (reverse-proxied onto the app's own origin) so the HttpOnly cookie is sent.
//
// Rotation-on-use means the server revokes the presented refresh token on each
// use; concurrent 401s must therefore NOT each fire their own refresh (that
// would look like a replay and revoke the family). We serialize behind a single
// in-flight promise: the first caller refreshes, everyone else awaits it.

import { setAccessToken, clearAccessToken } from "./authToken";

interface RefreshResponse {
  access_token: string;
}

let inFlight: Promise<boolean> | null = null;

// tryRefreshSession attempts one refresh, returning true if a new access token
// was obtained. Concurrent callers share the same attempt.
export function tryRefreshSession(): Promise<boolean> {
  if (inFlight) return inFlight;
  inFlight = doRefresh().finally(() => {
    inFlight = null;
  });
  return inFlight;
}

async function doRefresh(): Promise<boolean> {
  let res: Response;
  try {
    // Same-origin + credentials so the HttpOnly refresh cookie is attached; the
    // refresh token is never read or sent by JS.
    res = await fetch("/auth/token/refresh", {
      method: "POST",
      credentials: "include",
    });
  } catch {
    return false; // network error — leave state as-is, caller can retry later
  }
  if (!res.ok) {
    // No/expired/invalid refresh cookie — drop the in-memory access token so the
    // app falls back to sign-in.
    clearAccessToken();
    return false;
  }
  const data = (await res.json()) as RefreshResponse;
  if (!data.access_token) {
    clearAccessToken();
    return false;
  }
  setAccessToken(data.access_token);
  return true;
}
