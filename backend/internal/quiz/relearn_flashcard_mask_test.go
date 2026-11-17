package quiz

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// The vocabulary flashcard example "let the cat out of the bag" carries two
// examples that use a VARIANT surface form ("let the whole cat out of the bag"):
// one with a highlight naming it, one without.
const (
	flashcardMaskWord        = "let the cat out of the bag"
	flashcardMaskHighlighted = "She let the whole cat out of the bag at dinner."
	flashcardMaskNoHighlight = "He let the whole cat out of the bag before the party."
)

// TestExampleData_FlashcardWordsCarryHighlight pins that loadFlashcardWords
// (reached through LoadAllWords over config.example.yml) carries each example's
// highlight on FreeformCard.Examples, so reverse surfaces can mask the exact
// surface form.
func TestExampleData_FlashcardWordsCarryHighlight(t *testing.T) {
	svc := newExampleService(t, t.TempDir())

	words, err := svc.LoadAllWords(0)
	require.NoError(t, err)
	var fc *FreeformCard
	for i := range words {
		if words[i].NotebookName == "vocabulary" && words[i].Expression == flashcardMaskWord {
			fc = &words[i]
		}
	}
	require.NotNil(t, fc, "the vocabulary flashcard %q must load", flashcardMaskWord)
	assert.Equal(t, []Example{
		{Text: flashcardMaskHighlighted, Highlight: "let the whole cat out of the bag"},
		{Text: flashcardMaskNoHighlight},
	}, fc.Examples)
}

// TestExampleData_FlashcardReverseRelearnMasksVariant drives a reverse miss on
// the flashcard through the real write path and asserts the Relearn reverse card
// never shows the answer: the highlighted variant is blanked, and the variant
// without a highlight (which cannot be masked) is dropped rather than shown raw.
func TestExampleData_FlashcardReverseRelearnMasksVariant(t *testing.T) {
	ctx := context.Background()
	svc := newExampleService(t, t.TempDir())

	reverse, err := svc.LoadReverseCards(0, []string{"vocabulary"}, false, true, nil)
	require.NoError(t, err)
	var rc *ReverseCard
	for i := range reverse {
		if reverse[i].Expression == flashcardMaskWord {
			rc = &reverse[i]
		}
	}
	require.NotNil(t, rc, "reverse quiz must serve %q", flashcardMaskWord)
	// The normal reverse quiz shows only the maskable (highlighted) example.
	require.Len(t, rc.Contexts, 1)
	assert.Equal(t, "She ______ at dinner.", rc.Contexts[0].MaskedContext)

	require.NoError(t, svc.SaveReverseResult(ctx, 0, *rc, GradeResult{Correct: false, Quality: 0}, 1000))
	pool, err := svc.LoadRelearnPool(0, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)

	card := relearnCardFor(pool, flashcardMaskWord)
	require.NotNil(t, card, "reverse-missed %q must be in the relearn pool", flashcardMaskWord)
	require.Equal(t, notebook.QuizTypeReverse, card.Format)

	require.Len(t, card.Contexts, 1, "only the highlighted example may be shown; the unmaskable variant is dropped")
	for _, c := range card.Contexts {
		masked := strings.ToLower(c.MaskedContext)
		assert.Contains(t, masked, "______")
		assert.NotContains(t, masked, "whole cat")
		assert.NotContains(t, masked, flashcardMaskWord)
		assert.NotEqual(t, flashcardMaskNoHighlight, c.Context)
	}
	assert.Equal(t, card.Contexts, card.reverseCard.Contexts)
}

func TestReverseHintContext_NothingMaskedIsDropped(t *testing.T) {
	_, ok := reverseHintContext("Nothing relevant here.", flashcardMaskWord, "", "")
	assert.False(t, ok)

	_, ok = reverseHintContext(flashcardMaskNoHighlight, flashcardMaskWord, "", "")
	assert.False(t, ok, "a variant surface form with no highlight must not be shown unmasked")

	rc, ok := reverseHintContext(flashcardMaskHighlighted, flashcardMaskWord, "", "let the whole cat out of the bag")
	require.True(t, ok)
	assert.Equal(t, "She ______ at dinner.", rc.MaskedContext)
}

// TestRelearnMaskedContexts_HighlightWinsOverContext pins that a sentence present
// in both Contexts (no highlight) and Examples (with highlight) is masked using
// the highlight rather than dropped.
func TestRelearnMaskedContexts_HighlightWinsOverContext(t *testing.T) {
	got := relearnMaskedContexts(FreeformCard{
		Expression: flashcardMaskWord,
		Contexts:   []inference.Context{{Context: flashcardMaskHighlighted}},
		Examples:   []Example{{Text: flashcardMaskHighlighted, Highlight: "let the whole cat out of the bag"}},
	})
	require.Len(t, got, 1)
	assert.Equal(t, "She ______ at dinner.", got[0].MaskedContext)
}
