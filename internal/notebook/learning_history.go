package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type LearningHistory struct {
	Metadata    LearningHistoryMetadata     `yaml:"metadata"`
	Scenes      []LearningScene             `yaml:"scenes,omitempty"`
	Expressions []LearningHistoryExpression `yaml:"expressions,omitempty"`
}

type LearningHistoryMetadata struct {
	NotebookID string `yaml:"id"`
	Title      string `yaml:"title"`
	Type       string `yaml:"type,omitempty"`
}

func NewLearningHistories(directory string) (map[string][]LearningHistory, error) {
	return loadYamlFilesAsMap[[]LearningHistory](directory, func(path string, info os.FileInfo) bool {
		return !info.IsDir() && filepath.Ext(path) == ".yml"
	})
}

func (h LearningHistory) GetLogs(
	notebookTitle, sceneTitle string, definition Note,
) []LearningRecord {
	if h.Metadata.Title != notebookTitle {
		return nil
	}

	// For flashcard type, search in expressions directly
	if h.Metadata.Type == "flashcard" {
		for _, expression := range h.Expressions {
			if expression.Expression != definition.Expression && expression.Expression != definition.Definition {
				continue
			}
			return expression.LearnedLogs
		}
		return nil
	}

	// Search through scenes for the matching scene title
	for _, scene := range h.Scenes {
		if scene.Metadata.Title != sceneTitle {
			continue
		}

		// Search through expressions in this scene
		for _, expression := range scene.Expressions {
			if expression.Expression != definition.Expression && expression.Expression != definition.Definition {
				continue
			}

			return expression.LearnedLogs
		}
	}
	return nil
}

type LearningScene struct {
	Metadata    LearningSceneMetadata `yaml:"metadata"`
	Expressions []LearningHistoryExpression
}

type LearningSceneMetadata struct {
	Title string `yaml:"title"`
}

// LearningRecord represents a single learning event for an expression
type LearningRecord struct {
	Status         LearnedStatus `yaml:"status,omitempty"`
	LearnedAt      Date          `yaml:"learned_at,omitempty"`
	Quality        int           `yaml:"quality,omitempty"`          // 0-5 grade
	ResponseTimeMs int64         `yaml:"response_time_ms,omitempty"` // milliseconds
	QuizType       string        `yaml:"quiz_type,omitempty"`        // "freeform" or "notebook"
	IntervalDays   int           `yaml:"interval_days,omitempty"`    // days until next review
}

type LearningHistoryExpression struct {
	Expression     string           `yaml:"expression"`
	LearnedLogs    []LearningRecord `yaml:"learned_logs"`
	EasinessFactor float64          `yaml:"easiness_factor,omitempty"` // default 2.5

	// Reverse quiz fields - track separately from regular quiz
	ReverseLogs           []LearningRecord `yaml:"reverse_logs,omitempty"`
	ReverseEasinessFactor float64          `yaml:"reverse_easiness_factor,omitempty"` // default 2.5
}

func (exp LearningHistoryExpression) GetLatestStatus() LearnedStatus {
	if len(exp.LearnedLogs) == 0 {
		return learnedStatusLearning
	}
	// Get the first element since new logs are prepended
	lastLog := exp.LearnedLogs[0]
	return lastLog.Status
}

// GetLogsForQuizType returns learning logs for the specified quiz type
func (exp LearningHistoryExpression) GetLogsForQuizType(quizType QuizType) []LearningRecord {
	if quizType == QuizTypeReverse {
		return exp.ReverseLogs
	}
	return exp.LearnedLogs
}

// GetEasinessFactorForQuizType returns the easiness factor for the specified quiz type
func (exp LearningHistoryExpression) GetEasinessFactorForQuizType(quizType QuizType) float64 {
	if quizType == QuizTypeReverse {
		if exp.ReverseEasinessFactor == 0 {
			return DefaultEasinessFactor
		}
		return exp.ReverseEasinessFactor
	}
	if exp.EasinessFactor == 0 {
		return DefaultEasinessFactor
	}
	return exp.EasinessFactor
}

