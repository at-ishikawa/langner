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

// These tests drive the REAL Service/Reader built from examples/ through
// config.example.yml's directories (see newExampleService / exampleNotebooksConfig)
// and seed the leftover learning-history STATE that triggers each bug on disk — no
// hand-built card. They are the reproductions for the two spaced-repetition
// scheduling bugs, kept as regression guards.
//
// Deterministic time: NeedsReverseReview compares learned_at+interval_days against
// time.Now() and has no clock seam; the whole codebase's SR tests pin behavior with
// timestamps RELATIVE to now (e.g. relearn_standard_quiz_fixes_test.go backdates by
// -72h). These follow the same idiom — a 90-day interval reviewed 2 days ago is
// firmly inside its interval (not due) and one reviewed 91 days ago is firmly past
// it (due) — which exercises the exact 2026-08-16 (excluded) / 2026-11-12 (included)
// boundary the report describes without an invasive clock refactor.

// newExampleServiceShuffleDisabled builds the example Service with a caller-chosen
// DisableShuffle so a test can exercise the flashcard reverse due-check, which the
// loader bypasses only when shuffle is disabled (test mode).
func newExampleServiceWithShuffle(t *testing.T, learningDir string, disableShuffle bool) *Service {
	t.Helper()
	ctrl := gomock.NewController(t)
	return NewService(
		exampleNotebooksConfig(t, learningDir),
		mock_inference.NewMockClient(ctrl),
		make(map[string]rapidapi.Response),
		learning.NewYAMLLearningRepository(learningDir, nil),
		config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}, DisableShuffle: disableShuffle},
	)
}

// TestExampleData_ReverseEligibilityIsTrackPure pins BUG 2: reverse-quiz
// eligibility must be computed ONLY from the reverse track. A word answered
// correctly only in REVERSE (a 90-day interval set 2 days ago) is inside its
// reverse interval and must NOT be re-asked in reverse — even with the
// includeUnstudied toggle on — and must reappear once its interval has elapsed.
//
// Before the fix the story/flashcard reverse gates decided "studied" from the
// FORWARD track (HasFreeformAnswer / HasAnyCorrectAnswer), so a reverse-only word
// looked pristine and, under includeUnstudied, was served regardless of its
// reverse due-date (the two tracks crossed). Covered on the story path (always
// due-checked) and the flashcard path (due-checked only with shuffle enabled).
func TestExampleData_ReverseEligibilityIsTrackPure(t *testing.T) {
	// reverseOnlySeed writes a single expression whose ONLY history is one correct
	// reverse log (interval 90) `daysAgo` in the past, in the on-disk shape the real
	// reverse write path produces. No forward/freeform answer — the crux of the bug.
	reverseOnlySeed := func(metadata, scene, expr string, daysAgo int) string {
		at := time.Now().Add(-time.Duration(daysAgo) * 24 * time.Hour).UTC().Format(time.RFC3339)
		if scene == "" { // flashcard shape: flat expressions under the notebook
			return "- metadata:\n    title: " + metadata + "\n  expressions:\n" +
				"    - expression: " + expr + "\n" +
				"      reverse_logs:\n" +
				"        - status: understood\n" +
				"          learned_at: \"" + at + "\"\n" +
				"          quality: 5\n" +
				"          quiz_type: reverse\n" +
				"          interval_days: 90\n"
		}
		return "- metadata:\n    title: " + metadata + "\n  scenes:\n" +
			"    - metadata:\n        title: " + scene + "\n      expressions:\n" +
			"        - expression: " + expr + "\n" +
			"          reverse_logs:\n" +
			"            - status: understood\n" +
			"              learned_at: \"" + at + "\"\n" +
			"              quality: 5\n" +
			"              quiz_type: reverse\n" +
			"              interval_days: 90\n"
	}

	served := func(t *testing.T, svc *Service, notebookID, word string) bool {
		t.Helper()
		cards, err := svc.LoadReverseCards([]string{notebookID}, false, true, nil) // includeUnstudied=true
		require.NoError(t, err)
		for _, c := range cards {
			if c.Expression == word {
				return true
			}
		}
		return false
	}

	t.Run("story path", func(t *testing.T) {
		// "hang out" occurs once in the Friends example (unlike "break the ice"),
		// so the deduped reverse card is the one this seed's history covers.
		const nb, file = "friends", "friends.yml"
		const meta, scene, word = "Friends S01E01 - The Pilot", "Central Perk - Morning Coffee", "hang out"

		learningDir := t.TempDir()
		svc := newExampleService(t, learningDir)

		require.NoError(t, os.WriteFile(filepath.Join(learningDir, file),
			[]byte(reverseOnlySeed(meta, scene, word, 2)), 0o644))
		assert.False(t, served(t, svc, nb, word),
			"a reverse-only word inside its 90-day reverse interval must NOT be re-asked in reverse, even with includeUnstudied")

		require.NoError(t, os.WriteFile(filepath.Join(learningDir, file),
			[]byte(reverseOnlySeed(meta, scene, word, 91)), 0o644))
		assert.True(t, served(t, svc, nb, word),
			"once its reverse interval has elapsed the word is due for reverse again")
	})

	t.Run("flashcard path", func(t *testing.T) {
		const nb, file = "vocabulary", "vocabulary.yml"
		const meta, word = "English Vocabulary Examples", "ephemeral"

		learningDir := t.TempDir()
		// Flashcard reverse due-check runs only when shuffle is NOT disabled.
		svc := newExampleServiceWithShuffle(t, learningDir, false)

		require.NoError(t, os.WriteFile(filepath.Join(learningDir, file),
			[]byte(reverseOnlySeed(meta, "", word, 2)), 0o644))
		assert.False(t, served(t, svc, nb, word),
			"a reverse-only flashcard inside its 90-day reverse interval must NOT be re-asked in reverse, even with includeUnstudied")

		require.NoError(t, os.WriteFile(filepath.Join(learningDir, file),
			[]byte(reverseOnlySeed(meta, "", word, 91)), 0o644))
		assert.True(t, served(t, svc, nb, word),
			"once its reverse interval has elapsed the flashcard is due for reverse again")
	})
}

