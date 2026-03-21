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

	if quizType == notebook.QuizTypeReverse {
		if !s.setSkipInterval(updater, info, intervalDays, true) {
			// Create a new expression with a skip record for reverse
			updater.UpdateOrCreateExpressionWithQualityForReverse(
				info.NotebookName,
				info.StoryTitle,
				info.SceneTitle,
				info.Expression,
				info.Expression,
				true,  // mark as correct so it's not treated as misunderstood
				true,
				5,     // high quality so it gets a large interval
				0,
			)
			// Now set the interval on the newly created record
			s.setSkipInterval(updater, info, intervalDays, true)
		}
	} else {
		if !s.setSkipInterval(updater, info, intervalDays, false) {
			// Create a new expression with a skip record
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
			// Now set the interval on the newly created record
			s.setSkipInterval(updater, info, intervalDays, false)
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
func (s *Service) setSkipInterval(updater *notebook.LearningHistoryUpdater, info CardInfo, intervalDays int, isReverse bool) bool {
	for _, h := range updater.GetHistory() {
		if h.Metadata.Title != info.StoryTitle {
			continue
		}

		if h.Metadata.Type == "flashcard" || (info.StoryTitle == "flashcards" && info.SceneTitle == "") {
			for _, expr := range h.Expressions {
				if !strings.EqualFold(expr.Expression, info.Expression) {
					continue
				}
				logs := expr.LearnedLogs
				if isReverse {
					logs = expr.ReverseLogs
				}
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
				logs := expr.LearnedLogs
				if isReverse {
					logs = expr.ReverseLogs
				}
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
	isReverse := quizType == notebook.QuizTypeReverse
	s.setSkipInterval(updater, info, 0, isReverse)

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
	isReverse := quizType == notebook.QuizTypeReverse
	nextReview := s.toggleLastAnswer(updater, info, isReverse)

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, info.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return "", fmt.Errorf("failed to save learning history for %q: %w", info.NotebookName, err)
	}
	return nextReview, nil
}

// UndoOverrideAnswer reverts the most recent answer override (toggles back).
// Returns the new next review date string (YYYY-MM-DD format, empty if none).
func (s *Service) UndoOverrideAnswer(info CardInfo, quizType notebook.QuizType) (string, error) {
	// Undo is the same as override: toggle the correctness again.
	return s.OverrideAnswer(info, quizType)
}

// toggleLastAnswer toggles the correctness status and quality of the most recent
// learning log entry. Returns the new next review date.
func (s *Service) toggleLastAnswer(updater *notebook.LearningHistoryUpdater, info CardInfo, isReverse bool) string {
	for _, h := range updater.GetHistory() {
		if h.Metadata.Title != info.StoryTitle {
			continue
		}

		if h.Metadata.Type == "flashcard" || (info.StoryTitle == "flashcards" && info.SceneTitle == "") {
			for ei, expr := range h.Expressions {
				if !strings.EqualFold(expr.Expression, info.Expression) {
					continue
				}
				return toggleLogs(&h.Expressions[ei], isReverse, s.calculator)
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
				return toggleLogs(&scene.Expressions[ei], isReverse, s.calculator)
			}
		}
	}
	return ""
}

func toggleLogs(expr *notebook.LearningHistoryExpression, isReverse bool, calculator notebook.IntervalCalculator) string {
	var logs []notebook.LearningRecord
	if isReverse {
		logs = expr.ReverseLogs
	} else {
		logs = expr.LearnedLogs
	}

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

	// Recalculate interval using calculator
	ef := expr.EasinessFactor
	if isReverse {
		ef = expr.ReverseEasinessFactor
	}
	if ef == 0 {
		ef = notebook.DefaultEasinessFactor
	}
	var previousLogs []notebook.LearningRecord
	if len(logs) > 1 {
		previousLogs = logs[1:]
	}
	newInterval, newEF := calculator.CalculateInterval(previousLogs, log.Quality, ef)
	log.IntervalDays = newInterval

	if newEF > 0 {
		if isReverse {
			expr.ReverseEasinessFactor = newEF
		} else {
			expr.EasinessFactor = newEF
		}
	}

	if isReverse {
		expr.ReverseLogs = logs
	} else {
		expr.LearnedLogs = logs
	}

	if newInterval > 0 {
		nextDate := log.LearnedAt.AddDate(0, 0, newInterval)
		return nextDate.Format("2006-01-02")
	}
	return ""
}
