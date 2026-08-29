package server

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	mock_notebook "github.com/at-ishikawa/langner/internal/mocks/notebook"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// TestQuizHandler_LearnPageSkip_NotShadowedByStaleGrammarCard reproduces the
// e2e regression: the shared-DB e2e run executes a grammar quiz before the
// Learn-page word-card skip scenario, so the ephemeral grammar store holds a
// blank whose id used to start at 1 — colliding with a Learn-page word's DB
// note_id (also small, from a fresh import). resolveCardInfo consults the
// ephemeral stores before the DB, so the deliberate Exclude resolved to the
// stale grammar correction ("…-the-john") instead of the word the user
// clicked, and the DB skip resolver then errored ("no matching note or
// origin"), reverting the checkbox.
//
// The fix offsets every ephemeral session card id above any DB note id
// (sessionIDBase), so the two id spaces are disjoint. This test drives a REAL
// grammar quiz to populate the store, asserts the offset invariant, then
// asserts a small Learn-page DB note_id resolves through the DB (note_id set,
// correct expression/notebook) rather than the stale grammar card.
func TestQuizHandler_LearnPageSkip_NotShadowedByStaleGrammarCard(t *testing.T) {
	ctx := context.Background()
	handler, _ := newGrammarHandler(t)

	// A grammar quiz runs first, populating the ephemeral grammar store — the
	// exact ordering the shared-DB e2e harness produces.
	start, err := handler.StartGrammarQuiz(ctx, connect.NewRequest(&apiv1.StartGrammarQuizRequest{
		NotebookIds: []string{"journal"},
	}))
	require.NoError(t, err)
	require.Len(t, start.Msg.GetPosts(), 1)
	require.Len(t, start.Msg.GetPosts()[0].GetBlanks(), 1)
	blank := start.Msg.GetPosts()[0].GetBlanks()[0]

	// Regression guard: session card ids live in the offset range, disjoint from
	// DB note ids, so a small Learn-page note_id can never collide with a stale
	// session card. Without the offset this id would be 1 and shadow note 1.
	require.GreaterOrEqual(t, blank.GetNoteId(), sessionIDBase,
		"ephemeral session card ids must be offset above DB note ids")

	// A Learn-page word card carries a real (small) DB note id. Excluding it must
	// resolve THAT note through the DB, not the stale grammar card left above.
	const dbNoteID int64 = 1
	ctrl := gomock.NewController(t)
	mockRepo := mock_notebook.NewMockNoteRepository(ctrl)
	handler.SetNoteRepository(mockRepo)
	mockRepo.EXPECT().FindByID(gomock.Any(), int64(dbNoteID)).Return(&notebook.NoteRecord{
		ID:            dbNoteID,
		Usage:         "break the ice",
		Entry:         "break the ice",
		NotebookNotes: []notebook.NotebookNote{{NotebookID: "idioms", Group: "Common Idioms"}},
	}, nil)

	info, err := handler.resolveCardInfo(ctx, dbNoteID)
	require.NoError(t, err)
	assert.Equal(t, int64(dbNoteID), info.NoteID, "Learn-page skip carries the DB note id for a homograph-safe write")
	assert.Equal(t, "break the ice", info.Expression, "must resolve the clicked word, not the stale grammar correction")
	assert.Equal(t, "idioms", info.NotebookName)
	assert.NotEqual(t, "note-the-john", info.Expression, "must not resolve to the grammar correction sense id")
}
