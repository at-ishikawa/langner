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

func TestNewStoryNotebookWriter(t *testing.T) {
	reader, err := NewReader(nil, nil, nil, nil, nil)
	require.NoError(t, err)

	writer := NewStoryNotebookWriter(reader, "template.md")
	assert.NotNil(t, writer)
	assert.Equal(t, reader, writer.reader)
	assert.Equal(t, "template.md", writer.templatePath)
}

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
		{
			name:             "Unknown conversion style returns plain text",
			text:             "I have {{ test phrase }} here.",
			conversionStyle:  ConversionStyle(99),
			targetExpression: "",
			expected:         "I have test phrase here.",
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
								Statements: []string{},
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

func TestFilterStoryNotebooks(t *testing.T) {
	tests := []struct {
		name                    string
		storyNotebooks          []StoryNotebook
		learningHistory         []LearningHistory
		dictionaryMap           map[string]rapidapi.Response
		sortDesc                bool
		includeNoCorrectAnswers bool
		useSpacedRepetition     bool
		preserveOrder           bool
		expectedWordCount       int
		expectedWords           []string
		expectedEventOrder      []string // if non-nil, verify notebook event order
		wantErr                 bool
		wantErrMsg              string
	}{
		{
			name: "empty expression returns error",
			storyNotebooks: []StoryNotebook{
				{
					Event: "Story 1",
					Scenes: []StoryScene{
						{
							Title:         "Scene 1",
							Conversations: []Conversation{{Speaker: "A", Quote: "test"}},
							Definitions:   []Note{{Expression: "   ", Meaning: "test"}},
						},
					},
				},
			},
			includeNoCorrectAnswers: true,
			wantErr:                 true,
			wantErrMsg:              "empty definition.Expression",
		},
		{
			name: "empty conversations and statements returns error",
			storyNotebooks: []StoryNotebook{
				{
					Event: "Story 1",
					Scenes: []StoryScene{
						{
							Title:       "Scene 1",
							Definitions: []Note{{Expression: "test", Meaning: "a trial"}},
						},
					},
				},
			},
			includeNoCorrectAnswers: true,
			useSpacedRepetition:     true,
			wantErr:                 true,
			wantErrMsg:              "empty scene.Conversations and Statements",
		},
		{
			name: "notebook with no scenes is skipped",
			storyNotebooks: []StoryNotebook{
				{Event: "Empty", Scenes: []StoryScene{}},
			},
			includeNoCorrectAnswers: true,
			expectedWordCount:       0,
		},
		{
			name: "includeNoCorrectAnswers false filters words without correct answers",
			storyNotebooks: []StoryNotebook{
				{
					Event: "Story 1",
					Scenes: []StoryScene{
						{
							Title:         "Scene 1",
							Conversations: []Conversation{{Speaker: "A", Quote: "test word"}},
							Definitions:   []Note{{Expression: "test", Meaning: "a trial"}},
						},
					},
				},
			},
			includeNoCorrectAnswers: false,
			expectedWordCount:       0,
		},
		{
			name: "sort descending with multiple notebooks",
			storyNotebooks: []StoryNotebook{
				{
					Event: "Story 1",
					Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []StoryScene{
						{
							Title:         "Scene 1",
							Conversations: []Conversation{{Speaker: "A", Quote: "first test word"}},
							Definitions:   []Note{{Expression: "first", Meaning: "first"}},
						},
					},
				},
				{
					Event: "Story 2",
					Date:  time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []StoryScene{
						{
							Title:         "Scene 2",
							Conversations: []Conversation{{Speaker: "B", Quote: "second test word"}},
							Definitions:   []Note{{Expression: "second", Meaning: "second"}},
						},
					},
				},
			},
			sortDesc:                true,
			includeNoCorrectAnswers: true,
			useSpacedRepetition:     true,
			expectedWordCount:       2,
			expectedWords:           []string{"second", "first"},
			expectedEventOrder:      []string{"Story 2", "Story 1"},
		},
		{
			name: "sort ascending with multiple notebooks",
			storyNotebooks: []StoryNotebook{
				{
					Event: "Story 2",
					Date:  time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []StoryScene{
						{
							Title:         "Scene 2",
							Conversations: []Conversation{{Speaker: "B", Quote: "second test word"}},
							Definitions:   []Note{{Expression: "second", Meaning: "second"}},
						},
					},
				},
				{
					Event: "Story 1",
					Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []StoryScene{
						{
							Title:         "Scene 1",
							Conversations: []Conversation{{Speaker: "A", Quote: "first test word"}},
							Definitions:   []Note{{Expression: "first", Meaning: "first"}},
						},
					},
				},
			},
			includeNoCorrectAnswers: true,
			useSpacedRepetition:     true,
			expectedWordCount:       2,
			expectedWords:           []string{"first", "second"},
			expectedEventOrder:      []string{"Story 1", "Story 2"},
		},
		{
			name: "preserveOrder skips sorting",
			storyNotebooks: []StoryNotebook{
				{
					Event: "Story 2",
					Date:  time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []StoryScene{
						{
							Title:         "Scene 2",
							Conversations: []Conversation{{Speaker: "B", Quote: "second test word"}},
							Definitions:   []Note{{Expression: "second", Meaning: "second"}},
						},
					},
				},
				{
					Event: "Story 1",
					Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []StoryScene{
						{
							Title:         "Scene 1",
							Conversations: []Conversation{{Speaker: "A", Quote: "first test word"}},
							Definitions:   []Note{{Expression: "first", Meaning: "first"}},
						},
					},
				},
			},
			includeNoCorrectAnswers: true,
			useSpacedRepetition:     true,
			preserveOrder:           true,
			expectedWordCount:       2,
			expectedWords:           []string{"second", "first"},
			expectedEventOrder:      []string{"Story 2", "Story 1"},
		},
		{
			name: "setDetails error with out-of-range dictionary number",
			storyNotebooks: []StoryNotebook{
				{
					Event: "Story 1",
					Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []StoryScene{
						{
							Title:         "Scene 1",
							Conversations: []Conversation{{Speaker: "A", Quote: "This is a test word"}},
							Definitions: []Note{
								{Expression: "test", Meaning: "test meaning", DictionaryNumber: 5},
							},
						},
					},
				},
			},
			dictionaryMap: map[string]rapidapi.Response{
				"test": {
					Word: "test",
					Results: []rapidapi.Result{
						{Definition: "a trial"},
					},
				},
			},
			includeNoCorrectAnswers: true,
			useSpacedRepetition:     true,
			wantErr:                 true,
			wantErrMsg:              "definition.setDetails()",
		},
		{
			name: "useSpacedRepetition=false, usable status - word NOT included",
			storyNotebooks: []StoryNotebook{
				{
					Event: "Story 1",
					Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []StoryScene{
						{
							Title: "Scene 1",
							Conversations: []Conversation{
								{Speaker: "A", Quote: "This is a test word"},
							},
							Definitions: []Note{
								{Expression: "test", Meaning: "test meaning"},
							},
						},
					},
				},
			},
			learningHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "notebook1",
						Title:      "Story 1",
					},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{Title: "Scene 1"},
							Expressions: []LearningHistoryExpression{
								{
									Expression: "test",
									LearnedLogs: []LearningRecord{
										{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(time.Now().Add(-60 * 24 * time.Hour))},
									},
								},
							},
						},
					},
				},
			},
			sortDesc:                false,
			includeNoCorrectAnswers: true,
			useSpacedRepetition:     false,
			expectedWordCount:       0,
			expectedWords:           nil,
		},
		{
			name: "useSpacedRepetition=true, usable status past interval - word INCLUDED",
			storyNotebooks: []StoryNotebook{
				{
					Event: "Story 1",
					Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []StoryScene{
						{
							Title: "Scene 1",
							Conversations: []Conversation{
								{Speaker: "A", Quote: "This is a test word"},
							},
							Definitions: []Note{
								{Expression: "test", Meaning: "test meaning"},
							},
						},
					},
				},
			},
			learningHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "notebook1",
						Title:      "Story 1",
					},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{Title: "Scene 1"},
							Expressions: []LearningHistoryExpression{
								{
									Expression: "test",
									LearnedLogs: []LearningRecord{
										// 1 correct answer, threshold is 3 days, 4 days ago - should need learning
										{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(time.Now().Add(-4 * 24 * time.Hour))},
									},
								},
							},
						},
					},
				},
			},
			sortDesc:                false,
			includeNoCorrectAnswers: true,
			useSpacedRepetition:     true,
			expectedWordCount:       1,
			expectedWords:           []string{"test"},
		},
		{
			name: "useSpacedRepetition=true, usable status NOT past interval - word NOT included",
			storyNotebooks: []StoryNotebook{
				{
					Event: "Story 1",
					Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []StoryScene{
						{
							Title: "Scene 1",
							Conversations: []Conversation{
								{Speaker: "A", Quote: "This is a test word"},
							},
							Definitions: []Note{
								{Expression: "test", Meaning: "test meaning"},
							},
						},
					},
				},
			},
			learningHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "notebook1",
						Title:      "Story 1",
					},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{Title: "Scene 1"},
							Expressions: []LearningHistoryExpression{
								{
									Expression: "test",
									LearnedLogs: []LearningRecord{
										// 1 correct answer, threshold is 3 days, 1 day ago - should NOT need learning
										{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(time.Now().Add(-1 * 24 * time.Hour))},
									},
								},
							},
						},
					},
				},
			},
			sortDesc:                false,
			includeNoCorrectAnswers: true,
			useSpacedRepetition:     true,
			expectedWordCount:       0,
			expectedWords:           nil,
		},
		{
			name: "Both modes - misunderstood status always included (useSpacedRepetition=false)",
			storyNotebooks: []StoryNotebook{
				{
					Event: "Story 1",
					Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []StoryScene{
						{
							Title: "Scene 1",
							Conversations: []Conversation{
								{Speaker: "A", Quote: "This is a test word"},
							},
							Definitions: []Note{
								{Expression: "test", Meaning: "test meaning"},
							},
						},
					},
				},
			},
			learningHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "notebook1",
						Title:      "Story 1",
					},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{Title: "Scene 1"},
							Expressions: []LearningHistoryExpression{
								{
									Expression: "test",
									LearnedLogs: []LearningRecord{
										{Status: LearnedStatusMisunderstood, LearnedAt: NewDate(time.Now())},
									},
								},
							},
						},
					},
				},
			},
			sortDesc:                false,
			includeNoCorrectAnswers: true,
			useSpacedRepetition:     false,
			expectedWordCount:       1,
			expectedWords:           []string{"test"},
		},
		{
			name: "Both modes - misunderstood status always included (useSpacedRepetition=true)",
			storyNotebooks: []StoryNotebook{
				{
					Event: "Story 1",
					Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []StoryScene{
						{
							Title: "Scene 1",
							Conversations: []Conversation{
								{Speaker: "A", Quote: "This is a test word"},
							},
							Definitions: []Note{
								{Expression: "test", Meaning: "test meaning"},
							},
						},
					},
				},
			},
			learningHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "notebook1",
						Title:      "Story 1",
					},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{Title: "Scene 1"},
							Expressions: []LearningHistoryExpression{
								{
									Expression: "test",
									LearnedLogs: []LearningRecord{
										{Status: LearnedStatusMisunderstood, LearnedAt: NewDate(time.Now())},
									},
								},
							},
						},
					},
				},
			},
			sortDesc:                false,
			includeNoCorrectAnswers: true,
			useSpacedRepetition:     true,
			expectedWordCount:       1,
			expectedWords:           []string{"test"},
		},
		{
			name: "Both modes - no learning history always included (useSpacedRepetition=false)",
			storyNotebooks: []StoryNotebook{
				{
					Event: "Story 1",
					Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []StoryScene{
						{
							Title: "Scene 1",
							Conversations: []Conversation{
								{Speaker: "A", Quote: "This is a test word"},
							},
							Definitions: []Note{
								{Expression: "test", Meaning: "test meaning"},
							},
						},
					},
				},
			},
			learningHistory:         []LearningHistory{},
			sortDesc:                false,
			includeNoCorrectAnswers: true,
			useSpacedRepetition:     false,
			expectedWordCount:       1,
			expectedWords:           []string{"test"},
		},
		{
			name: "Both modes - no learning history always included (useSpacedRepetition=true)",
			storyNotebooks: []StoryNotebook{
				{
					Event: "Story 1",
					Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []StoryScene{
						{
							Title: "Scene 1",
							Conversations: []Conversation{
								{Speaker: "A", Quote: "This is a test word"},
							},
							Definitions: []Note{
								{Expression: "test", Meaning: "test meaning"},
							},
						},
					},
				},
			},
			learningHistory:         []LearningHistory{},
			sortDesc:                false,
			includeNoCorrectAnswers: true,
			useSpacedRepetition:     true,
			expectedWordCount:       1,
			expectedWords:           []string{"test"},
		},
		{
			name: "duplicate expression entries - first empty, second usable - word NOT included",
			storyNotebooks: []StoryNotebook{
				{
					Event: "Story 1",
					Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Scenes: []StoryScene{
						{
							Title: "Scene 1",
							Conversations: []Conversation{
								{Speaker: "A", Quote: "I need to break the ice"},
							},
							Definitions: []Note{
								{Expression: "break the ice", Definition: "break someone's ice", Meaning: "initiate conversation"},
							},
						},
					},
				},
			},
			learningHistory: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "notebook1",
						Title:      "Story 1",
					},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{Title: "Scene 1"},
							Expressions: []LearningHistoryExpression{
								{
									Expression:  "break the ice",
									LearnedLogs: []LearningRecord{},
								},
								{
									Expression: "break someone's ice",
									LearnedLogs: []LearningRecord{
										{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(time.Now().Add(-60 * 24 * time.Hour))},
									},
								},
							},
						},
					},
				},
			},
			sortDesc:                false,
			includeNoCorrectAnswers: true,
			useSpacedRepetition:     false,
			expectedWordCount:       0,
			expectedWords:           nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dictionaryMap := tt.dictionaryMap
			if dictionaryMap == nil {
				dictionaryMap = map[string]rapidapi.Response{}
			}

			result, err := FilterStoryNotebooks(
				tt.storyNotebooks,
				tt.learningHistory,
				dictionaryMap,
				tt.sortDesc,
				tt.includeNoCorrectAnswers,
				tt.useSpacedRepetition,
				tt.preserveOrder,
			)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tt.wantErrMsg)
				}
				return
			}
			require.NoError(t, err)

			if tt.expectedEventOrder != nil {
				require.Len(t, result, len(tt.expectedEventOrder))
				for i, expectedEvent := range tt.expectedEventOrder {
					assert.Equal(t, expectedEvent, result[i].Event)
				}
			}

			// Count total words
			var wordCount int
			var words []string
			for _, notebook := range result {
				for _, scene := range notebook.Scenes {
					for _, definition := range scene.Definitions {
						wordCount++
						words = append(words, definition.Expression)
					}
				}
			}

			assert.Equal(t, tt.expectedWordCount, wordCount, "Expected %d words, got %d", tt.expectedWordCount, wordCount)
			assert.Equal(t, tt.expectedWords, words, "Expected words %v, got %v", tt.expectedWords, words)
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

			reader, err := NewReader([]string{tempDir}, []string{}, nil, nil, map[string]rapidapi.Response{})
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

