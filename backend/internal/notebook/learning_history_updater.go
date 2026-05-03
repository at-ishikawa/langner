package notebook

import (
	"strings"
	"time"
)

// normalizeQuotes replaces smart quotes with ASCII equivalents for comparison.
func normalizeQuotes(s string) string {
	r := strings.NewReplacer(
		"\u2018", "'", "\u2019", "'", // smart single quotes
		"\u201C", "\"", "\u201D", "\"", // smart double quotes
	)
	return r.Replace(s)
}

// LearningHistoryUpdater provides methods to update learning history
type LearningHistoryUpdater struct {
	history    []LearningHistory
	calculator IntervalCalculator
}

// NewLearningHistoryUpdater creates a new updater with the given history and calculator.
func NewLearningHistoryUpdater(history []LearningHistory, calculator IntervalCalculator) *LearningHistoryUpdater {
	if calculator == nil {
		calculator = &SM2Calculator{}
	}
	return &LearningHistoryUpdater{
		history:    history,
		calculator: calculator,
	}
}

// findOrCreateStory finds an existing story or creates a new one.
// flatType is a non-empty string (e.g. "flashcard", "etymology") when the
// history should use top-level Expressions instead of nested Scenes.
func (u *LearningHistoryUpdater) findOrCreateStory(notebookID, storyTitle, flatType string) int {
	normalizedTitle := normalizeQuotes(storyTitle)
	for i, h := range u.history {
		if normalizeQuotes(h.Metadata.Title) == normalizedTitle {
			return i
		}
	}

	newStory := LearningHistory{
		Metadata: LearningHistoryMetadata{
			NotebookID: notebookID,
			Title:      storyTitle,
		},
	}

	if flatType != "" {
		newStory.Metadata.Type = flatType
		newStory.Expressions = []LearningHistoryExpression{}
	} else {
		newStory.Scenes = []LearningScene{}
	}

	u.history = append(u.history, newStory)
	return len(u.history) - 1
}

// findOrCreateScene finds an existing scene or creates a new one.
// Uses normalizeQuotes so that titles with smart quotes (e.g. from book
// imports) match titles with ASCII apostrophes (from user input).
func (u *LearningHistoryUpdater) findOrCreateScene(storyIndex int, sceneTitle string) int {
	normalizedTitle := normalizeQuotes(sceneTitle)
	for i, s := range u.history[storyIndex].Scenes {
		if normalizeQuotes(s.Metadata.Title) == normalizedTitle {
			return i
		}
	}

	newScene := LearningScene{
		Metadata: LearningSceneMetadata{
			Title: sceneTitle,
		},
		Expressions: []LearningHistoryExpression{},
	}
	u.history[storyIndex].Scenes = append(u.history[storyIndex].Scenes, newScene)
	return len(u.history[storyIndex].Scenes) - 1
}

// GetHistory returns the updated history
func (u *LearningHistoryUpdater) GetHistory() []LearningHistory {
	return u.history
}

