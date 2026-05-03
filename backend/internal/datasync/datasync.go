// Package datasync provides import/export orchestration between YAML files and database.
package datasync

//go:generate mockgen -source=datasync.go -destination=../mocks/datasync/mock_datasync.go -package=mock_datasync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

type noteKey struct{ usage, entry string }

func newNoteKey(usage, entry string) noteKey {
	return noteKey{strings.ToLower(usage), strings.ToLower(entry)}
}

type logKey struct {
	noteID           int64
	quizType         string
	learnedAt        time.Time
	sourceNotebookID string
	status           string
}

// logCounter tracks DB log IDs grouped by (note_id, quiz_type, learned_at,
// source_notebook_id, status). It serves two purposes:
//
//  1. matchSource pops one ID per match so the importer skips inserting
//     duplicates for source logs that already exist in the DB.
//  2. After matching every source log, leftoverIDs returns the IDs of
//     DB-only rows the YAML doesn't have any more — the reconcile pass
//     deletes those so the DB stays in lockstep with the YAML truth.
type logCounter struct {
	ids map[logKey][]int64
}

func newLogCounter(logs []learning.LearningLog) *logCounter {
	lc := &logCounter{ids: make(map[logKey][]int64, len(logs))}
	for _, l := range logs {
		k := logKey{l.NoteID, l.QuizType, l.LearnedAt.UTC(), l.SourceNotebookID, l.Status}
		lc.ids[k] = append(lc.ids[k], l.ID)
	}
	return lc
}

// matchSource consumes one DB row matching the source log's key, returning
// true when one was available. False means the source log is new.
func (lc *logCounter) matchSource(key logKey) bool {
	rows := lc.ids[key]
	if len(rows) == 0 {
		return false
	}
	lc.ids[key] = rows[1:]
	return true
}