func TestOutputStoryNotebooks(t *testing.T) {
	storiesDir := t.TempDir()
	outputDir := t.TempDir()

	// Create story data
	storyDir := filepath.Join(storiesDir, "test-story")
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, WriteYamlFile(filepath.Join(storyDir, "index.yml"), Index{
		Kind: "story", ID: "test-story", Name: "Test Story",
		NotebookPaths: []string{"stories.yml"},
	}))
	require.NoError(t, WriteYamlFile(filepath.Join(storyDir, "stories.yml"), []StoryNotebook{
		{
			Event: "Episode 1",
			Date:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Scenes: []StoryScene{
				{
					Title:         "Scene 1",
					Conversations: []Conversation{{Speaker: "A", Quote: "A {{ tricky }} situation."}},
					Definitions:   []Note{{Expression: "tricky", Meaning: "difficult to deal with"}},
				},
			},
		},
	}))

	reader, err := NewReader([]string{storiesDir}, nil, nil, nil, nil)
	require.NoError(t, err)

	writer := NewStoryNotebookWriter(reader, "")

	t.Run("success", func(t *testing.T) {
		learningHistories := map[string][]LearningHistory{
			"test-story": {
				{
					Metadata: LearningHistoryMetadata{NotebookID: "test-story", Title: "Episode 1"},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{Title: "Scene 1"},
							Expressions: []LearningHistoryExpression{
								{Expression: "tricky", LearnedLogs: []LearningRecord{{Status: "misunderstood", LearnedAt: NewDate(time.Now())}}},
							},
						},
					},
				},
			},
		}

		err := writer.OutputStoryNotebooks("test-story", map[string]rapidapi.Response{}, learningHistories, false, outputDir, false)
		require.NoError(t, err)

		outputFile := filepath.Join(outputDir, "test-story.md")
		_, err = os.Stat(outputFile)
		assert.NoError(t, err)
	})

	t.Run("story not found", func(t *testing.T) {
		err := writer.OutputStoryNotebooks("nonexistent", map[string]rapidapi.Response{}, map[string][]LearningHistory{}, false, outputDir, false)
		assert.Error(t, err)
	})

	t.Run("empty notebooks after filter", func(t *testing.T) {
		// Use a learning history that filters out all words
		learningHistories := map[string][]LearningHistory{
			"test-story": {
				{
					Metadata: LearningHistoryMetadata{NotebookID: "test-story", Title: "Episode 1"},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{Title: "Scene 1"},
							Expressions: []LearningHistoryExpression{
								{
									Expression: "tricky",
									LearnedLogs: []LearningRecord{
										{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(time.Now().Add(-1 * 24 * time.Hour))},
									},
								},
							},
						},
					},
				},
			},
		}

		// This should produce an output even if all words are filtered (no definitions remaining)
		err := writer.OutputStoryNotebooks("test-story", map[string]rapidapi.Response{}, learningHistories, true, outputDir, false)
		// No error - it just writes a notebook with no definitions
		assert.NoError(t, err)
	})
}