// TestExampleData_RelearnReAskDedupsByExpression pins BUG 1: the end-of-session
// re-ask round (the Relearn pool) must contain EXACTLY the set of expressions
// failed this session, deduped by expression — K failed → K re-ask cards.
//
// A freeform miss mirror-writes BOTH the recognition (LearnedLogs) and reverse
// (ReverseLogs) series, so before the fix ONE failed word produced TWO relearn
// cards (recognition + reverse); 4 failed words → 8 cards. Driven end to end
// through the real freeform save path and LoadRelearnPool.
func TestExampleData_RelearnReAskDedupsByExpression(t *testing.T) {
	ctx := context.Background()
	// Plain (origin-free) flashcard words, so each miss is a plain vocab card, not
	// an origin family card (origin words already fold to one card by a separate
	// path). Failing more than one proves the count scales 1:1 with failures.
	words := []string{"serendipity", "ephemeral", "ubiquitous", "juxtapose"}

	for _, K := range []int{0, 1, 4} {
		learningDir := t.TempDir()
		svc := newExampleService(t, learningDir)
		all, err := svc.LoadAllWords()
		require.NoError(t, err)

		failed := map[string]bool{}
		for _, w := range words[:K] {
			var card *FreeformCard
			for i := range all {
				if all[i].Expression == w {
					card = &all[i]
				}
			}
			require.NotNilf(t, card, "example flashcard %q must load", w)
			require.NoError(t, svc.SaveFreeformResult(ctx, *card, FreeformGradeResult{Correct: false, Quality: 1}, 1000))
			failed[w] = true
		}

		pool, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
		require.NoError(t, err)

		require.Lenf(t, pool, K,
			"re-ask round must hold exactly the %d expressions failed this session, deduped by expression (got %d cards)", K, len(pool))

		byExpr := map[string]int{}
		for _, c := range pool {
			byExpr[c.Entry]++
			assert.Truef(t, failed[c.Entry], "re-ask card %q was not failed this session (no due-date selection may feed the round)", c.Entry)
			assert.Equalf(t, notebook.QuizTypeReverse, c.Format,
				"a freeform miss re-drills in reverse (the stronger recall test), once")
		}
		for w := range failed {
			assert.Equalf(t, 1, byExpr[w], "%q must appear exactly once in the re-ask round", w)
		}
	}
}

// reverseCountFor returns the start-screen reverse "words available" count
// (NotebookSummary.ReverseReviewCount) for one notebook, with includeUnstudied=true
// (the toggle state the user reported the bug under).
func reverseCountFor(t *testing.T, svc *Service, notebookID string) int {
	t.Helper()
	summaries, err := svc.LoadNotebookSummaries(true)
	require.NoError(t, err)
	for i := range summaries {
		if summaries[i].NotebookID == notebookID {
			return summaries[i].ReverseReviewCount
		}
	}
	t.Fatalf("notebook %q not found in summaries", notebookID)
	return 0
}

