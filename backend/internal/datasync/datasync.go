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

// ImportNotes imports pre-converted source NoteRecords into the database.
func (imp *Importer) ImportNotes(ctx context.Context, sourceNotes []notebook.NoteRecord, opts ImportOptions) (*ImportResult, error) {
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

	// Build caches from DB notes
	for i := range allNotes {
		state.noteCache[noteKey{allNotes[i].Usage, allNotes[i].Entry}] = &allNotes[i]
		for _, nn := range allNotes[i].NotebookNotes {
			state.nnCache[nnKey{nn.NoteID, nn.NotebookType, nn.NotebookID, nn.Group, nn.Subgroup}] = true
		}
		// Clear so we only track NEW notebook_notes during classification
		allNotes[i].NotebookNotes = nil
	}

	// Classify each source record
	for i := range sourceNotes {
		imp.classifyRecord(&sourceNotes[i], opts, state)
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

func (imp *Importer) classifyRecord(src *notebook.NoteRecord, opts ImportOptions, state *classifyState) {
	key := noteKey{src.Usage, src.Entry}
	existing := state.noteCache[key]

	if existing == nil {
		// Brand new note -- all NNs are new
		state.newNotes = append(state.newNotes, src)
		state.noteCache[key] = src
		_, _ = fmt.Fprintf(imp.writer, "  [NEW]  %q (%s)\n", src.Usage, src.Entry)
		state.result.NotesNew++
		state.result.NotebookNew += len(src.NotebookNotes)
		return
	}

	// Existing DB note: handle skip/update
	if opts.UpdateExisting {
		existing.Meaning = src.Meaning
		existing.Level = src.Level
		existing.DictionaryNumber = src.DictionaryNumber
		if _, alreadyCounted := state.updateNotes[key]; !alreadyCounted {
			state.result.NotesUpdated++
		}
		state.updateNotes[key] = existing
		_, _ = fmt.Fprintf(imp.writer, "  [UPDATE]  %q (%s)\n", src.Usage, src.Entry)
	} else {
		_, _ = fmt.Fprintf(imp.writer, "  [SKIP]  %q (%s)\n", src.Usage, src.Entry)
		state.result.NotesSkipped++
	}

	// Check each notebook_note link from the source
	for _, nn := range src.NotebookNotes {
		nnk := nnKey{existing.ID, nn.NotebookType, nn.NotebookID, nn.Group, nn.Subgroup}
		if state.nnCache[nnk] {
			state.result.NotebookSkipped++
			continue
		}
		state.newNNs = append(state.newNNs, notebook.NotebookNote{
			NoteID: existing.ID, NotebookType: nn.NotebookType, NotebookID: nn.NotebookID, Group: nn.Group, Subgroup: nn.Subgroup,
		})
		state.nnCache[nnk] = true
		state.result.NotebookNew++
	}
}
