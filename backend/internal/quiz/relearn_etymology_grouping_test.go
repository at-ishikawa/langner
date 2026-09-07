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
	"github.com/at-ishikawa/langner/internal/learning"
	mock_inference "github.com/at-ishikawa/langner/internal/mocks/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// originGroupFixture builds the on-disk shape of an origin-bearing word (an
// etymology origin declared as a scene root + a word that derives from it) and a
// recent miss of that word, so LoadRelearnPool must fold it into an origin family
// card. It exercises three placements of the SAME word, each of which was a real
// code path in the etymology-Relearn work:
//
//   - "defs": the word lives in a definitions book WITH its own origin_parts.
//   - "etym": the word is embedded INSIDE the etymology notebook (config
//     etymology_directories), loaded by appendEtymologyNotebookWords.
//   - "dedup": the word lives in a plain definitions book WITHOUT origin_parts
//     AND is embedded in the etymology notebook WITH origin_parts. The plain
//     entry wins the canonical dedup (L1/L4 — one series per word); the origin
//     must be MERGED onto it or the word never groups (the reported bug).
//
// direction selects the missed quiz type: notebook (recognition) or reverse.
// The etymology origin uses generic public Latin ("facere", to make/do) and the
// derived word "deficient" — ordinary dictionary content, not user data.
func originGroupFixture(t *testing.T, placement string, direction notebook.QuizType) *Service {
	t.Helper()
	dir := t.TempDir()
	etymDir := filepath.Join(dir, "etymology")
	defsDir := filepath.Join(dir, "definitions")
	learningDir := filepath.Join(dir, "learning")

	// The word entry, with origin_parts. The FIRST ref ("de") is deliberately
	// NOT declared as an origin, and "facere" carries a from_form that is an
	// english_form (not a declared form) — the exact shape that must still
	// resolve "facere" as the primary origin.
	wordDef := `  - expression: deficient
    meaning: "Not having enough; lacking"
    part_of_speech: adjective
    note: 'de "down" + facere = made down, made less'
    origin_parts:
      - { origin: de, language: Latin }
      - { origin: facere, language: Latin, from_form: fic }
`

	etymYAML := `metadata:
  title: "fac, fic, fect (to make, do)"
origins:
  - origin: facere
    type: root
    language: Latin
    meaning: "to make, do"
    english_forms: [fac, fact, fic, fect, fy]
`
	if placement == "etym" || placement == "dedup" {
		etymYAML += "definitions:\n" + wordDef
	}
	etymBook := filepath.Join(etymDir, "roots")
	require.NoError(t, os.MkdirAll(etymBook, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(etymBook, "index.yml"), []byte(
		"id: roots\nkind: Etymology\nname: Roots\nnotebooks:\n  - ./s1.yml\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(etymBook, "s1.yml"), []byte(etymYAML), 0644))

	// Which learning-history file / notebook id the miss is recorded under.
	notebookID := "roots"

	switch placement {
	case "defs":
		defsBook := filepath.Join(defsDir, "roots")
		require.NoError(t, os.MkdirAll(defsBook, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(defsBook, "index.yml"), []byte(
			"id: roots\nnotebooks:\n  - ./s1.yml\n"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(defsBook, "s1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
  - metadata:
      index: 0
      title: S1
    expressions:
    - expression: deficient
      id: deficient
      meaning: "Not having enough; lacking"
      note: 'de "down" + facere = made down, made less'
      origin_parts:
        - { origin: de, language: Latin }
        - { origin: facere, language: Latin, from_form: fic }
`), 0644))
	case "dedup":
		// A plain definitions entry (no origin_parts) in a DIFFERENT book wins
		// the dedup; the etymology-notebook copy's origin must merge onto it.
		notebookID = "vocab"
		defsBook := filepath.Join(defsDir, "vocab")
		require.NoError(t, os.MkdirAll(defsBook, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(defsBook, "index.yml"), []byte(
			"id: vocab\nnotebooks:\n  - ./s1.yml\n"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(defsBook, "s1.yml"), []byte(`- metadata:
    title: "Book"
  scenes:
  - metadata:
      index: 0
      title: S1
    expressions:
    - expression: deficient
      id: deficient
      meaning: "Not having enough; lacking"
`), 0644))
	}

	require.NoError(t, os.MkdirAll(learningDir, 0755))
	recent := time.Now().Add(-time.Hour).Format(time.RFC3339)
	series := "learned_logs"
	if direction == notebook.QuizTypeReverse {
		series = "reverse_logs"
	}
	learnYAML := fmt.Sprintf(`- metadata:
    id: %s
    title: "Book"
    type: definition
  expressions:
    - expression: deficient
      id: deficient
      type: vocabulary
      %s:
        - status: misunderstood
          learned_at: "%s"
`, notebookID, series, recent)
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, notebookID+".yml"), []byte(learnYAML), 0644))

	ctrl := gomock.NewController(t)
	return NewService(config.NotebooksConfig{
		EtymologyDirectories:   []string{etymDir},
		DefinitionsDirectories: []string{defsDir},
		LearningNotesDirectory: learningDir,
	}, mock_inference.NewMockClient(ctrl), make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil),
		config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}, DisableShuffle: true})
}

// TestLoadRelearnPool_OriginBearingMissGroupsByOrigin pins that an origin-bearing
// vocabulary word missed in EITHER direction (recognition or reverse) folds into
// its origin family card (QUIZ_TYPE_ETYMOLOGY_ORIGIN), regardless of whether the
// word lives in a definitions book, is embedded in the etymology notebook, or is
// deduped against a plain (origin-less) book entry. A reverse miss keeps its
// produce-the-word direction. Regression for: origin-bearing words that were
// deduped against a plain book entry lost their origin and showed as plain
// standalone reverse/recognition cards instead of grouping.
func TestLoadRelearnPool_OriginBearingMissGroupsByOrigin(t *testing.T) {
	for _, placement := range []string{"defs", "etym", "dedup"} {
		for _, direction := range []notebook.QuizType{notebook.QuizTypeNotebook, notebook.QuizTypeReverse} {
			t.Run(placement+"/"+string(direction), func(t *testing.T) {
				svc := originGroupFixture(t, placement, direction)

				// Exactly one canonical card for the word (L1/L4: one series).
				words, err := svc.LoadAllWords(0)
				require.NoError(t, err)
				count := 0
				for _, w := range words {
					if w.Expression == "deficient" {
						count++
					}
				}
				require.Equal(t, 1, count, "the word must have exactly one canonical card (one log series)")

				pool, err := svc.LoadRelearnPool(0, time.Now().Add(-24 * time.Hour))
				require.NoError(t, err)

				var card *RelearnCard
				for i := range pool {
					if pool[i].Entry == "deficient" {
						card = &pool[i]
					}
				}
				require.NotNil(t, card, "the missed word must be in the relearn pool")
				assert.Equal(t, notebook.QuizTypeEtymologyOrigin, card.Format,
					"an origin-bearing miss must fold into the origin family card, not a plain card")
				assert.Equal(t, "facere", card.OriginText)
				assert.Equal(t, "to make, do", card.OriginMeaning)
				assert.Equal(t, direction, card.Direction,
					"the word must be drilled in the direction it was missed")

				if direction == notebook.QuizTypeReverse {
					// Reverse preserved: the card grades produce-the-word via its
					// reverse grader card, not the meaning grader.
					assert.Equal(t, "deficient", card.ReverseCard().Expression,
						"a reverse-missed origin word must carry the reverse grader card")
				} else {
					assert.Equal(t, "deficient", card.VocabCard().Entry,
						"a recognition-missed origin word must carry the recognition grader card")
				}
			})
		}
	}
}