// TestExampleData_ReverseReviewCountExcludesNotDueReverseOnly pins the START-SCREEN
// surface of the same track-purity defect (the user's actual bug 1): the reverse
// quiz's "words available" count (NotebookSummary.ReverseReviewCount) inflated by
// including a mature, NOT-due, reverse-only word. That count flows through
// countReverseStoryDefinitions / countReverseFlashcardCards / countDefinitionNotes,
// which call the SAME reverse gates reverseSeriesDue now governs.
//
// For each path: measure the baseline count (all example words pristine → all
// counted when includeUnstudied=true), then seed ONE word as reverse-only and NOT
// due (mature reverse_logs[0]: understood, interval 90, reviewed ~2 days ago, with
// NO forward/freeform log — the crux). A track-pure count drops by exactly one; a
// forward-keyed count leaves it counted (inflated).
//
// Before the fix the story and flashcard counts FAIL here (the not-due reverse-only
// word is still counted, so seeded == baseline). The definitions count already used
// HasAnyCorrectAnswerInAnyDirection and was correct; its assertion locks that in.
func TestExampleData_ReverseReviewCountExcludesNotDueReverseOnly(t *testing.T) {
	// reverseOnlyNotDue writes one expression whose ONLY history is a mature,
	// not-due reverse log (interval 90, reviewed 2 days ago), in the real on-disk
	// shape. scene=="" selects the flat flashcard shape; otherwise the scene shape.
	reverseOnlyNotDue := func(metadata, scene, expr, id string) string {
		at := time.Now().Add(-2 * 24 * time.Hour).UTC().Format(time.RFC3339)
		idLine := ""
		if id != "" {
			idLine = "          id: " + id + "\n"
		}
		if scene == "" { // flashcard: flat expressions under the notebook
			flatID := ""
			if id != "" {
				flatID = "      id: " + id + "\n"
			}
			return "- metadata:\n    title: " + metadata + "\n  expressions:\n" +
				"    - expression: " + expr + "\n" + flatID +
				"      reverse_logs:\n" +
				"        - status: understood\n" +
				"          learned_at: \"" + at + "\"\n" +
				"          quality: 5\n" +
				"          quiz_type: reverse\n" +
				"          interval_days: 90\n"
		}
		return "- metadata:\n    title: " + metadata + "\n  scenes:\n" +
			"    - metadata:\n        title: " + scene + "\n      expressions:\n" +
			"        - expression: " + expr + "\n" + idLine +
			"          reverse_logs:\n" +
			"            - status: understood\n" +
			"              learned_at: \"" + at + "\"\n" +
			"              quality: 5\n" +
			"              quiz_type: reverse\n" +
			"              interval_days: 90\n"
	}

	for _, tc := range []struct {
		name              string
		notebookID, file  string
		metadata, scene   string
		word, id          string
		expectDropWithFix bool // story/flashcard: fixed; definitions: already correct
	}{
		{"story", "friends", "friends.yml", "Friends S01E01 - The Pilot", "Central Perk - Morning Coffee", "hang out", "", true},
		{"flashcard", "vocabulary", "vocabulary.yml", "English Vocabulary Examples", "", "ephemeral", "", true},
		{"definitions", "roots-demo", "roots-demo.yml", "Root Words", "fac / ag", "deficient", "deficient-demo", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			learningDir := t.TempDir()
			svc := newExampleService(t, learningDir)

			baseline := reverseCountFor(t, svc, tc.notebookID)
			require.Greaterf(t, baseline, 0, "precondition: %q has due reverse words to count", tc.notebookID)

			require.NoError(t, os.WriteFile(filepath.Join(learningDir, tc.file),
				[]byte(reverseOnlyNotDue(tc.metadata, tc.scene, tc.word, tc.id)), 0o644))
			seeded := reverseCountFor(t, svc, tc.notebookID)

			assert.Equalf(t, baseline-1, seeded,
				"a mature NOT-due reverse-only word must NOT inflate the reverse start-screen count (baseline %d, got %d)", baseline, seeded)
		})
	}
}

// reverseCardsContain reports whether the served reverse cards include word.
func reverseCardsContain(cards []ReverseCard, word string) bool {
	for i := range cards {
		if cards[i].Expression == word {
			return true
		}
	}
	return false
}

