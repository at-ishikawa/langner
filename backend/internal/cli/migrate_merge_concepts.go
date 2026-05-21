// Package cli — destructive migration that merges per-member learning
// history entries into the head expression of each definitions concept.
package cli

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// MergeConcepts walks the configured definitions books for any `concepts:`
// declarations and merges every non-head member's learning history entry
// into the head's entry within the same scene of the same notebook's
// learning-history YAML file.
//
// The operation is destructive and one-way: non-head member entries are
// removed from the YAML. The head ends up holding the union of every
// member's LearnedLogs/ReverseLogs/EtymologyBreakdownLogs/EtymologyAssemblyLogs
// (sorted newest-first), the union of every member's SkippedAt
// timestamps (earliest non-zero wins per quiz type), and a rewritten
// interval_days on its newest log per quiz-type — set to the min across
// every member's newest interval for that quiz type (members with no
// logs of that type are skipped, not treated as zero).
//
// Callers are expected to have committed the learning_notes directory to
// version control before running so they can revert if needed.
//
// dryRun=true reports what would change without writing anything.
func MergeConcepts(learningNotesDir string, definitionsDirs []string, dryRun bool) error {
	if learningNotesDir == "" {
		return fmt.Errorf("learning notes directory is required")
	}
	if len(definitionsDirs) == 0 {
		return fmt.Errorf("at least one definitions directory is required")
	}

	idxByBook, err := loadConceptIndexesByBook(definitionsDirs)
	if err != nil {
		return fmt.Errorf("load definitions concepts: %w", err)
	}
	if len(idxByBook) == 0 {
		fmt.Println("No definitions concepts found — nothing to merge.")
		return nil
	}

	histories, err := notebook.NewLearningHistories(learningNotesDir)
	if err != nil {
		return fmt.Errorf("load learning histories: %w", err)
	}

	notebookNames := make([]string, 0, len(idxByBook))
	for name := range idxByBook {
		notebookNames = append(notebookNames, name)
	}
	sort.Strings(notebookNames)

	for _, name := range notebookNames {
		idx := idxByBook[name]
		historyList, ok := histories[name]
		if !ok {
			fmt.Printf("Skipping %q: no learning history file\n", name)
			continue
		}
		merged := mergeConceptsInHistory(historyList, idx)
		if merged == 0 {
			fmt.Printf("No merges needed in %q\n", name)
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] Would merge %d concept member(s) into head(s) in %q\n", merged, name)
			continue
		}
		path := filepath.Join(learningNotesDir, name+".yml")
		if err := notebook.WriteYamlFile(path, historyList); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		fmt.Printf("Merged %d concept member(s) in %q\n", merged, name)
	}

	if dryRun {
		fmt.Println("Dry-run complete; no files modified.")
	} else {
		fmt.Println("Merge complete!")
	}
	return nil
}

// conceptBookIndex maps expressions to their concept head within one book.
type conceptBookIndex struct {
	toHead  map[string]string // member expression -> head expression
	members map[string][]string
}

func loadConceptIndexesByBook(definitionsDirs []string) (map[string]*conceptBookIndex, error) {
	_, raw, _, err := notebook.NewDefinitionsMap(definitionsDirs)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*conceptBookIndex)
	for bookID, defs := range raw {
		idx := &conceptBookIndex{
			toHead:  make(map[string]string),
			members: make(map[string][]string),
		}
		for _, def := range defs {
			for _, c := range def.Concepts {
				if c.Head == "" || len(c.Expressions) == 0 {
					continue
				}
				if _, exists := idx.members[c.Head]; !exists {
					idx.members[c.Head] = append([]string(nil), c.Expressions...)
				}
				for _, m := range c.Expressions {
					if m == "" {
						continue
					}
					if _, already := idx.toHead[m]; already {
						continue
					}
					idx.toHead[m] = c.Head
				}
			}
		}
		if len(idx.toHead) > 0 {
			result[bookID] = idx
		}
	}
	return result, nil
}

// mergeConceptsInHistory merges concept members into heads across every
// scene and every flashcard-style top-level expressions block in
// historyList. Returns the count of non-head member entries removed.
// historyList is mutated in place.
func mergeConceptsInHistory(historyList []notebook.LearningHistory, idx *conceptBookIndex) int {
	total := 0
	for histIdx := range historyList {
		hist := &historyList[histIdx]

		if len(hist.Expressions) > 0 {
			n, merged := mergeConceptsInExpressionList(hist.Expressions, idx)
			hist.Expressions = merged
			total += n
		}

		for sceneIdx := range hist.Scenes {
			n, merged := mergeConceptsInExpressionList(hist.Scenes[sceneIdx].Expressions, idx)
			hist.Scenes[sceneIdx].Expressions = merged
			total += n
		}
	}
	return total
}

