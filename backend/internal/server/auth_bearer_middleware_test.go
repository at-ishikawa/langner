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

// TestAuthBearerMiddleware proves the bearer is the ONLY auth path: a valid
// bearer authenticates the request context, and a bad/absent bearer leaves the
// context empty (the interceptor then 401s).
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
