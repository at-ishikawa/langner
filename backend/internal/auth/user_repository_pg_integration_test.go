package auth

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/schemas"
)

// TestUserRepository_LivePostgres_Integration exercises the account upsert /
// lookup path against a real Postgres: an unseen google_sub registers a row
// with an auto-generated username, a returning google_sub is idempotent (no
// duplicate row) and PRESERVES the username, and distinct accounts get distinct
// usernames. No email/name is stored.
//
// Requires LANGNER_INTEGRATION_DB_URL pointing at a writable throwaway
// Postgres. Skipped otherwise so local runs stay fast; CI wires it to the
// postgres service container.
func TestUserRepository_LivePostgres_Integration(t *testing.T) {
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	require.NoError(t, db.Ping())

	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	repo := NewUserRepository(db)
	ctx := context.Background()

	// First sign-in registers the account with an auto-generated username.
	user, err := repo.Upsert(ctx, "google-sub-abc")
	require.NoError(t, err)
	assert.NotZero(t, user.ID)
	assert.True(t, strings.HasPrefix(user.Username, "user_"), "username auto-generated: %q", user.Username)
	require.NotEmpty(t, user.Username)

	// FindByID returns the same account (no PII columns exist).
	byID, err := repo.FindByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, user.ID, byID.ID)
	assert.Equal(t, "google-sub-abc", byID.GoogleSub)
	assert.Equal(t, user.Username, byID.Username)

	// Second sign-in for the same google_sub is idempotent: same row, and the
	// username is PRESERVED (only a future "update username" changes it).
	again, err := repo.Upsert(ctx, "google-sub-abc")
	require.NoError(t, err)
	assert.Equal(t, user.ID, again.ID)
	assert.Equal(t, user.Username, again.Username, "re-sign-in must not regenerate the username")

	var count int
	require.NoError(t, db.Get(&count, `SELECT COUNT(*) FROM users`))
	assert.Equal(t, 1, count, "re-sign-in must not create a second row")

	// A different google_sub is a distinct account with a distinct username.
	other, err := repo.Upsert(ctx, "google-sub-xyz")
	require.NoError(t, err)
	assert.NotEqual(t, user.ID, other.ID)
	assert.NotEqual(t, user.Username, other.Username, "distinct accounts get distinct usernames")
}
