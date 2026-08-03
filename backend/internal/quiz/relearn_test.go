package quiz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

func TestRelearnRecognitionContexts_GuaranteesOneAnchoredContext(t *testing.T) {
	// A word with no example sentences must still yield one context carrying the
	// meaning as reference_definition — otherwise the meaning grader returns no
	// answer and the card is always marked incorrect.
	got := relearnRecognitionContexts(FreeformCard{Meaning: "all-knowing"})
	require.Len(t, got, 1)
	assert.Equal(t, "all-knowing", got[0].ReferenceDefinition)
}

func TestRelearnRecognitionContexts_SetsReferenceOnEveryContext(t *testing.T) {
	got := relearnRecognitionContexts(FreeformCard{
		Meaning: "abstaining from worldly pleasures",
		Contexts: []inference.Context{
			{Context: "an ascetic monk"},
			{Context: "an ascetic life"},
		},
	})
	require.Len(t, got, 2)
	for _, c := range got {
		assert.Equal(t, "abstaining from worldly pleasures", c.ReferenceDefinition,
			"the known meaning is the grader's authoritative ground truth")
	}
	assert.Equal(t, "an ascetic monk", got[0].Context, "existing context sentences are preserved")
}

// TestLoadRelearnPool_ResolvesHomographByID pins invariant L2 (symmetric read
// and write) for the Relearn pool: two vocab entries that share the SAME
// expression AND part_of_speech but carry distinct stable ids and distinct
// meanings must each resolve to their OWN card by id — never collapse into one
// last-write-wins entry showing the other sense's meaning.
//
// Before the id-keyed resolution, both senses collided: the candidate map keyed
// only by (format, notebook, expression) collapsed the pair into a single
// candidate, and the vocab index keyed only by expression returned whichever
// card was written last — so the surviving card displayed the WRONG meaning.
func TestLoadRelearnPool_ResolvesHomographByID(t *testing.T) {
	const (
		notebookID  = "vocab"
		expr        = "bank"
		firstID     = "bank-river"
		secondID    = "bank-money"
		firstMean   = "the land alongside a river"
		secondMean  = "a financial institution"
		partOfSpeak = "noun"
	)

	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()

	notebookDir := filepath.Join(flashcardsDir, notebookID)
	require.NoError(t, os.MkdirAll(notebookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(notebookDir, "index.yml"), []byte(
		"id: "+notebookID+"\nname: \"Vocabulary\"\nnotebooks:\n  - ./cards.yml\n"), 0644))
	// Two senses of "bank" — same spelling, same part_of_speech, distinct id +
	// meaning — in ONE notebook.
	require.NoError(t, os.WriteFile(filepath.Join(notebookDir, "cards.yml"), []byte(
		`- title: "Flashcards"
  date: 2025-01-15T00:00:00Z
  cards:
    - id: `+firstID+`
      expression: `+expr+`
      part_of_speech: `+partOfSpeak+`
      meaning: `+firstMean+`
    - id: `+secondID+`
      expression: `+expr+`
      part_of_speech: `+partOfSpeak+`
      meaning: `+secondMean+`
`), 0644))

	// Learning history: both senses failed (misunderstood) in-window; the first
	// (bank-river) is the most-recent wrong, so a bug that keeps the newest
	// candidate but resolves by expression would surface the second sense.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, notebookID+".yml"), []byte(
		`- metadata:
    id: `+notebookID+`
    title: "Flashcards"
    type: flashcard
  expressions:
    - id: `+firstID+`
      expression: `+expr+`
      type: vocabulary
      learned_logs:
        - status: misunderstood
          learned_at: "2026-07-20T00:00:00Z"
    - id: `+secondID+`
      expression: `+expr+`
      type: vocabulary
      learned_logs:
        - status: misunderstood
          learned_at: "2026-07-19T00:00:00Z"
`), 0644))

	ctrl := gomock.NewController(t)
	mockClient := mock_inference.NewMockClient(ctrl) // never called: LoadRelearnPool does no grading
	svc := NewService(config.NotebooksConfig{
		FlashcardsDirectories:  []string{flashcardsDir},
		LearningNotesDirectory: learningDir,
	}, mockClient, make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil),
		config.QuizConfig{})

	windowStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	cards, err := svc.LoadRelearnPool(windowStart)
	require.NoError(t, err)

	// Each id resolves to its own card: two cards, each carrying its own sense's
	// meaning. A collision would produce one card (or two cards both showing the
	// second sense).
	require.Len(t, cards, 2, "each distinct id must yield its own relearn card")
	meanings := map[string]bool{}
	for _, c := range cards {
		assert.Equal(t, expr, c.Entry)
		meanings[c.Meaning] = true
	}
	assert.True(t, meanings[firstMean],
		"the first sense (%s) must resolve to its own meaning by id, not the other sense's", firstID)
	assert.True(t, meanings[secondMean],
		"the second sense (%s) must resolve to its own meaning by id", secondID)
}

