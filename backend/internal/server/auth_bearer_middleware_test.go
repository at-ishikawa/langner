package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/auth"
)

// TestAuthBearerMiddleware proves bearer acceptance is ADDITIVE: a valid bearer
// authenticates, a bad/absent bearer leaves the context empty (the interceptor
// then 401s), and — composed inside the cookie middleware — a cookie'd request
// keeps its cookie identity even if a (different) bearer is also present.
func TestAuthBearerMiddleware(t *testing.T) {
	tokens, err := auth.NewTokenSigner([]byte("bearer-key"))
	require.NoError(t, err)

	var gotSession auth.Session
	var gotOK bool
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotSession, gotOK = auth.SessionFromContext(r.Context())
	})
	mw := AuthBearerMiddleware(inner, tokens)

	t.Run("valid bearer authenticates", func(t *testing.T) {
		gotOK = false
		token, err := tokens.Sign(11, time.Now())
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		mw.ServeHTTP(httptest.NewRecorder(), req)
		assert.True(t, gotOK)
		assert.Equal(t, int64(11), gotSession.UserID)
	})

	t.Run("no header leaves context empty", func(t *testing.T) {
		gotOK = false
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		mw.ServeHTTP(httptest.NewRecorder(), req)
		assert.False(t, gotOK)
	})

	t.Run("invalid bearer leaves context empty", func(t *testing.T) {
		gotOK = false
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer not-a-jwt")
		mw.ServeHTTP(httptest.NewRecorder(), req)
		assert.False(t, gotOK)
	})
}

// TestCookieAndBearerCompose proves the intended composition
// AuthCookieMiddleware(AuthBearerMiddleware(inner)) does not weaken cookie auth:
// a cookie'd request authenticates by cookie (and wins over any bearer), a
// bearer-only request authenticates by bearer, and an anonymous request reaches
// the inner handler with an empty context (which the interceptor rejects).
func TestCookieAndBearerCompose(t *testing.T) {
	sessions, err := auth.NewSessionSigner([]byte("cookie-key"))
	require.NoError(t, err)
	tokens, err := auth.NewTokenSigner([]byte("bearer-key"))
	require.NoError(t, err)

	var gotSession auth.Session
	var gotOK bool
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotSession, gotOK = auth.SessionFromContext(r.Context())
	})
	stack := AuthCookieMiddleware(AuthBearerMiddleware(inner, tokens), sessions)

	t.Run("cookie request still works", func(t *testing.T) {
		gotOK = false
		value, err := sessions.Sign(auth.Session{UserID: 5, ExpiresAt: time.Now().Add(time.Hour)})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: value})
		stack.ServeHTTP(httptest.NewRecorder(), req)
		assert.True(t, gotOK)
		assert.Equal(t, int64(5), gotSession.UserID)
	})

	t.Run("cookie wins over a different bearer", func(t *testing.T) {
		gotOK = false
		value, err := sessions.Sign(auth.Session{UserID: 5, ExpiresAt: time.Now().Add(time.Hour)})
		require.NoError(t, err)
		bearer, err := tokens.Sign(99, time.Now())
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: value})
		req.Header.Set("Authorization", "Bearer "+bearer)
		stack.ServeHTTP(httptest.NewRecorder(), req)
		assert.True(t, gotOK)
		assert.Equal(t, int64(5), gotSession.UserID, "cookie identity must win")
	})

	t.Run("bearer-only request authenticates", func(t *testing.T) {
		gotOK = false
		bearer, err := tokens.Sign(7, time.Now())
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+bearer)
		stack.ServeHTTP(httptest.NewRecorder(), req)
		assert.True(t, gotOK)
		assert.Equal(t, int64(7), gotSession.UserID)
	})

	t.Run("anonymous request stays empty", func(t *testing.T) {
		gotOK = false
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		stack.ServeHTTP(httptest.NewRecorder(), req)
		assert.False(t, gotOK)
	})
}
