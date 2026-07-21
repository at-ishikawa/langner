package learning

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// YAMLLearningRepository reads learning history from YAML files and writes
// learning logs as LearningHistory YAML files.
type YAMLLearningRepository struct {
	directory  string
	outputDir  string
	calculator notebook.IntervalCalculator
}

// NewYAMLLearningRepository creates a new YAMLLearningRepository for reading.
func NewYAMLLearningRepository(directory string, calculator notebook.IntervalCalculator) *YAMLLearningRepository {
	if calculator == nil {
		calculator = &notebook.SM2Calculator{}
	}
	return &YAMLLearningRepository{directory: directory, calculator: calculator}
}

// NewYAMLLearningRepositoryWriter creates a new YAMLLearningRepository for writing.
func NewYAMLLearningRepositoryWriter(outputDir string) *YAMLLearningRepository {
	return &YAMLLearningRepository{outputDir: outputDir}
}

// FindByNotebookID reads learning YAML files, filters by notebook ID, and
// returns flattened expressions. Handles flashcard (top-level) vs story (scene) types.
func (r *YAMLLearningRepository) FindByNotebookID(notebookID string) ([]notebook.LearningHistoryExpression, error) {
	histories, err := notebook.NewLearningHistories(r.directory)
	if err != nil {
		return nil, fmt.Errorf("load learning histories: %w", err)
	}

	var result []notebook.LearningHistoryExpression
	for _, fileHistories := range histories {
		for _, h := range fileHistories {
			if h.Metadata.NotebookID != notebookID {
				continue
			}
			if h.Metadata.Type == "flashcard" {
				result = append(result, h.Expressions...)
				continue
			}
			for _, scene := range h.Scenes {
				result = append(result, scene.Expressions...)
			}
		}
	}

	return result, nil
}

// WriteAll converts learning logs to LearningHistory YAML files grouped by notebook.
func (r *YAMLLearningRepository) WriteAll(notes []notebook.NoteRecord, logs []LearningLog) error {
	noteByID := make(map[int64]*notebook.NoteRecord, len(notes))
	for i := range notes {
		noteByID[notes[i].ID] = &notes[i]
	}

	// Group logs by (noteID, sourceNotebookID)
	type noteNotebook struct {
		noteID     int64
		notebookID string
	}
	logsByNoteNotebook := make(map[noteNotebook][]LearningLog)
	for _, log := range logs {
		key := noteNotebook{log.NoteID, log.SourceNotebookID}
		logsByNoteNotebook[key] = append(logsByNoteNotebook[key], log)
	}

	// Group notes by NotebookID and detect notebook type per notebook
	type notebookInfo struct {
		noteIDs     []int64
		isFlashcard bool
	}
	notebookMap := make(map[string]*notebookInfo)
	var notebookIDs []string

	for _, note := range notes {
		for _, nn := range note.NotebookNotes {
			info := notebookMap[nn.NotebookID]
			if info == nil {
				info = &notebookInfo{}
				notebookMap[nn.NotebookID] = info
				notebookIDs = append(notebookIDs, nn.NotebookID)
			}
			info.noteIDs = append(info.noteIDs, note.ID)
			if nn.NotebookType == "flashcard" {
				info.isFlashcard = true
			}
		}
	}

	sort.Strings(notebookIDs)

	for _, nbID := range notebookIDs {
		info := notebookMap[nbID]

		// Deduplicate note IDs preserving order
		seen := make(map[int64]bool)
		var uniqueNoteIDs []int64
		for _, id := range info.noteIDs {
			if !seen[id] {
				seen[id] = true
				uniqueNoteIDs = append(uniqueNoteIDs, id)
			}
		}

		// Build per-note log map filtered by source notebook
		filteredLogs := make(map[int64][]LearningLog)
		for _, id := range uniqueNoteIDs {
			if logs := logsByNoteNotebook[noteNotebook{id, nbID}]; len(logs) > 0 {
				filteredLogs[id] = logs
			}
		}

		var histories []notebook.LearningHistory
		if info.isFlashcard {
			histories = r.buildFlashcardHistories(nbID, uniqueNoteIDs, noteByID, filteredLogs)
		} else {
			histories = r.buildStoryHistories(nbID, uniqueNoteIDs, noteByID, filteredLogs)
		}

		if len(histories) == 0 {
			continue
		}

		dir := filepath.Join(r.outputDir, "learning_notes")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}

		filePath := filepath.Join(dir, nbID+".yml")
		if err := notebook.WriteYamlFile(filePath, histories); err != nil {
			return fmt.Errorf("write learning history %s: %w", nbID, err)
		}
	}

	return nil
}

