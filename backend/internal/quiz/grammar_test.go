package quiz

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	inferencemock "github.com/at-ishikawa/langner/internal/inference/mock"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// writeJournalNotebook creates a minimal journal (prose) notebook plus a
// matching corrections notebook in temp dirs, returning both directories.
func writeJournalNotebook(t *testing.T) (journalDir, correctionsDir string) {
	t.Helper()
	base := t.TempDir()

	journalDir = filepath.Join(base, "journal")
	require.NoError(t, os.MkdirAll(journalDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(journalDir, "index.yml"), []byte(
		"id: journal\nname: \"English Journal\"\nnotebooks:\n  - ./posts.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(journalDir, "posts.yml"), []byte(
		"- id: e1\n  text: \"Yesterday the John called me.\"\n"), 0o644))

	correctionsDir = filepath.Join(base, "journal-corrections")
	require.NoError(t, os.MkdirAll(correctionsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(correctionsDir, "index.yml"), []byte(
		"id: journal\nnotebooks:\n  - ./corr.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(correctionsDir, "corr.yml"), []byte(
		`- id: e1
  corrections:
    - line: 1
      incorrect: "the John"
      correct: "John"
      category: article
      reason: "No article before a personal name."
`), 0o644))
	return journalDir, correctionsDir
}

func newGrammarService(t *testing.T, journalDir, correctionsDir, learningDir string) *Service {
	t.Helper()
	quizCfg := config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}}
	calc := notebook.NewIntervalCalculator(quizCfg.Algorithm, quizCfg.FixedIntervals)
	return NewService(
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
}

func TestService_GrammarQuiz_LoadGradeSave(t *testing.T) {
	ctx := context.Background()
	journalDir, correctionsDir := writeJournalNotebook(t)
	learningDir := t.TempDir()
	svc := newGrammarService(t, journalDir, correctionsDir, learningDir)

	// 1. A fresh notebook yields one due card carrying the full post + the span.
	cards, err := svc.LoadGrammarCards("journal")
	require.NoError(t, err)
	require.Len(t, cards, 1)
	card := cards[0]
	assert.Equal(t, "e1-L1-1", card.MistakeID)
	assert.Equal(t, "the John", card.Incorrect)
	assert.Equal(t, "John", card.Correct)
	assert.Equal(t, "article", card.Category)
	assert.Contains(t, card.Content, "the John called me")
	assert.Equal(t, string(notebook.LearnedStatusLearning), card.Status)

	// 2. A correct fix grades correct and records an "understood" log keyed by
	// the correction id under a flat "grammar" history.
	result, err := svc.GradeGrammarAnswer(ctx, card, "John", 1200)
	require.NoError(t, err)
	assert.True(t, result.Correct)
	require.NoError(t, svc.SaveGrammarResult(ctx, card, result, 1200))

	raw, err := os.ReadFile(filepath.Join(learningDir, "journal.yml"))
	require.NoError(t, err)
	var got []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))
	require.Len(t, got, 1)
	assert.Equal(t, "grammar", got[0].Metadata.Type)
	require.Len(t, got[0].Expressions, 1)
	assert.Equal(t, "e1-L1-1", got[0].Expressions[0].Expression)
	require.NotEmpty(t, got[0].Expressions[0].LearnedLogs)
	assert.Equal(t, notebook.LearnedStatusUnderstood, got[0].Expressions[0].LearnedLogs[0].Status)

	// 3. Having just been answered correctly, the mistake is no longer due.
	cards, err = svc.LoadGrammarCards("journal")
	require.NoError(t, err)
	assert.Empty(t, cards)
}

func TestService_LoadJournalNotebookSummaries(t *testing.T) {
	journalDir, correctionsDir := writeJournalNotebook(t)
	learningDir := t.TempDir()
	svc := newGrammarService(t, journalDir, correctionsDir, learningDir)

	summaries, err := svc.LoadJournalNotebookSummaries()
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "journal", summaries[0].NotebookID)
	assert.Equal(t, "Journal", summaries[0].Kind)
	assert.Equal(t, 1, summaries[0].GrammarReviewCount)
}

func TestService_GrammarQuiz_WrongAnswerStaysDue(t *testing.T) {
	ctx := context.Background()
	journalDir, correctionsDir := writeJournalNotebook(t)
	learningDir := t.TempDir()
	svc := newGrammarService(t, journalDir, correctionsDir, learningDir)

	cards, err := svc.LoadGrammarCards("journal")
	require.NoError(t, err)
	require.Len(t, cards, 1)

	// The deterministic mock marks answers starting with "wrong" incorrect.
	result, err := svc.GradeGrammarAnswer(ctx, cards[0], "wrong guess", 900)
	require.NoError(t, err)
	assert.False(t, result.Correct)
	require.NoError(t, svc.SaveGrammarResult(ctx, cards[0], result, 900))

	// A misunderstood mistake remains due on the next load.
	cards, err = svc.LoadGrammarCards("journal")
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, string(notebook.LearnedStatusMisunderstood), cards[0].Status)
}
