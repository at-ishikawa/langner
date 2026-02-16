// Package datasync provides import/export orchestration between YAML files and database.
package datasync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

type noteKey struct{ usage, entry string }

type nnKey struct {
	noteID                                 int64
	notebookType, notebookID, group string
}

type logKey struct {
	noteID    int64
	quizType  string
	learnedAt time.Time
}

// ImportResult tracks counts for each import operation.
type ImportResult struct {
	NotesNew          int
	NotesSkipped      int
	NotesUpdated      int
	NotebookNew       int
	NotebookSkipped   int
	LearningNew       int
	LearningSkipped   int
	LearningWarnings  int
	DictionaryNew     int
	DictionarySkipped int
	DictionaryUpdated int
}

// ImportOptions controls import behavior.
type ImportOptions struct {
	DryRun         bool
	UpdateExisting bool
}

// Importer reads YAML notebook data and writes to DB.
type Importer struct {
	noteRepo       notebook.NoteRepository
	learningRepo   learning.LearningRepository
	dictionaryRepo dictionary.DictionaryRepository
	writer         io.Writer
}

// NewImporter creates a new Importer.
func NewImporter(noteRepo notebook.NoteRepository, learningRepo learning.LearningRepository, dictionaryRepo dictionary.DictionaryRepository, writer io.Writer) *Importer {
	return &Importer{
		noteRepo:       noteRepo,
		learningRepo:   learningRepo,
		dictionaryRepo: dictionaryRepo,
		writer:         writer,
	}
}

// ImportNotes imports notes and notebook_note links from story and flashcard indexes.
func (imp *Importer) ImportNotes(ctx context.Context, storyIndexes map[string]notebook.Index, flashcardIndexes map[string]notebook.FlashcardIndex, opts ImportOptions) (*ImportResult, error) {
	var result ImportResult

	allNotes, err := imp.noteRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("FindAll() > %w", err)
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
							return nil, fmt.Errorf("importNote() > %w", err)
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
					return nil, fmt.Errorf("importNote() > %w", err)
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

	images := make([]notebook.NoteImage, len(def.Images))
	for i, img := range def.Images {
		images[i] = notebook.NoteImage{
			URL:       img,
			SortOrder: i,
		}
	}

	references := make([]notebook.NoteReference, len(def.References))
	for i, ref := range def.References {
		references[i] = notebook.NoteReference{
			Link:        ref.URL,
			Description: ref.Description,
			SortOrder:   i,
		}
	}

	existing := noteCache[noteKey{usage, entry}]

	var noteID int64
	if existing != nil {
		noteID = existing.ID
		if !opts.UpdateExisting {
			fmt.Fprintf(imp.writer, "  [SKIP]  %q (%s)\n", usage, entry)
			result.NotesSkipped++
		} else {
			existing.Meaning = def.Meaning
			existing.Level = string(def.Level)
			existing.DictionaryNumber = def.DictionaryNumber
			if !opts.DryRun {
				if err := imp.noteRepo.Update(ctx, existing); err != nil {
					return fmt.Errorf("Update() > %w", err)
				}
			}
			fmt.Fprintf(imp.writer, "  [UPDATE]  %q (%s)\n", usage, entry)
			result.NotesUpdated++
		}
	} else {
		n := &notebook.NoteRecord{
			Usage:            usage,
			Entry:            entry,
			Meaning:          def.Meaning,
			Level:            string(def.Level),
			DictionaryNumber: def.DictionaryNumber,
			Images:           images,
			References:       references,
			NotebookNotes: []notebook.NotebookNote{
				{
					NotebookType: notebookType,
					NotebookID:   notebookID,
					Group:        group,
					Subgroup:     subgroup,
				},
			},
		}
		if !opts.DryRun {
			if err := imp.noteRepo.Create(ctx, n); err != nil {
				return fmt.Errorf("Create() > %w", err)
			}
			noteID = n.ID
		}
		noteCache[noteKey{usage, entry}] = n
		fmt.Fprintf(imp.writer, "  [NEW]  %q (%s)\n", usage, entry)
		result.NotesNew++

		// Notebook note was created inline with the note
		result.NotebookNew++
		return nil
	}

	// Create notebook_note link for existing notes
	if nnCache[nnKey{noteID, notebookType, notebookID, group}] {
		result.NotebookSkipped++
		return nil
	}

	if !opts.DryRun {
		nn := &notebook.NotebookNote{
			NoteID:       noteID,
			NotebookType: notebookType,
			NotebookID:   notebookID,
			Group:        group,
			Subgroup:     subgroup,
		}
		if err := imp.noteRepo.CreateNotebookNote(ctx, nn); err != nil {
			return fmt.Errorf("CreateNotebookNote() > %w", err)
		}
	}
	nnCache[nnKey{noteID, notebookType, notebookID, group}] = true
	result.NotebookNew++
	return nil
}

