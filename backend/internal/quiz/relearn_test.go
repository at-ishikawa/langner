package quiz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/inference"
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
