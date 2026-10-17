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
// On a Vercel PREVIEW, the backend is this PR's own backend preview deployment,
// whose URL isn't known ahead of time. Both projects share the same branch slug
// and account scope, so derive the backend's branch URL from THIS (frontend)
// deployment's VERCEL_BRANCH_URL by swapping the project-name prefix:
//   langner-git-<slug>-at-ishikawa.vercel.app → langner-api-git-<slug>-...
// Used when NEXT_PUBLIC_API_BASE_URL / AUTH_PROXY_TARGET aren't set explicitly.
// (Very long/odd branch names can get truncated+hashed by Vercel, breaking the
// swap — keep preview branch names short, or set the env vars explicitly.)
function derivePreviewBackendURL(): string | undefined {
  if (process.env.VERCEL_ENV !== "preview") return undefined;
  const branchURL = process.env.VERCEL_BRANCH_URL; // host only, no scheme
  if (!branchURL || !branchURL.startsWith("langner-git-")) return undefined;
  return `https://${branchURL.replace(/^langner-git-/, "langner-api-git-")}`;
}

const previewBackendURL = derivePreviewBackendURL();
// On a Vercel PREVIEW the derived sibling-backend URL WINS over any
// project-level NEXT_PUBLIC_API_BASE_URL / AUTH_PROXY_TARGET — those point at
// production and are applied to every environment, so without this a preview
// frontend would call the prod backend (cross-origin /auth/me → CORS blocked →
// bounced to sign-in). Off preview (previewBackendURL undefined) the explicit
// env vars are used, then the localhost default.
const apiBaseUrl = previewBackendURL ?? process.env.NEXT_PUBLIC_API_BASE_URL;
const authProxyTarget =
  previewBackendURL ?? process.env.AUTH_PROXY_TARGET ?? "http://localhost:8080";
const apiHost = apiBaseUrl ? new URL(apiBaseUrl).hostname : undefined;

const nextConfig: NextConfig = {
  // Inject the (possibly derived) API base into the client bundle so the browser
  // calls this PR's backend preview. Cross-origin RPCs rely on the backend's
  // Preview-scope CORS allowing the frontend preview origin (glob in serverapp).
  env: apiBaseUrl ? { NEXT_PUBLIC_API_BASE_URL: apiBaseUrl } : {},
  allowedDevOrigins: apiHost ? [apiHost] : [],
  async rewrites() {
    // Proxy ONLY the cookie-bearing WEB auth endpoints onto this origin: the
    // Google sign-in/callback (so the OAuth state + refresh cookies are
    // first-party) and the token refresh/revoke endpoints the SPA calls.
    // Everything else (the quiz RPCs, /auth/me) talks to the API directly with
    // the bearer token. The CLI device-login flow is NOT proxied: its approval
    // page is a FRONTEND route (/auth/device) whose form submits straight to the
    // API (Google sign-in + deny), so it needs no cookie and no first-party
    // origin here.
    return [
      { source: "/auth/google/:path*", destination: `${authProxyTarget}/auth/google/:path*` },
      { source: "/auth/token/:path*", destination: `${authProxyTarget}/auth/token/:path*` },
    ];
  },
};

export default nextConfig;
