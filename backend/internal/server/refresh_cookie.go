package server

import (
	"net/http"
	"time"
)

// refreshCookieName is the HttpOnly cookie that carries the WEB client's refresh
// token. The browser sends it automatically to the auth endpoints; JavaScript
// can never read it (unlike a localStorage token), so an XSS bug cannot
// exfiltrate it. The CLI does not use this cookie — it carries its refresh token
// in the request body — so the refresh/revoke handlers accept either carrier.
const refreshCookieName = "langner_refresh"

// refreshCookiePath scopes the cookie to the auth endpoints that consume it
// (/auth/token/refresh, /auth/token/revoke). Only those paths are reverse-
// proxied onto the SPA's own origin, so the cookie stays first-party there and
// is never attached to ordinary API/RPC calls.
const refreshCookiePath = "/auth/token"

// setRefreshCookie writes the rotating refresh token as an HttpOnly cookie.
// secure/sameSite mirror the OAuth state cookie's config. maxAge bounds the
// cookie to the refresh token's own lifetime.
func setRefreshCookie(w http.ResponseWriter, token string, secure bool, sameSite http.SameSite, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     refreshCookiePath,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		MaxAge:   int(maxAge.Seconds()),
	})
}

// clearRefreshCookie expires the refresh cookie (logout). The attributes (Path,
// Secure, SameSite) must match the ones it was set with for the browser to
// replace it.
func clearRefreshCookie(w http.ResponseWriter, secure bool, sameSite http.SameSite) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		MaxAge:   -1,
	})
}

// readRefreshCookie returns the refresh token carried in the cookie, or "" when
// the request has none (a CLI call, which carries it in the body instead).
func readRefreshCookie(r *http.Request) string {
	c, err := r.Cookie(refreshCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}
