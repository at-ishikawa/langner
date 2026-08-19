package datasync

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mock_notebook "github.com/at-ishikawa/langner/internal/mocks/notebook"
	"github.com/at-ishikawa/langner/internal/notebook"
)

func repoRootForDatasyncTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	for path := wd; path != "/" && path != "."; path = filepath.Dir(path) {
		if info, err := os.Stat(filepath.Join(path, "examples", "conflicts-demo")); err == nil && info.IsDir() {
			return path
		}
	}
	t.Fatalf("examples/conflicts-demo not found above %s", wd)
	return ""
}

// TestImportNotes_IdlessConflicts_HardError drives the REAL importer path
// (ImportNotes with a real YAMLNoteRepository over the intentionally broken
// examples/conflicts-demo books) and asserts import ABORTS with a clean,
// aggregated DuplicateWordsError instead of silently collapsing a homograph or
// crashing deep in an insert. The mock noteRepo is never reached — the gate
// fires right after reading the source. This is the reproduction: on the code
// before the gate, ImportNotes proceeded (no error); after, it errors.
func TestImportNotes_IdlessConflicts_HardError(t *testing.T) {
	ctrl := gomock.NewController(t)
	// noteRepo must NOT be called: the conflict gate returns before load.
	noteRepo := mock_notebook.NewMockNoteRepository(ctrl)

	reader, err := notebook.NewReader(nil, nil, nil,
		[]string{filepath.Join(repoRootForDatasyncTest(t), "examples", "conflicts-demo")}, nil, nil)
	require.NoError(t, err)
	noteSource := notebook.NewYAMLNoteRepository(reader)

	imp := NewImporter(noteRepo, nil, noteSource, nil, nil, nil, io.Discard)

	_, err = imp.ImportNotes(context.Background(), ImportOptions{})
	require.Error(t, err, "import must reject undisambiguated id-less homographs")

	var dupErr *notebook.DuplicateWordsError
	require.ErrorAs(t, err, &dupErr, "must be a DuplicateWordsError, not a raw scan/constraint error")

	words := map[string]bool{}
	for _, w := range dupErr.Words {
		words[w.Word] = true
	}
	assert.True(t, words["bank"], "conflicting id-less 'bank' must be reported")
	assert.True(t, words["spring"], "conflicting id-less 'spring' must be reported")
	assert.False(t, words["delta"], "identical-meaning id-less 'delta' must NOT be reported (it dedups)")
	assert.Len(t, dupErr.Words, 2, "ALL conflicts reported together, not one-at-a-time")

	msg := err.Error()
	assert.Contains(t, msg, "Add a distinct 'id:'")
	assert.Contains(t, msg, "finance-book")
	assert.Contains(t, msg, "geography-book")
}
