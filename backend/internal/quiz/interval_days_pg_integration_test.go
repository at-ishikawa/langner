package quiz

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/schemas"
)

// TestInterval_LivePostgres_Integration proves the reported bug is fixed
// against a real Postgres, wired the way langner-server runs in DB-only mode
// (DBLearningRepository write + DBHistoryStore read), and built from the
// example notebooks through the real config/Service construction.
//
// It reproduces the failure — a correct answer used to land in learning_logs
// with interval_days=0 — and asserts:
//  1. A first correct answer stores a COMPUTED interval (ladder[1]=7), not 0,
//     for both the forward (SaveResult) and reverse (SaveReverseResult) paths.
//  2. A second correct answer READS the prior log back from the DB and advances
//     the interval (7 → ladder[2]=30) — which can only happen if the prior is
//     resolved through the DB history store (learning-history invariant L2).
//     Without the fix the stored interval would be 0 and no advance possible.
//
// Requires LANGNER_INTEGRATION_DB_URL (the postgres:16 CI service); skipped
// otherwise so local runs stay fast.
func TestInterval_LivePostgres_Integration(t *testing.T) {
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

	learningDir := t.TempDir()
	cfg := exampleNotebooksConfig(t, learningDir)

	// Production-like DB-only wiring (mirrors bootstrap.BuildStateRepositories).
	dbLearningRepo := learning.NewDBLearningRepository(db)
	dbNoteRepo := notebook.NewDBNoteRepository(db)
	dbSkipFlagRepo := notebook.NewDBSkipFlagRepository(db)
	dbOriginRepo := notebook.NewDBEtymologyOriginRepository(db)
	historyStore := learning.NewDBHistoryStore(
		dbNoteRepo, dbLearningRepo, dbOriginRepo, dbSkipFlagRepo,
		notebook.NewDBGrammarCorrectionRepository(db),
	)
	svc := NewService(cfg, nil, make(map[string]rapidapi.Response), dbLearningRepo,
		config.QuizConfig{Algorithm: "fixed", FixedIntervals: fixedLadder})
	svc.SetHistoryStore(historyStore)

	// Source a REAL card identity from the example notebooks through the real
	// loader — not a hand-built fixture.
	words, err := svc.LoadAllWords()
	require.NoError(t, err)
	var w FreeformCard
	for _, c := range words {
		if c.Expression != "" && c.ConceptHead == "" {
			w = c
			break
		}
	}
	require.NotEmpty(t, w.Expression, "expected at least one plain example word")

	ctx := context.Background()
	latestInterval := func(quizType string) int {
		var iv int
		require.NoError(t, db.Get(&iv, `
			SELECT interval_days FROM learning_logs
			WHERE quiz_type = $1
			ORDER BY learned_at DESC, id DESC
			LIMIT 1`, quizType))
		return iv
	}

	// --- Forward (SaveResult): first correct answer must store 7, not 0. ---
	card := Card{
		ID: w.ID, NotebookName: w.NotebookName,
		StoryTitle: w.StoryTitle, SceneTitle: w.SceneTitle,
		Entry: w.Expression, OriginalEntry: w.OriginalExpression,
	}
	require.NoError(t, svc.SaveResult(ctx, card, GradeResult{Correct: true, Quality: 4}, 1200))
	assert.Equal(t, 7, latestInterval("notebook"),
		"first correct forward answer must store ladder[1]=7, not the buggy 0")

	// --- Reverse (SaveReverseResult): first correct answer must store 7. ---
	rc := ReverseCard{
		ID: w.ID, NotebookName: w.NotebookName,
		StoryTitle: w.StoryTitle, SceneTitle: w.SceneTitle,
		Expression: w.Expression, AltForm: w.OriginalExpression,
	}
	require.NoError(t, svc.SaveReverseResult(ctx, rc, GradeResult{Correct: true, Quality: 4}, 1200))
	assert.Equal(t, 7, latestInterval("reverse"),
		"first correct reverse answer must store ladder[1]=7, not the buggy 0")

	// Backdate the reverse log so the next correct answer is past its interval
	// (defeating the early-review guard) and a genuine advance is observable.
	_, err = db.Exec(`UPDATE learning_logs SET learned_at = $1 WHERE quiz_type = 'reverse'`,
		time.Now().Add(-60*24*time.Hour))
	require.NoError(t, err)

	// --- Streak: second correct reverse answer reads the prior from the DB
	// and advances 7 → 30. If the prior weren't read (or were stored 0) this
	// would recompute to 7, so 30 proves the DB read + interval computation. ---
	require.NoError(t, svc.SaveReverseResult(ctx, rc, GradeResult{Correct: true, Quality: 4}, 1200))
	assert.Equal(t, 30, latestInterval("reverse"),
		"a second correct reverse answer must read the backdated prior from the DB and advance to ladder[2]=30")
}
