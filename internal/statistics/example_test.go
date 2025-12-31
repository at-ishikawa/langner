package statistics_test

import (
	"fmt"

	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/statistics"
)

func ExampleCalculateStatistics() {
	// Load learning histories from a directory
	histories, err := notebook.NewLearningHistories("/path/to/learning/history")
	if err != nil {
		panic(err)
	}

	// Calculate statistics for all time (no filters)
	result := statistics.CalculateStatistics(histories, 0, 0)
	for _, stat := range result.Periods {
		fmt.Printf("Period %s: %d new words (%d unique), %d relearns (%d unique)\n",
			stat.Period, stat.NewWordsCount, stat.NewWordsUnique,
			stat.RelearnsCount, stat.RelearnsUnique)
	}

	// Print aggregate totals
	fmt.Printf("Total: %d new words (%d unique), %d relearns (%d unique)\n",
		result.Aggregate.NewWordsCount, result.Aggregate.NewWordsUnique,
		result.Aggregate.RelearnsCount, result.Aggregate.RelearnsUnique)
}
