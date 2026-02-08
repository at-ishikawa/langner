package cli

import (
	"fmt"
	"path/filepath"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// MigrateLearningHistory migrates all learning history files to the new SM-2 format
func MigrateLearningHistory(learningNotesDir string) error {
	histories, err := notebook.NewLearningHistories(learningNotesDir)
	if err != nil {
		return fmt.Errorf("failed to load learning histories: %w", err)
	}

	for notebookName, historyList := range histories {
		modified := false
		for histIdx := range historyList {
			hist := &historyList[histIdx]

			if hist.Metadata.Type == "flashcard" {
				for exprIdx := range hist.Expressions {
					if migrateExpression(&hist.Expressions[exprIdx]) {
						modified = true
					}
				}
				continue
			}

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
func migrateExpression(exp *notebook.LearningHistoryExpression) bool {
	modified := false

	if exp.EasinessFactor == 0 {
		exp.EasinessFactor = calculateEasinessFactor(exp.LearnedLogs)
		modified = true
	}

	for logIdx := range exp.LearnedLogs {
		log := &exp.LearnedLogs[logIdx]

		if log.Quality == 0 {
			if log.Status == notebook.LearnedStatusMisunderstood {
				log.Quality = int(notebook.QualityWrong)
			} else {
				log.Quality = int(notebook.QualityCorrect)
			}
			modified = true
		}

		if log.IntervalDays == 0 {
			log.IntervalDays = calculateLegacyInterval(logIdx, exp.LearnedLogs)
			modified = true
		}
	}

	return modified
}

// calculateEasinessFactor calculates EF from learning history pattern
func calculateEasinessFactor(logs []notebook.LearningRecord) float64 {
	if len(logs) == 0 {
		return notebook.DefaultEasinessFactor
	}

	ef := notebook.DefaultEasinessFactor

	// Process logs from oldest to newest (reverse order since newest is first)
	for i := len(logs) - 1; i >= 0; i-- {
		log := logs[i]

		quality := log.Quality
		if quality == 0 {
			if log.Status == notebook.LearnedStatusMisunderstood {
				quality = int(notebook.QualityWrong)
			} else {
				quality = int(notebook.QualityCorrect)
			}
		}

		correctStreak := countCorrectFromIndex(logs, i)
		ef = notebook.UpdateEasinessFactor(ef, quality, correctStreak)
	}

	return ef
}

// countCorrectFromIndex counts consecutive correct answers from the given index to the end (oldest)
func countCorrectFromIndex(logs []notebook.LearningRecord, fromIndex int) int {
	count := 0
	for j := fromIndex + 1; j < len(logs); j++ {
		if logs[j].Status == notebook.LearnedStatusMisunderstood {
			break
		}
		if logs[j].Status != "" {
			count++
		}
	}
	return count
}

// calculateLegacyInterval calculates interval for old records based on position
func calculateLegacyInterval(logIndex int, logs []notebook.LearningRecord) int {
	count := 0
	for j := logIndex; j < len(logs); j++ {
		if logs[j].Status != "" && logs[j].Status != notebook.LearnedStatusMisunderstood {
			count++
		}
	}
	return notebook.GetThresholdDaysFromCount(count)
}
