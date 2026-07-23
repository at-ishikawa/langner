package notebook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeIDLessDuplicates(t *testing.T) {
	older := NewDate(time.Now().Add(-72 * time.Hour))
	newer := NewDate(time.Now().Add(-1 * time.Hour))

	histories := []LearningHistory{{
		Metadata: LearningHistoryMetadata{Title: "wpme"},
		Scenes: []LearningScene{{
			Metadata: LearningSceneMetadata{Title: "Session 5"},
			Expressions: []LearningHistoryExpression{
				// Forked pair: the migrated id-bearing entry (older log) and
				// the id-less duplicate a quiz created today (newer log).
				{Expression: "taxidermy", ID: "taxidermy", LearnedLogs: []LearningRecord{
					{Status: LearnedStatusUnderstood, LearnedAt: older, Quality: 4},
				}},
				{Expression: "taxidermy", LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: newer, Quality: 1},
				}},
				// Genuine homograph: two ids — must be left untouched.
				{Expression: "bank", ID: "bank-money"},
				{Expression: "bank", ID: "bank-river"},
				// Pure legacy id-less with no id-bearing sibling — untouched.
				{Expression: "orphan"},
			},
		}},
	}}

	merged := MergeIDLessDuplicates(histories)
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
	require.Len(t, tax.LearnedLogs, 2, "both logs are merged onto the id-bearing entry")
	assert.Equal(t, LearnedStatusMisunderstood, tax.LearnedLogs[0].Status, "newest log is first")
	assert.Equal(t, LearnedStatusUnderstood, tax.LearnedLogs[1].Status)
	assert.Equal(t, 2, banks, "the homograph pair is left untouched")
	assert.Equal(t, 1, orphans, "a pure-legacy id-less entry is left untouched")
}
