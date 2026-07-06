package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/schemas"
)

// This file covers what the user's frustration is fundamentally about:
// PER-QUIZ-TYPE INTEGRATION COVERAGE against a live Postgres. sqlmock
// tests don't verify actual constraint semantics; unit-level YAML
// tests don't exercise the DB write path at all. A regression in
// ensureNoteExists or SaveResult for any quiz type is invisible to
// both — which is why the user kept hitting different write-path
// bugs after the MySQL→Postgres migration.
//
// Each test:
//   1. Boots a full QuizHandler wired to MultiLearningRepository
//      (YAML primary + live-DB secondary), matching production.
//   2. Runs the RPC pair for one quiz type: Start* → Submit*.
//   3. Repeats the answer for the same expression a second time —
//      the exact interaction that broke on "duplicate key value
//      violates unique constraint notes_usage_entry_key".
//   4. Asserts learning_logs holds both rows with the expected
//      quiz_type and the shared notes row was reused (not
//      duplicated).
//
// Skipped locally; required in CI via LANGNER_INTEGRATION_DB_URL
// wired to the postgres:16 service container.

const integrationDBEnv = "LANGNER_INTEGRATION_DB_URL"

// pgIntegrationHandler builds a QuizHandler backed by a real Postgres
// on a fresh schema. Returns the handler, the fixture directory (so
// tests can read the learning-history YAML back if needed), and the
// db handle so tests can assert against learning_logs / notes.
func pgIntegrationHandler(t *testing.T, mockClient inference.Client) (*QuizHandler, *sqlx.DB, string) {
	t.Helper()

	dsn := os.Getenv(integrationDBEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping live-Postgres RPC coverage", integrationDBEnv)
	}

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())

	// Fresh schema so every subtest starts from a clean slate — the
	// bugs the user hit surface most reliably when notes/logs are
	// created from scratch rather than inherited from a previous
	// run's leftover rows.
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	// Build minimal fixtures: one story, one flashcard set. Same
	// shape as newTestHandlerWithFixtures but written inline so the
	// integration suite doesn't share mutable state with the
	// sqlmock tests.
	storiesDir := t.TempDir()
	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(storiesDir, "test-story"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(storiesDir, "test-story", "index.yml"), []byte(`id: test-story
name: Test Story
notebooks:
  - ./episodes.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(storiesDir, "test-story", "episodes.yml"), []byte(`- event: "Chapter One"
  date: 2025-01-15T00:00:00Z
  scenes:
    - scene: "Opening"
      conversations:
        - speaker: "Alice"
          quote: "That sounds preposterous to me."
      definitions:
        - expression: "preposterous"
          meaning: "contrary to reason or common sense"
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-story.yml"), []byte(`- metadata:
    notebook_id: test-story
    title: "Chapter One"
  scenes:
    - metadata:
        title: "Opening"
      expressions:
        - expression: "preposterous"
          learned_logs:
            - status: "misunderstood"
              learned_at: "2025-01-14"
              quiz_type: "freeform"
`), 0644))

	require.NoError(t, os.MkdirAll(filepath.Join(flashcardsDir, "test-vocab"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(flashcardsDir, "test-vocab", "index.yml"), []byte(`id: test-vocab
name: Test Vocabulary
notebooks:
  - ./cards.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(flashcardsDir, "test-vocab", "cards.yml"), []byte(`- title: "Basic Words"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "serendipity"
      meaning: "a fortunate discovery by accident"
      examples:
        - "It was pure serendipity that they met."
`), 0644))
	// loadFlashcardCards hard-codes StoryTitle="flashcards" on the
	// read side, and SaveResult / UpdateOrCreateExpressionWithQuality
	// use that same value on the write side. Seed the learning
	// history with title="flashcards" so the SubmitAnswer's log
	// appends to this block instead of forking a new one — otherwise
	// FindExpressionByAnyName returns the wrong block's expression
	// during OverrideAnswer and the DB update silently no-ops.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-vocab.yml"), []byte(`- metadata:
    notebook_id: test-vocab
    title: "flashcards"
    type: "flashcard"
  expressions:
    - expression: "serendipity"
      learned_logs:
        - status: "understood"
          learned_at: "2025-01-14"
          quiz_type: "freeform"
