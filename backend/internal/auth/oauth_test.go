package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOAuthClient_Endpoint(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"provider-token","token_type":"Bearer","expires_in":3600}`))
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer provider-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"sub":"sub-1","email":"user@example.com","name":"User"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer provider.Close()

	tests := []struct {
		name         string
		endpoint     Endpoint
		wantAuthURL  string
		wantUserInfo bool
	}{
		{
			name:        "empty endpoint keeps Google",
			endpoint:    Endpoint{},
			wantAuthURL: "https://accounts.google.com/",
		},
		{
			name: "override points at the configured provider",
			endpoint: Endpoint{
				AuthURL:     provider.URL + "/authorize",
				TokenURL:    provider.URL + "/token",
				UserInfoURL: provider.URL + "/userinfo",
			},
			wantAuthURL:  provider.URL + "/authorize",
			wantUserInfo: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewOAuthClient("client-id", "secret", "http://localhost/callback", tt.endpoint, nil)
			assert.True(t, strings.HasPrefix(client.AuthCodeURL("state-1"), tt.wantAuthURL), client.AuthCodeURL("state-1"))
			if !tt.wantUserInfo {
				return
			}

			token, err := client.Exchange(context.Background(), "code-1")
			require.NoError(t, err)
			info, err := client.FetchUserInfo(context.Background(), token)
			require.NoError(t, err)
			assert.Equal(t, UserInfo{Sub: "sub-1", Email: "user@example.com", Name: "User"}, info)
		})
	}
}