// UpdateOrCreateExpressionWithQuality updates or creates an expression with SM-2 quality assessment.
// originalExpression is the original expression form (e.g., Note.Expression) which may differ from
// expression (e.g., Note.Definition) when a definition is used as the lookup key. If originalExpression
// is non-empty, both forms are checked when matching existing entries to avoid duplicates.
func (u *LearningHistoryUpdater) UpdateOrCreateExpressionWithQuality(
	notebookID, storyTitle, sceneTitle, expression, originalExpression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) bool {
	isFlashcard := storyTitle == "flashcards" && sceneTitle == ""
	normalizedSceneTitle := normalizeQuotes(sceneTitle)

	for hi, h := range u.history {
		if normalizeQuotes(h.Metadata.Title) != normalizeQuotes(storyTitle) {
			continue
		}

		if isFlashcard || h.Metadata.Type == "flashcard" {
			for ei, exp := range h.Expressions {
				if exp.Expression != expression && (originalExpression == "" || exp.Expression != originalExpression) {
					continue
				}
				exp.AddRecordWithQuality(u.calculator, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
				u.history[hi].Expressions[ei] = exp
				return true
			}
			continue
		}

		for si, s := range h.Scenes {
			if normalizeQuotes(s.Metadata.Title) != normalizedSceneTitle {
				continue
			}

			for ei, exp := range s.Expressions {
				if exp.Expression != expression && (originalExpression == "" || exp.Expression != originalExpression) {
					continue
				}
				exp.AddRecordWithQuality(u.calculator, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
				u.history[hi].Scenes[si].Expressions[ei] = exp
				return true
			}
		}
	}

	u.createNewExpressionWithQuality(notebookID, storyTitle, sceneTitle, expression, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
	return false
}

// UpdateOrCreateExpressionWithQualityForReverse updates or creates an expression with SM-2 quality assessment for reverse quiz.
// originalExpression is the original expression form (e.g., Note.Expression) which may differ from
// expression (e.g., Note.Definition) when a definition is used as the lookup key. If originalExpression
// is non-empty, both forms are checked when matching existing entries to avoid duplicates.
func (u *LearningHistoryUpdater) UpdateOrCreateExpressionWithQualityForReverse(
	notebookID, storyTitle, sceneTitle, expression, originalExpression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) bool {
	isFlashcard := storyTitle == "flashcards" && sceneTitle == ""
	normalizedSceneTitle := normalizeQuotes(sceneTitle)

	for hi, h := range u.history {
		if normalizeQuotes(h.Metadata.Title) != normalizeQuotes(storyTitle) {
			continue
		}

		if isFlashcard || h.Metadata.Type == "flashcard" {
			for ei, exp := range h.Expressions {
				if exp.Expression != expression && (originalExpression == "" || exp.Expression != originalExpression) {
					continue
				}
				exp.AddRecordWithQualityForReverse(u.calculator, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
				u.history[hi].Expressions[ei] = exp
				return true
			}
			continue
		}

		for si, s := range h.Scenes {
			if normalizeQuotes(s.Metadata.Title) != normalizedSceneTitle {
				continue
			}

			for ei, exp := range s.Expressions {
				if exp.Expression != expression && (originalExpression == "" || exp.Expression != originalExpression) {
					continue
				}
				exp.AddRecordWithQualityForReverse(u.calculator, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
				u.history[hi].Scenes[si].Expressions[ei] = exp
				return true
			}
		}
	}

	u.createNewExpressionWithQualityForReverse(notebookID, storyTitle, sceneTitle, expression, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
	return false
}

// createNewExpressionWithQualityForReverse creates a new expression entry with quality data for reverse quiz
func (u *LearningHistoryUpdater) createNewExpressionWithQualityForReverse(
	notebookID, storyTitle, sceneTitle, expression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) {
	flatType := ""
	if storyTitle == "flashcards" && sceneTitle == "" {
		flatType = "flashcard"
	}
	storyIndex := u.findOrCreateStory(notebookID, storyTitle, flatType)

	newExpression := LearningHistoryExpression{
		Expression:  expression,
		LearnedLogs: []LearningRecord{},
		ReverseLogs: []LearningRecord{},
	}
	newExpression.AddRecordWithQualityForReverse(u.calculator, isCorrect, isKnownWord, quality, responseTimeMs, quizType)

	if len(newExpression.ReverseLogs) == 0 {
		return
	}

	if flatType != "" || u.history[storyIndex].Metadata.Type == "flashcard" {
		u.history[storyIndex].Expressions = append(
			u.history[storyIndex].Expressions,
			newExpression,
		)
		return
	}

	sceneIndex := u.findOrCreateScene(storyIndex, sceneTitle)
	u.history[storyIndex].Scenes[sceneIndex].Expressions = append(
		u.history[storyIndex].Scenes[sceneIndex].Expressions,
		newExpression,
	)
}

// createNewExpressionWithQuality creates a new expression entry with quality data
func (u *LearningHistoryUpdater) createNewExpressionWithQuality(
	notebookID, storyTitle, sceneTitle, expression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) {
	flatType := ""
	if storyTitle == "flashcards" && sceneTitle == "" {
		flatType = "flashcard"
	}
	storyIndex := u.findOrCreateStory(notebookID, storyTitle, flatType)

	newExpression := LearningHistoryExpression{
		Expression:  expression,
		LearnedLogs: []LearningRecord{},
	}
	newExpression.AddRecordWithQuality(u.calculator, isCorrect, isKnownWord, quality, responseTimeMs, quizType)

	if len(newExpression.LearnedLogs) == 0 {
		return
	}

	if flatType != "" || u.history[storyIndex].Metadata.Type == "flashcard" {
		u.history[storyIndex].Expressions = append(
			u.history[storyIndex].Expressions,
			newExpression,
		)
		return
	}

	sceneIndex := u.findOrCreateScene(storyIndex, sceneTitle)
	u.history[storyIndex].Scenes[sceneIndex].Expressions = append(
		u.history[storyIndex].Scenes[sceneIndex].Expressions,
		newExpression,
	)
}

// FindExpressionByName searches for an expression across all histories, returning
// a pointer to the expression. Returns nil if not found.
func (u *LearningHistoryUpdater) FindExpressionByName(expression string) *LearningHistoryExpression {
	for hi := range u.history {
		h := &u.history[hi]
		// Always search top-level expressions first (flashcard, etymology, etc.)
		for ei := range h.Expressions {
			if strings.EqualFold(h.Expressions[ei].Expression, expression) {
				return &h.Expressions[ei]
			}
		}
		// Then search scenes
		for si := range h.Scenes {
			for ei := range h.Scenes[si].Expressions {
				if strings.EqualFold(h.Scenes[si].Expressions[ei].Expression, expression) {
					return &h.Scenes[si].Expressions[ei]
				}
			}
		}
	}
	return nil
}

// OverrideLog finds a learning log by learnedAt date and quiz type, then overrides it.
// Returns the original values for undo purposes.
func (u *LearningHistoryUpdater) OverrideLog(
	expression string,
	quizType QuizType,
	learnedAt string,
	markCorrect *bool,
	nextReviewDate string,
) (originalQuality int, originalStatus string, originalIntervalDays int, newNextReview string, found bool) {
	expr := u.FindExpressionByName(expression)
	if expr == nil {
		return 0, "", 0, "", false
	}

	logs := expr.GetLogsForQuizType(quizType)
	for i, log := range logs {
		if log.LearnedAt.Format("2006-01-02") != learnedAt && log.LearnedAt.Format(time.RFC3339) != learnedAt {
			continue
		}

		originalQuality = log.Quality
		originalStatus = string(log.Status)
		originalIntervalDays = log.IntervalDays

		if markCorrect != nil {
			if *markCorrect {
				logs[i].Quality = 3
				logs[i].Status = LearnedStatusUnderstood
			} else {
				logs[i].Quality = 1
				logs[i].Status = LearnedStatusMisunderstood
			}

			// Derive EF from logs before this entry
			var previousLogs []LearningRecord
			if i+1 < len(logs) {
				previousLogs = logs[i+1:]
			}
			derivedEF := u.calculator.DeriveEF(previousLogs)
			newInterval, _ := u.calculator.CalculateInterval(previousLogs, logs[i].Quality, derivedEF)
			logs[i].IntervalDays = newInterval
		}

		if nextReviewDate != "" {
			nextDate, err := time.Parse("2006-01-02", nextReviewDate)
			if err == nil {
				intervalDays := int(nextDate.Sub(log.LearnedAt.Time).Hours() / 24)
				if intervalDays < 1 {
					intervalDays = 1
				}
				logs[i].IntervalDays = intervalDays
				logs[i].OverrideInterval = intervalDays
			}
		}

		// Write back the logs
		switch quizType {
		case QuizTypeReverse:
			expr.ReverseLogs = logs
		case QuizTypeEtymologyStandard:
			expr.EtymologyBreakdownLogs = logs
		case QuizTypeEtymologyReverse:
			expr.EtymologyAssemblyLogs = logs
		default:
			expr.LearnedLogs = logs
		}

		newNextReview = logs[i].LearnedAt.AddDate(0, 0, logs[i].IntervalDays).Format("2006-01-02")
		return originalQuality, originalStatus, originalIntervalDays, newNextReview, true
	}

	return 0, "", 0, "", false
}

// UndoOverrideLog restores original values for a learning log entry.
func (u *LearningHistoryUpdater) UndoOverrideLog(
	expression string,
	quizType QuizType,
	learnedAt string,
	originalQuality int,
	originalStatus string,
	originalIntervalDays int,
) (correct bool, nextReview string, found bool) {
	expr := u.FindExpressionByName(expression)
	if expr == nil {
		return false, "", false
	}

	logs := expr.GetLogsForQuizType(quizType)
	for i, log := range logs {
		if log.LearnedAt.Format("2006-01-02") != learnedAt && log.LearnedAt.Format(time.RFC3339) != learnedAt {
			continue
		}

		logs[i].Quality = originalQuality
		logs[i].Status = LearnedStatus(originalStatus)
		logs[i].IntervalDays = originalIntervalDays
		logs[i].OverrideInterval = 0

		switch quizType {
		case QuizTypeReverse:
			expr.ReverseLogs = logs
		case QuizTypeEtymologyStandard:
			expr.EtymologyBreakdownLogs = logs
		case QuizTypeEtymologyReverse:
			expr.EtymologyAssemblyLogs = logs
		default:
			expr.LearnedLogs = logs
		}

		correct = logs[i].Quality >= 3
		nextReview = logs[i].LearnedAt.AddDate(0, 0, logs[i].IntervalDays).Format("2006-01-02")
		return correct, nextReview, true
	}

	return false, "", false
}

// SetSkippedAt records a skip for the given quiz type at the given timestamp.
// Returns false if the expression isn't found in any history.
func (u *LearningHistoryUpdater) SetSkippedAt(expression string, quizType QuizType, skippedAt string) bool {
	expr := u.FindExpressionByName(expression)
	if expr == nil {
		return false
	}
	expr.SkippedAt = expr.SkippedAt.Set(quizType, skippedAt)
	return true
}

// UpdateOrCreateExpressionWithQualityForEtymology updates or creates an expression with SM-2 quality assessment for etymology quiz.
//
// Etymology learning history is stored under per-session scenes (sceneTitle =
// the session's metadata.title) so multi-sense origins are tracked separately.
// Callers must always pass a non-empty sceneTitle.
func (u *LearningHistoryUpdater) UpdateOrCreateExpressionWithQualityForEtymology(
	notebookID, storyTitle, sceneTitle, expression, originalExpression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) bool {
	normalizedSceneTitle := normalizeQuotes(sceneTitle)

	for hi, h := range u.history {
		if normalizeQuotes(h.Metadata.Title) != normalizeQuotes(storyTitle) {
			continue
		}

		for si, s := range h.Scenes {
			if normalizeQuotes(s.Metadata.Title) != normalizedSceneTitle {
				continue
			}

			for ei, exp := range s.Expressions {
				if exp.Expression != expression && (originalExpression == "" || exp.Expression != originalExpression) {
					continue
				}
				exp.AddRecordWithQualityForEtymology(u.calculator, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
				u.history[hi].Scenes[si].Expressions[ei] = exp
				return true
			}
		}
	}

	u.createNewExpressionWithQualityForEtymology(notebookID, storyTitle, sceneTitle, expression, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
	return false
}

// createNewExpressionWithQualityForEtymology creates a new expression entry with quality data for etymology quiz
func (u *LearningHistoryUpdater) createNewExpressionWithQualityForEtymology(
	notebookID, storyTitle, sceneTitle, expression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) {
	storyIndex := u.findOrCreateStory(notebookID, storyTitle, "")
	// Mark as etymology so the validator skips the per-scene duplicate check
	// (multi-sense origins legitimately appear in multiple session scenes).
	if u.history[storyIndex].Metadata.Type == "" {
		u.history[storyIndex].Metadata.Type = "etymology"
	}

	newExpression := LearningHistoryExpression{
		Expression:  expression,
		LearnedLogs: []LearningRecord{},
	}
	newExpression.AddRecordWithQualityForEtymology(u.calculator, isCorrect, isKnownWord, quality, responseTimeMs, quizType)

	logs := newExpression.GetLogsForQuizType(quizType)
	if len(logs) == 0 {
		return
	}

	sceneIndex := u.findOrCreateScene(storyIndex, sceneTitle)
	u.history[storyIndex].Scenes[sceneIndex].Expressions = append(
		u.history[storyIndex].Scenes[sceneIndex].Expressions,
		newExpression,
	)
}

// ClearSkippedAt removes the skip for the given quiz type. The expression
// remains skipped for any other quiz types still set in its SkippedAt map.
func (u *LearningHistoryUpdater) ClearSkippedAt(expression string, quizType QuizType) bool {
	expr := u.FindExpressionByName(expression)
	if expr == nil {
		return false
	}
	expr.SkippedAt.Clear(quizType)
	return true
}
