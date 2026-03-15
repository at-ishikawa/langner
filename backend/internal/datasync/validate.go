package datasync

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// ValidationMismatch represents a single mismatch found during validation.
type ValidationMismatch struct {
	Category string
	Message  string
}

func (m ValidationMismatch) String() string {
	return fmt.Sprintf("[%s] %s", m.Category, m.Message)
}

// NotebookStats holds per-notebook statistics.
type NotebookStats struct {
	DefinitionCount int
}

// LearningExpressionStats holds learning log counts for an expression.
type LearningExpressionStats struct {
	LearnedLogCount int
	ReverseLogCount int
}

// DataStats holds aggregated statistics for a dataset.
type DataStats struct {
	TotalNotes      int
	NotebookStats   map[string]*NotebookStats                      // notebookID -> stats
	LearningStats   map[string]map[string]*LearningExpressionStats // notebookID -> expression -> stats
	DictionaryCount int
}

// ValidateResult holds the results of a round-trip validation.
type ValidateResult struct {
	Mismatches    []ValidationMismatch
	SourceStats   DataStats
	ExportedStats DataStats
}

// HasMismatches returns true if any mismatches were found.
func (r *ValidateResult) HasMismatches() bool {
	return len(r.Mismatches) > 0
}

// buildNoteStats aggregates note records into per-notebook statistics.
func buildNoteStats(notes []notebook.NoteRecord) DataStats {
	stats := DataStats{
		NotebookStats: make(map[string]*NotebookStats),
	}

	type nk struct{ usage, entry string }
	uniqueNotes := make(map[nk]bool)

	for _, rec := range notes {
		uniqueNotes[nk{strings.ToLower(rec.Usage), strings.ToLower(rec.Entry)}] = true
		for _, nn := range rec.NotebookNotes {
			ns, ok := stats.NotebookStats[nn.NotebookID]
			if !ok {
				ns = &NotebookStats{}
				stats.NotebookStats[nn.NotebookID] = ns
			}
			ns.DefinitionCount++
		}
	}

	stats.TotalNotes = len(uniqueNotes)
	return stats
}

// buildLearningStats aggregates learning expressions by notebook.
func buildLearningStats(learningByNotebook map[string][]notebook.LearningHistoryExpression) map[string]map[string]*LearningExpressionStats {
	result := make(map[string]map[string]*LearningExpressionStats)

	for nbID, expressions := range learningByNotebook {
		exprStats := make(map[string]*LearningExpressionStats)
		for _, expr := range expressions {
			es := exprStats[expr.Expression]
			if es == nil {
				es = &LearningExpressionStats{}
				exprStats[expr.Expression] = es
			}
			es.LearnedLogCount += len(expr.LearnedLogs)
			es.ReverseLogCount += len(expr.ReverseLogs)
		}
		result[nbID] = exprStats
	}

	return result
}

