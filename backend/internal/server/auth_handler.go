package server

import (
	"context"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/auth"
)

const (
	// stateCookieName holds the short-lived signed OAuth CSRF state. This is the
	// ONLY browser cookie langner uses — it is same-site to the API host during
	// the Google redirect and is NOT an app session (the session cookie is gone;
	// the SPA authenticates with a bearer access token held in memory).
	stateCookieName = "langner_oauth_state"

	stateTTL = 10 * time.Minute
)

// AuthHandler serves the plain-HTTP endpoints of the Google-OAuth flow. These
// are registered on the mux OUTSIDE the connect interceptor so an
// unauthenticated user can reach them to sign in. On a successful web sign-in it
// mints a langner access JWT and delivers it to the SPA in the URL fragment
// (no session cookie, no refresh token for the web). When the signed OAuth
// state carries a device user_code (the CLI device flow), the callback approves
// that device row from the just-verified identity instead (M3).
type AuthHandler struct {
	authenticator  auth.Authenticator
	tokens         *auth.TokenSigner
	state          *auth.StateSigner
	users          *auth.UserRepository
	deviceCodes    *auth.CLIDeviceCodeRepository
	allowedEmails  []string
	frontendURL    string
	allowedOrigins []string
	cookieSecure   bool
	cookieSameSite http.SameSite
	now            func() time.Time
}

// AuthHandlerConfig wires an AuthHandler.
type AuthHandlerConfig struct {
	Authenticator auth.Authenticator
	Tokens        *auth.TokenSigner
	State         *auth.StateSigner
	Users         *auth.UserRepository
	// DeviceCodes approves the CLI device row inside the callback (M3). May be
	// nil in tests that never exercise the device path.
	DeviceCodes   *auth.CLIDeviceCodeRepository
	AllowedEmails []string
	FrontendURL   string
	// AllowedOrigins is the set of frontend origins a post-sign-in redirect may
	// target (the open-redirect allowlist for `next`); the configured
	// FrontendURL origin is always allowed.
	AllowedOrigins []string
	CookieSecure   bool
	CookieSameSite http.SameSite
	// Now is optional; defaults to time.Now. Tests pin it.
	Now func() time.Time
}

// NewAuthHandler builds an AuthHandler.
func NewAuthHandler(cfg AuthHandlerConfig) *AuthHandler {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &AuthHandler{
		authenticator:  cfg.Authenticator,
		tokens:         cfg.Tokens,
		state:          cfg.State,
		users:          cfg.Users,
		deviceCodes:    cfg.DeviceCodes,
		allowedEmails:  cfg.AllowedEmails,
		frontendURL:    cfg.FrontendURL,
		allowedOrigins: cfg.AllowedOrigins,
		cookieSecure:   cfg.CookieSecure,
		cookieSameSite: cfg.CookieSameSite,
		now:            now,
	}
}

// Login mints a signed CSRF state — folding the SPA return target (`next`) and
// the optional device `user_code` INTO the signed state so they survive the
// Google round-trip tamper-proof — stores it in a temporary cookie, and
// redirects the browser to Google's consent screen.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	next := r.URL.Query().Get("next")
	userCode := auth.NormalizeUserCode(r.URL.Query().Get("user_code"))
	state, err := h.state.Issue(stateTTL, auth.StateClaims{Next: next, UserCode: userCode})
	if err != nil {
		slog.Error("auth: issue state failed", "error", err)
		http.Redirect(w, r, h.frontendURL+"/login?error=server_error", http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: h.cookieSameSite,
		MaxAge:   int(stateTTL.Seconds()),
	})
	http.Redirect(w, r, h.authenticator.AuthCodeURL(state), http.StatusFound)
}

// Callback completes the OAuth round-trip: verify state, exchange the code,
// fetch the profile, enforce the allowlist, upsert the user — then either
// approve the CLI device row (when the signed state carries a user_code) or
// mint a langner access JWT and hand it to the SPA in the URL fragment. It sets
// NO session cookie. A non-allowlisted email is bounced to the login page.
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stateCookie, err := r.Cookie(stateCookieName)
	stateParam := r.URL.Query().Get("state")
	// Always clear the state cookie once we've read it.
	h.clearCookie(w, stateCookieName)
	if err != nil || stateParam == "" || stateParam != stateCookie.Value {
		http.Redirect(w, r, h.frontendURL+"/login?error=invalid_state", http.StatusFound)
		return
	}
	claims, err := h.state.Verify(stateParam)
	if err != nil {
		http.Redirect(w, r, h.frontendURL+"/login?error=invalid_state", http.StatusFound)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, h.frontendURL+"/login?error=missing_code", http.StatusFound)
		return
	}

	token, err := h.authenticator.Exchange(ctx, code)
	if err != nil {
		slog.Error("auth: token exchange failed", "error", err)
		http.Redirect(w, r, h.frontendURL+"/login?error=exchange_failed", http.StatusFound)
		return
	}

	info, err := h.authenticator.FetchUserInfo(ctx, token)
	if err != nil {
		slog.Error("auth: fetch userinfo failed", "error", err)
		http.Redirect(w, r, h.frontendURL+"/login?error=userinfo_failed", http.StatusFound)
		return
	}

	// Allowlist uses the plaintext email straight from Google — never a stored
	// value — so encryption-at-rest never affects who may sign in.
	if !auth.IsAllowed(info.Email, h.allowedEmails) {
		http.Redirect(w, r, h.frontendURL+"/login?error=not_allowed", http.StatusFound)
		return
	}

	user, err := h.users.Upsert(ctx, info.Sub)
	if err != nil {
		slog.Error("auth: upsert user failed", "error", err)
		http.Redirect(w, r, h.frontendURL+"/login?error=server_error", http.StatusFound)
		return
	}

	// Device approval (M3): a user_code in the signed state means this sign-in
	// is approving a CLI device. Bind the just-verified identity to the device
	// row IN THIS REQUEST — there is no session to carry it to a later POST.
	if claims.UserCode != "" {
		h.approveDevice(ctx, claims.UserCode, user.ID)
		h.renderMessage(w, "Approved. You can return to your terminal.")
		return
	}

	// Web sign-in: mint the access JWT and hand it to the SPA via the URL
	// fragment (never a query param — a fragment is not sent to a server). No
	// refresh token for the web and no session cookie.
	access, err := h.tokens.Sign(user.ID, h.now())
	if err != nil {
		slog.Error("auth: sign access token failed", "error", err)
		http.Redirect(w, r, h.frontendURL+"/login?error=server_error", http.StatusFound)
		return
	}
	http.Redirect(w, r, h.callbackRedirect(claims.Next, access), http.StatusFound)
}

