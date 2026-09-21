package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/at-ishikawa/langner/internal/auth"
)

// fakeAuthenticator stands in for Google so the callback's state / allowlist /
// redirect logic is exercised without real credentials or network access.
type fakeAuthenticator struct {
	info auth.UserInfo
}

func (f fakeAuthenticator) AuthCodeURL(state string) string {
	return "https://accounts.google.example/o/oauth2/v2/auth?state=" + url.QueryEscape(state)
}

func (f fakeAuthenticator) Exchange(_ context.Context, _ string) (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: "fake-access-token"}, nil
}

func (f fakeAuthenticator) FetchUserInfo(_ context.Context, _ *oauth2.Token) (auth.UserInfo, error) {
	return f.info, nil
}

func newTestAuthHandler(t *testing.T, allowed []string, info auth.UserInfo) *AuthHandler {
	t.Helper()
	tokens, err := auth.NewTokenSigner([]byte("handler-test-token-key"))
	require.NoError(t, err)
	state, err := auth.NewStateSigner([]byte("handler-test-token-key"))
	require.NoError(t, err)
	return NewAuthHandler(AuthHandlerConfig{
		Authenticator: fakeAuthenticator{info: info},
		Tokens:        tokens,
		State:         state,
		// Users is intentionally nil: the allowlist-reject / invalid-state paths
		// must return BEFORE any user upsert, so touching the DB here would panic.
		Users:          nil,
		AllowedEmails:  allowed,
		FrontendURL:    "http://frontend.example",
		AllowedOrigins: []string{"http://frontend.example"},
		CookieSecure:   false,
		CookieSameSite: http.SameSiteLaxMode,
	})
}

// noSessionCookie asserts the response set NO auth session cookie (there is no
// session cookie anymore — only the short-lived OAuth state cookie may appear).
func noSessionCookie(rec *httptest.ResponseRecorder) bool {
	for _, c := range rec.Result().Cookies() {
		if c.Name != stateCookieName && c.Value != "" {
			return false
		}
	}
	return true
}

func TestAuthHandler_Callback_AllowlistReject(t *testing.T) {
	h := newTestAuthHandler(t, []string{"admin@example.com"},
		auth.UserInfo{Sub: "google-sub-123", Email: "intruder@example.com", Name: "Intruder"})

	// A valid, matching state so the flow reaches the allowlist check.
	stateVal, err := h.state.Issue(stateTTL, auth.StateClaims{})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet,
		"/auth/google/callback?state="+url.QueryEscape(stateVal)+"&code=any-code", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: stateVal})
	rec := httptest.NewRecorder()

	h.Callback(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "http://frontend.example/login?error=not_allowed", rec.Header().Get("Location"))
	assert.True(t, noSessionCookie(rec), "no session cookie must be set for a rejected email")
	assert.NotContains(t, rec.Header().Get("Location"), "access_token", "no token may leak for a rejected email")
}

func TestAuthHandler_Callback_InvalidState(t *testing.T) {
	h := newTestAuthHandler(t, []string{"admin@example.com"},
		auth.UserInfo{Sub: "s", Email: "admin@example.com", Name: "Admin"})

	// State cookie present but query state does not match.
	stateVal, err := h.state.Issue(stateTTL, auth.StateClaims{})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet,
		"/auth/google/callback?state=mismatched&code=any-code", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: stateVal})
	rec := httptest.NewRecorder()

	h.Callback(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "http://frontend.example/login?error=invalid_state", rec.Header().Get("Location"))
	assert.True(t, noSessionCookie(rec))
}

func TestAuthHandler_Me_Unauthenticated(t *testing.T) {
	h := newTestAuthHandler(t, nil, auth.UserInfo{})
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()

	h.Me(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthHandler_Logout_NoOp(t *testing.T) {
	h := newTestAuthHandler(t, nil, auth.UserInfo{})
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	rec := httptest.NewRecorder()

	h.Logout(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.True(t, noSessionCookie(rec), "web logout clears no server cookie")
}

// TestAuthHandler_CallbackRedirect_OpenRedirectGuard pins the allowlist on the
// post-sign-in `next` target: an allowlisted origin is honored (fragment token
// + preserved path), a foreign origin falls back to the configured frontend
// origin, and the token is always delivered in the fragment (never the query).
func TestAuthHandler_CallbackRedirect_OpenRedirectGuard(t *testing.T) {
	h := newTestAuthHandler(t, nil, auth.UserInfo{})

	t.Run("allowlisted absolute next preserves origin + path", func(t *testing.T) {
		got := h.callbackRedirect("http://frontend.example/quiz/standard?x=1", "JWT")
		assert.True(t, strings.HasPrefix(got, "http://frontend.example/auth/callback#access_token=JWT"), got)
		assert.Contains(t, got, "next=")
		assert.Contains(t, got, url.QueryEscape("/quiz/standard?x=1"))
	})

	t.Run("foreign origin falls back to the frontend origin", func(t *testing.T) {
		got := h.callbackRedirect("http://evil.example/steal", "JWT")
		assert.Equal(t, "http://frontend.example/auth/callback#access_token=JWT", got)
	})

	t.Run("relative next stays on the frontend origin", func(t *testing.T) {
		got := h.callbackRedirect("/learn", "JWT")
		assert.True(t, strings.HasPrefix(got, "http://frontend.example/auth/callback#access_token=JWT"), got)
		assert.Contains(t, got, "next="+url.QueryEscape("/learn"))
	})

	t.Run("protocol-relative next is rejected", func(t *testing.T) {
		got := h.callbackRedirect("//evil.example/x", "JWT")
		assert.Equal(t, "http://frontend.example/auth/callback#access_token=JWT", got)
	})

	t.Run("token is never placed in the query", func(t *testing.T) {
		got := h.callbackRedirect("/learn", "JWT")
		before, _, _ := strings.Cut(got, "#")
		assert.NotContains(t, before, "JWT")
	})
}
