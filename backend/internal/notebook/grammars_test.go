package notebook

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDerivedCorrectionID_SlugifyCollision documents the raw collision source
// the uniqueness pass exists to resolve: two DIFFERENT entry titles that
// slugify to the same string derive the SAME correction id at the same scene
// index + sequence. Without disambiguation both corrections would share one
// learning-log series (L1 violation).
func TestDerivedCorrectionID_SlugifyCollision(t *testing.T) {
	a := DerivedCorrectionID("journal", "Week 16", 0, 1)
	b := DerivedCorrectionID("journal", "Week-16", 0, 1)
	c := DerivedCorrectionID("journal", "WEEK.16", 0, 1)
	assert.Equal(t, "journal-week-16-s0-1", a)
	assert.Equal(t, a, b, "distinct titles that slugify alike collide today")
	assert.Equal(t, a, c, "distinct titles that slugify alike collide today")
}

// TestCorrectionID_ExplicitIDReuse documents the second raw collision source:
// two corrections given the SAME explicit id: return the same id verbatim.
func TestCorrectionID_ExplicitIDReuse(t *testing.T) {
	x := CorrectionID("journal", "Note 1", 0, 1, Correction{ID: "dup", Incorrect: "the John"})
	y := CorrectionID("journal", "Note 2", 3, 9, Correction{ID: "dup", Incorrect: "a apple"})
	assert.Equal(t, "dup", x)
	assert.Equal(t, x, y, "reused explicit ids collide today")
}

// idOf returns the frozen/assigned id of a correction after the uniqueness
// pass, by content, from a title→scene→corrections structure.
func idOf(byTitle map[string]map[int][]Correction, incorrect string) string {
	for _, byScene := range byTitle {
		for _, corrections := range byScene {
			for _, c := range corrections {
				if c.Incorrect == incorrect {
					return c.ID
				}
			}
		}
	}
	return ""
}

func TestEnsureUniqueCorrectionIDs(t *testing.T) {
	t.Run("slugify collision across entries becomes distinct", func(t *testing.T) {
		byTitle := map[string]map[int][]Correction{
			"Week 16": {0: {{Incorrect: "the John", Correct: "John"}}},
			"Week-16": {0: {{Incorrect: "a apple", Correct: "an apple"}}},
		}
		ensureUniqueCorrectionIDs("journal", byTitle)
		id1 := idOf(byTitle, "the John")
		id2 := idOf(byTitle, "a apple")
		assert.NotEmpty(t, id1)
		assert.NotEmpty(t, id2)
		assert.NotEqual(t, id1, id2, "colliding corrections must get distinct ids")
	})

	t.Run("reused explicit id becomes distinct", func(t *testing.T) {
		byTitle := map[string]map[int][]Correction{
			"Note 1": {0: {{ID: "dup", Incorrect: "the John", Correct: "John"}}},
			"Note 2": {0: {{ID: "dup", Incorrect: "a apple", Correct: "an apple"}}},
		}
		ensureUniqueCorrectionIDs("journal", byTitle)
		assert.NotEqual(t, idOf(byTitle, "the John"), idOf(byTitle, "a apple"))
	})

	t.Run("byte-identical duplicate is dropped", func(t *testing.T) {
		byTitle := map[string]map[int][]Correction{
			"Note 1": {0: {
				{ID: "dup", Incorrect: "the John", Correct: "John", Category: "article", Reason: "no article before a name"},
				{ID: "dup", Incorrect: "the John", Correct: "John", Category: "article", Reason: "no article before a name"},
			}},
		}
		ensureUniqueCorrectionIDs("journal", byTitle)
		require.Len(t, byTitle["Note 1"][0], 1, "the identical duplicate is collapsed to one")
		assert.Equal(t, "dup", byTitle["Note 1"][0][0].ID)
	})

	t.Run("non-colliding correction keeps its exact derived id", func(t *testing.T) {
		byTitle := map[string]map[int][]Correction{
			"Note 1": {0: {{Incorrect: "the John", Correct: "John"}}},
		}
		ensureUniqueCorrectionIDs("journal", byTitle)
		// Byte-identical to today's DerivedCorrectionID output: history preserved.
		assert.Equal(t, "journal-note-1-s0-1", idOf(byTitle, "the John"))
	})

	t.Run("explicit non-colliding id is untouched", func(t *testing.T) {
		byTitle := map[string]map[int][]Correction{
			"Note 1": {0: {{ID: "note-the-john", Incorrect: "the John", Correct: "John"}}},
		}
		ensureUniqueCorrectionIDs("journal", byTitle)
		assert.Equal(t, "note-the-john", idOf(byTitle, "the John"))
	})

	t.Run("residual duplicate id is warned, not fatal", func(t *testing.T) {
		var buf bytes.Buffer
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
		t.Cleanup(func() { slog.SetDefault(prev) })

		// A hand-built structure with two corrections already sharing an id (as
		// if a content-hash clash slipped past disambiguation) must be reported,
		// not panic the load.
		byTitle := map[string]map[int][]Correction{
			"Note 1": {0: {
				{ID: "same", Incorrect: "the John", Correct: "John"},
				{ID: "same", Incorrect: "a apple", Correct: "an apple"},
			}},
		}
		warnResidualDuplicateCorrectionIDs("journal", byTitle)
		assert.Contains(t, buf.String(), "duplicate grammar correction id after disambiguation")
		assert.Contains(t, buf.String(), "same")
	})

	t.Run("disambiguation is order-independent", func(t *testing.T) {
		build := func(first, second Correction) map[string]map[int][]Correction {
			return map[string]map[int][]Correction{
				"Note 1": {0: {first, second}},
			}
		}
		a := Correction{ID: "dup", Incorrect: "the John", Correct: "John"}
		b := Correction{ID: "dup", Incorrect: "a apple", Correct: "an apple"}

		ab := build(a, b)
		ba := build(b, a)
		ensureUniqueCorrectionIDs("journal", ab)
		ensureUniqueCorrectionIDs("journal", ba)

		// The id each correction ends up with is derived from its content, so it
		// is identical regardless of the order the two collide in the file.
		assert.Equal(t, idOf(ab, "the John"), idOf(ba, "the John"))
		assert.Equal(t, idOf(ab, "a apple"), idOf(ba, "a apple"))
		assert.NotEqual(t, idOf(ab, "the John"), idOf(ab, "a apple"))
	})
}