// TestLoadRelearnPool_ConceptMemberUsesHeadMeaning pins the consummate bug:
// a word folded into a definitions family-concept must be shown in relearn
// under the concept HEAD and its umbrella meaning — the same as the standard
// quiz — not resolved to its own raw sense by last-write-wins. "bank" here has
// two senses and is a concept member; relearn must show the concept meaning.
func TestLoadRelearnPool_ConceptMemberUsesHeadMeaning(t *testing.T) {
	ctrl := gomock.NewController(t)
	defsDir := t.TempDir()
	learningDir := t.TempDir()

	bookDir := filepath.Join(defsDir, "test-book")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(`id: test-book
notebooks:
  - ./ch.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "ch.yml"), []byte(`- metadata:
    title: Chapter 1
  scenes:
  - metadata:
      index: 0
      title: S1
    expressions:
    - expression: bank
      id: bank
      meaning: the land beside a river
    - expression: bank
      id: bank-2
      meaning: a place to keep money
  concepts:
  - head: bank
    meaning: umbrella meaning for the bank family
    expressions:
    - bank
`), 0644))

	recent := time.Now().Add(-30 * time.Minute).Format(time.RFC3339)
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-book.yml"), []byte(fmt.Sprintf(`- metadata:
    notebook_id: test-book
    title: Chapter 1
  scenes:
  - metadata:
      title: S1
    expressions:
    - expression: bank
      learned_logs:
      - status: misunderstood
        learned_at: %q
        quiz_type: notebook
`, recent)), 0644))

	svc := NewService(config.NotebooksConfig{
		DefinitionsDirectories: []string{defsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	cards, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)

	var found bool
	for _, c := range cards {
		if c.Entry == "bank" {
			found = true
			assert.Equal(t, "umbrella meaning for the bank family", c.Meaning,
				"a concept member must show the concept head's meaning, not a raw sense")
		}
	}
	require.True(t, found, "the failed concept member must appear in the relearn pool")
}

// TestLoadRelearnPool_GrammarMiss pins Part A of the grammar/Relearn fix: a
// missed grammar correction (a "misunderstood" log under the flat "grammar"
// learning-history bucket) must resurface in the Relearn pool as a
// QuizTypeGrammar card carrying the entry's full content and the mistaken
// span — not be silently dropped because relearnSeries mislabeled it
// QuizTypeNotebook and it then failed vocab resolution (the pre-fix bug).
func TestLoadRelearnPool_GrammarMiss(t *testing.T) {
	storiesDir, grammarsDir := writeGrammarNotebook(t)
	learningDir := t.TempDir()

	// A learning history entry for the correction, written the same way
	// SaveGrammarBlank writes it: flat "grammar" bucket, Expression == ID ==
	// the correction's stable senseID, status misunderstood.
	recent := time.Now().Add(-30 * time.Minute).Format(time.RFC3339)
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "journal.yml"), []byte(fmt.Sprintf(`- metadata:
    id: journal
    title: journal
    type: grammar
  expressions:
    - id: note-the-john
      expression: note-the-john
      learned_logs:
        - status: misunderstood
          learned_at: %q
          quiz_type: grammar
`, recent)), 0o644))

	svc := newGrammarService(t, storiesDir, grammarsDir, learningDir)

	cards, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)
	require.Len(t, cards, 1, "the missed correction must yield exactly one relearn card")

	card := cards[0]
	assert.Equal(t, notebook.QuizTypeGrammar, card.Format)
	assert.Contains(t, card.Content, "the John called me", "the card carries the whole entry, like the live grammar quiz")
	assert.Equal(t, "the John", card.Incorrect)
	// NotebookName carries the notebook ID (not the display name) so a
	// deliberate Exclude (SkipWord) resolves to the correct <id>.yml.
	assert.Equal(t, "journal", card.NotebookName)

	// The card must grade like the live quiz: GradeGrammarBlank against the
	// card's own grading inputs (GrammarCard(), never sent to the client).
	result, err := svc.GradeGrammarBlank(context.Background(), card.Content, card.GrammarCard(), "John", 1200)
	require.NoError(t, err)
	assert.True(t, result.Correct)
}

// TestLoadRelearnPool_GrammarSamePostMultipleBlanks pins the contract the
// frontend's post-grouping relies on: when a single journal post has MORE THAN
// ONE due correction, the pool emits one card per correction, every card
// carries the SAME full post text (so the client can group them into one
// progressive post view), each card keeps its OWN mistaken span and grades
// individually — and a correction of the same post that is NOT due (its latest
// log is not "misunderstood") is not emitted at all, so relearn only ever asks
// the due blanks while still rendering the whole post for context.
func TestLoadRelearnPool_GrammarSamePostMultipleBlanks(t *testing.T) {
	base := t.TempDir()
	learningDir := t.TempDir()

	storyDir := filepath.Join(base, "stories", "journal")
	require.NoError(t, os.MkdirAll(storyDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "index.yml"), []byte(
		"id: journal\nname: \"English Journal\"\nnotebooks:\n  - ./posts.yml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(storyDir, "posts.yml"), []byte(
		"- event: \"Note 1\"\n  scenes:\n    - scene: \"\"\n      statements:\n        - \"Yesterday the John called me and then I go home.\"\n"), 0o644))

	grammarsDir := filepath.Join(base, "grammars", "journal")
	require.NoError(t, os.MkdirAll(grammarsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(grammarsDir, "index.yml"), []byte(
		"id: journal\nnotebooks:\n  - ./corr.yml\n"), 0o644))
	// One post, three corrections: two will be due, one will not be.
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
        - id: note-go
          incorrect: "go"
          correct: "went"
          category: tense
          reason: "Use past tense for a past event."
        - id: note-me
          incorrect: "called me"
          correct: "phoned me"
          category: word-choice
          reason: "Prefer a more precise verb."
`), 0o644))

	// Two corrections failed (misunderstood) in-window; the third is understood,
	// so it is NOT due and must not enter the pool.
	recent := time.Now().Add(-30 * time.Minute).Format(time.RFC3339)
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "journal.yml"), []byte(fmt.Sprintf(`- metadata:
    id: journal
    title: journal
    type: grammar
  expressions:
    - id: note-the-john
      expression: note-the-john
      learned_logs:
        - status: misunderstood
          learned_at: %q
          quiz_type: grammar
    - id: note-go
      expression: note-go
      learned_logs:
        - status: misunderstood
          learned_at: %q
          quiz_type: grammar
    - id: note-me
      expression: note-me
      learned_logs:
        - status: understood
          learned_at: %q
          quiz_type: grammar
`, recent, recent, recent)), 0o644))

	svc := newGrammarService(t, filepath.Join(base, "stories"), filepath.Join(base, "grammars"), learningDir)

	cards, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)

	var grammar []RelearnCard
	for _, c := range cards {
		if c.Format == notebook.QuizTypeGrammar {
			grammar = append(grammar, c)
		}
	}
	require.Len(t, grammar, 2, "only the two DUE corrections are emitted; the understood one is excluded")

	incorrects := map[string]bool{}
	for _, c := range grammar {
		incorrects[c.Incorrect] = true
		// Every due blank carries the SAME whole post text — the group key the
		// client folds them by into one progressive post view.
		assert.Equal(t, grammar[0].Content, c.Content, "all blanks of one post share the full post text")
		assert.Contains(t, c.Content, "the John called me and then I go home")
		// Each blank grades on its OWN correction, individually (no collapsing).
		want := "John"
		if c.Incorrect == "go" {
			want = "went"
		}
		result, err := svc.GradeGrammarBlank(context.Background(), c.Content, c.GrammarCard(), want, 1200)
		require.NoError(t, err)
		assert.True(t, result.Correct, "blank %q must grade against its own reference", c.Incorrect)
	}
	assert.True(t, incorrects["the John"], "the first due mistake is asked")
	assert.True(t, incorrects["go"], "the second due mistake is asked")
	assert.False(t, incorrects["called me"], "a not-due correction of the same post is not asked")
}
