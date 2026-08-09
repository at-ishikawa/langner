package quiz

import (
	"context"
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

// exampleNotebooksConfig builds a NotebooksConfig pointing at the repo's
// examples/ tree (the exact directories config.example.yml wires) with the
// learning-notes directory redirected to a caller-owned temp dir so a test can
// record a miss without touching the shipped demo data. It reproduces the REAL
// Service/Reader construction the app uses, not a hand-built card.
func exampleNotebooksConfig(t *testing.T, learningDir string) config.NotebooksConfig {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	root := ""
	for dir != "/" {
		if _, err := os.Stat(filepath.Join(dir, "config.example.yml")); err == nil {
			root = dir
			break
		}
		dir = filepath.Dir(dir)
	}
	if root == "" {
		t.Skip("repo root (config.example.yml) not found")
	}
	ex := filepath.Join(root, "examples")
	return config.NotebooksConfig{
		StoriesDirectories:     []string{filepath.Join(ex, "stories")},
		JournalsDirectories:    []string{filepath.Join(ex, "journals")},
		FlashcardsDirectories:  []string{filepath.Join(ex, "flashcards")},
		BooksDirectories:       []string{filepath.Join(ex, "books")},
		DefinitionsDirectories: []string{filepath.Join(ex, "definitions")},
		EtymologyDirectories:   []string{filepath.Join(ex, "etymology")},
		GrammarsDirectories:    []string{filepath.Join(ex, "grammars")},
		LearningNotesDirectory: learningDir,
	}
}

func newExampleService(t *testing.T, learningDir string) *Service {
	t.Helper()
	ctrl := gomock.NewController(t)
	return NewService(
		exampleNotebooksConfig(t, learningDir),
		mock_inference.NewMockClient(ctrl),
		make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil),
		config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}, DisableShuffle: true},
	)
}

func relearnCardFor(pool []RelearnCard, entry string) *RelearnCard {
	for i := range pool {
		if pool[i].Entry == entry {
			return &pool[i]
		}
	}
	return nil
}

