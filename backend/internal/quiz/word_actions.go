package quiz

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// loadSingleLearningHistory reads only the YAML for the requested notebook
// instead of walking the entire learning_notes directory. The previous
// implementation called NewLearningHistories on every Skip/Resume RPC,
// re-parsing every notebook's YAML; toggling the "All" master in the UI
// fires 3 parallel RPCs, tripling the cost. Returns an empty slice if the
// notebook's history file doesn't exist yet (a freshly-imported word).
func loadSingleLearningHistory(dir, notebookName string) ([]notebook.LearningHistory, error) {
	path := filepath.Join(dir, notebookName+".yml")
	hist, err := notebook.ReadLearningHistoryFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return hist, err
}

// CardInfo holds the minimal information needed to identify a word
// in the learning history for skip/resume/override operations.
type CardInfo struct {
	NotebookName string
	StoryTitle   string
	SceneTitle   string
	Expression   string
}

// CardInfoFromCard converts a Card to CardInfo.
func CardInfoFromCard(card Card) CardInfo {
	return CardInfo{
		NotebookName: card.NotebookName,
		StoryTitle:   card.StoryTitle,
		SceneTitle:   card.SceneTitle,
		Expression:   card.Entry,
	}
}

// CardInfoFromFreeformCard converts a FreeformCard to CardInfo.
func CardInfoFromFreeformCard(card FreeformCard) CardInfo {
	return CardInfo{
		NotebookName: card.NotebookName,
		StoryTitle:   card.StoryTitle,
		SceneTitle:   card.SceneTitle,
		Expression:   card.Expression,
	}
}

// CardInfoFromReverseCard converts a ReverseCard to CardInfo.
func CardInfoFromReverseCard(card ReverseCard) CardInfo {
	return CardInfo{
		NotebookName: card.NotebookName,
		StoryTitle:   card.StoryTitle,
		SceneTitle:   card.SceneTitle,
		Expression:   card.Expression,
	}
}

// SkipWord excludes a word from each of the given quiz types in a single
// read-modify-write of the notebook's learning history YAML. Batching avoids
// the race that bit the per-type API: when the UI's "All" toggle issued one
// RPC per type concurrently, every handler read the same pre-update file
// and the last writer overwrote the others, dropping skips.
//
// The skip is recorded as a per-(expression, quiz_type) timestamp on
// SkippedAt; quiz card loaders filter against that field. The skipUntil
// parameter is accepted for RPC compatibility but is not honored —
// exclusion is permanent until ResumeWord clears the slot.
//
// If the expression has no learning history yet, SkipWord seeds an entry so
// the skip has somewhere to live, then writes the skips onto it.
//
// When the expression belongs to a definitions concept (see Card.ConceptHead),
// the skip propagates to every sibling member of the concept in the same
// notebook — that's the "skip union" guarantee the read-side collapse
// relies on. Until migration moves logs to the head, the simplest way to
// keep both reads-from-head and reads-from-members consistent is to write
// the skip on each member entry.
func (s *Service) SkipWord(info CardInfo, skipUntil string, quizTypes []notebook.QuizType) error {
	if len(quizTypes) == 0 {
		return fmt.Errorf("at least one quiz type is required to skip a word")
	}
	history, err := loadSingleLearningHistory(s.notebooksConfig.LearningNotesDirectory, info.NotebookName)
	if err != nil {
		return fmt.Errorf("failed to load learning history for %q: %w", info.NotebookName, err)
	}

	updater := notebook.NewLearningHistoryUpdater(history, s.calculator)

	expressions := s.conceptMembersOrSelf(info.NotebookName, info.Expression)

	// Create a learned-log-free stub for each member if the expression has
	// no history yet — SetSkippedAt needs an entry to attach to, but we
	// must not invent a fake "quality 5" review log just because the user
	// skipped the word.
	for _, expr := range expressions {
		updater.EnsureExpressionStubForSkip(info.NotebookName, info.StoryTitle, info.SceneTitle, expr)
	}

	skippedAt := time.Now().Format(time.RFC3339)
	for _, expr := range expressions {
		for _, qt := range quizTypes {
			if !updater.SetSkippedAt(expr, qt, skippedAt) {
				return fmt.Errorf("failed to record skip for expression %q (%s) in notebook %q", expr, qt, info.NotebookName)
			}
		}
	}

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, info.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return fmt.Errorf("failed to save learning history for %q: %w", info.NotebookName, err)
	}
	return nil
}