// approveDevice binds the just-verified identity to a still-pending, unexpired
// device row for user_code. It is best-effort and silent: any failure (bad,
// expired, or already-consumed code) leaves the row untouched and the CLI keeps
// polling `authorization_pending` — no enumeration signal is rendered.
func (h *AuthHandler) approveDevice(ctx context.Context, userCode string, userID int64) {
	if h.deviceCodes == nil {
		return
	}
	uc := auth.NormalizeUserCode(userCode)
	row, err := h.deviceCodes.FindByUserCode(ctx, uc)
	if err != nil || row.Status != auth.DeviceStatusPending || !h.now().Before(row.ExpiresAt) {
		return
	}
	if err := h.deviceCodes.Approve(ctx, row.ID, userID, h.now()); err != nil {
		slog.Error("auth: approve device failed", "error", err)
	}
}

// callbackRedirect builds the SPA fragment-delivery URL:
// <allowed-origin>/auth/callback#access_token=<jwt>[&next=<path>]. The origin is
// allowlisted against the configured frontend origins (open-redirect guard);
// `next`'s path is preserved so the SPA can return the user where they were.
func (h *AuthHandler) callbackRedirect(next, access string) string {
	base := h.frontendOrigin()
	nextPath := "/"
	if next != "" {
		if u, err := url.Parse(next); err == nil {
			if u.Scheme != "" || u.Host != "" {
				// Absolute next: only honor it when its origin is allowlisted.
				if origin := u.Scheme + "://" + u.Host; h.originAllowed(origin) {
					base = origin
					nextPath = pathWithQuery(u)
				}
			} else if strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") {
				// Relative next path on the frontend origin.
				nextPath = next
			}
		}
	}
	frag := "access_token=" + access
	if nextPath != "" && nextPath != "/" {
		frag += "&next=" + url.QueryEscape(nextPath)
	}
	return base + "/auth/callback#" + frag
}

// pathWithQuery returns the path (+ raw query) of a parsed URL, guaranteed to
// start with "/".
func pathWithQuery(u *url.URL) string {
	p := u.EscapedPath()
	if p == "" {
		p = "/"
	}
	if u.RawQuery != "" {
		p += "?" + u.RawQuery
	}
	return p
}

// frontendOrigin returns scheme://host of the configured frontend URL, or the
// raw frontend URL if it cannot be parsed as an origin.
func (h *AuthHandler) frontendOrigin() string {
	if u, err := url.Parse(h.frontendURL); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	return h.frontendURL
}

// originAllowed reports whether origin is the configured frontend origin or one
// of the configured allowed origins. A "*" CORS wildcard is deliberately NOT
// honored for redirects (that would be an open redirect).
func (h *AuthHandler) originAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	if origin == h.frontendOrigin() {
		return true
	}
	for _, o := range h.allowedOrigins {
		if o != "*" && o == origin {
			return true
		}
	}
	return false
}

// Logout is a no-op for the web: there is no server-side session to clear (the
// SPA drops its in-memory access token client-side). The CLI logs out via
// /auth/token/revoke instead.
func (h *AuthHandler) Logout(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// meResponse is the JSON shape of /auth/me. Only the non-PII username is
// exposed — no email/name is stored or returned.
type meResponse struct {
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username,omitempty"`
}

// Me returns the signed-in user's username from the verified session
// (populated in the request context by AuthBearerMiddleware). It responds 401
// when there is no valid bearer.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	sess, ok := auth.SessionFromContext(r.Context())
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(meResponse{Authenticated: false})
		return
	}
	user, err := h.users.FindByID(r.Context(), sess.UserID)
	if err != nil {
		slog.Error("auth: find user for /auth/me failed", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(meResponse{Authenticated: false})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(meResponse{Authenticated: true, Username: user.Username})
}

// renderMessage writes a minimal HTML page (used for the device-approval
// result).
func (h *AuthHandler) renderMessage(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><body style="font-family:sans-serif;max-width:32rem;margin:4rem auto;"><p>` +
		template.HTMLEscapeString(msg) + `</p></body></html>`))
}

func (h *AuthHandler) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: h.cookieSameSite,
		MaxAge:   -1,
	})
}
