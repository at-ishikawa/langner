package main

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

// TestResolveNotebookOwner_RealAccountsOnly_LivePostgres_Integration pins that
// `notebooks set-owner --user-id` (via resolveNotebookOwner) only ever assigns a
// notebook's owner to a REAL Google account — never a tooling `e2e-test|…`
// account (which nobody signs in as, so a private notebook owned by it would be
// visible to no one) and never a nonexistent id. userID 0 means "unowned".
//
// Requires LANGNER_INTEGRATION_DB_URL; DROPs/recreates the public schema, so it
// must run isolated (own step or -p 1).
func TestResolveNotebookOwner_RealAccountsOnly_LivePostgres_Integration(t *testing.T) {
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}
	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())

	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))
	ctx := context.Background()

	real := seedIntegrationUser(t, db, "google|owner-real-sub")
	tooling := seedIntegrationUser(t, db, "e2e-test|tool@example.com")

	// 0 → unowned (valid for a public notebook), no error.
	owner, err := resolveNotebookOwner(ctx, db, 0)
	require.NoError(t, err)
	assert.Nil(t, owner)

	// A real Google account is accepted and returned as the owner.
	owner, err = resolveNotebookOwner(ctx, db, real)
	require.NoError(t, err)
	require.NotNil(t, owner)
	assert.Equal(t, real, *owner)

	// A tooling account is rejected — it must never own a production notebook.
	_, err = resolveNotebookOwner(ctx, db, tooling)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tooling account")

	// A nonexistent id is rejected.
	_, err = resolveNotebookOwner(ctx, db, 999999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no account with id")
}