// conceptMembersOrSelf returns the list of concept-sibling expressions for
// expression in the given notebook, including expression itself. When the
// expression doesn't belong to any concept (or the reader fails to load),
// it returns [expression]. Used by SkipWord/ResumeWord to propagate skips
// across all members of the same concept.
func (s *Service) conceptMembersOrSelf(notebookName, expression string) []string {
	reader, err := s.newReader()
	if err != nil {
		return []string{expression}
	}
	index := buildConceptIndex(reader, notebookName)
	info, ok := index[expression]
	if !ok || info == nil {
		return []string{expression}
	}
	return append([]string(nil), info.Members...)
}

// ResumeWord clears skips for each of the given quiz types so the word
// reappears in those modes. Other quiz types' skips are left intact, so a
// word excluded from multiple modes only resumes the ones the caller lists.
// Batched into a single read-modify-write for the same race-free reason as
// SkipWord.
func (s *Service) ResumeWord(info CardInfo, quizTypes []notebook.QuizType) error {
	if len(quizTypes) == 0 {
		return fmt.Errorf("at least one quiz type is required to resume a word")
	}
	history, err := loadSingleLearningHistory(s.notebooksConfig.LearningNotesDirectory, info.NotebookName)
	if err != nil {
		return fmt.Errorf("failed to load learning history for %q: %w", info.NotebookName, err)
	}

	updater := notebook.NewLearningHistoryUpdater(history, s.calculator)
	for _, expr := range s.conceptMembersOrSelf(info.NotebookName, info.Expression) {
		for _, qt := range quizTypes {
			updater.ClearSkippedAt(expr, qt)
		}
	}

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, info.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return fmt.Errorf("failed to save learning history for %q: %w", info.NotebookName, err)
	}
	return nil
}

// OverrideAnswer toggles the correctness of the most recent answer for a word.
// Returns the new next review date string (YYYY-MM-DD format, empty if none).
func (s *Service) OverrideAnswer(info CardInfo, quizType notebook.QuizType) (string, error) {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return "", fmt.Errorf("failed to load learning histories: %w", err)
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[info.NotebookName], s.calculator)
	nextReview := s.toggleLastAnswer(updater, info, quizType)

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, info.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return "", fmt.Errorf("failed to save learning history for %q: %w", info.NotebookName, err)
	}
	return nextReview, nil
}

// UndoOverrideAnswer reverts the most recent answer override (toggles back).
// Returns the new next review date string (YYYY-MM-DD format, empty if none).
func (s *Service) UndoOverrideAnswer(info CardInfo, quizType notebook.QuizType) (string, error) {
	return s.OverrideAnswer(info, quizType)
}

// toggleLastAnswer toggles the correctness status and quality of the most recent
// learning log entry. Returns the new next review date.
func (s *Service) toggleLastAnswer(updater *notebook.LearningHistoryUpdater, info CardInfo, quizType notebook.QuizType) string {
	for _, h := range updater.GetHistory() {
		if h.Metadata.Title != info.StoryTitle {
			continue
		}

		if len(h.Expressions) > 0 {
			for ei, expr := range h.Expressions {
				if !strings.EqualFold(expr.Expression, info.Expression) {
					continue
				}
				return toggleLogs(&h.Expressions[ei], quizType, s.calculator)
			}
			continue
		}

		for _, scene := range h.Scenes {
			if scene.Metadata.Title != info.SceneTitle {
				continue
			}
			for ei, expr := range scene.Expressions {
				if !strings.EqualFold(expr.Expression, info.Expression) {
					continue
				}
				return toggleLogs(&scene.Expressions[ei], quizType, s.calculator)
			}
		}
	}
	return ""
}

func toggleLogs(expr *notebook.LearningHistoryExpression, quizType notebook.QuizType, calculator notebook.IntervalCalculator) string {
	logs := expr.GetLogsForQuizType(quizType)

	if len(logs) == 0 {
		return ""
	}

	log := &logs[0]
	if log.Status == notebook.LearnedStatusMisunderstood {
		if quizType == notebook.QuizTypeEtymologyFreeform || quizType == notebook.QuizTypeFreeform {
			log.Status = notebook.LearnedStatusCanBeUsed
		} else {
			log.Status = notebook.LearnedStatusUnderstood
		}
		log.Quality = 4
	} else {
		log.Status = notebook.LearnedStatusMisunderstood
		log.Quality = 1
	}

	// Replay the older-log chain with this flipped entry appended so the
	// recomputed interval matches what `validate --fix` would produce.
	var previousLogs []notebook.LearningRecord
	if len(logs) > 1 {
		previousLogs = logs[1:]
	}
	newInterval, _ := calculator.NextIntervalForWrite(previousLogs, *log)
	log.IntervalDays = newInterval

	expr.SetLogsForQuizType(quizType, logs)

	if newInterval > 0 {
		nextDate := log.LearnedAt.AddDate(0, 0, newInterval)
		return nextDate.Format("2006-01-02")
	}
	return ""
}
