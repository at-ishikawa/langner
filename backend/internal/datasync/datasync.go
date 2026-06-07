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

// EtymologyOriginFormSource emits the YAML-declared forms for every origin
// across every etymology session. The importer uses these to populate
// etymology_origin_forms.
type EtymologyOriginFormSource interface {
	FindAll(ctx context.Context) ([]notebook.EtymologyOriginFormForImport, error)
}

// SemanticConceptSource emits one entry per concept declaration across all
// etymology sessions in the user's data.
type SemanticConceptSource interface {
	FindAll(ctx context.Context) ([]notebook.SemanticConceptForImport, error)
}

// ConceptRelationSource emits one entry per relation declaration across all
// etymology sessions in the user's data.
type ConceptRelationSource interface {
	FindAll(ctx context.Context) ([]notebook.ConceptRelationForImport, error)
}

// DefinitionConceptSource emits one entry per concepts: block declaration
// across all definitions books. Definition concepts group member
// expressions (the head + its variants) under one umbrella meaning.
type DefinitionConceptSource interface {
	FindAll(ctx context.Context) ([]notebook.DefinitionConceptForImport, error)
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
	FormsNew       int
	FormsUpdated   int
	FormsSkipped   int
	FormsDeleted   int
	// Concept and relation counts populated by ImportSemanticConcepts /
	// ImportConceptRelations. Skipped covers members whose origin couldn't be
	// resolved in the same session.
	ConceptsNew         int
	ConceptsUpdated     int
	ConceptsDeleted     int
	ConceptMembersNew   int
	ConceptMembersSkipped int
	ConceptMembersDeleted int
	RelationsNew     int
	RelationsDeleted int
	// Definition concept counts populated by ImportDefinitionConcepts.
	// Mirrors the semantic-side fields but tracks the definitions-book
	// concept tables (definition_concepts / definition_concept_members).
	DefinitionConceptsNew         int
	DefinitionConceptsUpdated     int
	DefinitionConceptsDeleted     int
	DefinitionConceptMembersNew   int
	DefinitionConceptMembersDeleted int
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
	etymologyOriginRepo     notebook.EtymologyOriginRepository
	noteOriginPartRepo      notebook.NoteOriginPartRepository
	etymologyOriginSrc      EtymologyOriginSource
	etymologyDefSrc         EtymologyDefinitionSource
	etymologyOriginFormRepo notebook.EtymologyOriginFormRepository
	etymologyOriginFormSrc  EtymologyOriginFormSource
	semanticConceptRepo     notebook.SemanticConceptRepository
	semanticConceptSrc      SemanticConceptSource
	conceptRelationRepo     notebook.ConceptRelationRepository
	conceptRelationSrc      ConceptRelationSource
	definitionConceptRepo   notebook.DefinitionConceptRepository
	definitionConceptSrc    DefinitionConceptSource
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
// Forms are configured separately via WithEtymologyForms.
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

// WithEtymologyForms configures the etymology origin forms import
// dependencies. When either is nil, ImportAll skips the forms phase and
// note_origin_parts.origin_form_id is left unpopulated.
func (imp *Importer) WithEtymologyForms(
	formRepo notebook.EtymologyOriginFormRepository,
	formSrc EtymologyOriginFormSource,
) *Importer {
	imp.etymologyOriginFormRepo = formRepo
	imp.etymologyOriginFormSrc = formSrc
	return imp
}

// WithSemanticConcepts configures the concepts import dependencies. When
// either is nil, ImportAll skips the concept and relation phases.
func (imp *Importer) WithSemanticConcepts(
	conceptRepo notebook.SemanticConceptRepository,
	conceptSrc SemanticConceptSource,
) *Importer {
	imp.semanticConceptRepo = conceptRepo
	imp.semanticConceptSrc = conceptSrc
	return imp
}

// WithConceptRelations configures the concept relations import dependencies.
// When either is nil, ImportAll skips the relations phase.
func (imp *Importer) WithConceptRelations(
	relationRepo notebook.ConceptRelationRepository,
	relationSrc ConceptRelationSource,
) *Importer {
	imp.conceptRelationRepo = relationRepo
	imp.conceptRelationSrc = relationSrc
	return imp
}

// WithDefinitionConcepts configures the definitions-side concept import
// dependencies. When either is nil, ImportAll skips the definition-concept
// phase (notes.concept_key cache survives independently — see migration
// 012 — but the canonical declaration won't be persisted).
func (imp *Importer) WithDefinitionConcepts(
	conceptRepo notebook.DefinitionConceptRepository,
	conceptSrc DefinitionConceptSource,
) *Importer {
	imp.definitionConceptRepo = conceptRepo
	imp.definitionConceptSrc = conceptSrc
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
		existing.ConceptKey = src.ConceptKey
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

// DefinitionsBookSink writes per-book definitions YAML output. Today only
// the concepts: block is round-tripped through this sink — note bodies
// for definitions books are written via the regular NoteSink path under
// books/. WriteConcepts groups by (bookID -> session_title -> concepts)
// so each session file holds the concepts declared inside it.
type DefinitionsBookSink interface {
	WriteConcepts(perBook map[string]map[string][]notebook.DefinitionConcept) error
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

// ExportDefinitionConceptsResult tracks counts for definition-concept
// export — the number of concepts written across all definitions books.
type ExportDefinitionConceptsResult struct {
	ConceptsExported int
}

// Exporter reads DB data and writes to YAML files.
type Exporter struct {
	noteRepo       notebook.NoteRepository
	learningRepo   learning.LearningRepository
	dictionaryRepo dictionary.DictionaryRepository
	noteSink       NoteSink
	learningSink   LearningSink
	dictionarySink DictionarySink
	// Optional definition-concept side. When either is nil,
	// ExportAll skips the definition-concept phase.
	definitionConceptRepo notebook.DefinitionConceptRepository
	definitionsBookSink   DefinitionsBookSink
	writer                io.Writer
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

// WithDefinitionConcepts configures the definitions-concept export
// dependencies. When either is nil, ExportAll skips the concepts phase
// (note bodies still round-trip via NoteSink/books/).
func (exp *Exporter) WithDefinitionConcepts(
	conceptRepo notebook.DefinitionConceptRepository,
	sink DefinitionsBookSink,
) *Exporter {
	exp.definitionConceptRepo = conceptRepo
	exp.definitionsBookSink = sink
	return exp
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

// ExportDefinitionConcepts reads definition_concepts + members from the
// database and writes them back to the definitions YAML output via the
// configured sink. Each concept is grouped by (notebook_id, session_title)
// so the resulting YAML mirrors how the importer originally read them.
// Returns nil when no sink is configured.
func (exp *Exporter) ExportDefinitionConcepts(ctx context.Context) (*ExportDefinitionConceptsResult, error) {
	if exp.definitionConceptRepo == nil || exp.definitionsBookSink == nil {
		return &ExportDefinitionConceptsResult{}, nil
	}

	concepts, err := exp.definitionConceptRepo.ListAllDefinitionConcepts(ctx)
	if err != nil {
		return nil, fmt.Errorf("load definition concepts: %w", err)
	}
	ids := make([]int64, 0, len(concepts))
	for _, c := range concepts {
		ids = append(ids, c.ID)
	}
	members, err := exp.definitionConceptRepo.ListDefinitionConceptMembersByConceptIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load definition concept members: %w", err)
	}

	// Group members by concept_id so each concept can attach its
	// expression list. Within a concept the members are sorted for
	// deterministic output.
	membersByConcept := make(map[int64][]notebook.DefinitionConceptMemberRecord, len(concepts))
	for _, m := range members {
		membersByConcept[m.ConceptID] = append(membersByConcept[m.ConceptID], m)
	}

	// Build the per-book per-session concept slices. The session title
	// of a concept is taken from the first member's session (members
	// declared in the same session group together; concepts declared
	// across multiple sessions emit under the first-seen session,
	// which is enough for the round-trip since meaning is shared).
	perBook := make(map[string]map[string][]notebook.DefinitionConcept)
	conceptCount := 0
	for _, c := range concepts {
		ms := membersByConcept[c.ID]
		sort.Slice(ms, func(i, j int) bool {
			if ms[i].SessionTitle != ms[j].SessionTitle {
				return ms[i].SessionTitle < ms[j].SessionTitle
			}
			return ms[i].Expression < ms[j].Expression
		})
		sessionTitle := ""
		if len(ms) > 0 {
			sessionTitle = ms[0].SessionTitle
		}
		expressions := make([]string, 0, len(ms))
		for _, m := range ms {
			expressions = append(expressions, m.Expression)
		}
		if perBook[c.NotebookID] == nil {
			perBook[c.NotebookID] = make(map[string][]notebook.DefinitionConcept)
		}
		perBook[c.NotebookID][sessionTitle] = append(perBook[c.NotebookID][sessionTitle], notebook.DefinitionConcept{
			Head:         c.Head,
			Meaning:      c.Meaning,
			Expressions:  expressions,
			SessionTitle: sessionTitle,
		})
		conceptCount++
	}

	if err := exp.definitionsBookSink.WriteConcepts(perBook); err != nil {
		return nil, fmt.Errorf("write definition concepts: %w", err)
	}

	_, _ = fmt.Fprintf(exp.writer, "  Exported %d definition concepts\n", conceptCount)
	return &ExportDefinitionConceptsResult{
		ConceptsExported: conceptCount,
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
		key := etymologyOriginCompositeKey(row.NotebookID, row.SessionTitle, row.Sense, row.Origin, row.Language)
		existingByKey[key] = row
	}

	var toInsert []*notebook.EtymologyOriginRecord
	for i := range source {
		src := source[i]
		key := etymologyOriginCompositeKey(src.NotebookID, src.SessionTitle, src.Sense, src.Origin, src.Language)
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

// ImportEtymologyOriginForms reads form entries declared on each origin in
// YAML and reconciles them against etymology_origin_forms. YAML is the
// source of truth: rows the YAML still declares are inserted or updated,
// rows the YAML no longer claims (scoped to origins the YAML still owns)
// are deleted. Origins that aren't in the DB cause their forms to be
// silently skipped — origins must be imported first.
func (imp *Importer) ImportEtymologyOriginForms(ctx context.Context, opts ImportOptions, result *ImportEtymologyResult) error {
	if imp.etymologyOriginFormRepo == nil || imp.etymologyOriginFormSrc == nil || imp.etymologyOriginRepo == nil {
		return nil
	}

	sourceForms, err := imp.etymologyOriginFormSrc.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("read source etymology origin forms: %w", err)
	}

	allOrigins, err := imp.etymologyOriginRepo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("load origins for form binding: %w", err)
	}
	originIDByKey := make(map[string]int64, len(allOrigins))
	for _, o := range allOrigins {
		originIDByKey[etymologyOriginCompositeKey(o.NotebookID, o.SessionTitle, o.Sense, o.Origin, o.Language)] = o.ID
	}

	existing, err := imp.etymologyOriginFormRepo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("load existing etymology_origin_forms: %w", err)
	}
	existingByKey := make(map[string]*notebook.EtymologyOriginFormRecord, len(existing))
	for i := range existing {
		row := &existing[i]
		existingByKey[etymologyOriginFormCompositeKey(row.OriginID, row.Role, row.Form)] = row
	}

	// Track which origin IDs the YAML claims forms for. The reconcile pass
	// only deletes rows that belong to those origins — origins missing from
	// the YAML are handled by ImportEtymologyOrigins' own reconcile.
	claimedOrigins := make(map[int64]bool)
	claimedKeys := make(map[string]bool)

	var toInsert []*notebook.EtymologyOriginFormRecord
	var toUpdate []*notebook.EtymologyOriginFormRecord
	for _, src := range sourceForms {
		originID, ok := originIDByKey[etymologyOriginCompositeKey(src.NotebookID, src.SessionTitle, src.Sense, src.Origin, src.Language)]
		if !ok {
			result.FormsSkipped++
			continue
		}
		claimedOrigins[originID] = true
		key := etymologyOriginFormCompositeKey(originID, src.Role, src.Form)
		claimedKeys[key] = true
		if row, ok := existingByKey[key]; ok {
			if row.Note != src.Note || row.SortOrder != src.SortOrder {
				row.Note = src.Note
				row.SortOrder = src.SortOrder
				toUpdate = append(toUpdate, row)
				result.FormsUpdated++
			} else {
				result.FormsSkipped++
			}
			continue
		}
		rec := &notebook.EtymologyOriginFormRecord{
			OriginID:  originID,
			Form:      src.Form,
			Role:      src.Role,
			Note:      src.Note,
			SortOrder: src.SortOrder,
		}
		toInsert = append(toInsert, rec)
		// Track in the map so subsequent passes (note_origin_parts
		// FromForm binding) can resolve the form to its ID after BatchCreate.
		existingByKey[key] = rec
		result.FormsNew++
	}

	if !opts.DryRun && len(toInsert) > 0 {
		if err := imp.etymologyOriginFormRepo.BatchCreate(ctx, toInsert); err != nil {
			return fmt.Errorf("batch create etymology_origin_forms: %w", err)
		}
	}
	if !opts.DryRun && len(toUpdate) > 0 {
		if err := imp.etymologyOriginFormRepo.BatchUpdate(ctx, toUpdate); err != nil {
			return fmt.Errorf("batch update etymology_origin_forms: %w", err)
		}
	}

	// Reconcile: drop rows on YAML-claimed origins that the YAML no longer
	// declares. Rows on origins the YAML doesn't claim at all are deleted
	// transitively by ImportEtymologyOrigins' future reconcile (today it
	// doesn't delete origins either, so unclaimed origin forms persist).
	var staleIDs []int64
	for _, row := range existing {
		if !claimedOrigins[row.OriginID] {
			continue
		}
		key := etymologyOriginFormCompositeKey(row.OriginID, row.Role, row.Form)
		if claimedKeys[key] {
			continue
		}
		staleIDs = append(staleIDs, row.ID)
	}
	if len(staleIDs) > 0 {
		_, _ = fmt.Fprintf(imp.writer, "  [RECONCILE] removing %d stale etymology_origin_form(s)\n", len(staleIDs))
		if !opts.DryRun {
			if err := imp.etymologyOriginFormRepo.BatchDelete(ctx, staleIDs); err != nil {
				return fmt.Errorf("delete stale etymology_origin_forms: %w", err)
			}
		}
		result.FormsDeleted = len(staleIDs)
	}
	return nil
}

func etymologyOriginFormCompositeKey(originID int64, role, form string) string {
	return fmt.Sprintf("%d\x00%s\x00%s", originID, role, form)
}

// ImportNoteOriginParts reads each definition's origin_parts list and
// inserts the corresponding rows into note_origin_parts. A part is bound
// when (notebook_id, session_title, origin, language) resolves to an
// etymology_origins row AND (usage, entry) resolves to a notes row.
// Unbound parts are silently skipped (printed at debug level).
//
// When a definition's origin_part has a from_form, the part is also bound
// to the matching etymology_origin_forms row (case-insensitive on the
// form string). A missing form is logged as a warning but doesn't block
// the insert — origin_form_id stays NULL.
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
	// Three lookup tiers. Most specific to least:
	//   1. (notebook, session, sense, origin, language) — exact match when
	//      a ref pins a sense (e.g. osteopath's pathos with sense:disease).
	//   2. (notebook, session, origin, language) — fallback for refs that
	//      don't pin a sense. Picks the first matching origin row.
	//   3. (origin, language) — cross-notebook fallback for refs that
	//      omit notebook/session entirely.
	originIDByFullKey := make(map[string]int64, len(allOrigins))
	originIDByBaseKey := make(map[string]int64, len(allOrigins))
	originIDByNameOnly := make(map[string]int64, len(allOrigins))
	for _, o := range allOrigins {
		full := etymologyOriginCompositeKey(o.NotebookID, o.SessionTitle, o.Sense, strings.ToLower(o.Origin), o.Language)
		originIDByFullKey[full] = o.ID
		base := etymologyOriginBaseKey(o.NotebookID, o.SessionTitle, strings.ToLower(o.Origin), o.Language)
		if _, ok := originIDByBaseKey[base]; !ok {
			originIDByBaseKey[base] = o.ID
		}
		nameOnly := strings.Join([]string{strings.ToLower(o.Origin), o.Language}, "\x00")
		if _, ok := originIDByNameOnly[nameOnly]; !ok {
			originIDByNameOnly[nameOnly] = o.ID
		}
	}

	// Form lookup keyed by (origin_id, lower(form)). Multiple roles can
	// share a form string; we pick the first row and let the validator
	// flag ambiguity at the YAML layer.
	formIDByKey := make(map[string]int64)
	if imp.etymologyOriginFormRepo != nil {
		allForms, err := imp.etymologyOriginFormRepo.FindAll(ctx)
		if err != nil {
			return fmt.Errorf("load forms for origin-part binding: %w", err)
		}
		for _, f := range allForms {
			key := fmt.Sprintf("%d\x00%s", f.OriginID, strings.ToLower(f.Form))
			if _, ok := formIDByKey[key]; !ok {
				formIDByKey[key] = f.ID
			}
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
			full := etymologyOriginCompositeKey(d.NotebookID, d.SessionTitle, ref.Sense, strings.ToLower(ref.Origin), ref.Language)
			originID, ok := originIDByFullKey[full]
			if !ok {
				base := etymologyOriginBaseKey(d.NotebookID, d.SessionTitle, strings.ToLower(ref.Origin), ref.Language)
				originID, ok = originIDByBaseKey[base]
				if ok && ref.Sense == "" {
					_, _ = fmt.Fprintf(imp.writer, "  [WARN] note_origin_part for %q references %q (%s) without `sense:` — falling back to first declared sense; pin a sense if this origin is multi-sense in session %q\n",
						d.Expression, ref.Origin, ref.Language, d.SessionTitle)
				}
			}
			if !ok {
				nameOnly := strings.Join([]string{strings.ToLower(ref.Origin), ref.Language}, "\x00")
				originID, ok = originIDByNameOnly[nameOnly]
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

			var formID *int64
			if ref.FromForm != "" {
				if id, ok := formIDByKey[fmt.Sprintf("%d\x00%s", originID, strings.ToLower(ref.FromForm))]; ok {
					id := id
					formID = &id
				} else {
					_, _ = fmt.Fprintf(imp.writer, "  [WARN] note_origin_part for %q has from_form %q with no matching form on origin %q\n",
						d.Expression, ref.FromForm, ref.Origin)
				}
			}

			toInsert = append(toInsert, &notebook.NoteOriginPartRecord{
				NoteID:       noteID,
				OriginID:     originID,
				OriginFormID: formID,
				SortOrder:    sortOrder,
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

// bookConceptMemberKey identifies one (concept, origin) membership claim
// from the YAML, keyed by concept_key + origin_id so a member that survives
// origin resolution can be looked up by the eventual concept_id.
type bookConceptMemberKey struct {
	ConceptKey string
	OriginID   int64
}

// bookConceptData aggregates the YAML-side state for one book during
// concept ingestion: which concepts the book declares (and the first
// declaration of each, which wins for meaning/note), and the set of
// resolved (concept, origin) memberships with their declaring session.
type bookConceptData struct {
	declOrder []string
	firstDecl map[string]notebook.SemanticConceptForImport
	memberSet map[bookConceptMemberKey]string
}

// ImportSemanticConcepts reconciles semantic_concepts and
// semantic_concept_members against the YAML source. Within a book the first
// declaration of a concept_key sets meaning/note; subsequent declarations
// contribute only members. Members whose (origin, language) doesn't resolve
// to an etymology_origins row in the declaring session emit a warning and
// are skipped. Reconcile drops concepts/members the YAML no longer claims.
func (imp *Importer) ImportSemanticConcepts(ctx context.Context, opts ImportOptions, result *ImportEtymologyResult) error {
	if imp.semanticConceptRepo == nil || imp.semanticConceptSrc == nil || imp.etymologyOriginRepo == nil {
		return nil
	}

	source, err := imp.semanticConceptSrc.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("read source semantic concepts: %w", err)
	}

	allOrigins, err := imp.etymologyOriginRepo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("load origins for concept member binding: %w", err)
	}
	// Concept members are matched sense-agnostically: a member references
	// (origin, language) within a session; if the origin is multi-sense in
	// that session, the first sense wins. (Same trade-off as un-pinned
	// origin_parts above. Could be refined by adding `sense:` to
	// ConceptMember later.)
	originIDByBase := make(map[string]int64, len(allOrigins))
	for _, o := range allOrigins {
		base := etymologyOriginBaseKey(o.NotebookID, o.SessionTitle, o.Origin, o.Language)
		if _, ok := originIDByBase[base]; !ok {
			originIDByBase[base] = o.ID
		}
	}

	byBook := make(map[string]*bookConceptData)
	for _, src := range source {
		b, ok := byBook[src.NotebookID]
		if !ok {
			b = &bookConceptData{
				firstDecl: make(map[string]notebook.SemanticConceptForImport),
				memberSet: make(map[bookConceptMemberKey]string),
			}
			byBook[src.NotebookID] = b
		}
		first, seen := b.firstDecl[src.Key]
		if !seen {
			b.firstDecl[src.Key] = src
			b.declOrder = append(b.declOrder, src.Key)
		} else if first.Meaning != src.Meaning || first.Note != src.Note {
			_, _ = fmt.Fprintf(imp.writer,
				"  [WARN] concept %q in book %q redeclared with different meaning/note (keeping first)\n",
				src.Key, src.NotebookID)
		}
		// Resolve each member against the declaring session's origins.
		for _, m := range src.Members {
			key := etymologyOriginBaseKey(src.NotebookID, src.SessionTitle, m.Origin, m.Language)
			originID, ok := originIDByBase[key]
			if !ok {
				_, _ = fmt.Fprintf(imp.writer,
					"  [WARN] concept %q member %q (%s) not found in session %q of book %q\n",
					src.Key, m.Origin, m.Language, src.SessionTitle, src.NotebookID)
				result.ConceptMembersSkipped++
				continue
			}
			b.memberSet[bookConceptMemberKey{ConceptKey: src.Key, OriginID: originID}] = src.SessionTitle
		}
	}

	for notebookID, b := range byBook {
		if err := imp.reconcileBookConcepts(ctx, opts, notebookID, b, result); err != nil {
			return err
		}
	}
	return nil
}

// reconcileBookConcepts upserts concepts and members for one book. Returns
// after writing each phase so callers see partial progress in --dry-run.
func (imp *Importer) reconcileBookConcepts(
	ctx context.Context,
	opts ImportOptions,
	notebookID string,
	b *bookConceptData,
	result *ImportEtymologyResult,
) error {
	existingConcepts, err := imp.semanticConceptRepo.ListSemanticConceptsByNotebook(ctx, notebookID)
	if err != nil {
		return fmt.Errorf("load existing concepts for %s: %w", notebookID, err)
	}
	conceptIDByKey := make(map[string]int64, len(existingConcepts))
	conceptRecByKey := make(map[string]*SemanticConceptRecordAlias, len(existingConcepts))
	for i := range existingConcepts {
		row := &existingConcepts[i]
		conceptIDByKey[row.ConceptKey] = row.ID
		conceptRecByKey[row.ConceptKey] = (*SemanticConceptRecordAlias)(row)
	}

	var newConcepts []*notebook.SemanticConceptRecord
	var updateConcepts []*notebook.SemanticConceptRecord
	claimedConceptKeys := make(map[string]bool, len(b.declOrder))
	for _, key := range b.declOrder {
		decl := b.firstDecl[key]
		claimedConceptKeys[key] = true
		if existing, ok := conceptRecByKey[key]; ok {
			if existing.Meaning != decl.Meaning || existing.Note != decl.Note {
				existing.Meaning = decl.Meaning
				existing.Note = decl.Note
				updateConcepts = append(updateConcepts, (*notebook.SemanticConceptRecord)(existing))
				result.ConceptsUpdated++
			}
			continue
		}
		rec := &notebook.SemanticConceptRecord{
			NotebookID: notebookID,
			ConceptKey: key,
			Meaning:    decl.Meaning,
			Note:       decl.Note,
		}
		newConcepts = append(newConcepts, rec)
		result.ConceptsNew++
	}

	if !opts.DryRun && len(newConcepts) > 0 {
		if err := imp.semanticConceptRepo.BatchCreateConcepts(ctx, newConcepts); err != nil {
			return fmt.Errorf("batch create semantic_concepts: %w", err)
		}
		for _, rec := range newConcepts {
			conceptIDByKey[rec.ConceptKey] = rec.ID
		}
	}
	if !opts.DryRun && len(updateConcepts) > 0 {
		if err := imp.semanticConceptRepo.BatchUpdateConcepts(ctx, updateConcepts); err != nil {
			return fmt.Errorf("batch update semantic_concepts: %w", err)
		}
	}

	// Member reconcile: existing members keyed by (concept_id, origin_id);
	// drop those the YAML no longer claims, insert the rest.
	ids := make([]int64, 0, len(conceptIDByKey))
	for _, id := range conceptIDByKey {
		ids = append(ids, id)
	}
	existingMembers, err := imp.semanticConceptRepo.ListSemanticConceptMembersByConceptIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("load existing concept members for %s: %w", notebookID, err)
	}
	type memberDBKey struct {
		ConceptID int64
		OriginID  int64
	}
	existingMemberID := make(map[memberDBKey]int64, len(existingMembers))
	for _, m := range existingMembers {
		existingMemberID[memberDBKey{m.ConceptID, m.OriginID}] = m.ID
	}

	var newMembers []*notebook.SemanticConceptMemberRecord
	claimedMemberKeys := make(map[memberDBKey]bool, len(b.memberSet))
	for memberKey, sessionTitle := range b.memberSet {
		conceptID, ok := conceptIDByKey[memberKey.ConceptKey]
		if !ok {
			// First-pass dry-run path: concept not yet in DB.
			continue
		}
		dbKey := memberDBKey{conceptID, memberKey.OriginID}
		claimedMemberKeys[dbKey] = true
		if _, exists := existingMemberID[dbKey]; exists {
			continue
		}
		newMembers = append(newMembers, &notebook.SemanticConceptMemberRecord{
			ConceptID:    conceptID,
			OriginID:     memberKey.OriginID,
			SessionTitle: sessionTitle,
		})
		result.ConceptMembersNew++
	}
	if !opts.DryRun && len(newMembers) > 0 {
		if err := imp.semanticConceptRepo.BatchCreateMembers(ctx, newMembers); err != nil {
			return fmt.Errorf("batch create semantic_concept_members: %w", err)
		}
	}

	// Reconcile: delete DB-only members on still-claimed concepts.
	var staleMemberIDs []int64
	for dbKey, id := range existingMemberID {
		if !claimedMemberKeys[dbKey] && claimedConceptKeyForID(conceptIDByKey, dbKey.ConceptID, claimedConceptKeys) {
			staleMemberIDs = append(staleMemberIDs, id)
		}
	}
	if len(staleMemberIDs) > 0 {
		_, _ = fmt.Fprintf(imp.writer, "  [RECONCILE] removing %d stale semantic_concept_member(s) in %s\n", len(staleMemberIDs), notebookID)
		if !opts.DryRun {
			if err := imp.semanticConceptRepo.BatchDeleteMembers(ctx, staleMemberIDs); err != nil {
				return fmt.Errorf("delete stale semantic_concept_members: %w", err)
			}
		}
		result.ConceptMembersDeleted += len(staleMemberIDs)
	}

	// Reconcile: delete entire concepts the YAML no longer claims (cascade
	// drops their members and relations).
	var staleConceptIDs []int64
	for _, row := range existingConcepts {
		if !claimedConceptKeys[row.ConceptKey] {
			staleConceptIDs = append(staleConceptIDs, row.ID)
		}
	}
	if len(staleConceptIDs) > 0 {
		_, _ = fmt.Fprintf(imp.writer, "  [RECONCILE] removing %d stale semantic_concept(s) in %s\n", len(staleConceptIDs), notebookID)
		if !opts.DryRun {
			if err := imp.semanticConceptRepo.BatchDeleteConcepts(ctx, staleConceptIDs); err != nil {
				return fmt.Errorf("delete stale semantic_concepts: %w", err)
			}
		}
		result.ConceptsDeleted += len(staleConceptIDs)
	}
	return nil
}

// SemanticConceptRecordAlias is a local alias used during reconcile so we
// can mutate the existing DB row in place without exposing internal types.
type SemanticConceptRecordAlias notebook.SemanticConceptRecord

// claimedConceptKeyForID returns true when the concept ID still belongs to
// a YAML-claimed concept. Used so we don't try to delete members of a
// concept that's about to be cascade-deleted anyway.
func claimedConceptKeyForID(conceptIDByKey map[string]int64, conceptID int64, claimedKeys map[string]bool) bool {
	for key, id := range conceptIDByKey {
		if id == conceptID {
			return claimedKeys[key]
		}
	}
	return false
}

// ImportConceptRelations reconciles concept_relations against the YAML source.
// Symmetric relations expand to two directed rows (A->B and B->A) with
// is_directed=false; directed relations write one row with is_directed=true.
// Endpoints that don't resolve to a concept in the book are skipped.
// Reconcile drops rows the YAML no longer claims.
func (imp *Importer) ImportConceptRelations(ctx context.Context, opts ImportOptions, result *ImportEtymologyResult) error {
	if imp.conceptRelationRepo == nil || imp.conceptRelationSrc == nil || imp.semanticConceptRepo == nil {
		return nil
	}

	source, err := imp.conceptRelationSrc.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("read source concept relations: %w", err)
	}

	type relationDBKey struct {
		NotebookID    string
		Type          string
		FromConceptID int64
		ToConceptID   int64
	}

	// Group source by book so we can resolve keys against per-book concepts.
	byBook := make(map[string][]notebook.ConceptRelationForImport)
	for _, src := range source {
		byBook[src.NotebookID] = append(byBook[src.NotebookID], src)
	}

	for notebookID, rels := range byBook {
		concepts, err := imp.semanticConceptRepo.ListSemanticConceptsByNotebook(ctx, notebookID)
		if err != nil {
			return fmt.Errorf("load concepts for relation binding in %s: %w", notebookID, err)
		}
		idByKey := make(map[string]int64, len(concepts))
		for _, c := range concepts {
			idByKey[c.ConceptKey] = c.ID
		}

		claimed := make(map[relationDBKey]bool)
		var toCreate []*notebook.ConceptRelationRecord
		for _, r := range rels {
			fromID, fromOK := idByKey[r.FromKey]
			toID, toOK := idByKey[r.ToKey]
			if !fromOK || !toOK {
				continue
			}
			pairs := []relationDBKey{{notebookID, r.Type, fromID, toID}}
			if !r.IsDirected {
				pairs = append(pairs, relationDBKey{notebookID, r.Type, toID, fromID})
			}
			for _, p := range pairs {
				if claimed[p] {
					continue
				}
				claimed[p] = true
				toCreate = append(toCreate, &notebook.ConceptRelationRecord{
					NotebookID:    p.NotebookID,
					Type:          p.Type,
					FromConceptID: p.FromConceptID,
					ToConceptID:   p.ToConceptID,
					IsDirected:    r.IsDirected,
				})
			}
		}

		existing, err := imp.conceptRelationRepo.ListConceptRelationsByNotebook(ctx, notebookID)
		if err != nil {
			return fmt.Errorf("load existing relations for %s: %w", notebookID, err)
		}
		existingByKey := make(map[relationDBKey]int64, len(existing))
		for _, row := range existing {
			existingByKey[relationDBKey{row.NotebookID, row.Type, row.FromConceptID, row.ToConceptID}] = row.ID
		}

		var newRelations []*notebook.ConceptRelationRecord
		for _, rec := range toCreate {
			k := relationDBKey{rec.NotebookID, rec.Type, rec.FromConceptID, rec.ToConceptID}
			if _, ok := existingByKey[k]; ok {
				continue
			}
			newRelations = append(newRelations, rec)
			result.RelationsNew++
		}
		if !opts.DryRun && len(newRelations) > 0 {
			if err := imp.conceptRelationRepo.BatchCreateRelations(ctx, newRelations); err != nil {
				return fmt.Errorf("batch create concept_relations: %w", err)
			}
		}

		var staleIDs []int64
		for k, id := range existingByKey {
			if !claimed[k] {
				staleIDs = append(staleIDs, id)
			}
		}
		if len(staleIDs) > 0 {
			_, _ = fmt.Fprintf(imp.writer, "  [RECONCILE] removing %d stale concept_relation(s) in %s\n", len(staleIDs), notebookID)
			if !opts.DryRun {
				if err := imp.conceptRelationRepo.BatchDeleteRelations(ctx, staleIDs); err != nil {
					return fmt.Errorf("delete stale concept_relations: %w", err)
				}
			}
			result.RelationsDeleted += len(staleIDs)
		}
	}
	return nil
}

// ImportDefinitionConcepts reconciles definition_concepts and
// definition_concept_members against the YAML source. Within a book the
// first declaration of a head sets meaning; subsequent declarations
// contribute only members. Reconcile drops concepts/members the YAML no
// longer claims. This mirrors ImportSemanticConcepts but for
// definitions-side concepts: members are raw expression strings rather
// than (origin, language) tuples, so there's no origin-resolution step.
func (imp *Importer) ImportDefinitionConcepts(ctx context.Context, opts ImportOptions, result *ImportEtymologyResult) error {
	if imp.definitionConceptRepo == nil || imp.definitionConceptSrc == nil {
		return nil
	}

	source, err := imp.definitionConceptSrc.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("read source definition concepts: %w", err)
	}

	// Per-book aggregation: first declaration wins on meaning; members
	// accumulate with their declaring session title.
	type bookData struct {
		declOrder []string
		firstDecl map[string]notebook.DefinitionConceptForImport
		memberSet map[string]map[string]string // head -> expression -> sessionTitle
	}
	byBook := make(map[string]*bookData)
	for _, src := range source {
		b, ok := byBook[src.NotebookID]
		if !ok {
			b = &bookData{
				firstDecl: make(map[string]notebook.DefinitionConceptForImport),
				memberSet: make(map[string]map[string]string),
			}
			byBook[src.NotebookID] = b
		}
		first, seen := b.firstDecl[src.Head]
		if !seen {
			b.firstDecl[src.Head] = src
			b.declOrder = append(b.declOrder, src.Head)
		} else if first.Meaning != src.Meaning {
			_, _ = fmt.Fprintf(imp.writer,
				"  [WARN] definition concept %q in book %q redeclared with different meaning (keeping first)\n",
				src.Head, src.NotebookID)
		}
		if b.memberSet[src.Head] == nil {
			b.memberSet[src.Head] = make(map[string]string)
		}
		for _, m := range src.Members {
			if _, exists := b.memberSet[src.Head][m]; !exists {
				b.memberSet[src.Head][m] = src.SessionTitle
			}
		}
	}

	for notebookID, b := range byBook {
		if err := imp.reconcileBookDefinitionConcepts(ctx, opts, notebookID, b.declOrder, b.firstDecl, b.memberSet, result); err != nil {
			return err
		}
	}
	return nil
}

// reconcileBookDefinitionConcepts upserts definition concepts and members
// for one book. Mirrors reconcileBookConcepts on the semantic side: new
// concepts insert, changed meaning updates, vanished members/concepts are
// deleted via ON DELETE CASCADE-aware batch deletes.
func (imp *Importer) reconcileBookDefinitionConcepts(
	ctx context.Context,
	opts ImportOptions,
	notebookID string,
	declOrder []string,
	firstDecl map[string]notebook.DefinitionConceptForImport,
	memberSet map[string]map[string]string,
	result *ImportEtymologyResult,
) error {
	existingConcepts, err := imp.definitionConceptRepo.ListDefinitionConceptsByNotebook(ctx, notebookID)
	if err != nil {
		return fmt.Errorf("load existing definition concepts for %s: %w", notebookID, err)
	}
	conceptIDByHead := make(map[string]int64, len(existingConcepts))
	conceptByHead := make(map[string]*notebook.DefinitionConceptRecord, len(existingConcepts))
	for i := range existingConcepts {
		row := &existingConcepts[i]
		conceptIDByHead[row.Head] = row.ID
		conceptByHead[row.Head] = row
	}

	var newConcepts []*notebook.DefinitionConceptRecord
	var updateConcepts []*notebook.DefinitionConceptRecord
	claimedHeads := make(map[string]bool, len(declOrder))
	for _, head := range declOrder {
		decl := firstDecl[head]
		claimedHeads[head] = true
		if existing, ok := conceptByHead[head]; ok {
			if existing.Meaning != decl.Meaning {
				existing.Meaning = decl.Meaning
				updateConcepts = append(updateConcepts, existing)
				result.DefinitionConceptsUpdated++
			}
			continue
		}
		rec := &notebook.DefinitionConceptRecord{
			NotebookID: notebookID,
			Head:       head,
			Meaning:    decl.Meaning,
		}
		newConcepts = append(newConcepts, rec)
		result.DefinitionConceptsNew++
	}

	if !opts.DryRun && len(newConcepts) > 0 {
		if err := imp.definitionConceptRepo.BatchCreateConcepts(ctx, newConcepts); err != nil {
			return fmt.Errorf("batch create definition_concepts: %w", err)
		}
		for _, rec := range newConcepts {
			conceptIDByHead[rec.Head] = rec.ID
		}
	}
	if !opts.DryRun && len(updateConcepts) > 0 {
		if err := imp.definitionConceptRepo.BatchUpdateConcepts(ctx, updateConcepts); err != nil {
			return fmt.Errorf("batch update definition_concepts: %w", err)
		}
	}

	// Member reconcile: load existing members and compare against the
	// YAML-claimed (concept_id, expression) set. New tuples insert;
	// surviving tuples no-op; vanished tuples delete.
	ids := make([]int64, 0, len(conceptIDByHead))
	for _, id := range conceptIDByHead {
		ids = append(ids, id)
	}
	existingMembers, err := imp.definitionConceptRepo.ListDefinitionConceptMembersByConceptIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("load existing definition concept members for %s: %w", notebookID, err)
	}
	type memberDBKey struct {
		ConceptID  int64
		Expression string
	}
	existingMemberID := make(map[memberDBKey]int64, len(existingMembers))
	for _, m := range existingMembers {
		existingMemberID[memberDBKey{m.ConceptID, m.Expression}] = m.ID
	}

	var newMembers []*notebook.DefinitionConceptMemberRecord
	claimedMemberKeys := make(map[memberDBKey]bool)
	for head, exprs := range memberSet {
		conceptID, ok := conceptIDByHead[head]
		if !ok {
			// First-pass dry-run: concept not yet in DB.
			continue
		}
		for expr, sessionTitle := range exprs {
			dbKey := memberDBKey{conceptID, expr}
			claimedMemberKeys[dbKey] = true
			if _, exists := existingMemberID[dbKey]; exists {
				continue
			}
			newMembers = append(newMembers, &notebook.DefinitionConceptMemberRecord{
				ConceptID:    conceptID,
				Expression:   expr,
				SessionTitle: sessionTitle,
			})
			result.DefinitionConceptMembersNew++
		}
	}
	if !opts.DryRun && len(newMembers) > 0 {
		if err := imp.definitionConceptRepo.BatchCreateMembers(ctx, newMembers); err != nil {
			return fmt.Errorf("batch create definition_concept_members: %w", err)
		}
	}

	// Reconcile: drop members on still-claimed concepts that the YAML no
	// longer declares. Members on concepts the YAML drops entirely will be
	// cascade-deleted with the concept itself below.
	var staleMemberIDs []int64
	for dbKey, id := range existingMemberID {
		if claimedMemberKeys[dbKey] {
			continue
		}
		if claimedDefinitionConceptHeadForID(conceptIDByHead, dbKey.ConceptID, claimedHeads) {
			staleMemberIDs = append(staleMemberIDs, id)
		}
	}
	if len(staleMemberIDs) > 0 {
		_, _ = fmt.Fprintf(imp.writer, "  [RECONCILE] removing %d stale definition_concept_member(s) in %s\n", len(staleMemberIDs), notebookID)
		if !opts.DryRun {
			if err := imp.definitionConceptRepo.BatchDeleteMembers(ctx, staleMemberIDs); err != nil {
				return fmt.Errorf("delete stale definition_concept_members: %w", err)
			}
		}
		result.DefinitionConceptMembersDeleted += len(staleMemberIDs)
	}

	// Reconcile: delete concepts the YAML no longer claims (cascade drops
	// dependent members).
	var staleConceptIDs []int64
	for _, row := range existingConcepts {
		if !claimedHeads[row.Head] {
			staleConceptIDs = append(staleConceptIDs, row.ID)
		}
	}
	if len(staleConceptIDs) > 0 {
		_, _ = fmt.Fprintf(imp.writer, "  [RECONCILE] removing %d stale definition_concept(s) in %s\n", len(staleConceptIDs), notebookID)
		if !opts.DryRun {
			if err := imp.definitionConceptRepo.BatchDeleteConcepts(ctx, staleConceptIDs); err != nil {
				return fmt.Errorf("delete stale definition_concepts: %w", err)
			}
		}
		result.DefinitionConceptsDeleted += len(staleConceptIDs)
	}
	return nil
}

// claimedDefinitionConceptHeadForID returns true when the concept ID still
// belongs to a YAML-claimed head — used so the member-reconcile loop
// doesn't try to delete members of a concept that's about to be
// cascade-deleted anyway.
func claimedDefinitionConceptHeadForID(conceptIDByHead map[string]int64, conceptID int64, claimedHeads map[string]bool) bool {
	for head, id := range conceptIDByHead {
		if id == conceptID {
			return claimedHeads[head]
		}
	}
	return false
}

// etymologyOriginCompositeKey builds the lookup key used across ingestion
// to match a YAML origin reference to its DB row. Sense is included since
// migration 011: multi-sense origins are distinct rows keyed by
// (notebook, session, origin, language, sense), and a reference that
// doesn't supply sense falls back via etymologyOriginBaseKey.
func etymologyOriginCompositeKey(notebookID, sessionTitle, sense, origin, language string) string {
	return strings.Join([]string{notebookID, sessionTitle, sense, origin, language}, "\x00")
}

// etymologyOriginBaseKey is the sense-agnostic fallback key. When an origin
// reference (in definitions or in a concept members list) doesn't specify
// a sense, the importer first tries the sense-specific key with sense=""
// and then this base key, picking the first record that matches the
// (notebook, session, origin, language) tuple. The base key is used to
// keep un-backfilled references resolving against multi-sense origins.
func etymologyOriginBaseKey(notebookID, sessionTitle, origin, language string) string {
	return strings.Join([]string{notebookID, sessionTitle, origin, language}, "\x00")
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
	if err := imp.ImportEtymologyOriginForms(ctx, opts, etymResult); err != nil {
		return nil, fmt.Errorf("import etymology origin forms: %w", err)
	}
	if err := imp.ImportNoteOriginParts(ctx, opts, etymResult); err != nil {
		return nil, fmt.Errorf("import note origin parts: %w", err)
	}
	if err := imp.ImportSemanticConcepts(ctx, opts, etymResult); err != nil {
		return nil, fmt.Errorf("import semantic concepts: %w", err)
	}
	if err := imp.ImportConceptRelations(ctx, opts, etymResult); err != nil {
		return nil, fmt.Errorf("import concept relations: %w", err)
	}
	if err := imp.ImportDefinitionConcepts(ctx, opts, etymResult); err != nil {
		return nil, fmt.Errorf("import definition concepts: %w", err)
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
	Notes              *ExportNotesResult
	Learning           *ExportLearningLogsResult
	Dictionary         *ExportDictionaryResult
	DefinitionConcepts *ExportDefinitionConceptsResult
}

// ExportAll runs all export steps: notes, learning logs, dictionary, and
// definition concepts (when configured).
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

	defConceptResult, err := exp.ExportDefinitionConcepts(ctx)
	if err != nil {
		return nil, fmt.Errorf("export definition concepts: %w", err)
	}

	return &ExportAllResult{
		Notes:              noteResult,
		Learning:           learningResult,
		Dictionary:         dictResult,
		DefinitionConcepts: defConceptResult,
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
