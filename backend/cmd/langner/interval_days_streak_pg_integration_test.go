package main

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/bootstrap"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/datasync"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/schemas"
)

// TestIntervalDaysStreak_LivePostgres_Integration proves the reported bug is
// fixed AND that streaks advance, against a live Postgres wired exactly as
// langner-server runs in DB mode (import via the real ImportAll, read via
// DBHistoryStore). It reproduces the failure — a correct answer landed in
// learning_logs with interval_days=0 — and asserts:
//
//  1. A first correct answer stores a COMPUTED interval (fixed-ladder[1]=7),
//     not 0, for both reverse (SaveReverseResult) and forward (SaveResult).
//  2. A second correct answer READS the (backdated) prior log back from the DB
//     via the same loadHistories/DBHistoryStore path the loaders use and
//     ADVANCES the interval (7 -> ladder[2]=30). Without reading the prior it
//     would recompute to 7 again — that regression (empty priors) is exactly
//     what a note with no notebook_notes link would cause, so importing the
//     notebook (as production does) is load-bearing here.
//
// The interval computation uses the Service's own calculator, so the Service is
// built with the fixed ladder for deterministic values; import (which only
// copies data) still runs from config.example.yml unchanged.
//
// Requires LANGNER_INTEGRATION_DB_URL (CI's postgres:16); skipped otherwise.
// DROPs and recreates the public schema, so it must run isolated (own step /
// -p 1), matching its sibling cmd/langner live-PG tests.
func TestIntervalDaysStreak_LivePostgres_Integration(t *testing.T) {
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

	// Import the example notebooks the SAME way import-db does, so every note
	// gets its notebook_notes link and DBHistoryStore can reconstruct it.
	importer := newImporterFromConfig(cfg, db, io.Discard)
	_, err = importer.ImportAll(context.Background(), datasync.ImportOptions{})
	require.NoError(t, err)

	repos := bootstrap.BuildStateRepositories(cfg.Notebooks, cfg.Quiz, db)
	require.NotNil(t, repos.HistoryStore)
	// Fixed-ladder Service so the computed intervals are deterministic (7, 30).
	fixedLadder := []int{1, 7, 30, 90, 365, 1095, 1825}
	svc := quiz.NewService(cfg.Notebooks, nil, nil, repos.Learning,
		config.QuizConfig{Algorithm: "fixed", FixedIntervals: fixedLadder})
	svc.SetHistoryStore(repos.HistoryStore)

	const bookID = "reverse-progress-demo"
	ctx := context.Background()

	noteID := func(usage string) int64 {
		var id int64
		require.NoError(t, db.GetContext(ctx, &id,
			`SELECT id FROM notes WHERE "usage" = $1 AND sense_id = ''`, usage))
		return id
	}
	latestInterval := func(id int64, quizType string) int {
		var iv int
		require.NoError(t, db.GetContext(ctx, &iv, `
			SELECT interval_days FROM learning_logs
			WHERE note_id = $1 AND quiz_type = $2
			ORDER BY learned_at DESC, id DESC LIMIT 1`, id, quizType))
		return iv
	}
	// Backdate every log for (note, quiz_type) far enough that the next correct
	// answer is past its interval — so the early-review guard doesn't suppress
	// the advance and a genuine streak step is observable.
	backdate := func(id int64, quizType string) {
		_, uerr := db.ExecContext(ctx,
			`UPDATE learning_logs SET learned_at = $1 WHERE note_id = $2 AND quiz_type = $3`,
			time.Now().Add(-60*24*time.Hour), id, quizType)
		require.NoError(t, uerr)
	}

	// ---- Reverse streak (cardiovascular): first correct -> 7, then -> 30. ----
	revCards, err := svc.LoadReverseCards([]string{bookID}, false, true, nil)
	require.NoError(t, err)
	var revCard quiz.ReverseCard
	for _, c := range revCards {
		if c.AltForm == "cardiovascular" || c.Expression == "cardiovascular" {
			revCard = c
			break
		}
	}
	require.Equal(t, "cardiovascular", revCard.AltForm, "reverse demo word must be due")

	cvID := noteID("cardiovascular")
	require.NoError(t, svc.SaveReverseResult(ctx, revCard, quiz.GradeResult{Correct: true, Quality: 4}, 1000))
	assert.Equal(t, 7, latestInterval(cvID, "reverse"),
		"first correct reverse answer must store ladder[1]=7, not the buggy 0")

	backdate(cvID, "reverse")
	require.NoError(t, svc.SaveReverseResult(ctx, revCard, quiz.GradeResult{Correct: true, Quality: 4}, 1000))
	assert.Equal(t, 30, latestInterval(cvID, "reverse"),
		"second correct reverse answer must read the backdated prior from the DB and advance to ladder[2]=30")

	// ---- Forward streak (renal): first correct -> 7, then -> 30. ----
	fwdCards, err := svc.LoadCards([]string{bookID}, true, nil)
	require.NoError(t, err)
	var fwdCard quiz.Card
	for _, c := range fwdCards {
		if c.OriginalEntry == "renal" || c.Entry == "renal" {
			fwdCard = c
			break
		}
	}
	require.Equal(t, "renal", fwdCard.OriginalEntry, "forward demo word must be due")

	renalID := noteID("renal")
	require.NoError(t, svc.SaveResult(ctx, fwdCard, quiz.GradeResult{Correct: true, Quality: 4}, 1000))
	assert.Equal(t, 7, latestInterval(renalID, "notebook"),
		"first correct forward answer must store ladder[1]=7, not the buggy 0")

	backdate(renalID, "notebook")
	require.NoError(t, svc.SaveResult(ctx, fwdCard, quiz.GradeResult{Correct: true, Quality: 4}, 1000))
	assert.Equal(t, 30, latestInterval(renalID, "notebook"),
		"second correct forward answer must read the backdated prior from the DB and advance to ladder[2]=30")
}
