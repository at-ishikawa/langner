package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/schemas"
)

// newRefreshTestDB spins a migrated schema with one user and returns the db +
// that user's id. integrationDBEnv is declared in quiz_handler_pg_integration_test.go.
func newRefreshTestDB(t *testing.T) (*sqlx.DB, int64) {
	t.Helper()
	dsn := os.Getenv(integrationDBEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping live-Postgres refresh-token coverage", integrationDBEnv)
	}
	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))
	var userID int64
	require.NoError(t, db.Get(&userID,
		`INSERT INTO users (google_sub, username) VALUES ('refresh-user','refresh-user') RETURNING id`))
	return db, userID
}

func newDeviceHandler(db *sqlx.DB) *DeviceAuthHandler {
	tokens, _ := auth.NewTokenSigner([]byte("refresh-test-signing-key"))
	return NewDeviceAuthHandler(DeviceAuthHandlerConfig{
		RefreshTokens:  auth.NewCLIRefreshTokenRepository(db),
		Tokens:         tokens,
		CookieSameSite: http.SameSiteLaxMode,
	})
}

// A replay (reuse of an already-rotated token) revokes ONLY its own family — so
// a compromise on the web session must not log out the user's CLI device, and
// vice-versa. This is the whole point of per-login families.
func TestDeviceRefresh_ReplayRevokesOnlyItsFamily(t *testing.T) {
	db, userID := newRefreshTestDB(t)
	repo := auth.NewCLIRefreshTokenRepository(db)
	h := newDeviceHandler(db)
	now := time.Now()
	exp := now.Add(24 * time.Hour)

	// Two independent families: the web login and a CLI device.
	webTok, _ := auth.GenerateRefreshToken()
	cliTok, _ := auth.GenerateRefreshToken()
	_, err := repo.Create(t.Context(), userID, "web-family", auth.HashSecret(webTok), exp)
	require.NoError(t, err)
	_, err = repo.Create(t.Context(), userID, "cli-family", auth.HashSecret(cliTok), exp)
	require.NoError(t, err)

	// Rotate the web token once (normal refresh), so the original web token is now
	// revoked-away but the family is still live via its successor.
	rec := doRefreshBody(h, webTok)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Replay the ORIGINAL (now rotated-away) web token → compromise signal.
	rec = doRefreshBody(h, webTok)
	require.Equal(t, http.StatusBadRequest, rec.Code, "a replayed token is rejected")

	// The web family is fully revoked (even its live successor)…
	var liveWeb int
	require.NoError(t, db.Get(&liveWeb,
		`SELECT count(*) FROM cli_refresh_tokens WHERE family_id='web-family' AND revoked_at IS NULL`))
	assert.Equal(t, 0, liveWeb, "replay must revoke the whole web family")

	// …but the CLI family is untouched — the CLI stays logged in.
	cliRow, err := repo.FindByHash(t.Context(), auth.HashSecret(cliTok))
	require.NoError(t, err)
	assert.False(t, cliRow.RevokedAt.Valid, "a web replay must NOT revoke the CLI family")
}

// The web carrier: a refresh presented via the HttpOnly cookie returns the new
// refresh token as a Set-Cookie and NOT in the body; the CLI carrier (body)
// returns it in the body. Same endpoint, reply-in-kind.
func TestDeviceRefresh_CookieVsBodyCarrier(t *testing.T) {
	db, userID := newRefreshTestDB(t)
	repo := auth.NewCLIRefreshTokenRepository(db)
	h := newDeviceHandler(db)
	exp := time.Now().Add(24 * time.Hour)

	// Cookie carrier.
	webTok, _ := auth.GenerateRefreshToken()
	_, err := repo.Create(t.Context(), userID, "web-family", auth.HashSecret(webTok), exp)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/auth/token/refresh", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: webTok})
	rec := httptest.NewRecorder()
	h.Refresh(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body tokenResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.NotEmpty(t, body.AccessToken, "cookie refresh still returns an access token in the body")
	assert.Empty(t, body.RefreshToken, "cookie refresh must NOT echo the refresh token in the body")
	var setCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == refreshCookieName {
			setCookie = c
		}
	}
	require.NotNil(t, setCookie, "cookie refresh rotates the token into a new Set-Cookie")
	assert.True(t, setCookie.HttpOnly)
	assert.NotEqual(t, webTok, setCookie.Value, "rotation changes the token")

	// Body carrier (CLI).
	cliTok, _ := auth.GenerateRefreshToken()
	_, err = repo.Create(t.Context(), userID, "cli-family", auth.HashSecret(cliTok), exp)
	require.NoError(t, err)
	rec = doRefreshBody(h, cliTok)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.NotEmpty(t, body.RefreshToken, "body refresh returns the rotated token in the body (CLI)")
}

func doRefreshBody(h *DeviceAuthHandler, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/auth/token/refresh",
		strings.NewReader(`{"refresh_token":"`+token+`"}`))
	rec := httptest.NewRecorder()
	h.Refresh(rec, req)
	return rec
}
