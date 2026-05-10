package quiz

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/notebook"
)

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

// SkipWord excludes a word from the given quiz type. The skip is recorded
// as a per-(expression, quiz_type) timestamp on SkippedAt; quiz card loaders
// filter against that field. The skipUntil parameter is accepted for RPC
// compatibility but is not currently honored — exclusion is permanent until
// ResumeWord clears the slot.
//
// If the expression has no learning history yet, SkipWord seeds an entry so
// the skip has somewhere to live, then writes the skip onto it.
func (s *Service) SkipWord(info CardInfo, skipUntil string, quizType notebook.QuizType) error {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return fmt.Errorf("failed to load learning histories: %w", err)
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[info.NotebookName], s.calculator)

	if !findExpressionForSkip(updater, info) {
		// Seed an expression entry so SetSkippedAt has something to mark.
		// The seeded record uses quality 5 ("usable") to avoid polluting
		// the SR schedule — the skip itself is what excludes it from quizzes.
		seedSkippedExpression(updater, info, quizType)
	}

	skippedAt := time.Now().Format(time.RFC3339)
	if !updater.SetSkippedAt(info.Expression, quizType, skippedAt) {
		return fmt.Errorf("failed to record skip for expression %q in notebook %q", info.Expression, info.NotebookName)
	}

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, info.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return fmt.Errorf("failed to save learning history for %q: %w", info.NotebookName, err)
	}
	return nil
}

// findExpressionForSkip returns true if the expression already exists in any
// scene of the learning history for the given notebook + story title. The
// quiz_type isn't part of the lookup because the SkippedAt map lives on the
// expression record itself.
func findExpressionForSkip(updater *notebook.LearningHistoryUpdater, info CardInfo) bool {
	for _, h := range updater.GetHistory() {
		if h.Metadata.Title != info.StoryTitle {
			continue
		}
		for _, expr := range h.Expressions {
			if strings.EqualFold(expr.Expression, info.Expression) {
				return true
			}
		}
		for _, scene := range h.Scenes {
			for _, expr := range scene.Expressions {
				if strings.EqualFold(expr.Expression, info.Expression) {
					return true
				}
			}
		}
	}
	return false
}

// seedSkippedExpression creates a learning history entry so SetSkippedAt
// has somewhere to attach the skip. Uses quality 5 ("usable") with
// responseTimeMs=0 so the SR schedule treats this as a no-op review.
func seedSkippedExpression(updater *notebook.LearningHistoryUpdater, info CardInfo, quizType notebook.QuizType) {
	switch quizType {
	case notebook.QuizTypeReverse:
		updater.UpdateOrCreateExpressionWithQualityForReverse(
			info.NotebookName, info.StoryTitle, info.SceneTitle,
			info.Expression, info.Expression, true, true, 5, 0, quizType,
		)
	case notebook.QuizTypeEtymologyStandard, notebook.QuizTypeEtymologyReverse, notebook.QuizTypeEtymologyFreeform:
		updater.UpdateOrCreateExpressionWithQualityForEtymology(
			info.NotebookName, info.StoryTitle, info.SceneTitle,
			info.Expression, info.Expression, true, true, 5, 0, quizType,
		)
	default:
		updater.UpdateOrCreateExpressionWithQuality(
			info.NotebookName, info.StoryTitle, info.SceneTitle,
			info.Expression, info.Expression, true, true, 5, 0, quizType,
		)
	}
}

// ResumeWord clears the skip for the given quiz type so the word reappears
// in that quiz mode. Other quiz types' skips are left intact, so a word
// excluded from both `reverse` and `etymology_freeform` only resumes the
// type the caller specifies.
func (s *Service) ResumeWord(info CardInfo, quizType notebook.QuizType) error {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return fmt.Errorf("failed to load learning histories: %w", err)
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[info.NotebookName], s.calculator)
	updater.ClearSkippedAt(info.Expression, quizType)

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

	// Derive EF from logs before this entry and recalculate interval
	var previousLogs []notebook.LearningRecord
	if len(logs) > 1 {
		previousLogs = logs[1:]
	}
	derivedEF := calculator.DeriveEF(previousLogs)
	newInterval, _ := calculator.CalculateInterval(previousLogs, log.Quality, derivedEF)
	log.IntervalDays = newInterval

	expr.SetLogsForQuizType(quizType, logs)

	if newInterval > 0 {
		nextDate := log.LearnedAt.AddDate(0, 0, newInterval)
		return nextDate.Format("2006-01-02")
	}
	return ""
}