// TestExampleData_OriginGroupsInRelearn drives the real example config end to
// end (the exact Service/Reader construction the server uses) to lock the
// origin-family grouping contract for words declared under examples/:
//
//   - deficient / transact carry origin_parts ON their definitions note. Their
//     origin resolves in BOTH the normal reverse quiz (what the user sees) AND
//     in Relearn, and a miss in either direction folds into its origin family
//     card in the direction it was missed.
//
// This is the "reverse miss must group under its root, reverse direction
// preserved, example scene present" contract, exercised through
// LoadReverseCards -> SaveReverseResult -> LoadRelearnPool (no hand-built card).
func TestExampleData_OriginGroupsInRelearn(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		word   string
		origin string
	}{
		{"deficient", "facere"},
		{"transact", "agere"},
	} {
		t.Run(tc.word+"/reverse", func(t *testing.T) {
			svc := newExampleService(t, t.TempDir())
			reverse, err := svc.LoadReverseCards([]string{"roots-demo"}, false, true, nil)
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

// TestExampleData_OriginResolutionParity records, through the real example
// config, WHERE a word's origin resolves across the three quiz surfaces. It
// documents the ground truth uncovered while diagnosing the "origin words don't
// group in Relearn" report:
//
//   - deficient / transact (origin_parts on the note): origin resolves on EVERY
//     surface (reverse, forward, LoadAllWords/relearn).
//   - factory (origin declared ONLY in the etymology notebook, merged onto the
//     plain definitions entry by appendEtymologyNotebookWords): origin resolves
//     in LoadAllWords (freeform + relearn) but NOT in the normal reverse/forward
//     quizzes, because that etymology-origin enrichment runs ONLY in
//     LoadAllWords. This asymmetry is a real divergence; the assertions below
//     PIN the current behavior so a future unification (applying the same
//     enrichment in LoadCards/LoadReverseCards) is a deliberate, visible change.
func TestExampleData_OriginResolutionParity(t *testing.T) {
	svc := newExampleService(t, t.TempDir())

	reverse, err := svc.LoadReverseCards([]string{"roots-demo"}, false, true, nil)
	require.NoError(t, err)
	forward, err := svc.LoadCards([]string{"roots-demo"}, true, nil)
	require.NoError(t, err)
	all, err := svc.LoadAllWords()
	require.NoError(t, err)

	reverseHas := func(expr string) bool {
		for _, c := range reverse {
			if c.Expression == expr {
				return len(c.WordDetail.OriginParts) > 0
			}
		}
		return false
	}
	forwardHas := func(expr string) bool {
		for _, c := range forward {
			if c.Entry == expr {
				return len(c.WordDetail.OriginParts) > 0
			}
		}
		return false
	}
	allHas := func(expr string) bool {
		for _, c := range all {
			if c.Expression == expr && len(c.WordDetail.OriginParts) > 0 {
				return true
			}
		}
		return false
	}

	// origin_parts on the note: resolves everywhere -> groups everywhere.
	for _, w := range []string{"deficient", "transact"} {
		assert.Truef(t, reverseHas(w), "reverse quiz resolves origin for %q", w)
		assert.Truef(t, forwardHas(w), "forward quiz resolves origin for %q", w)
		assert.Truef(t, allHas(w), "LoadAllWords resolves origin for %q", w)
	}

	// factory: origin only in the etymology notebook -> resolves in LoadAllWords
	// (freeform + relearn) via enrichment, but the normal reverse/forward quizzes
	// currently DO NOT enrich. Pinned as the known asymmetry (see doc above).
	assert.True(t, allHas("factory"), "LoadAllWords enriches factory's etymology origin")
	assert.False(t, reverseHas("factory"), "KNOWN GAP: reverse quiz does not enrich etymology origin")
	assert.False(t, forwardHas("factory"), "KNOWN GAP: forward quiz does not enrich etymology origin")
}

// TestExampleData_ExcludeFromOriginFamilyDropsGrouping reproduces the EXACT
// reported symptom and shows its cause: a word whose origin resolves in the
// normal quiz is nevertheless shown as a plain reverse card in Relearn (not
// grouped) IF it carries the deliberate etymology-origin exclusion marker
// (skipped_at["etymology_origin"], written only by the ExcludeEtymologyWord RPC
// behind the etymology browse page's "Exclude from quizzes" control). Relearn
// honors that marker by design (quiz-ui-invariants U1: the word stays due as a
// plain card). This is the ONLY mechanism that yields "normal quiz shows origin,
// Relearn shows a plain reverse card"; there is no origin-resolution divergence.
func TestExampleData_ExcludeFromOriginFamilyDropsGrouping(t *testing.T) {
	ctx := context.Background()
	svc := newExampleService(t, t.TempDir())

	reverse, err := svc.LoadReverseCards([]string{"roots-demo"}, false, true, nil)
	require.NoError(t, err)
	var rc *ReverseCard
	for i := range reverse {
		if reverse[i].Expression == "deficient" {
			rc = &reverse[i]
		}
	}
	require.NotNil(t, rc)
	require.NotEmpty(t, rc.WordDetail.OriginParts, "normal reverse quiz shows origin")

	// The deliberate per-word "Exclude from origin family" action.
	require.NoError(t, svc.SkipWord(CardInfo{
		NotebookName: rc.NotebookName, StoryTitle: rc.StoryTitle, SceneTitle: rc.SceneTitle, Expression: "deficient",
	}, "", []notebook.QuizType{notebook.QuizTypeEtymologyOrigin}))

	require.NoError(t, svc.SaveReverseResult(ctx, *rc, GradeResult{Correct: false, Quality: 0}, 1000))
	pool, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)

	card := relearnCardFor(pool, "deficient")
	require.NotNil(t, card, "the word is still due in Relearn (a miss never drops it) ...")
	assert.Equal(t, notebook.QuizTypeReverse, card.Format,
		"... but excluded from its origin family it shows as a plain reverse card, not grouped")
	assert.Empty(t, card.OriginText)
}
