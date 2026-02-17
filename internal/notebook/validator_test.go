package notebook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      ValidationError
		expected string
	}{
		{
			name: "error with location and suggestions",
			err: ValidationError{
				File:        "test.yml",
				Location:    "scene[0]",
				Message:     "invalid status",
				Suggestions: []string{"use 'understood'", "use 'usable'"},
			},
			expected: "test.yml (scene[0]): invalid status [Suggestion: use 'understood'; use 'usable']",
		},
		{
			name: "error without location",
			err: ValidationError{
				File:    "test.yml",
				Message: "file not found",
			},
			expected: "test.yml: file not found",
		},
		{
			name: "error without suggestions",
			err: ValidationError{
				File:     "test.yml",
				Location: "line 5",
				Message:  "syntax error",
			},
			expected: "test.yml (line 5): syntax error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestValidationResult_HasErrors(t *testing.T) {
	tests := []struct {
		name     string
		result   ValidationResult
		expected bool
	}{
		{
			name:     "no errors",
			result:   ValidationResult{},
			expected: false,
		},
		{
			name: "has learning notes errors",
			result: ValidationResult{
				LearningNotesErrors: []ValidationError{{Message: "error"}},
			},
			expected: true,
		},
		{
			name: "has consistency errors",
			result: ValidationResult{
				ConsistencyErrors: []ValidationError{{Message: "error"}},
			},
			expected: true,
		},
		{
			name: "has only warnings",
			result: ValidationResult{
				Warnings: []ValidationError{{Message: "warning"}},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.result.HasErrors())
		})
	}
}

func TestValidationResult_AddError(t *testing.T) {
	result := &ValidationResult{}

	learningErr := ValidationError{Message: "learning error"}
	result.AddError("learning_notes", learningErr)

	consistencyErr := ValidationError{Message: "consistency error"}
	result.AddError("consistency", consistencyErr)

	assert.Len(t, result.LearningNotesErrors, 1)
	assert.Equal(t, "error", result.LearningNotesErrors[0].Severity)
	assert.Len(t, result.ConsistencyErrors, 1)
	assert.Equal(t, "error", result.ConsistencyErrors[0].Severity)
}

func TestValidationResult_AddWarning(t *testing.T) {
	result := &ValidationResult{}

	warning := ValidationError{Message: "warning"}
	result.AddWarning(warning)

	assert.Len(t, result.Warnings, 1)
	assert.Equal(t, "warning", result.Warnings[0].Severity)
}

