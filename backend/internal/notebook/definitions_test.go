package notebook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefinitionsSceneMetadata_GetIndex(t *testing.T) {
	ptr := func(i int) *int { return &i }

	tests := []struct {
		name     string
		metadata DefinitionsSceneMetadata
		want     int
	}{
		{
			name:     "Scene nil, Index 0",
			metadata: DefinitionsSceneMetadata{Scene: nil, Index: 0},
			want:     0,
		},
		{
			name:     "Scene nil, Index 5",
			metadata: DefinitionsSceneMetadata{Scene: nil, Index: 5},
			want:     5,
		},
		{
			name:     "Scene ptr(0), Index 5 - Scene takes precedence",
			metadata: DefinitionsSceneMetadata{Scene: ptr(0), Index: 5},
			want:     0,
		},
		{
			name:     "Scene ptr(3), Index 5 - Scene takes precedence",
			metadata: DefinitionsSceneMetadata{Scene: ptr(3), Index: 5},
			want:     3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.metadata.GetIndex()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewDefinitionsMap(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		directories []string
		want        DefinitionsMap
		wantErr     bool
	}{
		{
			name: "empty directories",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			directories: []string{},
			want:        DefinitionsMap{},
			wantErr:     false,
		},
		{
			name: "empty string in directories",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			directories: []string{""},
			want:        DefinitionsMap{},
			wantErr:     false,
		},
		{
			name: "nonexistent directory is skipped",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			directories: []string{"/nonexistent/definitions/path"},
			want:        DefinitionsMap{},
			wantErr:     false,
		},
		{
			name: "non-yml files are skipped",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not yaml"), 0644))
				return dir
			},
			directories: nil,
			want:        DefinitionsMap{},
			wantErr:     false,
		},
		{
			name: "invalid YAML file returns error",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.yml"), []byte("not: [valid: yaml"), 0644))
				return dir
			},
			directories: nil,
			wantErr:     true,
		},
		{
			name: "valid definitions file",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				content := `- metadata:
    notebook: chapter-1.yml
  scenes:
    - metadata:
        title: Scene A
      expressions:
        - expression: test
          meaning: a test word
`
				err := os.WriteFile(filepath.Join(dir, "mybook.yml"), []byte(content), 0644)
				require.NoError(t, err)
				return dir
			},
			directories: nil, // will be set in test
			want: DefinitionsMap{
				"mybook": {
					"chapter-1.yml": {
						"__index_0": []Note{
							{Expression: "test", Meaning: "a test word"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple scenes in definitions",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				content := `- metadata:
    notebook: chapter-1.yml
  scenes:
    - metadata:
        index: 0
        title: First Scene
      expressions:
        - expression: first
          meaning: first word
    - metadata:
        index: 1
        title: Second Scene
      expressions:
        - expression: second
          meaning: second word
          statement_index: 1
`
				err := os.WriteFile(filepath.Join(dir, "mybook.yml"), []byte(content), 0644)
				require.NoError(t, err)
				return dir
			},
			directories: nil,
			want: DefinitionsMap{
				"mybook": {
					"chapter-1.yml": {
						"__index_0": []Note{
							{Expression: "first", Meaning: "first word"},
						},
						"__index_1": []Note{
							{Expression: "second", Meaning: "second word"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "definitions with title instead of notebook",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				content := `- metadata:
    title: Chapter I
  scenes:
    - metadata:
        title: Scene A
      expressions:
        - expression: test
          meaning: a test word
`
				err := os.WriteFile(filepath.Join(dir, "mybook.yml"), []byte(content), 0644)
				require.NoError(t, err)
				return dir
			},
			directories: nil,
			want: DefinitionsMap{
				"mybook": {
					"Chapter I": {
						"__index_0": []Note{
							{Expression: "test", Meaning: "a test word"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "skip definitions without notebook or title",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				content := `- metadata: {}
  scenes:
    - metadata:
        title: Scene A
      expressions:
        - expression: test
          meaning: a test word
`
				err := os.WriteFile(filepath.Join(dir, "mybook.yml"), []byte(content), 0644)
				require.NoError(t, err)
				return dir
			},
			directories: nil,
			want: DefinitionsMap{
				"mybook": {},
			},
			wantErr: false,
		},
		{
			name: "definitions with dictionary_number",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				content := `- metadata:
    title: Chapter I
  scenes:
    - metadata:
        title: Scene A
      expressions:
        - expression: colossal
          dictionary_number: 1
        - expression: test
          meaning: a test meaning
          dictionary_number: 2
`
				err := os.WriteFile(filepath.Join(dir, "mybook.yml"), []byte(content), 0644)
				require.NoError(t, err)
				return dir
			},
			directories: nil,
			want: DefinitionsMap{
				"mybook": {
					"Chapter I": {
						"__index_0": []Note{
							{Expression: "colossal", DictionaryNumber: 1},
							{Expression: "test", Meaning: "a test meaning", DictionaryNumber: 2},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "declared index differs from array position",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				// The definitions file has only one scene, but it declares
				// index: 1 (the second scene in the story). The array
				// position is 0, so the current code stores it at
				// __index_0 instead of __index_1.
				content := `- metadata:
    title: Episode 1
  scenes:
    - metadata:
        index: 1
        title: At the office
      expressions:
        - expression: break the ice
          meaning: initiate conversation in a social setting
`
				err := os.WriteFile(filepath.Join(dir, "show.yml"), []byte(content), 0644)
				require.NoError(t, err)
				return dir
			},
			directories: nil,
			want: DefinitionsMap{
				"show": {
					"Episode 1": {
						"__index_1": []Note{
							{Expression: "break the ice", Meaning: "initiate conversation in a social setting"},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)
			dirs := tt.directories
			if dirs == nil {
				dirs = []string{dir}
			}

			got, err := NewDefinitionsMap(dirs)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMergeDefinitionsIntoNotebooks(t *testing.T) {
	tests := []struct {
		name           string
		bookID         string
		notebooks      []StoryNotebook
		notebookPaths  []string
		definitionsMap DefinitionsMap
		want           []StoryNotebook
	}{
		{
			name:           "no definitions for book",
			bookID:         "unknown",
			notebooks:      []StoryNotebook{{Event: "test"}},
			notebookPaths:  []string{"chapter-1.yml"},
			definitionsMap: DefinitionsMap{},
			want:           []StoryNotebook{{Event: "test"}},
		},
		{
			name:   "merge definitions into scene",
			bookID: "mybook",
			notebooks: []StoryNotebook{
				{
					Event: "Chapter 1",
					Scenes: []StoryScene{
						{
							Title:      "Scene 1",
							Statements: []string{"This is a test statement."},
						},
					},
				},
			},
			notebookPaths: []string{"chapter-1.yml"},
			definitionsMap: DefinitionsMap{
				"mybook": {
					"chapter-1.yml": {
						"__index_0": []Note{
							{Expression: "test", Meaning: "a test word", },
						},
					},
				},
			},
			want: []StoryNotebook{
				{
					Event: "Chapter 1",
					Scenes: []StoryScene{
						{
							Title:      "Scene 1",
							Statements: []string{"This is a {{ test }} statement."},
							Definitions: []Note{
								{Expression: "test", Meaning: "a test word"},
							},
						},
					},
				},
			},
		},
		{
			name:   "no matching notebook path",
			bookID: "mybook",
			notebooks: []StoryNotebook{
				{
					Event: "Chapter 1",
					Scenes: []StoryScene{
						{
							Title:      "Scene 1",
							Statements: []string{"This is a test."},
						},
					},
				},
			},
			notebookPaths: []string{"other-chapter.yml"},
			definitionsMap: DefinitionsMap{
				"mybook": {
					"chapter-1.yml": {
						"__index_0": []Note{
							{Expression: "test", Meaning: "a test word", },
						},
					},
				},
			},
			want: []StoryNotebook{
				{
					Event: "Chapter 1",
					Scenes: []StoryScene{
						{
							Title:      "Scene 1",
							Statements: []string{"This is a test."},
						},
					},
				},
			},
		},
		{
			name:   "more notebooks than paths skips extra notebooks",
			bookID: "mybook",
			notebooks: []StoryNotebook{
				{Event: "Chapter 1", Scenes: []StoryScene{{Title: "Scene 1"}}},
				{Event: "Chapter 2", Scenes: []StoryScene{{Title: "Scene 1"}}},
			},
			notebookPaths: []string{"chapter-1.yml"},
			definitionsMap: DefinitionsMap{
				"mybook": {
					"chapter-1.yml": {
						"__index_0": []Note{{Expression: "test", Meaning: "a test word"}},
					},
				},
			},
			want: []StoryNotebook{
				{Event: "Chapter 1", Scenes: []StoryScene{{Title: "Scene 1", Definitions: []Note{{Expression: "test", Meaning: "a test word"}}}}},
				{Event: "Chapter 2", Scenes: []StoryScene{{Title: "Scene 1"}}},
			},
		},
		{
			name:   "scene title not in definitions is skipped",
			bookID: "mybook",
			notebooks: []StoryNotebook{
				{
					Event: "Chapter 1",
					Scenes: []StoryScene{
						{Title: "Scene 1"},
						{Title: "Scene 2"},
					},
				},
			},
			notebookPaths: []string{"chapter-1.yml"},
			definitionsMap: DefinitionsMap{
				"mybook": {
					"chapter-1.yml": {
						"__index_0": []Note{{Expression: "test", Meaning: "a test word"}},
					},
				},
			},
			want: []StoryNotebook{
				{
					Event: "Chapter 1",
					Scenes: []StoryScene{
						{Title: "Scene 1", Definitions: []Note{{Expression: "test", Meaning: "a test word"}}},
						{Title: "Scene 2"},
					},
				},
			},
		},
		{
			name:   "match by title when notebook path doesn't match",
			bookID: "mybook",
			notebooks: []StoryNotebook{
				{
					Event: "Chapter I",
					Scenes: []StoryScene{
						{
							Title:      "Scene 1",
							Statements: []string{"This is a test statement."},
						},
					},
				},
			},
			notebookPaths: []string{"009-chapter-1.yml"},
			definitionsMap: DefinitionsMap{
				"mybook": {
					"Chapter I": {
						"__index_0": []Note{
							{Expression: "test", Meaning: "a test word", },
						},
					},
				},
			},
			want: []StoryNotebook{
				{
					Event: "Chapter I",
					Scenes: []StoryScene{
						{
							Title:      "Scene 1",
							Statements: []string{"This is a {{ test }} statement."},
							Definitions: []Note{
								{Expression: "test", Meaning: "a test word"},
							},
						},
					},
				},
			},
		},
		{
			name:   "definitions at declared index merge into correct scene",
			bookID: "show",
			notebooks: []StoryNotebook{
				{
					Event: "Episode 1",
					Scenes: []StoryScene{
						{
							Title:         "Opening",
							Conversations: []Conversation{{Speaker: "Alice", Quote: "Good morning everyone."}},
						},
						{
							Title:         "At the office",
							Conversations: []Conversation{{Speaker: "Bob", Quote: "Let me break the ice here."}},
						},
					},
				},
			},
			notebookPaths: []string{"season1.yml"},
			definitionsMap: DefinitionsMap{
				"show": {
					"Episode 1": {
						// Definitions belong to scene index 1 ("At the office"),
						// NOT scene index 0 ("Opening").
						"__index_1": []Note{
							{Expression: "break the ice", Meaning: "initiate conversation in a social setting"},
						},
					},
				},
			},
			want: []StoryNotebook{
				{
					Event: "Episode 1",
					Scenes: []StoryScene{
						{
							Title:         "Opening",
							Conversations: []Conversation{{Speaker: "Alice", Quote: "Good morning everyone."}},
						},
						{
							Title:         "At the office",
							Conversations: []Conversation{{Speaker: "Bob", Quote: "Let me break the ice here."}},
							Definitions:   []Note{{Expression: "break the ice", Meaning: "initiate conversation in a social setting"}},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeDefinitionsIntoNotebooks(tt.bookID, tt.notebooks, tt.notebookPaths, tt.definitionsMap)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAddExpressionMarker(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		expression string
		want       string
	}{
		{
			name:       "simple word",
			text:       "This is a test statement.",
			expression: "test",
			want:       "This is a {{ test }} statement.",
		},
		{
			name:       "case insensitive",
			text:       "This is a Test statement.",
			expression: "test",
			want:       "This is a {{ Test }} statement.",
		},
		{
			name:       "already has markers for same expression",
			text:       "This is a {{ test }} statement.",
			expression: "test",
			want:       "This is a {{ test }} statement.",
		},
		{
			name:       "word not found",
			text:       "This is a statement.",
			expression: "test",
			want:       "This is a statement.",
		},
		{
			name:       "word boundary",
			text:       "testing is not test",
			expression: "test",
			want:       "testing is not {{ test }}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addExpressionMarker(tt.text, tt.expression)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReadDefinitionsFromBytes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []Definitions
		wantErr bool
	}{
		{
			name: "valid definitions",
			input: `- metadata:
    notebook: "001-chapter-1.yml"
  scenes:
    - metadata:
        index: 0
      expressions:
        - expression: "break the ice"
          meaning: "to initiate conversation in a social situation"
`,
			want: []Definitions{{
				Metadata: DefinitionsMetadata{Notebook: "001-chapter-1.yml"},
				Scenes: []DefinitionsScene{{
					Metadata:    DefinitionsSceneMetadata{Index: 0},
					Expressions: []Note{{Expression: "break the ice", Meaning: "to initiate conversation in a social situation"}},
				}},
			}},
			wantErr: false,
		},
		{
			name:    "invalid yaml",
			input:   "{{invalid yaml",
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadDefinitionsFromBytes([]byte(tt.input))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
