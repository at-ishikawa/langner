// Package datasync provides import/export orchestration between YAML files and database.
package datasync

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/at-ishikawa/langner/internal/notebook"
)

type noteKey struct{ usage, entry string }

type nnKey struct {
	noteID                                          int64
	notebookType, notebookID, group, subgroup string
}

// classifyState holds mutable state passed through the classification loop.
type classifyState struct {
	result      *ImportResult
	noteCache   map[noteKey]*notebook.NoteRecord
	nnCache     map[nnKey]bool
	newNotes    []*notebook.NoteRecord
	updateNotes map[noteKey]*notebook.NoteRecord
	newNNs      []notebook.NotebookNote
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
	allNotes, err := imp.noteRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load existing notes: %w", err)
	}

	state := &classifyState{
		result:      &ImportResult{},
		noteCache:   make(map[noteKey]*notebook.NoteRecord, len(allNotes)),
		nnCache:     make(map[nnKey]bool),
		updateNotes: make(map[noteKey]*notebook.NoteRecord),
	}

	// Build caches
	for i := range allNotes {
		state.noteCache[noteKey{allNotes[i].Usage, allNotes[i].Entry}] = &allNotes[i]
		for _, nn := range allNotes[i].NotebookNotes {
			state.nnCache[nnKey{nn.NoteID, nn.NotebookType, nn.NotebookID, nn.Group, nn.Subgroup}] = true
		}
		// Clear so we only track NEW notebook_notes during classification
		allNotes[i].NotebookNotes = nil
	}

	// Sort story index keys for deterministic ordering
	storyKeys := make([]string, 0, len(storyIndexes))
	for k := range storyIndexes {
		storyKeys = append(storyKeys, k)
	}
	sort.Strings(storyKeys)

	// Walk story/book notebooks
	for _, indexID := range storyKeys {
		index := storyIndexes[indexID]
		notebookType := "story"
		if index.IsBook() {
			notebookType = "book"
		}
		for _, storyNotebooks := range index.Notebooks {
			for _, sn := range storyNotebooks {
				for _, scene := range sn.Scenes {
					for _, def := range scene.Definitions {
						imp.classifyNote(def, indexID, notebookType, sn.Event, scene.Title, opts, state)
					}
				}
			}
		}
	}

	// Sort flashcard index keys for deterministic ordering
	flashcardKeys := make([]string, 0, len(flashcardIndexes))
	for k := range flashcardIndexes {
		flashcardKeys = append(flashcardKeys, k)
	}
	sort.Strings(flashcardKeys)

	// Walk flashcard notebooks
	for _, flashcardID := range flashcardKeys {
		flashcardIndex := flashcardIndexes[flashcardID]
		for _, fn := range flashcardIndex.Notebooks {
			for _, card := range fn.Cards {
				imp.classifyNote(card, flashcardID, "flashcard", fn.Title, "", opts, state)
			}
		}
	}

	// Flush batches
	if !opts.DryRun && len(state.newNotes) > 0 {
		if err := imp.noteRepo.BatchCreate(ctx, state.newNotes); err != nil {
			return nil, fmt.Errorf("batch create notes: %w", err)
		}
	}

	updates := make([]*notebook.NoteRecord, 0, len(state.updateNotes))
	for _, n := range state.updateNotes {
		updates = append(updates, n)
	}
	if !opts.DryRun && (len(updates) > 0 || len(state.newNNs) > 0) {
		if err := imp.noteRepo.BatchUpdate(ctx, updates, state.newNNs); err != nil {
			return nil, fmt.Errorf("batch update notes: %w", err)
		}
	}

	return state.result, nil
}

func (imp *Importer) classifyNote(def notebook.Note, notebookID, notebookType, group, subgroup string, opts ImportOptions, state *classifyState) {
	usage := def.Expression
	entry := def.Definition
	if entry == "" {
		entry = def.Expression
	}

	key := noteKey{usage, entry}
	existing := state.noteCache[key]

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
		state.newNotes = append(state.newNotes, n)
		state.noteCache[key] = n
		_, _ = fmt.Fprintf(imp.writer, "  [NEW]  %q (%s)\n", usage, entry)
		state.result.NotesNew++
		state.result.NotebookNew++
		return
	}

	// Note exists in cache -- could be a pending new note (ID==0) or existing DB note
	if existing.ID == 0 {
		// Already pending creation -- just add another notebook_note
		existing.NotebookNotes = append(existing.NotebookNotes, notebook.NotebookNote{
			NotebookType: notebookType, NotebookID: notebookID, Group: group, Subgroup: subgroup,
		})
		state.result.NotebookNew++
		return
	}

	// Existing DB note: handle skip/update
	if opts.UpdateExisting {
		existing.Meaning = def.Meaning
		existing.Level = string(def.Level)
		existing.DictionaryNumber = def.DictionaryNumber
		if _, alreadyCounted := state.updateNotes[key]; !alreadyCounted {
			state.result.NotesUpdated++
		}
		state.updateNotes[key] = existing
		_, _ = fmt.Fprintf(imp.writer, "  [UPDATE]  %q (%s)\n", usage, entry)
	} else {
		_, _ = fmt.Fprintf(imp.writer, "  [SKIP]  %q (%s)\n", usage, entry)
		state.result.NotesSkipped++
	}

	// Check notebook_note link
	nnk := nnKey{existing.ID, notebookType, notebookID, group, subgroup}
	if state.nnCache[nnk] {
		state.result.NotebookSkipped++
		return
	}

	state.newNNs = append(state.newNNs, notebook.NotebookNote{
		NoteID: existing.ID, NotebookType: notebookType, NotebookID: notebookID, Group: group, Subgroup: subgroup,
	})
	state.nnCache[nnk] = true
	state.result.NotebookNew++
}
