package quiz

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// containsFold reports a case-insensitive substring match.
func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

// TestExampleData_DefinitionsExamplesShownInRelearn pins that a definitions-book
// word — whose usage sentences live in `examples: [{text, highlight}]` and which
// has NO story scene prose — shows its example sentence in Relearn, driven end to
// end through the real Service/Reader from the examples/ tree:
//
//   - while ASKING (reverse), the card carries a MASKED example: the sentence with
//     the target word blanked (never revealing the answer), sourced from Examples.
//   - in FEEDBACK (ContextScenes), the card carries the FULL example text.
//
// Covered for BOTH the origin family card (rapidity → rapere, origin-bearing) and
// the single reverse card (abate, no origin_parts). abate's example uses the
// inflected form "abated" with an explicit highlight, proving reverse masking
// uses the per-example highlight (masking the lemma "abate" alone would leak it).
func TestExampleData_DefinitionsExamplesShownInRelearn(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name       string
		word       string
		answerWord string // the surface form that must be hidden in the masked hint
		wantFormat notebook.QuizType
	}{
		{"origin family card", "rapidity", "rapidity", notebook.QuizTypeEtymologyOrigin},
		{"single reverse card", "abate", "abated", notebook.QuizTypeReverse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newExampleService(t, t.TempDir())

			reverse, err := svc.LoadReverseCards([]string{"latin-roots-book"}, false, true, nil)
			require.NoError(t, err)
			var rc *ReverseCard
			for i := range reverse {
				if reverse[i].Expression == tc.word {
					rc = &reverse[i]
				}
			}
			require.NotNilf(t, rc, "reverse quiz must serve %q", tc.word)

			require.NoError(t, svc.SaveReverseResult(ctx, *rc, GradeResult{Correct: false, Quality: 0}, 1000))
			pool, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
			require.NoError(t, err)

			card := relearnCardFor(pool, tc.word)
			require.NotNilf(t, card, "reverse-missed %q must be in the relearn pool", tc.word)
			require.Equalf(t, tc.wantFormat, card.Format, "unexpected relearn format for %q", tc.word)
			if tc.wantFormat == notebook.QuizTypeEtymologyOrigin {
				require.Equal(t, notebook.QuizTypeReverse, card.Direction, "reverse direction preserved")
			}

			// (a) While asking: a MASKED example is present, the blank is there,
			// and the answer word is NOT revealed anywhere in the masked hint.
			require.NotEmptyf(t, card.Contexts, "%q must carry a masked usage hint while asking", tc.word)
			var maskedExample *ReverseContext
			for i := range card.Contexts {
				if containsFold(card.Contexts[i].Context, tc.word) || containsFold(card.Contexts[i].Context, tc.answerWord) {
					maskedExample = &card.Contexts[i]
				}
			}
			require.NotNilf(t, maskedExample, "the example sentence for %q must be among the masked contexts", tc.word)
			assert.Containsf(t, maskedExample.MaskedContext, "______", "the example must be blanked for %q", tc.word)
			assert.NotContainsf(t, strings.ToLower(maskedExample.MaskedContext), strings.ToLower(tc.answerWord),
				"the masked hint must NOT reveal the answer %q", tc.answerWord)
			// The unmasked Context keeps the full sentence.
			assert.Truef(t, containsFold(maskedExample.Context, tc.answerWord),
				"the unmasked context should keep the full sentence for %q", tc.word)

			// (b) In feedback: the FULL example text (answer visible) is present.
			var feedbackStatements []string
			for _, sc := range card.ContextScenes {
				feedbackStatements = append(feedbackStatements, sc.Statements...)
			}
			require.NotEmptyf(t, feedbackStatements, "%q feedback must carry the example sentence", tc.word)
			foundFull := false
			for _, s := range feedbackStatements {
				if containsFold(s, tc.answerWord) {
					foundFull = true
				}
			}
			assert.Truef(t, foundFull, "feedback must show the FULL example (with %q) for %q", tc.answerWord, tc.word)
		})
	}
}
