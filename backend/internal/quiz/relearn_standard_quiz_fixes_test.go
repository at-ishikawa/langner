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

// These three tests drive the real Service/Reader built from the examples/ tree
// (the exact construction `make dev` / ./langner-server uses — see
// newExampleService / exampleNotebooksConfig) end to end, one per verifiable
// Relearn fix. No hand-built RelearnCard / FreeformCard is used: every miss goes
// through the real quiz save path and the pool is derived by LoadRelearnPool.

// TestExampleData_ExcludedWordAbsentFromRelearn pins fix #1: a word deliberately
// excluded from a quiz mode (per-quiz-type skipped_at set via the real SkipWord
// path) must NOT appear in the Relearn pool, even though it has a genuine
// "misunderstood" log — matching the normal card loaders (quiz-ui-invariants U1).
// A separate wrong-but-not-excluded sibling stays in the pool (control).
func TestExampleData_ExcludedWordAbsentFromRelearn(t *testing.T) {
	ctx := context.Background()
	learningDir := t.TempDir()
	svc := newExampleService(t, learningDir)

	// Miss deficient AND transact in the reverse quiz through the real path.
	reverse, err := svc.LoadReverseCards([]string{"roots-demo"}, false, true, nil)
	require.NoError(t, err)
	missReverse := func(word string) {
		for i := range reverse {
			if reverse[i].Expression == word {
				require.NoError(t, svc.SaveReverseResult(ctx, reverse[i], GradeResult{Correct: false, Quality: 0}, 1000))
				return
			}
		}
		t.Fatalf("reverse quiz must serve %q", word)
	}
	missReverse("deficient")
	missReverse("transact")

	// Baseline: both missed words are in the pool before any exclusion.
	pool, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)
	require.NotNil(t, relearnCardFor(pool, "deficient"), "precondition: reverse-missed deficient is in the pool")
	require.NotNil(t, relearnCardFor(pool, "transact"), "precondition: reverse-missed transact is in the pool")

	// Exclude deficient from the reverse quiz via the real SkipWord path
	// (the ONLY thing that writes skipped_at). transact stays as the control.
	require.NoError(t, svc.SkipWord(
		CardInfo{NotebookName: "roots-demo", Expression: "deficient"},
		"", []notebook.QuizType{notebook.QuizTypeReverse}))

	pool, err = svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)
	assert.Nil(t, relearnCardFor(pool, "deficient"),
		"an excluded word (skipped_at set) must never enter the Relearn pool")
	assert.NotNil(t, relearnCardFor(pool, "transact"),
		"the non-excluded sibling is still re-drilled")
}

