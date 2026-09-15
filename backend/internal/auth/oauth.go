package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// UserInfo is the subset of Google's userinfo response the callback needs.
type UserInfo struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// Authenticator is the seam the OAuth callback depends on. The real
// implementation (OAuthClient) talks to Google; tests supply a fake so the
// callback's state/allowlist/upsert/cookie logic is exercised WITHOUT real
// Google credentials or network access.
type Authenticator interface {
	// AuthCodeURL returns the Google consent-screen URL for the given state.
	AuthCodeURL(state string) string
	// Exchange trades an authorization code for a token.
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)
	// FetchUserInfo resolves the signed-in user's profile from a token.
	FetchUserInfo(ctx context.Context, token *oauth2.Token) (UserInfo, error)
}

// UserInfoFetcher is the narrow seam for the userinfo call. It is split from
// Exchange so a real OAuthClient can keep Google's token exchange while a test
// (or a future provider) fakes only the profile lookup.
type UserInfoFetcher interface {
	FetchUserInfo(ctx context.Context, token *oauth2.Token) (UserInfo, error)
}

// OAuthClient is the production Authenticator backed by golang.org/x/oauth2 and
// Google's endpoint. Scopes are openid/email/profile.
type OAuthClient struct {
	config  *oauth2.Config
	fetcher UserInfoFetcher
}

// NewOAuthClient builds an OAuthClient. A nil fetcher defaults to the live
// Google userinfo endpoint.
func NewOAuthClient(clientID, clientSecret, redirectURL string, fetcher UserInfoFetcher) *OAuthClient {
	cfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}
	if fetcher == nil {
		fetcher = googleUserInfoFetcher{}
	}
	return &OAuthClient{config: cfg, fetcher: fetcher}
}

// AuthCodeURL returns Google's consent URL for the given state.
func (c *OAuthClient) AuthCodeURL(state string) string {
	return c.config.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

// Exchange trades an authorization code for an OAuth token.
func (c *OAuthClient) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	token, err := c.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}
	return token, nil
}

// FetchUserInfo resolves the profile via the configured fetcher.
func (c *OAuthClient) FetchUserInfo(ctx context.Context, token *oauth2.Token) (UserInfo, error) {
	return c.fetcher.FetchUserInfo(ctx, token)
}

// googleUserInfoFetcher calls Google's OpenID userinfo endpoint using the
// access token.
type googleUserInfoFetcher struct{}

const googleUserInfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"

func (googleUserInfoFetcher) FetchUserInfo(ctx context.Context, token *oauth2.Token) (UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return UserInfo{}, fmt.Errorf("build userinfo request: %w", err)
	}
	token.SetAuthHeader(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return UserInfo{}, fmt.Errorf("fetch userinfo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return UserInfo{}, fmt.Errorf("userinfo request failed with status %d", resp.StatusCode)
	}
	var info UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return UserInfo{}, fmt.Errorf("decode userinfo: %w", err)
	}
	return info, nil
}
