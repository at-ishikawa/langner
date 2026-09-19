package main

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/bootstrap"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/datasync"
	"github.com/at-ishikawa/langner/schemas"
)

// TestAuthBackfill_AssignsPreAuthHistoryToOwner_LivePostgres_Integration pins the
// auth-Phase-2 cutover step that turns a single-user app's accumulated history
// into an owned, per-user history: `auth backfill` / backfillOwner assigns every
// pre-auth (user_id IS NULL) row to one owner, so after switching auth on the
// owner still sees all their words. It also proves the safety guarantees: an
// already-attributed row is never reassigned, the op is idempotent, and the
// owner's scoped read surfaces the backfilled history.
//
// Requires LANGNER_INTEGRATION_DB_URL; DROPs/recreates the public schema, so it
// must run isolated (own step or -p 1).
func TestAuthBackfill_AssignsPreAuthHistoryToOwner_LivePostgres_Integration(t *testing.T) {
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
	_, err = importer.ImportAll(ctx, datasync.ImportOptions{})
	require.NoError(t, err)

	// Two notebook-linked notes: `preAuth` gets a legacy user_id=NULL log (the
	// history that existed before auth), `owned` gets a log already attributed to
	// another user (must NOT be reassigned by the backfill).
	var notes []candidateNote
	require.NoError(t, db.SelectContext(ctx, &notes, `
		SELECT n.id, n."usage", n.entry, n.sense_id, nn.notebook_id
		FROM notes n
		JOIN notebook_notes nn ON nn.note_id = n.id
		ORDER BY n.id
		LIMIT 2`))
	require.Len(t, notes, 2, "need two linked notes to seed pre-auth + owned history")
	preAuth, owned := notes[0], notes[1]

	insertLog := func(note candidateNote, userID interface{}, status string) {
		_, ierr := db.ExecContext(ctx, `
			INSERT INTO learning_logs (note_id, user_id, status, learned_at, quality, response_time_ms, quiz_type, interval_days, source_notebook_id)
			VALUES ($1, $2, $3, now(), 4, 1000, 'notebook', 7, $4)`,
			note.ID, userID, status, note.NotebookID)
		require.NoError(t, ierr)
	}

	other := seedIntegrationUser(t, db, "backfill-other-user")
	insertLog(preAuth, nil, "understood")   // pre-auth: user_id NULL
	insertLog(owned, other, "misunderstood") // already owned by another user
	// A pre-auth skip flag too, so the backfill's coverage of the skip tables is exercised.
	_, err = db.ExecContext(ctx,
		`INSERT INTO note_skip_flags (note_id, user_id, quiz_type, skipped_at) VALUES ($1, NULL, 'notebook', now())`, preAuth.ID)
	require.NoError(t, err)

	// Owner is find-or-created by email — no config, no prior sign-in required.
	ownerID, err := ensureUser(ctx, auth.NewUserRepository(db), "owner@example.com")
	require.NoError(t, err)
	require.NotEqual(t, other, ownerID)

	// Count every pre-auth (user_id IS NULL) row across the three per-user tables
	// — the import itself seeds history with no owner, plus the log + skip flag
	// seeded above. Backfill must reassign exactly that set.
	nullRows := func() int64 {
		var n int64
		require.NoError(t, db.GetContext(ctx, &n, `
			SELECT (SELECT count(*) FROM learning_logs   WHERE user_id IS NULL)
			     + (SELECT count(*) FROM note_skip_flags  WHERE user_id IS NULL)
			     + (SELECT count(*) FROM origin_skip_flags WHERE user_id IS NULL)`))
		return n
	}
	before := nullRows()
	require.Greater(t, before, int64(2), "pre-auth NULL history exists (import + seeded rows)")

	// Backfill: every pre-auth (NULL) row is reassigned to the owner.
	n, err := backfillOwner(ctx, db, ownerID)
	require.NoError(t, err)
	assert.Equal(t, before, n, "backfill reassigns exactly the pre-auth (NULL) rows")
	assert.EqualValues(t, 0, nullRows(), "no pre-auth (NULL) rows remain after backfill")

	// The pre-auth log now belongs to the owner...
	var preAuthOwner int64
	require.NoError(t, db.GetContext(ctx, &preAuthOwner,
		`SELECT user_id FROM learning_logs WHERE note_id = $1`, preAuth.ID))
	assert.Equal(t, ownerID, preAuthOwner, "pre-auth log is now the owner's")

	// ...and the already-owned log is untouched (no clobber).
	var ownedOwner int64
	require.NoError(t, db.GetContext(ctx, &ownedOwner,
		`SELECT user_id FROM learning_logs WHERE note_id = $1`, owned.ID))
	assert.Equal(t, other, ownedOwner, "an already-attributed row must never be reassigned")

	// Idempotent: a second run touches nothing.
	n2, err := backfillOwner(ctx, db, ownerID)
	require.NoError(t, err)
	assert.EqualValues(t, 0, n2, "re-running backfill reassigns no rows")

	// Observable: the owner's scoped read now surfaces the backfilled word, so
	// history that existed before auth is visible again once signed in.
	repos := bootstrap.BuildStateRepositories(cfg.Notebooks, cfg.Quiz, db)
	require.NotNil(t, repos.HistoryStore)
	histories, err := repos.HistoryStore.LoadForNotebooks(ctx, []string{preAuth.NotebookID}, ownerID)
	require.NoError(t, err)
	_, ok := findExpressionInHistories(histories[preAuth.NotebookID], preAuth.Usage, preAuth.Entry)
	assert.True(t, ok, "owner's scoped read surfaces the backfilled pre-auth history")
}
