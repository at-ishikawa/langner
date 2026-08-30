package quiz

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	posts, err := svc.LoadGrammarPosts(0, "journal", nil)
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
	require.NoError(t, svc.SaveGrammarBlank(ctx, 0, post.NotebookID, blank.SenseID, result, 1200))

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
	posts, err = svc.LoadGrammarPosts(0, "journal", nil)
	require.NoError(t, err)
	assert.Empty(t, posts)
}

// writeCollidingGrammarNotebook builds a journal whose TWO entries collide on a
// derived correction id: their titles ("Week 16" and "Week-16") slugify to the
// same string, so each entry's first correction derives the same id
// (journal-week-16-s0-1). Before the uniqueness pass both corrections shared one
// learning-log series; answering one dropped the other from every quiz.
func writeCollidingGrammarNotebook(t *testing.T) (storiesDir, grammarsDir string) {
	t.Helper()
	base := t.TempDir()

	storyDir := filepath.Join(base, "stories", "journal")
	require.NoError(t, os.MkdirAll(storyDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(
		"id: journal\nname: \"English Journal\"\nnotebooks:\n  - ./posts.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "posts.yml"), []byte(
		`- event: "Week 16"
  scenes:
    - scene: ""
      statements:
        - "Yesterday the John called me."
- event: "Week-16"
  scenes:
    - scene: ""
      statements:
        - "I ate a apple today."
`), 0o644))

	grammarsDir = filepath.Join(base, "grammars", "journal")
	require.NoError(t, os.MkdirAll(grammarsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(grammarsDir, "index.yml"), []byte(
		"id: journal\nnotebooks:\n  - ./corr.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(grammarsDir, "corr.yml"), []byte(
		`- metadata:
    title: "Week 16"
  scenes:
    - metadata:
        index: 0
      corrections:
        - incorrect: "the John"
          correct: "John"
          category: article
          reason: "No article before a personal name."
- metadata:
    title: "Week-16"
  scenes:
    - metadata:
        index: 0
      corrections:
        - incorrect: "a apple"
          correct: "an apple"
          category: article
          reason: "Use 'an' before a vowel sound."
`), 0o644))
	return filepath.Join(base, "stories"), filepath.Join(base, "grammars")
}

func findGrammarBlank(t *testing.T, posts []GrammarPost, incorrect string) GrammarBlank {
	t.Helper()
	for _, p := range posts {
		for _, b := range p.Blanks {
			if b.Incorrect == incorrect {
				return b
			}
		}
	}
	t.Fatalf("no blank for %q in posts", incorrect)
	return GrammarBlank{}
}

// TestService_GrammarQuiz_CollidingIDsStayIndependent reproduces the vanish and
// proves the fix: two corrections whose entry titles slugify alike get DISTINCT
// senseIDs, so answering one correctly leaves the OTHER still due (its own
// series is untouched). Under the pre-fix single-derived-id keying both shared
// one series and the second correction disappeared from the live quiz and the
// Relearn pool.
func TestService_GrammarQuiz_CollidingIDsStayIndependent(t *testing.T) {
	ctx := context.Background()
	storiesDir, grammarsDir := writeCollidingGrammarNotebook(t)
	learningDir := t.TempDir()
	svc := newGrammarService(t, storiesDir, grammarsDir, learningDir)

	posts, err := svc.LoadGrammarPosts(0, "journal", nil)
	require.NoError(t, err)
	theJohn := findGrammarBlank(t, posts, "the John")
	anApple := findGrammarBlank(t, posts, "a apple")
	require.NotEqual(t, theJohn.SenseID, anApple.SenseID,
		"colliding corrections must be stored under distinct senseIDs (L1)")

	// Answer "the John" correctly and persist it.
	result, err := svc.GradeGrammarBlank(ctx, posts[0].Content, theJohn, "John", 1000)
	require.NoError(t, err)
	require.True(t, result.Correct)
	require.NoError(t, svc.SaveGrammarBlank(ctx, 0, "journal", theJohn.SenseID, result, 1000))

	// The other correction is still due — it was NOT conflated with the answered
	// one, so it remains in the live grammar quiz...
	posts, err = svc.LoadGrammarPosts(0, "journal", nil)
	require.NoError(t, err)
	still := findGrammarBlank(t, posts, "a apple")
	assert.Equal(t, anApple.SenseID, still.SenseID)
	for _, p := range posts {
		for _, b := range p.Blanks {
			assert.NotEqual(t, "the John", b.Incorrect, "the answered correction is no longer due")
		}
	}

	// ...and in the Relearn pool once it has been missed (misunderstood).
	wrong, err := svc.GradeGrammarBlank(ctx, "", still, "", 500)
	require.NoError(t, err)
	require.False(t, wrong.Correct)
	require.NoError(t, svc.SaveGrammarBlank(ctx, 0, "journal", still.SenseID, wrong, 500))

	pool, err := svc.LoadRelearnPool(0, time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)
	var found bool
	for _, card := range pool {
		if card.Format == notebook.QuizTypeGrammar && card.Incorrect == "a apple" {
			found = true
		}
	}
	assert.True(t, found, "the still-due correction reaches the Relearn pool on its own series")
}

