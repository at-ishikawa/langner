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

// TestCredentialsRepository_LivePostgres_Integration exercises the per-user LLM
// credential set/get/delete path against a real Postgres — including that the
// API key is encrypted at rest (the persisted bytes never contain the plaintext
// key) and that GetCredential decrypts it back.
//
// Requires LANGNER_INTEGRATION_DB_URL pointing at a writable throwaway Postgres.
// Skipped otherwise so local runs stay fast; CI wires it to the postgres service
// container (same as the user-repository integration test).
func TestCredentialsRepository_LivePostgres_Integration(t *testing.T) {
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

	// A credential row references users(id) (FK), so create a user first.
	users := NewUserRepository(db, enc)
	ctx := context.Background()
	user, err := users.Upsert(ctx, "google-sub-cred", "Alice@Example.com", "Alice")
	require.NoError(t, err)

	repo := NewCredentialsRepository(db, enc)

	// No credential yet.
	_, ok, err := repo.GetCredential(ctx, user.ID)
	require.NoError(t, err)
	assert.False(t, ok, "a fresh user has no credential")

	// Register a credential; the key is encrypted at rest.
	const apiKey = "sk-super-secret-key-12345"
	require.NoError(t, repo.SetCredential(ctx, user.ID, "openai", apiKey, "gpt-4o-mini"))

	var raw llmCredentialRow
	require.NoError(t, db.Get(&raw,
		`SELECT provider, api_key_encrypted, model FROM user_llm_credentials WHERE user_id = $1`, user.ID))
	assert.NotContains(t, string(raw.APIKeyEncrypted), apiKey,
		"api key must be encrypted at rest, not stored as plaintext")

	// GetCredential decrypts it back.
	cred, ok, err := repo.GetCredential(ctx, user.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "openai", cred.Provider)
	assert.Equal(t, apiKey, cred.APIKey)
	assert.Equal(t, "gpt-4o-mini", cred.Model)

	// Re-registering overwrites in place (one credential per user).
	require.NoError(t, repo.SetCredential(ctx, user.ID, "openai", "sk-rotated-key", ""))
	var count int
	require.NoError(t, db.Get(&count, `SELECT COUNT(*) FROM user_llm_credentials WHERE user_id = $1`, user.ID))
	assert.Equal(t, 1, count, "re-registering must not create a second row")
	rotated, ok, err := repo.GetCredential(ctx, user.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "sk-rotated-key", rotated.APIKey)
	assert.Equal(t, "", rotated.Model)

	// An unknown provider is rejected.
	require.Error(t, repo.SetCredential(ctx, user.ID, "gemini", "sk-x", ""))

	// Delete clears it and is idempotent.
	require.NoError(t, repo.DeleteCredential(ctx, user.ID))
	_, ok, err = repo.GetCredential(ctx, user.ID)
	require.NoError(t, err)
	assert.False(t, ok)
	require.NoError(t, repo.DeleteCredential(ctx, user.ID), "deleting a missing credential is a no-op")
}