// TestExampleData_EtymologyNotebookExampleShownInRelearn pins fix #2: an
// etymology-notebook-ONLY word that carries a usage sentence in its `examples:`
// shows that sentence on its Relearn origin family card. Before the fix the
// etymology-notebook loader dropped `examples:` entirely (the struct had no
// field), so such a word re-drilled with no example at all. Driven end to end:
// LoadAllWords -> SaveFreeformResult (a real recognition miss) -> LoadRelearnPool.
func TestExampleData_EtymologyNotebookExampleShownInRelearn(t *testing.T) {
	ctx := context.Background()
	svc := newExampleService(t, t.TempDir())

	all, err := svc.LoadAllWords()
	require.NoError(t, err)
	var card *FreeformCard
	for i := range all {
		if all[i].Expression == "benefactor" {
			card = &all[i]
		}
	}
	require.NotNil(t, card, "benefactor (etymology-notebook only) must be loaded as a vocabulary card")
	require.NotEmpty(t, card.Examples, "the etymology-notebook example must survive onto the loaded card")

	require.NoError(t, svc.SaveFreeformResult(ctx, *card, FreeformGradeResult{Correct: false, Quality: 0}, 1000))

	pool, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)
	rc := relearnCardFor(pool, "benefactor")
	require.NotNil(t, rc, "the missed etymology word must be in the Relearn pool")
	require.Equal(t, notebook.QuizTypeEtymologyOrigin, rc.Format, "it folds into its origin family card")
	assert.Equal(t, "facere", rc.OriginText)

	// The example sentence appears in feedback (ContextScenes) AND as a
	// recognition hint (Examples) — it was previously dropped from both.
	var statements []string
	for _, sc := range rc.ContextScenes {
		statements = append(statements, sc.Statements...)
	}
	foundInFeedback := false
	for _, s := range statements {
		if containsFold(s, "benefactor paid for the new library wing") {
			foundInFeedback = true
		}
	}
	assert.True(t, foundInFeedback, "the etymology word's example must show in Relearn feedback")

	// A hint is present while answering, in whichever direction the card is
	// drilled: recognition shows the full example (Examples), reverse shows it
	// masked (Contexts). Both derive from the same example that was dropped
	// before the fix. (A freeform miss records both series, so the word folds
	// reverse — the stronger recall test — hence the masked-context hint here.)
	foundInHint := false
	for _, ex := range rc.Examples {
		if containsFold(ex.Text, "benefactor paid for the new library wing") {
			foundInHint = true
		}
	}
	for i := range rc.Contexts {
		if containsFold(rc.Contexts[i].Context, "benefactor paid for the new library wing") {
			foundInHint = true
		}
	}
	assert.True(t, foundInHint, "the etymology word's example must show as an answering hint")
}

// TestExampleData_GrammarMissStaysDueAcrossWindow pins fix #3: a missed grammar
// correction is re-derived from due-state, NOT the recent-miss window, so it
// keeps reappearing in Relearn every session until it is answered correctly —
// while a vocabulary miss is still window-limited. Both misses are recorded now
// and the pool is queried with a windowStart in the FUTURE (so both logs predate
// it): the grammar correction survives, the vocabulary miss drops out (control).
func TestExampleData_GrammarMissStaysDueAcrossWindow(t *testing.T) {
	ctx := context.Background()
	svc := newExampleService(t, t.TempDir())

	// Miss one grammar correction through the real grammar path.
	posts, err := svc.LoadGrammarPosts("journal", nil)
	require.NoError(t, err)
	require.NotEmpty(t, posts, "the example journal must have due grammar corrections")
	require.NotEmpty(t, posts[0].Blanks)
	blank := posts[0].Blanks[0]
	require.NoError(t, svc.SaveGrammarBlank(ctx, "journal", blank.SenseID, GradeResult{Correct: false, Quality: 0}, 1000))

	// Miss one vocabulary word through the real reverse path (the control).
	reverse, err := svc.LoadReverseCards([]string{"roots-demo"}, false, true, nil)
	require.NoError(t, err)
	missed := false
	for i := range reverse {
		if reverse[i].Expression == "deficient" {
			require.NoError(t, svc.SaveReverseResult(ctx, reverse[i], GradeResult{Correct: false, Quality: 0}, 1000))
			missed = true
		}
	}
	require.True(t, missed, "reverse quiz must serve deficient")

	// windowStart AFTER both misses: both logs are "before the window".
	pool, err := svc.LoadRelearnPool(time.Now().Add(time.Hour))
	require.NoError(t, err)

	var grammar *RelearnCard
	for i := range pool {
		if pool[i].Format == notebook.QuizTypeGrammar && pool[i].Incorrect == blank.Incorrect {
			grammar = &pool[i]
		}
	}
	require.NotNil(t, grammar,
		"a still-misunderstood grammar correction must stay in Relearn regardless of the window")

	assert.Nil(t, relearnCardFor(pool, "deficient"),
		"a vocabulary miss outside the window is NOT re-drilled (window still applies to vocab)")
}

