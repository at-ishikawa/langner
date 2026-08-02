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

// writeGrammarNotebook creates a minimal story (prose) plus a matching grammars
// notebook (corrections keyed by entry title) in temp dirs, returning both.
func writeGrammarNotebook(t *testing.T) (storyDir, grammarsDir string) {
	t.Helper()
	base := t.TempDir()

	storyDir = filepath.Join(base, "stories", "journal")
	require.NoError(t, os.MkdirAll(storyDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(
		"id: journal\nname: \"English Journal\"\nnotebooks:\n  - ./posts.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "posts.yml"), []byte(
		"- event: \"Note 1\"\n  scenes:\n    - scene: \"\"\n      statements:\n        - \"Yesterday the John called me.\"\n"), 0o644))

	grammarsDir = filepath.Join(base, "grammars", "journal")
	require.NoError(t, os.MkdirAll(grammarsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(grammarsDir, "index.yml"), []byte(
		"id: journal\nnotebooks:\n  - ./corr.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(grammarsDir, "corr.yml"), []byte(
		`- metadata:
    title: "Note 1"
  scenes:
    - metadata:
        index: 0
      corrections:
        - id: note-the-john
          incorrect: "the John"
          correct: "John"
          category: article
          reason: "No article before a personal name."
`), 0o644))
	return filepath.Join(base, "stories"), filepath.Join(base, "grammars")
}

func newGrammarService(t *testing.T, storiesDir, grammarsDir, learningDir string) *Service {
	t.Helper()
	quizCfg := config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}}
	calc := notebook.NewIntervalCalculator(quizCfg.Algorithm, quizCfg.FixedIntervals)
	return NewService(
		config.NotebooksConfig{
			StoriesDirectories:     []string{storiesDir},
			GrammarsDirectories:    []string{grammarsDir},
			LearningNotesDirectory: learningDir,
		},
		inferencemock.NewClient(),
		make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, calc),
		quizCfg,
	)
}

func TestService_GrammarQuiz_LoadGradeSave(t *testing.T) {
	ctx := context.Background()
	storiesDir, grammarsDir := writeGrammarNotebook(t)
	learningDir := t.TempDir()
	svc := newGrammarService(t, storiesDir, grammarsDir, learningDir)

	// 1. A fresh notebook yields one due entry carrying the full text + a blank.
	posts, err := svc.LoadGrammarPosts("journal", nil)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	post := posts[0]
	assert.Contains(t, post.Content, "the John called me")
	require.Len(t, post.Blanks, 1)
	blank := post.Blanks[0]
	assert.Equal(t, "note-the-john", blank.SenseID)
	assert.Equal(t, "the John", blank.Incorrect)
	assert.Equal(t, "John", blank.Correct)
	assert.Equal(t, string(notebook.LearnedStatusLearning), blank.Status)

	// 2. A correct fix grades correct and records an "understood" log keyed by
	// the correction id under a flat "grammar" history.
	result, err := svc.GradeGrammarBlank(ctx, post.Content, blank, "John", 1200)
	require.NoError(t, err)
	assert.True(t, result.Correct)
	require.NoError(t, svc.SaveGrammarBlank(ctx, post.NotebookID, blank.SenseID, result, 1200))

	raw, err := os.ReadFile(filepath.Join(learningDir, "journal.yml"))
	require.NoError(t, err)
	var got []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))
	require.Len(t, got, 1)
	assert.Equal(t, "grammar", got[0].Metadata.Type)
	require.Len(t, got[0].Expressions, 1)
	assert.Equal(t, "note-the-john", got[0].Expressions[0].Expression)
	require.NotEmpty(t, got[0].Expressions[0].LearnedLogs)
	assert.Equal(t, notebook.LearnedStatusUnderstood, got[0].Expressions[0].LearnedLogs[0].Status)

	// 3. Having just been answered correctly, the entry has no due blanks.
	posts, err = svc.LoadGrammarPosts("journal", nil)
	require.NoError(t, err)
	assert.Empty(t, posts)
}

func TestService_LoadGrammarStorySummaries(t *testing.T) {
	storiesDir, grammarsDir := writeGrammarNotebook(t)
	learningDir := t.TempDir()
	svc := newGrammarService(t, storiesDir, grammarsDir, learningDir)

	summaries, err := svc.LoadGrammarStorySummaries()
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "journal", summaries[0].NotebookID)
	assert.Equal(t, "Grammar", summaries[0].Kind)
	assert.Equal(t, "English Journal", summaries[0].Name)
	assert.Equal(t, 1, summaries[0].GrammarReviewCount)
	require.Len(t, summaries[0].Sections, 1)
	assert.Equal(t, "Note 1", summaries[0].Sections[0].Title)
	assert.Equal(t, 1, summaries[0].Sections[0].GrammarReviewCount)
}

func TestService_GrammarQuiz_WrongAnswerStaysDue(t *testing.T) {
	ctx := context.Background()
	storiesDir, grammarsDir := writeGrammarNotebook(t)
	learningDir := t.TempDir()
	svc := newGrammarService(t, storiesDir, grammarsDir, learningDir)

	posts, err := svc.LoadGrammarPosts("journal", nil)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	blank := posts[0].Blanks[0]

	// The deterministic mock marks answers starting with "wrong" incorrect.
	result, err := svc.GradeGrammarBlank(ctx, posts[0].Content, blank, "wrong guess", 900)
	require.NoError(t, err)
	assert.False(t, result.Correct)
	require.NoError(t, svc.SaveGrammarBlank(ctx, posts[0].NotebookID, blank.SenseID, result, 900))

	// A misunderstood mistake remains due on the next load.
	posts, err = svc.LoadGrammarPosts("journal", nil)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	require.Len(t, posts[0].Blanks, 1)
	assert.Equal(t, string(notebook.LearnedStatusMisunderstood), posts[0].Blanks[0].Status)
}
