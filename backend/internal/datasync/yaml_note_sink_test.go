package datasync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/notebook"
)

func TestYAMLNoteSink_WriteAll(t *testing.T) {
	tests := []struct {
		name             string
		notes            []notebook.NoteRecord
		wantNotesYAML    string
		wantNNYAML       string
	}{
		{
			name: "notes with notebook_notes use snake_case field names",
			notes: []notebook.NoteRecord{
				{
					ID:               1,
					Usage:            "break the ice",
					Entry:            "start a conversation",
					Meaning:          "to initiate social interaction",
					Level:            "intermediate",
					DictionaryNumber: 2,
					NotebookNotes: []notebook.NotebookNote{
						{NoteID: 1, NotebookType: "story", NotebookID: "test-story", Group: "Episode 1", Subgroup: "Opening"},
					},
				},
				{
					ID:      2,
					Usage:   "lose one's temper",
					Entry:   "lose one's temper",
					Meaning: "to become angry",
					NotebookNotes: []notebook.NotebookNote{
						{NoteID: 2, NotebookType: "flashcard", NotebookID: "vocab-cards", Group: "Common Idioms"},
					},
				},
			},
			wantNotesYAML: `- id: 1
  usage: break the ice
  entry: start a conversation
  meaning: to initiate social interaction
  level: intermediate
  dictionary_number: 2
- id: 2
  usage: lose one's temper
  entry: lose one's temper
  meaning: to become angry
`,
			wantNNYAML: `- note_id: 1
  notebook_type: story
  notebook_id: test-story
  group: Episode 1
  subgroup: Opening
- note_id: 2
  notebook_type: flashcard
  notebook_id: vocab-cards
  group: Common Idioms
`,
		},
		{
			name:  "empty notes",
			notes: []notebook.NoteRecord{},
			wantNotesYAML: `[]
`,
			wantNNYAML: `[]
`,
		},
		{
			name: "notes exclude embedded relations",
			notes: []notebook.NoteRecord{
				{
					ID:      1,
					Usage:   "resilient",
					Entry:   "resilient",
					Meaning: "able to recover",
					Images: []notebook.NoteImage{
						{URL: "https://example.com/img.png"},
					},
					References: []notebook.NoteReference{
						{Link: "https://example.com/ref"},
					},
					NotebookNotes: []notebook.NotebookNote{
						{NoteID: 1, NotebookType: "story", NotebookID: "test-story", Group: "Episode 1"},
					},
				},
			},
			wantNotesYAML: `- id: 1
  usage: resilient
  entry: resilient
  meaning: able to recover
`,
			wantNNYAML: `- note_id: 1
  notebook_type: story
  notebook_id: test-story
  group: Episode 1
`,
		},
		{
			name: "omitempty fields are excluded when zero",
			notes: []notebook.NoteRecord{
				{
					ID:      3,
					Usage:   "hello",
					Entry:   "hello",
					Meaning: "a greeting",
				},
			},
			wantNotesYAML: `- id: 3
  usage: hello
  entry: hello
  meaning: a greeting
`,
			wantNNYAML: `[]
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			sink := NewYAMLNoteSink(dir)

			err := sink.WriteAll(tt.notes)
			require.NoError(t, err)

			gotNotes, err := os.ReadFile(filepath.Join(dir, "notes.yml"))
			require.NoError(t, err)
			assert.Equal(t, tt.wantNotesYAML, string(gotNotes))

			gotNN, err := os.ReadFile(filepath.Join(dir, "notebook_notes.yml"))
			require.NoError(t, err)
			assert.Equal(t, tt.wantNNYAML, string(gotNN))
		})
	}

	t.Run("MkdirAll error returns error", func(t *testing.T) {
		// Use a path under a file (not a directory) to force MkdirAll to fail
		dir := t.TempDir()
		filePath := filepath.Join(dir, "blocker")
		require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o644))

		sink := NewYAMLNoteSink(filepath.Join(filePath, "subdir"))
		err := sink.WriteAll(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "create output directory")
	})

	t.Run("writeYAML notes.yml error returns error", func(t *testing.T) {
		dir := t.TempDir()
		// Create notes.yml as a directory to cause os.Create to fail
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "notes.yml"), 0o755))

		sink := NewYAMLNoteSink(dir)
		err := sink.WriteAll([]notebook.NoteRecord{{ID: 1, Usage: "hello", Entry: "hello"}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write notes.yml")
	})

	t.Run("writeYAML notebook_notes.yml error returns error", func(t *testing.T) {
		dir := t.TempDir()
		// Create notebook_notes.yml as a directory to cause os.Create to fail
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "notebook_notes.yml"), 0o755))

		sink := NewYAMLNoteSink(dir)
		err := sink.WriteAll([]notebook.NoteRecord{{ID: 1, Usage: "hello", Entry: "hello"}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write notebook_notes.yml")
	})
}