func (r *YAMLLearningRepository) buildFlashcardHistories(
	nbID string,
	noteIDs []int64,
	noteByID map[int64]*notebook.NoteRecord,
	logsByNoteID map[int64][]LearningLog,
) []notebook.LearningHistory {
	// Group notes by Group (title) for this notebook
	groupOrder := make(map[string]int)
	groupEntries := make(map[string][]int64)
	var counter int

	for _, noteID := range noteIDs {
		note := noteByID[noteID]
		if note == nil {
			continue
		}
		for _, nn := range note.NotebookNotes {
			if nn.NotebookID != nbID {
				continue
			}
			group := nn.Group
			if _, ok := groupOrder[group]; !ok {
				groupOrder[group] = counter
				counter++
			}
			groupEntries[group] = append(groupEntries[group], noteID)
		}
	}

	// Sort groups by insertion order
	groups := make([]string, 0, len(groupEntries))
	for g := range groupEntries {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groupOrder[groups[i]] < groupOrder[groups[j]]
	})

	// Track notes whose logs have been written to avoid duplicating across groups
	logsClaimed := make(map[int64]bool)

	var histories []notebook.LearningHistory
	for _, group := range groups {
		// Deduplicate note IDs within group
		seen := make(map[int64]bool)
		var expressions []notebook.LearningHistoryExpression
		for _, noteID := range groupEntries[group] {
			if seen[noteID] {
				continue
			}
			seen[noteID] = true
			note := noteByID[noteID]
			if note == nil {
				continue
			}
			var logs []LearningLog
			if !logsClaimed[noteID] {
				logs = logsByNoteID[noteID]
				logsClaimed[noteID] = true
			}
			expr := buildExpression(note.Entry, logs)
			expressions = append(expressions, expr)
		}

		histories = append(histories, notebook.LearningHistory{
			Metadata: notebook.LearningHistoryMetadata{
				NotebookID: nbID,
				Title:      group,
				Type:       "flashcard",
			},
			Expressions: expressions,
		})
	}

	return histories
}

