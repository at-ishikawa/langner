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
//
// DefinitionCount is incremented per unique
// (note, notebook_type, notebook_id, group) — the same key the
// notebook_notes schema enforces UNIQUE on. Source YAML can list the
// same note in multiple subgroups under the same group (e.g.
// "Episode 1 / Scene 1" and "Episode 1 / Scene 4"), but the import
// collapses those into a single notebook_notes row to satisfy the
// schema UNIQUE. Counting raw YAML occurrences would then report a
// drift the import is required to introduce — the round-trip
// comparison must use the schema-level key on both sides.
func buildNoteStats(notes []notebook.NoteRecord) DataStats {
	stats := DataStats{
		NotebookStats: make(map[string]*NotebookStats),
	}

	type nk struct{ usage, entry string }
	uniqueNotes := make(map[nk]bool)

	type nnk struct {
		usage, entry             string
		notebookType, notebookID string
		group                    string
	}

	// Global dedupe across ALL records: the importer's noteLookup is
	// case-insensitive and notes are uniqueified by (usage, entry), so
	// two source records that differ only in casing collapse into one
	// DB note. Counting them as separate NotebookNotes here would
	// double-count the same notebook_notes row. Different (usage,
	// entry) pairs still get counted independently even when their
	// (notebook, group) tuple matches.
	seen := make(map[nnk]bool)

	for _, rec := range notes {
		uniqueNotes[nk{strings.ToLower(rec.Usage), strings.ToLower(rec.Entry)}] = true

		for _, nn := range rec.NotebookNotes {
			k := nnk{
				usage:        strings.ToLower(rec.Usage),
				entry:        strings.ToLower(rec.Entry),
				notebookType: nn.NotebookType,
				notebookID:   nn.NotebookID,
				group:        nn.Group,
			}
			if seen[k] {
				continue
			}
			seen[k] = true
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
//
// Counts each record by its CANONICAL slot — the slot the YAML export
// path puts it in based on quiz_type — rather than the slot it
// happened to be parked in on disk. Older langner versions wrote
// etymology_freeform records inside etymology_breakdown_logs /
// etymology_assembly_logs, so a source YAML with those records reports
// 5 LearnedLogs while the round-trip export normalises them into
// LearnedLogs and reports 7. Routing on both sides by quiz_type makes
// the round-trip comparison apples-to-apples.
//
// The slot convention mirrors yaml_repository.go's buildExpression:
//   - reverse              → reverse slot
//   - etymology_breakdown  → etymology_breakdown slot (not counted)
//   - etymology_assembly   → etymology_assembly slot (not counted)
//   - everything else      → learned slot
func buildLearningStats(learningByNotebook map[string][]notebook.LearningHistoryExpression) map[string]map[string]*LearningExpressionStats {
	result := make(map[string]map[string]*LearningExpressionStats)

	tallyRecords := func(es *LearningExpressionStats, recs []notebook.LearningRecord, defaultSlot string) {
		for _, rec := range recs {
			qt := string(rec.QuizType)
			if qt == "" {
				qt = defaultSlot
			}
			switch qt {
			case string(notebook.QuizTypeReverse):
				es.ReverseLogCount++
			case string(notebook.QuizTypeEtymologyStandard),
				string(notebook.QuizTypeEtymologyReverse):
				// Counted under etymology slots, not learned/reverse.
				// validate doesn't surface those today.
			default:
				// notebook, freeform, etymology_freeform — all land in
				// learned_logs in the YAML convention.
				es.LearnedLogCount++
			}
		}
	}

	for nbID, expressions := range learningByNotebook {
		exprStats := make(map[string]*LearningExpressionStats)
		for _, expr := range expressions {
			// Lowercase the expression key so source and exported buckets
			// fold case variants together — the importer's noteLookup is
			// already case-insensitive, so the round-trip comparison must
			// match. Without this, the same word recorded as "All..." in
			// vocab YAML and "all..." in learning history shows up as two
			// distinct buckets each missing on the other side.
			key := strings.ToLower(strings.TrimSpace(expr.Expression))
			es := exprStats[key]
			if es == nil {
				es = &LearningExpressionStats{}
				exprStats[key] = es
			}
			tallyRecords(es, expr.LearnedLogs, string(notebook.QuizTypeNotebook))
			tallyRecords(es, expr.ReverseLogs, string(notebook.QuizTypeReverse))
			tallyRecords(es, expr.EtymologyBreakdownLogs, string(notebook.QuizTypeEtymologyStandard))
			tallyRecords(es, expr.EtymologyAssemblyLogs, string(notebook.QuizTypeEtymologyReverse))
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
