package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/at-ishikawa/langner/internal/auth"
)

const (
	// SessionCookieName is the signed, HTTP-only session cookie.
	SessionCookieName = "langner_session"
	// stateCookieName holds the short-lived signed OAuth CSRF state.
	stateCookieName = "langner_oauth_state"

	sessionTTL = 30 * 24 * time.Hour
	stateTTL   = 10 * time.Minute
)

// AuthHandler serves the plain-HTTP endpoints of the Google-OAuth flow. These
// are registered on the mux OUTSIDE the connect interceptor so an
// unauthenticated user can reach them to sign in.
type AuthHandler struct {
	authenticator  auth.Authenticator
	sessions       *auth.SessionSigner
	state          *auth.StateSigner
	users          *auth.UserRepository
	allowedEmails  []string
	frontendURL    string
	cookieSecure   bool
	cookieSameSite http.SameSite
}

// AuthHandlerConfig wires an AuthHandler.
type AuthHandlerConfig struct {
	Authenticator  auth.Authenticator
	Sessions       *auth.SessionSigner
	State          *auth.StateSigner
	Users          *auth.UserRepository
	AllowedEmails  []string
	FrontendURL    string
	CookieSecure   bool
	CookieSameSite http.SameSite
}

// NewAuthHandler builds an AuthHandler.
func NewAuthHandler(cfg AuthHandlerConfig) *AuthHandler {
	return &AuthHandler{
		authenticator:  cfg.Authenticator,
		sessions:       cfg.Sessions,
		state:          cfg.State,
		users:          cfg.Users,
		allowedEmails:  cfg.AllowedEmails,
		frontendURL:    cfg.FrontendURL,
		cookieSecure:   cfg.CookieSecure,
		cookieSameSite: cfg.CookieSameSite,
	}
}

// Login mints a signed CSRF state, stores it in a temporary cookie, and
// redirects the browser to Google's consent screen.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	state, err := h.state.Issue(stateTTL)
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
// fetch the profile, enforce the allowlist, upsert the user, set the session
// cookie, and redirect to the frontend. A non-allowlisted email is bounced to
// the login page with no cookie and no user row.
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stateCookie, err := r.Cookie(stateCookieName)
	stateParam := r.URL.Query().Get("state")
	// Always clear the state cookie once we've read it.
	h.clearCookie(w, stateCookieName)
	if err != nil || stateParam == "" || stateParam != stateCookie.Value || h.state.Verify(stateParam) != nil {
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

	user, err := h.users.Upsert(ctx, info.Sub, info.Email, info.Name)
	if err != nil {
		slog.Error("auth: upsert user failed", "error", err)
		http.Redirect(w, r, h.frontendURL+"/login?error=server_error", http.StatusFound)
		return
	}

	expiresAt := time.Now().Add(sessionTTL)
	value, err := h.sessions.Sign(auth.Session{UserID: user.ID, ExpiresAt: expiresAt})
	if err != nil {
		slog.Error("auth: sign session failed", "error", err)
		http.Redirect(w, r, h.frontendURL+"/login?error=server_error", http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: h.cookieSameSite,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	http.Redirect(w, r, h.frontendURL, http.StatusFound)
}

// Logout clears the session cookie.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.clearCookie(w, SessionCookieName)
	w.WriteHeader(http.StatusNoContent)
}

// meResponse is the JSON shape of /auth/me.
type meResponse struct {
	Authenticated bool   `json:"authenticated"`
	Email         string `json:"email,omitempty"`
	Name          string `json:"name,omitempty"`
}

// Me returns the signed-in user's email and name from the verified session
// (populated in the request context by the auth cookie middleware). It responds
// 401 when there is no valid session.
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
	_ = json.NewEncoder(w).Encode(meResponse{Authenticated: true, Email: user.Email, Name: user.Name})
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
