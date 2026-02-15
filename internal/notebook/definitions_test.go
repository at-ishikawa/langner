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
			name: "valid definitions file",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				content := `- metadata:
    notebook: chapter-1.yml
  scenes:
    - metadata:
        index: 0
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
						0: []Note{
							{Expression: "test", Meaning: "a test word", },
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
      expressions:
        - expression: first
          meaning: first word
    - metadata:
        index: 2
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
						0: []Note{
							{Expression: "first", Meaning: "first word", },
						},
						2: []Note{
							{Expression: "second", Meaning: "second word", },
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
        index: 0
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
						0: []Note{
							{Expression: "test", Meaning: "a test word", },
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
        index: 0
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
        index: 0
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
						0: []Note{
							{Expression: "colossal", DictionaryNumber: 1},
							{Expression: "test", Meaning: "a test meaning", DictionaryNumber: 2},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "definitions with scene field instead of index",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				content := `- metadata:
    title: Chapter I
  scenes:
    - metadata:
        scene: 5
      expressions:
        - expression: word
          meaning: a word
`
				err := os.WriteFile(filepath.Join(dir, "mybook.yml"), []byte(content), 0644)
				require.NoError(t, err)
				return dir
			},
			directories: nil,
			want: DefinitionsMap{
				"mybook": {
					"Chapter I": {
						5: []Note{
							{Expression: "word", Meaning: "a word"},
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
						0: []Note{
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
						0: []Note{
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
						0: []Note{
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
