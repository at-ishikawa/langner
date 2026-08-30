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
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/schemas"
)

// TestPerUserHistoryIsolation_LivePostgres_Integration is the auth-Phase-2 proof
// that learning history is per-user: two accounts answering the SAME shared note
// keep INDEPENDENT log series (one per (user, note, quiz mode) — the per-user
// extension of learning-history invariants L1/L4), and each user's read
// (LoadAll(ctx, userID)) surfaces only their own attempts.
//
// It drives the real Service wired exactly as langner-server wires it
// (bootstrap.BuildStateRepositories over config.example.yml's real notebooks),
// not a hand-built card — the mechanism verify-data-features-with-example-
// notebooks requires. Requires LANGNER_INTEGRATION_DB_URL; DROPs/recreates the
// public schema, so it must run isolated (own step or -p 1).
func TestPerUserHistoryIsolation_LivePostgres_Integration(t *testing.T) {
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

	importer := newImporterFromConfig(cfg, db, io.Discard)
	_, err = importer.ImportAll(context.Background(), datasync.ImportOptions{})
	require.NoError(t, err)

	ctx := context.Background()

	// One clean, never-quizzed note that both users will answer.
	var target candidateNote
	require.NoError(t, db.GetContext(ctx, &target, `
		SELECT n.id, n."usage", n.entry, n.sense_id, nn.notebook_id
		FROM notes n
		JOIN notebook_notes nn ON nn.note_id = n.id
		LEFT JOIN learning_logs ll ON ll.note_id = n.id
		WHERE ll.id IS NULL
		ORDER BY n.id
		LIMIT 1`))

	repos := bootstrap.BuildStateRepositories(cfg.Notebooks, cfg.Quiz, db)
	require.NotNil(t, repos.HistoryStore, "DB configured -> reads served from DB")
	svc := quiz.NewService(cfg.Notebooks, nil, nil, repos.Learning, cfg.Quiz)
	svc.SetHistoryStore(repos.HistoryStore)

	userA := seedIntegrationUser(t, db, "isolation-user-a")
	userB := seedIntegrationUser(t, db, "isolation-user-b")
	require.NotEqual(t, userA, userB)

	card := quiz.Card{
		ID:           target.SenseID,
		NotebookName: target.NotebookID,
		Entry:        target.Usage,
	}
	if target.Entry != target.Usage {
		card.OriginalEntry = target.Entry
	}

	// User A answers CORRECTLY; user B answers WRONG — the SAME shared note.
	require.NoError(t, svc.SaveResult(ctx, userA, card, quiz.GradeResult{Correct: true, Quality: 4}, 1000))
	require.NoError(t, svc.SaveResult(ctx, userB, card, quiz.GradeResult{Correct: false, Quality: 1}, 1000))

	// L1/L4 at the row level: exactly one log per (user, note, quiz mode) — two
	// rows total on the shared note, one owned by each user, never merged.
	type ownedLog struct {
		UserID int64  `db:"user_id"`
		Status string `db:"status"`
	}
	var logs []ownedLog
	require.NoError(t, db.SelectContext(ctx, &logs, `
		SELECT user_id, status FROM learning_logs
		WHERE note_id = $1 AND quiz_type = 'notebook'
		ORDER BY user_id`, target.ID))
	require.Len(t, logs, 2, "one log per user on the shared note")

	byUser := map[int64]string{}
	for _, l := range logs {
		byUser[l.UserID] = l.Status
	}
	assert.Equal(t, "understood", byUser[userA], "user A's correct attempt is A's own row")
	assert.Equal(t, "misunderstood", byUser[userB], "user B's wrong attempt is B's own row")

	// The read side is symmetric and isolated: each user's LoadAll reconstructs
	// ONLY their own attempt for the shared note.
	statusFor := func(userID int64) string {
		histories, lerr := repos.HistoryStore.LoadAll(ctx, userID)
		require.NoError(t, lerr)
		expr, ok := findExpressionInHistories(histories[target.NotebookID], target.Usage, target.Entry)
		require.True(t, ok, "shared note must appear in the user's history for notebook %q", target.NotebookID)
		return string(expr.GetLatestStatus())
	}
	assert.Equal(t, string(notebook.LearnedStatusUnderstood), statusFor(userA),
		"user A sees only their own correct attempt")
	assert.Equal(t, string(notebook.LearnedStatusMisunderstood), statusFor(userB),
		"user B sees only their own wrong attempt")
}
