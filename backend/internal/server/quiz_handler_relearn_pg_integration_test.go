package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference/mock"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/schemas"
)

// TestRelearn_LivePostgres_Integration proves, against a live Postgres, the two
// load-bearing promises of the Relearn Quiz on the DB path:
//  1. A relearn answer writes NOTHING to learning_logs (SM-2 and analytics are
//     untouched).
//  2. Relearn persists no state of its own, so the practised word is repeatable:
//     it stays in the pool and reappears in the next session even after a
//     correct answer, until it ages out of the window or is fixed in a real quiz.
//
// Runs only when LANGNER_INTEGRATION_DB_URL is set (the postgres:16 service in
// CI).
func TestRelearn_LivePostgres_Integration(t *testing.T) {
	dsn := os.Getenv(integrationDBEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping live-Postgres relearn coverage", integrationDBEnv)
	}

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())

	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()
	vocabDir := filepath.Join(flashcardsDir, "test-vocab")
	require.NoError(t, os.MkdirAll(vocabDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "index.yml"), []byte(`id: test-vocab
name: Test Vocabulary
notebooks:
  - ./cards.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "cards.yml"), []byte(`- title: "Basic Words"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "alpha"
      meaning: "the first thing"
`), 0644))
	recent := time.Now().Add(-2 * time.Hour).Format(time.RFC3339)
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-vocab.yml"), []byte(fmt.Sprintf(`- metadata:
    notebook_id: test-vocab
    title: "Basic Words"
    type: "flashcard"
  expressions:
    - expression: "alpha"
      learned_logs:
        - status: "misunderstood"
          learned_at: %q
          quiz_type: "notebook"
`, recent)), 0644))

	// Production-like wiring: dual repo + DB clear store.
	multiRepo := learning.NewMultiLearningRepository(
		learning.NewYAMLLearningRepository(learningDir, nil),
		learning.NewDBLearningRepository(db),
	)
	svc := quiz.NewService(config.NotebooksConfig{
		FlashcardsDirectories:  []string{flashcardsDir},
		LearningNotesDirectory: learningDir,
	}, mock.NewClient(), make(map[string]rapidapi.Response), multiRepo, config.QuizConfig{})
	handler := NewQuizHandler(svc)

	logCount := func() int {
		var n int
		require.NoError(t, db.Get(&n, `SELECT COUNT(*) FROM learning_logs`))
		return n
	}

	require.Equal(t, 0, logCount())

	start, err := handler.StartRelearnQuiz(context.Background(),
		connect.NewRequest(&apiv1.StartRelearnQuizRequest{WindowHours: 24}))
	require.NoError(t, err)
	require.Len(t, start.Msg.GetCards(), 1)
	noteID := start.Msg.GetCards()[0].GetNoteId()

	resp, err := handler.SubmitRelearnAnswer(context.Background(),
		connect.NewRequest(&apiv1.SubmitRelearnAnswerRequest{NoteId: noteID, Answer: "the first thing"}))
	require.NoError(t, err)
	require.True(t, resp.Msg.GetCorrect())

	// (1) No learning history was written.
	assert.Equal(t, 0, logCount(), "a relearn answer must not write to learning_logs")

	// (2) Relearn is repeatable: the word is still in the pool next session
	// because a correct relearn answer persists nothing.
	start2, err := handler.StartRelearnQuiz(context.Background(),
		connect.NewRequest(&apiv1.StartRelearnQuizRequest{WindowHours: 24}))
	require.NoError(t, err)
	require.Len(t, start2.Msg.GetCards(), 1, "a relearned word must reappear next session — relearn stores no clear state")
	assert.Equal(t, "alpha", start2.Msg.GetCards()[0].GetEntry())
}
