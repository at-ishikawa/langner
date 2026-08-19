package notebook

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoRootFromTest walks up until it finds the examples/ directory shipped at
// the repo root, so this test exercises the REAL example notebooks (not a
// hand-built fixture) through the REAL Reader.
func repoRootFromTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	for path := wd; path != "/" && path != "."; path = filepath.Dir(path) {
		if info, err := os.Stat(filepath.Join(path, "examples", "definitions")); err == nil && info.IsDir() {
			return path
		}
	}
	t.Fatalf("examples/definitions not found above %s", wd)
	return ""
}

// TestSharedSenseCollapsesToOneNote is the non-DB regression guard for the
// `notes_sense_id_key` (23505) import crash. The shipped example books
// shared-sense-a and shared-sense-b both declare "hypersensitive" with the SAME
// id (hypersensitive-shared) but a DIFFERENT surface entry (shared-sense-b's
// `definition:` normalizes it to "hyper-sensitive"). The DB is unique on
// sense_id alone, so these MUST resolve to ONE note; the old
// (sense_id, usage, entry) dedup emitted TWO records, which then collided in
// BatchCreate. CanonicalNoteKey now folds them by sense_id, so the reader emits
// exactly one note carrying a notebook_note for BOTH books (L1/L2).
//
// This runs without a database: it pins the reader half of the fix. The full
// end-to-end (import into Postgres, no 23505) is the DB-gated
// TestImportDB_SharedSenseAcrossNotebooks_LivePostgres_Integration.
func TestSharedSenseCollapsesToOneNote(t *testing.T) {
	root := repoRootFromTest(t)
	reader, err := NewReader(
		[]string{filepath.Join(root, "examples", "stories")},
		[]string{filepath.Join(root, "examples", "flashcards")},
		[]string{filepath.Join(root, "examples", "books")},
		[]string{filepath.Join(root, "examples", "definitions")},
		[]string{filepath.Join(root, "examples", "etymology")},
		nil,
	)
	require.NoError(t, err)

	notes, err := NewYAMLNoteRepository(reader).FindAll(context.Background())
	require.NoError(t, err)

	var matches []NoteRecord
	for _, n := range notes {
		if n.SenseID == "hypersensitive-shared" {
			matches = append(matches, n)
		}
	}
	require.Len(t, matches, 1,
		"a word claimed by two notebooks under one sense_id must collapse to ONE note — "+
			"emitting two here is what tripped notes_sense_id_key (23505) at import time")

	notebooks := map[string]bool{}
	for _, nn := range matches[0].NotebookNotes {
		notebooks[nn.NotebookID] = true
	}
	assert.True(t, notebooks["shared-sense-a"], "the collapsed note must keep its shared-sense-a membership")
	assert.True(t, notebooks["shared-sense-b"], "the collapsed note must keep its shared-sense-b membership")
	assert.Equal(t, "hypersensitive", matches[0].Usage, "canonical usage is the shared expression")
}