// AddRecordWithQualityForReverse adds a new learning record for reverse quiz with SM-2 quality data
func (exp *LearningHistoryExpression) AddRecordWithQualityForReverse(
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
) {
	correctStreak := GetCorrectStreak(exp.ReverseLogs)
	lastInterval := GetLastInterval(exp.ReverseLogs)

	status := LearnedStatusMisunderstood
	if isCorrect {
		if isKnownWord {
			status = learnedStatusUnderstood
		} else {
			status = learnedStatusCanBeUsed
		}
		correctStreak++
	}

	if exp.ReverseEasinessFactor == 0 {
		exp.ReverseEasinessFactor = DefaultEasinessFactor
	}

	exp.ReverseEasinessFactor = UpdateEasinessFactor(exp.ReverseEasinessFactor, quality, correctStreak)
	nextInterval := CalculateNextInterval(lastInterval, exp.ReverseEasinessFactor, quality, correctStreak)

	newRecord := LearningRecord{
		Status:         status,
		LearnedAt:      NewDate(),
		Quality:        quality,
		ResponseTimeMs: responseTimeMs,
		QuizType:       string(QuizTypeReverse),
		IntervalDays:   nextInterval,
	}

	exp.ReverseLogs = append([]LearningRecord{newRecord}, exp.ReverseLogs...)
}

// HasAnyCorrectAnswer returns true if the expression has at least one correct answer
// in the forward quiz (LearnedLogs). This is used to determine if a word is ready
// for reverse quiz - words should be learned in forward direction first.
func (exp LearningHistoryExpression) HasAnyCorrectAnswer() bool {
	for _, log := range exp.LearnedLogs {
		if log.Status == learnedStatusUnderstood ||
			log.Status == learnedStatusCanBeUsed ||
			log.Status == learnedStatusIntuitivelyUsed {
			return true
		}
	}
	return false
}

// NeedsReverseReview returns true if the expression needs reverse quiz review
// based on spaced repetition algorithm
func (exp LearningHistoryExpression) NeedsReverseReview() bool {
	if len(exp.ReverseLogs) == 0 {
		return true
	}

	lastLog := exp.ReverseLogs[0]

	// Always include misunderstood expressions for review
	if lastLog.Status == LearnedStatusMisunderstood {
		return true
	}

	// Use stored interval if available
	threshold := lastLog.IntervalDays
	if threshold == 0 {
		// Fallback: calculate based on correct streak
		correctCount := 0
		for _, log := range exp.ReverseLogs {
			if log.Status != LearnedStatusMisunderstood && log.Status != learnedStatusLearning {
				correctCount++
			}
		}
		threshold = GetThresholdDaysFromCount(correctCount)
	}

	// Calculate elapsed days since last review
	// LearnedAt is stored as RFC3339 timestamp, so we can calculate actual elapsed time
	now := time.Now()
	elapsed := now.Sub(lastLog.LearnedAt.Time)
	elapsedDays := int(elapsed.Hours() / 24)

	// Need review if elapsed days >= threshold
	return elapsedDays >= threshold
}

// AddRecordWithQuality adds a new learning record with SM-2 quality data
func (exp *LearningHistoryExpression) AddRecordWithQuality(
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) {
	correctStreak := GetCorrectStreak(exp.LearnedLogs)
	lastInterval := GetLastInterval(exp.LearnedLogs)

	status := LearnedStatusMisunderstood
	if isCorrect {
		if isKnownWord {
			status = learnedStatusUnderstood
		} else {
			status = learnedStatusCanBeUsed
		}
		correctStreak++
	}

	if exp.EasinessFactor == 0 {
		exp.EasinessFactor = DefaultEasinessFactor
	}

	exp.EasinessFactor = UpdateEasinessFactor(exp.EasinessFactor, quality, correctStreak)
	nextInterval := CalculateNextInterval(lastInterval, exp.EasinessFactor, quality, correctStreak)

	newRecord := LearningRecord{
		Status:         status,
		LearnedAt:      NewDate(),
		Quality:        quality,
		ResponseTimeMs: responseTimeMs,
		QuizType:       string(quizType),
		IntervalDays:   nextInterval,
	}

	exp.LearnedLogs = append([]LearningRecord{newRecord}, exp.LearnedLogs...)
}