func TestStoryScene_Validate(t *testing.T) {
	tests := []struct {
		name       string
		scene      StoryScene
		location   string
		wantErrors int
		wantMsg    string
	}{
		{
			name: "expression found with marker in conversation",
			scene: StoryScene{
				Conversations: []Conversation{{Speaker: "A", Quote: "The {{ eager }} student."}},
				Definitions:   []Note{{Expression: "eager", Meaning: "wanting to do something"}},
			},
			location:   "test",
			wantErrors: 0,
		},
		{
			name: "expression found without marker in conversation",
			scene: StoryScene{
				Conversations: []Conversation{{Speaker: "A", Quote: "The eager student."}},
				Definitions:   []Note{{Expression: "eager", Meaning: "wanting to do something"}},
			},
			location:   "test",
			wantErrors: 1,
			wantMsg:    "missing {{ }} markers",
		},
		{
			name: "expression not found at all",
			scene: StoryScene{
				Conversations: []Conversation{{Speaker: "A", Quote: "A completely different sentence."}},
				Definitions:   []Note{{Expression: "eager", Meaning: "wanting to do something"}},
			},
			location:   "test",
			wantErrors: 1,
			wantMsg:    "not found in any conversation",
		},
		{
			name: "expression marked as not_used",
			scene: StoryScene{
				Conversations: []Conversation{{Speaker: "A", Quote: "A different sentence."}},
				Definitions:   []Note{{Expression: "eager", Meaning: "wanting to do something", NotUsed: true}},
			},
			location:   "test",
			wantErrors: 0,
		},
		{
			name: "empty expression is skipped",
			scene: StoryScene{
				Conversations: []Conversation{{Speaker: "A", Quote: "A sentence."}},
				Definitions:   []Note{{Expression: "  ", Meaning: "blank"}},
			},
			location:   "test",
			wantErrors: 0,
		},
		{
			name: "expression found with marker in statement",
			scene: StoryScene{
				Statements:  []string{"The {{ eager }} student arrived."},
				Definitions: []Note{{Expression: "eager", Meaning: "wanting to do something"}},
			},
			location:   "test",
			wantErrors: 0,
		},
		{
			name: "expression found without marker in statement",
			scene: StoryScene{
				Statements:  []string{"The eager student arrived."},
				Definitions: []Note{{Expression: "eager", Meaning: "wanting to do something"}},
			},
			location:   "test",
			wantErrors: 1,
			wantMsg:    "missing {{ }} markers",
		},
		{
			name: "expression not found in statement either",
			scene: StoryScene{
				Statements:  []string{"A completely different statement."},
				Definitions: []Note{{Expression: "eager", Meaning: "wanting to do something"}},
			},
			location:   "test",
			wantErrors: 1,
			wantMsg:    "not found in any conversation",
		},
		{
			name: "expression with {{}} no-space markers",
			scene: StoryScene{
				Conversations: []Conversation{{Speaker: "A", Quote: "The {{eager}} student."}},
				Definitions:   []Note{{Expression: "eager", Meaning: "wanting to do something"}},
			},
			location:   "test",
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := tt.scene.Validate(tt.location)
			assert.Len(t, errors, tt.wantErrors)
			if tt.wantMsg != "" && len(errors) > 0 {
				assert.Contains(t, errors[0].Message, tt.wantMsg)
			}
		})
	}
}

func TestStoryNotebook_Validate(t *testing.T) {
	t.Run("no errors", func(t *testing.T) {
		nb := StoryNotebook{
			Event: "Episode 1",
			Scenes: []StoryScene{
				{
					Title:         "Scene 1",
					Conversations: []Conversation{{Speaker: "A", Quote: "The {{ eager }} student."}},
					Definitions:   []Note{{Expression: "eager", Meaning: "wanting to do something"}},
				},
			},
		}
		errors := nb.Validate("test")
		assert.Empty(t, errors)
	})

	t.Run("scene validation errors", func(t *testing.T) {
		nb := StoryNotebook{
			Event: "Episode 1",
			Scenes: []StoryScene{
				{
					Title:         "Scene 1",
					Conversations: []Conversation{{Speaker: "A", Quote: "No match here."}},
					Definitions:   []Note{{Expression: "missing", Meaning: "not here"}},
				},
			},
		}
		errors := nb.Validate("test")
		assert.NotEmpty(t, errors)
	})
}
