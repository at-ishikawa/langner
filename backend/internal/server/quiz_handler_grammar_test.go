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
	base := t.TempDir()

	journalDir := filepath.Join(base, "journal")
	require.NoError(t, os.MkdirAll(journalDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(journalDir, "index.yml"), []byte(
		"id: journal\nname: \"English Journal\"\nnotebooks:\n  - ./posts.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(journalDir, "posts.yml"), []byte(
		"- id: e1\n  text: \"Yesterday the John called me.\"\n"), 0o644))

	correctionsDir := filepath.Join(base, "journal-corrections")
	require.NoError(t, os.MkdirAll(correctionsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(correctionsDir, "index.yml"), []byte(
		"id: journal\nnotebooks:\n  - ./corr.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(correctionsDir, "corr.yml"), []byte(
		"- id: e1\n  corrections:\n    - line: 1\n      incorrect: \"the John\"\n      correct: \"John\"\n      category: article\n"), 0o644))

	learningDir := t.TempDir()
	quizCfg := config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}}
	calc := notebook.NewIntervalCalculator(quizCfg.Algorithm, quizCfg.FixedIntervals)
	svc := quiz.NewService(
		config.NotebooksConfig{
			JournalDirectories:            []string{journalDir},
			JournalCorrectionsDirectories: []string{correctionsDir},
			LearningNotesDirectory:        learningDir,
		},
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

	// Start: one due card carrying the full post; no reference correction leaked.
	start, err := handler.StartGrammarQuiz(ctx, connect.NewRequest(&apiv1.StartGrammarQuizRequest{
		NotebookIds: []string{"journal"},
	}))
	require.NoError(t, err)
	require.Len(t, start.Msg.GetCards(), 1)
	card := start.Msg.GetCards()[0]
	assert.Equal(t, "e1-L1-1", card.GetCardId())
	assert.Equal(t, "the John", card.GetIncorrect())
	assert.Contains(t, card.GetSentence(), "the John called me")
	assert.Equal(t, int32(1), card.GetLine())

	// Submit a correct fix: graded correct, reference correction revealed.
	sub, err := handler.SubmitGrammarAnswer(ctx, connect.NewRequest(&apiv1.SubmitGrammarAnswerRequest{
		NotebookId:     "journal",
		CardId:         "e1-L1-1",
		Answer:         "John",
		ResponseTimeMs: 1000,
	}))
	require.NoError(t, err)
	assert.True(t, sub.Msg.GetCorrect())
	assert.Equal(t, "John", sub.Msg.GetCorrectAnswer())
	assert.Equal(t, "the John", sub.Msg.GetIncorrect())

	// Persisted under a flat "grammar" history keyed by the correction id.
	raw, err := os.ReadFile(filepath.Join(learningDir, "journal.yml"))
	require.NoError(t, err)
	var got []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))
	require.Len(t, got, 1)
	assert.Equal(t, "grammar", got[0].Metadata.Type)
	require.Len(t, got[0].Expressions, 1)
	assert.Equal(t, "e1-L1-1", got[0].Expressions[0].Expression)
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
