package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	sessions, err := auth.NewSessionSigner([]byte("handler-test-session-key"))
	require.NoError(t, err)
	state, err := auth.NewStateSigner([]byte("handler-test-session-key"))
	require.NoError(t, err)
	return NewAuthHandler(AuthHandlerConfig{
		Authenticator: fakeAuthenticator{info: info},
		Sessions:      sessions,
		State:         state,
		// Users is intentionally nil: the allowlist-reject path must return
		// BEFORE any user upsert, so touching the DB here would panic the test.
		Users:          nil,
		AllowedEmails:  allowed,
		FrontendURL:    "http://frontend.example",
		CookieSecure:   false,
		CookieSameSite: http.SameSiteLaxMode,
	})
}

func sessionCookieSet(rec *httptest.ResponseRecorder) bool {
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName && c.Value != "" {
			return true
		}
	}
	return false
}

func TestAuthHandler_Callback_AllowlistReject(t *testing.T) {
	h := newTestAuthHandler(t, []string{"admin@example.com"},
		auth.UserInfo{Sub: "google-sub-123", Email: "intruder@example.com", Name: "Intruder"})

	// A valid, matching state so the flow reaches the allowlist check.
	stateVal, err := h.state.Issue(stateTTL)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet,
		"/auth/google/callback?state="+url.QueryEscape(stateVal)+"&code=any-code", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: stateVal})
	rec := httptest.NewRecorder()

	h.Callback(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "http://frontend.example/login?error=not_allowed", rec.Header().Get("Location"))
	assert.False(t, sessionCookieSet(rec), "no session cookie must be set for a rejected email")
}

func TestAuthHandler_Callback_InvalidState(t *testing.T) {
	h := newTestAuthHandler(t, []string{"admin@example.com"},
		auth.UserInfo{Sub: "s", Email: "admin@example.com", Name: "Admin"})

	// State cookie present but query state does not match.
	stateVal, err := h.state.Issue(stateTTL)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet,
		"/auth/google/callback?state=mismatched&code=any-code", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: stateVal})
	rec := httptest.NewRecorder()

	h.Callback(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "http://frontend.example/login?error=invalid_state", rec.Header().Get("Location"))
	assert.False(t, sessionCookieSet(rec))
}

func TestAuthHandler_Me_Unauthenticated(t *testing.T) {
	h := newTestAuthHandler(t, nil, auth.UserInfo{})
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()

	h.Me(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
