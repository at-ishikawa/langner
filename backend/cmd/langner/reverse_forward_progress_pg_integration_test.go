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
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/schemas"
)

// TestVocabularyProgress_LivePostgres_Integration reproduces (and guards) the
// user-reported bug: in DB mode a CORRECT vocabulary answer did not reduce the
// due pool on reload — they answered reverse words and the next session still
// showed all of them.
//
// Root cause (learning-history invariant L2): the reader/importer key a note by
// its canonical identity (usage = the WORD `expression`, entry = the
// `definition` when present, or sense_id). The save path instead resolved the
// note from the card's SHOWN surface (the definition text for a definitions
// entry). For an ID-LESS word whose definition differs from its word, that
// surface didn't match the imported note, so DBLearningRepository.ensureNoteExists
// forged a PHANTOM note (with no notebook_notes link — invisible to the reader),
// the attempt's log never reached the real word, and the recognition/reverse due
// count never dropped. Id-bearing words were unaffected (resolved by sense_id).
//
// The failing shape is exercised through the real example notebook
// examples/definitions/reverse-progress-demo (an id-less word `cardiovascular`
// for reverse and `renal` for forward, both carrying a distinct `definition:`),
// loaded through config.example.yml and imported via the SAME import-db path the
// server uses, then driven through the SAME LoadReverseCards/LoadCards +
// SaveReverseResult/SaveResult the RPC handlers use.
//
// It asserts BOTH the reverse and the forward paths (shared root cause). On the
// pre-fix code the log lands on a phantom (write note_id != imported note id) and
// the word stays due; after the fix the log lands on the imported note and the
// word drops.
//
// Requires LANGNER_INTEGRATION_DB_URL (CI's postgres:16); skipped otherwise.
// DROPs and recreates the public schema, so it must run isolated (own workflow
// step, or -p 1).
func TestVocabularyProgress_LivePostgres_Integration(t *testing.T) {
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

	// Build the Service exactly as langner-server does in DB mode.
	repos := bootstrap.BuildStateRepositories(cfg.Notebooks, cfg.Quiz, db)
	require.NotNil(t, repos.HistoryStore)
	svc := quiz.NewService(cfg.Notebooks, nil, nil, repos.Learning, cfg.Quiz)
	svc.SetHistoryStore(repos.HistoryStore)
	userID := seedIntegrationUser(t, db, "reverse-progress-user")

	const bookID = "reverse-progress-demo"
	ctx := context.Background()

	// ---------------------------------------------------------------------
	// REVERSE: the reported failing case (id-less + a distinct `definition:`).
	// ---------------------------------------------------------------------
	loadReverse := func() []quiz.ReverseCard {
		cards, lerr := svc.LoadReverseCards(userID, []string{bookID}, false, true, nil)
		require.NoError(t, lerr)
		return cards
	}
	// A definitions reverse card holds the definition surface in Expression and
	// the word in AltForm; match on either so we find the word regardless.
	findReverse := func(cards []quiz.ReverseCard, word string) (quiz.ReverseCard, bool) {
		for _, c := range cards {
			if c.AltForm == word || c.Expression == word {
				return c, true
			}
		}
		return quiz.ReverseCard{}, false
	}

	revCard, ok := findReverse(loadReverse(), "cardiovascular")
	require.True(t, ok, "the fresh id-less definition-bearing word must be due for reverse")

	require.NoError(t, svc.SaveReverseResult(ctx, userID, revCard, quiz.GradeResult{Correct: true, Quality: 4}, 1000))

	// Diagnostic: the reverse log must land on the IMPORTED note, not a phantom.
	var writeNoteID int64
	require.NoError(t, db.GetContext(ctx, &writeNoteID,
		`SELECT note_id FROM learning_logs WHERE quiz_type = 'reverse' ORDER BY id DESC LIMIT 1`))
	var importedNoteID int64
	require.NoError(t, db.GetContext(ctx, &importedNoteID,
		`SELECT id FROM notes WHERE "usage" = 'cardiovascular' AND sense_id = ''`),
		"the imported id-less note is keyed by (usage=word)")
	assert.Equal(t, importedNoteID, writeNoteID,
		"reverse write must target the imported note by its canonical word, not forge a phantom from the shown definition surface")
	// The definition surface must not have spawned a phantom note.
	var phantom int
	require.NoError(t, db.GetContext(ctx, &phantom,
		`SELECT COUNT(*) FROM notes WHERE "usage" = 'relating to the heart and blood vessels'`))
	assert.Equal(t, 0, phantom, "the shown definition surface must never become its own note")

	// Reload: the answered reverse word is no longer due (the reported symptom).
	_, stillDue := findReverse(loadReverse(), "cardiovascular")
	assert.False(t, stillDue, "a correctly-answered reverse word must drop from the reverse due pool on reload")

	// Controls: id-bearing (pulmonary) and id-less-without-definition (hepatic)
	// remain due here only because we did not answer them — sanity that we
	// dropped exactly the one word.
	_, pulmonaryDue := findReverse(loadReverse(), "pulmonary")
	assert.True(t, pulmonaryDue, "an unanswered word stays due")

	// ---------------------------------------------------------------------
	// FORWARD/standard: same shared root cause, a separate id-less word.
	// ---------------------------------------------------------------------
	loadForward := func() []quiz.Card {
		cards, lerr := svc.LoadCards(userID, []string{bookID}, true, nil)
		require.NoError(t, lerr)
		return cards
	}
	// A definitions forward card holds the definition surface in Entry and the
	// word in OriginalEntry.
	findForward := func(cards []quiz.Card, word string) (quiz.Card, bool) {
		for _, c := range cards {
			if c.OriginalEntry == word || c.Entry == word {
				return c, true
			}
		}
		return quiz.Card{}, false
	}

	fwdCard, ok := findForward(loadForward(), "renal")
	require.True(t, ok, "the fresh id-less definition-bearing word must be due for the standard quiz")

	require.NoError(t, svc.SaveResult(ctx, userID, fwdCard, quiz.GradeResult{Correct: true, Quality: 4}, 1000))

	var fwdWriteNoteID int64
	require.NoError(t, db.GetContext(ctx, &fwdWriteNoteID,
		`SELECT note_id FROM learning_logs WHERE quiz_type = 'notebook' ORDER BY id DESC LIMIT 1`))
	var renalNoteID int64
	require.NoError(t, db.GetContext(ctx, &renalNoteID,
		`SELECT id FROM notes WHERE "usage" = 'renal' AND sense_id = ''`))
	assert.Equal(t, renalNoteID, fwdWriteNoteID,
		"forward write must target the imported note by its canonical word too")

	_, fwdStillDue := findForward(loadForward(), "renal")
	assert.False(t, fwdStillDue, "a correctly-answered standard word must drop from the recognition due pool on reload")

	// ---------------------------------------------------------------------
	// ORDERING: a word with PRIOR imported reverse history must advance too.
	// pulmonary is id-bearing (resolved by sense_id, no phantom concern), so
	// this isolates the DB-log-ordering bug: a runtime attempt is INSERTed last
	// and gets the HIGHEST id, so without sorting the reconstructed ReverseLogs
	// newest-first the STALE imported miss would remain [0] and NeedsReverseReview
	// would keep the word due forever despite a correct answer.
	// ---------------------------------------------------------------------
	var pulmonaryNoteID int64
	require.NoError(t, db.GetContext(ctx, &pulmonaryNoteID,
		`SELECT id FROM notes WHERE sense_id = 'pulmonary-demo'`))
	// Seed a stale misunderstood reverse attempt (30 days ago) the way a prior
	// session's import would — it takes a LOWER id than the runtime answer below.
	_, err = db.ExecContext(ctx,
		`INSERT INTO learning_logs (note_id, status, learned_at, quality, response_time_ms, quiz_type, interval_days, source_notebook_id)
		 VALUES ($1, 'misunderstood', $2, 1, 0, 'reverse', 1, $3)`,
		pulmonaryNoteID, time.Now().AddDate(0, 0, -30), bookID)
	require.NoError(t, err)

	pulCard, ok := findReverse(loadReverse(), "pulmonary")
	require.True(t, ok, "a word whose latest reverse attempt is a miss must be due")

	require.NoError(t, svc.SaveReverseResult(ctx, userID, pulCard, quiz.GradeResult{Correct: true, Quality: 4}, 1000))

	// The reconstructed history must surface the just-written attempt as latest,
	// not the older imported miss (proves the newest-first ordering).
	hist, err := repos.HistoryStore.LoadAll(ctx, userID)
	require.NoError(t, err)
	pulExpr, found := findExpressionInHistories(hist[bookID], "pulmonary", "relating to the lungs")
	require.True(t, found)
	require.NotEmpty(t, pulExpr.ReverseLogs)
	assert.Equal(t, notebook.LearnedStatusUnderstood, pulExpr.ReverseLogs[0].Status,
		"the latest reverse attempt (the runtime correct answer) must be ReverseLogs[0], not the older imported miss")

	_, pulmonaryDueAfter := findReverse(loadReverse(), "pulmonary")
	assert.False(t, pulmonaryDueAfter,
		"a correct reverse answer must drop the word even when an older imported miss has a lower DB id (DB-log-ordering)")
}
