// Thin client for the backend's plain-HTTP auth endpoints. The session lives
// in an HTTP-only cookie the browser cannot read, so authentication state is
// discovered by calling /auth/me (never by inspecting the cookie).

export const API_BASE =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export interface AuthUser {
  email: string;
  name: string;
}

interface MeResponse {
  authenticated: boolean;
  email?: string;
  name?: string;
}

// getMe returns the signed-in user, or null when unauthenticated (401).
export async function getMe(): Promise<AuthUser | null> {
  let res: Response;
  try {
    res = await fetch(`${API_BASE}/auth/me`, { credentials: "include" });
  } catch {
    return null;
  }
  if (!res.ok) {
    return null;
  }
  const data = (await res.json()) as MeResponse;
  if (!data.authenticated || !data.email) {
    return null;
  }
  return { email: data.email, name: data.name ?? "" };
}

// login starts the Google OAuth flow by navigating to the backend, which
// redirects to Google's consent screen.
export function login(): void {
  window.location.href = `${API_BASE}/auth/google/login`;
}

// logout clears the session cookie on the backend, then returns to /login.
export async function logout(): Promise<void> {
  try {
    await fetch(`${API_BASE}/auth/logout`, {
      method: "POST",
      credentials: "include",
    });
  } finally {
    window.location.href = "/login";
  }
}
