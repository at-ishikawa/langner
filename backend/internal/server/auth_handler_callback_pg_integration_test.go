package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/schemas"
)

// integrationDBEnv is declared in quiz_handler_pg_integration_test.go (same
// package). Drives the web OAuth callback success path against a live Postgres and proves
// the session-persistence feature end-to-end: the callback mints an access
// token AND a refresh token, delivers both in the redirect fragment, stores the
// refresh token hashed, and the stored token is usable by /auth/token/refresh.
func TestAuthHandler_Callback_MintsUsableWebRefreshToken(t *testing.T) {
	dsn := os.Getenv(integrationDBEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping live-Postgres web refresh-token coverage", integrationDBEnv)
	}
	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	tokens, err := auth.NewTokenSigner([]byte("callback-refresh-test-key"))
	require.NoError(t, err)
	state, err := auth.NewStateSigner([]byte("callback-refresh-test-key"))
	require.NoError(t, err)
	refreshRepo := auth.NewCLIRefreshTokenRepository(db)

	h := NewAuthHandler(AuthHandlerConfig{
		Authenticator:  fakeAuthenticator{info: auth.UserInfo{Sub: "google-sub-web", Email: "admin@example.com", Name: "Admin"}},
		Tokens:         tokens,
		State:          state,
		Users:          auth.NewUserRepository(db),
		RefreshTokens:  refreshRepo,
		AllowedEmails:  []string{"admin@example.com"},
		FrontendURL:    "http://frontend.example",
		AllowedOrigins: []string{"http://frontend.example"},
		CookieSameSite: http.SameSiteLaxMode,
	})

	stateVal, err := h.state.Issue(stateTTL, auth.StateClaims{})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet,
		"/auth/google/callback?state="+url.QueryEscape(stateVal)+"&code=any-code", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: stateVal})
	rec := httptest.NewRecorder()

	h.Callback(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)

	// The access token still rides the fragment (SPA reads it into memory).
	loc := rec.Header().Get("Location")
	frag := loc[strings.Index(loc, "#")+1:]
	params, err := url.ParseQuery(frag)
	require.NoError(t, err)
	require.NotEmpty(t, params.Get("access_token"), "callback must deliver an access token in the fragment")
	require.Empty(t, params.Get("refresh_token"), "refresh token must NOT be in the fragment")

	// The refresh token is delivered ONLY as an HttpOnly cookie — never readable
	// by JS. Find it in Set-Cookie, confirm its attributes, and that it is stored
	// hashed (usable by /auth/token/refresh) under a fresh family.
	var refreshCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == refreshCookieName {
			refreshCookie = c
		}
	}
	require.NotNil(t, refreshCookie, "callback must set the refresh cookie")
	assert.True(t, refreshCookie.HttpOnly, "refresh cookie must be HttpOnly (JS can't read it)")
	assert.Equal(t, refreshCookiePath, refreshCookie.Path)
	assert.NotEmpty(t, refreshCookie.Value)

	row, err := refreshRepo.FindByHash(req.Context(), auth.HashSecret(refreshCookie.Value))
	require.NoError(t, err, "the cookie's refresh token must be stored hashed")
	assert.False(t, row.RevokedAt.Valid, "a freshly minted refresh token is not revoked")
	assert.NotEmpty(t, row.FamilyID, "a web sign-in starts a refresh-token family")
}
