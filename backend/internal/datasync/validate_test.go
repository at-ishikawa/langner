package datasync

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/at-ishikawa/langner/internal/notebook"
)

func TestValidateRoundTrip(t *testing.T) {
	tests := []struct {
		name                       string
		sourceNotes                []notebook.NoteRecord
		exportedNotes              []notebook.NoteRecord
		sourceLearningByNotebook   map[string][]notebook.LearningHistoryExpression
		exportedLearningByNotebook map[string][]notebook.LearningHistoryExpression
		sourceDictCount            int
		exportedDictCount          int
		wantMismatches             int
		wantCategories             []string
	}{
		{
			name: "matching data has no mismatches",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene1"},
					},
				},
				{
					Usage: "let the cat out", Entry: "let the cat out",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene2"},
					},
				},
			},
			exportedNotes: []notebook.NoteRecord{
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene1"},
					},
				},
				{
					Usage: "let the cat out", Entry: "let the cat out",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene2"},
					},
				},
			},
			sourceLearningByNotebook: map[string][]notebook.LearningHistoryExpression{
				"s1": {
					{Expression: "break the ice", LearnedLogs: []notebook.LearningRecord{{Status: "understood"}}},
				},
			},
			exportedLearningByNotebook: map[string][]notebook.LearningHistoryExpression{
				"s1": {
					{Expression: "break the ice", LearnedLogs: []notebook.LearningRecord{{Status: "understood"}}},
				},
			},
			sourceDictCount: 5,
			exportedDictCount: 5,
			wantMismatches: 0,
		},
		{
			name: "note count mismatch",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene1"},
					},
				},
			},
			exportedNotes:              []notebook.NoteRecord{},
			sourceLearningByNotebook:   map[string][]notebook.LearningHistoryExpression{},
			exportedLearningByNotebook: map[string][]notebook.LearningHistoryExpression{},
			wantMismatches:             3, // notes + notebooks + notebook_definitions
			wantCategories:             []string{"notes", "notebooks", "notebook_definitions"},
		},
		{
			name: "per-notebook definition count mismatch",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene1"},
					},
				},
				{
					Usage: "lose one's temper", Entry: "lose one's temper",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene1"},
					},
				},
			},
			exportedNotes: []notebook.NoteRecord{
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene1"},
					},
				},
			},
			sourceLearningByNotebook:   map[string][]notebook.LearningHistoryExpression{},
			exportedLearningByNotebook: map[string][]notebook.LearningHistoryExpression{},
			wantMismatches:             2, // notes + notebook_definitions
			wantCategories:             []string{"notes", "notebook_definitions"},
		},
		{
			name: "learning log count mismatch",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene1"},
					},
				},
			},
			exportedNotes: []notebook.NoteRecord{
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene1"},
					},
				},
			},
			sourceLearningByNotebook: map[string][]notebook.LearningHistoryExpression{
				"s1": {
					{Expression: "break the ice", LearnedLogs: []notebook.LearningRecord{
						{Status: "understood"},
						{Status: "misunderstood"},
					}},
				},
			},
			exportedLearningByNotebook: map[string][]notebook.LearningHistoryExpression{
				"s1": {
					{Expression: "break the ice", LearnedLogs: []notebook.LearningRecord{
						{Status: "understood"},
					}},
				},
			},
			wantMismatches: 1,
			wantCategories: []string{"learning_logs"},
		},
		{
			name: "reverse log count mismatch",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene1"},
					},
				},
			},
			exportedNotes: []notebook.NoteRecord{
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene1"},
					},
				},
			},
			sourceLearningByNotebook: map[string][]notebook.LearningHistoryExpression{
				"s1": {
					{Expression: "break the ice", ReverseLogs: []notebook.LearningRecord{
						{Status: "understood"},
					}},
				},
			},
			exportedLearningByNotebook: map[string][]notebook.LearningHistoryExpression{
				"s1": {
					{Expression: "break the ice", ReverseLogs: []notebook.LearningRecord{}},
				},
			},
			wantMismatches: 1,
			wantCategories: []string{"learning_logs"},
		},
		{
			name: "dictionary count mismatch",
			sourceNotes:                []notebook.NoteRecord{},
			exportedNotes:              []notebook.NoteRecord{},
			sourceLearningByNotebook:   map[string][]notebook.LearningHistoryExpression{},
			exportedLearningByNotebook: map[string][]notebook.LearningHistoryExpression{},
			sourceDictCount:            10,
			exportedDictCount:          8,
			wantMismatches:             1,
			wantCategories:             []string{"dictionary"},
		},
		{
			name: "case-differing source notes deduplicated to match exported",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage: "Break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene1"},
					},
				},
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene2"},
					},
				},
			},
			exportedNotes: []notebook.NoteRecord{
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene1"},
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene2"},
					},
				},
			},
			sourceLearningByNotebook:   map[string][]notebook.LearningHistoryExpression{},
			exportedLearningByNotebook: map[string][]notebook.LearningHistoryExpression{},
			wantMismatches:             0,
		},
		{
			name: "multiple notebooks with mixed results",
			sourceNotes: []notebook.NoteRecord{
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene1"},
						{NotebookType: "flashcard", NotebookID: "f1", Group: "Vocab"},
					},
				},
			},
			exportedNotes: []notebook.NoteRecord{
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1", Group: "Ep1", Subgroup: "Scene1"},
						{NotebookType: "flashcard", NotebookID: "f1", Group: "Vocab"},
					},
				},
			},
			sourceLearningByNotebook:   map[string][]notebook.LearningHistoryExpression{},
			exportedLearningByNotebook: map[string][]notebook.LearningHistoryExpression{},
			sourceDictCount:            3,
			exportedDictCount:          3,
			wantMismatches:             0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			result := ValidateRoundTrip(
				tt.sourceNotes, tt.exportedNotes,
				tt.sourceLearningByNotebook, tt.exportedLearningByNotebook,
				tt.sourceDictCount, tt.exportedDictCount,
				&buf,
			)

			assert.Equal(t, tt.wantMismatches, len(result.Mismatches),
				"mismatch count: got %v", result.Mismatches)

			if len(tt.wantCategories) > 0 {
				gotCategories := make(map[string]bool)
				for _, m := range result.Mismatches {
					gotCategories[m.Category] = true
				}
				for _, cat := range tt.wantCategories {
					assert.True(t, gotCategories[cat], "expected category %q in mismatches", cat)
				}
			}

			// Verify output was written
			assert.Contains(t, buf.String(), "Round-Trip Validation")
		})
	}
}

