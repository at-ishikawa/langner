package notebook

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestYAMLNoteRepository_FindAll(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) *Reader
		want    []NoteRecord
		wantErr bool
	}{
		{
			name: "story notebooks with multiple scenes and definitions",
			setup: func(t *testing.T) *Reader {
				env := newTestFlashcardEnv(t)
				storyDir := env.createStoryIndex("my-story", "My Story", []string{"./season01.yml"})

				content := `- event: "Episode 1"
  date: 2025-01-01T00:00:00Z
  scenes:
    - scene: "Opening Scene"
      conversations:
        - speaker: "Alice"
          quote: "Let's {{ break the ice }}."
      definitions:
        - expression: "break the ice"
          meaning: "to initiate social interaction"
          level: "B2"
          dictionary_number: 1
          images:
            - "https://example.com/ice.jpg"
          references:
            - url: "https://example.com/ref"
              description: "Reference about idioms"
    - scene: "Closing Scene"
      conversations:
        - speaker: "Bob"
          quote: "Time to {{ call it a day }}."
      definitions:
        - expression: "call it a day"
          meaning: "to stop working"
`
				env.createCardFile(storyDir, "season01.yml", content)

				reader, err := NewReader([]string{env.tempDir}, nil, nil, nil, nil)
				require.NoError(t, err)
				return reader
			},
			want: []NoteRecord{
				{
					Usage:            "break the ice",
					Entry:            "break the ice",
					Meaning:          "to initiate social interaction",
					Level:            "B2",
					DictionaryNumber: 1,
					Images:           []NoteImage{{URL: "https://example.com/ice.jpg", SortOrder: 0}},
					References:       []NoteReference{{Link: "https://example.com/ref", Description: "Reference about idioms", SortOrder: 0}},
					NotebookNotes:    []NotebookNote{{NotebookType: "story", NotebookID: "my-story", Group: "Episode 1", Subgroup: "Opening Scene"}},
				},
				{
					Usage:         "call it a day",
					Entry:         "call it a day",
					Meaning:       "to stop working",
					Images:        []NoteImage{},
					References:    []NoteReference{},
					NotebookNotes: []NotebookNote{{NotebookType: "story", NotebookID: "my-story", Group: "Episode 1", Subgroup: "Closing Scene"}},
				},
			},
		},
		{
			name: "flashcard notebooks with cards",
			setup: func(t *testing.T) *Reader {
				env := newTestFlashcardEnv(t)
				flashcardDir := env.createFlashcardIndex("my-flashcards", "My Flashcards", []string{"./cards.yml"})

				content := `- title: "Common Phrases"
  date: 2025-01-01T00:00:00Z
  cards:
    - expression: "give up"
      meaning: "to stop trying"
    - expression: "look forward to"
      definition: "look forward"
      meaning: "to feel excited about something that is going to happen"
`
				env.createCardFile(flashcardDir, "cards.yml", content)

				reader, err := NewReader(nil, []string{env.tempDir}, nil, nil, nil)
				require.NoError(t, err)
				return reader
			},
			want: []NoteRecord{
				{
					Usage:         "give up",
					Entry:         "give up",
					Meaning:       "to stop trying",
					Images:        []NoteImage{},
					References:    []NoteReference{},
					NotebookNotes: []NotebookNote{{NotebookType: "flashcard", NotebookID: "my-flashcards", Group: "Common Phrases", Subgroup: ""}},
				},
				{
					Usage:         "look forward to",
					Entry:         "look forward",
					Meaning:       "to feel excited about something that is going to happen",
					Images:        []NoteImage{},
					References:    []NoteReference{},
					NotebookNotes: []NotebookNote{{NotebookType: "flashcard", NotebookID: "my-flashcards", Group: "Common Phrases", Subgroup: ""}},
				},
			},
		},
		{
			name: "deduplication merges notebook notes for same expression",
			setup: func(t *testing.T) *Reader {
				env := newTestFlashcardEnv(t)

				// Create a story with "break the ice"
				storyDir := env.createStoryIndex("story-one", "Story One", []string{"./episodes.yml"})
				storyContent := `- event: "Episode 1"
  date: 2025-01-01T00:00:00Z
  scenes:
    - scene: "Scene A"
      conversations:
        - speaker: "A"
          quote: "{{ break the ice }}"
      definitions:
        - expression: "break the ice"
          meaning: "to initiate social interaction"
`
				env.createCardFile(storyDir, "episodes.yml", storyContent)

				// Create a separate flashcard directory for flashcards
				flashcardBaseDir := filepath.Join(env.tempDir, "flashcards")
				err := os.MkdirAll(flashcardBaseDir, 0755)
				require.NoError(t, err)

				flashcardDir := filepath.Join(flashcardBaseDir, "fc-one")
				err = os.MkdirAll(flashcardDir, 0755)
				require.NoError(t, err)

				indexContent := "id: fc-one\nname: \"Flashcard One\"\nnotebooks:\n  - ./cards.yml\n"
				err = os.WriteFile(filepath.Join(flashcardDir, "index.yml"), []byte(indexContent), 0644)
				require.NoError(t, err)

				fcContent := `- title: "Idioms"
  date: 2025-02-01T00:00:00Z
  cards:
    - expression: "break the ice"
      meaning: "to initiate social interaction"
`
				err = os.WriteFile(filepath.Join(flashcardDir, "cards.yml"), []byte(fcContent), 0644)
				require.NoError(t, err)

				// Use storyDir's parent for stories, flashcardBaseDir for flashcards
				reader, err := NewReader([]string{env.tempDir}, []string{flashcardBaseDir}, nil, nil, nil)
				require.NoError(t, err)
				return reader
			},
			want: []NoteRecord{
				{
					Usage:      "break the ice",
					Entry:      "break the ice",
					Meaning:    "to initiate social interaction",
					Images:     []NoteImage{},
					References: []NoteReference{},
					NotebookNotes: []NotebookNote{
						{NotebookType: "story", NotebookID: "story-one", Group: "Episode 1", Subgroup: "Scene A"},
						{NotebookType: "flashcard", NotebookID: "fc-one", Group: "Idioms", Subgroup: ""},
					},
				},
			},
		},
		{
			name: "book type indexes use book notebook type",
			setup: func(t *testing.T) *Reader {
				env := newTestFlashcardEnv(t)
				booksDir := filepath.Join(env.tempDir, "books")
				err := os.MkdirAll(booksDir, 0755)
				require.NoError(t, err)

				bookDir := filepath.Join(booksDir, "my-book")
				err = os.MkdirAll(bookDir, 0755)
				require.NoError(t, err)

				indexContent := "id: my-book\nkind: Books\nname: \"My Book\"\nnotebooks:\n  - ./chapter01.yml\n"
				err = os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(indexContent), 0644)
				require.NoError(t, err)

				chapterContent := `- event: "Chapter 1"
  date: 2025-01-01T00:00:00Z
  scenes:
    - scene: "Paragraph 1"
      statements:
        - "The {{ silver lining }} was evident."
      definitions:
        - expression: "silver lining"
          meaning: "a positive aspect in an otherwise negative situation"
`
				err = os.WriteFile(filepath.Join(bookDir, "chapter01.yml"), []byte(chapterContent), 0644)
				require.NoError(t, err)

				reader, err := NewReader(nil, nil, []string{booksDir}, nil, nil)
				require.NoError(t, err)
				return reader
			},
			want: []NoteRecord{
				{
					Usage:         "silver lining",
					Entry:         "silver lining",
					Meaning:       "a positive aspect in an otherwise negative situation",
					Images:        []NoteImage{},
					References:    []NoteReference{},
					NotebookNotes: []NotebookNote{{NotebookType: "book", NotebookID: "my-book", Group: "Chapter 1", Subgroup: "Paragraph 1"}},
				},
			},
		},
		{
			name: "empty reader returns empty slice",
			setup: func(t *testing.T) *Reader {
				reader, err := NewReader(nil, nil, nil, nil, nil)
				require.NoError(t, err)
				return reader
			},
			want: []NoteRecord{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := tt.setup(t)
			repo := NewYAMLNoteRepository(reader)

			got, err := repo.FindAll(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			if len(tt.want) == 0 {
				assert.Empty(t, got)
				return
			}

			require.Len(t, got, len(tt.want))
			for i, wantRec := range tt.want {
				gotRec := got[i]
				assert.Equal(t, wantRec.Usage, gotRec.Usage, "record %d Usage", i)
				assert.Equal(t, wantRec.Entry, gotRec.Entry, "record %d Entry", i)
				assert.Equal(t, wantRec.Meaning, gotRec.Meaning, "record %d Meaning", i)
				assert.Equal(t, wantRec.Level, gotRec.Level, "record %d Level", i)
				assert.Equal(t, wantRec.DictionaryNumber, gotRec.DictionaryNumber, "record %d DictionaryNumber", i)

				require.Len(t, gotRec.Images, len(wantRec.Images), "record %d Images length", i)
				for j, wantImg := range wantRec.Images {
					assert.Equal(t, wantImg.URL, gotRec.Images[j].URL, "record %d image %d URL", i, j)
					assert.Equal(t, wantImg.SortOrder, gotRec.Images[j].SortOrder, "record %d image %d SortOrder", i, j)
				}

				require.Len(t, gotRec.References, len(wantRec.References), "record %d References length", i)
				for j, wantRef := range wantRec.References {
					assert.Equal(t, wantRef.Link, gotRec.References[j].Link, "record %d reference %d Link", i, j)
					assert.Equal(t, wantRef.Description, gotRec.References[j].Description, "record %d reference %d Description", i, j)
					assert.Equal(t, wantRef.SortOrder, gotRec.References[j].SortOrder, "record %d reference %d SortOrder", i, j)
				}

				require.Len(t, gotRec.NotebookNotes, len(wantRec.NotebookNotes), "record %d NotebookNotes length", i)
				for j, wantNN := range wantRec.NotebookNotes {
					assert.Equal(t, wantNN.NotebookType, gotRec.NotebookNotes[j].NotebookType, "record %d notebook note %d NotebookType", i, j)
					assert.Equal(t, wantNN.NotebookID, gotRec.NotebookNotes[j].NotebookID, "record %d notebook note %d NotebookID", i, j)
					assert.Equal(t, wantNN.Group, gotRec.NotebookNotes[j].Group, "record %d notebook note %d Group", i, j)
					assert.Equal(t, wantNN.Subgroup, gotRec.NotebookNotes[j].Subgroup, "record %d notebook note %d Subgroup", i, j)
				}
			}
		})
	}
}