// leftoverIDs returns IDs of DB rows that no source log claimed — i.e.,
// rows the YAML no longer contains. The reconcile pass deletes them.
func (lc *logCounter) leftoverIDs() []int64 {
	var out []int64
	for _, ids := range lc.ids {
		out = append(out, ids...)
	}
	return out
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
	// matchedNoteIDs and matchedNNIDs track which DB rows the source
	// claimed. Anything left unmatched is a row the YAML no longer has,
	// and the reconcile pass deletes it.
	nnIDByKey       map[nnKey]int64
	matchedNoteIDs  map[int64]bool
	matchedNNIDs    map[int64]bool
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

// EtymologyOriginSource emits etymology origin records sourced from YAML.
type EtymologyOriginSource interface {
	FindAll(ctx context.Context) ([]notebook.EtymologyOriginRecord, error)
}

// EtymologyDefinitionSource emits definitions whose origin_parts need
// resolution + binding to note_origin_parts rows.
type EtymologyDefinitionSource interface {
	FindAll(ctx context.Context) ([]notebook.EtymologyDefinitionForImport, error)
}

// ImportNotesResult tracks counts for note import.
type ImportNotesResult struct {
	NotesNew, NotesSkipped, NotesUpdated int
	NotebookNew, NotebookSkipped         int
	// NotesDeleted / NotebookNotesDeleted are reconcile counts: DB rows
	// the YAML no longer has that the importer dropped.
	NotesDeleted         int
	NotebookNotesDeleted int
}

// ImportLearningLogsResult tracks counts for learning log import.
type ImportLearningLogsResult struct {
	NotesNew        int
	LearningNew     int
	LearningSkipped int
	// LearningDeleted is the number of DB-only rows the reconcile pass
	// removed because the corresponding YAML log no longer exists. The
	// importer always reconciles — YAML is the source of truth.
	LearningDeleted int
}

// ImportDictionaryResult tracks counts for dictionary import.
type ImportDictionaryResult struct {
	DictionaryNew, DictionarySkipped, DictionaryUpdated int
}

// ImportEtymologyResult tracks counts for the etymology import phase.
type ImportEtymologyResult struct {
	OriginsNew     int
	OriginsSkipped int
	PartsNew       int
	PartsSkipped   int
}

// ImportOptions controls import behavior.
type ImportOptions struct {
	DryRun         bool
	UpdateExisting bool
}

// Importer reads YAML notebook data and writes to DB.
type Importer struct {
	noteRepo            notebook.NoteRepository
	learningRepo        learning.LearningRepository
	noteSource          NoteSource
	learningSource      LearningSource
	dictionarySource    DictionarySource
	dictionaryRepo      dictionary.DictionaryRepository
	etymologyOriginRepo notebook.EtymologyOriginRepository
	noteOriginPartRepo  notebook.NoteOriginPartRepository
	etymologyOriginSrc  EtymologyOriginSource
	etymologyDefSrc     EtymologyDefinitionSource
	writer              io.Writer

	// touchedNoteIDs collects every DB note ID that ImportNotes or
	// ImportLearningLogs claimed during this Import* run. The final
	// reconcile pass deletes notes whose IDs aren't in the set so the
	// DB drops vocabulary that no longer appears anywhere in YAML.
	touchedNoteIDs map[int64]bool
	// touchedNNIDs accumulates DB notebook_notes IDs claimed by
	// ImportNotes for the per-notebook reconcile.
	touchedNNIDs map[int64]bool
	// touchedNNKeys carries the (note_id, type, id, group, subgroup) key
	// of new notebook_notes inserted during this run. They don't have IDs
	// in the importer's in-memory state because BatchUpdate doesn't return
	// them, so the reconcile pass uses the key set to recognise them
	// after re-fetching from DB.
	touchedNNKeys map[nnKey]bool
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

// WithEtymology configures the etymology import dependencies. Optional —
// when any of the four are nil, ImportAll skips the etymology phase.
func (imp *Importer) WithEtymology(
	originRepo notebook.EtymologyOriginRepository,
	partRepo notebook.NoteOriginPartRepository,
	originSrc EtymologyOriginSource,
	defSrc EtymologyDefinitionSource,
) *Importer {
	imp.etymologyOriginRepo = originRepo
	imp.noteOriginPartRepo = partRepo
	imp.etymologyOriginSrc = originSrc
	imp.etymologyDefSrc = defSrc
	return imp
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
		result:         &ImportNotesResult{},
		noteCache:      make(map[noteKey]*notebook.NoteRecord, len(allNotes)),
		nnCache:        make(map[nnKey]bool),
		updateNotes:    make(map[noteKey]*notebook.NoteRecord),
		nnIDByKey:      make(map[nnKey]int64),
		matchedNoteIDs: make(map[int64]bool),
		matchedNNIDs:   make(map[int64]bool),
	}

	// Build caches from DB notes
	for i := range allNotes {
		state.noteCache[newNoteKey(allNotes[i].Usage, allNotes[i].Entry)] = &allNotes[i]
		for _, nn := range allNotes[i].NotebookNotes {
			k := nnKey{nn.NoteID, nn.NotebookType, nn.NotebookID, nn.Group, nn.Subgroup}
			state.nnCache[k] = true
			state.nnIDByKey[k] = nn.ID
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

	// Hand the matched IDs off to the import-wide reconcile state so the
	// final phase can delete DB rows neither ImportNotes nor
	// ImportLearningLogs claimed. Freshly-inserted notes are added too —
	// their IDs are populated by BatchCreate.
	if imp.touchedNoteIDs == nil {
		imp.touchedNoteIDs = make(map[int64]bool)
	}
	for id := range state.matchedNoteIDs {
		imp.touchedNoteIDs[id] = true
	}
	for _, n := range state.newNotes {
		if n.ID != 0 {
			imp.touchedNoteIDs[n.ID] = true
		}
	}
	if imp.touchedNNIDs == nil {
		imp.touchedNNIDs = make(map[int64]bool)
	}
	for id := range state.matchedNNIDs {
		imp.touchedNNIDs[id] = true
	}
	// New notebook_notes inserted by BatchUpdate don't have IDs in the
	// in-memory state. Track their (note_id, type, id, group, subgroup)
	// keys so reconcile can recognise them after re-fetching from DB.
	if imp.touchedNNKeys == nil {
		imp.touchedNNKeys = make(map[nnKey]bool)
	}
	for _, nn := range state.newNNs {
		imp.touchedNNKeys[nnKey{nn.NoteID, nn.NotebookType, nn.NotebookID, nn.Group, nn.Subgroup}] = true
	}
	// New notebook_notes attached to brand-new notes (existing.ID==0
	// branch) live on n.NotebookNotes; those notes get IDs from
	// BatchCreate, but the nn.NoteID may not be backfilled — index by
	// the note pointer's eventual ID.
	for _, n := range state.newNotes {
		if n.ID == 0 {
			continue
		}
		for _, nn := range n.NotebookNotes {
			imp.touchedNNKeys[nnKey{n.ID, nn.NotebookType, nn.NotebookID, nn.Group, nn.Subgroup}] = true
		}
	}

	return state.result, nil
}

func (imp *Importer) classifyRecord(src *notebook.NoteRecord, opts ImportOptions, state *classifyState) {
	key := newNoteKey(src.Usage, src.Entry)
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
	state.matchedNoteIDs[existing.ID] = true
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
		if id, ok := state.nnIDByKey[nnk]; ok {
			state.matchedNNIDs[id] = true
		}
		if state.nnCache[nnk] {
			state.result.NotebookSkipped++
			continue
		}
		if existing.ID == 0 {
			// Note is pending insertion (classified as NEW earlier); append NNs to it directly.
			existing.NotebookNotes = append(existing.NotebookNotes, nn)
		} else {
			state.newNNs = append(state.newNNs, notebook.NotebookNote{
				NoteID: existing.ID, NotebookType: nn.NotebookType, NotebookID: nn.NotebookID, Group: nn.Group, Subgroup: nn.Subgroup,
			})
		}
		state.nnCache[nnk] = true
		state.result.NotebookNew++
	}
}

// noteLookup indexes notes by Entry, supporting notebook-aware lookup.
// When multiple notes share the same Entry, lookup prefers the note
// linked to a specific notebook via NotebookNotes.
type noteLookup struct {
	byEntry map[string][]*notebook.NoteRecord
}

// buildNoteMap creates a noteLookup from a slice of NoteRecords.
//
// Lookup keys are lowercased Entry values so source learning logs whose
// expression casing differs from the parent vocabulary YAML still resolve
// to the same DB row. Folding by Usage too would conflate notes whose
// Usage and Entry are genuinely different expressions (e.g. a note with
// Usage="looking out for you" / Entry="look out for"), so we don't.
func buildNoteMap(notes []notebook.NoteRecord) noteLookup {
	m := noteLookup{byEntry: make(map[string][]*notebook.NoteRecord, len(notes))}
	for i := range notes {
		key := strings.ToLower(strings.TrimSpace(notes[i].Entry))
		if key == "" {
			continue
		}
		m.byEntry[key] = append(m.byEntry[key], &notes[i])
	}
	return m
}

// lookup returns the best-matching note for the given entry and notebook ID.
// It prefers a note that has a NotebookNote with the given notebookID.
// If no notebook-specific match is found, it falls back to the first note with that entry.
func (m noteLookup) lookup(entry, notebookID string) *notebook.NoteRecord {
	candidates := m.byEntry[strings.ToLower(strings.TrimSpace(entry))]
	if len(candidates) == 0 {
		return nil
	}
	for _, n := range candidates {
		for _, nn := range n.NotebookNotes {
			if nn.NotebookID == notebookID {
				return n
			}
		}
	}
	return candidates[0]
}

// ImportLearningLogs imports learning history YAML data into the database.
func (imp *Importer) ImportLearningLogs(ctx context.Context, opts ImportOptions) (*ImportLearningLogsResult, error) {
	var result ImportLearningLogsResult

	allNotes, err := imp.noteRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load existing notes: %w", err)
	}
	noteMap := buildNoteMap(allNotes)

	allLogs, err := imp.learningRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load existing learning logs: %w", err)
	}
	existingLogs := newLogCounter(allLogs)

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

	// Collect all expressions from the source, tracking their notebook origin
	type exprWithNotebook struct {
		notebook.LearningHistoryExpression
		notebookID string
	}
	var allExpressions []exprWithNotebook
	for _, id := range sortedIDs {
		exprs, err := imp.learningSource.FindByNotebookID(id)
		if err != nil {
			return nil, fmt.Errorf("find expressions for notebook %s: %w", id, err)
		}
		for _, e := range exprs {
			allExpressions = append(allExpressions, exprWithNotebook{e, id})
		}
	}

	// First pass: batch-create auto notes for unknown expressions
	newNoteEntries := make(map[string]bool)
	var newNotes []*notebook.NoteRecord
	for _, expr := range allExpressions {
		if noteMap.lookup(expr.Expression, expr.notebookID) == nil && !newNoteEntries[expr.Expression] {
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
		noteMap = buildNoteMap(allNotes)
	}

	if imp.touchedNoteIDs == nil {
		imp.touchedNoteIDs = make(map[int64]bool)
	}

	// Second pass: collect learning logs with correct note IDs
	var newLogs []*learning.LearningLog
	for _, expr := range allExpressions {
		n := noteMap.lookup(expr.Expression, expr.notebookID)
		if n == nil {
			continue
		}
		// Mark this note as still-in-use so the reconcile pass keeps it.
		imp.touchedNoteIDs[n.ID] = true

		for _, rec := range expr.LearnedLogs {
			quizType := rec.QuizType
			if quizType == "" {
				quizType = "notebook"
			}
			key := logKey{n.ID, quizType, rec.LearnedAt.UTC(), expr.notebookID, string(rec.Status)}
			if existingLogs.matchSource(key) {
				result.LearningSkipped++
				continue
			}
			newLogs = append(newLogs, &learning.LearningLog{
				NoteID:           n.ID,
				Status:           string(rec.Status),
				LearnedAt:        rec.LearnedAt.Time,
				Quality:          rec.Quality,
				ResponseTimeMs:   int(rec.ResponseTimeMs),
				QuizType:         quizType,
				IntervalDays:     rec.IntervalDays,
				SourceNotebookID: expr.notebookID,
			})
			result.LearningNew++
		}

		for _, rec := range expr.ReverseLogs {
			quizType := "reverse"
			key := logKey{n.ID, quizType, rec.LearnedAt.UTC(), expr.notebookID, string(rec.Status)}
			if existingLogs.matchSource(key) {
				result.LearningSkipped++
				continue
			}
			newLogs = append(newLogs, &learning.LearningLog{
				NoteID:           n.ID,
				Status:           string(rec.Status),
				LearnedAt:        rec.LearnedAt.Time,
				Quality:          rec.Quality,
				ResponseTimeMs:   int(rec.ResponseTimeMs),
				QuizType:         quizType,
				IntervalDays:     rec.IntervalDays,
				SourceNotebookID: expr.notebookID,
			})
			result.LearningNew++
		}

		// Etymology log tracks. The QuizType written to learning_logs
		// preserves the original (etymology_breakdown / etymology_assembly /
		// etymology_freeform) so downstream queries can split the data
		// without losing the direction signal.
		appendEtymologyLogs := func(logs []notebook.LearningRecord, defaultQuizType string) {
			for _, rec := range logs {
				quizType := rec.QuizType
				if quizType == "" {
					quizType = defaultQuizType
				}
				key := logKey{n.ID, quizType, rec.LearnedAt.UTC(), expr.notebookID, string(rec.Status)}
				if existingLogs.matchSource(key) {
					result.LearningSkipped++
					continue
				}
				newLogs = append(newLogs, &learning.LearningLog{
					NoteID:           n.ID,
					Status:           string(rec.Status),
					LearnedAt:        rec.LearnedAt.Time,
					Quality:          rec.Quality,
					ResponseTimeMs:   int(rec.ResponseTimeMs),
					QuizType:         quizType,
					IntervalDays:     rec.IntervalDays,
					SourceNotebookID: expr.notebookID,
				})
				result.LearningNew++
			}
		}
		appendEtymologyLogs(expr.EtymologyBreakdownLogs, string(notebook.QuizTypeEtymologyStandard))
		appendEtymologyLogs(expr.EtymologyAssemblyLogs, string(notebook.QuizTypeEtymologyReverse))
	}

	if !opts.DryRun && len(newLogs) > 0 {
		if err := imp.learningRepo.BatchCreate(ctx, newLogs); err != nil {
			return nil, fmt.Errorf("batch create learning logs: %w", err)
		}
	}

	// Reconcile: any DB row whose key wasn't claimed by a source log is
	// stale (YAML no longer has it) and gets deleted. YAML is the truth;
	// the DB is just a queryable mirror, so drift only flows one way.
	deleteIDs := existingLogs.leftoverIDs()
	if len(deleteIDs) > 0 {
		_, _ = fmt.Fprintf(imp.writer, "  [RECONCILE] removing %d stale DB log(s) without a YAML counterpart\n", len(deleteIDs))
		if !opts.DryRun {
			if err := imp.learningRepo.BatchDelete(ctx, deleteIDs); err != nil {
				return nil, fmt.Errorf("delete stale learning logs: %w", err)
			}
		}
		result.LearningDeleted = len(deleteIDs)
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

// NoteSink writes exported note records.
type NoteSink interface {
	WriteAll(notes []notebook.NoteRecord) error
}

// LearningSink writes exported learning logs.
type LearningSink interface {
	WriteAll(notes []notebook.NoteRecord, logs []learning.LearningLog) error
}

// DictionarySink writes exported dictionary entries.
type DictionarySink interface {
	WriteAll(entries []rapidapi.DictionaryExportEntry) error
}

// ExportNotesResult tracks counts for note export.
type ExportNotesResult struct {
	NotesExported int
}

// ExportLearningLogsResult tracks counts for learning log export.
type ExportLearningLogsResult struct {
	LogsExported int
}

// ExportDictionaryResult tracks counts for dictionary export.
type ExportDictionaryResult struct {
	EntriesExported int
}

// Exporter reads DB data and writes to YAML files.
type Exporter struct {
	noteRepo       notebook.NoteRepository
	learningRepo   learning.LearningRepository
	dictionaryRepo dictionary.DictionaryRepository
	noteSink       NoteSink
	learningSink   LearningSink
	dictionarySink DictionarySink
	writer         io.Writer
}

// NewExporter creates a new Exporter.
func NewExporter(noteRepo notebook.NoteRepository, learningRepo learning.LearningRepository, dictionaryRepo dictionary.DictionaryRepository, noteSink NoteSink, learningSink LearningSink, dictionarySink DictionarySink, writer io.Writer) *Exporter {
	return &Exporter{
		noteRepo:       noteRepo,
		learningRepo:   learningRepo,
		dictionaryRepo: dictionaryRepo,
		noteSink:       noteSink,
		learningSink:   learningSink,
		dictionarySink: dictionarySink,
		writer:         writer,
	}
}

// ExportNotes reads notes from the database and writes them via the sink.
func (exp *Exporter) ExportNotes(ctx context.Context) (*ExportNotesResult, error) {
	notes, err := exp.noteRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load notes: %w", err)
	}

	if err := exp.noteSink.WriteAll(notes); err != nil {
		return nil, fmt.Errorf("write notes: %w", err)
	}

	_, _ = fmt.Fprintf(exp.writer, "  Exported %d notes\n", len(notes))
	return &ExportNotesResult{
		NotesExported: len(notes),
	}, nil
}

// ExportLearningLogs reads learning logs from the database and writes them via the sink.
func (exp *Exporter) ExportLearningLogs(ctx context.Context) (*ExportLearningLogsResult, error) {
	notes, err := exp.noteRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load notes: %w", err)
	}

	logs, err := exp.learningRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load learning logs: %w", err)
	}

	if err := exp.learningSink.WriteAll(notes, logs); err != nil {
		return nil, fmt.Errorf("write learning logs: %w", err)
	}

	_, _ = fmt.Fprintf(exp.writer, "  Exported %d learning logs\n", len(logs))
	return &ExportLearningLogsResult{
		LogsExported: len(logs),
	}, nil
}