// mergeConceptsInExpressionList merges concept members into heads within
// a single expression list (one scene's worth of entries, or a flashcard
// notebook's top-level list). Returns the count of removed entries and
// the new slice.
//
// The merge is destructive: each non-head member entry contributes its
// logs and skip timestamps to the head's entry and is then dropped.
func mergeConceptsInExpressionList(
	in []notebook.LearningHistoryExpression, idx *conceptBookIndex,
) (int, []notebook.LearningHistoryExpression) {
	if len(in) == 0 || idx == nil {
		return 0, in
	}

	// Find heads present in the list (might be missing if only members
	// have logs so far — in that case we promote the first member).
	headIndex := make(map[string]int)
	for i, e := range in {
		if _, isHead := idx.members[e.Expression]; isHead {
			headIndex[e.Expression] = i
		}
	}

	out := make([]notebook.LearningHistoryExpression, 0, len(in))
	removed := 0

	for _, e := range in {
		head, isMember := idx.toHead[e.Expression]
		if !isMember {
			out = append(out, e)
			continue
		}
		if e.Expression == head {
			out = append(out, e)
			continue
		}

		// Non-head member.
		_, headPresent := headIndex[head]
		if !headPresent {
			// Head has no entry yet — promote this member into the head
			// slot by renaming, and register it so subsequent members of
			// the same concept merge into the promoted row.
			e.Expression = head
			out = append(out, e)
			headIndex[head] = -1 // signal "head now exists in out"
			removed++            // count as a modification so the file gets rewritten
			continue
		}
		// Merge into the existing head entry already in `out`.
		mergeMemberInto(out, head, e)
		removed++
	}

	// After merging, recompute the "min latest interval_days per
	// quiz-type" override on heads that had at least one member merged.
	for headName := range idx.members {
		for i := range out {
			if out[i].Expression != headName {
				continue
			}
			rewriteLatestIntervalToMin(&out[i], idx.members[headName], in)
			break
		}
	}

	return removed, out
}

// mergeMemberInto folds the member's logs and skip map into the head's
// entry in out. The head entry is located by name in out. Logs are
// concatenated and re-sorted newest-first; skip timestamps are unioned
// with earliest-non-zero winning on a per-quiz-type collision.
func mergeMemberInto(out []notebook.LearningHistoryExpression, head string, member notebook.LearningHistoryExpression) {
	for i := range out {
		if out[i].Expression != head {
			continue
		}
		out[i].LearnedLogs = mergeLogsNewestFirst(out[i].LearnedLogs, member.LearnedLogs)
		out[i].ReverseLogs = mergeLogsNewestFirst(out[i].ReverseLogs, member.ReverseLogs)
		out[i].EtymologyBreakdownLogs = mergeLogsNewestFirst(out[i].EtymologyBreakdownLogs, member.EtymologyBreakdownLogs)
		out[i].EtymologyAssemblyLogs = mergeLogsNewestFirst(out[i].EtymologyAssemblyLogs, member.EtymologyAssemblyLogs)
		out[i].SkippedAt = mergeSkippedAt(out[i].SkippedAt, member.SkippedAt)
		return
	}
}

// mergeLogsNewestFirst concatenates a and b, then sorts by LearnedAt
// descending so the result respects the chronological-newest-first
// invariant the rest of the codebase relies on.
func mergeLogsNewestFirst(a, b []notebook.LearningRecord) []notebook.LearningRecord {
	if len(b) == 0 {
		return a
	}
	if len(a) == 0 {
		out := append([]notebook.LearningRecord(nil), b...)
		sort.Slice(out, func(i, j int) bool { return out[i].LearnedAt.After(out[j].LearnedAt.Time) })
		return out
	}
	out := append([]notebook.LearningRecord(nil), a...)
	out = append(out, b...)
	sort.Slice(out, func(i, j int) bool { return out[i].LearnedAt.After(out[j].LearnedAt.Time) })
	return out
}

// mergeSkippedAt unions two SkippedAt maps. On per-quiz-type collisions
// the earliest non-zero timestamp wins (the user skipped this earlier).
// Both inputs may be nil.
func mergeSkippedAt(a, b notebook.SkippedAtMap) notebook.SkippedAtMap {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(notebook.SkippedAtMap, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if existing, ok := out[k]; ok {
			if existing == "" {
				out[k] = v
				continue
			}
			if v == "" {
				continue
			}
			if v < existing {
				out[k] = v
			}
			continue
		}
		out[k] = v
	}
	return out
}

// rewriteLatestIntervalToMin overrides the head's newest log's
// interval_days, per quiz-type, to the min across every member's newest
// pre-merge interval_days for that quiz-type. Members with no logs of
// that type are skipped (they don't contribute zero to the minimum).
// pre is the pre-merge expression list — used so we can read each
// member's original latest interval_days without depending on the merged
// head's already-folded logs.
func rewriteLatestIntervalToMin(head *notebook.LearningHistoryExpression, members []string, pre []notebook.LearningHistoryExpression) {
	rewrite := func(getter func(*notebook.LearningHistoryExpression) *[]notebook.LearningRecord) {
		logs := getter(head)
		if logs == nil || len(*logs) == 0 {
			return
		}
		// Sorted newest-first already (mergeLogsNewestFirst), but guard.
		sort.Slice(*logs, func(i, j int) bool { return (*logs)[i].LearnedAt.After((*logs)[j].LearnedAt.Time) })
		minVal := (*logs)[0].IntervalDays
		// Walk pre to find each member's latest interval_days.
		for _, m := range members {
			for i := range pre {
				if pre[i].Expression != m {
					continue
				}
				memLogs := getter(&pre[i])
				if memLogs == nil || len(*memLogs) == 0 {
					break
				}
				sort.Slice(*memLogs, func(a, b int) bool { return (*memLogs)[a].LearnedAt.After((*memLogs)[b].LearnedAt.Time) })
				latest := (*memLogs)[0].IntervalDays
				if latest > 0 && (minVal <= 0 || latest < minVal) {
					minVal = latest
				}
				break
			}
		}
		if minVal > 0 {
			(*logs)[0].IntervalDays = minVal
		}
	}
	rewrite(func(e *notebook.LearningHistoryExpression) *[]notebook.LearningRecord { return &e.LearnedLogs })
	rewrite(func(e *notebook.LearningHistoryExpression) *[]notebook.LearningRecord { return &e.ReverseLogs })
	rewrite(func(e *notebook.LearningHistoryExpression) *[]notebook.LearningRecord { return &e.EtymologyBreakdownLogs })
	rewrite(func(e *notebook.LearningHistoryExpression) *[]notebook.LearningRecord { return &e.EtymologyAssemblyLogs })
}
