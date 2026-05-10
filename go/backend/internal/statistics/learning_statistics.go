package statistics

import (
	"fmt"
	"sort"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// LearningStatistics holds statistics for a time period
type LearningStatistics struct {
	Period         string // "2025-01" for monthly, "2025" for yearly
	NewWordsCount  int    // Total new word learns (first successful learn)
	NewWordsUnique int    // Unique expressions learned for first time
	RelearnsCount  int    // Total relearn events
	RelearnsUnique int    // Unique expressions relearned
}

// AggregateStatistics holds totals across all periods with global unique counts
type AggregateStatistics struct {
	NewWordsCount  int // Total new word learns across all periods
	NewWordsUnique int // Unique expressions learned (deduplicated across periods)
	RelearnsCount  int // Total relearn events across all periods
	RelearnsUnique int // Unique expressions relearned (deduplicated across periods)
}

// StatisticsResult holds both per-period and aggregate statistics
type StatisticsResult struct {
	Periods   []LearningStatistics
	Aggregate AggregateStatistics
}

// periodData tracks counts per period
type periodData struct {
	newWordsTotal  int
	newWordsUnique map[string]struct{}
	relearnsTotal  int
	relearnsUnique map[string]struct{}
}

// CalculateStatistics calculates learning statistics from learning histories.
// It accepts optional year and month filters (0 means no filter).
// A "new word" is counted when an expression first reaches "understood" or "usable" status.
// A "relearn" is counted for subsequent successful reviews (not "misunderstood").
func CalculateStatistics(histories map[string][]notebook.LearningHistory, year, month int) StatisticsResult {
	stats := make(map[string]*periodData)
	// Track global unique expressions across all periods
	globalNewWordsUnique := make(map[string]struct{})
	globalRelearnsUnique := make(map[string]struct{})

	for _, historyList := range histories {
		for _, history := range historyList {
			for _, scene := range history.Scenes {
				for _, expr := range scene.Expressions {
					processExpression(expr, history.Metadata.Title, scene.Metadata.Title, year, month, stats, globalNewWordsUnique, globalRelearnsUnique)
				}
			}
		}
	}

	return buildResult(stats, globalNewWordsUnique, globalRelearnsUnique)
}

// processExpression processes a single expression's learning logs
func processExpression(
	expr notebook.LearningHistoryExpression,
	notebookTitle, sceneTitle string,
	year, month int,
	stats map[string]*periodData,
	globalNewWordsUnique, globalRelearnsUnique map[string]struct{},
) {
	if len(expr.LearnedLogs) == 0 {
		return
	}

	// Create unique key for this expression
	exprKey := fmt.Sprintf("%s|%s|%s", notebookTitle, sceneTitle, expr.Expression)

	// LearnedLogs are stored newest first (prepended), so iterate in reverse to find first successful learn
	foundFirstSuccess := false

	for i := len(expr.LearnedLogs) - 1; i >= 0; i-- {
		log := expr.LearnedLogs[i]

		// Skip misunderstood - these don't count as learning
		if log.Status == notebook.LearnedStatusMisunderstood {
			continue
		}

		// Skip zero dates
		if log.LearnedAt.IsZero() {
			continue
		}

		logYear := log.LearnedAt.Year()
		logMonth := int(log.LearnedAt.Month())

		// Check if this log matches the filter
		if !matchesFilter(logYear, logMonth, year, month) {
			// Still track if this was the first success (for relearn logic)
			if !foundFirstSuccess {
				foundFirstSuccess = true
			}
			continue
		}

		period := fmt.Sprintf("%d-%02d", logYear, logMonth)
		ensurePeriodExists(stats, period)

		if !foundFirstSuccess {
			// This is the first successful learn
			foundFirstSuccess = true
			stats[period].newWordsTotal++
			stats[period].newWordsUnique[exprKey] = struct{}{}
			globalNewWordsUnique[exprKey] = struct{}{}
		} else {
			// This is a relearn
			stats[period].relearnsTotal++
			stats[period].relearnsUnique[exprKey] = struct{}{}
			globalRelearnsUnique[exprKey] = struct{}{}
		}
	}
}

func ensurePeriodExists(stats map[string]*periodData, period string) {
	if stats[period] == nil {
		stats[period] = &periodData{
			newWordsUnique: make(map[string]struct{}),
			relearnsUnique: make(map[string]struct{}),
		}
	}
}

func matchesFilter(logYear, logMonth, filterYear, filterMonth int) bool {
	if filterYear == 0 {
		return true
	}
	if logYear != filterYear {
		return false
	}
	if filterMonth == 0 {
		return true
	}
	return logMonth == filterMonth
}

func buildResult(stats map[string]*periodData, globalNewWordsUnique, globalRelearnsUnique map[string]struct{}) StatisticsResult {
	periods := make([]LearningStatistics, 0, len(stats))

	var totalNewWords, totalRelearns int
	for period, data := range stats {
		periods = append(periods, LearningStatistics{
			Period:         period,
			NewWordsCount:  data.newWordsTotal,
			NewWordsUnique: len(data.newWordsUnique),
			RelearnsCount:  data.relearnsTotal,
			RelearnsUnique: len(data.relearnsUnique),
		})
		totalNewWords += data.newWordsTotal
		totalRelearns += data.relearnsTotal
	}

	// Sort by period descending (newest first)
	sort.Slice(periods, func(i, j int) bool {
		return periods[i].Period > periods[j].Period
	})

	return StatisticsResult{
		Periods: periods,
		Aggregate: AggregateStatistics{
			NewWordsCount:  totalNewWords,
			NewWordsUnique: len(globalNewWordsUnique),
			RelearnsCount:  totalRelearns,
			RelearnsUnique: len(globalRelearnsUnique),
		},
	}
}
