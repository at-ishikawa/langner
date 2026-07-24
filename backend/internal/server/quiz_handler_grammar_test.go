package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	inferencemock "github.com/at-ishikawa/langner/internal/inference/mock"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
)

func newGrammarHandler(t *testing.T) (*QuizHandler, string) {
	t.Helper()
	journalDir := filepath.Join(t.TempDir(), "journal")
	require.NoError(t, os.MkdirAll(journalDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(journalDir, "index.yml"), []byte(
		"id: journal\nname: \"English Journal\"\nnotebooks:\n  - ./entries.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(journalDir, "entries.yml"), []byte(
		`- title: "July"
  date: 2026-07-01T00:00:00Z
  entries:
    - id: e1
      text: "Yesterday the John called me."
      mistakes:
        - id: m1
          incorrect: "the John"
          correct: "John"
          category: article
`), 0o644))

	learningDir := t.TempDir()
	quizCfg := config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}}
	calc := notebook.NewIntervalCalculator(quizCfg.Algorithm, quizCfg.FixedIntervals)
	svc := quiz.NewService(
		config.NotebooksConfig{JournalDirectories: []string{journalDir}, LearningNotesDirectory: learningDir},
		inferencemock.NewClient(),
		make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, calc),
		quizCfg,
	)
	return NewQuizHandler(svc), learningDir
}

func TestQuizHandler_GrammarQuiz(t *testing.T) {
	ctx := context.Background()
	handler, learningDir := newGrammarHandler(t)

	// Start: one due card, no reference correction leaked to the client.
	start, err := handler.StartGrammarQuiz(ctx, connect.NewRequest(&apiv1.StartGrammarQuizRequest{
		NotebookIds: []string{"journal"},
	}))
	require.NoError(t, err)
	require.Len(t, start.Msg.GetCards(), 1)
	card := start.Msg.GetCards()[0]
	assert.Equal(t, "m1", card.GetCardId())
	assert.Equal(t, "the John", card.GetIncorrect())
	assert.Contains(t, card.GetSentence(), "the John called me")

	// Submit a correct fix: graded correct, reference correction revealed.
	sub, err := handler.SubmitGrammarAnswer(ctx, connect.NewRequest(&apiv1.SubmitGrammarAnswerRequest{
		NotebookId:     "journal",
		CardId:         "m1",
		Answer:         "John",
		ResponseTimeMs: 1000,
	}))
	require.NoError(t, err)
	assert.True(t, sub.Msg.GetCorrect())
	assert.Equal(t, "John", sub.Msg.GetCorrectAnswer())
	assert.Equal(t, "the John", sub.Msg.GetIncorrect())

	// Persisted under a flat "grammar" history keyed by the mistake id.
	raw, err := os.ReadFile(filepath.Join(learningDir, "journal.yml"))
	require.NoError(t, err)
	var got []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))
	require.Len(t, got, 1)
	assert.Equal(t, "grammar", got[0].Metadata.Type)
	require.Len(t, got[0].Expressions, 1)
	assert.Equal(t, "m1", got[0].Expressions[0].Expression)
	require.NotEmpty(t, got[0].Expressions[0].LearnedLogs)
}

func TestQuizHandler_SubmitGrammarAnswer_NotFound(t *testing.T) {
	handler, _ := newGrammarHandler(t)
	_, err := handler.SubmitGrammarAnswer(context.Background(), connect.NewRequest(&apiv1.SubmitGrammarAnswerRequest{
		NotebookId: "journal",
		CardId:     "does-not-exist",
		Answer:     "John",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