// ValidateRoundTrip compares source and exported data to validate import/export correctness.
func ValidateRoundTrip(
	sourceNotes []notebook.NoteRecord,
	exportedNotes []notebook.NoteRecord,
	sourceLearningByNotebook map[string][]notebook.LearningHistoryExpression,
	exportedLearningByNotebook map[string][]notebook.LearningHistoryExpression,
	sourceDictCount int,
	exportedDictCount int,
	writer io.Writer,
) *ValidateResult {
	result := &ValidateResult{
		SourceStats:   buildNoteStats(sourceNotes),
		ExportedStats: buildNoteStats(exportedNotes),
	}
	result.SourceStats.DictionaryCount = sourceDictCount
	result.ExportedStats.DictionaryCount = exportedDictCount
	result.SourceStats.LearningStats = buildLearningStats(sourceLearningByNotebook)
	result.ExportedStats.LearningStats = buildLearningStats(exportedLearningByNotebook)

	// Compare total notes
	if result.SourceStats.TotalNotes != result.ExportedStats.TotalNotes {
		result.Mismatches = append(result.Mismatches, ValidationMismatch{
			Category: "notes",
			Message: fmt.Sprintf("total unique note count mismatch: source=%d, exported=%d",
				result.SourceStats.TotalNotes, result.ExportedStats.TotalNotes),
		})
	}

	// Compare notebook count
	if len(result.SourceStats.NotebookStats) != len(result.ExportedStats.NotebookStats) {
		result.Mismatches = append(result.Mismatches, ValidationMismatch{
			Category: "notebooks",
			Message: fmt.Sprintf("notebook count mismatch: source=%d, exported=%d",
				len(result.SourceStats.NotebookStats), len(result.ExportedStats.NotebookStats)),
		})
	}

	// Compare per-notebook definition counts
	allNotebookIDs := mergeStringKeys(result.SourceStats.NotebookStats, result.ExportedStats.NotebookStats)
	sort.Strings(allNotebookIDs)

	for _, nbID := range allNotebookIDs {
		srcCount, expCount := 0, 0
		if ns := result.SourceStats.NotebookStats[nbID]; ns != nil {
			srcCount = ns.DefinitionCount
		}
		if ns := result.ExportedStats.NotebookStats[nbID]; ns != nil {
			expCount = ns.DefinitionCount
		}
		if srcCount != expCount {
			result.Mismatches = append(result.Mismatches, ValidationMismatch{
				Category: "notebook_definitions",
				Message: fmt.Sprintf("notebook %q definition count mismatch: source=%d, exported=%d",
					nbID, srcCount, expCount),
			})
		}
	}

	// Compare learning logs per expression per notebook
	allLearningNBIDs := mergeStringKeys(result.SourceStats.LearningStats, result.ExportedStats.LearningStats)
	sort.Strings(allLearningNBIDs)

	for _, nbID := range allLearningNBIDs {
		srcExprs := result.SourceStats.LearningStats[nbID]
		expExprs := result.ExportedStats.LearningStats[nbID]
		if srcExprs == nil {
			srcExprs = make(map[string]*LearningExpressionStats)
		}
		if expExprs == nil {
			expExprs = make(map[string]*LearningExpressionStats)
		}

		allExprs := mergeStringKeys(srcExprs, expExprs)
		sort.Strings(allExprs)

		for _, expr := range allExprs {
			srcLearned, srcReverse := 0, 0
			if es := srcExprs[expr]; es != nil {
				srcLearned = es.LearnedLogCount
				srcReverse = es.ReverseLogCount
			}
			expLearned, expReverse := 0, 0
			if es := expExprs[expr]; es != nil {
				expLearned = es.LearnedLogCount
				expReverse = es.ReverseLogCount
			}

			if srcLearned != expLearned {
				result.Mismatches = append(result.Mismatches, ValidationMismatch{
					Category: "learning_logs",
					Message: fmt.Sprintf("notebook %q expression %q learned log count mismatch: source=%d, exported=%d",
						nbID, expr, srcLearned, expLearned),
				})
			}
			if srcReverse != expReverse {
				result.Mismatches = append(result.Mismatches, ValidationMismatch{
					Category: "learning_logs",
					Message: fmt.Sprintf("notebook %q expression %q reverse log count mismatch: source=%d, exported=%d",
						nbID, expr, srcReverse, expReverse),
				})
			}
		}
	}

	// Compare dictionary counts
	if sourceDictCount != exportedDictCount {
		result.Mismatches = append(result.Mismatches, ValidationMismatch{
			Category: "dictionary",
			Message: fmt.Sprintf("dictionary entry count mismatch: source=%d, exported=%d",
				sourceDictCount, exportedDictCount),
		})
	}

	printValidationSummary(writer, result)
	return result
}

