import type { NextConfig } from "next";

// AUTH_PROXY_TARGET is the backend origin the auth endpoints are reverse-proxied
// to so they share the SPA's own origin — which is what lets the refresh token
// live in a first-party HttpOnly cookie (see src/lib/refresh.ts).
//
// This is a SERVER-SIDE hop (the Next server calls the backend), so it must be an
// address the Next PROCESS can reach — NOT the browser-facing NEXT_PUBLIC_API_BASE_URL,
// which in split setups (e.g. WSL, where the browser uses a host-only name the
// Next server can't route to) would time out. It therefore defaults to a
// locally-reachable backend and is overridden with AUTH_PROXY_TARGET when the
// backend lives elsewhere but is still reachable from the Next server (prod/Vercel).
const apiBaseUrl = process.env.NEXT_PUBLIC_API_BASE_URL;
const authProxyTarget = process.env.AUTH_PROXY_TARGET ?? "http://localhost:8080";
const apiHost = apiBaseUrl ? new URL(apiBaseUrl).hostname : undefined;

const nextConfig: NextConfig = {
  allowedDevOrigins: apiHost ? [apiHost] : [],
  async rewrites() {
    // Proxy ONLY the cookie-bearing auth endpoints onto this origin: the Google
    // sign-in/callback (so the OAuth state + refresh cookies are first-party) and
    // the token refresh/revoke endpoints the SPA calls. Everything else (the quiz
    // RPCs, /auth/me) keeps talking to the API directly with the bearer token.
    return [
      { source: "/auth/google/:path*", destination: `${authProxyTarget}/auth/google/:path*` },
      { source: "/auth/token/:path*", destination: `${authProxyTarget}/auth/token/:path*` },
      { source: "/auth/device/:path*", destination: `${authProxyTarget}/auth/device/:path*` },
    ];
  },
};

export default nextConfig;
