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

// TestLoadRelearnPool_HomographResolvesToFailedSense pins the issue #32 fix: two
// same-spelling notes that differ only by part_of_speech (a homograph like
// "record" the noun vs the verb) must resolve to the sense that was actually
// failed. Before the fix the vocab index was keyed by expression alone
// (last-write-wins), so both failures collapsed onto whichever card loaded
// last, and the relearn card could show the wrong meaning.
func TestLoadRelearnPool_HomographResolvesToFailedSense(t *testing.T) {
	ctrl := gomock.NewController(t)
	flashcardsDir := t.TempDir()
	learningDir := t.TempDir()

	const nounMeaning = "a lasting written account of something"
	const verbMeaning = "to set something down in writing"

	// Source notebook: the noun and verb senses of "record" live in the SAME
	// notebook, distinguished only by part_of_speech. The verb card is listed
	// last on purpose — under the old expression-only index it would win the
	// last-write-wins slot for both senses.
	vocabDir := filepath.Join(flashcardsDir, "test-vocab")
	require.NoError(t, os.MkdirAll(vocabDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "index.yml"), []byte(`id: test-vocab
name: Test Vocabulary
notebooks:
  - ./cards.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(vocabDir, "cards.yml"), []byte(fmt.Sprintf(`- title: "Homographs"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "record"
      part_of_speech: "noun"
      meaning: %q
    - expression: "record"
      part_of_speech: "verb"
      meaning: %q
`, nounMeaning, verbMeaning)), 0644))

	// Both senses were recently failed in the recognition quiz; each carries
	// its own part_of_speech on the learning-history entry.
	recent := time.Now().Add(-30 * time.Minute).Format(time.RFC3339)
	history := fmt.Sprintf(`- metadata:
    notebook_id: test-vocab
    title: "Homographs"
    type: "flashcard"
  expressions:
    - expression: "record"
      part_of_speech: "noun"
      learned_logs:
        - status: "misunderstood"
          learned_at: %q
          quiz_type: "notebook"
    - expression: "record"
      part_of_speech: "verb"
      learned_logs:
        - status: "misunderstood"
          learned_at: %q
          quiz_type: "notebook"
`, recent, recent)
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-vocab.yml"), []byte(history), 0644))

	svc := NewService(config.NotebooksConfig{
		FlashcardsDirectories:  []string{flashcardsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil), config.QuizConfig{})

	cards, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)

	// Each sense yields its own card showing its own meaning — the two do not
	// collide in the index.
	byMeaning := make(map[string]RelearnCard, len(cards))
	for _, c := range cards {
		byMeaning[c.Meaning] = c
	}
	require.Contains(t, byMeaning, nounMeaning,
		"the failed noun sense must resolve to the noun's meaning, not the verb's")
	require.Contains(t, byMeaning, verbMeaning,
		"the failed verb sense must resolve to the verb's meaning, not the noun's")
	assert.Equal(t, nounMeaning, byMeaning[nounMeaning].vocabCard.Meaning,
		"the graded vocab card for the noun sense carries the noun meaning")
}