`), 0644))

	yamlRepo := learning.NewYAMLLearningRepository(learningDir, nil)
	dbRepo := learning.NewDBLearningRepository(db)
	multiRepo := learning.NewMultiLearningRepository(yamlRepo, dbRepo)

	svc := quiz.NewService(config.NotebooksConfig{
		StoriesDirectories:     []string{storiesDir},
		FlashcardsDirectories:  []string{flashcardsDir},
		LearningNotesDirectory: learningDir,
	}, mockClient, make(map[string]rapidapi.Response), multiRepo, config.QuizConfig{})

	return NewQuizHandler(svc), db, learningDir
}

// assertDBHasQuizAnswers checks that the learning_logs table holds
// exactly the expected number of rows for (expression, quiz_type),
// with a single shared notes row backing them. This is what a
// regression in ensureNoteExists / SaveResult / MultiLearningRepository
// silently gets wrong.
func assertDBHasQuizAnswers(t *testing.T, db *sqlx.DB, expression, quizType string, expectedLogs int) {
	t.Helper()

	var noteCount int
	require.NoError(t, db.Get(&noteCount,
		`SELECT COUNT(*) FROM notes WHERE "usage" = $1 AND entry = $1`, expression))
	assert.Equal(t, 1, noteCount,
		"expression %q must have exactly one backing notes row (not duplicated by repeated quiz answers)", expression)

	var logCount int
	require.NoError(t, db.Get(&logCount, `
		SELECT COUNT(*) FROM learning_logs ll
		JOIN notes n ON n.id = ll.note_id
		WHERE n."usage" = $1 AND n.entry = $1 AND ll.quiz_type = $2`,
		expression, quizType))
	assert.Equal(t, expectedLogs, logCount,
		"expected %d learning_logs rows with quiz_type=%q for %q, got %d",
		expectedLogs, quizType, expression, logCount)
}

// TestQuizHandler_Standard_LivePostgres_Integration exercises
// StartQuiz → SubmitAnswer twice. Before the ensureNoteExists UPSERT
// fix the second call surfaced "duplicate key value violates unique
// constraint notes_usage_entry_key" as the user reported.
func TestQuizHandler_Standard_LivePostgres_Integration(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler, db, _ := pgIntegrationHandler(t, mockClient)

	answerCard := func(t *testing.T) {
		startResp, err := handler.StartQuiz(context.Background(),
			connect.NewRequest(&apiv1.StartQuizRequest{
				NotebookIds:      []string{"test-vocab"},
				IncludeUnstudied: true,
			}))
		require.NoError(t, err)
		require.NotEmpty(t, startResp.Msg.GetFlashcards(), "start-quiz must produce a card for the fresh notebook")
		noteID := startResp.Msg.GetFlashcards()[0].GetNoteId()

		mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
			inference.AnswerMeaningsResponse{Answers: []inference.AnswerMeaning{{
				Expression: "serendipity",
				Meaning:    "a fortunate discovery by accident",
				AnswersForContext: []inference.AnswersForContext{{Correct: true, Reason: "ok", Quality: 4}},
			}}}, nil)
		_, err = handler.SubmitAnswer(context.Background(),
			connect.NewRequest(&apiv1.SubmitAnswerRequest{
				NoteId: noteID, Answer: "a fortunate discovery by accident", ResponseTimeMs: 1000,
			}))
		require.NoError(t, err)
	}

	answerCard(t)
	answerCard(t)

	assertDBHasQuizAnswers(t, db, "serendipity", "notebook", 2)
}

// TestQuizHandler_Reverse_LivePostgres_Integration covers the
// reverse-quiz write path, which routes through
// UpdateOrCreateExpressionWithQualityForReverse on the YAML side and
// DBLearningRepository.Create with quiz_type="reverse" on the DB
// side. Sharing the same ensureNoteExists code as Standard means
// this test would have caught the user's bug too — but wasn't there.
func TestQuizHandler_Reverse_LivePostgres_Integration(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler, db, _ := pgIntegrationHandler(t, mockClient)

	answer := func(t *testing.T) {
		startResp, err := handler.StartReverseQuiz(context.Background(),
			connect.NewRequest(&apiv1.StartReverseQuizRequest{
				NotebookIds: []string{"test-vocab"}, IncludeUnstudied: true,
			}))
		require.NoError(t, err)
		require.NotEmpty(t, startResp.Msg.GetFlashcards())
		noteID := startResp.Msg.GetFlashcards()[0].GetNoteId()

		mockClient.EXPECT().ValidateWordForm(gomock.Any(), gomock.Any()).Return(
			inference.ValidateWordFormResponse{
				Classification: inference.ClassificationSameWord,
				Reason:         "exact match",
				Quality:        4,
			}, nil)
		_, err = handler.SubmitReverseAnswer(context.Background(),
			connect.NewRequest(&apiv1.SubmitReverseAnswerRequest{
				NoteId: noteID, Answer: "serendipity", ResponseTimeMs: 1000,
			}))
		require.NoError(t, err)
	}
	answer(t)
	answer(t)

	assertDBHasQuizAnswers(t, db, "serendipity", "reverse", 2)
}

// TestQuizHandler_Freeform_LivePostgres_Integration covers the
// freeform path. Freeform writes to LearnedLogs on the YAML side
// (via UpdateOrCreateExpressionWithQuality) AND ReverseLogs (via
// UpdateOrCreateExpressionWithQualityForReverse) — the DB write is
// a single row with quiz_type="freeform". Two freeform answers must
// leave exactly two DB rows with quiz_type="freeform", not four,
// and both must share the same notes row.
func TestQuizHandler_Freeform_LivePostgres_Integration(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler, db, _ := pgIntegrationHandler(t, mockClient)

	// StartFreeformQuiz populates the handler's freeform card set;
	// SubmitFreeformAnswer then matches the user's Word/Meaning
	// against that set. No NoteId is passed — the handler resolves
	// the card by expression match.
	_, err := handler.StartFreeformQuiz(context.Background(),
		connect.NewRequest(&apiv1.StartFreeformQuizRequest{}))
	require.NoError(t, err)

	answer := func(t *testing.T) {
		mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
			inference.AnswerMeaningsResponse{Answers: []inference.AnswerMeaning{{
				Expression: "preposterous",
				Meaning:    "contrary to reason or common sense",
				AnswersForContext: []inference.AnswersForContext{{Correct: true, Reason: "ok", Quality: 4}},
			}}}, nil)
		_, err := handler.SubmitFreeformAnswer(context.Background(),
			connect.NewRequest(&apiv1.SubmitFreeformAnswerRequest{
				Word: "preposterous", Meaning: "contrary to reason or common sense", ResponseTimeMs: 1000,
			}))
		require.NoError(t, err)
	}
	answer(t)
	answer(t)

	assertDBHasQuizAnswers(t, db, "preposterous", "freeform", 2)
}

// TestQuizHandler_BatchSubmit_LivePostgres_Integration covers the
// batch write path (BatchSubmitAnswers). BatchSubmitAnswers loops
// SaveResult per card, so if two cards in one batch reference the
// same expression (or the same expression is re-submitted across
// batches) the ensureNoteExists race would fire under the old
// SELECT-then-INSERT code. This test exercises that path too.
func TestQuizHandler_BatchSubmit_LivePostgres_Integration(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler, db, _ := pgIntegrationHandler(t, mockClient)

	startResp, err := handler.StartQuiz(context.Background(),
		connect.NewRequest(&apiv1.StartQuizRequest{
			NotebookIds:      []string{"test-vocab"},
			IncludeUnstudied: true,
		}))
	require.NoError(t, err)
	require.NotEmpty(t, startResp.Msg.GetFlashcards())
	noteID := startResp.Msg.GetFlashcards()[0].GetNoteId()

	// Two batch calls, each answering the same card, so the second
	// call takes the "note already exists" path through
	// ensureNoteExists — the exact failure mode the user reported.
	for i := 0; i < 2; i++ {
		mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
			inference.AnswerMeaningsResponse{Answers: []inference.AnswerMeaning{{
				Expression: "serendipity",
				Meaning:    "a fortunate discovery by accident",
				AnswersForContext: []inference.AnswersForContext{{Correct: true, Reason: fmt.Sprintf("batch-%d", i), Quality: 4}},
			}}}, nil)
		_, err = handler.BatchSubmitAnswers(context.Background(),
			connect.NewRequest(&apiv1.BatchSubmitAnswersRequest{
				Answers: []*apiv1.SubmitAnswerRequest{{
					NoteId: noteID, Answer: "a fortunate discovery by accident", ResponseTimeMs: 1000,
				}},
			}))
		require.NoError(t, err)
	}

	assertDBHasQuizAnswers(t, db, "serendipity", "notebook", 2)
}

// TestQuizHandler_Standard_OverrideAnswer_LivePostgres_Integration
// covers the recently-fixed override path end-to-end: answer a card,
// then flip the answer with OverrideAnswer, then confirm the DB row
// (not just the YAML) reflects the new status. Locks in the
// Mark-as-Correct fix so a future refactor can't silently drop the
// MultiLearningRepository.UpdateLog secondary write.
func TestQuizHandler_Standard_OverrideAnswer_LivePostgres_Integration(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl)
	handler, db, _ := pgIntegrationHandler(t, mockClient)

	startResp, err := handler.StartQuiz(context.Background(),
		connect.NewRequest(&apiv1.StartQuizRequest{
			NotebookIds:      []string{"test-vocab"},
			IncludeUnstudied: true,
		}))
	require.NoError(t, err)
	noteID := startResp.Msg.GetFlashcards()[0].GetNoteId()

	// Submit an incorrect answer so the log lands as misunderstood /
	// quality 1 on both stores.
	mockClient.EXPECT().AnswerMeanings(gomock.Any(), gomock.Any()).Return(
		inference.AnswerMeaningsResponse{Answers: []inference.AnswerMeaning{{
			Expression: "serendipity",
			Meaning:    "a fortunate discovery by accident",
			AnswersForContext: []inference.AnswersForContext{{Correct: false, Reason: "wrong", Quality: 1}},
		}}}, nil)
	submitResp, err := handler.SubmitAnswer(context.Background(),
		connect.NewRequest(&apiv1.SubmitAnswerRequest{
			NoteId: noteID, Answer: "wrong meaning", ResponseTimeMs: 1000,
		}))
	require.NoError(t, err)
	learnedAt := submitResp.Msg.GetLearnedAt()
	require.NotEmpty(t, learnedAt)

	// Confirm the DB row landed as misunderstood.
	var status string
	require.NoError(t, db.Get(&status, `
		SELECT ll.status FROM learning_logs ll
		JOIN notes n ON n.id = ll.note_id
		WHERE n."usage" = 'serendipity' AND ll.quiz_type = 'notebook'
		ORDER BY ll.learned_at DESC LIMIT 1`))
	require.Equal(t, "misunderstood", status)

	// Mark it correct — this exercises MultiLearningRepository.UpdateLog,
	// which was silently no-op'ing on the DB side before the recent fix.
	markCorrect := true
	_, err = handler.OverrideAnswer(context.Background(),
		connect.NewRequest(&apiv1.OverrideAnswerRequest{
			NoteId:      noteID,
			QuizType:    apiv1.QuizType_QUIZ_TYPE_STANDARD,
			LearnedAt:   learnedAt,
			MarkCorrect: &markCorrect,
		}))
	require.NoError(t, err)

	// The DB row must now show "understood" — if MirrorValues isn't
	// forwarded to the DB path, the row would still say
	// "misunderstood" and this assertion would trip.
	require.NoError(t, db.Get(&status, `
		SELECT ll.status FROM learning_logs ll
		JOIN notes n ON n.id = ll.note_id
		WHERE n."usage" = 'serendipity' AND ll.quiz_type = 'notebook'
		ORDER BY ll.learned_at DESC LIMIT 1`))
	assert.Equal(t, "understood", status,
		"OverrideAnswer must flip the DB row's status, not just the YAML side")
}
