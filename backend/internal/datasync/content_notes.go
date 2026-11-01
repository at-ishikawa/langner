package datasync

import (
	"context"
	"fmt"
	"io"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// EnsureContentNotes creates the notes (and notebook_notes links) a stored
// notebook's words need to be quizzed and to bind learning logs. It is
// additive: existing notes are kept and nothing is deleted. Returns the number
// of notes created.
func EnsureContentNotes(ctx context.Context, db *sqlx.DB, nc notebook.NotebookContent) (int, error) {
	reader, err := notebook.NewContentReader(ctx, notebook.NewMemoryContent([]notebook.NotebookContent{nc}), nil)
	if err != nil {
		return 0, err
	}
	importer := NewImporter(notebook.NewDBNoteRepository(db), nil, notebook.NewYAMLNoteRepository(reader), nil, nil, nil, io.Discard)
	return importer.EnsureNotesForNotebook(ctx, nc.ID)
}

// EnsureAllContentNotes runs EnsureContentNotes for every stored notebook. The
// e2e reset uses it to restore the notes it truncates.
func EnsureAllContentNotes(ctx context.Context, db *sqlx.DB) error {
	repo := notebook.NewContentRepository(db)
	catalog, err := repo.Catalog(ctx)
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	var ids []string
	for _, e := range catalog {
		if !seen[e.NotebookID] {
			seen[e.NotebookID] = true
			ids = append(ids, e.NotebookID)
		}
	}
	contents, err := repo.Load(ctx, ids)
	if err != nil {
		return err
	}
	for _, nc := range contents {
		if _, err := EnsureContentNotes(ctx, db, nc); err != nil {
			return fmt.Errorf("create notes of notebook %q: %w", nc.ID, err)
		}
	}
	return nil
}