func printValidationSummary(writer io.Writer, result *ValidateResult) {
	_, _ = fmt.Fprintf(writer, "\n=== Round-Trip Validation ===\n")

	_, _ = fmt.Fprintf(writer, "\nNotes:\n")
	_, _ = fmt.Fprintf(writer, "  Source:   %d unique notes across %d notebooks\n",
		result.SourceStats.TotalNotes, len(result.SourceStats.NotebookStats))
	_, _ = fmt.Fprintf(writer, "  Exported: %d unique notes across %d notebooks\n",
		result.ExportedStats.TotalNotes, len(result.ExportedStats.NotebookStats))

	_, _ = fmt.Fprintf(writer, "\nPer-Notebook Definition Counts:\n")
	allNBIDs := mergeStringKeys(result.SourceStats.NotebookStats, result.ExportedStats.NotebookStats)
	sort.Strings(allNBIDs)
	for _, nbID := range allNBIDs {
		srcCount, expCount := 0, 0
		if ns := result.SourceStats.NotebookStats[nbID]; ns != nil {
			srcCount = ns.DefinitionCount
		}
		if ns := result.ExportedStats.NotebookStats[nbID]; ns != nil {
			expCount = ns.DefinitionCount
		}
		marker := " "
		if srcCount != expCount {
			marker = "!"
		}
		_, _ = fmt.Fprintf(writer, "  %s %-40s source=%-4d exported=%-4d\n", marker, nbID, srcCount, expCount)
	}

	// Learning stats summary per notebook
	_, _ = fmt.Fprintf(writer, "\nPer-Notebook Learning Logs:\n")
	allLearningNBIDs2 := mergeStringKeys(result.SourceStats.LearningStats, result.ExportedStats.LearningStats)
	sort.Strings(allLearningNBIDs2)

	totalSrcLogs, totalExpLogs := 0, 0
	for _, nbID := range allLearningNBIDs2 {
		srcExprs := result.SourceStats.LearningStats[nbID]
		expExprs := result.ExportedStats.LearningStats[nbID]

		srcLogs, expLogs, srcExprCount, expExprCount := 0, 0, 0, 0
		if srcExprs != nil {
			srcExprCount = len(srcExprs)
			for _, es := range srcExprs {
				srcLogs += es.LearnedLogCount + es.ReverseLogCount
			}
		}
		if expExprs != nil {
			expExprCount = len(expExprs)
			for _, es := range expExprs {
				expLogs += es.LearnedLogCount + es.ReverseLogCount
			}
		}
		totalSrcLogs += srcLogs
		totalExpLogs += expLogs

		logsMarker := " "
		if srcLogs != expLogs {
			logsMarker = "!"
		}
		exprMarker := " "
		if srcExprCount != expExprCount {
			exprMarker = "!"
		}
		_, _ = fmt.Fprintf(writer, "  %s %-40s expressions: source=%-4d exported=%-4d  %slogs: source=%-5d exported=%-5d\n",
			logsMarker, nbID, srcExprCount, expExprCount, exprMarker, srcLogs, expLogs)
	}
	_, _ = fmt.Fprintf(writer, "\nLearning Logs Total:\n")
	_, _ = fmt.Fprintf(writer, "  Source:   %d total logs\n", totalSrcLogs)
	_, _ = fmt.Fprintf(writer, "  Exported: %d total logs\n", totalExpLogs)

	_, _ = fmt.Fprintf(writer, "\nDictionary:\n")
	_, _ = fmt.Fprintf(writer, "  Source:   %d entries\n", result.SourceStats.DictionaryCount)
	_, _ = fmt.Fprintf(writer, "  Exported: %d entries\n", result.ExportedStats.DictionaryCount)

	_, _ = fmt.Fprintf(writer, "\n=== Result ===\n")
	if !result.HasMismatches() {
		_, _ = fmt.Fprintf(writer, "All validations passed!\n")
	} else {
		_, _ = fmt.Fprintf(writer, "Found %d mismatch(es):\n", len(result.Mismatches))
		for _, m := range result.Mismatches {
			_, _ = fmt.Fprintf(writer, "  - %s\n", m)
		}
	}
	_, _ = fmt.Fprintf(writer, "\n")
}

// mergeStringKeys returns the union of keys from two maps.
func mergeStringKeys[V any](a, b map[string]V) []string {
	seen := make(map[string]bool)
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	return keys
}
