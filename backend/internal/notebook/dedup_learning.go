package notebook

import (
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
		mergedAny := false
		for _, j := range g.idlessIdx {
			src := exprs[j]
			target.LearnedLogs = append(target.LearnedLogs, src.LearnedLogs...)
			target.ReverseLogs = append(target.ReverseLogs, src.ReverseLogs...)
			target.EtymologyBreakdownLogs = append(target.EtymologyBreakdownLogs, src.EtymologyBreakdownLogs...)
			target.EtymologyAssemblyLogs = append(target.EtymologyAssemblyLogs, src.EtymologyAssemblyLogs...)
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
			mergedAny = true
		}
		if mergedAny {
			// Replay each series oldest->newest so the merged-in logs get an
			// interval computed against the FULL combined history, not the
			// empty history of the forked entry they were written on (a
			// forked "correct" answer had a first-attempt interval; the real
			// streak yields a much longer one). RecalculateAll preserves
			// manual OverrideInterval entries and returns storage order.
			_, target.LearnedLogs = calculator.RecalculateAll(target.LearnedLogs)
			_, target.ReverseLogs = calculator.RecalculateAll(target.ReverseLogs)
			_, target.EtymologyBreakdownLogs = calculator.RecalculateAll(target.EtymologyBreakdownLogs)
			_, target.EtymologyAssemblyLogs = calculator.RecalculateAll(target.EtymologyAssemblyLogs)
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
