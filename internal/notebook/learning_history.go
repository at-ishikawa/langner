package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type LearningHistory struct {
	Metadata LearningHistoryMetadata `yaml:"metadata"`
	Scenes   []LearningScene         `yaml:"scenes"`
}

type LearningHistoryMetadata struct {
	NotebookID string `yaml:"id"`
	Title      string `yaml:"title"`
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

type LearningHistoryExpression struct {
	Expression  string           `yaml:"expression"`
	LearnedLogs []LearningRecord `yaml:"learned_logs"`
}

func (exp LearningHistoryExpression) GetLatestStatus() LearnedStatus {
	if len(exp.LearnedLogs) == 0 {
		return learnedStatusLearning
	}
	// Get the first element since AddRecord prepends new logs
	lastLog := exp.LearnedLogs[0]
	return lastLog.Status
}

func (exp *LearningHistoryExpression) AddRecord(isCorrect, isKnownWord bool) {
	status := LearnedStatusMisunderstood
	if isCorrect {
		if isKnownWord {
			status = learnedStatusUnderstood
		} else {
			status = learnedStatusCanBeUsed
		}
	}

	// Check if the status would change from the current status
	currentStatus := exp.GetLatestStatus()
	if currentStatus == status {
		// Status hasn't changed, don't add a duplicate record
		return
	}

	exp.LearnedLogs = append([]LearningRecord{
		{
			Status:    status,
			LearnedAt: NewDate(),
		},
	}, exp.LearnedLogs...)
}

// AddRecordAlways adds a learning record for QA command
// Records both correct and incorrect answers
func (exp *LearningHistoryExpression) AddRecordAlways(isCorrect, isKnownWord bool) {
	var status LearnedStatus

	if isCorrect {
		status = learnedStatusUnderstood
		if !isKnownWord {
			status = learnedStatusCanBeUsed
		}
	} else {
		// Record misunderstood status for incorrect answers
		status = LearnedStatusMisunderstood

		// Check if the last status was already misunderstood
		if exp.GetLatestStatus() == LearnedStatusMisunderstood {
			// Don't record duplicate consecutive misunderstood entries
			return
		}
	}

	// Record the learning attempt
	exp.LearnedLogs = append([]LearningRecord{
		{
			Status:    status,
			LearnedAt: NewDate(),
		},
	}, exp.LearnedLogs...)
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

	seenDates := make(map[string]bool)
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

		// Check for duplicate dates
		dateStr := log.LearnedAt.Format("2006-01-02")
		if seenDates[dateStr] {
			errors = append(errors, ValidationError{
				Location: location,
				Message:  fmt.Sprintf("duplicate learned_at date: %s", dateStr),
			})
		}
		seenDates[dateStr] = true

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
