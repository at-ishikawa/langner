package datasync

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mock_notebook "github.com/at-ishikawa/langner/internal/mocks/notebook"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// TestImportNotes_OversizedSenseID_HardError drives the real ImportNotes path
// over examples/oversized-id-demo (a word whose `id:` is 400 chars) and asserts
// import ABORTS with a clean, named OversizedSenseIDError before any DB write,
// instead of failing deep in an insert with `value too long for type character
// varying(380)` (22001). The mock noteRepo is never reached — the gate fires
// right after reading the source. Before the gate, ImportNotes proceeded and
// the DB would raise the raw 22001; after, it errors here.
func TestImportNotes_OversizedSenseID_HardError(t *testing.T) {
	ctrl := gomock.NewController(t)
	noteRepo := mock_notebook.NewMockNoteRepository(ctrl) // must NOT be called

	reader, err := notebook.NewReader(nil, nil, nil,
		[]string{filepath.Join(repoRootForDatasyncTest(t), "examples", "oversized-id-demo")}, nil, nil)
	require.NoError(t, err)
	noteSource := notebook.NewYAMLNoteRepository(reader)

	imp := NewImporter(noteRepo, nil, noteSource, nil, nil, nil, io.Discard)

	_, err = imp.ImportNotes(context.Background(), ImportOptions{})
	require.Error(t, err, "import must reject an id longer than the sense_id column")

	var overErr *notebook.OversizedSenseIDError
	require.ErrorAs(t, err, &overErr, "must be an OversizedSenseIDError, not a raw 22001")
	require.Len(t, overErr.Notes, 1)
	assert.Equal(t, "cartography", overErr.Notes[0].Word)
	assert.Greater(t, overErr.Notes[0].Length, notebook.MaxSenseIDLen)
	assert.Contains(t, err.Error(), "shorten the 'id:'")
}
