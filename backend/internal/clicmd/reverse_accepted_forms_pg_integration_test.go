package clicmd

import (
	"context"
	"io"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/bootstrap"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/datasync"
	"github.com/at-ishikawa/langner/internal/inference"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/internal/server"
	"github.com/at-ishikawa/langner/schemas"
)

// TestReverseAcceptedForms_LivePostgres_Integration drives the reverse quiz the
// way langner-server runs in DB mode: the example notebooks from
// config.example.yml imported into Postgres, then StartReverseQuiz and
// BatchSubmitReverseAnswers through the real handler. The answers are the
// forms the cards themselves use (the story's "stepped on my toes", the
// flashcards' example highlights) or a single-word misspelling, so they must be
// graded without the LLM: the grader is a gomock client with no expectations,
// which fails the test if it is called. A reverse miss then reaches Relearn,
// whose reverse card is graded the same way.
func TestReverseAcceptedForms_LivePostgres_Integration(t *testing.T) {
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
	importOwner := seedIntegrationUser(t, db, "reverse-forms-import-owner")
	_, err = newImporterFromConfig(cfg, db, io.Discard).ImportAll(context.Background(), datasync.ImportOptions{OwnerID: importOwner})
	require.NoError(t, err)

	repos := bootstrap.BuildStateRepositories(cfg.Notebooks, cfg.Quiz, db)
	client := mock_inference.NewMockClient(gomock.NewController(t))
	svc := quiz.NewService(cfg.Notebooks, inference.StaticResolver(client), nil, repos.Learning, cfg.Quiz)
	svc.SetHistoryStore(repos.HistoryStore)
	userID := seedIntegrationUser(t, db, "reverse-forms-user")
	ctx := auth.WithUserID(context.Background(), userID)

	h := server.NewQuizHandler(svc)
	h.SetNoteRepository(repos.Note)
	start, err := h.StartReverseQuiz(ctx, connect.NewRequest(&apiv1.StartReverseQuizRequest{
		NotebookIds: []string{"office-talk", "reverse-forms"}, IncludeUnstudied: true,
	}))
	require.NoError(t, err)
	noteByMeaning := map[string]int64{}
	for _, c := range start.Msg.GetFlashcards() {
		noteByMeaning[c.GetMeaning()] = c.GetNoteId()
	}

	cases := []struct {
		meaning string
		answer  string
		correct bool
	}{
		{"to upset someone by getting involved in something that is their responsibility", "stepped on my toes", true},
		{"to move or travel from one place to another", "went", true},
		{"to stay calm in a difficult situation", "kept her cool", true},
		{"to tell people secret information", "spilled the beens", true},
		{"a period of a thousand years", "millenium", false},
	}
	var answers []*apiv1.SubmitReverseAnswerRequest
	for _, tc := range cases {
		noteID := noteByMeaning[tc.meaning]
		require.NotZero(t, noteID, "%q must be served by the reverse quiz", tc.meaning)
		answers = append(answers, &apiv1.SubmitReverseAnswerRequest{NoteId: noteID, Answer: tc.answer, ResponseTimeMs: 2000})
	}
	resp, err := h.BatchSubmitReverseAnswers(ctx, connect.NewRequest(&apiv1.BatchSubmitReverseAnswersRequest{Answers: answers}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetResponses(), len(cases))
	for i, tc := range cases {
		r := resp.Msg.GetResponses()[i]
		assert.Equal(t, tc.correct, r.GetCorrect(), "%q", tc.answer)
		if tc.correct {
			assert.Equal(t, string(inference.ClassificationSameWord), r.GetClassification(), "%q", tc.answer)
		} else {
			assert.Equal(t, "Misspelled — compare your spelling with the answer.", r.GetReason())
		}
		assert.NotEmpty(t, r.GetLearnedAt(), "%q must be saved", tc.answer)
	}

	// The misspelled answer was saved as a reverse miss, so the word is in
	// Relearn; its reverse card accepts the word and rejects the misspelling,
	// again without the LLM.
	relearn, err := h.StartRelearnQuiz(ctx, connect.NewRequest(&apiv1.StartRelearnQuizRequest{}))
	require.NoError(t, err)
	var relearnID int64
	for _, c := range relearn.Msg.GetCards() {
		if c.GetEntry() == "millennium" && c.GetSourceQuizType() == apiv1.QuizType_QUIZ_TYPE_REVERSE {
			relearnID = c.GetNoteId()
		}
	}
	require.NotZero(t, relearnID, "the reverse miss must be in the relearn pool")
	graded, err := h.BatchSubmitRelearnAnswers(ctx, connect.NewRequest(&apiv1.BatchSubmitRelearnAnswersRequest{
		Answers: []*apiv1.SubmitRelearnAnswerRequest{
			{NoteId: relearnID, Answer: "millennium", ResponseTimeMs: 2000},
			{NoteId: relearnID, Answer: "millenium", ResponseTimeMs: 2000},
		},
	}))
	require.NoError(t, err)
	assert.True(t, graded.Msg.GetResponses()[0].GetCorrect())
	assert.False(t, graded.Msg.GetResponses()[1].GetCorrect())
}
