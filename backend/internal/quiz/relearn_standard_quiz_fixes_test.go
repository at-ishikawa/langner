package quiz

import (
	"context"
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
