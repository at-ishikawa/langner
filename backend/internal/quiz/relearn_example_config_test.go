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
	"github.com/at-ishikawa/langner/internal/inference"
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
		inference.StaticResolver(mock_inference.NewMockClient(ctrl)),
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
			reverse, err := svc.LoadReverseCards(0, []string{"roots-demo"}, false, true, nil)
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

			require.NoError(t, svc.SaveReverseResult(ctx, 0, *rc, GradeResult{Correct: false, Quality: 0}, 1000))
			pool, err := svc.LoadRelearnPool(0, time.Now().Add(-24*time.Hour))
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

	reverse, err := svc.LoadReverseCards(0, []string{"roots-demo"}, false, true, nil)
	require.NoError(t, err)
	forward, err := svc.LoadCards(0, []string{"roots-demo"}, true, nil)
	require.NoError(t, err)
	all, err := svc.LoadAllWords(0)
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

// TestExampleData_OriginGroupingIndependentOfEtymologyOriginSkip is the durable
// regression guard for the vestigial-marker bug (it replaces the temporary
// origin-grouping diagnostic logging that first exposed it). The property it pins:
//
//	an origin-bearing word groups in Relearn under its ROOT regardless of any
//	per-quiz-type skip state — in BOTH drill directions.
//
// The symptom it reproduces: a word whose origin resolves in the normal quiz
// (primaryOrigin=true) was nevertheless shown as a PLAIN card in Relearn, with no
// origin, whenever it carried a skipped_at["etymology_origin"] marker. Since #41
// that marker is VESTIGIAL — the standalone etymology-origin quiz is gone, Relearn
// has no Exclude control, and nothing deliberately sets or clears the marker; the
// only writers left are the removed quiz and the old "Don't Know" bug
// (4c7fd4de/991816dd). Honoring it stranded real words as plain cards forever.
//
// The marker is written through the SAME write path that produced it on the
// user's disk (SkipWord / SetSkippedAt → skipped_at["etymology_origin"]), driven
// through the real Service/Reader. This is exactly the "seed legacy learning
// history STATE left by a removed feature" case: example notebooks alone (which
// carry no such marker) never reproduce it. The normal-quiz skip filtering
// (notebook/reverse/freeform skipped_at) is a separate path and is intentionally
// NOT exercised or changed here.
func TestExampleData_OriginGroupingIndependentOfEtymologyOriginSkip(t *testing.T) {
	ctx := context.Background()

	// recordMiss drives a real miss of `expr` in the given direction through the
	// real Service, so the Relearn pool sees a genuine misunderstood log for the
	// series that direction replays (reverse -> ReverseLogs, recognition ->
	// LearnedLogs).
	recordMiss := func(t *testing.T, svc *Service, direction notebook.QuizType, expr string) {
		t.Helper()
		switch direction {
		case notebook.QuizTypeReverse:
			cards, err := svc.LoadReverseCards(0, []string{"roots-demo"}, false, true, nil)
			require.NoError(t, err)
			for i := range cards {
				if cards[i].Expression == expr {
					require.NotEmpty(t, cards[i].WordDetail.OriginParts, "normal quiz shows origin (primaryOrigin=true)")
					require.NoError(t, svc.SaveReverseResult(ctx, 0, cards[i], GradeResult{Correct: false, Quality: 0}, 1000))
					return
				}
			}
		case notebook.QuizTypeNotebook:
			cards, err := svc.LoadCards(0, []string{"roots-demo"}, true, nil)
			require.NoError(t, err)
			for i := range cards {
				if cards[i].Entry == expr {
					require.NotEmpty(t, cards[i].WordDetail.OriginParts, "normal quiz shows origin (primaryOrigin=true)")
					require.NoError(t, svc.SaveResult(ctx, 0, cards[i], GradeResult{Correct: false, Quality: 0}, 1000))
					return
				}
			}
		}
		t.Fatalf("word %q not served in direction %q", expr, direction)
	}

	for _, tc := range []struct {
		name      string
		direction notebook.QuizType
	}{
		{"reverse", notebook.QuizTypeReverse},
		{"recognition", notebook.QuizTypeNotebook},
	} {
		t.Run(tc.name, func(t *testing.T) {
			learningDir := t.TempDir()
			svc := newExampleService(t, learningDir)

			// Set the vestigial etymology_origin skip marker on "deficient" via the
			// real write path, and confirm it landed on disk.
			require.NoError(t, svc.SkipWord(0,
				CardInfo{NotebookName: "roots-demo", Expression: "deficient"},
				"", []notebook.QuizType{notebook.QuizTypeEtymologyOrigin}))
			histories, err := notebook.NewLearningHistories(learningDir)
			require.NoError(t, err)
			require.True(t,
				notebook.IsExpressionExcludedForQuizType(histories["roots-demo"], "", notebook.QuizTypeEtymologyOrigin, "deficient"),
				"precondition: the vestigial etymology_origin marker is on disk")

			recordMiss(t, svc, tc.direction, "deficient") // marked word
			recordMiss(t, svc, tc.direction, "transact")  // unmarked control

			pool, err := svc.LoadRelearnPool(0, time.Now().Add(-24*time.Hour))
			require.NoError(t, err)

			// The marked word groups under its root DESPITE the vestigial marker,
			// drilled in the direction it was missed.
			marked := relearnCardFor(pool, "deficient")
			require.NotNil(t, marked, "a miss never drops a word from Relearn")
			assert.Equalf(t, notebook.QuizTypeEtymologyOrigin, marked.Format,
				"grouping must be independent of etymology_origin skip state: deficient folds into its origin family card")
			assert.Equal(t, "facere", marked.OriginText)
			assert.Equal(t, tc.direction, marked.Direction, "drilled in the direction it was missed")

			// The unmarked sibling groups too (control).
			sibling := relearnCardFor(pool, "transact")
			require.NotNil(t, sibling)
			assert.Equal(t, notebook.QuizTypeEtymologyOrigin, sibling.Format)
			assert.Equal(t, "agere", sibling.OriginText)
		})
	}
}