func TestValidator_validateLearningNotesStructure(t *testing.T) {
	tests := []struct {
		name                    string
		files                   []learningHistoryFile
		expectedErrorCount      int
		expectedWarningCount    int
		errorMessageContains    []string
		warningMessageContains  []string
	}{
		{
			name: "valid learning notes",
			files: []learningHistoryFile{
				{
					path: "test.yml",
					contents: []LearningHistory{
						{
							Metadata: LearningHistoryMetadata{Title: "Test Episode"},
							Scenes: []LearningScene{
								{
									Metadata: LearningSceneMetadata{Title: "Scene 1"},
									Expressions: []LearningHistoryExpression{
										{
											Expression: "test word",
											LearnedLogs: []LearningRecord{
												{Status: learnedStatusUnderstood, LearnedAt: NewDate(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC))},
												{Status: learnedStatusLearning, LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount:   0,
			expectedWarningCount: 0,
		},
		{
			name: "empty expression",
			files: []learningHistoryFile{
				{
					path: "test.yml",
					contents: []LearningHistory{
						{
							Metadata: LearningHistoryMetadata{Title: "Test Episode"},
							Scenes: []LearningScene{
								{
									Metadata: LearningSceneMetadata{Title: "Scene 1"},
									Expressions: []LearningHistoryExpression{
										{Expression: ""},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount:   1,
			errorMessageContains: []string{"expression field is empty"},
		},
		{
			name: "invalid status",
			files: []learningHistoryFile{
				{
					path: "test.yml",
					contents: []LearningHistory{
						{
							Metadata: LearningHistoryMetadata{Title: "Test Episode"},
							Scenes: []LearningScene{
								{
									Metadata: LearningSceneMetadata{Title: "Scene 1"},
									Expressions: []LearningHistoryExpression{
										{
											Expression: "test",
											LearnedLogs: []LearningRecord{
												{Status: LearnedStatus("invalid"), LearnedAt: NewDate(time.Now())},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount:   1,
			errorMessageContains: []string{"invalid status"},
		},
		{
			name: "missing learned_at",
			files: []learningHistoryFile{
				{
					path: "test.yml",
					contents: []LearningHistory{
						{
							Metadata: LearningHistoryMetadata{Title: "Test Episode"},
							Scenes: []LearningScene{
								{
									Metadata: LearningSceneMetadata{Title: "Scene 1"},
									Expressions: []LearningHistoryExpression{
										{
											Expression: "test",
											LearnedLogs: []LearningRecord{
												{Status: learnedStatusUnderstood, LearnedAt: Date{}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount:   1,
			errorMessageContains: []string{"learned_at is required"},
		},
		{
			name: "wrong chronological order",
			files: []learningHistoryFile{
				{
					path: "test.yml",
					contents: []LearningHistory{
						{
							Metadata: LearningHistoryMetadata{Title: "Test Episode"},
							Scenes: []LearningScene{
								{
									Metadata: LearningSceneMetadata{Title: "Scene 1"},
									Expressions: []LearningHistoryExpression{
										{
											Expression: "test",
											LearnedLogs: []LearningRecord{
												{Status: learnedStatusLearning, LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
												{Status: learnedStatusUnderstood, LearnedAt: NewDate(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC))},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount:   1,
			errorMessageContains: []string{"not in chronological order"},
		},
		{
			name: "no learned_logs warning",
			files: []learningHistoryFile{
				{
					path: "test.yml",
					contents: []LearningHistory{
						{
							Metadata: LearningHistoryMetadata{Title: "Test Episode"},
							Scenes: []LearningScene{
								{
									Metadata: LearningSceneMetadata{Title: "Scene 1"},
									Expressions: []LearningHistoryExpression{
										{
											Expression:  "test",
											LearnedLogs: []LearningRecord{},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount:     0,
			expectedWarningCount:   0, // No longer warn about empty learned_logs
			warningMessageContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &Validator{}
			result := &ValidationResult{}

			validator.validateLearningNotesStructure(tt.files, result)

			assert.Len(t, result.LearningNotesErrors, tt.expectedErrorCount)
			assert.Len(t, result.Warnings, tt.expectedWarningCount)

			for _, contains := range tt.errorMessageContains {
				found := false
				for _, err := range result.LearningNotesErrors {
					if strings.Contains(err.Message, contains) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected error message to contain: %s", contains)
			}

			for _, contains := range tt.warningMessageContains {
				found := false
				for _, warn := range result.Warnings {
					if strings.Contains(warn.Message, contains) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected warning message to contain: %s", contains)
			}
		})
	}
}

func TestValidator_validateConsistency(t *testing.T) {
	tests := []struct {
		name                 string
		learningFiles        []learningHistoryFile
		storyFiles           []storyNotebookFile
		expectedErrorCount   int
		expectedWarningCount int
		errorMessageContains []string
	}{
		{
			name: "matching expressions",
			learningFiles: []learningHistoryFile{
				{
					path: "learning.yml",
					contents: []LearningHistory{
						{
							Metadata: LearningHistoryMetadata{Title: "Episode 1"},
							Scenes: []LearningScene{
								{
									Metadata: LearningSceneMetadata{Title: "Scene 1"},
									Expressions: []LearningHistoryExpression{
										{Expression: "test word"},
									},
								},
							},
						},
					},
				},
			},
			storyFiles: []storyNotebookFile{
				{
					path: "story.yml",
					contents: []StoryNotebook{
						{
							Event: "Episode 1",
							Scenes: []StoryScene{
								{
									Title: "Scene 1",
									Definitions: []Note{
										{Expression: "test word"},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount: 0,
		},
		{
			name: "orphaned learning note",
			learningFiles: []learningHistoryFile{
				{
					path: "learning.yml",
					contents: []LearningHistory{
						{
							Metadata: LearningHistoryMetadata{Title: "Episode 1"},
							Scenes: []LearningScene{
								{
									Metadata: LearningSceneMetadata{Title: "Scene 1"},
									Expressions: []LearningHistoryExpression{
										{Expression: "nonexistent word"},
									},
								},
							},
						},
					},
				},
			},
			storyFiles: []storyNotebookFile{
				{
					path: "story.yml",
					contents: []StoryNotebook{
						{
							Event: "Episode 1",
							Scenes: []StoryScene{
								{
									Title: "Scene 1",
									Definitions: []Note{
										{Expression: "different word"},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount:   1,
			expectedWarningCount: 1, // "different word" will generate a missing learning note warning
			errorMessageContains: []string{"orphaned learning note"},
		},
		{
			name: "duplicate expressions in same scene",
			learningFiles: []learningHistoryFile{
				{
					path: "learning.yml",
					contents: []LearningHistory{
						{
							Metadata: LearningHistoryMetadata{Title: "Episode 1"},
							Scenes: []LearningScene{
								{
									Metadata: LearningSceneMetadata{Title: "Scene 1"},
									Expressions: []LearningHistoryExpression{
										{Expression: "test word"},
										{Expression: "test word"},
									},
								},
							},
						},
					},
				},
			},
			storyFiles: []storyNotebookFile{
				{
					path: "story.yml",
					contents: []StoryNotebook{
						{
							Event: "Episode 1",
							Scenes: []StoryScene{
								{
									Title: "Scene 1",
									Definitions: []Note{
										{Expression: "test word"},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount:   1,
			errorMessageContains: []string{"duplicate expression"},
		},
		{
			name: "expression with definition match",
			learningFiles: []learningHistoryFile{
				{
					path: "learning.yml",
					contents: []LearningHistory{
						{
							Metadata: LearningHistoryMetadata{Title: "Episode 1"},
							Scenes: []LearningScene{
								{
									Metadata: LearningSceneMetadata{Title: "Scene 1"},
									Expressions: []LearningHistoryExpression{
										{Expression: "base form"},
									},
								},
							},
						},
					},
				},
			},
			storyFiles: []storyNotebookFile{
				{
					path: "story.yml",
					contents: []StoryNotebook{
						{
							Event: "Episode 1",
							Scenes: []StoryScene{
								{
									Title: "Scene 1",
									Definitions: []Note{
										{Expression: "full expression", Definition: "base form"},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount: 0,
		},
		{
			name:          "missing learning notes warning",
			learningFiles: []learningHistoryFile{},
			storyFiles: []storyNotebookFile{
				{
					path: "story.yml",
					contents: []StoryNotebook{
						{
							Event: "Episode 1",
							Scenes: []StoryScene{
								{
									Title: "Scene 1",
									Definitions: []Note{
										{Expression: "untracked word"},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount:   0,
			expectedWarningCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &Validator{}
			result := &ValidationResult{}

			validator.validateConsistency(tt.learningFiles, tt.storyFiles, result)

			assert.Len(t, result.ConsistencyErrors, tt.expectedErrorCount)
			assert.Len(t, result.Warnings, tt.expectedWarningCount)

			for _, contains := range tt.errorMessageContains {
				found := false
				for _, err := range result.ConsistencyErrors {
					if strings.Contains(err.Message, contains) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected error message to contain: %s", contains)
			}
		})
	}
}

func TestValidator_validateDictionaryReferences(t *testing.T) {
	// Create temp directory for test dictionaries
	tmpDir, err := os.MkdirTemp("", "dict-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test dictionary file
	dictPath := filepath.Join(tmpDir, "testword.json")
	err = os.WriteFile(dictPath, []byte(`{"word":"testword"}`), 0644)
	require.NoError(t, err)

	tests := []struct {
		name                 string
		storyFiles           []storyNotebookFile
		expectedErrorCount   int
		errorMessageContains []string
	}{
		{
			name: "valid dictionary reference",
			storyFiles: []storyNotebookFile{
				{
					path: "story.yml",
					contents: []StoryNotebook{
						{
							Event: "Episode 1",
							Scenes: []StoryScene{
								{
									Title: "Scene 1",
									Definitions: []Note{
										{Expression: "testword", DictionaryNumber: 1},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount: 0,
		},
		{
			name: "missing dictionary file",
			storyFiles: []storyNotebookFile{
				{
					path: "story.yml",
					contents: []StoryNotebook{
						{
							Event: "Episode 1",
							Scenes: []StoryScene{
								{
									Title: "Scene 1",
									Definitions: []Note{
										{Expression: "nonexistent", DictionaryNumber: 1},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount:   1,
			errorMessageContains: []string{"dictionary file not found"},
		},
		{
			name: "no dictionary number - no error",
			storyFiles: []storyNotebookFile{
				{
					path: "story.yml",
					contents: []StoryNotebook{
						{
							Event: "Episode 1",
							Scenes: []StoryScene{
								{
									Title: "Scene 1",
									Definitions: []Note{
										{Expression: "nonexistent", DictionaryNumber: 0},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &Validator{dictionaryDir: tmpDir}
			result := &ValidationResult{}

			validator.validateDictionaryReferences(tt.storyFiles, result)

			assert.Len(t, result.ConsistencyErrors, tt.expectedErrorCount)

			for _, contains := range tt.errorMessageContains {
				found := false
				for _, err := range result.ConsistencyErrors {
					if strings.Contains(err.Message, contains) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected error message to contain: %s", contains)
			}
		})
	}
}


func TestValidator_validateDefinitionsInConversations(t *testing.T) {
	tests := []struct {
		name                 string
		storyFiles           []storyNotebookFile
		expectedErrorCount   int
		errorMessageContains []string
	}{
		{
			name: "expression found with exact case match",
			storyFiles: []storyNotebookFile{
				{
					path: "story.yml",
					contents: []StoryNotebook{
						{
							Event: "Episode 1",
							Scenes: []StoryScene{
								{
									Title: "Scene 1",
									Conversations: []Conversation{
										{Speaker: "A", Quote: "This is a {{ test }} phrase"},
									},
									Definitions: []Note{
										{Expression: "test"},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount: 0,
		},
		{
			name: "expression found with different case - should accept",
			storyFiles: []storyNotebookFile{
				{
					path: "story.yml",
					contents: []StoryNotebook{
						{
							Event: "Episode 1",
							Scenes: []StoryScene{
								{
									Title: "Scene 1",
									Conversations: []Conversation{
										{Speaker: "A", Quote: "This is a {{ Test }} phrase"},
									},
									Definitions: []Note{
										{Expression: "test"},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount: 0, // Should not error on case mismatch
		},
		{
			name: "expression with markers but different case - should accept",
			storyFiles: []storyNotebookFile{
				{
					path: "story.yml",
					contents: []StoryNotebook{
						{
							Event: "Episode 1",
							Scenes: []StoryScene{
								{
									Title: "Scene 1",
									Conversations: []Conversation{
										{Speaker: "A", Quote: "{{ Steel your mind }} for the test"},
									},
									Definitions: []Note{
										{Expression: "steel your mind"},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount: 0, // Should accept case-insensitive match
		},
		{
			name: "expression not found in conversation",
			storyFiles: []storyNotebookFile{
				{
					path: "story.yml",
					contents: []StoryNotebook{
						{
							Event: "Episode 1",
							Scenes: []StoryScene{
								{
									Title: "Scene 1",
									Conversations: []Conversation{
										{Speaker: "A", Quote: "This is a phrase"},
									},
									Definitions: []Note{
										{Expression: "missing"},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount:   1,
			errorMessageContains: []string{"not found in any conversation"},
		},
		{
			name: "expression found without markers",
			storyFiles: []storyNotebookFile{
				{
					path: "story.yml",
					contents: []StoryNotebook{
						{
							Event: "Episode 1",
							Scenes: []StoryScene{
								{
									Title: "Scene 1",
									Conversations: []Conversation{
										{Speaker: "A", Quote: "This is a test phrase"},
									},
									Definitions: []Note{
										{Expression: "test"},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount:   1,
			errorMessageContains: []string{"missing {{ }} markers"},
		},
		{
			name: "expression marked as not_used - no error",
			storyFiles: []storyNotebookFile{
				{
					path: "story.yml",
					contents: []StoryNotebook{
						{
							Event: "Episode 1",
							Scenes: []StoryScene{
								{
									Title: "Scene 1",
									Conversations: []Conversation{
										{Speaker: "A", Quote: "This is a phrase"},
									},
									Definitions: []Note{
										{Expression: "unused", NotUsed: true},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount: 0,
		},
		{
			name: "inflected form with markers - should accept",
			storyFiles: []storyNotebookFile{
				{
					path: "story.yml",
					contents: []StoryNotebook{
						{
							Event: "Episode 1",
							Scenes: []StoryScene{
								{
									Title: "Scene 1",
									Conversations: []Conversation{
										{Speaker: "A", Quote: "The lands are {{ enveloped }} in cold"},
									},
									Definitions: []Note{
										{Expression: "enveloped", Definition: "envelop"},
									},
								},
							},
						},
					},
				},
			},
			expectedErrorCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &Validator{}
			result := &ValidationResult{}

			validator.validateDefinitionsInConversations(tt.storyFiles, result)

			assert.Len(t, result.ConsistencyErrors, tt.expectedErrorCount)

			for _, contains := range tt.errorMessageContains {
				found := false
				for _, err := range result.ConsistencyErrors {
					if strings.Contains(err.Message, contains) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected error message to contain: %s", contains)
			}
		})
	}
}

func TestDeriveNotebookID(t *testing.T) {
	tests := []struct {
		name     string
		notebook StoryNotebook
		want     string
	}{
		{
			name: "series name with spaces",
			notebook: StoryNotebook{
				Metadata: Metadata{Series: "Friends"},
			},
			want: "friends",
		},
		{
			name: "series name with multiple words",
			notebook: StoryNotebook{
				Metadata: Metadata{Series: "The Office"},
			},
			want: "the-office",
		},
		{
			name: "series name already lowercase",
			notebook: StoryNotebook{
				Metadata: Metadata{Series: "breaking bad"},
			},
			want: "breaking-bad",
		},
		{
			name: "empty series name",
			notebook: StoryNotebook{
				Metadata: Metadata{Series: ""},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveNotebookID(&tt.notebook)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidator_Fix(t *testing.T) {
	tests := []struct {
		name                  string
		existingLearningNotes []LearningHistory
		storyNotebook         []StoryNotebook
		wantWarningsCount     int
		wantExpressionsLen    int
		wantExpressions       []string
	}{
		{
			name:                  "creates missing learning notes",
			existingLearningNotes: nil,
			storyNotebook: []StoryNotebook{
				{
					Event: "Friends S01E01",
					Metadata: Metadata{
						Series:  "Friends",
						Season:  1,
						Episode: 1,
					},
					Scenes: []StoryScene{
						{
							Title: "Central Perk",
							Definitions: []Note{
								{Expression: "nothing"},
								{Expression: "going", Definition: "go"},
							},
						},
					},
				},
			},
			wantWarningsCount:  4, // new file + new episode + 2 expressions
			wantExpressionsLen: 2,
			wantExpressions:    []string{"nothing", "go"},
		},
		{
			name: "adds to existing learning notes",
			existingLearningNotes: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "friends",
						Title:      "Friends S01E01",
					},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{Title: "Central Perk"},
							Expressions: []LearningHistoryExpression{
								{Expression: "existing", LearnedLogs: []LearningRecord{}},
							},
						},
					},
				},
			},
			storyNotebook: []StoryNotebook{
				{
					Event: "Friends S01E01",
					Metadata: Metadata{
						Series:  "Friends",
						Season:  1,
						Episode: 1,
					},
					Scenes: []StoryScene{
						{
							Title: "Central Perk",
							Definitions: []Note{
								{Expression: "existing"},
								{Expression: "new expression"},
							},
						},
					},
				},
			},
			wantWarningsCount:  1, // only new expression
			wantExpressionsLen: 2,
			wantExpressions:    []string{"existing", "new expression"},
		},
		{
			name: "no changes needed",
			existingLearningNotes: []LearningHistory{
				{
					Metadata: LearningHistoryMetadata{
						NotebookID: "friends",
						Title:      "Friends S01E01",
					},
					Scenes: []LearningScene{
						{
							Metadata: LearningSceneMetadata{Title: "Central Perk"},
							Expressions: []LearningHistoryExpression{
								{Expression: "test", LearnedLogs: []LearningRecord{}},
							},
						},
					},
				},
			},
			storyNotebook: []StoryNotebook{
				{
					Event: "Friends S01E01",
					Metadata: Metadata{
						Series:  "Friends",
						Season:  1,
						Episode: 1,
					},
					Scenes: []StoryScene{
						{
							Title: "Central Perk",
							Definitions: []Note{
								{Expression: "test"},
							},
						},
					},
				},
			},
			wantWarningsCount:  0,
			wantExpressionsLen: 1,
			wantExpressions:    []string{"test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directories
			tmpDir, err := os.MkdirTemp("", "validator-fix-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			learningNotesDir := filepath.Join(tmpDir, "learning_notes")
			storiesDir := filepath.Join(tmpDir, "stories")
			dictionaryDir := filepath.Join(tmpDir, "dictionaries")

			require.NoError(t, os.MkdirAll(learningNotesDir, 0755))
			require.NoError(t, os.MkdirAll(storiesDir, 0755))
			require.NoError(t, os.MkdirAll(dictionaryDir, 0755))

			// Create existing learning notes if provided
			learningNotesPath := filepath.Join(learningNotesDir, "friends.yml")
			if tt.existingLearningNotes != nil {
				require.NoError(t, WriteYamlFile(learningNotesPath, tt.existingLearningNotes))
			}

			// Create story notebook file
			storyPath := filepath.Join(storiesDir, "friends.yml")
			require.NoError(t, WriteYamlFile(storyPath, tt.storyNotebook))

			// Create validator
			validator := NewValidator(learningNotesDir, []string{storiesDir}, []string{}, dictionaryDir)

			// Run Fix
			result, err := validator.Fix()
			require.NoError(t, err)

			// Verify warnings count
			assert.Len(t, result.Warnings, tt.wantWarningsCount)

			// Check that learning notes file exists
			assert.FileExists(t, learningNotesPath)

			// Read the learning notes
			learningHistories, err := readYamlFile[[]LearningHistory](learningNotesPath)
			require.NoError(t, err)

			// Verify structure
			require.Len(t, learningHistories, 1)
			assert.Equal(t, "friends", learningHistories[0].Metadata.NotebookID)
			assert.Equal(t, "Friends S01E01", learningHistories[0].Metadata.Title)

			require.Len(t, learningHistories[0].Scenes, 1)
			assert.Equal(t, "Central Perk", learningHistories[0].Scenes[0].Metadata.Title)

			// Verify expressions
			require.Len(t, learningHistories[0].Scenes[0].Expressions, tt.wantExpressionsLen)

			for i, wantExpr := range tt.wantExpressions {
				assert.Equal(t, wantExpr, learningHistories[0].Scenes[0].Expressions[i].Expression)
				assert.Empty(t, learningHistories[0].Scenes[0].Expressions[i].LearnedLogs)
			}
		})
	}
}

func TestExtractSeriesName(t *testing.T) {
	tests := []struct {
		name       string
		eventLower string
		want       string
	}{
		{
			name:       "event with season keyword",
			eventLower: "friends season 1 episode 1",
			want:       "friends",
		},
		{
			name:       "event with episode keyword",
			eventLower: "the office episode 5",
			want:       "the office",
		},
		{
			name:       "event without keywords",
			eventLower: "random title",
			want:       "",
		},
		{
			name:       "season at start",
			eventLower: "season 1",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSeriesName(tt.eventLower)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidator_EventsRelated(t *testing.T) {
	v := &Validator{}

	tests := []struct {
		name   string
		event1 string
		event2 string
		want   bool
	}{
		{
			name:   "same series with episode keyword",
			event1: "Friends Episode 1",
			event2: "Friends Episode 2",
			want:   true,
		},
		{
			name:   "same series with season keyword",
			event1: "Friends Season 1",
			event2: "Friends Season 2",
			want:   true,
		},
		{
			name:   "different series with episode keyword",
			event1: "Friends Episode 1",
			event2: "Seinfeld Episode 1",
			want:   false,
		},
		{
			name:   "no episode or season keywords",
			event1: "Random Title A",
			event2: "Random Title B",
			want:   false,
		},
		{
			name:   "one has episode, other does not",
			event1: "Friends Episode 1",
			event2: "Random Title",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := v.eventsRelated(tt.event1, tt.event2)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidator_ValidateFlashcardNotebooks(t *testing.T) {
	tests := []struct {
		name               string
		files              []flashcardNotebookFile
		expectedErrorCount int
	}{
		{
			name: "valid flashcard notebooks",
			files: []flashcardNotebookFile{
				{
					path: "test.yml",
					contents: []FlashcardNotebook{
						{
							Title: "Unit 1",
							Cards: []Note{
								{Expression: "hello", Meaning: "a greeting"},
							},
						},
					},
				},
			},
			expectedErrorCount: 0,
		},
		{
			name: "flashcard notebook with empty title",
			files: []flashcardNotebookFile{
				{
					path: "test.yml",
					contents: []FlashcardNotebook{
						{
							Title: "",
							Cards: []Note{
								{Expression: "hello", Meaning: "a greeting"},
							},
						},
					},
				},
			},
			expectedErrorCount: 1,
		},
		{
			name: "flashcard with empty expression",
			files: []flashcardNotebookFile{
				{
					path: "test.yml",
					contents: []FlashcardNotebook{
						{
							Title: "Unit 1",
							Cards: []Note{
								{Expression: "", Meaning: "a greeting"},
							},
						},
					},
				},
			},
			expectedErrorCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &Validator{}
			result := &ValidationResult{}

			v.validateFlashcardNotebooks(tt.files, result)

			assert.Len(t, result.LearningNotesErrors, tt.expectedErrorCount)
		})
	}
}

func TestValidator_LoadFlashcardNotebooks(t *testing.T) {
	// Create temp directory with flashcard YAML files
	tmpDir, err := os.MkdirTemp("", "flashcard-load-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a valid flashcard YAML file
	content := `- title: "Unit 1"
  date: 2025-01-15T00:00:00Z
  cards:
    - expression: "hello"
      meaning: "a greeting"
`
	err = os.WriteFile(filepath.Join(tmpDir, "cards.yml"), []byte(content), 0644)
	require.NoError(t, err)

	// Create index.yml (should be skipped)
	err = os.WriteFile(filepath.Join(tmpDir, "index.yml"), []byte("id: test\n"), 0644)
	require.NoError(t, err)

	v := &Validator{flashcardsDirs: []string{tmpDir}}

	files, err := v.loadFlashcardNotebooks()
	require.NoError(t, err)
	assert.Len(t, files, 1)
	assert.Len(t, files[0].contents, 1)
	assert.Equal(t, "Unit 1", files[0].contents[0].Title)
}

func TestValidator_Validate(t *testing.T) {
	// Create temp directories
	tmpDir, err := os.MkdirTemp("", "validator-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	learningNotesDir := filepath.Join(tmpDir, "learning_notes")
	storiesDir := filepath.Join(tmpDir, "stories")
	flashcardsDir := filepath.Join(tmpDir, "flashcards")
	dictionaryDir := filepath.Join(tmpDir, "dictionaries")

	require.NoError(t, os.MkdirAll(learningNotesDir, 0755))
	require.NoError(t, os.MkdirAll(storiesDir, 0755))
	require.NoError(t, os.MkdirAll(flashcardsDir, 0755))
	require.NoError(t, os.MkdirAll(dictionaryDir, 0755))

	// Create valid story notebook
	storyContent := []StoryNotebook{
		{
			Event: "Episode 1",
			Scenes: []StoryScene{
				{
					Title: "Scene 1",
					Conversations: []Conversation{
						{Speaker: "A", Quote: "Let's {{ break the ice }}"},
					},
					Definitions: []Note{
						{Expression: "break the ice", Meaning: "to initiate social interaction"},
					},
				},
			},
		},
	}
	require.NoError(t, WriteYamlFile(filepath.Join(storiesDir, "test.yml"), storyContent))

	// Create matching learning notes
	learningContent := []LearningHistory{
		{
			Metadata: LearningHistoryMetadata{Title: "Episode 1"},
			Scenes: []LearningScene{
				{
					Metadata: LearningSceneMetadata{Title: "Scene 1"},
					Expressions: []LearningHistoryExpression{
						{
							Expression: "break the ice",
							LearnedLogs: []LearningRecord{
								{Status: learnedStatusUnderstood, LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
							},
						},
					},
				},
			},
		},
	}
	require.NoError(t, WriteYamlFile(filepath.Join(learningNotesDir, "test.yml"), learningContent))

	// Create valid flashcard notebook
	flashcardContent := []FlashcardNotebook{
		{
			Title: "Common Idioms",
			Cards: []Note{
				{Expression: "lose one's temper", Meaning: "to become very angry"},
			},
		},
	}
	require.NoError(t, WriteYamlFile(filepath.Join(flashcardsDir, "idioms.yml"), flashcardContent))

	v := NewValidator(learningNotesDir, []string{storiesDir}, []string{flashcardsDir}, dictionaryDir)

	result, err := v.Validate()
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestValidator_DuplicateExpressionsAcrossScenes(t *testing.T) {
	t.Run("validates duplicate expressions across scenes in same episode", func(t *testing.T) {
		files := []learningHistoryFile{
			{
				path: "test.yml",
				contents: []LearningHistory{
					{
						Metadata: LearningHistoryMetadata{Title: "Episode 1"},
						Scenes: []LearningScene{
							{
								Metadata: LearningSceneMetadata{Title: "Scene A"},
								Expressions: []LearningHistoryExpression{
									{
										Expression: "ovulate",
										LearnedLogs: []LearningRecord{
											{Status: learnedStatusUnderstood, LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
										},
									},
								},
							},
							{
								Metadata: LearningSceneMetadata{Title: "Scene B"},
								Expressions: []LearningHistoryExpression{
									{
										Expression: "ovulate",
										LearnedLogs: []LearningRecord{
											{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC))},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		validator := &Validator{}
		result := &ValidationResult{}
		validator.validateLearningNotesStructure(files, result)

		// Should have an error about duplicate expressions
		require.True(t, result.HasErrors())
		assert.Contains(t, result.LearningNotesErrors[0].Message, `expression "ovulate" appears in multiple scenes`)
	})

	t.Run("fixes duplicate expressions across scenes in same episode", func(t *testing.T) {
		files := []learningHistoryFile{
			{
				path: "test.yml",
				contents: []LearningHistory{
					{
						Metadata: LearningHistoryMetadata{Title: "Episode 1"},
						Scenes: []LearningScene{
							{
								Metadata: LearningSceneMetadata{Title: "Scene A"},
								Expressions: []LearningHistoryExpression{
									{
										Expression: "ovulate",
										LearnedLogs: []LearningRecord{
											{Status: learnedStatusUnderstood, LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
										},
									},
								},
							},
							{
								Metadata: LearningSceneMetadata{Title: "Scene B"},
								Expressions: []LearningHistoryExpression{
									{
										Expression: "ovulate",
										LearnedLogs: []LearningRecord{
											{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC))},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		validator := &Validator{}
		result := &ValidationResult{}
		fixed := validator.fixLearningNotesStructure(files, result)

		// Should only have one "ovulate" expression in the first scene
		assert.Len(t, fixed[0].contents[0].Scenes[0].Expressions, 1)
		assert.Equal(t, "ovulate", fixed[0].contents[0].Scenes[0].Expressions[0].Expression)

		// Should have 2 learning logs (merged from both scenes)
		assert.Len(t, fixed[0].contents[0].Scenes[0].Expressions[0].LearnedLogs, 2)

		// Second scene should have no expressions
		assert.Len(t, fixed[0].contents[0].Scenes[1].Expressions, 0)

		// Learning logs should be sorted newest first
		logs := fixed[0].contents[0].Scenes[0].Expressions[0].LearnedLogs
		assert.Equal(t, "2025-01-02", logs[0].LearnedAt.Format("2006-01-02"))
		assert.Equal(t, "2025-01-01", logs[1].LearnedAt.Format("2006-01-02"))

		// Should have a warning about merging
		assert.True(t, len(result.Warnings) > 0)
		assert.Contains(t, result.Warnings[0].Message, "Merged duplicate expression")
	})
}

func TestValidator_FixDictionaryReferences(t *testing.T) {
	tmpDir := t.TempDir()
	dictionaryDir := filepath.Join(tmpDir, "dict")
	require.NoError(t, os.MkdirAll(dictionaryDir, 0755))

	// Create a dictionary file for "hello"
	require.NoError(t, os.WriteFile(filepath.Join(dictionaryDir, "hello.json"), []byte(`{}`), 0644))

	v := &Validator{dictionaryDir: dictionaryDir}

	t.Run("removes dictionary_number when file missing", func(t *testing.T) {
		files := []storyNotebookFile{
			{
				path: "test.yml",
				contents: []StoryNotebook{
					{
						Event: "Episode 1",
						Scenes: []StoryScene{
							{
								Title: "Scene 1",
								Definitions: []Note{
									{Expression: "nonexistent", DictionaryNumber: 1},
								},
							},
						},
					},
				},
			},
		}
		result := &ValidationResult{}
		fixed := v.fixDictionaryReferences(files, result)
		assert.Equal(t, 0, fixed[0].contents[0].Scenes[0].Definitions[0].DictionaryNumber)
		assert.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, "Removed dictionary_number")
	})

	t.Run("keeps dictionary_number when file exists", func(t *testing.T) {
		files := []storyNotebookFile{
			{
				path: "test.yml",
				contents: []StoryNotebook{
					{
						Event: "Episode 1",
						Scenes: []StoryScene{
							{
								Title: "Scene 1",
								Definitions: []Note{
									{Expression: "hello", DictionaryNumber: 1},
								},
							},
						},
					},
				},
			},
		}
		result := &ValidationResult{}
		fixed := v.fixDictionaryReferences(files, result)
		assert.Equal(t, 1, fixed[0].contents[0].Scenes[0].Definitions[0].DictionaryNumber)
		assert.Len(t, result.Warnings, 0)
	})

	t.Run("skips definitions without dictionary_number", func(t *testing.T) {
		files := []storyNotebookFile{
			{
				path: "test.yml",
				contents: []StoryNotebook{
					{
						Event: "Episode 1",
						Scenes: []StoryScene{
							{
								Title: "Scene 1",
								Definitions: []Note{
									{Expression: "hello", DictionaryNumber: 0},
								},
							},
						},
					},
				},
			},
		}
		result := &ValidationResult{}
		fixed := v.fixDictionaryReferences(files, result)
		assert.Equal(t, 0, fixed[0].contents[0].Scenes[0].Definitions[0].DictionaryNumber)
		assert.Len(t, result.Warnings, 0)
	})

	t.Run("uses definition field for lookup when set", func(t *testing.T) {
		files := []storyNotebookFile{
			{
				path: "test.yml",
				contents: []StoryNotebook{
					{
						Event: "Episode 1",
						Scenes: []StoryScene{
							{
								Title: "Scene 1",
								Definitions: []Note{
									{Expression: "running", Definition: "hello", DictionaryNumber: 1},
								},
							},
						},
					},
				},
			},
		}
		result := &ValidationResult{}
		fixed := v.fixDictionaryReferences(files, result)
		// "hello" dict file exists, so it should keep the dictionary_number
		assert.Equal(t, 1, fixed[0].contents[0].Scenes[0].Definitions[0].DictionaryNumber)
	})
}

func TestValidator_FixMismatchedScenes(t *testing.T) {
	v := &Validator{}

	t.Run("moves expression to correct scene", func(t *testing.T) {
		storyFiles := []storyNotebookFile{
			{
				path: "story.yml",
				contents: []StoryNotebook{
					{
						Event: "Episode 1",
						Scenes: []StoryScene{
							{Title: "Scene A", Definitions: []Note{{Expression: "eager"}}},
							{Title: "Scene B", Definitions: []Note{{Expression: "brave"}}},
						},
					},
				},
			},
		}
		learningFiles := []learningHistoryFile{
			{
				path: "learning.yml",
				contents: []LearningHistory{
					{
						Metadata: LearningHistoryMetadata{Title: "Episode 1"},
						Scenes: []LearningScene{
							{
								Metadata: LearningSceneMetadata{Title: "Scene A"},
								Expressions: []LearningHistoryExpression{
									{Expression: "eager"},
									{Expression: "brave"}, // wrong scene
								},
							},
							{
								Metadata: LearningSceneMetadata{Title: "Scene B"},
								Expressions: []LearningHistoryExpression{},
							},
						},
					},
				},
			},
		}

		result := &ValidationResult{}
		fixed := v.fixMismatchedScenes(learningFiles, storyFiles, result)

		// Scene A should only have "eager"
		assert.Len(t, fixed[0].contents[0].Scenes[0].Expressions, 1)
		assert.Equal(t, "eager", fixed[0].contents[0].Scenes[0].Expressions[0].Expression)

		// Scene B should have "brave"
		assert.Len(t, fixed[0].contents[0].Scenes[1].Expressions, 1)
		assert.Equal(t, "brave", fixed[0].contents[0].Scenes[1].Expressions[0].Expression)

		assert.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, "Moved expression")
	})

	t.Run("creates new scene when target scene does not exist", func(t *testing.T) {
		storyFiles := []storyNotebookFile{
			{
				path: "story.yml",
				contents: []StoryNotebook{
					{
						Event: "Episode 1",
						Scenes: []StoryScene{
							{Title: "Scene A", Definitions: []Note{{Expression: "eager"}}},
							{Title: "Scene B", Definitions: []Note{{Expression: "brave"}}},
						},
					},
				},
			},
		}
		learningFiles := []learningHistoryFile{
			{
				path: "learning.yml",
				contents: []LearningHistory{
					{
						Metadata: LearningHistoryMetadata{Title: "Episode 1"},
						Scenes: []LearningScene{
							{
								Metadata: LearningSceneMetadata{Title: "Scene A"},
								Expressions: []LearningHistoryExpression{
									{Expression: "eager"},
									{Expression: "brave"}, // wrong scene, Scene B doesn't exist in learning notes
								},
							},
						},
					},
				},
			},
		}

		result := &ValidationResult{}
		fixed := v.fixMismatchedScenes(learningFiles, storyFiles, result)

		// Scene A should only have "eager"
		assert.Len(t, fixed[0].contents[0].Scenes[0].Expressions, 1)
		assert.Equal(t, "eager", fixed[0].contents[0].Scenes[0].Expressions[0].Expression)

		// A new Scene B should have been created with "brave"
		require.Len(t, fixed[0].contents[0].Scenes, 2)
		assert.Equal(t, "Scene B", fixed[0].contents[0].Scenes[1].Metadata.Title)
		assert.Len(t, fixed[0].contents[0].Scenes[1].Expressions, 1)
		assert.Equal(t, "brave", fixed[0].contents[0].Scenes[1].Expressions[0].Expression)
	})
}

func TestValidator_FixExpressionNames(t *testing.T) {
	v := &Validator{}

	t.Run("updates expression to use definition from story", func(t *testing.T) {
		storyFiles := []storyNotebookFile{
			{
				path: "story.yml",
				contents: []StoryNotebook{
					{
						Event: "Episode 1",
						Scenes: []StoryScene{
							{
								Title: "Scene A",
								Definitions: []Note{
									{Expression: "ran away", Definition: "run away"},
								},
							},
						},
					},
				},
			},
		}
		learningFiles := []learningHistoryFile{
			{
				path: "learning.yml",
				contents: []LearningHistory{
					{
						Metadata: LearningHistoryMetadata{Title: "Episode 1"},
						Scenes: []LearningScene{
							{
								Metadata: LearningSceneMetadata{Title: "Scene A"},
								Expressions: []LearningHistoryExpression{
									{Expression: "ran away"},
								},
							},
						},
					},
				},
			},
		}

		result := &ValidationResult{}
		fixed := v.fixExpressionNames(learningFiles, storyFiles, result)

		assert.Equal(t, "run away", fixed[0].contents[0].Scenes[0].Expressions[0].Expression)
		assert.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, "Updated expression")
	})

	t.Run("does not update when expression already matches definition", func(t *testing.T) {
		storyFiles := []storyNotebookFile{
			{
				path: "story.yml",
				contents: []StoryNotebook{
					{
						Event: "Episode 1",
						Scenes: []StoryScene{
							{
								Title: "Scene A",
								Definitions: []Note{
									{Expression: "eager", Definition: "eager"},
								},
							},
						},
					},
				},
			},
		}
		learningFiles := []learningHistoryFile{
			{
				path: "learning.yml",
				contents: []LearningHistory{
					{
						Metadata: LearningHistoryMetadata{Title: "Episode 1"},
						Scenes: []LearningScene{
							{
								Metadata: LearningSceneMetadata{Title: "Scene A"},
								Expressions: []LearningHistoryExpression{
									{Expression: "eager"},
								},
							},
						},
					},
				},
			},
		}

		result := &ValidationResult{}
		fixed := v.fixExpressionNames(learningFiles, storyFiles, result)

		assert.Equal(t, "eager", fixed[0].contents[0].Scenes[0].Expressions[0].Expression)
		assert.Len(t, result.Warnings, 0)
	})
}

func TestValidator_FixConsistency(t *testing.T) {
	v := &Validator{}

	t.Run("removes orphaned expressions without learned_logs", func(t *testing.T) {
		storyFiles := []storyNotebookFile{
			{
				path: "story.yml",
				contents: []StoryNotebook{
					{
						Event: "Episode 1",
						Scenes: []StoryScene{
							{
								Title: "Scene A",
								Definitions: []Note{
									{Expression: "eager"},
								},
							},
						},
					},
				},
			},
		}
		learningFiles := []learningHistoryFile{
			{
				path: "learning.yml",
				contents: []LearningHistory{
					{
						Metadata: LearningHistoryMetadata{Title: "Episode 1"},
						Scenes: []LearningScene{
							{
								Metadata: LearningSceneMetadata{Title: "Scene A"},
								Expressions: []LearningHistoryExpression{
									{Expression: "eager", LearnedLogs: []LearningRecord{}},
									{Expression: "orphaned_word", LearnedLogs: []LearningRecord{}}, // not in story, no logs
								},
							},
						},
					},
				},
			},
		}

		result := &ValidationResult{}
		fixed := v.fixConsistency(learningFiles, storyFiles, result)

		// Should keep "eager" (exists in story) and remove "orphaned_word" (not in story, no logs)
		assert.Len(t, fixed[0].contents[0].Scenes[0].Expressions, 1)
		assert.Equal(t, "eager", fixed[0].contents[0].Scenes[0].Expressions[0].Expression)
		assert.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, "Removed orphaned expression")
	})

	t.Run("keeps orphaned expressions with learned_logs", func(t *testing.T) {
		storyFiles := []storyNotebookFile{
			{
				path: "story.yml",
				contents: []StoryNotebook{
					{
						Event: "Episode 1",
						Scenes: []StoryScene{
							{Title: "Scene A", Definitions: []Note{{Expression: "eager"}}},
						},
					},
				},
			},
		}
		learningFiles := []learningHistoryFile{
			{
				path: "learning.yml",
				contents: []LearningHistory{
					{
						Metadata: LearningHistoryMetadata{Title: "Episode 1"},
						Scenes: []LearningScene{
							{
								Metadata: LearningSceneMetadata{Title: "Scene A"},
								Expressions: []LearningHistoryExpression{
									{Expression: "eager", LearnedLogs: []LearningRecord{}},
									{Expression: "orphaned_word", LearnedLogs: []LearningRecord{{Status: "usable"}}}, // not in story but has logs
								},
							},
						},
					},
				},
			},
		}

		result := &ValidationResult{}
		fixed := v.fixConsistency(learningFiles, storyFiles, result)

		assert.Len(t, fixed[0].contents[0].Scenes[0].Expressions, 2)
		assert.Len(t, result.Warnings, 0)
	})
}

func TestValidator_Fix_WithDictionaryReferences(t *testing.T) {
	tmpDir := t.TempDir()
	learningNotesDir := filepath.Join(tmpDir, "learning_notes")
	storiesDir := filepath.Join(tmpDir, "stories")
	dictionaryDir := filepath.Join(tmpDir, "dictionaries")

	require.NoError(t, os.MkdirAll(learningNotesDir, 0755))
	require.NoError(t, os.MkdirAll(storiesDir, 0755))
	require.NoError(t, os.MkdirAll(dictionaryDir, 0755))

	// Create story with dictionary references
	storyPath := filepath.Join(storiesDir, "test.yml")
	require.NoError(t, WriteYamlFile(storyPath, []StoryNotebook{
		{
			Event: "Episode 1",
			Metadata: Metadata{Series: "Test Show", Season: 1, Episode: 1},
			Scenes: []StoryScene{
				{
					Title: "Scene 1",
					Definitions: []Note{
						{Expression: "eager", DictionaryNumber: 1, Meaning: "wanting to do something"},
						{Expression: "brave", DictionaryNumber: 2, Meaning: "courageous"},
					},
				},
			},
		},
	}))

	// Create dictionary file for "eager" only
	require.NoError(t, os.WriteFile(filepath.Join(dictionaryDir, "eager.json"), []byte(`{}`), 0644))

	v := NewValidator(learningNotesDir, []string{storiesDir}, []string{}, dictionaryDir)
	result, err := v.Fix()
	require.NoError(t, err)

	// Should have warnings about creating learning notes and removing dictionary reference
	hasRemovedDictWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w.Message, "Removed dictionary_number") && strings.Contains(w.Message, "brave") {
			hasRemovedDictWarning = true
		}
	}
	assert.True(t, hasRemovedDictWarning, "should have warning about removing dictionary_number for brave")

	// Verify the story file was updated
	var stories []StoryNotebook
	stories, err = readYamlFile[[]StoryNotebook](storyPath)
	require.NoError(t, err)
	// "eager" should keep dictionary_number, "brave" should have it removed
	assert.Equal(t, 1, stories[0].Scenes[0].Definitions[0].DictionaryNumber)
	assert.Equal(t, 0, stories[0].Scenes[0].Definitions[1].DictionaryNumber)
}

func TestValidator_Fix_WithMismatchedScenes(t *testing.T) {
	tmpDir := t.TempDir()
	learningNotesDir := filepath.Join(tmpDir, "learning_notes")
	storiesDir := filepath.Join(tmpDir, "stories")
	dictionaryDir := filepath.Join(tmpDir, "dictionaries")

	require.NoError(t, os.MkdirAll(learningNotesDir, 0755))
	require.NoError(t, os.MkdirAll(storiesDir, 0755))
	require.NoError(t, os.MkdirAll(dictionaryDir, 0755))

	// Create story
	require.NoError(t, WriteYamlFile(filepath.Join(storiesDir, "test.yml"), []StoryNotebook{
		{
			Event: "Episode 1",
			Metadata: Metadata{Series: "Test Show", Season: 1, Episode: 1},
			Scenes: []StoryScene{
				{Title: "Scene A", Definitions: []Note{{Expression: "eager", Meaning: "wanting to do something"}}},
				{Title: "Scene B", Definitions: []Note{{Expression: "brave", Meaning: "courageous"}}},
			},
		},
	}))

	// Create learning notes with mismatched scene (brave in Scene A instead of Scene B)
	require.NoError(t, WriteYamlFile(filepath.Join(learningNotesDir, "test-show.yml"), []LearningHistory{
		{
			Metadata: LearningHistoryMetadata{NotebookID: "test-show", Title: "Episode 1"},
			Scenes: []LearningScene{
				{
					Metadata: LearningSceneMetadata{Title: "Scene A"},
					Expressions: []LearningHistoryExpression{
						{Expression: "eager", LearnedLogs: []LearningRecord{}},
						{Expression: "brave", LearnedLogs: []LearningRecord{{Status: "usable"}}},
					},
				},
			},
		},
	}))

	v := NewValidator(learningNotesDir, []string{storiesDir}, []string{}, dictionaryDir)
	result, err := v.Fix()
	require.NoError(t, err)

	// Should have a warning about moving the expression
	hasMovedWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w.Message, "Moved expression") && strings.Contains(w.Message, "brave") {
			hasMovedWarning = true
		}
	}
	assert.True(t, hasMovedWarning, "should have warning about moving brave to correct scene")
}
