// Package datasync provides import/export orchestration between YAML files and database.
package datasync

import (
	"context"
	"fmt"
	"io"

	"github.com/at-ishikawa/langner/internal/notebook"
)

type noteKey struct{ usage, entry string }

type nnKey struct {
	noteID                                 int64
	notebookType, notebookID, group string
}

// ImportResult tracks counts for each import operation.
type ImportResult struct {
	NotesNew        int
	NotesSkipped    int
	NotesUpdated    int
	NotebookNew     int
	NotebookSkipped int
}

// ImportOptions controls import behavior.
type ImportOptions struct {
	DryRun         bool
	UpdateExisting bool
}

// Importer reads YAML notebook data and writes to DB.
type Importer struct {
	noteRepo notebook.NoteRepository
	writer   io.Writer
}

// NewImporter creates a new Importer.
func NewImporter(noteRepo notebook.NoteRepository, writer io.Writer) *Importer {
	return &Importer{
		noteRepo: noteRepo,
		writer:   writer,
	}
}

// ImportNotes imports notes and notebook_note links from story and flashcard indexes.
func (imp *Importer) ImportNotes(ctx context.Context, storyIndexes map[string]notebook.Index, flashcardIndexes map[string]notebook.FlashcardIndex, opts ImportOptions) (*ImportResult, error) {
	var result ImportResult

	allNotes, err := imp.noteRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load existing notes: %w", err)
	}

	noteCache := make(map[noteKey]*notebook.NoteRecord, len(allNotes))
	for i := range allNotes {
		noteCache[noteKey{allNotes[i].Usage, allNotes[i].Entry}] = &allNotes[i]
	}

	nnCache := make(map[nnKey]bool)
	for _, n := range allNotes {
		for _, nn := range n.NotebookNotes {
			nnCache[nnKey{nn.NoteID, nn.NotebookType, nn.NotebookID, nn.Group}] = true
		}
	}

	// Import story/book notebooks
	for indexID, index := range storyIndexes {
		notebookType := "story"
		if index.IsBook() {
			notebookType = "book"
		}

		for _, storyNotebooks := range index.Notebooks {
			for _, sn := range storyNotebooks {
				for _, scene := range sn.Scenes {
					for _, def := range scene.Definitions {
						if err := imp.importNote(ctx, def, indexID, notebookType, sn.Event, scene.Title, opts, &result, noteCache, nnCache); err != nil {
							return nil, fmt.Errorf("import story note: %w", err)
						}
					}
				}
			}
		}
	}

	// Import flashcard notebooks
	for flashcardID, flashcardIndex := range flashcardIndexes {
		for _, fn := range flashcardIndex.Notebooks {
			for _, card := range fn.Cards {
				if err := imp.importNote(ctx, card, flashcardID, "flashcard", fn.Title, "", opts, &result, noteCache, nnCache); err != nil {
					return nil, fmt.Errorf("import flashcard note: %w", err)
				}
			}
		}
	}

	return &result, nil
}

func (imp *Importer) importNote(ctx context.Context, def notebook.Note, notebookID, notebookType, group, subgroup string, opts ImportOptions, result *ImportResult, noteCache map[noteKey]*notebook.NoteRecord, nnCache map[nnKey]bool) error {
	usage := def.Expression
	entry := def.Definition
	if entry == "" {
		entry = def.Expression
	}

	existing := noteCache[noteKey{usage, entry}]

	// Create new note if it does not exist
	if existing == nil {
		images := make([]notebook.NoteImage, len(def.Images))
		for i, img := range def.Images {
			images[i] = notebook.NoteImage{URL: img, SortOrder: i}
		}
		references := make([]notebook.NoteReference, len(def.References))
		for i, ref := range def.References {
			references[i] = notebook.NoteReference{Link: ref.URL, Description: ref.Description, SortOrder: i}
		}

		n := &notebook.NoteRecord{
			Usage:            usage,
			Entry:            entry,
			Meaning:          def.Meaning,
			Level:            string(def.Level),
			DictionaryNumber: def.DictionaryNumber,
			Images:           images,
			References:       references,
			NotebookNotes: []notebook.NotebookNote{
				{NotebookType: notebookType, NotebookID: notebookID, Group: group, Subgroup: subgroup},
			},
		}
		if !opts.DryRun {
			if err := imp.noteRepo.Create(ctx, n); err != nil {
				return fmt.Errorf("create note: %w", err)
			}
		}
		noteCache[noteKey{usage, entry}] = n
		_, _ = fmt.Fprintf(imp.writer, "  [NEW]  %q (%s)\n", usage, entry)
		result.NotesNew++
		result.NotebookNew++
		return nil
	}

	// Handle existing note: skip or update
	if !opts.UpdateExisting {
		_, _ = fmt.Fprintf(imp.writer, "  [SKIP]  %q (%s)\n", usage, entry)
		result.NotesSkipped++
	} else {
		existing.Meaning = def.Meaning
		existing.Level = string(def.Level)
		existing.DictionaryNumber = def.DictionaryNumber
		if !opts.DryRun {
			if err := imp.noteRepo.Update(ctx, existing); err != nil {
				return fmt.Errorf("update note: %w", err)
			}
		}
		_, _ = fmt.Fprintf(imp.writer, "  [UPDATE]  %q (%s)\n", usage, entry)
		result.NotesUpdated++
	}

	// Create notebook_note link for existing notes
	if nnCache[nnKey{existing.ID, notebookType, notebookID, group}] {
		result.NotebookSkipped++
		return nil
	}

	if !opts.DryRun {
		nn := &notebook.NotebookNote{
			NoteID:       existing.ID,
			NotebookType: notebookType,
			NotebookID:   notebookID,
			Group:        group,
			Subgroup:     subgroup,
		}
		if err := imp.noteRepo.CreateNotebookNote(ctx, nn); err != nil {
			return fmt.Errorf("create notebook note link: %w", err)
		}
	}
	nnCache[nnKey{existing.ID, notebookType, notebookID, group}] = true
	result.NotebookNew++
	return nil
}
