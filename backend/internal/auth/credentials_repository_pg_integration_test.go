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

// TestCredentialsRepository_LivePostgres_Integration exercises the per-user,
// per-provider LLM credential path against a real Postgres: encryption at rest,
// multiple providers per user, primary selection, and delete-with-promotion.
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

	users := NewUserRepository(db)
	ctx := context.Background()
	user, err := users.Upsert(ctx, "google-sub-cred")
	require.NoError(t, err)

	repo := NewCredentialsRepository(db, enc)

	// No credential yet.
	metas, err := repo.ListCredentials(ctx, user.ID)
	require.NoError(t, err)
	assert.Empty(t, metas, "a fresh user has no credentials")

	// Register the first provider; it becomes primary, encrypted at rest.
	const openaiKey = "sk-super-secret-key-12345"
	require.NoError(t, repo.SetCredential(ctx, user.ID, "openai", openaiKey, "gpt-4o-mini", false))

	var raw llmCredentialRow
	require.NoError(t, db.Get(&raw,
		`SELECT provider, api_key_encrypted, model, is_primary FROM user_llm_credentials WHERE user_id = $1`, user.ID))
	assert.NotContains(t, string(raw.APIKeyEncrypted), openaiKey, "api key must be encrypted at rest")

	metas, err = repo.ListCredentials(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, metas, 1)
	assert.Equal(t, "openai", metas[0].Provider)
	assert.True(t, metas[0].IsPrimary, "the first credential is the primary")

	// Register a SECOND provider (not primary) — the first stays primary.
	require.NoError(t, repo.SetCredential(ctx, user.ID, "gemini", "gm-secret", "", false))
	metas, err = repo.ListCredentials(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, metas, 2)
	assert.Equal(t, "openai", metas[0].Provider, "primary listed first")
	assert.True(t, metas[0].IsPrimary)
	assert.False(t, metas[1].IsPrimary)

	// The resolver reads decrypted creds primary-first with both keys intact.
	creds, err := repo.DecryptedCredentials(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, creds, 2)
	assert.True(t, creds[0].IsPrimary)
	assert.Equal(t, openaiKey, creds[0].APIKey)
	assert.Equal(t, "gm-secret", creds[1].APIKey)

	// Switch the primary to gemini (no key re-entry).
	require.NoError(t, repo.SetPrimaryProvider(ctx, user.ID, "gemini"))
	metas, err = repo.ListCredentials(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, metas, 2)
	assert.Equal(t, "gemini", metas[0].Provider)
	assert.True(t, metas[0].IsPrimary)
	assert.Equal(t, "openai", metas[1].Provider)
	assert.False(t, metas[1].IsPrimary)

	// Rotating openai's key doesn't add a row or change the primary.
	require.NoError(t, repo.SetCredential(ctx, user.ID, "openai", "sk-rotated", "", false))
	creds, err = repo.DecryptedCredentials(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, creds, 2)
	for _, c := range creds {
		if c.Provider == "openai" {
			assert.Equal(t, "sk-rotated", c.APIKey)
			assert.False(t, c.IsPrimary)
		}
	}

	// An unknown provider is rejected.
	require.Error(t, repo.SetCredential(ctx, user.ID, "anthropic", "x", "", false))

	// Deleting the primary (gemini) promotes the remaining provider (openai).
	require.NoError(t, repo.DeleteCredential(ctx, user.ID, "gemini"))
	metas, err = repo.ListCredentials(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, metas, 1)
	assert.Equal(t, "openai", metas[0].Provider)
	assert.True(t, metas[0].IsPrimary, "the surviving provider is promoted to primary")

	// Delete the last one; idempotent.
	require.NoError(t, repo.DeleteCredential(ctx, user.ID, "openai"))
	metas, err = repo.ListCredentials(ctx, user.ID)
	require.NoError(t, err)
	assert.Empty(t, metas)
	require.NoError(t, repo.DeleteCredential(ctx, user.ID, "openai"), "deleting a missing credential is a no-op")
}