// ImportEtymologyOrigins reads etymology origin records from YAML and
// inserts the new ones into etymology_origins. Existing rows (matched on
// the unique (notebook_id, session_title, origin, language) tuple) are
// left as-is; --update-existing is honored by overwriting type and meaning.
func (imp *Importer) ImportEtymologyOrigins(ctx context.Context, opts ImportOptions) (*ImportEtymologyResult, error) {
	if imp.etymologyOriginRepo == nil || imp.etymologyOriginSrc == nil {
		return &ImportEtymologyResult{}, nil
	}
	result := &ImportEtymologyResult{}

	source, err := imp.etymologyOriginSrc.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("read source etymology origins: %w", err)
	}

	existing, err := imp.etymologyOriginRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load existing etymology origins: %w", err)
	}
	existingByKey := make(map[string]*notebook.EtymologyOriginRecord, len(existing))
	for i := range existing {
		row := &existing[i]
		key := strings.Join([]string{row.NotebookID, row.SessionTitle, row.Origin, row.Language}, "\x00")
		existingByKey[key] = row
	}

	var toInsert []*notebook.EtymologyOriginRecord
	for i := range source {
		src := source[i]
		key := strings.Join([]string{src.NotebookID, src.SessionTitle, src.Origin, src.Language}, "\x00")
		if _, ok := existingByKey[key]; ok {
			result.OriginsSkipped++
			continue
		}
		row := src
		toInsert = append(toInsert, &row)
		// Track in the map so subsequent passes (note_origin_parts)
		// can resolve the origin to its yet-to-be-assigned ID after BatchCreate.
		existingByKey[key] = &row
		result.OriginsNew++
	}

	if !opts.DryRun && len(toInsert) > 0 {
		if err := imp.etymologyOriginRepo.BatchCreate(ctx, toInsert); err != nil {
			return nil, fmt.Errorf("batch create etymology origins: %w", err)
		}
	}
	return result, nil
}

