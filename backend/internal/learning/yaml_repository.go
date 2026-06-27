package learning

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

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
//
// origins carries every etymology_origins row in the DB. Origin-typed
// logs in `logs` (NoteID == 0, OriginID != 0) get attributed back to
// their origin's notebook and emitted as `type: origin` expressions in
// the same YAML file as that notebook's vocab logs. Without this path
// origin logs survive sync-db's import but vanish from the round-trip,
// which is the source of every `source=N, exported=0` row the
// validator reported for word-power-made-easy origins.
func (r *YAMLLearningRepository) WriteAll(notes []notebook.NoteRecord, logs []LearningLog, origins []notebook.EtymologyOriginRecord) error {
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
	originLogsByID := make(map[int64][]LearningLog)
	for _, log := range logs {
		if log.OriginID > 0 {
			originLogsByID[log.OriginID] = append(originLogsByID[log.OriginID], log)
			continue
		}
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

	// Group origins by their owning notebook so each notebook's file
	// picks up its origin expressions even if no vocab note in the
	// notebook has touched a log.
	originsByNotebook := make(map[string][]notebook.EtymologyOriginRecord)
	for _, o := range origins {
		originsByNotebook[o.NotebookID] = append(originsByNotebook[o.NotebookID], o)
		if _, ok := notebookMap[o.NotebookID]; !ok {
			notebookMap[o.NotebookID] = &notebookInfo{}
			notebookIDs = append(notebookIDs, o.NotebookID)
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

		histories = appendOriginHistories(histories, nbID, originsByNotebook[nbID], originLogsByID)

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

// appendOriginHistories adds origin-typed LearningHistoryExpression
// rows into the existing per-notebook histories. Each origin lands
// under a LearningHistory block whose Title matches the origin's
// session_title, with a scene of the same title — the convention the
// DBHistoryStore reconstructor uses on the read side, so the
// round-trip stays symmetric. Validate counts logs per expression
// name regardless of scene structure, so a different scene title in
// the original source YAML doesn't trip the comparison.
func appendOriginHistories(
	histories []notebook.LearningHistory,
	nbID string,
	origins []notebook.EtymologyOriginRecord,
	originLogsByID map[int64][]LearningLog,
) []notebook.LearningHistory {
	if len(origins) == 0 {
		return histories
	}

	// Sort origins for deterministic output: by session_title, origin, sense.
	sort.Slice(origins, func(i, j int) bool {
		if origins[i].SessionTitle != origins[j].SessionTitle {
			return origins[i].SessionTitle < origins[j].SessionTitle
		}
		if origins[i].Origin != origins[j].Origin {
			return origins[i].Origin < origins[j].Origin
		}
		return origins[i].Sense < origins[j].Sense
	})

	historyIdxByTitle := make(map[string]int, len(histories))
	for i := range histories {
		historyIdxByTitle[histories[i].Metadata.Title] = i
	}

	for _, o := range origins {
		expr := buildOriginExpression(o.Origin, originLogsByID[o.ID])
		title := o.SessionTitle
		idx, ok := historyIdxByTitle[title]
		if !ok {
			histories = append(histories, notebook.LearningHistory{
				Metadata: notebook.LearningHistoryMetadata{NotebookID: nbID, Title: title},
				Scenes: []notebook.LearningScene{{
					Metadata:    notebook.LearningSceneMetadata{Title: title},
					Expressions: []notebook.LearningHistoryExpression{expr},
				}},
			})
			historyIdxByTitle[title] = len(histories) - 1
			continue
		}
		// Reuse the existing block; find a scene with matching title or
		// append a new one. Scene-level placement isn't observable by
		// the validate diff (counts are per-expression, not per-scene)
		// but keeps the YAML structure tidy.
		sceneFound := false
		for j := range histories[idx].Scenes {
			if histories[idx].Scenes[j].Metadata.Title == title {
				histories[idx].Scenes[j].Expressions = append(histories[idx].Scenes[j].Expressions, expr)
				sceneFound = true
				break
			}
		}
		if !sceneFound {
			histories[idx].Scenes = append(histories[idx].Scenes, notebook.LearningScene{
				Metadata:    notebook.LearningSceneMetadata{Title: title},
				Expressions: []notebook.LearningHistoryExpression{expr},
			})
		}
	}
	return histories
}

// buildOriginExpression mirrors buildExpression but tags the result as
// type: origin. Logs flow into the same slots based on quiz_type.
func buildOriginExpression(origin string, logs []LearningLog) notebook.LearningHistoryExpression {
	expr := buildExpression(origin, logs)
	expr.Type = notebook.LearningExpressionTypeOrigin
	return expr
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
		default:
			// notebook (standard), freeform, etymology_freeform — all
			// land in learned_logs in the YAML convention.
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
	if dir == "" { dir = r.directory }
	learningHistories, err := notebook.NewLearningHistories(dir)
	if err != nil { return fmt.Errorf("load learning histories: %w", err) }
	updater := notebook.NewLearningHistoryUpdater(learningHistories[log.NotebookName], r.calculator)
	quizType := notebook.QuizType(log.QuizType)
	// Freeform requires active recall of both the word and its meaning, so a
	// correct answer means the user can actually use the word, not just
	// recognise it. Pass isKnownWord=false to get "usable" status. Standard
	// and reverse quizzes pass isKnownWord=true for "understood".
	isKnownWord := quizType != notebook.QuizTypeFreeform
	if quizType == notebook.QuizTypeReverse {
		updater.UpdateOrCreateExpressionWithQualityForReverse(log.NotebookName, log.StoryTitle, log.SceneTitle, log.Expression, log.OriginalExpression, log.IsCorrect, true, log.Quality, int64(log.ResponseTimeMs), notebook.QuizTypeReverse)
	} else {
		updater.UpdateOrCreateExpressionWithQuality(log.NotebookName, log.StoryTitle, log.SceneTitle, log.Expression, log.OriginalExpression, log.IsCorrect, isKnownWord, log.Quality, int64(log.ResponseTimeMs), quizType)
		// Freeform quiz tests word recall (similar to reverse), so also update reverse logs
		if quizType == notebook.QuizTypeFreeform {
			updater.UpdateOrCreateExpressionWithQualityForReverse(log.NotebookName, log.StoryTitle, log.SceneTitle, log.Expression, log.OriginalExpression, log.IsCorrect, isKnownWord, log.Quality, int64(log.ResponseTimeMs), notebook.QuizTypeFreeform)
		}
	}
	notePath := filepath.Join(dir, log.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil { return fmt.Errorf("write learning history for %q: %w", log.NotebookName, err) }
	return nil
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
