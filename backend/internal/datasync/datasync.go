// Package datasync provides import/export orchestration between YAML files and database.
package datasync

//go:generate mockgen -source=datasync.go -destination=../mocks/datasync/mock_datasync.go -package=mock_datasync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

type noteKey struct{ usage, entry string }

type logKey struct {
	noteID    int64
	quizType  string
	learnedAt time.Time
}

type nnKey struct {
	noteID                                    int64
	notebookType, notebookID, group, subgroup string
}

// classifyState holds mutable state passed through the classification loop.
type classifyState struct {
	result      *ImportNotesResult
	noteCache   map[noteKey]*notebook.NoteRecord
	nnCache     map[nnKey]bool
	newNotes    []*notebook.NoteRecord
	updateNotes map[noteKey]*notebook.NoteRecord
	newNNs      []notebook.NotebookNote
}

// NoteSource provides source note records for import.
type NoteSource interface {
	FindAll(ctx context.Context) ([]notebook.NoteRecord, error)
}

// LearningSource provides learning history expressions by notebook.
type LearningSource interface {
	FindByNotebookID(notebookID string) ([]notebook.LearningHistoryExpression, error)
}

// DictionarySource provides cached dictionary API responses.
type DictionarySource interface {
	ReadAll() ([]rapidapi.Response, error)
}

// NoteSink writes exported note records.
type NoteSink interface {
	WriteAll(notes []notebook.NoteRecord) error
}

// LearningSink writes exported learning log records.
type LearningSink interface {
	WriteAll(logs []learning.LearningLog) error
}

// ImportNotesResult tracks counts for note import.
type ImportNotesResult struct {
	NotesNew, NotesSkipped, NotesUpdated int
	NotebookNew, NotebookSkipped         int
}

// ImportLearningLogsResult tracks counts for learning log import.
type ImportLearningLogsResult struct {
	NotesNew        int
	LearningNew     int
	LearningSkipped int
}

// ImportDictionaryResult tracks counts for dictionary import.
type ImportDictionaryResult struct {
	DictionaryNew, DictionarySkipped, DictionaryUpdated int
}

// ImportOptions controls import behavior.
type ImportOptions struct {
	DryRun         bool
	UpdateExisting bool
}

// Importer reads YAML notebook data and writes to DB.
type Importer struct {
	noteRepo         notebook.NoteRepository
	learningRepo     learning.LearningRepository
	noteSource       NoteSource
	learningSource   LearningSource
	dictionarySource DictionarySource
	dictionaryRepo   dictionary.DictionaryRepository
	writer           io.Writer
}

// NewImporter creates a new Importer.
func NewImporter(noteRepo notebook.NoteRepository, learningRepo learning.LearningRepository, noteSource NoteSource, learningSource LearningSource, dictionarySource DictionarySource, dictionaryRepo dictionary.DictionaryRepository, writer io.Writer) *Importer {
	return &Importer{
		noteRepo:         noteRepo,
		learningRepo:     learningRepo,
		noteSource:       noteSource,
		learningSource:   learningSource,
		dictionarySource: dictionarySource,
		dictionaryRepo:   dictionaryRepo,
		writer:           writer,
	}
}

// ImportNotes reads source notes and imports them into the database.
func (imp *Importer) ImportNotes(ctx context.Context, opts ImportOptions) (*ImportNotesResult, error) {
	sourceNotes, err := imp.noteSource.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("read source notes: %w", err)
	}

	allNotes, err := imp.noteRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load existing notes: %w", err)
	}

	state := &classifyState{
		result:      &ImportNotesResult{},
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

// ImportLearningLogs imports learning history YAML data into the database.
func (imp *Importer) ImportLearningLogs(ctx context.Context, opts ImportOptions) (*ImportLearningLogsResult, error) {
	var result ImportLearningLogsResult

	allNotes, err := imp.noteRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load existing notes: %w", err)
	}
	noteMap := make(map[string]*notebook.NoteRecord, len(allNotes))
	for i := range allNotes {
		noteMap[allNotes[i].Entry] = &allNotes[i]
	}

	allLogs, err := imp.learningRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load existing learning logs: %w", err)
	}
	logCache := make(map[logKey]bool, len(allLogs))
	for _, l := range allLogs {
		logCache[logKey{l.NoteID, l.QuizType, l.LearnedAt}] = true
	}

	// Extract unique notebook IDs from notes
	notebookIDs := make(map[string]bool)
	for _, n := range allNotes {
		for _, nn := range n.NotebookNotes {
			notebookIDs[nn.NotebookID] = true
		}
	}
	sortedIDs := make([]string, 0, len(notebookIDs))
	for id := range notebookIDs {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)

	// Collect all expressions from the source
	var allExpressions []notebook.LearningHistoryExpression
	for _, id := range sortedIDs {
		exprs, err := imp.learningSource.FindByNotebookID(id)
		if err != nil {
			return nil, fmt.Errorf("find expressions for notebook %s: %w", id, err)
		}
		allExpressions = append(allExpressions, exprs...)
	}

	// First pass: batch-create auto notes for unknown expressions
	newNoteEntries := make(map[string]bool)
	var newNotes []*notebook.NoteRecord
	for _, expr := range allExpressions {
		if _, ok := noteMap[expr.Expression]; !ok && !newNoteEntries[expr.Expression] {
			newNoteEntries[expr.Expression] = true
			n := &notebook.NoteRecord{
				Usage: expr.Expression,
				Entry: expr.Expression,
			}
			newNotes = append(newNotes, n)
			_, _ = fmt.Fprintf(imp.writer, "  [NEW]  %q (%s)\n", expr.Expression, expr.Expression)
			result.NotesNew++
		}
	}

	if !opts.DryRun && len(newNotes) > 0 {
		if err := imp.noteRepo.BatchCreate(ctx, newNotes); err != nil {
			return nil, fmt.Errorf("batch create notes: %w", err)
		}
		// Re-fetch all notes to get the auto-generated IDs
		allNotes, err = imp.noteRepo.FindAll(ctx)
		if err != nil {
			return nil, fmt.Errorf("reload notes after batch create: %w", err)
		}
		noteMap = make(map[string]*notebook.NoteRecord, len(allNotes))
		for i := range allNotes {
			noteMap[allNotes[i].Entry] = &allNotes[i]
		}
	}

	// Second pass: collect learning logs with correct note IDs
	var newLogs []*learning.LearningLog
	for _, expr := range allExpressions {
		n, ok := noteMap[expr.Expression]
		if !ok {
			continue
		}

		for _, rec := range expr.LearnedLogs {
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
				EasinessFactor: expr.EasinessFactor,
			})
			logCache[logKey{n.ID, quizType, rec.LearnedAt.Time}] = true
			result.LearningNew++
		}

		for _, rec := range expr.ReverseLogs {
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
				EasinessFactor: expr.ReverseEasinessFactor,
			})
			logCache[logKey{n.ID, quizType, rec.LearnedAt.Time}] = true
			result.LearningNew++
		}
	}

	if !opts.DryRun && len(newLogs) > 0 {
		if err := imp.learningRepo.BatchCreate(ctx, newLogs); err != nil {
			return nil, fmt.Errorf("batch create learning logs: %w", err)
		}
	}

	return &result, nil
}