func (r *YAMLLearningRepository) buildStoryHistories(
	nbID string,
	noteIDs []int64,
	noteByID map[int64]*notebook.NoteRecord,
	logsByNoteID map[int64][]LearningLog,
) []notebook.LearningHistory {
	// Group notes by Group (event) then Subgroup (scene)
	type eventScene struct {
		event string
		scene string
	}

	eventOrder := make(map[string]int)
	sceneOrder := make(map[eventScene]int)
	sceneEntries := make(map[eventScene][]int64)
	var eventCounter, sceneCounter int
	eventScenes := make(map[string][]string)

	for _, noteID := range noteIDs {
		note := noteByID[noteID]
		if note == nil {
			continue
		}
		for _, nn := range note.NotebookNotes {
			if nn.NotebookID != nbID {
				continue
			}
			event := nn.Group
			scene := nn.Subgroup

			if _, ok := eventOrder[event]; !ok {
				eventOrder[event] = eventCounter
				eventCounter++
			}

			es := eventScene{event, scene}
			if _, ok := sceneOrder[es]; !ok {
				sceneOrder[es] = sceneCounter
				sceneCounter++
				eventScenes[event] = append(eventScenes[event], scene)
			}

			sceneEntries[es] = append(sceneEntries[es], noteID)
		}
	}

	// Sort events by insertion order
	events := make([]string, 0, len(eventOrder))
	for e := range eventOrder {
		events = append(events, e)
	}
	sort.Slice(events, func(i, j int) bool {
		return eventOrder[events[i]] < eventOrder[events[j]]
	})

	// Track notes whose logs have been written to avoid duplicating across scenes
	logsClaimed := make(map[int64]bool)

	var histories []notebook.LearningHistory
	for _, event := range events {
		scenes := eventScenes[event]
		// Sort scenes by insertion order
		sort.Slice(scenes, func(i, j int) bool {
			return sceneOrder[eventScene{event, scenes[i]}] < sceneOrder[eventScene{event, scenes[j]}]
		})

		var learningScenes []notebook.LearningScene
		for _, scene := range scenes {
			es := eventScene{event, scene}
			// Deduplicate note IDs within scene
			seen := make(map[int64]bool)
			var expressions []notebook.LearningHistoryExpression
			for _, noteID := range sceneEntries[es] {
				if seen[noteID] {
					continue
				}
				seen[noteID] = true
				note := noteByID[noteID]
				if note == nil {
					continue
				}
				var logs []LearningLog
				if !logsClaimed[noteID] {
					logs = logsByNoteID[noteID]
					logsClaimed[noteID] = true
				}
				expr := buildExpression(note.Entry, logs)
				expressions = append(expressions, expr)
			}

			learningScenes = append(learningScenes, notebook.LearningScene{
				Metadata: notebook.LearningSceneMetadata{
					Title: scene,
				},
				Expressions: expressions,
			})
		}

		histories = append(histories, notebook.LearningHistory{
			Metadata: notebook.LearningHistoryMetadata{
				NotebookID: nbID,
				Title:      event,
			},
			Scenes: learningScenes,
		})
	}

	return histories
}

func buildExpression(
	entry string,
	logs []LearningLog,
) notebook.LearningHistoryExpression {
	// Route each log into the matching slot on LearningHistoryExpression.
	// The original implementation lumped everything that wasn't reverse
	// into learned_logs, which destroyed quiz_type information on
	// round-trip: a word with etymology_breakdown_logs / etymology_
	// assembly_logs in the source YAML came back with all of them merged
	// into learned_logs (e.g. gauche with 1 source learned log + 7
	// etymology logs exported as 8 learned logs). Match each YAML slot
	// to the quiz_type values that get stored there at import time.
	var learnedLogs, reverseLogs, breakdownLogs, assemblyLogs []LearningLog
	for _, log := range logs {
		switch log.QuizType {
		case string(notebook.QuizTypeReverse):
			reverseLogs = append(reverseLogs, log)
		case string(notebook.QuizTypeEtymologyStandard): // stored as "etymology_breakdown"
			breakdownLogs = append(breakdownLogs, log)
		case string(notebook.QuizTypeEtymologyReverse): // stored as "etymology_assembly"
			assemblyLogs = append(assemblyLogs, log)
		case string(notebook.QuizTypeEtymologyFreeform):
			// Freeform is one event that exercises both directions of
			// recall, and AddRecordWithQualityForEtymology mirrors it
			// into both YAML slots on the write side. Export has to
			// mirror the same way so the round-trip matches — landing
			// it in learned_logs (the previous default behavior)
			// silently inflated LearnedLogCount by every freeform
			// event, which validate-db surfaced as a +2 delta on
			// short polysemous roots like "alter".
			breakdownLogs = append(breakdownLogs, log)
			assemblyLogs = append(assemblyLogs, log)
		default:
			// notebook (standard) and freeform (vocabulary, not
			// etymology) land in learned_logs in the YAML convention.
			learnedLogs = append(learnedLogs, log)
		}
	}

	// Sort each slot descending by LearnedAt (newest first).
	sortDescByLearnedAt := func(s []LearningLog) {
		sort.Slice(s, func(i, j int) bool {
			return s[i].LearnedAt.After(s[j].LearnedAt)
		})
	}
	sortDescByLearnedAt(learnedLogs)
	sortDescByLearnedAt(reverseLogs)
	sortDescByLearnedAt(breakdownLogs)
	sortDescByLearnedAt(assemblyLogs)

	return notebook.LearningHistoryExpression{
		Expression:             entry,
		LearnedLogs:            convertToRecords(learnedLogs),
		ReverseLogs:            convertToRecords(reverseLogs),
		EtymologyBreakdownLogs: convertToRecords(breakdownLogs),
		EtymologyAssemblyLogs:  convertToRecords(assemblyLogs),
	}
}

