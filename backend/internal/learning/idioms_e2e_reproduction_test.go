package learning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// TestIdiomsE2EFixtureProducesBothFlashcards reproduces the e2e fixture
// state for the "idioms" flashcard notebook and asserts the
// DBHistoryStore → FilterFlashcardNotebooks chain leaves BOTH cards in
// the standard-quiz result. The Frontend Standard quiz test fails on CI
// because the cards never render; the root cause is either
//   (a) buildExpressionFromDBOrder producing logs the YAML reader didn't,
//   (b) needsToLearn filtering a card out unexpectedly,
//   (c) a divergence between the YAML reader's LearningHistory shape and
//       the one DBHistoryStore returns.
//
// This test plays the e2e fixture (frontend/e2e/fixtures/learning_notes/idioms.yml)
// through buildExpressionFromDBOrder directly, then feeds the result into
// FilterFlashcardNotebooks just as quiz.loadFlashcardCards would. It
// fails when LoadCards' downstream would return 0 cards.
func TestIdiomsE2EFixtureProducesBothFlashcards(t *testing.T) {
	// LearningLog rows the importer would write from idioms.yml, with
	// note_id values mimicking what BatchCreate assigns under id ASC
	// (YAML array order).
	const (
		breakNoteID = int64(10)
		loseNoteID  = int64(11)
	)
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	breakLogs := []LearningLog{
		{ID: 1, NoteID: breakNoteID, Status: "understood", LearnedAt: t0, Quality: 4, QuizType: "freeform", IntervalDays: 1, SourceNotebookID: "idioms"},
		{ID: 2, NoteID: breakNoteID, Status: "misunderstood", LearnedAt: t1, Quality: 1, QuizType: "notebook", IntervalDays: 1, SourceNotebookID: "idioms"},
		{ID: 3, NoteID: breakNoteID, Status: "misunderstood", LearnedAt: t1, Quality: 1, QuizType: "reverse", IntervalDays: 1, SourceNotebookID: "idioms"},
	}
	loseLogs := []LearningLog{
		{ID: 4, NoteID: loseNoteID, Status: "understood", LearnedAt: t0, Quality: 4, QuizType: "freeform", IntervalDays: 1, SourceNotebookID: "idioms"},
	}

	breakExpr := newExpressionFromNote(
		notebook.NoteRecord{
			ID: breakNoteID, Usage: "break the ice", Entry: "break the ice",
			NotebookNotes: []notebook.NotebookNote{{NoteID: breakNoteID, NotebookType: "flashcard", NotebookID: "idioms", Group: "Common Idioms"}},
		},
		breakLogs,
		nil,
	)
	loseExpr := newExpressionFromNote(
		notebook.NoteRecord{
			ID: loseNoteID, Usage: "lose one's temper", Entry: "lose one's temper",
			NotebookNotes: []notebook.NotebookNote{{NoteID: loseNoteID, NotebookType: "flashcard", NotebookID: "idioms", Group: "Common Idioms"}},
		},
		loseLogs,
		nil,
	)

	histories := []notebook.LearningHistory{{
		Metadata: notebook.LearningHistoryMetadata{
			NotebookID: "idioms",
			Title:      "Common Idioms",
			Type:       "flashcard",
		},
		Expressions: []notebook.LearningHistoryExpression{breakExpr, loseExpr},
	}}

	flashcards := []notebook.FlashcardNotebook{{
		Title:       "Common Idioms",
		Description: "Common idioms used for e2e tests",
		Date:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Cards: []notebook.Note{
			{Expression: "break the ice", Meaning: "a way to start a conversation in a social setting"},
			{Expression: "lose one's temper", Meaning: "to become angry"},
		},
	}}

	// Standard quiz calls FilterFlashcardNotebooks with
	// sortDesc=false, includeNoCorrectAnswers=true (Include unstudied
	// is on in the failing scenario), quizType=QuizTypeNotebook.
	filtered, err := notebook.FilterFlashcardNotebooks(flashcards, histories, nil, false, true, notebook.QuizTypeNotebook)
	require.NoError(t, err)
	require.Len(t, filtered, 1, "exactly one flashcard notebook should survive the filter")
	cards := filtered[0].Cards
	require.Len(t, cards, 2, "both Idioms cards must survive the Standard-quiz filter — the e2e test asserts the first card 'break the ice' renders as the quiz heading")

	assert.Equal(t, "break the ice", cards[0].Expression)
	assert.Equal(t, "lose one's temper", cards[1].Expression)
}
