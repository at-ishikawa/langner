package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/gen-protos/api/v1/apiv1connect"
	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference/mock"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/schemas"
)

// headerRoundTripper attaches fixed headers to every outgoing request so a
// Connect client can present a bearer or a cookie.
type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h headerRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	for k, v := range h.headers {
		r.Header.Set(k, v)
	}
	return h.base.RoundTrip(r)
}

func quizClientWithHeaders(baseURL string, headers map[string]string) apiv1connect.QuizServiceClient {
	return apiv1connect.NewQuizServiceClient(
		&http.Client{Transport: headerRoundTripper{base: http.DefaultTransport, headers: headers}},
		baseURL,
	)
}

// TestDeviceFlow_RealConfig_LivePostgres drives the whole CLI device-flow bearer
// feature end-to-end against a real Postgres, constructing the server the way
// cmd/langner-server/main.go does: config loaded from config.example.yml (auth
// + token signing keys set via env, exactly as production supplies them), the
// SAME exported constructors (NewSessionSigner / NewTokenSigner / the CLI repos
// / NewDeviceAuthHandler), a real gated Connect RPC guarded by the real
// NewAuthInterceptor, and the real AuthCookieMiddleware(AuthBearerMiddleware(…))
// stack. It proves: (1) the device flow mints a bearer that authenticates a
// gated RPC; (2) an anonymous RPC still 401s; (3) bearer does NOT weaken the
// cookie (a cookie'd RPC still authenticates); (4) refresh rotates and reusing
// the rotated token revokes the family; (5) revoke makes a later refresh fail.
func TestDeviceFlow_RealConfig_LivePostgres(t *testing.T) {
	dsn := os.Getenv(integrationDBEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping live-Postgres device-flow coverage", integrationDBEnv)
	}

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	// --- Load config the way the server does, from config.example.yml, with the
	// signing keys supplied via env exactly as production/dev provides them. ---
	t.Setenv("SESSION_SIGNING_KEY", "test-session-signing-key")
	t.Setenv("TOKEN_SIGNING_KEY", "test-token-signing-key")
	repoRoot := repoRootDir(t)
	loader, err := config.NewConfigLoader(filepath.Join(repoRoot, "config.example.yml"))
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)
	require.True(t, cfg.Auth.Enabled(), "cookie auth must be enabled by SESSION_SIGNING_KEY")
	require.True(t, cfg.Auth.TokenEnabled(), "bearer auth must be enabled by TOKEN_SIGNING_KEY")

	// --- Build the auth/device components with the SAME constructors main.go
	// uses in buildAuth/buildDeviceAuth. ---
	sessions, err := auth.NewSessionSigner(auth.DecodeKey(cfg.Auth.SessionSigningKey))
	require.NoError(t, err)
	tokens, err := auth.NewTokenSigner(auth.DecodeKey(cfg.Auth.TokenSigningKey))
	require.NoError(t, err)
	deviceCodes := auth.NewCLIDeviceCodeRepository(db)
	refreshTokens := auth.NewCLIRefreshTokenRepository(db)
	users := auth.NewUserRepository(db)
	deviceHandler := NewDeviceAuthHandler(DeviceAuthHandlerConfig{
		DeviceCodes:     deviceCodes,
		RefreshTokens:   refreshTokens,
		Tokens:          tokens,
		FrontendURL:     cfg.Auth.FrontendURL,
		VerificationURI: "http://example/auth/device",
	})

	// --- A real gated Connect RPC (QuizService), interceptor identical to
	// main.go's, so the bearer/cookie must clear the SAME gate the app uses. ---
	quizSvc := quiz.NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{t.TempDir()},
		FlashcardsDirectories:  []string{t.TempDir()},
		LearningNotesDirectory: t.TempDir(),
	}, mock.NewClient(), map[string]rapidapi.Response{},
		learning.NewMultiLearningRepository(learning.NewYAMLLearningRepository(t.TempDir(), nil), learning.NewDBLearningRepository(db)),
		config.QuizConfig{})
	quizHandler := NewQuizHandler(quizSvc)
	interceptors := connect.WithInterceptors(NewAuthInterceptor())
	mux := http.NewServeMux()
	quizPath, quizH := apiv1connect.NewQuizServiceHandler(quizHandler, interceptors)
	mux.Handle(quizPath, quizH)
	mux.HandleFunc("/auth/device/code", deviceHandler.RequestCode)
	mux.HandleFunc("/auth/device/token", deviceHandler.Token)
	mux.HandleFunc("/auth/device/approve", deviceHandler.Approve)
	mux.HandleFunc("/auth/device", deviceHandler.ShowApprovalPage)
	mux.HandleFunc("/auth/token/refresh", deviceHandler.Refresh)
	mux.HandleFunc("/auth/token/revoke", deviceHandler.Revoke)

	// The exact main.go middleware stack: cookie OUTSIDE, bearer INSIDE.
	root := AuthCookieMiddleware(AuthBearerMiddleware(mux, tokens), sessions)
	srv := httptest.NewServer(root)
	t.Cleanup(srv.Close)

	// Seed the approving user (the identity a real Google sign-in would resolve).
	user, err := users.Upsert(context.Background(), "device-flow-user")
	require.NoError(t, err)

	httpClient := &http.Client{Timeout: 10 * time.Second}

	// runDeviceFlow requests a code, approves it with a synthesized session
	// cookie (standing in for the browser Google sign-in, exactly like the
	// issue-test-cookie e2e helper), polls once, and returns the token pair.
	runDeviceFlow := func(t *testing.T) (access, refresh string) {
		t.Helper()
		var dc struct {
			DeviceCode string `json:"device_code"`
			UserCode   string `json:"user_code"`
		}
		postJSON(t, httpClient, srv.URL+"/auth/device/code", `{"client_id":"langner-cli"}`, http.StatusOK, &dc)
		require.NotEmpty(t, dc.DeviceCode)
		require.NotEmpty(t, dc.UserCode)

		// Approve: sign a session cookie for the seeded user and POST the form.
		cookieVal, err := sessions.Sign(auth.Session{UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour)})
		require.NoError(t, err)
		form := url.Values{"user_code": {dc.UserCode}}
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/auth/device/approve", strings.NewReader(form.Encode()))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: cookieVal})
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		body := readClose(t, resp)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Contains(t, body, "Approved")

		// Poll → success.
		var tok struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			TokenType    string `json:"token_type"`
		}
		pollBody := `{"grant_type":"urn:ietf:params:oauth:grant-type:device_code","device_code":"` + dc.DeviceCode + `","client_id":"langner-cli"}`
		postJSON(t, httpClient, srv.URL+"/auth/device/token", pollBody, http.StatusOK, &tok)
		require.Equal(t, "Bearer", tok.TokenType)
		require.NotEmpty(t, tok.AccessToken)
		require.NotEmpty(t, tok.RefreshToken)
		return tok.AccessToken, tok.RefreshToken
	}

	t.Run("pending before approval, then bearer authenticates a gated RPC", func(t *testing.T) {
		// A code that is requested but not yet approved polls as pending.
		var dc struct {
			DeviceCode string `json:"device_code"`
			UserCode   string `json:"user_code"`
		}
		postJSON(t, httpClient, srv.URL+"/auth/device/code", `{"client_id":"langner-cli"}`, http.StatusOK, &dc)
		var oe struct {
			Error string `json:"error"`
		}
		pollBody := `{"grant_type":"urn:ietf:params:oauth:grant-type:device_code","device_code":"` + dc.DeviceCode + `","client_id":"langner-cli"}`
		postJSON(t, httpClient, srv.URL+"/auth/device/token", pollBody, http.StatusBadRequest, &oe)
		assert.Equal(t, "authorization_pending", oe.Error)

		access, _ := runDeviceFlow(t)

		// Bearer authenticates the gated RPC (NOT Unauthenticated).
		bearerClient := quizClientWithHeaders(srv.URL, map[string]string{"Authorization": "Bearer " + access})
		_, err := bearerClient.GetQuizOptions(context.Background(), connect.NewRequest(&apiv1.GetQuizOptionsRequest{}))
		assert.NotEqual(t, connect.CodeUnauthenticated, connect.CodeOf(err), "valid bearer must clear the auth gate; got %v", err)

		// Anonymous RPC still 401s.
		anonClient := quizClientWithHeaders(srv.URL, nil)
		_, err = anonClient.GetQuizOptions(context.Background(), connect.NewRequest(&apiv1.GetQuizOptionsRequest{}))
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err), "anonymous RPC must 401")
	})

	t.Run("bearer does not weaken cookie auth", func(t *testing.T) {
		cookieVal, err := sessions.Sign(auth.Session{UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour)})
		require.NoError(t, err)
		cookieClient := quizClientWithHeaders(srv.URL, map[string]string{"Cookie": SessionCookieName + "=" + cookieVal})
		_, err = cookieClient.GetQuizOptions(context.Background(), connect.NewRequest(&apiv1.GetQuizOptionsRequest{}))
		assert.NotEqual(t, connect.CodeUnauthenticated, connect.CodeOf(err), "a cookie'd RPC must still authenticate; got %v", err)
	})

	t.Run("refresh rotates; reusing the rotated token revokes the family", func(t *testing.T) {
		_, refresh0 := runDeviceFlow(t)

		// Rotate: refresh0 → (access1, refresh1). refresh0 is now revoked.
		var pair1 struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		}
		postJSON(t, httpClient, srv.URL+"/auth/token/refresh", `{"grant_type":"refresh_token","refresh_token":"`+refresh0+`"}`, http.StatusOK, &pair1)
		require.NotEmpty(t, pair1.RefreshToken)
		assert.NotEqual(t, refresh0, pair1.RefreshToken, "refresh must rotate the token")

		// Reuse refresh0 (already rotated away) → invalid_grant AND family revoke.
		var oe struct {
			Error string `json:"error"`
		}
		postJSON(t, httpClient, srv.URL+"/auth/token/refresh", `{"grant_type":"refresh_token","refresh_token":"`+refresh0+`"}`, http.StatusBadRequest, &oe)
		assert.Equal(t, "invalid_grant", oe.Error)

		// The rotated-to refresh1 is now dead too (whole family revoked).
		postJSON(t, httpClient, srv.URL+"/auth/token/refresh", `{"grant_type":"refresh_token","refresh_token":"`+pair1.RefreshToken+`"}`, http.StatusBadRequest, &oe)
		assert.Equal(t, "invalid_grant", oe.Error, "family revocation must kill the rotated token too")
	})

	t.Run("revoke makes a later refresh fail", func(t *testing.T) {
		_, refresh := runDeviceFlow(t)
		// Revoke (logout) always 200.
		postJSON(t, httpClient, srv.URL+"/auth/token/revoke", `{"refresh_token":"`+refresh+`"}`, http.StatusOK, nil)
		// A subsequent refresh with the revoked token fails.
		var oe struct {
			Error string `json:"error"`
		}
		postJSON(t, httpClient, srv.URL+"/auth/token/refresh", `{"grant_type":"refresh_token","refresh_token":"`+refresh+`"}`, http.StatusBadRequest, &oe)
		assert.Equal(t, "invalid_grant", oe.Error)
	})
}

// postJSON POSTs a JSON body, asserts the status code, and decodes the response
// into out (when non-nil).
func postJSON(t *testing.T, client *http.Client, url, body string, wantStatus int, out any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, wantStatus, resp.StatusCode, "POST %s", url)
	if out != nil {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
	}
}

// readClose reads and closes a response body, returning it as a string.
func readClose(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(data)
}

// repoRootDir walks up from the test's working dir to the repo root that holds
// config.example.yml.
func repoRootDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "config.example.yml")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("could not locate config.example.yml above %s", dir)
	return ""
}
