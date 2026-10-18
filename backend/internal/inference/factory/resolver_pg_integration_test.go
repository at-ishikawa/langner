package factory

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/inference/fallback"
	"github.com/at-ishikawa/langner/schemas"
)

// TestResolver_LivePostgres_Integration proves the per-user resolver builds the
// right client from stored credentials: none → ErrNoCredential; one provider →
// that provider's client; two providers → a rate-limit/quota fallback wrapper
// (the primary first, the other as fallback). Postgres-only; skipped without
// LANGNER_INTEGRATION_DB_URL (CI wires the postgres service).
func TestResolver_LivePostgres_Integration(t *testing.T) {
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

	enc, err := auth.NewEncryptor([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	ctx := context.Background()
	user, err := auth.NewUserRepository(db).Upsert(ctx, "resolver-sub")
	require.NoError(t, err)

	repo := auth.NewCredentialsRepository(db, enc)
	resolver := NewResolver(repo)

	// No credential → ErrNoCredential.
	_, err = resolver.ResolveClient(ctx, user.ID)
	require.True(t, errors.Is(err, inference.ErrNoCredential), "no credential must resolve to ErrNoCredential, got %v", err)

	// One provider → a concrete client, NOT a fallback wrapper. Invalidate after
	// the credential change to mirror the production contract: the LLMCredential
	// handler calls resolver.Invalidate on every add/rotate/delete so the client
	// cache never serves a client built from stale credentials. (This test drives
	// repo.SetCredential directly, bypassing that handler, so it invalidates here.)
	require.NoError(t, repo.SetCredential(ctx, user.ID, "openai", "sk-primary", "", false))
	resolver.Invalidate(user.ID)
	single, err := resolver.ResolveClient(ctx, user.ID)
	require.NoError(t, err)
	_, isFallback := single.(*fallback.Client)
	assert.False(t, isFallback, "a single provider must not be wrapped in a fallback")

	// A second provider → fallback wrapper (primary + the other as fallback).
	require.NoError(t, repo.SetCredential(ctx, user.ID, "gemini", "gm-secondary", "", false))
	resolver.Invalidate(user.ID)
	paired, err := resolver.ResolveClient(ctx, user.ID)
	require.NoError(t, err)
	_, isFallback = paired.(*fallback.Client)
	assert.True(t, isFallback, "two providers must resolve to a fallback wrapper")
}
