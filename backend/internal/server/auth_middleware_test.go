package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/auth"
)

func TestNewAuthInterceptor(t *testing.T) {
	interceptor := NewAuthInterceptor()

	var sawUserID int64
	var sawUserOK bool
	var called bool
	next := func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		called = true
		sawUserID, sawUserOK = auth.UserIDFromContext(ctx)
		return nil, nil
	}
	wrapped := interceptor(next)

	t.Run("no session is rejected with CodeUnauthenticated", func(t *testing.T) {
		called = false
		_, err := wrapped(context.Background(), nil)
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
		assert.False(t, called, "handler must not run without a session")
	})

	t.Run("valid session passes and injects the user id", func(t *testing.T) {
		called = false
		ctx := auth.WithSession(context.Background(), auth.Session{UserID: 7, ExpiresAt: time.Now().Add(time.Hour)})
		_, err := wrapped(ctx, nil)
		require.NoError(t, err)
		assert.True(t, called)
		assert.True(t, sawUserOK)
		assert.Equal(t, int64(7), sawUserID)
	})
}

func TestAuthCookieMiddleware(t *testing.T) {
	signer, err := auth.NewSessionSigner([]byte("mw-key"))
	require.NoError(t, err)

	var gotSession auth.Session
	var gotOK bool
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotSession, gotOK = auth.SessionFromContext(r.Context())
	})
	mw := AuthCookieMiddleware(inner, signer)

	t.Run("valid cookie stows session", func(t *testing.T) {
		gotOK = false
		value, err := signer.Sign(auth.Session{UserID: 9, ExpiresAt: time.Now().Add(time.Hour)})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: value})
		mw.ServeHTTP(httptest.NewRecorder(), req)
		assert.True(t, gotOK)
		assert.Equal(t, int64(9), gotSession.UserID)
	})

	t.Run("missing cookie leaves context empty but passes through", func(t *testing.T) {
		gotOK = false
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		mw.ServeHTTP(httptest.NewRecorder(), req)
		assert.False(t, gotOK)
	})

	t.Run("invalid cookie leaves context empty but passes through", func(t *testing.T) {
		gotOK = false
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "tampered.value"})
		mw.ServeHTTP(httptest.NewRecorder(), req)
		assert.False(t, gotOK)
	})
}
