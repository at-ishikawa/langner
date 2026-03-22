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
	var learnedLogs, reverseLogs []LearningLog
	for _, log := range logs {
		if log.QuizType == "reverse" {
			reverseLogs = append(reverseLogs, log)
		} else {
			learnedLogs = append(learnedLogs, log)
		}
	}

	// Sort both descending by LearnedAt (newest first)
	sort.Slice(learnedLogs, func(i, j int) bool {
		return learnedLogs[i].LearnedAt.After(learnedLogs[j].LearnedAt)
	})
	sort.Slice(reverseLogs, func(i, j int) bool {
		return reverseLogs[i].LearnedAt.After(reverseLogs[j].LearnedAt)
	})

	expr := notebook.LearningHistoryExpression{
		Expression: entry,
	}

	// Extract easiness factor from the latest log for each quiz type
	if len(learnedLogs) > 0 {
		expr.EasinessFactor = learnedLogs[0].EasinessFactor
	}
	if len(reverseLogs) > 0 {
		expr.ReverseEasinessFactor = reverseLogs[0].EasinessFactor
	}

	// Convert to LearningRecord
	expr.LearnedLogs = convertToRecords(learnedLogs)
	expr.ReverseLogs = convertToRecords(reverseLogs)

	return expr
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
	if quizType == notebook.QuizTypeReverse {
		updater.UpdateOrCreateExpressionWithQualityForReverse(log.NotebookName, log.StoryTitle, log.SceneTitle, log.Expression, log.OriginalExpression, log.IsCorrect, true, log.Quality, int64(log.ResponseTimeMs))
	} else {
		updater.UpdateOrCreateExpressionWithQuality(log.NotebookName, log.StoryTitle, log.SceneTitle, log.Expression, log.OriginalExpression, log.IsCorrect, true, log.Quality, int64(log.ResponseTimeMs), quizType)
		// Freeform quiz tests word recall (similar to reverse), so also update reverse logs
		if quizType == notebook.QuizTypeFreeform {
			updater.UpdateOrCreateExpressionWithQualityForReverse(log.NotebookName, log.StoryTitle, log.SceneTitle, log.Expression, log.OriginalExpression, log.IsCorrect, true, log.Quality, int64(log.ResponseTimeMs))
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