// ImportLearningLogs imports learning history records into the database.
func (imp *Importer) ImportLearningLogs(ctx context.Context, learningHistories map[string][]notebook.LearningHistory, opts ImportOptions) (*ImportResult, error) {
	var result ImportResult

	allNotes, err := imp.noteRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("FindAll() > %w", err)
	}
	noteMap := make(map[string]*notebook.NoteRecord, len(allNotes))
	for i := range allNotes {
		noteMap[allNotes[i].Entry] = &allNotes[i]
	}

	allLogs, err := imp.learningRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("learningRepo.FindAll() > %w", err)
	}
	logCache := make(map[logKey]bool, len(allLogs))
	for _, l := range allLogs {
		logCache[logKey{l.NoteID, l.QuizType, l.LearnedAt}] = true
	}

	// Collect all expressions across all histories
	type historyExpression struct {
		expr notebook.LearningHistoryExpression
	}
	var allExpressions []historyExpression
	for _, histories := range learningHistories {
		for _, h := range histories {
			if h.Metadata.Type == "flashcard" {
				for _, expr := range h.Expressions {
					allExpressions = append(allExpressions, historyExpression{expr: expr})
				}
			} else {
				for _, scene := range h.Scenes {
					for _, expr := range scene.Expressions {
						allExpressions = append(allExpressions, historyExpression{expr: expr})
					}
				}
			}
		}
	}

	// First pass: batch-create auto notes for unknown expressions
	newNoteEntries := make(map[string]bool)
	var newNotes []*notebook.NoteRecord
	for _, he := range allExpressions {
		if _, ok := noteMap[he.expr.Expression]; !ok && !newNoteEntries[he.expr.Expression] {
			newNoteEntries[he.expr.Expression] = true
			n := &notebook.NoteRecord{
				Usage: he.expr.Expression,
				Entry: he.expr.Expression,
			}
			newNotes = append(newNotes, n)
			fmt.Fprintf(imp.writer, "  [NEW]  %q (%s)\n", he.expr.Expression, he.expr.Expression)
			result.NotesNew++
		}
	}

	if !opts.DryRun && len(newNotes) > 0 {
		if err := imp.noteRepo.BatchCreate(ctx, newNotes); err != nil {
			return nil, fmt.Errorf("BatchCreate() > %w", err)
		}
		// Re-fetch all notes to get the auto-generated IDs
		allNotes, err = imp.noteRepo.FindAll(ctx)
		if err != nil {
			return nil, fmt.Errorf("FindAll() > %w", err)
		}
		noteMap = make(map[string]*notebook.NoteRecord, len(allNotes))
		for i := range allNotes {
			noteMap[allNotes[i].Entry] = &allNotes[i]
		}
	}

	// Second pass: collect learning logs with correct note IDs
	var newLogs []*learning.LearningLog
	for _, he := range allExpressions {
		n, ok := noteMap[he.expr.Expression]
		if !ok {
			// In dry-run mode, notes weren't actually created
			continue
		}

		for _, rec := range he.expr.LearnedLogs {
			quizType := rec.QuizType
			if quizType == "" {
				quizType = "notebook"
			}
			if logCache[logKey{n.ID, quizType, rec.LearnedAt.Time}] {
				result.LearningSkipped++
				continue
			}
			newLogs = append(newLogs, &learning.LearningLog{
				NoteID:         n.ID,
				Status:         string(rec.Status),
				LearnedAt:      rec.LearnedAt.Time,
				Quality:        rec.Quality,
				ResponseTimeMs: int(rec.ResponseTimeMs),
				QuizType:       quizType,
				IntervalDays:   rec.IntervalDays,
				EasinessFactor: he.expr.EasinessFactor,
			})
			logCache[logKey{n.ID, quizType, rec.LearnedAt.Time}] = true
			result.LearningNew++
		}

		for _, rec := range he.expr.ReverseLogs {
			quizType := "reverse"
			if logCache[logKey{n.ID, quizType, rec.LearnedAt.Time}] {
				result.LearningSkipped++
				continue
			}
			newLogs = append(newLogs, &learning.LearningLog{
				NoteID:         n.ID,
				Status:         string(rec.Status),
				LearnedAt:      rec.LearnedAt.Time,
				Quality:        rec.Quality,
				ResponseTimeMs: int(rec.ResponseTimeMs),
				QuizType:       quizType,
				IntervalDays:   rec.IntervalDays,
				EasinessFactor: he.expr.ReverseEasinessFactor,
			})
			logCache[logKey{n.ID, quizType, rec.LearnedAt.Time}] = true
			result.LearningNew++
		}
	}

	if !opts.DryRun && len(newLogs) > 0 {
		if err := imp.learningRepo.BatchCreate(ctx, newLogs); err != nil {
			return nil, fmt.Errorf("BatchCreate() > %w", err)
		}
	}

	return &result, nil
}

// ImportDictionary imports dictionary API responses into the database.
func (imp *Importer) ImportDictionary(ctx context.Context, responses []rapidapi.Response, opts ImportOptions) (*ImportResult, error) {
	var result ImportResult

	allEntries, err := imp.dictionaryRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("dictionaryRepo.FindAll() > %w", err)
	}
	entryCache := make(map[string]*dictionary.DictionaryEntry, len(allEntries))
	for i := range allEntries {
		entryCache[allEntries[i].Word] = &allEntries[i]
	}

	var newEntries []*dictionary.DictionaryEntry
	var updateEntries []*dictionary.DictionaryEntry
	for _, resp := range responses {
		data, err := json.Marshal(resp)
		if err != nil {
			return nil, fmt.Errorf("json.Marshal() > %w", err)
		}

		existing := entryCache[resp.Word]

		if existing != nil {
			if !opts.UpdateExisting {
				result.DictionarySkipped++
				continue
			}
			existing.Response = data
			updateEntries = append(updateEntries, existing)
			result.DictionaryUpdated++
			continue
		}

		newEntries = append(newEntries, &dictionary.DictionaryEntry{
			Word:       resp.Word,
			SourceType: "rapidapi",
			Response:   data,
		})
		result.DictionaryNew++
	}

	if !opts.DryRun {
		allBatch := append(newEntries, updateEntries...)
		if err := imp.dictionaryRepo.BatchUpsert(ctx, allBatch); err != nil {
			return nil, fmt.Errorf("BatchUpsert() > %w", err)
		}
	}

	return &result, nil
}