func TestBuildNoteStats(t *testing.T) {
	tests := []struct {
		name                string
		notes               []notebook.NoteRecord
		wantTotalNotes      int
		wantNotebookCounts  map[string]int
	}{
		{
			name: "distinct notes across notebooks",
			notes: []notebook.NoteRecord{
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1"},
						{NotebookType: "flashcard", NotebookID: "f1"},
					},
				},
				{
					Usage: "lose one's temper", Entry: "lose one's temper",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1"},
					},
				},
			},
			wantTotalNotes:     2,
			wantNotebookCounts: map[string]int{"s1": 2, "f1": 1},
		},
		{
			name: "case-insensitive deduplication of unique notes",
			notes: []notebook.NoteRecord{
				{
					Usage: "Break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1"},
					},
				},
				{
					Usage: "break the ice", Entry: "break the ice",
					NotebookNotes: []notebook.NotebookNote{
						{NotebookType: "story", NotebookID: "s1"},
					},
				},
			},
			wantTotalNotes:     1,
			wantNotebookCounts: map[string]int{"s1": 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := buildNoteStats(tt.notes)

			assert.Equal(t, tt.wantTotalNotes, stats.TotalNotes)
			for nbID, wantCount := range tt.wantNotebookCounts {
				assert.Equal(t, wantCount, stats.NotebookStats[nbID].DefinitionCount)
			}
		})
	}
}

func TestBuildLearningStats(t *testing.T) {
	learning := map[string][]notebook.LearningHistoryExpression{
		"s1": {
			{
				Expression:  "break the ice",
				LearnedLogs: []notebook.LearningRecord{{Status: "understood"}, {Status: "misunderstood"}},
				ReverseLogs: []notebook.LearningRecord{{Status: "understood"}},
			},
		},
	}

	stats := buildLearningStats(learning)

	assert.Equal(t, 2, stats["s1"]["break the ice"].LearnedLogCount)
	assert.Equal(t, 1, stats["s1"]["break the ice"].ReverseLogCount)
}
