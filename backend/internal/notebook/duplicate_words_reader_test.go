package notebook

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFindAll_IdlessConflicts_DetectedNotIdentical loads the intentionally
// broken examples/conflicts-demo books through the REAL Reader and asserts the
// id-less duplicate policy:
//
//   - "bank" and "spring": id-less, declared in two books with DIFFERENT
//     meanings → reported as conflicts (the homographs to disambiguate).
//   - "delta": id-less, SAME meaning in both books → NOT reported (it dedups).
//
// FindAll itself does not error (read-only callers keep working); it records
// the conflicts for the importer to gate on.
func TestFindAll_IdlessConflicts_DetectedNotIdentical(t *testing.T) {
	root := repoRootFromTest(t)
	reader, err := NewReader(nil, nil, nil,
		[]string{filepath.Join(root, "examples", "conflicts-demo")}, nil, nil)
	require.NoError(t, err)

	repo := NewYAMLNoteRepository(reader)
	_, err = repo.FindAll(context.Background())
	require.NoError(t, err, "FindAll must NOT error on conflicts — it records them for the importer")

	byWord := map[string]DuplicateWord{}
	for _, w := range repo.DuplicateWordConflicts() {
		byWord[w.Word] = w
	}
	require.Contains(t, byWord, "bank", "id-less 'bank' with two meanings must be flagged")
	require.Contains(t, byWord, "spring", "id-less 'spring' with two meanings must be flagged")
	assert.NotContains(t, byWord, "delta", "id-less 'delta' with one meaning must NOT be flagged (it dedups)")

	// Each conflict names both notebooks and their differing meanings.
	bank := byWord["bank"]
	locs := map[string]string{}
	for _, o := range bank.Occurrences {
		locs[o.NotebookID] = o.Meaning
	}
	require.Len(t, locs, 2)
	assert.Contains(t, locs["finance-book"], "money")
	assert.Contains(t, locs["geography-book"], "river")

	// The aggregated error message is actionable and lists all conflicts.
	msg := (&DuplicateWordsError{Words: repo.DuplicateWordConflicts()}).Error()
	assert.Contains(t, msg, "Add a distinct 'id:'")
	assert.Contains(t, msg, `"bank"`)
	assert.Contains(t, msg, `"spring"`)
	assert.NotContains(t, msg, `"delta"`)
}