func convertToRecords(logs []LearningLog) []notebook.LearningRecord {
	if len(logs) == 0 {
		return nil
	}
	records := make([]notebook.LearningRecord, len(logs))
	for i, log := range logs {
		records[i] = notebook.LearningRecord{
			Status:         notebook.LearnedStatus(log.Status),
			LearnedAt:      notebook.NewDate(log.LearnedAt),
			Quality:        log.Quality,
			ResponseTimeMs: int64(log.ResponseTimeMs),
			QuizType:       log.QuizType,
			IntervalDays:   log.IntervalDays,
		}
	}
	return records
}

func (r *YAMLLearningRepository) Create(_ context.Context, log *LearningLog) error {
	dir := log.LearningNotesDir
	if dir == "" {
		dir = r.directory
	}
	learningHistories, err := notebook.NewLearningHistories(dir)
	if err != nil {
		return fmt.Errorf("load learning histories: %w", err)
	}
	updater := notebook.NewLearningHistoryUpdater(learningHistories[log.NotebookName], r.calculator)
	quizType := notebook.QuizType(log.QuizType)
	// Freeform requires active recall of both the word and its meaning, so a
	// correct answer means the user can actually use the word, not just
	// recognise it. Pass isKnownWord=false to get "usable" status. Standard
	// and reverse quizzes pass isKnownWord=true for "understood".
	isKnownWord := quizType != notebook.QuizTypeFreeform
	if quizType == notebook.QuizTypeReverse {
		updater.UpdateOrCreateExpressionWithQualityForReverse(log.NotebookName, log.StoryTitle, log.SceneTitle, log.Expression, log.OriginalExpression, log.SenseID, log.IsCorrect, true, log.Quality, int64(log.ResponseTimeMs), notebook.QuizTypeReverse)
	} else {
		updater.UpdateOrCreateExpressionWithQuality(log.NotebookName, log.StoryTitle, log.SceneTitle, log.Expression, log.OriginalExpression, log.SenseID, log.IsCorrect, isKnownWord, log.Quality, int64(log.ResponseTimeMs), quizType)
		// Freeform quiz tests word recall (similar to reverse), so also update reverse logs
		if quizType == notebook.QuizTypeFreeform {
			updater.UpdateOrCreateExpressionWithQualityForReverse(log.NotebookName, log.StoryTitle, log.SceneTitle, log.Expression, log.OriginalExpression, log.SenseID, log.IsCorrect, isKnownWord, log.Quality, int64(log.ResponseTimeMs), notebook.QuizTypeFreeform)
		}
	}
	notePath := filepath.Join(dir, log.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return fmt.Errorf("write learning history for %q: %w", log.NotebookName, err)
	}
	return nil
}