// TestExampleData_ReverseHintNeverRevealsAnswer pins fix: on a REVERSE relearn
// card for an etymology-notebook word, the shown example hint must NEVER reveal
// the answer's surface form.
//
//   - enact: its example uses the inflected "enacted" WITH a highlight naming it,
//     so the reverse hint is shown with "enacted" blanked.
//   - react: its example uses the inflected "reacted" with NO highlight, so the
//     lemma "react" cannot whole-word match it — the leaky example is DROPPED
//     from the reverse hint (never shown unmasked). The full sentence still
//     appears in the FEEDBACK scene (answer visible after grading).
//
// Driven through the real config: LoadAllWords -> SaveFreeformResult (records
// both series, so the pool drills the word in reverse) -> LoadRelearnPool.
func TestExampleData_ReverseHintNeverRevealsAnswer(t *testing.T) {
	ctx := context.Background()

	reverseCardFor := func(t *testing.T, word string) *RelearnCard {
		t.Helper()
		svc := newExampleService(t, t.TempDir())
		all, err := svc.LoadAllWords()
		require.NoError(t, err)
		var card *FreeformCard
		for i := range all {
			if all[i].Expression == word {
				card = &all[i]
			}
		}
		require.NotNilf(t, card, "%q must load", word)
		require.NoError(t, svc.SaveFreeformResult(ctx, *card, FreeformGradeResult{Correct: false, Quality: 1}, 1000))
		pool, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
		require.NoError(t, err)
		rc := relearnCardFor(pool, word)
		require.NotNilf(t, rc, "%q must be in the pool", word)
		require.Equal(t, notebook.QuizTypeEtymologyOrigin, rc.Format)
		require.Equal(t, notebook.QuizTypeReverse, rc.Direction, "a freeform miss drills reverse")
		return rc
	}

	t.Run("inflected example WITH highlight is shown masked", func(t *testing.T) {
		rc := reverseCardFor(t, "enact")
		require.NotEmpty(t, rc.Contexts, "the highlighted example must be shown as a masked hint")
		var found *ReverseContext
		for i := range rc.Contexts {
			if containsFold(rc.Contexts[i].Context, "enacted") {
				found = &rc.Contexts[i]
			}
		}
		require.NotNil(t, found, "the enacted example must be present as a hint")
		assert.Contains(t, found.MaskedContext, "______", "the answer must be blanked")
		assert.NotContains(t, strings.ToLower(found.MaskedContext), "enacted",
			"the reverse hint must NOT reveal the inflected answer")
	})

	t.Run("inflected example WITHOUT highlight is dropped, never shown unmasked", func(t *testing.T) {
		rc := reverseCardFor(t, "react")
		for i := range rc.Contexts {
			assert.NotContainsf(t, strings.ToLower(rc.Contexts[i].MaskedContext), "reacted",
				"a reverse hint must never reveal the answer; the un-highlightable example must be dropped (got %q)",
				rc.Contexts[i].MaskedContext)
		}
		// The full example still exists for the FEEDBACK view (answer visible
		// only after grading), so we didn't lose the sentence entirely.
		var feedback []string
		for _, sc := range rc.ContextScenes {
			feedback = append(feedback, sc.Statements...)
		}
		foundFull := false
		for _, s := range feedback {
			if containsFold(s, "reacted") {
				foundFull = true
			}
		}
		assert.True(t, foundFull, "the full example remains available in feedback")
	})
}

