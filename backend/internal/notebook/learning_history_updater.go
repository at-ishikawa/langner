package notebook

import (
	"fmt"
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

			// Replay the chain of older logs with this entry appended so
			// the recomputed interval matches what `validate --fix` would
			// produce: same chain logic, same early-review guard.
			var previousLogs []LearningRecord
			if i+1 < len(logs) {
				previousLogs = logs[i+1:]
			}
			newInterval, _ := u.calculator.NextIntervalForWrite(previousLogs, logs[i])
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

// EnsureExpressionStubForSkip creates a learned-log-free stub for the
// expression at (notebookID, storyTitle, sceneTitle) when no entry exists
// yet. The stub holds only the expression name; SetSkippedAt then writes
// the skip timestamp onto it. This is the path the notebook detail page's
// per-type skip checkboxes take when the user clicks Skip on a word that
// hasn't been studied yet — without it, the only way to record a skip
// was UpdateOrCreateExpressionWithQuality, which fabricated a fake
// "quality 5" learned_log entry that pretended the user had answered
// the word correctly.
//
// If the expression already exists anywhere in the history, this is a
// no-op and SetSkippedAt updates the existing record in place.
//
// sceneTitle "" stores the stub at the top-level Expressions list
// (flashcard-style); a non-empty value nests it under that scene.
func (u *LearningHistoryUpdater) EnsureExpressionStubForSkip(
	notebookID, storyTitle, sceneTitle, expression string,
) {
	if u.FindExpressionByName(expression) != nil {
		return
	}
	stub := LearningHistoryExpression{Expression: expression}
	if sceneTitle == "" {
		idx := u.findOrCreateStory(notebookID, storyTitle, "flashcard")
		u.history[idx].Expressions = append(u.history[idx].Expressions, stub)
		return
	}
	storyIdx := u.findOrCreateStory(notebookID, storyTitle, "")
	sceneIdx := u.findOrCreateScene(storyIdx, sceneTitle)
	u.history[storyIdx].Scenes[sceneIdx].Expressions = append(
		u.history[storyIdx].Scenes[sceneIdx].Expressions, stub,
	)
}

// UpdateOrCreateExpressionWithQualityForEtymology updates or creates an
// origin entry. Lookup matches on (session, expression, Type=origin)
// across EVERY scene in the matching session — scene title is not part
// of the key. This is what stops an origin's learning history from
// splitting into two entries when the scene title pickBestSceneForOrigin
// derives drifts over time: today's writer might be told to use
// "derma (skin)" while the prior writer used "gyne (woman)", but if the
// origin already lives under "gyne (woman)" we update there. Vocab
// entries are filtered out so an etymology log can't pollute a same-
// named word; legacy type-empty entries that already carry etymology
// logs are upgraded in place to Type=origin so re-runs converge on the
// typed shape.
//
// Only when no existing origin entry is found do we create a new one,
// and only then does sceneTitle matter (it's the location for the new
// entry, derived by pickBestSceneForOrigin).
func (u *LearningHistoryUpdater) UpdateOrCreateExpressionWithQualityForEtymology(
	notebookID, storyTitle, sceneTitle, expression, originalExpression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) bool {
	for hi, h := range u.history {
		if normalizeQuotes(h.Metadata.Title) != normalizeQuotes(storyTitle) {
			continue
		}

		for si, s := range h.Scenes {
			for ei, exp := range s.Expressions {
				if exp.Expression != expression && (originalExpression == "" || exp.Expression != originalExpression) {
					continue
				}
				// Skip vocab entries — only update origin entries (or
				// legacy type-empty entries that already carry etymology
				// logs, which we can safely upgrade).
				if exp.Type != LearningExpressionTypeOrigin {
					if exp.Type != "" {
						continue
					}
					if len(exp.EtymologyBreakdownLogs) == 0 && len(exp.EtymologyAssemblyLogs) == 0 {
						continue
					}
				}
				if exp.Type == "" {
					exp.Type = LearningExpressionTypeOrigin
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

// createNewExpressionWithQualityForEtymology creates a new expression entry
// with quality data for etymology quizzes. The entry is tagged
// Type=origin so it never collides with a vocab entry sharing the same
// name in the same scene (e.g., "ego" the word vs the Latin root).
func (u *LearningHistoryUpdater) createNewExpressionWithQualityForEtymology(
	notebookID, storyTitle, sceneTitle, expression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) {
	storyIndex := u.findOrCreateStory(notebookID, storyTitle, "")

	newExpression := LearningHistoryExpression{
		Expression:  expression,
		Type:        LearningExpressionTypeOrigin,
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

// AssertNoDuplicateOriginsInSession returns a non-nil error if the given
// session block holds the same origin expression under more than one
// scene. Used by SaveEtymologyOriginResult as a structural guard right
// before WriteYamlFile so any write that would re-introduce the "two
// logos sessions" class of bug fails loudly instead of silently
// corrupting the YAML. After the etymology source migration this
// invariant cannot fail in normal operation — the writer always
// addresses scenes the source declares — so a trip indicates either a
// real regression or hand-edited data that needs reconciliation.
func AssertNoDuplicateOriginsInSession(history []LearningHistory, notebookID, sessionTitle string) error {
	normalised := normalizeQuotes(sessionTitle)
	for _, h := range history {
		if normalizeQuotes(h.Metadata.Title) != normalised {
			continue
		}
		scenesByOrigin := make(map[string][]string)
		for _, scene := range h.Scenes {
			for _, expr := range scene.Expressions {
				if expr.Type != LearningExpressionTypeOrigin {
					continue
				}
				name := strings.TrimSpace(expr.Expression)
				if name == "" {
					continue
				}
				scenesByOrigin[name] = append(scenesByOrigin[name], scene.Metadata.Title)
			}
		}
		for origin, scenes := range scenesByOrigin {
			if len(scenes) > 1 {
				return fmt.Errorf(
					"invariant violation: origin %q appears in %d scenes (%v) within notebook %q session %q — refusing to write",
					origin, len(scenes), scenes, notebookID, sessionTitle,
				)
			}
		}
	}
	return nil
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