// Validate validates a LearningHistoryExpression
func (exp *LearningHistoryExpression) Validate(location string) []ValidationError {
	var errors []ValidationError

	// Check if expression is not empty
	if strings.TrimSpace(exp.Expression) == "" {
		errors = append(errors, ValidationError{
			Location: location,
			Message:  "expression field is empty",
		})
		return errors
	}

	// Skip validation if no learned_logs
	if len(exp.LearnedLogs) == 0 {
		return errors
	}

	// Validate learned logs
	validStatuses := map[LearnedStatus]bool{
		learnedStatusLearning:        true,
		LearnedStatusMisunderstood:   true,
		learnedStatusUnderstood:      true,
		learnedStatusCanBeUsed:       true,
		learnedStatusIntuitivelyUsed: true,
	}

	var prevDate time.Time

	for logIdx, log := range exp.LearnedLogs {
		logLocation := fmt.Sprintf("%s -> log[%d]", location, logIdx)

		// Validate status
		if !validStatuses[log.Status] {
			errors = append(errors, ValidationError{
				Location: logLocation,
				Message:  fmt.Sprintf("invalid status: %q", log.Status),
				Suggestions: []string{
					"valid statuses are: '', 'misunderstood', 'understood', 'usable', 'intuitive'",
				},
			})
		}

		// Validate date format and required field
		if log.LearnedAt.IsZero() {
			errors = append(errors, ValidationError{
				Location: logLocation,
				Message:  "learned_at is required but missing or invalid",
				Suggestions: []string{
					"use format YYYY-MM-DD",
				},
			})
			continue
		}

		// Check chronological order (logs should be sorted newest first)
		if logIdx > 0 && log.LearnedAt.After(prevDate) {
			errors = append(errors, ValidationError{
				Location: logLocation,
				Message:  fmt.Sprintf("learned_logs not in chronological order (newest first): %s comes after %s", log.LearnedAt.Format("2006-01-02"), prevDate.Format("2006-01-02")),
				Suggestions: []string{
					"sort learned_logs by date in descending order (newest first)",
				},
			})
		}
		prevDate = log.LearnedAt.Time
	}

	return errors
}

// Validate validates a LearningScene
func (scene *LearningScene) Validate(location string) []ValidationError {
	var errors []ValidationError

	for exprIdx, expr := range scene.Expressions {
		exprLocation := fmt.Sprintf("%s -> expression[%d]: %s", location, exprIdx, expr.Expression)
		if exprErrors := expr.Validate(exprLocation); len(exprErrors) > 0 {
			errors = append(errors, exprErrors...)
		}
	}

	return errors
}

// Validate validates a LearningHistory
func (h *LearningHistory) Validate(location string) []ValidationError {
	var errors []ValidationError

	// For flashcard type, validate expressions directly
	if h.Metadata.Type == "flashcard" {
		for exprIdx, expr := range h.Expressions {
			exprLocation := fmt.Sprintf("%s -> expression[%d]: %s", location, exprIdx, expr.Expression)
			if exprErrors := expr.Validate(exprLocation); len(exprErrors) > 0 {
				errors = append(errors, exprErrors...)
			}
		}

		// Check for duplicate expressions in flashcard format
		expressionSeen := make(map[string]bool)
		for _, expr := range h.Expressions {
			expression := strings.TrimSpace(expr.Expression)
			if expression == "" {
				continue
			}
			if expressionSeen[expression] {
				errors = append(errors, ValidationError{
					Location: location,
					Message:  fmt.Sprintf("duplicate expression %q in flashcard format", expression),
				})
			}
			expressionSeen[expression] = true
		}

		return errors
	}

	// For story type (default), validate scenes
	for sceneIdx, scene := range h.Scenes {
		sceneLocation := fmt.Sprintf("%s -> scene[%d]: %s", location, sceneIdx, scene.Metadata.Title)
		if sceneErrors := scene.Validate(sceneLocation); len(sceneErrors) > 0 {
			errors = append(errors, sceneErrors...)
		}
	}

	// Check for duplicate expressions across different scenes in the same episode
	episodeExpressions := make(map[string][]string) // expression -> list of scene titles
	for _, scene := range h.Scenes {
		sceneTitle := strings.TrimSpace(scene.Metadata.Title)
		for _, expr := range scene.Expressions {
			expression := strings.TrimSpace(expr.Expression)
			if expression == "" {
				continue
			}
			episodeExpressions[expression] = append(episodeExpressions[expression], sceneTitle)
		}
	}

	// Report duplicates across scenes
	for expression, scenes := range episodeExpressions {
		if len(scenes) > 1 {
			errors = append(errors, ValidationError{
				Location: location,
				Message:  fmt.Sprintf("expression %q appears in multiple scenes: %v", expression, scenes),
				Suggestions: []string{
					"run validate --fix to merge duplicate expressions",
				},
			})
		}
	}

	return errors
}