// TestService_GrammarQuiz_ByteIdenticalDuplicateDeduped verifies that two
// byte-identical corrections that share an id (a true duplicate) collapse to a
// single blank rather than showing twice.
func TestService_GrammarQuiz_ByteIdenticalDuplicateDeduped(t *testing.T) {
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
		`- metadata:
    title: "Note 1"
  scenes:
    - metadata:
        index: 0
      corrections:
        - id: dup
          incorrect: "the John"
          correct: "John"
          category: article
          reason: "No article before a personal name."
        - id: dup
          incorrect: "the John"
          correct: "John"
          category: article
          reason: "No article before a personal name."
`), 0o644))

	svc := newGrammarService(t, filepath.Join(base, "stories"), filepath.Join(base, "grammars"), t.TempDir())
	posts, err := svc.LoadGrammarPosts(0, "journal", nil)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	assert.Len(t, posts[0].Blanks, 1, "byte-identical duplicate corrections collapse to one blank")
}

func TestService_LoadGrammarStorySummaries(t *testing.T) {
	storiesDir, grammarsDir := writeGrammarNotebook(t)
	learningDir := t.TempDir()
	svc := newGrammarService(t, storiesDir, grammarsDir, learningDir)

	summaries, err := svc.LoadGrammarStorySummaries(0)
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

	posts, err := svc.LoadGrammarPosts(0, "journal", nil)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	blank := posts[0].Blanks[0]

	// The deterministic mock marks answers starting with "wrong" incorrect.
	result, err := svc.GradeGrammarBlank(ctx, posts[0].Content, blank, "wrong guess", 900)
	require.NoError(t, err)
	assert.False(t, result.Correct)
	require.NoError(t, svc.SaveGrammarBlank(ctx, 0, posts[0].NotebookID, blank.SenseID, result, 900))

	// A misunderstood mistake remains due on the next load.
	posts, err = svc.LoadGrammarPosts(0, "journal", nil)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	require.Len(t, posts[0].Blanks, 1)
	assert.Equal(t, string(notebook.LearnedStatusMisunderstood), posts[0].Blanks[0].Status)
}

// TestLoadNotebookSummaries_JournalTaggedKind verifies FIX 1(b): a notebook
// loaded from a journals directory is surfaced in LoadNotebookSummaries with
// Kind "Journal" (so the Vocabulary tab can exclude it and a Journals tab can
// list it), while the SAME notebook still appears as a separate "Grammar"
// summary for the grammar quiz tab.
func TestLoadNotebookSummaries_JournalTaggedKind(t *testing.T) {
	base := t.TempDir()

	journalDir := filepath.Join(base, "journals", "journal")
	require.NoError(t, os.MkdirAll(journalDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(journalDir, "index.yml"), []byte(
		"id: journal\nname: \"English Journal\"\nnotebooks:\n  - ./posts.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(journalDir, "posts.yml"), []byte(
		"- event: \"Note 1\"\n  scenes:\n    - scene: \"\"\n      statements:\n        - \"Yesterday the John called me.\"\n"), 0o644))

	grammarsDir := filepath.Join(base, "grammars", "journal")
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

	learningDir := t.TempDir()
	quizCfg := config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}}
	svc := NewService(
		config.NotebooksConfig{
			JournalsDirectories:    []string{filepath.Join(base, "journals")},
			GrammarsDirectories:    []string{filepath.Join(base, "grammars")},
			LearningNotesDirectory: learningDir,
		},
		inferencemock.NewClient(),
		make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, notebook.NewIntervalCalculator(quizCfg.Algorithm, quizCfg.FixedIntervals)),
		quizCfg,
	)

	summaries, err := svc.LoadNotebookSummaries(0, true)
	require.NoError(t, err)

	var journalKind, grammarKind bool
	for _, s := range summaries {
		if s.NotebookID != "journal" {
			continue
		}
		switch s.Kind {
		case "Journal":
			journalKind = true
			// This journal has only statements (prose) and no scene
			// definitions, so it structurally has zero vocabulary. The
			// Vocabulary tab gates on VocabularyCount > 0, so this empty
			// journal drops out of the picker.
			assert.Equal(t, 0, s.VocabularyCount, "definition-less journal must have VocabularyCount 0")
		case "Grammar":
			grammarKind = true
			// Grammar summaries are correction drills, never vocabulary.
			assert.Equal(t, 0, s.VocabularyCount, "grammar summary must have VocabularyCount 0")
		default:
			t.Errorf("journal notebook must not be plain vocabulary (kind %q)", s.Kind)
		}
	}
	assert.True(t, journalKind, "journal must be tagged Kind=Journal (not plain vocabulary)")
	assert.True(t, grammarKind, "journal must still surface a Grammar summary for the grammar quiz tab")
}

