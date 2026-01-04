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

func TestConvertMarkersInText(t *testing.T) {
	definitions := []Note{
		{Expression: "test phrase"},
		{Expression: "another word"},
	}

	tests := []struct {
		name             string
		text             string
		conversionStyle  ConversionStyle
		targetExpression string
		expected         string
	}{
		{
			name:             "Markdown - highlight specific expression",
			text:             "I have {{ test phrase }} and {{ another word }} here.",
			conversionStyle:  ConversionStyleMarkdown,
			targetExpression: "test phrase",
			expected:         "I have **test phrase** and another word here.",
		},
		{
			name:             "Terminal - highlight specific expression",
			text:             "I have {{ test phrase }} and {{ another word }} here.",
			conversionStyle:  ConversionStyleTerminal,
			targetExpression: "test phrase",
			expected:         "I have \x1b[1mtest phrase\x1b[22m and another word here.",
		},
		{
			name:             "Plain - all plain text",
			text:             "I have {{ test phrase }} and {{ another word }} here.",
			conversionStyle:  ConversionStylePlain,
			targetExpression: "test phrase",
			expected:         "I have test phrase and another word here.",
		},
		{
			name:             "Empty target - all expressions highlighted",
			text:             "I have {{ test phrase }} and {{ another word }} here.",
			conversionStyle:  ConversionStyleMarkdown,
			targetExpression: "",
			expected:         "I have **test phrase** and **another word** here.",
		},
		{
			name:             "Non-learning expression removed",
			text:             "I have {{ test phrase }} and {{ unknown }} here.",
			conversionStyle:  ConversionStyleMarkdown,
			targetExpression: "",
			expected:         "I have **test phrase** and unknown here.",
		},
		{
			name:             "Case insensitive matching",
			text:             "I have {{ TEST PHRASE }} here.",
			conversionStyle:  ConversionStyleMarkdown,
			targetExpression: "test phrase",
			expected:         "I have **TEST PHRASE** here.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ConvertMarkersInText(tc.text, definitions, tc.conversionStyle, tc.targetExpression)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestAssetsStoryConverter_convertToAssetsStoryTemplate(t *testing.T) {
	testDate := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	notebooks := []StoryNotebook{
		{
			Event: "Test Story",
			Date:  testDate,
			Metadata: Metadata{
				Series:  "Test Series",
				Season:  1,
				Episode: 1,
			},
			Scenes: []StoryScene{
				{
					Title: "Scene 1",
					Conversations: []Conversation{
						{Speaker: "A", Quote: "This is a {{ test phrase }} here."},
					},
					Definitions: []Note{
						{Expression: "test phrase", Meaning: "A phrase for testing"},
					},
				},
			},
		},
	}

	tests := []struct {
		name string
		want assets.StoryTemplate
	}{
		{
			name: "Markdown conversion",
			want: assets.StoryTemplate{
				Notebooks: []assets.StoryNotebook{
					{
						Event: "Test Story",
						Date:  testDate,
						Metadata: assets.Metadata{
							Series:  "Test Series",
							Season:  1,
							Episode: 1,
						},
						Scenes: []assets.StoryScene{
							{
								Title: "Scene 1",
								Conversations: []assets.Conversation{
									{Speaker: "A", Quote: "This is a **test phrase** here."},
								},
								Definitions: []assets.StoryNote{
									{
										Expression: "test phrase",
										Meaning:    "A phrase for testing",
										// Other fields will be empty strings/nil
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := newAssetsStoryConverter()
			result := converter.convertToAssetsStoryTemplate(notebooks)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestReader_ReadAllStoryNotebooksMap(t *testing.T) {
	tests := []struct {
		name            string
		setupFunc       func(t *testing.T) string
		expectedIndexes []string
		expectedError   bool
	}{
		{
			name: "Successfully reads multiple notebooks",
			setupFunc: func(t *testing.T) string {
				tempDir := t.TempDir()

				// Create first notebook
				notebook1Dir := filepath.Join(tempDir, "notebook1")
				require.NoError(t, os.MkdirAll(notebook1Dir, 0755))

				index1 := Index{
					Kind:          "story",
					ID:            "notebook1",
					Name:          "First Notebook",
					NotebookPaths: []string{"stories.yml"},
				}
				indexPath1 := filepath.Join(notebook1Dir, "index.yml")
				require.NoError(t, WriteYamlFile(indexPath1, index1))

				stories1 := []StoryNotebook{
					{
						Event: "Story 1",
						Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						Scenes: []StoryScene{
							{
								Title: "Scene 1",
								Conversations: []Conversation{
									{Speaker: "A", Quote: "Test quote"},
								},
								Definitions: []Note{
									{Expression: "test", Meaning: "test meaning"},
								},
							},
						},
					},
				}
				storiesPath1 := filepath.Join(notebook1Dir, "stories.yml")
				require.NoError(t, WriteYamlFile(storiesPath1, stories1))

				// Create second notebook
				notebook2Dir := filepath.Join(tempDir, "notebook2")
				require.NoError(t, os.MkdirAll(notebook2Dir, 0755))

				index2 := Index{
					Kind:          "story",
					ID:            "notebook2",
					Name:          "Second Notebook",
					NotebookPaths: []string{"stories.yml"},
				}
				indexPath2 := filepath.Join(notebook2Dir, "index.yml")
				require.NoError(t, WriteYamlFile(indexPath2, index2))

				stories2 := []StoryNotebook{
					{
						Event: "Story 2",
						Date:  time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
						Scenes: []StoryScene{
							{
								Title: "Scene 2",
								Conversations: []Conversation{
									{Speaker: "B", Quote: "Another quote"},
								},
								Definitions: []Note{
									{Expression: "word", Meaning: "word meaning"},
								},
							},
						},
					},
				}
				storiesPath2 := filepath.Join(notebook2Dir, "stories.yml")
				require.NoError(t, WriteYamlFile(storiesPath2, stories2))

				return tempDir
			},
			expectedIndexes: []string{"notebook1", "notebook2"},
			expectedError:   false,
		},
		{
			name: "Returns empty map when no notebooks exist",
			setupFunc: func(t *testing.T) string {
				return t.TempDir()
			},
			expectedIndexes: []string{},
			expectedError:   false,
		},
		{
			name: "Successfully reads single notebook",
			setupFunc: func(t *testing.T) string {
				tempDir := t.TempDir()

				notebookDir := filepath.Join(tempDir, "single-notebook")
				require.NoError(t, os.MkdirAll(notebookDir, 0755))

				index := Index{
					Kind:          "story",
					ID:            "single",
					Name:          "Single Notebook",
					NotebookPaths: []string{"stories.yml"},
				}
				indexPath := filepath.Join(notebookDir, "index.yml")
				require.NoError(t, WriteYamlFile(indexPath, index))

				stories := []StoryNotebook{
					{
						Event: "Single Story",
						Date:  time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
						Scenes: []StoryScene{
							{
								Title: "Single Scene",
								Conversations: []Conversation{
									{Speaker: "C", Quote: "Single quote"},
								},
								Definitions: []Note{
									{Expression: "single", Meaning: "single meaning"},
								},
							},
						},
					},
				}
				storiesPath := filepath.Join(notebookDir, "stories.yml")
				require.NoError(t, WriteYamlFile(storiesPath, stories))

				return tempDir
			},
			expectedIndexes: []string{"single"},
			expectedError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := tt.setupFunc(t)

			reader, err := NewReader(tempDir, "", map[string]rapidapi.Response{})
			require.NoError(t, err)

			result, err := reader.ReadAllStoryNotebooksMap()

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, len(tt.expectedIndexes), len(result))

			for _, expectedID := range tt.expectedIndexes {
				notebooks, ok := result[expectedID]
				assert.True(t, ok, "Expected notebook ID %s to be in result", expectedID)
				assert.NotEmpty(t, notebooks, "Expected notebooks for ID %s to not be empty", expectedID)
			}
		})
	}
}
