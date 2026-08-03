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

// newEtymologyHandler wires an etymology notebook and a matching definitions
// notebook (same book id "roots") so buildOriginFamilies attaches the word
// family to the origin. The book has one origin ("scribo") with two derived
// words ("describe", "inscribe") — generic examples, no personal data.
func newEtymologyHandler(t *testing.T) (*QuizHandler, string) {
	t.Helper()
	base := t.TempDir()
	const bookID = "roots"

	etymBook := filepath.Join(base, "etymology", bookID)
	require.NoError(t, os.MkdirAll(etymBook, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(etymBook, "index.yml"), []byte(
		"id: roots\nkind: Etymology\nname: Roots\nnotebooks:\n  - ./s1.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etymBook, "s1.yml"), []byte(
		"metadata:\n  title: \"Session 1\"\norigins:\n  - origin: \"scribo\"\n    type: root\n    language: Latin\n    meaning: to write\n"), 0o644))

	defsBook := filepath.Join(base, "definitions", bookID)
	require.NoError(t, os.MkdirAll(defsBook, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(defsBook, "index.yml"), []byte(
		"id: roots\nnotebooks:\n  - ./s1.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(defsBook, "s1.yml"), []byte(
		"- metadata:\n    title: \"Session 1\"\n  scenes:\n  - metadata:\n      index: 0\n      title: S1\n    expressions:\n    - expression: describe\n      meaning: to represent in words\n      origin_parts:\n      - origin: scribo\n    - expression: inscribe\n      meaning: to write or carve on a surface\n      origin_parts:\n      - origin: scribo\n"), 0o644))

	learningDir := t.TempDir()
	quizCfg := config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}, DisableShuffle: true}
	calc := notebook.NewIntervalCalculator(quizCfg.Algorithm, quizCfg.FixedIntervals)
	svc := quiz.NewService(
		config.NotebooksConfig{
			EtymologyDirectories:   []string{filepath.Join(base, "etymology")},
			DefinitionsDirectories: []string{filepath.Join(base, "definitions")},
			LearningNotesDirectory: learningDir,
		},
		inferencemock.NewClient(),
		make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, calc),
		quizCfg,
	)
	return NewQuizHandler(svc), learningDir
}

// TestQuizHandler_EtymologyWordExcludeResume proves the ExcludeEtymologyWord /
// ResumeEtymologyWord RPCs write the exact skipped_at key buildOriginFamilies
// reads (learning-history L2): the excluded word drops from its origin family
// in StartEtymologyOriginQuiz, an origin with every word excluded is no longer
// offered, and Resume restores the word. It asserts both the on-disk YAML and
// the loader-facing read (IsExpressionExcludedForQuizType).
func TestQuizHandler_EtymologyWordExcludeResume(t *testing.T) {
	ctx := context.Background()
	handler, learningDir := newEtymologyHandler(t)

	familyExpressions := func() []string {
		start, err := handler.StartEtymologyOriginQuiz(ctx, connect.NewRequest(&apiv1.StartEtymologyOriginQuizRequest{
			EtymologyNotebookIds: []string{"roots"},
			IncludeUnstudied:     true,
		}))
		require.NoError(t, err)
		var exprs []string
		for _, c := range start.Msg.GetCards() {
			for _, w := range c.GetWords() {
				exprs = append(exprs, w.GetExpression())
			}
		}
		return exprs
	}
	readHistories := func() []notebook.LearningHistory {
		raw, err := os.ReadFile(filepath.Join(learningDir, "roots.yml"))
		require.NoError(t, err)
		var histories []notebook.LearningHistory
		require.NoError(t, yaml.Unmarshal(raw, &histories))
		return histories
	}
	exclude := func(expr string) {
		_, err := handler.ExcludeEtymologyWord(ctx, connect.NewRequest(&apiv1.ExcludeEtymologyWordRequest{
			NotebookId: "roots", Expression: expr,
		}))
		require.NoError(t, err)
	}

	// Baseline: the origin carries both family words.
	assert.ElementsMatch(t, []string{"describe", "inscribe"}, familyExpressions())

	// Exclude "describe": only that word drops; the origin stays quizzable, and
	// the on-disk skipped_at (the loader's read key) agrees.
	exclude("describe")
	assert.Equal(t, []string{"inscribe"}, familyExpressions())
	assert.True(t, notebook.IsExpressionExcludedForQuizType(
		readHistories(), "", notebook.QuizTypeEtymologyOrigin, "describe"))
	assert.False(t, notebook.IsExpressionExcludedForQuizType(
		readHistories(), "", notebook.QuizTypeEtymologyOrigin, "inscribe"))

	// Exclude "inscribe" too: the origin now has no family and is not offered.
	exclude("inscribe")
	assert.Empty(t, familyExpressions())

	// Resume "describe": it reappears; "inscribe" stays excluded.
	_, err := handler.ResumeEtymologyWord(ctx, connect.NewRequest(&apiv1.ResumeEtymologyWordRequest{
		NotebookId: "roots", Expression: "describe",
	}))
	require.NoError(t, err)
	assert.Equal(t, []string{"describe"}, familyExpressions())
	assert.False(t, notebook.IsExpressionExcludedForQuizType(
		readHistories(), "", notebook.QuizTypeEtymologyOrigin, "describe"))
}