// TestService_LoadGrammarMistakes_DueAndExcluded verifies FIX 3: the standalone
// mistake list returns every correction (due AND excluded), Exclude sets the
// SAME grammar skipped_at the live quiz path filters on (so an excluded mistake
// is is_excluded here and drops out of LoadGrammarPosts), and Resume clears it.
func TestService_LoadGrammarMistakes_DueAndExcluded(t *testing.T) {
	storiesDir, grammarsDir := writeGrammarNotebook(t)
	learningDir := t.TempDir()
	svc := newGrammarService(t, storiesDir, grammarsDir, learningDir)

	// A fresh notebook: the one correction is listed, due, not excluded.
	mistakes, err := svc.LoadGrammarMistakes(0, "journal", nil)
	require.NoError(t, err)
	require.Len(t, mistakes, 1)
	assert.Equal(t, "note-the-john", mistakes[0].SenseID)
	assert.Equal(t, "the John", mistakes[0].Incorrect)
	assert.Equal(t, "John", mistakes[0].Correct)
	assert.False(t, mistakes[0].IsExcluded)

	// Exclude it via the same SkipWord path the handler / live quiz uses.
	info := CardInfo{NotebookName: "journal", StoryTitle: notebook.JournalStoryTitle, Expression: "note-the-john"}
	require.NoError(t, svc.SkipWord(0, info, "", []notebook.QuizType{notebook.QuizTypeGrammar}))

	// The on-disk YAML records the grammar exclusion.
	raw, err := os.ReadFile(filepath.Join(learningDir, "journal.yml"))
	require.NoError(t, err)
	var got []notebook.LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))
	require.Len(t, got, 1)
	assert.Equal(t, "grammar", got[0].Metadata.Type)
	require.Len(t, got[0].Expressions, 1)
	assert.True(t, got[0].Expressions[0].SkippedAt.IsSkipped(notebook.QuizTypeGrammar),
		"Exclude must set the grammar skipped_at marker")

	// The list still returns it, now flagged excluded...
	mistakes, err = svc.LoadGrammarMistakes(0, "journal", nil)
	require.NoError(t, err)
	require.Len(t, mistakes, 1)
	assert.True(t, mistakes[0].IsExcluded, "an excluded mistake stays in the list flagged is_excluded")

	// ...but the live quiz path no longer offers it (same marker).
	posts, err := svc.LoadGrammarPosts(0, "journal", nil)
	require.NoError(t, err)
	assert.Empty(t, posts, "Exclude removes the mistake from the live grammar quiz / Relearn pool")

	// Resume clears it: it is due and offered again.
	require.NoError(t, svc.ResumeWord(0, info, []notebook.QuizType{notebook.QuizTypeGrammar}))
	mistakes, err = svc.LoadGrammarMistakes(0, "journal", nil)
	require.NoError(t, err)
	require.Len(t, mistakes, 1)
	assert.False(t, mistakes[0].IsExcluded, "Resume clears the exclusion")
	posts, err = svc.LoadGrammarPosts(0, "journal", nil)
	require.NoError(t, err)
	require.Len(t, posts, 1, "Resume makes the mistake due again")
}