// UpdateLog rewrites the YAML log entry identified by the lookup keys.
// Two modes:
//
//   - in.MirrorValues != nil (Undo path): restore the log to the
//     captured pre-override snapshot exactly. Delegates to
//     updater.UndoOverrideLog so status/quality/interval_days come
//     back byte-identical.
//   - otherwise (fresh Override path): apply in.MarkCorrect via
//     updater.OverrideLog, which recomputes interval_days through
//     the calculator and, for freeform variants, syncs both halves
//     of the paired log lists.
func (r *YAMLLearningRepository) UpdateLog(_ context.Context, in UpdateLogInput) (UpdateLogResult, error) {
	dir := r.directory
	histories, err := notebook.NewLearningHistories(dir)
	if err != nil {
		return UpdateLogResult{}, fmt.Errorf("load learning histories: %w", err)
	}
	updater := notebook.NewLearningHistoryUpdater(histories[in.NotebookName], r.calculator)

	var res notebook.OverrideLogResult
	if in.MirrorValues != nil {
		undo := updater.UndoOverrideLog(notebook.UndoOverrideLogInput{
			ID:                   in.ID,
			Expression:           in.Expression,
			OriginalExpression:   in.OriginalExpression,
			QuizType:             notebook.QuizType(in.QuizType),
			LearnedAt:            formatLearnedAt(in.LearnedAt),
			OriginalQuality:      in.MirrorValues.Quality,
			OriginalStatus:       in.MirrorValues.Status,
			OriginalIntervalDays: in.MirrorValues.IntervalDays,
		})
		if !undo.Found {
			return UpdateLogResult{}, nil
		}
		// UndoOverrideLog doesn't carry the pre-restore values (there's
		// no callsite that needs them), so we surface the caller-
		// supplied MirrorValues as the "new" state and leave Original*
		// zeroed for this branch.
		res = notebook.OverrideLogResult{
			NewNextReviewDate: undo.NewNextReviewDate,
			Found:             true,
		}
	} else {
		res = updater.OverrideLog(notebook.OverrideLogInput{
			ID:                 in.ID,
			Expression:         in.Expression,
			OriginalExpression: in.OriginalExpression,
			QuizType:           notebook.QuizType(in.QuizType),
			LearnedAt:          formatLearnedAt(in.LearnedAt),
			MarkCorrect:        in.MarkCorrect,
		})
		if !res.Found {
			return UpdateLogResult{}, nil
		}
	}

	notePath := filepath.Join(dir, in.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return UpdateLogResult{}, fmt.Errorf("write learning history for %q: %w", in.NotebookName, err)
	}
	// Read back the just-written entry so the caller can mirror the
	// exact bytes onto the secondary store. The updater's result
	// already carries originals; we re-resolve the expression to read
	// out the new status/quality/interval that landed on disk.
	expr := updater.FindExpressionByID(in.ID, in.Expression, in.OriginalExpression)
	var newStatus string
	var newQuality, newInterval int
	if expr != nil {
		logs := expr.GetLogsForQuizType(notebook.QuizType(in.QuizType))
		for _, l := range logs {
			if l.LearnedAt.Format(time.RFC3339) == formatLearnedAt(in.LearnedAt) ||
				l.LearnedAt.Format("2006-01-02") == formatLearnedAt(in.LearnedAt) {
				newStatus = string(l.Status)
				newQuality = l.Quality
				newInterval = l.IntervalDays
				break
			}
		}
	}
	return UpdateLogResult{
		OriginalQuality:      res.OriginalQuality,
		OriginalStatus:       res.OriginalStatus,
		OriginalIntervalDays: res.OriginalIntervalDays,
		NewQuality:           newQuality,
		NewStatus:            newStatus,
		NewIntervalDays:      newInterval,
		NewNextReviewDate:    res.NewNextReviewDate,
		Found:                true,
	}, nil
}

// formatLearnedAt picks the same string format the updater's
// indexLogByLearnedAt accepts. Prefers RFC3339 so micro-second-precise
// timestamps round-trip; falls back to YYYY-MM-DD for older logs the
// importer stored without time-of-day.
func formatLearnedAt(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0 {
		return t.Format("2006-01-02")
	}
	return t.Format(time.RFC3339)
}

func (r *YAMLLearningRepository) FindAll(_ context.Context) ([]LearningLog, error) {
	return nil, fmt.Errorf("FindAll is not supported for YAML learning repository")
}

func (r *YAMLLearningRepository) BatchCreate(_ context.Context, _ []*LearningLog) error {
	return fmt.Errorf("BatchCreate is not supported for YAML learning repository")
}

// BatchDelete is a no-op on the YAML side. The reconcile pass only ever
// targets the DB; YAML is the source of truth.
func (r *YAMLLearningRepository) BatchDelete(_ context.Context, _ []int64) error {
	return fmt.Errorf("BatchDelete is not supported for YAML learning repository")
}
