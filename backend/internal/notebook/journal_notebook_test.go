package notebook

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateJournalEntries(t *testing.T) {
	tests := []struct {
		name         string
		entries      []JournalEntry
		wantErrCount int
	}{
		{
			name:         "valid",
			entries:      []JournalEntry{{ID: "ep1", Text: "some prose"}},
			wantErrCount: 0,
		},
		{
			name:         "missing id and text",
			entries:      []JournalEntry{{}},
			wantErrCount: 2,
		},
		{
			name:         "duplicate id",
			entries:      []JournalEntry{{ID: "ep1", Text: "a"}, {ID: "ep1", Text: "b"}},
			wantErrCount: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Len(t, ValidateJournalEntries(tt.entries, "test.yml"), tt.wantErrCount)
		})
	}
}

func TestValidateJournalCorrections(t *testing.T) {
	text := "the John called me and he suggested to play a game"
	tests := []struct {
		name          string
		set           JournalCorrections
		wantErrCount  int
		wantErrSubstr string
	}{
		{
			name: "valid",
			set: JournalCorrections{ID: "ep1", Corrections: []Correction{
				{Line: 1, Incorrect: "the John", Correct: "John"},
				{Line: 1, Incorrect: "suggested to play", Correct: "suggested playing"},
			}},
			wantErrCount: 0,
		},
		{
			name:          "incorrect span not in text",
			set:           JournalCorrections{ID: "ep1", Corrections: []Correction{{Line: 1, Incorrect: "nonexistent", Correct: "x"}}},
			wantErrCount:  1,
			wantErrSubstr: "not found in post text",
		},
		{
			name:          "correct equals incorrect",
			set:           JournalCorrections{ID: "ep1", Corrections: []Correction{{Line: 1, Incorrect: "the John", Correct: "the John"}}},
			wantErrCount:  1,
			wantErrSubstr: "identical",
		},
		{
			name: "duplicate explicit id",
			set: JournalCorrections{ID: "ep1", Corrections: []Correction{
				{ID: "dup", Incorrect: "the John", Correct: "John"},
				{ID: "dup", Incorrect: "the John", Correct: "John"},
			}},
			wantErrCount:  1,
			wantErrSubstr: "duplicate correction id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateJournalCorrections(tt.set, text, "test.yml")
			assert.Len(t, got, tt.wantErrCount)
			if tt.wantErrSubstr != "" {
				assert.Contains(t, got[0].Message, tt.wantErrSubstr)
			}
		})
	}
}

func TestCorrection_DerivedID(t *testing.T) {
	assert.Equal(t, "explicit", Correction{ID: "explicit", Line: 5}.DerivedID("ep1", 1))
	assert.Equal(t, "ep1-L13-1", Correction{Line: 13}.DerivedID("ep1", 1))
	assert.Equal(t, "ep1-L13-2", Correction{Line: 13}.DerivedID("ep1", 2))
}

func TestCorrectionCategoryCounts(t *testing.T) {
	sets := []JournalCorrections{{
		ID: "ep1",
		Corrections: []Correction{
			{Category: "verb+prep"},
			{Category: "article"},
			{Category: "verb+prep"},
			{Category: ""},
		},
	}}
	want := []CategoryCount{
		{Category: "verb+prep", Count: 2},
		{Category: "article", Count: 1},
		{Category: "uncategorized", Count: 1},
	}
	assert.Equal(t, want, CorrectionCategoryCounts(sets))
}
