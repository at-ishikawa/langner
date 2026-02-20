package notebook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlashcardNotebook_Validate(t *testing.T) {
	tests := []struct {
		name       string
		notebook   FlashcardNotebook
		location   string
		wantErrors int
		wantMsg    string
	}{
		{
			name: "valid notebook",
			notebook: FlashcardNotebook{
				Title: "Vocabulary Unit 1",
				Cards: []Note{
					{Expression: "hello", Meaning: "a greeting"},
					{Expression: "world", Images: []string{"world.png"}},
				},
			},
			location:   "test.yml",
			wantErrors: 0,
		},
		{
			name: "empty title",
			notebook: FlashcardNotebook{
				Title: "",
				Cards: []Note{
					{Expression: "hello", Meaning: "a greeting"},
				},
			},
			location:   "test.yml",
			wantErrors: 1,
			wantMsg:    "title is empty",
		},
		{
			name: "whitespace only title",
			notebook: FlashcardNotebook{
				Title: "   ",
				Cards: []Note{
					{Expression: "hello", Meaning: "a greeting"},
				},
			},
			location:   "test.yml",
			wantErrors: 1,
			wantMsg:    "title is empty",
		},
		{
			name: "empty expression in card",
			notebook: FlashcardNotebook{
				Title: "Unit 1",
				Cards: []Note{
					{Expression: "", Meaning: "a greeting"},
				},
			},
			location:   "test.yml",
			wantErrors: 1,
			wantMsg:    "expression is empty",
		},
		{
			name: "card with no meaning or images",
			notebook: FlashcardNotebook{
				Title: "Unit 1",
				Cards: []Note{
					{Expression: "hello", Meaning: "", Images: nil},
				},
			},
			location:   "test.yml",
			wantErrors: 1,
			wantMsg:    "has neither meaning nor images",
		},
		{
			name: "multiple errors",
			notebook: FlashcardNotebook{
				Title: "",
				Cards: []Note{
					{Expression: "", Meaning: "a greeting"},
					{Expression: "hello", Meaning: ""},
				},
			},
			location:   "test.yml",
			wantErrors: 3,
		},
		{
			name: "no cards - valid",
			notebook: FlashcardNotebook{
				Title: "Empty Unit",
				Cards: []Note{},
			},
			location:   "test.yml",
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := tt.notebook.Validate(tt.location)
			assert.Len(t, errors, tt.wantErrors)
			if tt.wantMsg != "" && len(errors) > 0 {
				assert.Contains(t, errors[0].Message, tt.wantMsg)
			}
		})
	}
}

func TestFilterFlashcardNotebooks(t *testing.T) {
	now := time.Now()
	longAgo := now.Add(-365 * 24 * time.Hour)

	tests := []struct {
		name      string
		notebooks []FlashcardNotebook
		history   []LearningHistory
		sortDesc  bool
		wantLen   int
		wantErr   bool
	}{
		{
			name: "empty notebooks",
			notebooks: []FlashcardNotebook{},
			history:   nil,
			wantLen:   0,
		},
		{
			name: "notebook with empty cards is skipped",
			notebooks: []FlashcardNotebook{
				{Title: "Empty", Cards: []Note{}},
			},
			history: nil,
			wantLen: 0,
		},
		{
			name: "cards with no logs need to learn",
			notebooks: []FlashcardNotebook{
				{
					Title: "Unit 1",
					Date:  now,
					Cards: []Note{
						{Expression: "hello", Meaning: "a greeting"},
					},
				},
			},
			history: nil,
			wantLen: 1,
		},
		{
			name: "empty expression returns error",
			notebooks: []FlashcardNotebook{
				{
					Title: "Unit 1",
					Date:  now,
					Cards: []Note{
						{Expression: "   ", Meaning: "a greeting"},
					},
				},
			},
			history: nil,
			wantErr: true,
		},
		{
			name: "sort ascending by date",
			notebooks: []FlashcardNotebook{
				{
					Title: "Newer",
					Date:  now,
					Cards: []Note{
						{Expression: "hello", Meaning: "a greeting"},
					},
				},
				{
					Title: "Older",
					Date:  longAgo,
					Cards: []Note{
						{Expression: "world", Meaning: "the earth"},
					},
				},
			},
			history:  nil,
			sortDesc: false,
			wantLen:  2,
		},
		{
			name: "sort descending by date",
			notebooks: []FlashcardNotebook{
				{
					Title: "Older",
					Date:  longAgo,
					Cards: []Note{
						{Expression: "hello", Meaning: "a greeting"},
					},
				},
				{
					Title: "Newer",
					Date:  now,
					Cards: []Note{
						{Expression: "world", Meaning: "the earth"},
					},
				},
			},
			history:  nil,
			sortDesc: true,
			wantLen:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FilterFlashcardNotebooks(tt.notebooks, tt.history, nil, tt.sortDesc)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, result, tt.wantLen)

			if tt.sortDesc && len(result) >= 2 {
				assert.True(t, result[0].Date.After(result[1].Date))
			}
			if !tt.sortDesc && len(result) >= 2 {
				assert.True(t, result[0].Date.Before(result[1].Date))
			}
		})
	}
}
