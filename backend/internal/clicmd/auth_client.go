package clicmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const cliClientID = "langner-cli"
const deviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"

// AuthClient talks to a server's plain-HTTP auth endpoints (device flow,
// refresh, revoke, /auth/me). It is separate from the Connect RPC client.
type AuthClient struct {
	baseURL string
	http    *http.Client
}

// NewAuthClient builds an AuthClient for a server base URL.
func NewAuthClient(baseURL string) *AuthClient {
	return &AuthClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// DeviceCodeResponse mirrors POST /auth/device/code.
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// TokenResponse mirrors the success shape of the token/refresh endpoints.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

// oauthErrorResponse is the RFC 8628 error body.
type oauthErrorResponse struct {
	Error string `json:"error"`
}

// OAuthError carries an RFC 8628 error code from a 4xx auth response so callers
// can branch on authorization_pending / slow_down / expired_token / etc.
type OAuthError struct {
	Code string
}

func (e *OAuthError) Error() string { return e.Code }

// RequestDeviceCode starts the device flow.
func (c *AuthClient) RequestDeviceCode(ctx context.Context) (*DeviceCodeResponse, error) {
	var out DeviceCodeResponse
	if err := c.postJSON(ctx, "/auth/device/code", map[string]string{"client_id": cliClientID}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PollToken polls once for the device_code. On a 4xx it returns an *OAuthError.
func (c *AuthClient) PollToken(ctx context.Context, deviceCode string) (*TokenResponse, error) {
	var out TokenResponse
	if err := c.postJSON(ctx, "/auth/device/token", map[string]string{
		"grant_type":  deviceGrantType,
		"device_code": deviceCode,
		"client_id":   cliClientID,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Refresh rotates the refresh token for a fresh pair.
func (c *AuthClient) Refresh(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	var out TokenResponse
	if err := c.postJSON(ctx, "/auth/token/refresh", map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Revoke revokes a refresh token (logout). Idempotent server-side.
func (c *AuthClient) Revoke(ctx context.Context, refreshToken string) error {
	return c.postJSON(ctx, "/auth/token/revoke", map[string]string{"refresh_token": refreshToken}, nil)
}

// MeResponse mirrors GET /auth/me.
type MeResponse struct {
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username"`
}

// Me calls /auth/me with a bearer token and returns the identity.
func (c *AuthClient) Me(ctx context.Context, accessToken string) (*MeResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/auth/me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call /auth/me: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, &OAuthError{Code: "unauthenticated"}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("/auth/me returned status %d", resp.StatusCode)
	}
	var out MeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode /auth/me: %w", err)
	}
	return &out, nil
}

// postJSON POSTs a JSON body and decodes a JSON response into out (may be nil).
// A 4xx with an {"error": "..."} body becomes an *OAuthError.
func (c *AuthClient) postJSON(ctx context.Context, path string, body any, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var oe oauthErrorResponse
		if json.Unmarshal(data, &oe) == nil && oe.Error != "" {
			return &OAuthError{Code: oe.Error}
		}
		return fmt.Errorf("POST %s returned status %d", path, resp.StatusCode)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode %s response: %w", path, err)
		}
	}
	return nil
}

// isOAuthErr reports whether err is an *OAuthError with the given code.
func isOAuthErr(err error, code string) bool {
	var oe *OAuthError
	return errors.As(err, &oe) && oe.Code == code
}
