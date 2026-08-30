package auth

import (
	"context"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/schemas"
)

// TestUserRepository_LivePostgres_Integration exercises the account upsert /
// lookup path against a real Postgres — including that PII is encrypted at rest
// (the persisted email/name bytes never contain the plaintext) and that the
// blind index enforces one-account-per-email.
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

	enc, err := NewEncryptor([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	repo := NewUserRepository(db, enc)
	ctx := context.Background()

	// First sign-in registers the account.
	user, err := repo.Upsert(ctx, "google-sub-abc", "Alice@Example.com", "Alice")
	require.NoError(t, err)
	assert.NotZero(t, user.ID)

	// The persisted PII columns must NOT contain the plaintext (encryption at
	// rest). Read the raw bytes straight from the row.
	var row userRow
	require.NoError(t, db.Get(&row,
		`SELECT id, google_sub, email_encrypted, name_encrypted FROM users WHERE id = $1`, user.ID))
	assert.NotContains(t, string(row.EmailEncrypted), "Alice@Example.com",
		"email must be encrypted at rest, not stored as plaintext")
	assert.NotContains(t, string(row.NameEncrypted), "Alice",
		"name must be encrypted at rest, not stored as plaintext")

	// FindByID decrypts for display.
	byID, err := repo.FindByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "Alice@Example.com", byID.Email)
	assert.Equal(t, "Alice", byID.Name)

	// FindByEmail resolves via the blind index and is case/space-insensitive.
	byEmail, err := repo.FindByEmail(ctx, "  alice@example.com ")
	require.NoError(t, err)
	assert.Equal(t, user.ID, byEmail.ID)

	// Second sign-in for the same google_sub updates in place (no duplicate
	// row) and reflects a changed display name.
	updated, err := repo.Upsert(ctx, "google-sub-abc", "Alice@Example.com", "Alice Smith")
	require.NoError(t, err)
	assert.Equal(t, user.ID, updated.ID)

	var count int
	require.NoError(t, db.Get(&count, `SELECT COUNT(*) FROM users`))
	assert.Equal(t, 1, count, "re-sign-in must not create a second row")

	reloaded, err := repo.FindByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "Alice Smith", reloaded.Name)
}
