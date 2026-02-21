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

	// Build caches
	noteCache := make(map[noteKey]*notebook.NoteRecord, len(allNotes))
	nnCache := make(map[nnKey]bool)
	for i := range allNotes {
		noteCache[noteKey{allNotes[i].Usage, allNotes[i].Entry}] = &allNotes[i]
		for _, nn := range allNotes[i].NotebookNotes {
			nnCache[nnKey{nn.NoteID, nn.NotebookType, nn.NotebookID, nn.Group}] = true
		}
		// Clear so we only track NEW notebook_notes during classification
		allNotes[i].NotebookNotes = nil
	}

	var newNotes []*notebook.NoteRecord
	updateMap := make(map[noteKey]*notebook.NoteRecord)

	// Walk story/book notebooks
	for indexID, index := range storyIndexes {
		notebookType := "story"
		if index.IsBook() {
			notebookType = "book"
		}
		for _, storyNotebooks := range index.Notebooks {
			for _, sn := range storyNotebooks {
				for _, scene := range sn.Scenes {
					for _, def := range scene.Definitions {
						imp.classifyNote(def, indexID, notebookType, sn.Event, scene.Title, opts, &result, noteCache, nnCache, &newNotes, updateMap)
					}
				}
			}
		}
	}

	// Walk flashcard notebooks
	for flashcardID, flashcardIndex := range flashcardIndexes {
		for _, fn := range flashcardIndex.Notebooks {
			for _, card := range fn.Cards {
				imp.classifyNote(card, flashcardID, "flashcard", fn.Title, "", opts, &result, noteCache, nnCache, &newNotes, updateMap)
			}
		}
	}

	// Flush batches
	if !opts.DryRun && len(newNotes) > 0 {
		if err := imp.noteRepo.BatchCreate(ctx, newNotes); err != nil {
			return nil, fmt.Errorf("batch create notes: %w", err)
		}
	}
	if !opts.DryRun && len(updateMap) > 0 {
		updates := make([]*notebook.NoteRecord, 0, len(updateMap))
		for _, n := range updateMap {
			updates = append(updates, n)
		}
		if err := imp.noteRepo.BatchUpdate(ctx, updates); err != nil {
			return nil, fmt.Errorf("batch update notes: %w", err)
		}
	}

	return &result, nil
}

func (imp *Importer) classifyNote(def notebook.Note, notebookID, notebookType, group, subgroup string, opts ImportOptions, result *ImportResult, noteCache map[noteKey]*notebook.NoteRecord, nnCache map[nnKey]bool, newNotes *[]*notebook.NoteRecord, updateMap map[noteKey]*notebook.NoteRecord) {
	usage := def.Expression
	entry := def.Definition
	if entry == "" {
		entry = def.Expression
	}

	key := noteKey{usage, entry}
	existing := noteCache[key]

	if existing == nil {
		// Brand new note
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
		*newNotes = append(*newNotes, n)
		noteCache[key] = n
		_, _ = fmt.Fprintf(imp.writer, "  [NEW]  %q (%s)\n", usage, entry)
		result.NotesNew++
		result.NotebookNew++
		return
	}

	// Note exists in cache -- could be a pending new note (ID==0) or existing DB note
	if existing.ID == 0 {
		// Already pending creation -- just add another notebook_note
		existing.NotebookNotes = append(existing.NotebookNotes, notebook.NotebookNote{
			NotebookType: notebookType, NotebookID: notebookID, Group: group, Subgroup: subgroup,
		})
		result.NotebookNew++
		return
	}

	// Existing DB note: handle skip/update
	if opts.UpdateExisting {
		existing.Meaning = def.Meaning
		existing.Level = string(def.Level)
		existing.DictionaryNumber = def.DictionaryNumber
		updateMap[key] = existing
		_, _ = fmt.Fprintf(imp.writer, "  [UPDATE]  %q (%s)\n", usage, entry)
		result.NotesUpdated++
	} else {
		_, _ = fmt.Fprintf(imp.writer, "  [SKIP]  %q (%s)\n", usage, entry)
		result.NotesSkipped++
	}

	// Check notebook_note link
	nnk := nnKey{existing.ID, notebookType, notebookID, group}
	if nnCache[nnk] {
		result.NotebookSkipped++
		return
	}

	existing.NotebookNotes = append(existing.NotebookNotes, notebook.NotebookNote{
		NoteID: existing.ID, NotebookType: notebookType, NotebookID: notebookID, Group: group, Subgroup: subgroup,
	})
	nnCache[nnk] = true
	updateMap[key] = existing
	result.NotebookNew++
}
