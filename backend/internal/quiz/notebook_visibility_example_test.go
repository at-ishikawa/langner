package quiz

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// fakeVisibility hides one notebook from everyone except ownerID. It stands in
// for the DB-backed NotebookACLRepository so the ENFORCEMENT half of auth
// Phase 3 (does every read path actually filter?) is exercised in CI without a
// database — the notebooks overlay table is Postgres-only and is covered by the
// *_LivePostgres_Integration tests. Only the visibility SOURCE is faked here;
// the notebooks are the real shipped example books loaded through the real
// Service/Reader, so this drives the true read paths, not hand-built cards.
type fakeVisibility struct {
	privateNotebook string
	ownerID         int64
}

func (f fakeVisibility) VisibleNotebookIDs(_ context.Context, userID int64) (notebook.VisibilityPredicate, error) {
	return func(notebookID string) bool {
		if notebookID == f.privateNotebook {
			return userID == f.ownerID
		}
		return true
	}, nil
}

// TestNotebookVisibility_ExampleService_FiltersEveryReadPath loads the shipped
// example books through the real Service with a fake ACL that makes
// visibility-private-demo private-to-owner, then checks EVERY read path the
// audit covers. One unfiltered path would leak the private book here.
func TestNotebookVisibility_ExampleService_FiltersEveryReadPath(t *testing.T) {
	const (
		ownerID    = int64(1)
		nonOwnerID = int64(2)
		public     = "visibility-public-demo"
		private    = "visibility-private-demo"
	)

	svc := newExampleService(t, t.TempDir())
	svc.SetNotebookACL(fakeVisibility{privateNotebook: private, ownerID: ownerID})

	summaryHas := func(userID int64, nbID string) bool {
		summaries, err := svc.LoadNotebookSummaries(userID, true)
		require.NoError(t, err)
		for _, s := range summaries {
			if s.NotebookID == nbID {
				return true
			}
		}
		return false
	}

	// Quiz options: both see public; only owner sees private.
	assert.True(t, summaryHas(ownerID, public), "owner sees the public book")
	assert.True(t, summaryHas(ownerID, private), "owner sees their private book")
	assert.True(t, summaryHas(nonOwnerID, public), "non-owner sees the public book")
	assert.False(t, summaryHas(nonOwnerID, private), "non-owner must NOT see the private book in options")

	// LoadCards: owner loads private cards; non-owner gets NotFound.
	ownerCards, err := svc.LoadCards(ownerID, []string{private}, true, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, ownerCards, "owner can load their private book's cards")

	_, err = svc.LoadCards(nonOwnerID, []string{private}, true, nil)
	require.Error(t, err)
	var notFound *NotFoundError
	assert.True(t, errors.As(err, &notFound), "non-owner loading the private book is NotFound, got %T", err)

	// LoadReverseCards: same gate.
	_, err = svc.LoadReverseCards(nonOwnerID, []string{private}, false, true, nil)
	require.Error(t, err)
	assert.True(t, errors.As(err, &notFound), "non-owner reverse-loading the private book is NotFound, got %T", err)

	// Freeform pool: a private book's words never enter a non-owner's pool.
	freeform := func(userID int64) map[string]bool {
		cards, err := svc.LoadAllWords(userID)
		require.NoError(t, err)
		out := map[string]bool{}
		for _, c := range cards {
			out[c.Expression] = true
		}
		return out
	}
	ownerPool := freeform(ownerID)
	assert.True(t, ownerPool["portage"], "owner freeform pool includes their private words")
	assert.True(t, ownerPool["spectate"], "owner freeform pool includes public words")

	nonOwnerPool := freeform(nonOwnerID)
	assert.True(t, nonOwnerPool["spectate"], "non-owner freeform pool includes public words")
	assert.False(t, nonOwnerPool["portage"], "private words must never enter a non-owner's freeform pool")
	assert.False(t, nonOwnerPool["comport"], "private words must never enter a non-owner's freeform pool")
}
