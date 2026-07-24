package notebook

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJournalNotebook_Validate(t *testing.T) {
	validEntry := JournalEntry{
		ID:   "e1",
		Text: "the John called me and he suggested to play a game",
		Mistakes: []Mistake{
			{ID: "m1", Incorrect: "the John", Correct: "John", Category: "article"},
			{ID: "m2", Incorrect: "suggested to play", Correct: "suggested playing", Category: "verb-pattern"},
		},
	}

	tests := []struct {
		name          string
		notebook      JournalNotebook
		wantErrCount  int
		wantErrSubstr string
	}{
		{
			name:         "valid notebook - no errors",
			notebook:     JournalNotebook{Title: "Journal", Entries: []JournalEntry{validEntry}},
			wantErrCount: 0,
		},
		{
			name:          "empty title",
			notebook:      JournalNotebook{Entries: []JournalEntry{validEntry}},
			wantErrCount:  1,
			wantErrSubstr: "title is empty",
		},
		{
			name: "entry missing id and text",
			notebook: JournalNotebook{
				Title:   "Journal",
				Entries: []JournalEntry{{}},
			},
			wantErrCount: 2,
		},
		{
			name: "incorrect span not in text",
			notebook: JournalNotebook{
				Title: "Journal",
				Entries: []JournalEntry{{
					ID:       "e1",
					Text:     "a clean sentence",
					Mistakes: []Mistake{{ID: "m1", Incorrect: "the John", Correct: "John"}},
				}},
			},
			wantErrCount:  1,
			wantErrSubstr: "not found in entry text",
		},
		{
			name: "correct equals incorrect",
			notebook: JournalNotebook{
				Title: "Journal",
				Entries: []JournalEntry{{
					ID:       "e1",
					Text:     "the John called",
					Mistakes: []Mistake{{ID: "m1", Incorrect: "the John", Correct: "the John"}},
				}},
			},
			wantErrCount:  1,
			wantErrSubstr: "identical",
		},
		{
			name: "duplicate mistake id",
			notebook: JournalNotebook{
				Title: "Journal",
				Entries: []JournalEntry{{
					ID:   "e1",
					Text: "the John called the John",
					Mistakes: []Mistake{
						{ID: "dup", Incorrect: "the John", Correct: "John"},
						{ID: "dup", Incorrect: "the John", Correct: "John"},
					},
				}},
			},
			wantErrCount:  1,
			wantErrSubstr: "duplicate mistake id",
		},
		{
			name: "mistake missing incorrect and correct",
			notebook: JournalNotebook{
				Title: "Journal",
				Entries: []JournalEntry{{
					ID:       "e1",
					Text:     "some text",
					Mistakes: []Mistake{{ID: "m1"}},
				}},
			},
			wantErrCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.notebook.Validate("test.yml")
			assert.Len(t, got, tt.wantErrCount)
			if tt.wantErrSubstr != "" {
				assert.Contains(t, got[0].Message, tt.wantErrSubstr)
			}
		})
	}
}

func TestCategoryCounts(t *testing.T) {
	notebooks := []JournalNotebook{
		{
			Entries: []JournalEntry{
				{Mistakes: []Mistake{
					{ID: "m1", Category: "preposition"},
					{ID: "m2", Category: "article"},
					{ID: "m3", Category: "preposition"},
				}},
				{Mistakes: []Mistake{
					{ID: "m4", Category: "preposition"},
					{ID: "m5", Category: ""},
				}},
			},
		},
	}

	got := CategoryCounts(notebooks)

	want := []CategoryCount{
		{Category: "preposition", Count: 3},
		{Category: "article", Count: 1},
		{Category: "uncategorized", Count: 1},
	}
	assert.Equal(t, want, got)
}
