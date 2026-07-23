package notebook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeIDLessDuplicates(t *testing.T) {
	old2 := NewDate(time.Now().Add(-60 * 24 * time.Hour))
	old1 := NewDate(time.Now().Add(-30 * 24 * time.Hour))
	today := NewDate(time.Now())

	histories := []LearningHistory{{
		Metadata: LearningHistoryMetadata{Title: "wpme"},
		Scenes: []LearningScene{{
			Metadata: LearningSceneMetadata{Title: "Session 5"},
			Expressions: []LearningHistoryExpression{
				// Forked pair: the migrated id-bearing entry carries a real
				// correct streak; the id-less fork a quiz created today holds a
				// correct answer whose interval was computed as a first attempt
				// (interval_days: 1) because the fork had no prior history.
				{Expression: "taxidermy", ID: "taxidermy", LearnedLogs: []LearningRecord{
					{Status: LearnedStatusUnderstood, LearnedAt: old1, Quality: 4, IntervalDays: 6},
					{Status: LearnedStatusUnderstood, LearnedAt: old2, Quality: 4, IntervalDays: 1},
				}},
				{Expression: "taxidermy", LearnedLogs: []LearningRecord{
					{Status: LearnedStatusUnderstood, LearnedAt: today, Quality: 4, IntervalDays: 1},
				}},
				// Genuine homograph: two ids — must be left untouched.
				{Expression: "bank", ID: "bank-money"},
				{Expression: "bank", ID: "bank-river"},
				// Pure legacy id-less with no id-bearing sibling — untouched.
				{Expression: "orphan"},
			},
		}},
	}}

	merged := MergeIDLessDuplicates(histories, nil)
	assert.Equal(t, 1, merged, "only the taxidermy fork is merged")

	exprs := histories[0].Scenes[0].Expressions
	require.Len(t, exprs, 4, "the id-less taxidermy duplicate is removed")

	var tax *LearningHistoryExpression
	banks, orphans := 0, 0
	for i := range exprs {
		switch exprs[i].Expression {
		case "taxidermy":
			tax = &exprs[i]
		case "bank":
			banks++
		case "orphan":
			orphans++
		}
	}
	require.NotNil(t, tax)
	assert.Equal(t, "taxidermy", tax.ID, "the surviving entry keeps its id")
	require.Len(t, tax.LearnedLogs, 3, "all logs are merged onto the id-bearing entry")
	assert.True(t, tax.LearnedLogs[0].LearnedAt.After(tax.LearnedLogs[1].LearnedAt.Time),
		"logs stay newest-first after the merge")
	// The merged-in (today's) log is the 3rd consecutive correct answer, so it
	// is recomputed against the real streak — well above the fork's
	// first-attempt value of 1.
	assert.Greater(t, tax.LearnedLogs[0].IntervalDays, 1,
		"the merged-in interval reflects the full streak, not the fork's 1")
	// The historical intervals must be left EXACTLY as stored — recomputing
	// the whole chain would rewrite the learner's real schedule.
	assert.Equal(t, 6, tax.LearnedLogs[1].IntervalDays, "historical interval is preserved")
	assert.Equal(t, 1, tax.LearnedLogs[2].IntervalDays, "historical interval is preserved")
	assert.Equal(t, 2, banks, "the homograph pair is left untouched")
	assert.Equal(t, 1, orphans, "a pure-legacy id-less entry is left untouched")
}