// ImportNoteOriginParts reads each definition's origin_parts list and
// inserts the corresponding rows into note_origin_parts. A part is bound
// when (notebook_id, session_title, origin, language) resolves to an
// etymology_origins row AND (usage, entry) resolves to a notes row.
// Unbound parts are silently skipped (printed at debug level).
//
// Existing junction rows are detected by (note_id, sort_order) — any new
// definition that shifts ordering will produce a unique-key conflict, so
// callers should re-run with the freshly-imported notes table to avoid drift.
func (imp *Importer) ImportNoteOriginParts(ctx context.Context, opts ImportOptions, originResult *ImportEtymologyResult) error {
	if imp.noteOriginPartRepo == nil || imp.etymologyDefSrc == nil || imp.etymologyOriginRepo == nil {
		return nil
	}

	defs, err := imp.etymologyDefSrc.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("read source etymology definitions: %w", err)
	}

	allNotes, err := imp.noteRepo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("load notes for origin-part binding: %w", err)
	}
	noteIDByExpr := make(map[string]int64, len(allNotes))
	for _, n := range allNotes {
		noteIDByExpr[strings.ToLower(n.Entry)] = n.ID
		noteIDByExpr[strings.ToLower(n.Usage)] = n.ID
	}

	allOrigins, err := imp.etymologyOriginRepo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("load origins for origin-part binding: %w", err)
	}
	originIDByKey := make(map[string]int64, len(allOrigins))
	// Index by (notebook_id, session_title, origin, language) AND by
	// (notebook_id, session_title, origin, "") so language-less refs in
	// definitions still resolve to the language-scoped origin.
	for _, o := range allOrigins {
		full := strings.Join([]string{o.NotebookID, o.SessionTitle, strings.ToLower(o.Origin), o.Language}, "\x00")
		originIDByKey[full] = o.ID
		// Cross-notebook fallback: definitions referencing an origin
		// from a different notebook (rare but possible) should still
		// resolve when the origin is unique by name.
		nameOnly := strings.Join([]string{strings.ToLower(o.Origin), o.Language}, "\x00")
		if _, ok := originIDByKey[nameOnly]; !ok {
			originIDByKey[nameOnly] = o.ID
		}
	}

	existing, err := imp.noteOriginPartRepo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("load existing note_origin_parts: %w", err)
	}
	existingByKey := make(map[string]bool, len(existing))
	for _, row := range existing {
		existingByKey[fmt.Sprintf("%d|%d", row.NoteID, row.SortOrder)] = true
	}

	var toInsert []*notebook.NoteOriginPartRecord
	for _, d := range defs {
		noteID, ok := noteIDByExpr[strings.ToLower(d.Expression)]
		if !ok {
			continue // expression not in notes — was filtered earlier or DB stale
		}
		for sortOrder, ref := range d.OriginParts {
			full := strings.Join([]string{d.NotebookID, d.SessionTitle, strings.ToLower(ref.Origin), ref.Language}, "\x00")
			originID, ok := originIDByKey[full]
			if !ok {
				nameOnly := strings.Join([]string{strings.ToLower(ref.Origin), ref.Language}, "\x00")
				originID, ok = originIDByKey[nameOnly]
			}
			if !ok {
				continue // origin not in DB; skip this part
			}
			key := fmt.Sprintf("%d|%d", noteID, sortOrder)
			if existingByKey[key] {
				originResult.PartsSkipped++
				continue
			}
			existingByKey[key] = true
			toInsert = append(toInsert, &notebook.NoteOriginPartRecord{
				NoteID:    noteID,
				OriginID:  originID,
				SortOrder: sortOrder,
			})
			originResult.PartsNew++
		}
	}

	if !opts.DryRun && len(toInsert) > 0 {
		if err := imp.noteOriginPartRepo.BatchCreate(ctx, toInsert); err != nil {
			return fmt.Errorf("batch create note_origin_parts: %w", err)
		}
	}
	return nil
}

