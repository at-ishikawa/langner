package cli

import (
	"fmt"

	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/statistics"
)

// RunAnalyzeReport displays learning statistics report from pre-loaded
// histories. The caller chooses the data source — CLI passes DB-backed
// histories so the report matches what the user did in the web UI;
// tests pass YAML-loaded histories for fixture coverage.
func RunAnalyzeReport(histories map[string][]notebook.LearningHistory, year, month int) error {
	// Calculate statistics
	result := statistics.CalculateStatistics(histories, year, month)

	// Display results
	if len(result.Periods) == 0 {
		fmt.Println("No learning records found for the specified period.")
		return nil
	}

	// Print header
	fmt.Println("Learning Statistics Report")
	fmt.Println("==========================")
	fmt.Println()
	fmt.Printf("%-10s  %-24s  %-24s\n", "Period", "New Words (Total/Unique)", "Relearns (Total/Unique)")
	fmt.Printf("%-10s  %-24s  %-24s\n", "------", "------------------------", "-----------------------")

	// Print each period
	for _, s := range result.Periods {
		fmt.Printf("%-10s  %-24s  %-24s\n",
			s.Period,
			fmt.Sprintf("%d / %d", s.NewWordsCount, s.NewWordsUnique),
			fmt.Sprintf("%d / %d", s.RelearnsCount, s.RelearnsUnique),
		)
	}

	// Print totals with global unique counts
	fmt.Println()
	fmt.Printf("%-10s  %-24s  %-24s\n",
		"Totals:",
		fmt.Sprintf("%d / %d", result.Aggregate.NewWordsCount, result.Aggregate.NewWordsUnique),
		fmt.Sprintf("%d / %d", result.Aggregate.RelearnsCount, result.Aggregate.RelearnsUnique),
	)

	return nil
}
