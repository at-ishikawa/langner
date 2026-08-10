package quiz

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// TestExampleData_NestedShapeOriginGroupsInRelearn reproduces the user's REAL
// notebook shape: a NESTED, join-keyed etymology notebook (event -> scenes ->
// origins) and a definitions notebook joined to it by the SAME `id`
// (latin-roots-book). The cleanest failing case is `rapidity`, whose
// origin_parts is a SINGLE declared origin { rapere, Latin, rap } where `rapere`
// is a declared root.
//
// It drives the REAL Service/Reader built from the examples/ tree (the exact
// construction config.example.yml uses), asserts the normal reverse quiz
// resolves the origin (what the user sees), then a reverse MISS must fold into
// its origin family card in Relearn — the same guarantee the flat-shape
// roots-demo book already meets.
func TestExampleData_NestedShapeOriginGroupsInRelearn(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		word   string
		origin string
	}{
		{"rapidity", "rapere"},    // single declared root — the task's cleanest case
		{"recipient", "capere"},   // re (prefix) + capere (root): must fold under the ROOT
		{"deceptive", "capere"},   // de (prefix) + capere (root): must fold under the ROOT
		{"prologue", "logos"},     // pro (Greek prefix) + logos (Greek root): cross-language
		{"illogical", "logos"},    // in (Latin prefix) + logos (Greek root): cross-language
		{"possessive", "posse"},   // posse root, multi-sense (sense: able)
		{"potent", "potens"},      // potens root, multi-sense (sense: powerful)
		{"dominion", "dominari"},  // dominari root, multi-sense (sense: rule)
	} {
		t.Run(tc.word+"/reverse", func(t *testing.T) {
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
			// Normal quiz shows the origin (the Etymology breakdown the user sees).
			require.NotEmptyf(t, rc.WordDetail.OriginParts,
				"normal reverse quiz must resolve origin for %q", tc.word)

			require.NoError(t, svc.SaveReverseResult(ctx, *rc, GradeResult{Correct: false, Quality: 0}, 1000))
			pool, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
			require.NoError(t, err)

			card := relearnCardFor(pool, tc.word)
			require.NotNilf(t, card, "reverse-missed %q must be in the relearn pool", tc.word)
			assert.Equalf(t, notebook.QuizTypeEtymologyOrigin, card.Format,
				"%q must fold into its origin family card, not a plain reverse card", tc.word)
			assert.Equal(t, tc.origin, card.OriginText)
			assert.Equal(t, notebook.QuizTypeReverse, card.Direction, "reverse direction preserved")
		})
	}
}
