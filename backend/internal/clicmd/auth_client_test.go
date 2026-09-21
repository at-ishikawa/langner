package clicmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuthClient_DeviceFlow exercises the CLI's HTTP client against a fake
// server: request a code, a pending poll returns an OAuthError, and a later
// poll succeeds with tokens.
func TestAuthClient_DeviceFlow(t *testing.T) {
	var polls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/device/code":
			_ = json.NewEncoder(w).Encode(DeviceCodeResponse{
				DeviceCode: "dev-code", UserCode: "WDJB-MJHT",
				VerificationURI: "http://x/auth/device", ExpiresIn: 900, Interval: 5,
			})
		case "/auth/device/token":
			if atomic.AddInt32(&polls, 1) == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
				return
			}
			_ = json.NewEncoder(w).Encode(TokenResponse{
				AccessToken: "access-jwt", TokenType: "Bearer", ExpiresIn: 86400, RefreshToken: "refresh-opaque",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := NewAuthClient(srv.URL)
	ctx := context.Background()

	dc, err := client.RequestDeviceCode(ctx)
	require.NoError(t, err)
	assert.Equal(t, "WDJB-MJHT", dc.UserCode)
	assert.Equal(t, 5, dc.Interval)

	_, err = client.PollToken(ctx, dc.DeviceCode)
	require.Error(t, err)
	assert.True(t, isOAuthErr(err, "authorization_pending"))

	tok, err := client.PollToken(ctx, dc.DeviceCode)
	require.NoError(t, err)
	assert.Equal(t, "access-jwt", tok.AccessToken)
	assert.Equal(t, "refresh-opaque", tok.RefreshToken)
}

func TestAuthClient_Me(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/me" {
			if r.Header.Get("Authorization") != "Bearer good" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(MeResponse{Authenticated: false})
				return
			}
			_ = json.NewEncoder(w).Encode(MeResponse{Authenticated: true, Username: "user_abc"})
		}
	}))
	defer srv.Close()

	client := NewAuthClient(srv.URL)
	me, err := client.Me(context.Background(), "good")
	require.NoError(t, err)
	assert.Equal(t, "user_abc", me.Username)

	_, err = client.Me(context.Background(), "bad")
	require.Error(t, err)
	assert.True(t, isOAuthErr(err, "unauthenticated"))
}

// TestEnsureAccessToken_ProactiveRefresh proves a near-expired access token is
// refreshed (and the rotated pair persisted) before use.
func TestEnsureAccessToken_ProactiveRefresh(t *testing.T) {
	withTempConfigHome(t)

	var refreshed int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token/refresh" {
			atomic.AddInt32(&refreshed, 1)
			_ = json.NewEncoder(w).Encode(TokenResponse{
				AccessToken: "fresh-jwt", TokenType: "Bearer", ExpiresIn: 86400, RefreshToken: "rotated-refresh",
			})
		}
	}))
	defer srv.Close()

	store, err := LoadCredentials()
	require.NoError(t, err)
	store.Set(srv.URL, ServerCredentials{
		AccessToken:          "stale-jwt",
		AccessTokenExpiresAt: time.Now().Add(10 * time.Second), // within skew → refresh
		RefreshToken:         "old-refresh",
	}, true)
	require.NoError(t, store.Save())

	tok, err := ensureAccessToken(context.Background(), store, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "fresh-jwt", tok)
	assert.EqualValues(t, 1, refreshed)

	// The rotated pair was persisted.
	reloaded, err := LoadCredentials()
	require.NoError(t, err)
	got, _ := reloaded.Get(srv.URL)
	assert.Equal(t, "fresh-jwt", got.AccessToken)
	assert.Equal(t, "rotated-refresh", got.RefreshToken)
}

func TestEnsureAccessToken_NotLoggedIn(t *testing.T) {
	withTempConfigHome(t)
	store, err := LoadCredentials()
	require.NoError(t, err)
	_, err = ensureAccessToken(context.Background(), store, "https://nope.example")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not logged in")
}

// TestBearerInterceptor_ForceRefresh_InvalidGrant proves a dead refresh token
// clears the entry and tells the user to log in again.
func TestBearerInterceptor_ForceRefresh_InvalidGrant(t *testing.T) {
	withTempConfigHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token/refresh" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
		}
	}))
	defer srv.Close()

	store, err := LoadCredentials()
	require.NoError(t, err)
	store.Set(srv.URL, ServerCredentials{RefreshToken: "dead"}, true)
	require.NoError(t, store.Save())

	i := NewBearerInterceptor(store, srv.URL)
	_, err = i.forceRefresh(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session expired")

	// Entry cleared.
	_, ok := store.Get(srv.URL)
	assert.False(t, ok)
}
