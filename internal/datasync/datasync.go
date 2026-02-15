// Package datasync provides import/export orchestration between YAML files and database.
package datasync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/note"
	"github.com/at-ishikawa/langner/internal/notebook"
)

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
	noteRepo       note.NoteRepository
	learningRepo   learning.LearningRepository
	dictionaryRepo dictionary.DictionaryRepository
	writer         io.Writer
}

// NewImporter creates a new Importer.
func NewImporter(noteRepo note.NoteRepository, learningRepo learning.LearningRepository, dictionaryRepo dictionary.DictionaryRepository, writer io.Writer) *Importer {
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
						if err := imp.importNote(ctx, def, indexID, notebookType, sn.Event, scene.Title, opts, &result); err != nil {
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
				if err := imp.importNote(ctx, card, flashcardID, "flashcard", fn.Title, "", opts, &result); err != nil {
					return nil, fmt.Errorf("importNote() > %w", err)
				}
			}
		}
	}

	return &result, nil
}

func (imp *Importer) importNote(ctx context.Context, def notebook.Note, notebookID, notebookType, group, subgroup string, opts ImportOptions, result *ImportResult) error {
	usage := def.Expression
	entry := def.Definition
	if entry == "" {
		entry = def.Expression
	}

	images := make([]note.NoteImage, len(def.Images))
	for i, img := range def.Images {
		images[i] = note.NoteImage{
			URL:       img,
			SortOrder: i,
		}
	}

	references := make([]note.NoteReference, len(def.References))
	for i, ref := range def.References {
		references[i] = note.NoteReference{
			Link:        ref.URL,
			Description: ref.Description,
			SortOrder:   i,
		}
	}

	existing, err := imp.noteRepo.FindByUsageAndEntry(ctx, usage, entry)
	if err != nil {
		return fmt.Errorf("FindByUsageAndEntry(%s, %s) > %w", usage, entry, err)
	}

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
		n := &note.Note{
			Usage:            usage,
			Entry:            entry,
			Meaning:          def.Meaning,
			Level:            string(def.Level),
			DictionaryNumber: def.DictionaryNumber,
			Images:           images,
			References:       references,
			NotebookNotes: []note.NotebookNote{
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
		fmt.Fprintf(imp.writer, "  [NEW]  %q (%s)\n", usage, entry)
		result.NotesNew++

		// Notebook note was created inline with the note
		result.NotebookNew++
		return nil
	}

	// Create notebook_note link for existing notes
	existingNN, err := imp.noteRepo.FindNotebookNote(ctx, noteID, notebookType, notebookID, group)
	if err != nil {
		return fmt.Errorf("FindNotebookNote() > %w", err)
	}
	if existingNN != nil {
		result.NotebookSkipped++
		return nil
	}

	if !opts.DryRun {
		nn := &note.NotebookNote{
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
	noteMap := make(map[string]*note.Note, len(allNotes))
	for i := range allNotes {
		noteMap[allNotes[i].Entry] = &allNotes[i]
	}

	for _, histories := range learningHistories {
		for _, h := range histories {
			var expressions []notebook.LearningHistoryExpression

			if h.Metadata.Type == "flashcard" {
				expressions = h.Expressions
			} else {
				for _, scene := range h.Scenes {
					expressions = append(expressions, scene.Expressions...)
				}
			}

			for _, expr := range expressions {
				n, ok := noteMap[expr.Expression]
				if !ok {
					fmt.Fprintf(imp.writer, "  [WARN]  note not found for %q\n", expr.Expression)
					result.LearningWarnings++
					continue
				}

				if err := imp.importLearningRecords(ctx, n.ID, expr.LearnedLogs, expr.EasinessFactor, false, opts, &result); err != nil {
					return nil, fmt.Errorf("importLearningRecords() > %w", err)
				}
				if err := imp.importLearningRecords(ctx, n.ID, expr.ReverseLogs, expr.ReverseEasinessFactor, true, opts, &result); err != nil {
					return nil, fmt.Errorf("importLearningRecords(reverse) > %w", err)
				}
			}
		}
	}

	return &result, nil
}

func (imp *Importer) importLearningRecords(ctx context.Context, noteID int64, records []notebook.LearningRecord, easinessFactor float64, forceReverse bool, opts ImportOptions, result *ImportResult) error {
	for _, rec := range records {
		quizType := rec.QuizType
		if quizType == "" {
			quizType = "notebook"
		}
		if forceReverse {
			quizType = "reverse"
		}

		existing, err := imp.learningRepo.FindByNoteQuizTypeAndLearnedAt(ctx, noteID, quizType, rec.LearnedAt.Time)
		if err != nil {
			return fmt.Errorf("FindByNoteQuizTypeAndLearnedAt() > %w", err)
		}
		if existing != nil {
			result.LearningSkipped++
			continue
		}

		if !opts.DryRun {
			log := &learning.LearningLog{
				NoteID:         noteID,
				Status:         string(rec.Status),
				LearnedAt:      rec.LearnedAt.Time,
				Quality:        rec.Quality,
				ResponseTimeMs: int(rec.ResponseTimeMs),
				QuizType:       quizType,
				IntervalDays:   rec.IntervalDays,
				EasinessFactor: easinessFactor,
			}
			if err := imp.learningRepo.Create(ctx, log); err != nil {
				return fmt.Errorf("Create() > %w", err)
			}
		}
		result.LearningNew++
	}
	return nil
}

// ImportDictionary imports dictionary API responses into the database.
func (imp *Importer) ImportDictionary(ctx context.Context, responses []rapidapi.Response, opts ImportOptions) (*ImportResult, error) {
	var result ImportResult

	for _, resp := range responses {
		data, err := json.Marshal(resp)
		if err != nil {
			return nil, fmt.Errorf("json.Marshal() > %w", err)
		}

		existing, err := imp.dictionaryRepo.FindByWord(ctx, resp.Word)
		if err != nil {
			return nil, fmt.Errorf("FindByWord(%s) > %w", resp.Word, err)
		}

		if existing != nil {
			if !opts.UpdateExisting {
				result.DictionarySkipped++
				continue
			}
			existing.Response = data
			if !opts.DryRun {
				if err := imp.dictionaryRepo.Upsert(ctx, existing); err != nil {
					return nil, fmt.Errorf("Upsert() > %w", err)
				}
			}
			result.DictionaryUpdated++
			continue
		}

		entry := &dictionary.DictionaryEntry{
			Word:       resp.Word,
			SourceType: "rapidapi",
			Response:   data,
		}
		if !opts.DryRun {
			if err := imp.dictionaryRepo.Upsert(ctx, entry); err != nil {
				return nil, fmt.Errorf("Upsert() > %w", err)
			}
		}
		result.DictionaryNew++
	}

	return &result, nil
}

// ExportData holds all exported data from the database.
type ExportData struct {
	Notes             []note.Note
	LearningLogs      []learning.LearningLog
	DictionaryEntries []dictionary.DictionaryEntry
}

// Exporter reads DB and returns domain structs.
type Exporter struct {
	noteRepo       note.NoteRepository
	learningRepo   learning.LearningRepository
	dictionaryRepo dictionary.DictionaryRepository
}

// NewExporter creates a new Exporter.
func NewExporter(noteRepo note.NoteRepository, learningRepo learning.LearningRepository, dictionaryRepo dictionary.DictionaryRepository) *Exporter {
	return &Exporter{
		noteRepo:       noteRepo,
		learningRepo:   learningRepo,
		dictionaryRepo: dictionaryRepo,
	}
}

// Export reads all data from the database.
func (e *Exporter) Export(ctx context.Context) (*ExportData, error) {
	notes, err := e.noteRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("noteRepo.FindAll() > %w", err)
	}

	logs, err := e.learningRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("learningRepo.FindAll() > %w", err)
	}

	entries, err := e.dictionaryRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("dictionaryRepo.FindAll() > %w", err)
	}

	return &ExportData{
		Notes:             notes,
		LearningLogs:      logs,
		DictionaryEntries: entries,
	}, nil
}
