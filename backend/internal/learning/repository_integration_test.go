package learning

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/schemas"
)

// TestDBLearningRepository_Create_LivePostgres_Integration exercises
// the quiz-answer write path against a real Postgres — the exact
// scenario the sqlmock tests can't verify because sqlmock accepts any
// query pattern regardless of the DB's actual constraint semantics.
//
// The test reproduces the bug the user hit in production: answer a
// quiz for an expression whose note already exists in the DB (which
// is what happens once sync-db has imported the source data). Under
// the old SELECT-then-INSERT ensureNoteExists, the second call would
// hit the (usage, entry) unique constraint. Under the new UPSERT it
// returns the existing note's id without an error.
//
// Requires LANGNER_INTEGRATION_DB_URL pointing at a writable
// throwaway Postgres. Skipped otherwise so local test runs stay
// fast. CI wires this to the postgres:16 service container that
// validate-examples already spins up.
func TestDBLearningRepository_Create_LivePostgres_Integration(t *testing.T) {
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.Ping())

	// Fresh schema so the test doesn't depend on ordering with other
	// tests or leftover state.
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	repo := NewDBLearningRepository(db)
	baseTime := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	// First quiz answer for "cardiology" — note doesn't exist yet.
	logA := &LearningLog{
		Expression: "cardiology", OriginalExpression: "",
		Status: "understood", LearnedAt: baseTime, Quality: 4, ResponseTimeMs: 1500,
		QuizType: "notebook", IntervalDays: 7, SourceNotebookID: "wpme",
	}
	require.NoError(t, repo.Create(context.Background(), logA))

	var noteCount int
	require.NoError(t, db.Get(&noteCount, `SELECT COUNT(*) FROM notes WHERE "usage" = 'cardiology' AND entry = 'cardiology'`))
	assert.Equal(t, 1, noteCount, "first answer must create exactly one note row")

	// Second quiz answer for the same expression — note NOW exists.
	// This is the code path the user's error was on. Old behavior:
	// SELECT-then-INSERT raced or missed and violated the unique
	// constraint. New behavior: UPSERT returns the existing id and
	// the log lands cleanly.
	logB := &LearningLog{
		Expression: "cardiology", OriginalExpression: "",
		Status: "misunderstood", LearnedAt: baseTime.Add(1 * time.Hour), Quality: 1, ResponseTimeMs: 3000,
		QuizType: "notebook", IntervalDays: 1, SourceNotebookID: "wpme",
	}
	require.NoError(t, repo.Create(context.Background(), logB),
		"second answer for existing note must NOT violate the (usage, entry) unique constraint")

	require.NoError(t, db.Get(&noteCount, `SELECT COUNT(*) FROM notes WHERE "usage" = 'cardiology' AND entry = 'cardiology'`))
	assert.Equal(t, 1, noteCount, "second answer must reuse the existing note, not create a second row")

	// Both logs must be linked to the same note.
	var logCount int
	require.NoError(t, db.Get(&logCount, `
		SELECT COUNT(*) FROM learning_logs ll
		JOIN notes n ON n.id = ll.note_id
		WHERE n."usage" = 'cardiology' AND n.entry = 'cardiology'`))
	assert.Equal(t, 2, logCount, "both quiz answers must be persisted against the shared note")

	// Different Expression + non-empty OriginalExpression — the
	// definitions-book variant where card.Entry is the Definition
	// and card.OriginalEntry is the Expression. ensureNoteExists
	// uses (Expression, OriginalExpression) as (usage, entry), and
	// must handle that shape without error too.
	logC := &LearningLog{
		Expression: "cardiologist", OriginalExpression: "cardiologists",
		Status: "understood", LearnedAt: baseTime.Add(2 * time.Hour), Quality: 4, ResponseTimeMs: 1200,
		QuizType: "notebook", IntervalDays: 3, SourceNotebookID: "wpme",
	}
	require.NoError(t, repo.Create(context.Background(), logC),
		"definitions-book card (Expression != OriginalExpression) must also upsert cleanly")

	require.NoError(t, db.Get(&noteCount, `SELECT COUNT(*) FROM notes WHERE "usage" = 'cardiologist' AND entry = 'cardiologists'`))
	assert.Equal(t, 1, noteCount, "definitions-book card creates its own note row keyed by (Expression, OriginalExpression)")
}