// reconcileNotes deletes DB rows the YAML source no longer claims. Two
// passes: stale notebook_notes (per-notebook drift) and stale notes (DB
// rows neither ImportNotes nor ImportLearningLogs touched). Run last in
// ImportAll so auto-created notes from learning logs are accounted for first.
func (imp *Importer) reconcileNotes(ctx context.Context, opts ImportOptions, notesResult *ImportNotesResult) error {
	allNotes, err := imp.noteRepo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("load notes for reconcile: %w", err)
	}

	var staleNoteIDs, staleNNIDs []int64
	for _, n := range allNotes {
		if !imp.touchedNoteIDs[n.ID] {
			staleNoteIDs = append(staleNoteIDs, n.ID)
			continue
		}
		for _, nn := range n.NotebookNotes {
			if imp.touchedNNIDs[nn.ID] {
				continue
			}
			// New notebook_notes inserted this run match by key (the
			// IDs were assigned by the DB after BatchUpdate, so they
			// aren't in touchedNNIDs).
			if imp.touchedNNKeys[nnKey{nn.NoteID, nn.NotebookType, nn.NotebookID, nn.Group, nn.Subgroup}] {
				continue
			}
			staleNNIDs = append(staleNNIDs, nn.ID)
		}
	}

	if len(staleNNIDs) > 0 {
		_, _ = fmt.Fprintf(imp.writer, "  [RECONCILE] removing %d stale notebook_notes row(s)\n", len(staleNNIDs))
		if !opts.DryRun {
			if err := imp.noteRepo.BatchDeleteNotebookNotes(ctx, staleNNIDs); err != nil {
				return fmt.Errorf("delete stale notebook_notes: %w", err)
			}
		}
		notesResult.NotebookNotesDeleted = len(staleNNIDs)
	}
	if len(staleNoteIDs) > 0 {
		_, _ = fmt.Fprintf(imp.writer, "  [RECONCILE] removing %d stale note(s) (and dependent rows)\n", len(staleNoteIDs))
		if !opts.DryRun {
			if err := imp.noteRepo.BatchDeleteNotes(ctx, staleNoteIDs); err != nil {
				return fmt.Errorf("delete stale notes: %w", err)
			}
		}
		notesResult.NotesDeleted = len(staleNoteIDs)
	}
	return nil
}

