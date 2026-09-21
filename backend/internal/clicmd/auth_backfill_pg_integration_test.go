package clicmd

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
// `issue-test-token` / a mistaken `--owner-email` run) mints a distinct
// `e2e-test|…` account. So history can end up stuck on a tooling account the
// owner never signs in as. `auth backfill --user-id <real id>` (claimHistory)
// must reclaim tooling-owned rows to the real account, while never touching a
// DIFFERENT real user's rows. (Since migration 028 made user_id NOT NULL there
// are no pre-auth NULL rows to reclaim any more; claimHistory keeps its NULL
// branch as a harmless no-op and the live job is the tooling reclaim.)
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

	// Accounts: the owner's REAL Google account, a TOOLING account (as an e2e
	// seed / mistaken --owner-email run would create), a SECOND real account that
	// must be left alone, and a throwaway owner for the baseline import.
	realOwner := seedIntegrationUser(t, db, "google|owner-real-sub")
	tooling := seedIntegrationUser(t, db, "e2e-test|old-owner@example.com")
	otherReal := seedIntegrationUser(t, db, "google|someone-else-sub")
	importOwner := seedIntegrationUser(t, db, "google|import-owner-sub")

	importer := newImporterFromConfig(cfg, db, io.Discard)
	// Baseline history, attributed to a real owner (user_id NOT NULL, migration
	// 028) — NOT tooling, so the claim below leaves it alone.
	_, err = importer.ImportAll(ctx, datasync.ImportOptions{OwnerID: importOwner})
	require.NoError(t, err)

	// Two never-quizzed notes (no imported logs), so each carries exactly the one
	// controlled row we attach.
	var notes []candidateNote
	require.NoError(t, db.SelectContext(ctx, &notes, `
		SELECT n.id, n."usage", n.entry, n.sense_id, nn.notebook_id
		FROM notes n
		JOIN notebook_notes nn ON nn.note_id = n.id
		LEFT JOIN learning_logs ll ON ll.note_id = n.id
		WHERE ll.id IS NULL
		ORDER BY n.id LIMIT 2`))
	require.Len(t, notes, 2)
	stuck, owned := notes[0], notes[1]

	insertLog := func(note candidateNote, userID int64, status string) {
		_, ierr := db.ExecContext(ctx, `
			INSERT INTO learning_logs (note_id, user_id, status, learned_at, quality, response_time_ms, quiz_type, interval_days, source_notebook_id)
			VALUES ($1,$2,$3, now(), 4, 1000, 'notebook', 7, $4)`, note.ID, userID, status, note.NotebookID)
		require.NoError(t, ierr)
	}
	// The bug's aftermath: real history stuck on the tooling account (e.g. an
	// import/seed that ran under the e2e/tooling account before the owner ever
	// signed in), which the owner never signs in as...
	insertLog(stuck, tooling, "understood")
	// ...a second real user's history that must NEVER be reassigned...
	insertLog(owned, otherReal, "misunderstood")
	// ...plus a tooling-owned skip flag on each table. user_id is NOT NULL now,
	// so there is no pre-auth NULL row: the claim's job is to reclaim
	// TOOLING-stuck rows to the real account.
	_, err = db.ExecContext(ctx, `INSERT INTO note_skip_flags (note_id, user_id, quiz_type, skipped_at) VALUES ($1, $2, 'notebook', now())`, stuck.ID, tooling)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO origin_skip_flags (origin_id, user_id, quiz_type, skipped_at) SELECT id, $1, 'etymology_origin', now() FROM etymology_origins LIMIT 1`, tooling)
	require.NoError(t, err)

	// Count what SHOULD move: every tooling-owned row (the stuck log + both skip
	// flags). NULL rows no longer exist (user_id NOT NULL).
	unowned := func() int64 {
		var n int64
		require.NoError(t, db.GetContext(ctx, &n, `
			SELECT
			  (SELECT count(*) FROM learning_logs    WHERE user_id = $1)
			+ (SELECT count(*) FROM note_skip_flags   WHERE user_id = $1)
			+ (SELECT count(*) FROM origin_skip_flags WHERE user_id = $1)`, tooling))
		return n
	}
	before := unowned()
	require.Greater(t, before, int64(2), "tooling-stuck rows exist")

	// The fix: claim reclaims tooling-owned rows to the real account.
	n, err := claimHistory(ctx, db, realOwner)
	require.NoError(t, err)
	assert.Equal(t, before, n, "claim reassigns exactly the tooling-owned rows")

	// The stuck log is now the real owner's — the exact bug this fixes.
	var stuckOwner int64
	require.NoError(t, db.GetContext(ctx, &stuckOwner, `SELECT user_id FROM learning_logs WHERE note_id = $1`, stuck.ID))
	assert.Equal(t, realOwner, stuckOwner, "history stuck on a tooling account is reclaimed to the real account")

	// The other real user's row is untouched.
	var ownedOwner int64
	require.NoError(t, db.GetContext(ctx, &ownedOwner, `SELECT user_id FROM learning_logs WHERE note_id = $1`, owned.ID))
	assert.Equal(t, otherReal, ownedOwner, "a different real user's history is never reassigned")

	// Nothing tooling-owned remains, and re-running is a no-op.
	assert.EqualValues(t, 0, unowned(), "no tooling-owned rows remain")
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