func TestConvertRecordToNote(t *testing.T) {
	tests := []struct {
		name string
		rec  NoteRecord
		want Note
	}{
		{
			name: "basic conversion with different usage and entry",
			rec: NoteRecord{
				Usage:            "break the ice",
				Entry:            "start a conversation",
				Meaning:          "to initiate social interaction",
				Level:            "unusable",
				DictionaryNumber: 2,
			},
			want: Note{
				Expression:       "break the ice",
				Definition:       "start a conversation",
				Meaning:          "to initiate social interaction",
				Level:            ExpressionLevelUnusable,
				DictionaryNumber: 2,
			},
		},
		{
			name: "definition omitted when usage equals entry",
			rec: NoteRecord{
				Usage:   "resilient",
				Entry:   "resilient",
				Meaning: "able to recover quickly",
			},
			want: Note{
				Expression: "resilient",
				Meaning:    "able to recover quickly",
			},
		},
		{
			name: "images sorted by sort order",
			rec: NoteRecord{
				Usage: "persevere",
				Entry: "persevere",
				Images: []NoteImage{
					{URL: "https://example.com/img2.png", SortOrder: 1},
					{URL: "https://example.com/img1.png", SortOrder: 0},
				},
			},
			want: Note{
				Expression: "persevere",
				Images:     []string{"https://example.com/img1.png", "https://example.com/img2.png"},
			},
		},
		{
			name: "references sorted by sort order",
			rec: NoteRecord{
				Usage: "tenacious",
				Entry: "tenacious",
				References: []NoteReference{
					{Link: "https://example.com/ref2", Description: "Second reference", SortOrder: 1},
					{Link: "https://example.com/ref1", Description: "First reference", SortOrder: 0},
				},
			},
			want: Note{
				Expression: "tenacious",
				References: []Reference{
					{URL: "https://example.com/ref1", Description: "First reference"},
					{URL: "https://example.com/ref2", Description: "Second reference"},
				},
			},
		},
		{
			name: "empty record produces minimal note",
			rec:  NoteRecord{},
			want: Note{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertRecordToNote(tt.rec)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestYAMLNoteRepository_WriteAll(t *testing.T) {
	tests := []struct {
		name    string
		notes   []NoteRecord
		verify  func(t *testing.T, outputDir string)
		wantErr bool
	}{
		{
			name: "story notes produce stories directory",
			notes: []NoteRecord{
				{
					Usage:   "break the ice",
					Entry:   "start a conversation",
					Meaning: "to initiate social interaction",
					NotebookNotes: []NotebookNote{
						{NotebookType: "story", NotebookID: "test-series", Group: "Episode 1", Subgroup: "Opening Scene"},
					},
				},
				{
					Usage:   "lose one's temper",
					Entry:   "lose one's temper",
					Meaning: "to become angry",
					NotebookNotes: []NotebookNote{
						{NotebookType: "story", NotebookID: "test-series", Group: "Episode 1", Subgroup: "Conflict Scene"},
					},
				},
			},
			verify: func(t *testing.T, outputDir string) {
				// Verify index.yml
				indexPath := filepath.Join(outputDir, "stories", "test-series", "index.yml")
				var index exportIndex
				readYAMLHelper(t, indexPath, &index)
				assert.Equal(t, "test-series", index.ID)
				assert.Equal(t, "story", index.Kind)
				assert.Equal(t, "test-series", index.Name)
				assert.Equal(t, []string{"./notebooks.yml"}, index.NotebookPaths)

				// Verify notebooks.yml
				nbPath := filepath.Join(outputDir, "stories", "test-series", "notebooks.yml")
				var notebooks []StoryNotebook
				readYAMLHelper(t, nbPath, &notebooks)
				require.Len(t, notebooks, 1)
				assert.Equal(t, "Episode 1", notebooks[0].Event)
				require.Len(t, notebooks[0].Scenes, 2)
				assert.Equal(t, "Opening Scene", notebooks[0].Scenes[0].Title)
				require.Len(t, notebooks[0].Scenes[0].Definitions, 1)
				assert.Equal(t, "break the ice", notebooks[0].Scenes[0].Definitions[0].Expression)
				assert.Equal(t, "start a conversation", notebooks[0].Scenes[0].Definitions[0].Definition)
				assert.Equal(t, "Conflict Scene", notebooks[0].Scenes[1].Title)
				require.Len(t, notebooks[0].Scenes[1].Definitions, 1)
				assert.Equal(t, "lose one's temper", notebooks[0].Scenes[1].Definitions[0].Expression)
			},
		},
		{
			name: "flashcard notes produce flashcards directory",
			notes: []NoteRecord{
				{
					Usage:   "resilient",
					Entry:   "resilient",
					Meaning: "able to recover quickly",
					NotebookNotes: []NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Common Adjectives"},
					},
				},
				{
					Usage:   "tenacious",
					Entry:   "tenacious",
					Meaning: "persistent and determined",
					NotebookNotes: []NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Common Adjectives"},
					},
				},
			},
			verify: func(t *testing.T, outputDir string) {
				// Verify index.yml
				indexPath := filepath.Join(outputDir, "flashcards", "vocab-cards", "index.yml")
				var index exportIndex
				readYAMLHelper(t, indexPath, &index)
				assert.Equal(t, "vocab-cards", index.ID)
				assert.Equal(t, "", index.Kind)
				assert.Equal(t, "vocab-cards", index.Name)
				assert.Equal(t, []string{"./cards.yml"}, index.NotebookPaths)

				// Verify cards.yml
				cardsPath := filepath.Join(outputDir, "flashcards", "vocab-cards", "cards.yml")
				var flashcards []FlashcardNotebook
				readYAMLHelper(t, cardsPath, &flashcards)
				require.Len(t, flashcards, 1)
				assert.Equal(t, "Common Adjectives", flashcards[0].Title)
				require.Len(t, flashcards[0].Cards, 2)
				assert.Equal(t, "resilient", flashcards[0].Cards[0].Expression)
				assert.Equal(t, "tenacious", flashcards[0].Cards[1].Expression)
			},
		},
		{
			name: "book notes produce books directory",
			notes: []NoteRecord{
				{
					Usage:   "endeavor",
					Entry:   "endeavor",
					Meaning: "to try hard",
					NotebookNotes: []NotebookNote{
						{NotebookType: "book", NotebookID: "test-book", Group: "Chapter 1", Subgroup: "Introduction"},
					},
				},
			},
			verify: func(t *testing.T, outputDir string) {
				// Verify books directory is used
				indexPath := filepath.Join(outputDir, "books", "test-book", "index.yml")
				var index exportIndex
				readYAMLHelper(t, indexPath, &index)
				assert.Equal(t, "test-book", index.ID)
				assert.Equal(t, "book", index.Kind)

				nbPath := filepath.Join(outputDir, "books", "test-book", "notebooks.yml")
				var notebooks []StoryNotebook
				readYAMLHelper(t, nbPath, &notebooks)
				require.Len(t, notebooks, 1)
				assert.Equal(t, "Chapter 1", notebooks[0].Event)
			},
		},
		{
			name: "definition omitted when usage equals entry",
			notes: []NoteRecord{
				{
					Usage:   "resilient",
					Entry:   "resilient",
					Meaning: "able to recover quickly",
					NotebookNotes: []NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Adjectives"},
					},
				},
			},
			verify: func(t *testing.T, outputDir string) {
				cardsPath := filepath.Join(outputDir, "flashcards", "vocab-cards", "cards.yml")
				var flashcards []FlashcardNotebook
				readYAMLHelper(t, cardsPath, &flashcards)
				require.Len(t, flashcards, 1)
				require.Len(t, flashcards[0].Cards, 1)
				assert.Equal(t, "resilient", flashcards[0].Cards[0].Expression)
				assert.Equal(t, "", flashcards[0].Cards[0].Definition)
			},
		},
		{
			name: "images and references round-trip",
			notes: []NoteRecord{
				{
					Usage:   "persevere",
					Entry:   "persevere",
					Meaning: "to persist despite difficulty",
					Images: []NoteImage{
						{URL: "https://example.com/img1.png", SortOrder: 0},
						{URL: "https://example.com/img2.png", SortOrder: 1},
					},
					References: []NoteReference{
						{Link: "https://example.com/ref1", Description: "First reference", SortOrder: 0},
					},
					NotebookNotes: []NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Verbs"},
					},
				},
			},
			verify: func(t *testing.T, outputDir string) {
				cardsPath := filepath.Join(outputDir, "flashcards", "vocab-cards", "cards.yml")
				var flashcards []FlashcardNotebook
				readYAMLHelper(t, cardsPath, &flashcards)
				require.Len(t, flashcards, 1)
				require.Len(t, flashcards[0].Cards, 1)
				card := flashcards[0].Cards[0]
				assert.Equal(t, []string{"https://example.com/img1.png", "https://example.com/img2.png"}, card.Images)
				require.Len(t, card.References, 1)
				assert.Equal(t, "https://example.com/ref1", card.References[0].URL)
				assert.Equal(t, "First reference", card.References[0].Description)
			},
		},
		{
			name: "note in multiple notebooks appears in both",
			notes: []NoteRecord{
				{
					Usage:   "break the ice",
					Entry:   "start a conversation",
					Meaning: "to initiate social interaction",
					NotebookNotes: []NotebookNote{
						{NotebookType: "story", NotebookID: "test-series", Group: "Episode 1", Subgroup: "Opening"},
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Common Idioms"},
					},
				},
			},
			verify: func(t *testing.T, outputDir string) {
				// Verify it appears in story
				nbPath := filepath.Join(outputDir, "stories", "test-series", "notebooks.yml")
				var notebooks []StoryNotebook
				readYAMLHelper(t, nbPath, &notebooks)
				require.Len(t, notebooks, 1)
				require.Len(t, notebooks[0].Scenes, 1)
				assert.Equal(t, "break the ice", notebooks[0].Scenes[0].Definitions[0].Expression)

				// Verify it appears in flashcard
				cardsPath := filepath.Join(outputDir, "flashcards", "vocab-cards", "cards.yml")
				var flashcards []FlashcardNotebook
				readYAMLHelper(t, cardsPath, &flashcards)
				require.Len(t, flashcards, 1)
				require.Len(t, flashcards[0].Cards, 1)
				assert.Equal(t, "break the ice", flashcards[0].Cards[0].Expression)
			},
		},
		{
			name:  "empty notes produce no directories",
			notes: []NoteRecord{},
			verify: func(t *testing.T, outputDir string) {
				entries, err := os.ReadDir(outputDir)
				require.NoError(t, err)
				assert.Empty(t, entries)
			},
		},
		{
			name: "notes without notebook notes produce no directories",
			notes: []NoteRecord{
				{
					Usage:   "orphan",
					Entry:   "orphan",
					Meaning: "a note without notebook assignment",
				},
			},
			verify: func(t *testing.T, outputDir string) {
				entries, err := os.ReadDir(outputDir)
				require.NoError(t, err)
				assert.Empty(t, entries)
			},
		},
		{
			name: "multiple events and scenes produce correct grouping",
			notes: []NoteRecord{
				{
					Usage: "break the ice", Entry: "start a conversation", Meaning: "to initiate social interaction",
					NotebookNotes: []NotebookNote{
						{NotebookType: "story", NotebookID: "test-series", Group: "Episode 1", Subgroup: "Opening"},
					},
				},
				{
					Usage: "lose one's temper", Entry: "lose one's temper", Meaning: "to become angry",
					NotebookNotes: []NotebookNote{
						{NotebookType: "story", NotebookID: "test-series", Group: "Episode 2", Subgroup: "Conflict"},
					},
				},
			},
			verify: func(t *testing.T, outputDir string) {
				nbPath := filepath.Join(outputDir, "stories", "test-series", "notebooks.yml")
				var notebooks []StoryNotebook
				readYAMLHelper(t, nbPath, &notebooks)
				require.Len(t, notebooks, 2)
				assert.Equal(t, "Episode 1", notebooks[0].Event)
				assert.Equal(t, "Episode 2", notebooks[1].Event)
			},
		},
		{
			name: "multiple flashcard groups produce correct grouping",
			notes: []NoteRecord{
				{
					Usage: "resilient", Entry: "resilient", Meaning: "able to recover quickly",
					NotebookNotes: []NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Adjectives"},
					},
				},
				{
					Usage: "persevere", Entry: "persevere", Meaning: "to persist",
					NotebookNotes: []NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Verbs"},
					},
				},
			},
			verify: func(t *testing.T, outputDir string) {
				cardsPath := filepath.Join(outputDir, "flashcards", "vocab-cards", "cards.yml")
				var flashcards []FlashcardNotebook
				readYAMLHelper(t, cardsPath, &flashcards)
				require.Len(t, flashcards, 2)
				assert.Equal(t, "Adjectives", flashcards[0].Title)
				assert.Equal(t, "Verbs", flashcards[1].Title)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputDir := t.TempDir()
			repo := NewYAMLNoteRepositoryWriter(outputDir)

			err := repo.WriteAll(tt.notes)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.verify(t, outputDir)
		})
	}
}