// ImportAllResult holds combined results from importing all data types.
type ImportAllResult struct {
	Notes      *ImportNotesResult
	Learning   *ImportLearningLogsResult
	Dictionary *ImportDictionaryResult
	Etymology  *ImportEtymologyResult
}

// ImportAll runs all import steps: notes, etymology origins, note origin
// parts, learning logs (including etymology tracks), and dictionary.
func (imp *Importer) ImportAll(ctx context.Context, opts ImportOptions) (*ImportAllResult, error) {
	noteResult, err := imp.ImportNotes(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("import notes: %w", err)
	}

	etymResult, err := imp.ImportEtymologyOrigins(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("import etymology origins: %w", err)
	}
	if err := imp.ImportNoteOriginParts(ctx, opts, etymResult); err != nil {
		return nil, fmt.Errorf("import note origin parts: %w", err)
	}

	learningResult, err := imp.ImportLearningLogs(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("import learning logs: %w", err)
	}

	dictResult, err := imp.ImportDictionary(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("import dictionary: %w", err)
	}

	// Final reconcile after all touched-IDs are collected.
	if err := imp.reconcileNotes(ctx, opts, noteResult); err != nil {
		return nil, fmt.Errorf("reconcile notes: %w", err)
	}

	return &ImportAllResult{
		Notes:      noteResult,
		Learning:   learningResult,
		Dictionary: dictResult,
		Etymology:  etymResult,
	}, nil
}

