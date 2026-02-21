package notebook

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