func TestYAMLNoteRepository_WriteAll_errors(t *testing.T) {
	tests := []struct {
		name  string
		notes []NoteRecord
	}{
		{
			name: "story notebook write error on invalid path",
			notes: []NoteRecord{
				{
					Usage: "break the ice", Entry: "start a conversation",
					NotebookNotes: []NotebookNote{
						{NotebookType: "story", NotebookID: "test-series", Group: "Episode 1", Subgroup: "Opening"},
					},
				},
			},
		},
		{
			name: "flashcard notebook write error on invalid path",
			notes: []NoteRecord{
				{
					Usage: "resilient", Entry: "resilient",
					NotebookNotes: []NotebookNote{
						{NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Adjectives"},
					},
				},
			},
		},
		{
			name: "book notebook write error on invalid path",
			notes: []NoteRecord{
				{
					Usage: "endeavor", Entry: "endeavor",
					NotebookNotes: []NotebookNote{
						{NotebookType: "book", NotebookID: "test-book", Group: "Chapter 1", Subgroup: "Intro"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use /dev/null as parent to force MkdirAll failure
			repo := NewYAMLNoteRepositoryWriter("/dev/null/invalid")
			err := repo.WriteAll(tt.notes)
			assert.Error(t, err)
		})
	}
}

// readYAMLHelper is a test helper that reads and unmarshals a YAML file.
func readYAMLHelper(t *testing.T, path string, dest interface{}) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read file %s", path)
	require.NoError(t, yaml.Unmarshal(data, dest), "unmarshal %s", path)
}
