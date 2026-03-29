package cli

import (
	"fmt"
	"path/filepath"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// MigrateLearningHistory migrates all learning history files to the current format.
// When recalculate is true, intervals are recalculated using the provided calculator.
func MigrateLearningHistory(learningNotesDir string, recalculate bool, calculator notebook.IntervalCalculator) error {
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
					if migrateExpression(&hist.Expressions[exprIdx], recalculate, calculator) {
						modified = true
					}
				}
				continue
			}

			for sceneIdx := range hist.Scenes {
				for exprIdx := range hist.Scenes[sceneIdx].Expressions {
					if migrateExpression(&hist.Scenes[sceneIdx].Expressions[exprIdx], recalculate, calculator) {
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
func migrateExpression(exp *notebook.LearningHistoryExpression, recalculate bool, calculator notebook.IntervalCalculator) bool {
	modified := false

	if recalculate {
		recalculateMetrics(exp, calculator)
		modified = true
		return modified
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

// recalculateMetrics forces a full recalculation of IntervalDays for all logs
func recalculateMetrics(exp *notebook.LearningHistoryExpression, calculator notebook.IntervalCalculator) {
	_, exp.LearnedLogs = calculator.RecalculateAll(exp.LearnedLogs)
	_, exp.ReverseLogs = calculator.RecalculateAll(exp.ReverseLogs)
}

// RecalculateIntervals recalculates intervals for all learning history files
// using the configured algorithm.
func RecalculateIntervals(learningNotesDir string, algorithm string, fixedIntervals []int) error {
	calculator := notebook.NewIntervalCalculator(algorithm, fixedIntervals)

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
					if recalculateExpression(&hist.Expressions[exprIdx], calculator) {
						modified = true
					}
				}
				continue
			}

			for sceneIdx := range hist.Scenes {
				for exprIdx := range hist.Scenes[sceneIdx].Expressions {
					if recalculateExpression(&hist.Scenes[sceneIdx].Expressions[exprIdx], calculator) {
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
			fmt.Printf("Recalculated: %s\n", notebookName)
		}
	}

	fmt.Printf("Recalculation complete (algorithm: %s)\n", algorithm)
	return nil
}

// recalculateExpression recalculates intervals for a single expression using the given calculator.
func recalculateExpression(exp *notebook.LearningHistoryExpression, calculator notebook.IntervalCalculator) bool {
	modified := false
	if len(exp.LearnedLogs) > 0 {
		newEF, newLogs := calculator.RecalculateAll(exp.LearnedLogs)
		exp.EasinessFactor = newEF
		exp.LearnedLogs = newLogs
		modified = true
	}
	if len(exp.ReverseLogs) > 0 {
		newEF, newLogs := calculator.RecalculateAll(exp.ReverseLogs)
		exp.ReverseEasinessFactor = newEF
		exp.ReverseLogs = newLogs
		modified = true
	}
	return modified
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
