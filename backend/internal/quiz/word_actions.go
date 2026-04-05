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

// SkipWord sets a skip interval on a word so it won't appear in quizzes until the given date.
// If skipUntil is empty, the word is skipped for a very long interval (10 years).
// quizType determines which log set to update (forward or reverse).
func (s *Service) SkipWord(info CardInfo, skipUntil string, quizType notebook.QuizType) error {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return fmt.Errorf("failed to load learning histories: %w", err)
	}

	var intervalDays int
	if skipUntil != "" {
		parsed, err := time.Parse("2006-01-02", skipUntil)
		if err != nil {
			return fmt.Errorf("invalid skip_until date %q: %w", skipUntil, err)
		}
		intervalDays = int(time.Until(parsed).Hours()/24) + 1
		if intervalDays < 1 {
			intervalDays = 1
		}
	} else {
		intervalDays = 3650 // ~10 years
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[info.NotebookName], s.calculator)

	switch quizType {
	case notebook.QuizTypeReverse:
		if !s.setSkipInterval(updater, info, intervalDays, quizType) {
			updater.UpdateOrCreateExpressionWithQualityForReverse(
				info.NotebookName,
				info.StoryTitle,
				info.SceneTitle,
				info.Expression,
				info.Expression,
				true,
				true,
				5,
				0,
				notebook.QuizTypeReverse,
			)
			s.setSkipInterval(updater, info, intervalDays, quizType)
		}
	case notebook.QuizTypeEtymologyStandard, notebook.QuizTypeEtymologyReverse:
		if !s.setSkipInterval(updater, info, intervalDays, quizType) {
			updater.UpdateOrCreateExpressionWithQualityForEtymology(
				info.NotebookName,
				info.StoryTitle,
				info.SceneTitle,
				info.Expression,
				info.Expression,
				true,
				true,
				5,
				0,
				quizType,
			)
			s.setSkipInterval(updater, info, intervalDays, quizType)
		}
	default:
		if !s.setSkipInterval(updater, info, intervalDays, quizType) {
			updater.UpdateOrCreateExpressionWithQuality(
				info.NotebookName,
				info.StoryTitle,
				info.SceneTitle,
				info.Expression,
				info.Expression,
				true,
				true,
				5,
				0,
				quizType,
			)
			s.setSkipInterval(updater, info, intervalDays, quizType)
		}
	}

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, info.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return fmt.Errorf("failed to save learning history for %q: %w", info.NotebookName, err)
	}
	return nil
}

// setSkipInterval finds the expression in the learning history and overrides
// the interval of the most recent log entry. Returns false if not found.
func (s *Service) setSkipInterval(updater *notebook.LearningHistoryUpdater, info CardInfo, intervalDays int, quizType notebook.QuizType) bool {
	for _, h := range updater.GetHistory() {
		if h.Metadata.Title != info.StoryTitle {
			continue
		}

		if len(h.Expressions) > 0 {
			for _, expr := range h.Expressions {
				if !strings.EqualFold(expr.Expression, info.Expression) {
					continue
				}
				logs := expr.GetLogsForQuizType(quizType)
				if len(logs) > 0 {
					logs[0].IntervalDays = intervalDays
					return true
				}
				return false
			}
			continue
		}

		for _, scene := range h.Scenes {
			if scene.Metadata.Title != info.SceneTitle {
				continue
			}
			for _, expr := range scene.Expressions {
				if !strings.EqualFold(expr.Expression, info.Expression) {
					continue
				}
				logs := expr.GetLogsForQuizType(quizType)
				if len(logs) > 0 {
					logs[0].IntervalDays = intervalDays
					return true
				}
				return false
			}
		}
	}
	return false
}

// ResumeWord resets the skip interval on a word so it appears in quizzes again.
// This sets the interval to 0, making the word immediately due for review.
func (s *Service) ResumeWord(info CardInfo, quizType notebook.QuizType) error {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return fmt.Errorf("failed to load learning histories: %w", err)
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[info.NotebookName], s.calculator)
	s.setSkipInterval(updater, info, 0, quizType)

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
		log.Status = notebook.LearnedStatusUnderstood
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
