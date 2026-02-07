package cli

import (
	"fmt"
	"path/filepath"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// MigrateLearningHistory migrates all learning history files to the new SM-2 format
func MigrateLearningHistory(learningNotesDir string) error {
	// Load all learning histories
	histories, err := notebook.NewLearningHistories(learningNotesDir)
	if err != nil {
		return fmt.Errorf("failed to load learning histories: %w", err)
	}

	for notebookName, historyList := range histories {
		modified := false
		for histIdx := range historyList {
			hist := &historyList[histIdx]

			// For flashcard type, migrate expressions directly
			if hist.Metadata.Type == "flashcard" {
				for exprIdx := range hist.Expressions {
					if migrateExpression(&hist.Expressions[exprIdx]) {
						modified = true
					}
				}
				continue
			}

			// For story type, migrate expressions in scenes
			for sceneIdx := range hist.Scenes {
				for exprIdx := range hist.Scenes[sceneIdx].Expressions {
					if migrateExpression(&hist.Scenes[sceneIdx].Expressions[exprIdx]) {
						modified = true
					}
				}
			}
		}

		if modified {
			notePath := filepath.Join(learningNotesDir, notebookName+".yml")
			if err := notebook.WriteYamlFile(notePath, historyList); err != nil {
				return fmt.Errorf("failed to write %s: %w", notePath, err)
			}
			fmt.Printf("Migrated: %s\n", notebookName)
		}
	}

	fmt.Println("Migration complete!")
	return nil
}

// migrateExpression migrates a single expression to the new format
// Returns true if any changes were made
func migrateExpression(exp *notebook.LearningHistoryExpression) bool {
	modified := false

	// Set EasinessFactor if not set
	if exp.EasinessFactor == 0 {
		exp.EasinessFactor = notebook.DefaultEasinessFactor
		modified = true
	}

	// Migrate each log entry
	for logIdx := range exp.LearnedLogs {
		log := &exp.LearnedLogs[logIdx]

		// Set Quality if not set
		if log.Quality == 0 {
			if log.Status == notebook.LearnedStatusMisunderstood {
				log.Quality = 1
			} else {
				log.Quality = 4 // assume normal for old correct answers
			}
			modified = true
		}

		// Set IntervalDays if not set
		if log.IntervalDays == 0 {
			log.IntervalDays = calculateLegacyInterval(logIdx, exp.LearnedLogs)
			modified = true
		}
	}

	return modified
}

// calculateLegacyInterval calculates interval for old records based on position
func calculateLegacyInterval(logIndex int, logs []notebook.LearningRecord) int {
	// Count correct answers from this log to the end (oldest)
	count := 0
	for j := logIndex; j < len(logs); j++ {
		if logs[j].Status != "" && logs[j].Status != notebook.LearnedStatusMisunderstood {
			count++
		}
	}
	return notebook.GetThresholdDaysFromCount(count)
}
