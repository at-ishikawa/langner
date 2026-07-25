package quiz

import (
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
