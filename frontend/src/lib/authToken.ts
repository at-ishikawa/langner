// In-memory langner access-token store.
//
// The web SPA holds ONLY the 24h access token, and ONLY in memory — never in a
// cookie and never in localStorage. It is delivered by the OAuth callback in
// the URL fragment (see src/app/auth/callback/page.tsx) and, on expiry or a
// hard reload, re-acquired by a silent redirect through Google. Because the
// token lives in module memory it is lost on a full page load, which is exactly
// the trigger for that silent re-auth.
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

// setAccessToken replaces the in-memory token and notifies subscribers.
export function setAccessToken(token: string | null): void {
  accessToken = token;
  emit();
}

// clearAccessToken drops the in-memory token (web logout).
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
