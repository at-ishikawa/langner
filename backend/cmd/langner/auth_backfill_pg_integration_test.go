package main

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/bootstrap"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/datasync"
	"github.com/at-ishikawa/langner/schemas"
)

// TestAuthBackfill_ClaimsUnownedHistoryToRealAccount_LivePostgres_Integration
// pins the per-user cutover claim, INCLUDING the bug it must not have: a real
// Google sign-in keys on Google's own sub, but tooling (`auth provision` /
// `issue-test-cookie` / a mistaken `--owner-email` run) mints a distinct
// `e2e-test|…` account. So history can end up stuck on a tooling account the
// owner never signs in as. `auth backfill --user-id <real id>` (claimHistory)
// must reclaim BOTH user_id-NULL rows AND tooling-owned rows to the real
// account, while never touching a DIFFERENT real user's rows.
//
// Requires LANGNER_INTEGRATION_DB_URL; DROPs/recreates the public schema, so it
// must run isolated (own step or -p 1).
func TestAuthBackfill_ClaimsUnownedHistoryToRealAccount_LivePostgres_Integration(t *testing.T) {
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}

	root := findRepoRoot(t)
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())

	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	loader, err := config.NewConfigLoader("config.example.yml")
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	ctx := context.Background()
	importer := newImporterFromConfig(cfg, db, io.Discard)
	_, err = importer.ImportAll(ctx, datasync.ImportOptions{}) // leaves history with user_id = NULL (pre-auth)
	require.NoError(t, err)

	// Accounts: the owner's REAL Google account, a TOOLING account (as a mistaken
	// --owner-email run would create), and a SECOND real account that must be
	// left alone.
	realOwner := seedIntegrationUser(t, db, "google|owner-real-sub")
	tooling := seedIntegrationUser(t, db, "e2e-test|old-owner@example.com")
	otherReal := seedIntegrationUser(t, db, "google|someone-else-sub")

	// Three linked notes to attach owned rows to.
	var notes []candidateNote
	require.NoError(t, db.SelectContext(ctx, &notes, `
		SELECT n.id, n."usage", n.entry, n.sense_id, nn.notebook_id
		FROM notes n JOIN notebook_notes nn ON nn.note_id = n.id
		ORDER BY n.id LIMIT 3`))
	require.Len(t, notes, 3)
	stuck, owned := notes[0], notes[1]

	insertLog := func(note candidateNote, userID interface{}, status string) {
		_, ierr := db.ExecContext(ctx, `
			INSERT INTO learning_logs (note_id, user_id, status, learned_at, quality, response_time_ms, quiz_type, interval_days, source_notebook_id)
			VALUES ($1,$2,$3, now(), 4, 1000, 'notebook', 7, $4)`, note.ID, userID, status, note.NotebookID)
		require.NoError(t, ierr)
	}
	// The bug's aftermath: real history stuck on the tooling account...
	insertLog(stuck, tooling, "understood")
	// ...a second real user's history that must NEVER be reassigned...
	insertLog(owned, otherReal, "misunderstood")
	// ...plus a pre-auth NULL skip flag and a tooling-owned origin skip flag.
	_, err = db.ExecContext(ctx, `INSERT INTO note_skip_flags (note_id, user_id, quiz_type, skipped_at) VALUES ($1, NULL, 'notebook', now())`, stuck.ID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO origin_skip_flags (origin_id, user_id, quiz_type, skipped_at) SELECT id, $1, 'etymology_origin', now() FROM etymology_origins LIMIT 1`, tooling)
	require.NoError(t, err)

	// Count what SHOULD move: every NULL row (import + the seeded skip flag) plus
	// every tooling-owned row (the stuck log + the tooling skip flag).
	unowned := func() int64 {
		var n int64
		require.NoError(t, db.GetContext(ctx, &n, `
			SELECT
			  (SELECT count(*) FROM learning_logs    WHERE user_id IS NULL OR user_id = $1)
			+ (SELECT count(*) FROM note_skip_flags   WHERE user_id IS NULL OR user_id = $1)
			+ (SELECT count(*) FROM origin_skip_flags WHERE user_id IS NULL OR user_id = $1)`, tooling))
		return n
	}
	before := unowned()
	require.Greater(t, before, int64(2), "pre-auth NULL history + tooling-stuck rows exist")

	// The fix: claim reclaims NULL + tooling-owned rows to the real account.
	n, err := claimHistory(ctx, db, realOwner)
	require.NoError(t, err)
	assert.Equal(t, before, n, "claim reassigns exactly the unowned (NULL + tooling) rows")

	// The stuck log is now the real owner's — the exact bug this fixes.
	var stuckOwner int64
	require.NoError(t, db.GetContext(ctx, &stuckOwner, `SELECT user_id FROM learning_logs WHERE note_id = $1`, stuck.ID))
	assert.Equal(t, realOwner, stuckOwner, "history stuck on a tooling account is reclaimed to the real account")

	// The other real user's row is untouched.
	var ownedOwner int64
	require.NoError(t, db.GetContext(ctx, &ownedOwner, `SELECT user_id FROM learning_logs WHERE note_id = $1`, owned.ID))
	assert.Equal(t, otherReal, ownedOwner, "a different real user's history is never reassigned")

	// Nothing unowned remains, and re-running is a no-op.
	assert.EqualValues(t, 0, unowned(), "no NULL or tooling-owned rows remain")
	n2, err := claimHistory(ctx, db, realOwner)
	require.NoError(t, err)
	assert.EqualValues(t, 0, n2, "re-running the claim moves nothing")

	// Observable: the owner's scoped read now surfaces the reclaimed history.
	repos := bootstrap.BuildStateRepositories(cfg.Notebooks, cfg.Quiz, db)
	require.NotNil(t, repos.HistoryStore)
	histories, err := repos.HistoryStore.LoadForNotebooks(ctx, []string{stuck.NotebookID}, realOwner)
	require.NoError(t, err)
	_, ok := findExpressionInHistories(histories[stuck.NotebookID], stuck.Usage, stuck.Entry)
	assert.True(t, ok, "owner's scoped read surfaces the reclaimed history")
}
