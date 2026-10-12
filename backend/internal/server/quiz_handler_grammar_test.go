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
	"github.com/at-ishikawa/langner/internal/inference"
	inferencemock "github.com/at-ishikawa/langner/internal/inference/mock"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
)

func newGrammarHandler(t *testing.T) (*QuizHandler, string) {
	t.Helper()
	base := t.TempDir()

	storyDir := filepath.Join(base, "stories", "journal")
	require.NoError(t, os.MkdirAll(storyDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(
		"id: journal\nname: \"English Journal\"\nnotebooks:\n  - ./posts.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "posts.yml"), []byte(
		"- event: \"Note 1\"\n  scenes:\n    - scene: \"\"\n      statements:\n        - \"Yesterday the John called me.\"\n"), 0o644))

	grammarsDir := filepath.Join(base, "grammars", "journal")
	require.NoError(t, os.MkdirAll(grammarsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(grammarsDir, "index.yml"), []byte(
		"id: journal\nnotebooks:\n  - ./corr.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(grammarsDir, "corr.yml"), []byte(
		"- metadata:\n    title: \"Note 1\"\n  scenes:\n    - metadata:\n        index: 0\n      corrections:\n        - id: note-the-john\n          incorrect: \"the John\"\n          correct: \"John\"\n          category: article\n"), 0o644))

	learningDir := t.TempDir()
	quizCfg := config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}}
	calc := notebook.NewIntervalCalculator(quizCfg.Algorithm, quizCfg.FixedIntervals)
	svc := quiz.NewService(
		config.NotebooksConfig{
			StoriesDirectories:     []string{filepath.Join(base, "stories")},
			GrammarsDirectories:    []string{filepath.Join(base, "grammars")},
			LearningNotesDirectory: learningDir,
		},
		inference.StaticResolver(inferencemock.NewClient()),
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
	assert.Equal(t, "note-the-john", blank.GetSenseId())
	assert.Equal(t, "the John", blank.GetIncorrect())

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
	assert.Equal(t, "note-the-john", got[0].Expressions[0].Expression)
	require.NotEmpty(t, got[0].Expressions[0].LearnedLogs)
}

// Override on a grammar blank must succeed even when the override RPC lands on
// a DIFFERENT (cold) serverless instance than the one that served the quiz. The
// grammar store is per-instance, so resolveCardInfo must re-resolve the stable
// grammarBlankID hash statelessly — rebuild the posts, re-hash, match — exactly
// as the submit path does, instead of 404ing through a notes FindByID that can
// never map a hash to a row. This is the cross-instance "note not found" bug.
func TestQuizHandler_OverrideGrammar_CrossInstance(t *testing.T) {
	ctx := context.Background()
	handlerA, _ := newGrammarHandler(t)

	start, err := handlerA.StartGrammarQuiz(ctx, connect.NewRequest(&apiv1.StartGrammarQuizRequest{
		NotebookIds: []string{"journal"},
	}))
	require.NoError(t, err)
	blank := start.Msg.GetPosts()[0].GetBlanks()[0]

	sub, err := handlerA.SubmitGrammarPost(ctx, connect.NewRequest(&apiv1.SubmitGrammarPostRequest{
		Answers: []*apiv1.GrammarBlankAnswer{{NoteId: blank.GetNoteId(), Answer: "John", ResponseTimeMs: 1000}},
	}))
	require.NoError(t, err)
	res := sub.Msg.GetResults()[0]
	require.NotEmpty(t, res.GetLearnedAt())

	// A second handler sharing the SAME service models a cold instance: its
	// grammarStore is empty, so the quiz note_id minted on handlerA is a pure
	// store miss here — the path that used to 404.
	handlerB := NewQuizHandler(handlerA.svc)
	_, inStore := handlerB.grammarStore[blank.GetNoteId()]
	require.False(t, inStore, "cold instance must not hold the blank in its per-instance store")

	markIncorrect := false
	ov, err := handlerB.OverrideAnswer(ctx, connect.NewRequest(&apiv1.OverrideAnswerRequest{
		NoteId:      blank.GetNoteId(),
		SenseId:     "note-the-john",
		QuizType:    apiv1.QuizType_QUIZ_TYPE_GRAMMAR,
		LearnedAt:   res.GetLearnedAt(),
		MarkCorrect: &markIncorrect,
	}))
	require.NoError(t, err, "cross-instance grammar override must resolve statelessly, not 404")
	assert.NotEmpty(t, ov.Msg.GetOriginalStatus(), "override must have found and flipped the just-written log")
}

// An unresolvable blank id is non-fatal: it is skipped with a placeholder
// result instead of aborting the whole post (serverless statelessness fix).
func TestQuizHandler_SubmitGrammarPost_UnresolvableBlankIsSkipped(t *testing.T) {
	handler, _ := newGrammarHandler(t)
	resp, err := handler.SubmitGrammarPost(context.Background(), connect.NewRequest(&apiv1.SubmitGrammarPostRequest{
		Answers: []*apiv1.GrammarBlankAnswer{{NoteId: 999999, Answer: "John"}},
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetResults(), 1)
	assert.False(t, resp.Msg.GetResults()[0].GetCorrect())
}
