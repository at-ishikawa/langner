// In-memory langner access-token store.
//
// The SPA holds ONLY the short-lived (24h) access token, and ONLY in memory —
// never in a cookie and never in localStorage. The long-lived REFRESH token is
// kept by the browser in an HttpOnly cookie that JavaScript cannot read (see
// src/lib/refresh.ts), so an XSS bug cannot exfiltrate a durable credential.
// On a full page load the in-memory access token is gone, which triggers a
// silent refresh (the cookie rides along) to mint a new one — keeping the user
// signed in for weeks without any token being readable by scripts.
//
// e2e seam: the harness injects `window.__LANGNER_ACCESS_TOKEN__` before app
// scripts run (Playwright addInitScript) so every full page load re-seeds the
// store without a real Google round-trip. In production that global is never
// set, so this is inert.

let accessToken: string | null = null;
const listeners = new Set<() => void>();

interface InjectedWindow {
  __LANGNER_ACCESS_TOKEN__?: string;
}

// readInjectedToken returns a token pre-injected on `window` (e2e only), or null.
function readInjectedToken(): string | null {
  if (typeof window === "undefined") return null;
  const injected = (window as InjectedWindow).__LANGNER_ACCESS_TOKEN__;
  return injected && injected !== "" ? injected : null;
}

// getAccessToken returns the current in-memory access token, falling back to a
// pre-injected token (e2e) on first read after a page load.
export function getAccessToken(): string | null {
  if (accessToken === null) {
    accessToken = readInjectedToken();
  }
  return accessToken;
}

// setAccessToken replaces the in-memory token and notifies subscribers. Called
// with the fragment token at sign-in and with the refreshed token on each
// silent refresh.
export function setAccessToken(token: string | null): void {
  accessToken = token;
  emit();
}

// clearAccessToken drops the in-memory token (web logout / unrecoverable refresh).
export function clearAccessToken(): void {
  accessToken = null;
  emit();
}

// subscribeToken registers a listener for token changes (for useSyncExternalStore).
export function subscribeToken(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function emit(): void {
  for (const listener of listeners) listener();
}
