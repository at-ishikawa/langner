package quiz

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReverseMask_InflectedHighlight_ExampleConfig is the end-to-end guard for
// the reverse-mask fix, driven through the REAL loader over config.example.yml
// (not a hand-built card, per verify-data-features-with-example-notebooks). The
// roots-untyped definitions book's `subduct` entry carries an example that
// highlights the INFLECTED surface form "subducted"; the loaded reverse card's
// MaskedContext must blank that inflected answer. This exercises the full path
// the server runs — reverseHintContext (highlight mask) then applyForwardMask —
// which is exactly where the leak was (the forward-mask pass discarded the
// highlight mask and re-exposed "subducted").
func TestReverseMask_InflectedHighlight_ExampleConfig(t *testing.T) {
	svc := newExampleServiceWithShuffle(t, t.TempDir(), true)

	summaries, err := svc.LoadNotebookSummaries(0, true)
	require.NoError(t, err)

	var card *ReverseCard
	for _, s := range summaries {
		cards, lerr := svc.LoadReverseCards(0, []string{s.NotebookID}, false, true, nil)
		require.NoError(t, lerr)
		for i := range cards {
			if strings.EqualFold(cards[i].Expression, "subduct") {
				c := cards[i]
				card = &c
				break
			}
		}
		if card != nil {
			break
		}
	}
	require.NotNil(t, card, "the roots-untyped 'subduct' reverse card must load")
	require.NotEmpty(t, card.Contexts, "subduct card must carry its highlighted example")

	// Every shown reverse example must blank the inflected answer — the exact
	// leak the fix prevents.
	for _, c := range card.Contexts {
		require.NotContainsf(t, strings.ToLower(c.MaskedContext), "subducted",
			"inflected answer 'subducted' must be masked in the reverse example (got %q)", c.MaskedContext)
		require.Containsf(t, c.MaskedContext, "______",
			"the answer blank must be present (got %q)", c.MaskedContext)
	}
}
