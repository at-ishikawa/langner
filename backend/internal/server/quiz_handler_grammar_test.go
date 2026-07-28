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

	// Start: one due post carrying the full text + one blank with a note_id.
	start, err := handler.StartGrammarQuiz(ctx, connect.NewRequest(&apiv1.StartGrammarQuizRequest{
		NotebookIds: []string{"journal"},
	}))
	require.NoError(t, err)
	require.Len(t, start.Msg.GetPosts(), 1)
	post := start.Msg.GetPosts()[0]
	assert.Contains(t, post.GetPostText(), "the John called me")
	require.Len(t, post.GetBlanks(), 1)
	blank := post.GetBlanks()[0]
	assert.Greater(t, blank.GetNoteId(), int64(0))
	assert.Equal(t, "e1-L1-1", blank.GetSenseId())
	assert.Equal(t, "the John", blank.GetIncorrect())
	assert.Equal(t, int32(1), blank.GetLine())

	// Submit the whole post; correct fix graded correct, reference revealed.
	sub, err := handler.SubmitGrammarPost(ctx, connect.NewRequest(&apiv1.SubmitGrammarPostRequest{
		Answers: []*apiv1.GrammarBlankAnswer{
			{NoteId: blank.GetNoteId(), Answer: "John", ResponseTimeMs: 1000},
		},
	}))
	require.NoError(t, err)
	require.Len(t, sub.Msg.GetResults(), 1)
	res := sub.Msg.GetResults()[0]
	assert.True(t, res.GetCorrect())
	assert.Equal(t, "John", res.GetCorrectAnswer())
	assert.Equal(t, "the John", res.GetIncorrect())
	assert.Equal(t, blank.GetNoteId(), res.GetNoteId())

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

func TestQuizHandler_SubmitGrammarPost_NotFound(t *testing.T) {
	handler, _ := newGrammarHandler(t)
	_, err := handler.SubmitGrammarPost(context.Background(), connect.NewRequest(&apiv1.SubmitGrammarPostRequest{
		Answers: []*apiv1.GrammarBlankAnswer{{NoteId: 999999, Answer: "John"}},
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