// TestExampleData_ReverseReviewCountEqualsLoaded pins the start-screen invariant
// display = reality: the reverse "words available" badge
// (NotebookSummary.ReverseReviewCount) MUST equal the number of cards the reverse
// quiz actually serves (len(LoadReverseCards(...))) for the same notebook +
// includeUnstudied. The badge over-counted because the count helpers
// (countReverseStoryDefinitions / countReverseFlashcardCards) omitted the reverse
// skipped_at filter that the loaders (loadStoryReverseCards / loadFlashcardReverseCards)
// apply — so a word excluded from the reverse quiz still inflated the badge (14 vs 12).
//
// The STORY subtest is the discriminator: it (1) asserts count == loaded on the
// pristine notebook, (2) excludes one served word from the reverse quiz via the
// real SkipWord path (CardInfoFromReverseCard — exactly what the Exclude button
// sends), then (3) asserts count still == loaded AND both dropped by exactly one
// (the skipped word is gone from both). Before the fix step 3 FAILS: the count
// ignored the skip, so it stayed at the baseline while the loader dropped the word
// (count > loaded).
//
// A general sweep then asserts count == loaded for EVERY example notebook (stories,
// flashcards, and definitions books — whose count, countDefinitionNotes, already
// folds concepts and applies skip), locking the invariant in across the board.
// Concept-collapse is definitions-only (buildConceptIndex reads definitions books),
// so the story/flashcard skip filter is the whole story/flashcard gap.
func TestExampleData_ReverseReviewCountEqualsLoaded(t *testing.T) {
	loadedLen := func(t *testing.T, svc *Service, id string) int {
		t.Helper()
		cards, err := svc.LoadReverseCards([]string{id}, false, true, nil)
		require.NoError(t, err)
		return len(cards)
	}

	// Story: excluding a served word from the reverse quiz drops BOTH the badge
	// and the loaded set by one, and they stay equal. This is the fix's proof —
	// before the fix the badge kept counting the reverse-excluded word.
	t.Run("story: reverse-excluded word drops count and loaded together", func(t *testing.T) {
		const notebookID, word = "friends", "hang out"
		svc := newExampleService(t, t.TempDir())

		base := reverseCountFor(t, svc, notebookID)
		cards, err := svc.LoadReverseCards([]string{notebookID}, false, true, nil)
		require.NoError(t, err)
		require.Equalf(t, len(cards), base, "precondition: count must equal loaded before any skip")
		require.Truef(t, reverseCardsContain(cards, word), "precondition: %q must be served", word)

		var target *ReverseCard
		for i := range cards {
			if cards[i].Expression == word {
				target = &cards[i]
			}
		}
		require.NotNil(t, target)
		require.NoError(t, svc.SkipWord(CardInfoFromReverseCard(*target), "", []notebook.QuizType{notebook.QuizTypeReverse}))

		after := reverseCountFor(t, svc, notebookID)
		assert.Equalf(t, loadedLen(t, svc, notebookID), after,
			"count must equal loaded after excluding a reverse word (display = reality)")
		assert.Equalf(t, base-1, after,
			"a reverse-skipped word must drop the badge by one (it is no longer served)")
		postCards, err := svc.LoadReverseCards([]string{notebookID}, false, true, nil)
		require.NoError(t, err)
		assert.Falsef(t, reverseCardsContain(postCards, word), "the excluded word must not be served")
	})

	// Flashcard: the invariant count == loaded holds — and keeps holding through a
	// SkipWord attempt because the count and loader apply the identical
	// isExpressionSkippedInHistory. NOTE: a flashcard reverse card hardcodes
	// StoryTitle "flashcards", so SkipWord stores the marker under
	// Metadata.Title="flashcards" while the reverse loaders look it up by the
	// notebook's real title — so the flashcard reverse Exclude does not currently
	// take effect (a SEPARATE, pre-existing exclude bug, flagged as follow-up).
	// Either way the badge tracks the loader, which is what this pins.
	t.Run("flashcard: count == loaded through a skip attempt", func(t *testing.T) {
		const notebookID, word = "vocabulary", "serendipity"
		svc := newExampleService(t, t.TempDir())

		base := reverseCountFor(t, svc, notebookID)
		require.Equalf(t, loadedLen(t, svc, notebookID), base, "count == loaded on the pristine flashcard notebook")

		cards, err := svc.LoadReverseCards([]string{notebookID}, false, true, nil)
		require.NoError(t, err)
		var target *ReverseCard
		for i := range cards {
			if cards[i].Expression == word {
				target = &cards[i]
			}
		}
		require.NotNil(t, target)
		require.NoError(t, svc.SkipWord(CardInfoFromReverseCard(*target), "", []notebook.QuizType{notebook.QuizTypeReverse}))

		assert.Equalf(t, loadedLen(t, svc, notebookID), reverseCountFor(t, svc, notebookID),
			"count must equal loaded after a skip attempt (both apply the same filter)")
	})

	// General invariant across every example notebook: the badge equals the served
	// count. Covers definitions books (concepts + skip via countDefinitionNotes).
	t.Run("all example notebooks: count == loaded", func(t *testing.T) {
		svc := newExampleService(t, t.TempDir())
		summaries, err := svc.LoadNotebookSummaries(true)
		require.NoError(t, err)
		checked := 0
		for _, s := range summaries {
			if s.ReverseReviewCount == 0 {
				continue
			}
			assert.Equalf(t, loadedLen(t, svc, s.NotebookID), s.ReverseReviewCount,
				"notebook %q: ReverseReviewCount must equal len(LoadReverseCards)", s.NotebookID)
			checked++
		}
		require.Greater(t, checked, 0, "at least one example notebook must have reverse cards to check")
	})
}