// ImportDictionary reads dictionary responses and imports them into the database.
func (imp *Importer) ImportDictionary(ctx context.Context, opts ImportOptions) (*ImportDictionaryResult, error) {
	responses, err := imp.dictionarySource.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read dictionary source: %w", err)
	}

	var result ImportDictionaryResult

	allEntries, err := imp.dictionaryRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load existing dictionary entries: %w", err)
	}
	entryCache := make(map[string]*dictionary.DictionaryEntry, len(allEntries))
	for i := range allEntries {
		entryCache[allEntries[i].Word] = &allEntries[i]
	}

	var batch []*dictionary.DictionaryEntry
	for _, resp := range responses {
		data, err := json.Marshal(resp)
		if err != nil {
			return nil, fmt.Errorf("marshal dictionary response: %w", err)
		}

		existing := entryCache[resp.Word]
		if existing == nil {
			batch = append(batch, &dictionary.DictionaryEntry{
				Word:       resp.Word,
				SourceType: "rapidapi",
				Response:   data,
			})
			result.DictionaryNew++
			continue
		}

		if !opts.UpdateExisting {
			result.DictionarySkipped++
			continue
		}

		existing.Response = data
		batch = append(batch, existing)
		result.DictionaryUpdated++
	}

	if !opts.DryRun {
		if err := imp.dictionaryRepo.BatchUpsert(ctx, batch); err != nil {
			return nil, fmt.Errorf("upsert dictionary entries: %w", err)
		}
	}

	return &result, nil
}

// ExportNotesResult tracks counts for note export.
type ExportNotesResult struct {
	NotesExported, NotebookNotesExported int
}

// ExportLearningLogsResult tracks counts for learning log export.
type ExportLearningLogsResult struct {
	LearningLogsExported int
}

// Exporter reads DB data and writes to YAML files.
type Exporter struct {
	noteRepo     notebook.NoteRepository
	learningRepo learning.LearningRepository
	noteSink     NoteSink
	learningSink LearningSink
	writer       io.Writer
}

// NewExporter creates a new Exporter.
func NewExporter(noteRepo notebook.NoteRepository, learningRepo learning.LearningRepository, noteSink NoteSink, learningSink LearningSink, writer io.Writer) *Exporter {
	return &Exporter{
		noteRepo:     noteRepo,
		learningRepo: learningRepo,
		noteSink:     noteSink,
		learningSink: learningSink,
		writer:       writer,
	}
}

// ExportNotes reads notes from the database and writes them to YAML.
func (exp *Exporter) ExportNotes(ctx context.Context) (*ExportNotesResult, error) {
	notes, err := exp.noteRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load notes: %w", err)
	}

	if err := exp.noteSink.WriteAll(notes); err != nil {
		return nil, fmt.Errorf("write notes: %w", err)
	}

	var nnCount int
	for _, n := range notes {
		nnCount += len(n.NotebookNotes)
	}

	_, _ = fmt.Fprintf(exp.writer, "  Exported %d notes, %d notebook_notes\n", len(notes), nnCount)
	return &ExportNotesResult{
		NotesExported:         len(notes),
		NotebookNotesExported: nnCount,
	}, nil
}

// ExportLearningLogs reads learning logs from the database and writes them to YAML.
func (exp *Exporter) ExportLearningLogs(ctx context.Context) (*ExportLearningLogsResult, error) {
	logs, err := exp.learningRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load learning logs: %w", err)
	}

	if err := exp.learningSink.WriteAll(logs); err != nil {
		return nil, fmt.Errorf("write learning logs: %w", err)
	}

	_, _ = fmt.Fprintf(exp.writer, "  Exported %d learning logs\n", len(logs))
	return &ExportLearningLogsResult{
		LearningLogsExported: len(logs),
	}, nil
}
