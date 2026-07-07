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
//  2. A correct answer records a relearn_clears marker that persists in the DB,
//     so the recovered word drops out of the next session.
//
// Runs only when LANGNER_INTEGRATION_DB_URL is set (the postgres:16 service in
// CI). The relearn_clears table comes from migration 017.
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
	handler.SetRelearnClearStore(learning.NewDBRelearnClearStore(db))

	logCount := func() int {
		var n int
		require.NoError(t, db.Get(&n, `SELECT COUNT(*) FROM learning_logs`))
		return n
	}
	clearCount := func() int {
		var n int
		require.NoError(t, db.Get(&n, `SELECT COUNT(*) FROM relearn_clears`))
		return n
	}

	require.Equal(t, 0, logCount())
	require.Equal(t, 0, clearCount())

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
	// (2) A clear marker was persisted.
	assert.Equal(t, 1, clearCount(), "a correct relearn answer must record one relearn_clears row")

	// The persisted marker excludes the word from the next session.
	start2, err := handler.StartRelearnQuiz(context.Background(),
		connect.NewRequest(&apiv1.StartRelearnQuizRequest{WindowHours: 24}))
	require.NoError(t, err)
	assert.Empty(t, start2.Msg.GetCards(), "a DB-cleared word must not reappear in the next session")
}
