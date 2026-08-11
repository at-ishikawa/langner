package quiz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// findFreeformCard returns the LoadAllWords card whose canonical expression
// matches expr, or nil.
func findFreeformCard(cards []FreeformCard, expr string) *FreeformCard {
	for i := range cards {
		if cards[i].Expression == expr {
			return &cards[i]
		}
	}
	return nil
}

// TestExampleData_EtymologyNotebookWordHasStableNoteID drives the REAL example
// config end to end (the exact Service/Reader construction the server uses) to
// pin the override-button contract for a word that lives ONLY inside an
// etymology notebook (examples/etymology/latin-verbs: "artifice", with no copy
// in any definitions/story/flashcard book).
//
// The bug: such words were built into quiz cards with ID="" (no stable sense
// id), because EtymologyDefinitionEntry had no id and appendEtymologyNotebookWords
// never assigned one. The Submit response then carried an empty sense_id, so the
// feedback's Mark-as-Correct / Undo override had no stable key to target the
// exact record with — the learning-history-invariants L2 hazard. (When a word
// is surfaced into a quiz whose feedback gates the override on a populated note
// identity, an empty id also hides the button.)
//
// The fix gives every etymology-notebook definition a stable NoteID(), used at
// card-build time so the SAME key flows into the write (SaveFreeformResult) and
// the read (GetLatestLearnedInfo) — write-key == read-key — and the override
// can target it. The assertions below FAIL before the fix (ID=="") and pass
// after.
func TestExampleData_EtymologyNotebookWordHasStableNoteID(t *testing.T) {
	ctx := context.Background()

	svc := newExampleService(t, t.TempDir())
	all, err := svc.LoadAllWords()
	require.NoError(t, err)

	card := findFreeformCard(all, "artifice")
	require.NotNil(t, card, "artifice (etymology-notebook-only word) must be served as an ordinary quiz card")

	// Core of the bug: the card must carry a non-empty stable sense id. This is
	// the exact field the Submit response echoes as sense_id and the override RPC
	// targets. Empty here == empty sense_id == the override cannot identify the
	// record.
	require.NotEmpty(t, card.ID, "etymology-notebook word must carry a stable note id (empty == empty sense_id)")

	// The id must be STABLE across an independent Service construction (a later
	// session), or a cross-session Mark-as-Correct would target a different key.
	svc2 := newExampleService(t, t.TempDir())
	all2, err := svc2.LoadAllWords()
	require.NoError(t, err)
	card2 := findFreeformCard(all2, "artifice")
	require.NotNil(t, card2)
	assert.Equal(t, card.ID, card2.ID, "note id must be stable across reloads")

	// L1/L4: a word that ALSO exists in a definitions book keeps that book's
	// canonical id — the etymology copy must not fork a second id/series.
	factory := findFreeformCard(all, "factory")
	require.NotNil(t, factory)
	assert.Equal(t, "factory-demo", factory.ID,
		"factory keeps its definitions-book id; the etymology copy must not fork a second series")

	// End-to-end symmetry (L2): a wrong freeform answer writes under card.ID,
	// GetLatestLearnedInfo reads it back under the same key (non-empty
	// learned_at), the on-disk entry carries that id, and an override targeting
	// that id persists.
	learningDir := t.TempDir()
	svc3 := newExampleService(t, learningDir)
	all3, err := svc3.LoadAllWords()
	require.NoError(t, err)
	c := findFreeformCard(all3, "artifice")
	require.NotNil(t, c)
	require.NotEmpty(t, c.ID)

	require.NoError(t, svc3.SaveFreeformResult(ctx, *c,
		FreeformGradeResult{Correct: false, Quality: 0, MatchedCard: c}, 1000))

	learnedAt, _ := svc3.GetLatestLearnedInfo(c.NotebookName, c.ID, c.Expression, notebook.QuizTypeFreeform)
	require.NotEmpty(t, learnedAt,
		"GetLatestLearnedInfo must find the just-written log under the same id (write-key == read-key)")

	// The on-disk learning entry carries the stable id, so a later lookup by id
	// (not just by name) resolves it — homograph-safe.
	histories, err := notebook.NewLearningHistories(learningDir)
	require.NoError(t, err)
	byID := notebook.FindExpressionInHistories(histories[c.NotebookName], c.ID)
	require.NotNil(t, byID, "the written learning entry must be resolvable by the stable id")
	assert.Equal(t, c.ID, byID.ID)

	before := byID.GetLogsForQuizType(notebook.QuizTypeFreeform)
	require.NotEmpty(t, before)
	assert.Equal(t, notebook.LearnedStatusMisunderstood, before[0].Status)

	// Mark-as-Correct override, targeting the stable id exactly as the handler
	// does (info.ID = response sense_id).
	info := CardInfoFromFreeformCard(*c)
	info.ID = c.ID
	info.LearnedAt = learnedAt
	mc := true
	info.MarkCorrect = &mc
	_, err = svc3.OverrideAnswer(info, notebook.QuizTypeFreeform)
	require.NoError(t, err)

	after, err := notebook.NewLearningHistories(learningDir)
	require.NoError(t, err)
	overridden := notebook.FindExpressionInHistories(after[c.NotebookName], c.ID)
	require.NotNil(t, overridden)
	logs := overridden.GetLogsForQuizType(notebook.QuizTypeFreeform)
	require.NotEmpty(t, logs)
	assert.NotEqual(t, notebook.LearnedStatusMisunderstood, logs[0].Status,
		"Mark-as-Correct override must persist against the stable id")
}