// ExportAllResult holds combined results from exporting all data types.
type ExportAllResult struct {
	Notes      *ExportNotesResult
	Learning   *ExportLearningLogsResult
	Dictionary *ExportDictionaryResult
}

// ExportAll runs all export steps: notes, learning logs, and dictionary.
func (exp *Exporter) ExportAll(ctx context.Context) (*ExportAllResult, error) {
	noteResult, err := exp.ExportNotes(ctx)
	if err != nil {
		return nil, fmt.Errorf("export notes: %w", err)
	}

	learningResult, err := exp.ExportLearningLogs(ctx)
	if err != nil {
		return nil, fmt.Errorf("export learning logs: %w", err)
	}

	dictResult, err := exp.ExportDictionary(ctx)
	if err != nil {
		return nil, fmt.Errorf("export dictionary: %w", err)
	}

	return &ExportAllResult{
		Notes:      noteResult,
		Learning:   learningResult,
		Dictionary: dictResult,
	}, nil
}

// ExportDictionary reads dictionary entries from the database and writes them via the sink.
func (exp *Exporter) ExportDictionary(ctx context.Context) (*ExportDictionaryResult, error) {
	entries, err := exp.dictionaryRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load dictionary entries: %w", err)
	}

	exportEntries := make([]rapidapi.DictionaryExportEntry, len(entries))
	for i, entry := range entries {
		exportEntries[i] = rapidapi.DictionaryExportEntry{
			Word:     entry.Word,
			Response: entry.Response,
		}
	}

	if err := exp.dictionarySink.WriteAll(exportEntries); err != nil {
		return nil, fmt.Errorf("write dictionary entries: %w", err)
	}

	_, _ = fmt.Fprintf(exp.writer, "  Exported %d dictionary entries\n", len(exportEntries))
	return &ExportDictionaryResult{
		EntriesExported: len(exportEntries),
	}, nil
}