// TestExampleData_OriginCardShowsRelatedFamily pins the post-answer "related
// words from this origin" reference on the Relearn origin card, driven through
// the real example config:
//
//   - a missed word (deficient → facere) produces the origin card;
//   - RelatedWords lists the OTHER facere words with their meanings;
//   - the DRILLED word itself (deficient) is excluded;
//   - a sibling EXCLUDED from quizzes via the real SkipWord path (factory) is
//     STILL listed — the related list is display-only reference, and a word the
//     learner excluded (learned, quizzing off) is exactly a known same-origin
//     word worth showing. Only the quiz/drilled pool filters skipped_at.
//
// Covered for both a recognition miss and a reverse miss (the reference is
// attached in either direction).
func TestExampleData_OriginCardShowsRelatedFamily(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name      string
		direction notebook.QuizType
	}{
		{"recognition", notebook.QuizTypeNotebook},
		{"reverse", notebook.QuizTypeReverse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newExampleService(t, t.TempDir())

			// Exclude a facere sibling from quizzes via the real SkipWord path.
			require.NoError(t, svc.SkipWord(
				CardInfo{NotebookName: "roots-demo", Expression: "factory", ID: "factory-demo"},
				"", []notebook.QuizType{notebook.QuizTypeNotebook}))

			// Miss deficient in the given direction through the real quiz path.
			switch tc.direction {
			case notebook.QuizTypeReverse:
				cards, err := svc.LoadReverseCards([]string{"roots-demo"}, false, true, nil)
				require.NoError(t, err)
				missed := false
				for i := range cards {
					if cards[i].Expression == "deficient" {
						require.NoError(t, svc.SaveReverseResult(ctx, cards[i], GradeResult{Correct: false, Quality: 0}, 1000))
						missed = true
					}
				}
				require.True(t, missed, "reverse quiz must serve deficient")
			default:
				cards, err := svc.LoadCards([]string{"roots-demo"}, true, nil)
				require.NoError(t, err)
				missed := false
				for i := range cards {
					if cards[i].Entry == "deficient" {
						require.NoError(t, svc.SaveResult(ctx, cards[i], GradeResult{Correct: false, Quality: 0}, 1000))
						missed = true
					}
				}
				require.True(t, missed, "standard quiz must serve deficient")
			}

			pool, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
			require.NoError(t, err)
			card := relearnCardFor(pool, "deficient")
			require.NotNil(t, card, "the missed word must be in the pool")
			require.Equal(t, notebook.QuizTypeEtymologyOrigin, card.Format)
			require.Equal(t, "facere", card.OriginText)

			related := map[string]string{}
			for _, m := range card.RelatedWords {
				related[m.Word] = m.Meaning
			}
			// A non-missed sibling appears WITH its meaning.
			require.Contains(t, related, "benefactor", "related family should list the sibling")
			assert.NotEmpty(t, related["benefactor"], "each related word carries a meaning")
			// The drilled word itself is NOT listed (it's the quiz item).
			assert.NotContains(t, related, "deficient", "the drilled word is excluded from related")
			// The skipped_at-excluded sibling IS listed — display-only reference
			// does not hide known words the learner turned quizzing off for.
			assert.Contains(t, related, "factory", "a quiz-excluded sibling is still shown as reference")
			assert.NotEmpty(t, related["factory"], "the excluded sibling carries its meaning")
		})
	}
}

// TestExampleData_RelatedFamilyResolvesOriginUniformly pins the fix for the
// empty/short related-family bug: a definitions word that inlines ONLY a declared
// prefix and leaves the ROOT to the etymology notebook must still fold under the
// root and appear in the origin card's related family — resolved the SAME way as
// siblings that inlined the full origin. Before the fix it folded under the
// prefix ("con") and was missing from the facere family. Driven through the real
// example config (confection: inline [con] in roots-demo, root facere completed
// from latin-verbs).
func TestExampleData_RelatedFamilyResolvesOriginUniformly(t *testing.T) {
	ctx := context.Background()
	svc := newExampleService(t, t.TempDir())

	cards, err := svc.LoadCards([]string{"roots-demo"}, true, nil)
	require.NoError(t, err)
	missed := false
	for i := range cards {
		if cards[i].Entry == "deficient" {
			require.NoError(t, svc.SaveResult(ctx, cards[i], GradeResult{Correct: false, Quality: 0}, 1000))
			missed = true
		}
	}
	require.True(t, missed, "standard quiz must serve deficient")

	pool, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)
	card := relearnCardFor(pool, "deficient")
	require.NotNil(t, card)
	require.Equal(t, "facere", card.OriginText)

	related := map[string]string{}
	for _, m := range card.RelatedWords {
		related[m.Word] = m.Meaning
	}
	assert.Containsf(t, related, "confection",
		"a word whose root is completed via the etymology notebook must fold under the root and appear as related; got %v", related)
	assert.NotEmpty(t, related["confection"], "each related word carries a meaning")
}

