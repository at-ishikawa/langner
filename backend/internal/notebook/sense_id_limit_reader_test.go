package notebook

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFindAll_OversizedSenseID_Detected loads the intentionally broken
// examples/oversized-id-demo book (a word whose `id:` is 400 chars, over the
// notes.sense_id VARCHAR(380) limit) through the real Reader and asserts it is
// recorded as oversized. FindAll does not error (read-only callers keep
// working); it records the offender for the importer to gate on, so a raw
// 22001 never reaches the user.
func TestFindAll_OversizedSenseID_Detected(t *testing.T) {
	root := repoRootFromTest(t)
	reader, err := NewReader(nil, nil, nil,
		[]string{filepath.Join(root, "examples", "oversized-id-demo")}, nil, nil)
	require.NoError(t, err)

	repo := NewYAMLNoteRepository(reader)
	_, err = repo.FindAll(context.Background())
	require.NoError(t, err, "FindAll must NOT error on an oversized id — it records it for the importer")

	big := repo.OversizedSenseIDs()
	require.Len(t, big, 1)
	assert.Equal(t, "cartography", big[0].Word)
	assert.Equal(t, "oversized-id-book", big[0].NotebookID)
	assert.Greater(t, big[0].Length, MaxSenseIDLen)

	msg := (&OversizedSenseIDError{Notes: big}).Error()
	assert.Contains(t, msg, "shorten the 'id:'")
	assert.Contains(t, msg, `"cartography"`)
	assert.Contains(t, msg, "oversized-id-book")
}

// TestFindAll_SenseIDWithinLimit_NotFlagged confirms the 200-char id in the
// shipped long-id-demo book (over the OLD 128, under the NEW 380) is NOT
// flagged — only ids over MaxSenseIDLen are.
func TestFindAll_SenseIDWithinLimit_NotFlagged(t *testing.T) {
	root := repoRootFromTest(t)
	reader, err := NewReader(nil, nil, nil,
		[]string{filepath.Join(root, "examples", "definitions")}, nil, nil)
	require.NoError(t, err)

	repo := NewYAMLNoteRepository(reader)
	notes, err := repo.FindAll(context.Background())
	require.NoError(t, err)
	assert.Empty(t, repo.OversizedSenseIDs(), "a 200-char id fits VARCHAR(380) and must not be flagged")

	var longevity *NoteRecord
	for i := range notes {
		if notes[i].Usage == "longevity" {
			longevity = &notes[i]
		}
	}
	require.NotNil(t, longevity, "the long-id-demo note must load")
	assert.Greater(t, len(longevity.SenseID), 128, "its id is over the OLD 128 limit (the reason for migration 023)")
	assert.LessOrEqual(t, len(longevity.SenseID), MaxSenseIDLen)
}
