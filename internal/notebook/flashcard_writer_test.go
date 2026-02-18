package notebook

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/assets"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFlashcardNotebookWriter(t *testing.T) {
	reader, err := NewReader(nil, nil, nil, nil, nil)
	require.NoError(t, err)

	writer := NewFlashcardNotebookWriter(reader, "template.md")
	assert.NotNil(t, writer)
	assert.Equal(t, reader, writer.reader)
	assert.Equal(t, "template.md", writer.templatePath)
}

func TestConvertToAssetsFlashcardTemplate(t *testing.T) {
	date := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		notebooks []FlashcardNotebook
		wantLen   int
	}{
		{
			name:      "empty notebooks",
			notebooks: []FlashcardNotebook{},
			wantLen:   0,
		},
		{
			name: "single notebook with cards",
			notebooks: []FlashcardNotebook{
				{
					Title:       "Unit 1",
					Description: "First unit",
					Date:        date,
					Cards: []Note{
						{
							Expression:    "hello",
							Definition:    "greeting",
							Meaning:       "a word used to greet",
							Examples:      []string{"Hello there!"},
							Pronunciation: "heh-loh",
							PartOfSpeech:  "interjection",
							Origin:        "Old English",
							Synonyms:      []string{"hi", "hey"},
							Antonyms:      []string{"goodbye"},
							Images:        []string{"hello.png"},
						},
					},
				},
			},
			wantLen: 1,
		},
		{
			name: "multiple notebooks",
			notebooks: []FlashcardNotebook{
				{Title: "Unit 1", Date: date, Cards: []Note{{Expression: "hello", Meaning: "a greeting"}}},
				{Title: "Unit 2", Date: date, Cards: []Note{{Expression: "world", Meaning: "the earth"}}},
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToAssetsFlashcardTemplate(tt.notebooks)
			assert.Len(t, result.Notebooks, tt.wantLen)

			if tt.wantLen > 0 {
				got := result.Notebooks[0]
				want := tt.notebooks[0]
				assert.Equal(t, want.Title, got.Title)
				assert.Equal(t, want.Description, got.Description)
				assert.Equal(t, want.Date, got.Date)
				assert.Len(t, got.Cards, len(want.Cards))
			}
		})
	}
}

func TestConvertFlashcardNotebook(t *testing.T) {
	date := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)

	nb := FlashcardNotebook{
		Title:       "Unit 1",
		Description: "First unit",
		Date:        date,
		Cards: []Note{
			{
				Expression:    "hello",
				Definition:    "greeting",
				Meaning:       "a word used to greet",
				Examples:      []string{"Hello there!"},
				Pronunciation: "heh-loh",
				PartOfSpeech:  "interjection",
				Origin:        "Old English",
				Synonyms:      []string{"hi", "hey"},
				Antonyms:      []string{"goodbye"},
				Images:        []string{"hello.png"},
			},
		},
	}

	result := convertFlashcardNotebook(nb)

	assert.Equal(t, "Unit 1", result.Title)
	assert.Equal(t, "First unit", result.Description)
	assert.Equal(t, date, result.Date)
	assert.Len(t, result.Cards, 1)

	card := result.Cards[0]
	assert.Equal(t, assets.FlashcardCard{
		Expression:    "hello",
		Definition:    "greeting",
		Meaning:       "a word used to greet",
		Examples:      []string{"Hello there!"},
		Pronunciation: "heh-loh",
		PartOfSpeech:  "interjection",
		Origin:        "Old English",
		Synonyms:      []string{"hi", "hey"},
		Antonyms:      []string{"goodbye"},
		Images:        []string{"hello.png"},
	}, card)
}

func TestOutputFlashcardNotebooks(t *testing.T) {
	flashcardsDir := t.TempDir()
	outputDir := t.TempDir()

	// Create flashcard data
	flashcardDir := filepath.Join(flashcardsDir, "test-flashcard")
	require.NoError(t, os.MkdirAll(flashcardDir, 0755))
	require.NoError(t, WriteYamlFile(filepath.Join(flashcardDir, "index.yml"), FlashcardIndex{
		ID: "test-flashcard", Name: "Test Flashcards",
		NotebookPaths: []string{"cards.yml"},
	}))
	require.NoError(t, WriteYamlFile(filepath.Join(flashcardDir, "cards.yml"), []FlashcardNotebook{
		{
			Title: "Common Words",
			Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Cards: []Note{
				{Expression: "hello", Meaning: "a greeting"},
			},
		},
	}))

	reader, err := NewReader(nil, []string{flashcardsDir}, nil, nil, nil)
	require.NoError(t, err)

	writer := NewFlashcardNotebookWriter(reader, "")

	t.Run("success", func(t *testing.T) {
		learningHistories := map[string][]LearningHistory{
			"test-flashcard": {
				{
					Metadata: LearningHistoryMetadata{NotebookID: "test-flashcard", Title: "flashcards", Type: "flashcard"},
					Expressions: []LearningHistoryExpression{
						{Expression: "hello", LearnedLogs: []LearningRecord{{Status: "misunderstood", LearnedAt: NewDate(time.Now())}}},
					},
				},
			},
		}

		err := writer.OutputFlashcardNotebooks("test-flashcard", map[string]rapidapi.Response{}, learningHistories, false, outputDir, false)
		require.NoError(t, err)

		outputFile := filepath.Join(outputDir, "test-flashcard.md")
		_, err = os.Stat(outputFile)
		assert.NoError(t, err)
	})

	t.Run("notebook not found", func(t *testing.T) {
		err := writer.OutputFlashcardNotebooks("nonexistent", map[string]rapidapi.Response{}, map[string][]LearningHistory{}, false, outputDir, false)
		assert.Error(t, err)
	})
}