// TestExampleData_RelatedFamilyRealRootsBook drives the user's real-shape roots
// book (latin-roots-book: definitions joined to an etymology notebook of the same
// id, every word carrying inline prefix+root origin_parts): missing one word
// surfaces its same-root siblings (declared under different prefixes / different
// units) as related, with meanings, excluding the drilled word.
func TestExampleData_RelatedFamilyRealRootsBook(t *testing.T) {
	ctx := context.Background()
	svc := newExampleService(t, t.TempDir())

	cards, err := svc.LoadCards([]string{"latin-roots-book"}, true, nil)
	require.NoError(t, err)
	missed := false
	for i := range cards {
		if cards[i].Entry == "recipient" {
			require.NoError(t, svc.SaveResult(ctx, cards[i], GradeResult{Correct: false, Quality: 0}, 1000))
			missed = true
		}
	}
	require.True(t, missed, "standard quiz must serve recipient")

	pool, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
	require.NoError(t, err)
	card := relearnCardFor(pool, "recipient")
	require.NotNil(t, card)
	require.Equal(t, "capere", card.OriginText)

	related := map[string]string{}
	for _, m := range card.RelatedWords {
		related[m.Word] = m.Meaning
	}
	assert.Containsf(t, related, "deceptive", "same-root sibling under a different prefix is listed; got %v", related)
	assert.NotEmpty(t, related["deceptive"])
	assert.NotContains(t, related, "recipient", "the drilled word is excluded from related")
}

// TestExampleData_RelatedFamilyUntypedRootLast pins the fix for origins declared
// WITHOUT a `type` field, where the etymology convention is prefix(es) first and
// the ROOT LAST (examples/etymology/roots-untyped). With untyped parts the old
// code folded a prefixed word under its FIRST part (the prefix) — abduct→"ab",
// subduct→"sub" — scattering them out of the ducere family so the origin card
// showed no relatives. After the fix every ducere word folds under the root, so
// a missed word's card lists its same-root siblings. Covered recognition AND
// reverse (a freeform miss records both series; drills reverse).
func TestExampleData_RelatedFamilyUntypedRootLast(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name      string
		direction notebook.QuizType
	}{
		{"recognition", notebook.QuizTypeNotebook},
		{"reverse", notebook.QuizTypeReverse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newExampleService(t, t.TempDir())

			switch tc.direction {
			case notebook.QuizTypeReverse:
				cards, err := svc.LoadReverseCards([]string{"roots-untyped"}, false, true, nil)
				require.NoError(t, err)
				missed := false
				for i := range cards {
					if cards[i].Expression == "abduct" {
						require.NoError(t, svc.SaveReverseResult(ctx, cards[i], GradeResult{Correct: false, Quality: 0}, 1000))
						missed = true
					}
				}
				require.True(t, missed, "reverse quiz must serve abduct")
			default:
				cards, err := svc.LoadCards([]string{"roots-untyped"}, true, nil)
				require.NoError(t, err)
				missed := false
				for i := range cards {
					if cards[i].Entry == "abduct" {
						require.NoError(t, svc.SaveResult(ctx, cards[i], GradeResult{Correct: false, Quality: 0}, 1000))
						missed = true
					}
				}
				require.True(t, missed, "standard quiz must serve abduct")
			}

			pool, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
			require.NoError(t, err)
			card := relearnCardFor(pool, "abduct")
			require.NotNil(t, card, "the missed word must be in the pool")
			require.Equalf(t, "ducere", card.OriginText,
				"a prefixed word with untyped parts must fold under the ROOT (last part), not its prefix")

			related := map[string]string{}
			for _, m := range card.RelatedWords {
				related[m.Word] = m.Meaning
			}
			assert.Containsf(t, related, "subduct", "same-root sibling (another prefix) must be listed; got %v", related)
			assert.Containsf(t, related, "ductile", "root-only sibling must be listed; got %v", related)
			assert.NotEmpty(t, related["subduct"])
			assert.NotContains(t, related, "abduct", "the drilled word is excluded from related")
		})
	}
}

