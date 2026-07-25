package notebook

import (
	"sort"
	"strings"
)

// MergeIDLessDuplicates repairs learning histories where a quiz write forked a
// new id-less entry next to an existing id-bearing entry for the same
// expression — the pre-fix bug where an id-less write (a concept card
// redirected to its head, or any write whose card lost its id) could not match
// the migrated, id-bearing series and created a duplicate instead.
//
// Within each scene (and the flashcard-level Expressions), when a group of
// same-expression entries contains exactly one id-bearing entry and one or
// more id-less entries, each id-less entry's logs are merged into the
// id-bearing entry (kept newest-first) and the id-less entry is dropped.
// Groups with two or more distinct ids (genuine homographs) or with no
// id-bearing entry (untouched legacy data) are left alone. Returns the number
// of id-less entries merged away.
func MergeIDLessDuplicates(histories []LearningHistory, calculator IntervalCalculator) int {
	if calculator == nil {
		calculator = &SM2Calculator{}
	}
	total := 0
	for hi := range histories {
		total += mergeIDLessInList(&histories[hi].Expressions, calculator)
		for si := range histories[hi].Scenes {
			total += mergeIDLessInList(&histories[hi].Scenes[si].Expressions, calculator)
		}
	}
	return total
}

func mergeIDLessInList(list *[]LearningHistoryExpression, calculator IntervalCalculator) int {
	exprs := *list
	type group struct {
		idIdx     []int
		idlessIdx []int
	}
	groups := make(map[string]*group)
	var order []string
	for i := range exprs {
		key := strings.ToLower(strings.TrimSpace(exprs[i].Expression))
		g := groups[key]
		if g == nil {
			g = &group{}
			groups[key] = g
			order = append(order, key)
		}
		if exprs[i].ID != "" {
			g.idIdx = append(g.idIdx, i)
		} else {
			g.idlessIdx = append(g.idlessIdx, i)
		}
	}

	remove := make(map[int]bool)
	for _, key := range order {
		g := groups[key]
		// Only repair the unambiguous case: exactly one id target + at least
		// one id-less fork. Homographs (>1 id) and pure-legacy groups (0 ids)
		// are left untouched.
		if len(g.idIdx) != 1 || len(g.idlessIdx) == 0 {
			continue
		}
		target := &exprs[g.idIdx[0]]
		for _, j := range g.idlessIdx {
			src := exprs[j]
			target.LearnedLogs = mergeSeries(target.LearnedLogs, src.LearnedLogs, calculator)
			target.ReverseLogs = mergeSeries(target.ReverseLogs, src.ReverseLogs, calculator)
			target.EtymologyBreakdownLogs = mergeSeries(target.EtymologyBreakdownLogs, src.EtymologyBreakdownLogs, calculator)
			target.EtymologyAssemblyLogs = mergeSeries(target.EtymologyAssemblyLogs, src.EtymologyAssemblyLogs, calculator)
			for qt, at := range src.SkippedAt {
				if at == "" {
					continue
				}
				if target.SkippedAt == nil {
					target.SkippedAt = make(SkippedAtMap)
				}
				if target.SkippedAt[qt] == "" {
					target.SkippedAt[qt] = at
				}
			}
			remove[j] = true
		}
	}

	if len(remove) == 0 {
		return 0
	}
	kept := make([]LearningHistoryExpression, 0, len(exprs)-len(remove))
	for i := range exprs {
		if remove[i] {
			continue
		}
		kept = append(kept, exprs[i])
	}
	*list = kept
	return len(remove)
}

// mergeSeries folds the forked entry's logs (added) into the surviving
// series (existing) and computes an interval for each ADDED log against the
// real history — the empty history of the fork gave a correct answer a
// first-attempt interval; the true streak yields a much longer one. Existing
// (historical) intervals are left exactly as stored: a full RecalculateAll
// replay would rewrite them, and this data's stored intervals diverge from a
// fresh replay, so only the newly merged-in logs are recomputed.
func mergeSeries(existing, added []LearningRecord, calc IntervalCalculator) []LearningRecord {
	if len(added) == 0 {
		return existing
	}
	merged := make([]LearningRecord, len(existing), len(existing)+len(added))
	copy(merged, existing)
	for _, a := range added {
		if a.OverrideInterval == 0 {
			prior := make([]LearningRecord, 0, len(merged))
			for _, m := range merged {
				if m.LearnedAt.Before(a.LearnedAt.Time) {
					prior = append(prior, m)
				}
			}
			sort.SliceStable(prior, func(i, j int) bool { return prior[i].LearnedAt.After(prior[j].LearnedAt.Time) })
			a.IntervalDays, _ = calc.NextIntervalForWrite(prior, a)
		}
		merged = append(merged, a)
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].LearnedAt.After(merged[j].LearnedAt.Time) })
	return merged
}