// TestExampleData_RelatedFamilyIncludesQuizExcludedSiblings mirrors the user's
// real state: several same-root siblings were LEARNED and excluded from quizzes
// (skipped_at set for notebook + reverse via the real SkipWord path), while one
// sibling is still drilled. The related-words reference must INCLUDE those
// excluded siblings (they are known same-origin words worth showing) and exclude
// only the drilled word. Before this change the family builder filtered
// skipped_at words out, so the drilled card showed an empty family. Covers
// recognition and reverse; seeds the leftover skipped_at STATE through the real
// write path (verify-data-features-with-example-notebooks.md).
func TestExampleData_RelatedFamilyIncludesQuizExcludedSiblings(t *testing.T) {
	ctx := context.Background()

	excluded := []struct{ expr, id string }{
		{"factory", "factory-demo"},
		{"confection", "confection-demo"},
	}

	for _, tc := range []struct {
		name      string
		direction notebook.QuizType
	}{
		{"recognition", notebook.QuizTypeNotebook},
		{"reverse", notebook.QuizTypeReverse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			learningDir := t.TempDir()
			svc := newExampleService(t, learningDir)

			// Seed the leftover state: exclude the facere siblings from quizzes.
			for _, w := range excluded {
				require.NoError(t, svc.SkipWord(
					CardInfo{NotebookName: "roots-demo", Expression: w.expr, ID: w.id},
					"", []notebook.QuizType{notebook.QuizTypeNotebook, notebook.QuizTypeReverse}))
			}
			histories, err := notebook.NewLearningHistories(learningDir)
			require.NoError(t, err)
			for _, w := range excluded {
				require.Truef(t,
					notebook.IsExpressionExcludedForQuizType(histories["roots-demo"], w.id, notebook.QuizTypeNotebook, w.expr),
					"precondition: %q must be excluded from quizzes on disk", w.expr)
			}

			// Miss the un-excluded sibling in the given direction.
			switch tc.direction {
			case notebook.QuizTypeReverse:
				cards, err := svc.LoadReverseCards([]string{"roots-demo"}, false, true, nil)
				require.NoError(t, err)
				missed := false
				for i := range cards {
					if cards[i].Expression == "deficient" {
						require.NoError(t, svc.SaveReverseResult(ctx, cards[i], GradeResult{Correct: false, Quality: 0}, 1000))
						missed = true
					}
				}
				require.True(t, missed, "reverse quiz must serve deficient")
			default:
				cards, err := svc.LoadCards([]string{"roots-demo"}, true, nil)
				require.NoError(t, err)
				missed := false
				for i := range cards {
					if cards[i].Entry == "deficient" {
						require.NoError(t, svc.SaveResult(ctx, cards[i], GradeResult{Correct: false, Quality: 0}, 1000))
						missed = true
					}
				}
				require.True(t, missed, "standard quiz must serve deficient")
			}

			pool, err := svc.LoadRelearnPool(time.Now().Add(-24 * time.Hour))
			require.NoError(t, err)
			card := relearnCardFor(pool, "deficient")
			require.NotNil(t, card)
			require.Equal(t, "facere", card.OriginText)

			related := map[string]string{}
			for _, m := range card.RelatedWords {
				related[m.Word] = m.Meaning
			}
			// The quiz-excluded siblings are STILL shown as reference, with meanings.
			for _, w := range excluded {
				assert.Containsf(t, related, w.expr,
					"a quiz-excluded sibling must still appear as related reference; got %v", related)
				assert.NotEmptyf(t, related[w.expr], "%q carries its meaning", w.expr)
			}
			// Only the drilled word is excluded from the reference.
			assert.NotContains(t, related, "deficient", "the drilled word is excluded from related")
		})
	}
}
