package notebook

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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
		name                   string
		files                  []learningHistoryFile
		expectedErrorCount     int
		expectedWarningCount   int
		errorMessageContains   []string
		warningMessageContains []string
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
												{Status: LearnedStatusUnderstood, LearnedAt: NewDate(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC))},
												{Status: LearnedStatusLearning, LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
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
												{Status: LearnedStatusUnderstood, LearnedAt: Date{}},
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
												{Status: LearnedStatusLearning, LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
												{Status: LearnedStatusUnderstood, LearnedAt: NewDate(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC))},
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

			validator.validateConsistency(tt.learningFiles, tt.storyFiles, nil, result)

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
			name: "expression found without markers - valid",
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
			expectedErrorCount: 0,
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
			validator := NewValidator(learningNotesDir, []string{storiesDir}, []string{}, []string{}, []string{}, dictionaryDir, nil)

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

// TestValidator_Fix_MergesHistoriesWithQuoteOnlyTitleDifference pins the
// expected behavior for a real bug: a learning_notes/<id>.yml file
// containing two top-level LearningHistory entries whose Metadata.Title
// values differ ONLY in apostrophe encoding (one uses U+2019 RIGHT SINGLE
// QUOTATION MARK, the other an ASCII apostrophe, which YAML serialises as
// 'It”s'). The reader treats them as two episodes, so a vocabulary word
// recorded in both keeps reappearing in daily quizzes — the SR algorithm
// finds the stale shorter-interval entry and marks the word due even
// after the user has answered it correctly weeks apart in the merged
// entry.
//
// Validator.Fix() previously dedup'd scenes WITHIN one history (via
// mergeDuplicateScenes) and expressions WITHIN one history (via
// fixLearningNotesStructure) but never merged two histories whose titles
// matched after normalizeQuotes. This test asserts the merged result has
// one history, one scene, one expression, and the union of all logs.
func TestValidator_Fix_MergesHistoriesWithQuoteOnlyTitleDifference(t *testing.T) {
	tmpDir := t.TempDir()
	learningNotesDir := filepath.Join(tmpDir, "learning_notes")
	storiesDir := filepath.Join(tmpDir, "stories")
	dictionaryDir := filepath.Join(tmpDir, "dictionaries")
	require.NoError(t, os.MkdirAll(learningNotesDir, 0o755))
	require.NoError(t, os.MkdirAll(storiesDir, 0o755))
	require.NoError(t, os.MkdirAll(dictionaryDir, 0o755))

	// learning_notes/test-show.yml has two histories whose titles compare
	// equal after normalizeQuotes — the first uses a smart apostrophe, the
	// second uses YAML's escaped ASCII apostrophe ('').
	require.NoError(t, os.WriteFile(filepath.Join(learningNotesDir, "test-show.yml"), []byte(
		"- metadata:\n"+
			"    id: test-show\n"+
			"    title: 'EPISODE ONE: HE’S BACK'\n"+
			"  scenes:\n"+
			"    - metadata:\n"+
			"        title: opening scene\n"+
			"      expressions:\n"+
			"        - expression: shared word\n"+
			"          learned_logs:\n"+
			"            - status: understood\n"+
			"              learned_at: \"2026-04-24T05:00:00-07:00\"\n"+
			"              quality: 4\n"+
			"              quiz_type: notebook\n"+
			"              interval_days: 30\n"+
			"- metadata:\n"+
			"    id: test-show\n"+
			"    title: 'EPISODE ONE: HE''S BACK'\n"+
			"  scenes:\n"+
			"    - metadata:\n"+
			"        title: opening scene\n"+
			"      expressions:\n"+
			"        - expression: shared word\n"+
			"          learned_logs:\n"+
			"            - status: understood\n"+
			"              learned_at: \"2026-04-04T05:00:00-07:00\"\n"+
			"              quality: 4\n"+
			"              quiz_type: notebook\n"+
			"              interval_days: 7\n",
	), 0o644))

	// Story notebook so the validator's "missing learning note" pass
	// doesn't add anything new — we only want to exercise the dedup path.
	require.NoError(t, os.WriteFile(filepath.Join(storiesDir, "test-show.yml"), []byte(
		"- event: 'EPISODE ONE: HE’S BACK'\n"+
			"  scenes:\n"+
			"    - scene: opening scene\n"+
			"      definitions:\n"+
			"        - expression: shared word\n",
	), 0o644))

	validator := NewValidator(learningNotesDir, []string{storiesDir}, []string{}, []string{}, []string{}, dictionaryDir, nil)
	_, err := validator.Fix()
	require.NoError(t, err)

	merged, err := readYamlFile[[]LearningHistory](filepath.Join(learningNotesDir, "test-show.yml"))
	require.NoError(t, err)

	require.Len(t, merged, 1, "two histories with quote-only title difference must collapse to one")
	require.Len(t, merged[0].Scenes, 1, "the single surviving history must have one scene")
	require.Len(t, merged[0].Scenes[0].Expressions, 1, "the shared expression must appear exactly once")

	logs := merged[0].Scenes[0].Expressions[0].LearnedLogs
	assert.Len(t, logs, 2, "logs from both histories must be combined onto the surviving expression")
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
								{Status: LearnedStatusUnderstood, LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
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

	v := NewValidator(learningNotesDir, []string{storiesDir}, []string{flashcardsDir}, []string{}, []string{}, dictionaryDir, nil)

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
											{Status: LearnedStatusUnderstood, LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
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
											{Status: LearnedStatusCanBeUsed, LearnedAt: NewDate(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC))},
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
											{Status: LearnedStatusUnderstood, LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
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
											{Status: LearnedStatusCanBeUsed, LearnedAt: NewDate(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC))},
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
								Metadata:    LearningSceneMetadata{Title: "Scene B"},
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

	t.Run("keeps expressions whose only data is skipped_at", func(t *testing.T) {
		// Reproduces a real bug: a word skipped from the notebook detail
		// page (no logs yet) was dropped by fixConsistency because the
		// keep-condition only checked logs and story membership. The
		// SkippedAt map was lost on the next `langner validate --fix`.
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
									// orphaned (not in story scene), no logs,
									// but skipped from a quiz mode — must
									// survive the fix pass.
									{
										Expression: "introvert",
										SkippedAt:  SkippedAtMap{"freeform": "2026-05-09T09:32:08-07:00"},
									},
								},
							},
						},
					},
				},
			},
		}

		result := &ValidationResult{}
		fixed := v.fixConsistency(learningFiles, storyFiles, result)

		require.Len(t, fixed[0].contents[0].Scenes[0].Expressions, 1, "skipped-only expression must not be dropped")
		assert.Equal(t, "introvert", fixed[0].contents[0].Scenes[0].Expressions[0].Expression)
		assert.True(t, fixed[0].contents[0].Scenes[0].Expressions[0].SkippedAt.IsSkippedAny())
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

// TestValidator_Fix_PreservesEveryPersistableField is a reflection-based
// regression guard against the class of bug where a new field is added to
// LearningHistoryExpression but the validator's fix pass forgets to honour
// it — silently dropping data on the next `langner validate --fix`. The
// concrete instance was SkippedAt: per-type skips were saved by SkipWord,
// then fixConsistency dropped the expression because it had no logs and
// wasn't in the story notebook.
//
// For every exported field on LearningHistoryExpression that participates
// in YAML round-tripping, this test:
//  1. builds a minimal expression with only Expression + that one field
//     populated to a non-zero value,
//  2. writes it to a learning_notes file with NO matching story file
//     (so the expression is "orphaned" — the case fixConsistency drops),
//  3. runs Validator.Fix(),
//  4. reloads the YAML from disk,
//  5. asserts the expression survived AND the field is still non-zero.
//
// Adding a new persisted field to LearningHistoryExpression will make this
// test fail until the fix pipeline is updated to recognize the field — or
// the field is explicitly added to the skip list below with a comment
// explaining why losing it during --fix is intentional.
func TestValidator_Fix_PreservesEveryPersistableField(t *testing.T) {
	// Fields the fix pass is allowed to mutate or drop. Each entry must
	// document why; an empty list keeps the guard tight.
	skip := map[string]string{
		"Expression":                       "the identifier itself, always set",
		"EasinessFactor":                   `yaml:"-" — derived on the fly`,
		"ReverseEasinessFactor":            `yaml:"-" — derived`,
		"EtymologyBreakdownEasinessFactor": `yaml:"-" — derived`,
		"EtymologyAssemblyEasinessFactor":  `yaml:"-" — derived`,
		// Type is a discriminator (vocabulary vs origin), not data. An
		// entry whose only data is Type is empty by intent and is
		// correctly dropped by fixConsistency. Round-tripping with
		// other data populated is exercised by the migration tests.
		"Type": "discriminator field, not 'data' that keeps an entry alive",
		// ID is the stable identity tag, like Type: an entry whose only
		// content is an ID (no logs, no skip) carries no learning data and
		// is correctly dropped by fixConsistency. When the entry has real
		// data the ID rides along on the whole struct and round-trips.
		"ID": "identity tag, not 'data' that keeps an entry alive",
	}

	rt := reflect.TypeOf(LearningHistoryExpression{})
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if reason, ok := skip[field.Name]; ok {
			t.Logf("skipping %s: %s", field.Name, reason)
			continue
		}
		// Skip yaml:"-" fields — they don't round-trip by design.
		if tag := field.Tag.Get("yaml"); tag == "-" {
			continue
		}

		t.Run(field.Name, func(t *testing.T) {
			value, ok := nonZeroValueFor(field.Type)
			if !ok {
				t.Fatalf("test cannot generate a non-zero value for field %s of type %s — extend nonZeroValueFor", field.Name, field.Type)
			}

			expr := LearningHistoryExpression{Expression: "needle"}
			reflect.ValueOf(&expr).Elem().FieldByName(field.Name).Set(value)

			learningDir := t.TempDir()
			storyDir := t.TempDir() // intentionally empty: forces the orphaned-expression code path

			// Write the history file with our single-field expression, nested
			// under a scene so it goes through fixConsistency (the path that
			// dropped SkippedAt-only entries).
			history := []LearningHistory{{
				Metadata: LearningHistoryMetadata{NotebookID: "fixture", Title: "Episode"},
				Scenes: []LearningScene{{
					Metadata:    LearningSceneMetadata{Title: "Scene"},
					Expressions: []LearningHistoryExpression{expr},
				}},
			}}
			path := filepath.Join(learningDir, "fixture.yml")
			require.NoError(t, WriteYamlFile(path, history))

			v := NewValidator(learningDir, []string{storyDir}, nil, nil, nil, "", nil)
			_, err := v.Fix()
			require.NoError(t, err)

			raw, err := os.ReadFile(path)
			require.NoError(t, err, "validate --fix must not delete the YAML file")

			var got []LearningHistory
			require.NoError(t, yaml.Unmarshal(raw, &got))
			require.NotEmpty(t, got, "validate --fix dropped the entire history")
			require.NotEmpty(t, got[0].Scenes, "validate --fix dropped the scene")
			require.NotEmpty(t, got[0].Scenes[0].Expressions,
				"validate --fix dropped the expression — field %s is not recognised as 'meaningful data' by fixConsistency",
				field.Name,
			)
			roundTripped := got[0].Scenes[0].Expressions[0]
			fieldValue := reflect.ValueOf(roundTripped).FieldByName(field.Name)
			assert.False(t, fieldValue.IsZero(),
				"field %s is zero after validate --fix; fix pipeline must preserve it",
				field.Name,
			)
		})
	}
}

// nonZeroValueFor returns a non-zero reflect.Value for a representative set
// of types LearningHistoryExpression uses. Returning ok=false makes the
// caller fail loudly when a new field type appears, so the test author has
// to extend this helper rather than silently skip the new field.
func nonZeroValueFor(t reflect.Type) (reflect.Value, bool) {
	switch t.Kind() {
	case reflect.String:
		return reflect.ValueOf("test").Convert(t), true
	case reflect.Int, reflect.Int32, reflect.Int64:
		return reflect.ValueOf(int64(1)).Convert(t), true
	case reflect.Float32, reflect.Float64:
		return reflect.ValueOf(1.0).Convert(t), true
	case reflect.Bool:
		return reflect.ValueOf(true), true
	case reflect.Slice:
		// LearningRecord is a complex struct; provide a fully-populated
		// element directly so a slice of one round-trips through YAML.
		if t.Elem() == reflect.TypeOf(LearningRecord{}) {
			elem := reflect.ValueOf(LearningRecord{
				Status:       LearnedStatusUnderstood,
				LearnedAt:    Date{Time: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)},
				Quality:      4,
				QuizType:     "freeform",
				IntervalDays: 7,
			})
			s := reflect.MakeSlice(t, 1, 1)
			s.Index(0).Set(elem)
			return s, true
		}
		elem, ok := nonZeroValueFor(t.Elem())
		if !ok {
			return reflect.Value{}, false
		}
		s := reflect.MakeSlice(t, 1, 1)
		s.Index(0).Set(elem)
		return s, true
	case reflect.Map:
		// SkippedAtMap is map[string]string under the hood.
		m := reflect.MakeMapWithSize(t, 1)
		key, ok1 := nonZeroValueFor(t.Key())
		val, ok2 := nonZeroValueFor(t.Elem())
		if !ok1 || !ok2 {
			return reflect.Value{}, false
		}
		// Use a real quiz-type string so legacy-format unmarshaling is
		// not surprised.
		if t.Key().Kind() == reflect.String && t.Elem().Kind() == reflect.String {
			key = reflect.ValueOf("freeform").Convert(t.Key())
			val = reflect.ValueOf("2026-05-09T09:32:08-07:00").Convert(t.Elem())
		}
		m.SetMapIndex(key, val)
		return m, true
	case reflect.Struct:
		// Date is a thin time wrapper; populate it with a non-zero time.
		if t == reflect.TypeOf(Date{}) {
			return reflect.ValueOf(Date{Time: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)}), true
		}
		return reflect.Value{}, false
	}
	return reflect.Value{}, false
}

// TestValidator_Fix_TypeFieldRoundTrips proves that the new Type
// discriminator on LearningHistoryExpression is preserved through a
// full Validator.Fix run when the entry has actual data — origin and
// vocabulary entries with the same name in the same scene must
// coexist as separate records and each must keep its Type.
func TestValidator_Fix_TypeFieldRoundTrips(t *testing.T) {
	learningDir := t.TempDir()
	storyDir := t.TempDir() // empty: orphan-detection isn't relevant here

	// Same-name collision: vocab "ego" + origin "ego" in the same scene.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "wpme.yml"), []byte(`- metadata:
    id: wpme
    title: "Session 1"
  scenes:
    - metadata:
        title: "ego (self)"
      expressions:
        - expression: ego
          type: vocabulary
          learned_logs:
            - status: understood
              learned_at: "2026-05-01"
              quality: 4
              quiz_type: notebook
        - expression: ego
          type: origin
          etymology_breakdown_logs:
            - status: understood
              learned_at: "2026-05-01"
              quality: 4
              quiz_type: etymology_freeform
`), 0644))

	v := NewValidator(learningDir, []string{storyDir}, nil, nil, nil, "", nil)
	_, err := v.Fix()
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(learningDir, "wpme.yml"))
	require.NoError(t, err)
	var got []LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))
	require.Len(t, got, 1)
	require.Len(t, got[0].Scenes, 1)
	exprs := got[0].Scenes[0].Expressions
	require.Lenf(t, exprs, 2, "vocab and origin entries with the same name must NOT merge — found: %+v", exprs)

	byType := make(map[string]LearningHistoryExpression, 2)
	for _, e := range exprs {
		byType[e.Type] = e
	}
	vocab, ok := byType[LearningExpressionTypeVocabulary]
	require.True(t, ok, "vocabulary entry missing")
	assert.NotEmpty(t, vocab.LearnedLogs, "vocab entry must keep its learned_logs")
	assert.Empty(t, vocab.EtymologyBreakdownLogs, "vocab entry must not absorb etymology logs")

	origin, ok := byType[LearningExpressionTypeOrigin]
	require.True(t, ok, "origin entry missing")
	assert.NotEmpty(t, origin.EtymologyBreakdownLogs, "origin entry must keep its etymology_breakdown_logs")
	assert.Empty(t, origin.LearnedLogs, "origin entry must not absorb vocab logs")
}

// TestValidator_Fix_MigratesEtymologyShape verifies that legacy etymology
// blocks (top-level title = notebook display name, type=etymology, with
// sessions stored as scenes) get rewritten into the canonical per-session
// shape with origins under a scene whose title comes from the matching
// definitions notebook.
func TestValidator_Fix_MigratesEtymologyShape(t *testing.T) {
	root := t.TempDir()
	learningDir := filepath.Join(root, "learning")
	storyDir := filepath.Join(root, "stories")
	defsDir := filepath.Join(root, "definitions")
	require.NoError(t, os.MkdirAll(learningDir, 0755))
	require.NoError(t, os.MkdirAll(storyDir, 0755))
	require.NoError(t, os.MkdirAll(defsDir, 0755))

	defsBookDir := filepath.Join(defsDir, "wpme")
	require.NoError(t, os.MkdirAll(defsBookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(defsBookDir, "index.yml"), []byte(`id: wpme
notebooks:
  - ./session2.yml
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(defsBookDir, "session2.yml"), []byte(`- metadata:
    title: "Session 2"
  scenes:
    - metadata:
        index: 0
        title: "ana (up, back)"
      expressions:
        - expression: anabolic
          meaning: "promoting cellular growth"
          origin_parts:
            - origin: ana
              language: Greek
`), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "wpme.yml"), []byte(`- metadata:
    id: wpme
    title: "Word Power Made Easy"
    type: etymology
  scenes:
    - metadata:
        title: "Session 2"
      expressions:
        - expression: ana
          etymology_breakdown_logs:
            - status: understood
              learned_at: "2026-05-01"
              quality: 4
              quiz_type: etymology_freeform
        - expression: untracked-origin
          etymology_breakdown_logs:
            - status: understood
              learned_at: "2026-05-01"
              quality: 4
              quiz_type: etymology_freeform
`), 0644))

	v := NewValidator(learningDir, []string{storyDir}, nil, []string{defsDir}, nil, "", nil)
	_, err := v.Fix()
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(learningDir, "wpme.yml"))
	require.NoError(t, err)
	var got []LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))

	for _, h := range got {
		assert.NotEqualf(t, "etymology", h.Metadata.Type,
			"legacy etymology block (title=%q) was not migrated", h.Metadata.Title)
		assert.NotEqualf(t, "Word Power Made Easy", h.Metadata.Title,
			"notebook-name top-level block must be replaced by per-session blocks")
	}
	require.Len(t, got, 1)
	require.Equal(t, "Session 2", got[0].Metadata.Title)

	scenesByTitle := make(map[string]LearningScene, len(got[0].Scenes))
	for _, scene := range got[0].Scenes {
		scenesByTitle[scene.Metadata.Title] = scene
	}
	matched, ok := scenesByTitle["ana (up, back)"]
	require.Truef(t, ok, "expected scene 'ana (up, back)' from definitions lookup; got: %v", keysOf(scenesByTitle))
	require.Len(t, matched.Expressions, 1)
	assert.Equal(t, "ana", matched.Expressions[0].Expression)

	fallback, ok := scenesByTitle["Session 2"]
	require.True(t, ok, "expected synthetic 'Session 2' scene for unmatched origins")
	require.Len(t, fallback.Expressions, 1)
	assert.Equal(t, "untracked-origin", fallback.Expressions[0].Expression)
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestValidator_Fix_PreservesSkippedOnlyOnDisk walks Validator.Fix end-to-end
// against a real YAML file containing an expression whose only data is
// SkippedAt. The previous fixConsistency keep-condition only checked logs
// and story membership and dropped this expression. After the fix, the
// expression and its SkippedAt map must survive a round trip through
// Validator.Fix.
func TestValidator_Fix_PreservesSkippedOnlyOnDisk(t *testing.T) {
	learningDir := t.TempDir()
	storyDir := t.TempDir() // intentionally empty so the expression is "orphaned"

	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "fixture.yml"), []byte(`- metadata:
    id: fixture
    title: "Episode 1"
  scenes:
    - metadata:
        title: "Scene A"
      expressions:
        - expression: "introvert"
          learned_logs: []
          skipped_at:
            freeform: "2026-05-09T09:32:08-07:00"
            reverse: "2026-05-09T09:32:08-07:00"
`), 0644))

	v := NewValidator(learningDir, []string{storyDir}, nil, nil, nil, "", nil)
	_, err := v.Fix()
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(learningDir, "fixture.yml"))
	require.NoError(t, err)
	var got []LearningHistory
	require.NoError(t, yaml.Unmarshal(raw, &got))
	require.Len(t, got, 1)
	require.Len(t, got[0].Scenes, 1)
	require.Len(t, got[0].Scenes[0].Expressions, 1,
		"validate --fix must NOT drop a skipped-only expression — the "+
			"previous keep-condition lost SkippedAt-only entries on every "+
			"--fix pass")
	expr := got[0].Scenes[0].Expressions[0]
	assert.Equal(t, "introvert", expr.Expression)
	assert.Contains(t, expr.SkippedAt, "freeform")
	assert.Contains(t, expr.SkippedAt, "reverse")
}

// Note: TestValidator_Fix_RecomputesClampedIntervals lived here. It
// pinned a "final pass" in fixLearningNotesStructure that recalculated
// every expression's interval_days at the end of every --fix run, on
// the theory it would repair logs the old calendar-day-truncation bug
// had clamped to 1.
//
// The pass was removed because it also rewrote historically-correct
// values: a hard-earned 30-day interval got flattened to 7 just because
// the current algorithm with the early-review guard would compute 7
// during a replay. Once the rewrite was a regular --fix outcome, every
// run silently degraded the user's spaced-repetition history. The
// invariant the user wants is "logs are immutable history; only the
// next interval is recomputed at quiz time," and the only way to honour
// that is to not recompute existing logs at all in --fix. If anyone
// later needs a one-time repair for the pre-calendar-day-guard
// clamping, it should ship as a dedicated migration CLI rather than
// hide in every --fix run.

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
			Event:    "Episode 1",
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

	v := NewValidator(learningNotesDir, []string{storiesDir}, []string{}, []string{}, []string{}, dictionaryDir, nil)
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
			Event:    "Episode 1",
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

	v := NewValidator(learningNotesDir, []string{storiesDir}, []string{}, []string{}, []string{}, dictionaryDir, nil)
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

func TestValidator_FixConsistency_KeepsEntriesWithOnlyReverseLogs(t *testing.T) {
	v := &Validator{}

	t.Run("keeps orphaned expressions with only reverse_logs", func(t *testing.T) {
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
									{
										Expression:  "orphaned_word",
										LearnedLogs: []LearningRecord{},
										ReverseLogs: []LearningRecord{{Status: "usable", LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))}},
									},
								},
							},
						},
					},
				},
			},
		}

		result := &ValidationResult{}
		fixed := v.fixConsistency(learningFiles, storyFiles, result)

		// Should keep both: "eager" (exists in story) and "orphaned_word" (has reverse_logs)
		assert.Len(t, fixed[0].contents[0].Scenes[0].Expressions, 2)
		assert.Equal(t, "eager", fixed[0].contents[0].Scenes[0].Expressions[0].Expression)
		assert.Equal(t, "orphaned_word", fixed[0].contents[0].Scenes[0].Expressions[1].Expression)
		assert.Len(t, result.Warnings, 0)
	})
}

func TestValidator_FixLearningNotesStructure_SameSceneMergeReverseLogs(t *testing.T) {
	validator := &Validator{}

	t.Run("preserves EasinessFactor when duplicate has only ReverseLogs", func(t *testing.T) {
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
										Expression: "break the ice",
										LearnedLogs: []LearningRecord{
											{Status: LearnedStatusUnderstood, LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)), Quality: 4},
										},
									},
									{
										Expression: "break the ice",
										ReverseLogs: []LearningRecord{
											{Status: LearnedStatusCanBeUsed, LearnedAt: NewDate(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)), Quality: 5},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		result := &ValidationResult{}
		fixed := validator.fixLearningNotesStructure(files, result)

		expr := fixed[0].contents[0].Scenes[0].Expressions[0]
		assert.Len(t, expr.LearnedLogs, 1)
		// ReverseLogs should be merged and ReverseEasinessFactor recalculated
		assert.Len(t, expr.ReverseLogs, 1)
	})

	t.Run("preserves ReverseEasinessFactor when duplicate has only LearnedLogs", func(t *testing.T) {
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
										Expression: "lose one's temper",
										ReverseLogs: []LearningRecord{
											{Status: LearnedStatusUnderstood, LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)), Quality: 4},
										},
									},
									{
										Expression: "lose one's temper",
										LearnedLogs: []LearningRecord{
											{Status: LearnedStatusCanBeUsed, LearnedAt: NewDate(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)), Quality: 5},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		result := &ValidationResult{}
		fixed := validator.fixLearningNotesStructure(files, result)

		expr := fixed[0].contents[0].Scenes[0].Expressions[0]
		assert.Len(t, expr.ReverseLogs, 1)
		// LearnedLogs should be merged and EasinessFactor recalculated
		assert.Len(t, expr.LearnedLogs, 1)
	})

	t.Run("merges both ReverseLogs and LearnedLogs", func(t *testing.T) {
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
										Expression: "hit the road",
										LearnedLogs: []LearningRecord{
											{Status: LearnedStatusUnderstood, LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)), Quality: 4},
										},
										ReverseLogs: []LearningRecord{
											{Status: LearnedStatusUnderstood, LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)), Quality: 4},
										},
									},
									{
										Expression: "hit the road",
										LearnedLogs: []LearningRecord{
											{Status: LearnedStatusCanBeUsed, LearnedAt: NewDate(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)), Quality: 5},
										},
										ReverseLogs: []LearningRecord{
											{Status: LearnedStatusCanBeUsed, LearnedAt: NewDate(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)), Quality: 5},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		result := &ValidationResult{}
		fixed := validator.fixLearningNotesStructure(files, result)

		require.Len(t, fixed[0].contents[0].Scenes[0].Expressions, 1)
		expr := fixed[0].contents[0].Scenes[0].Expressions[0]
		assert.Equal(t, "hit the road", expr.Expression)
		assert.Len(t, expr.LearnedLogs, 2)
		assert.Len(t, expr.ReverseLogs, 2)

		// Both logs should be sorted newest first
		assert.True(t, expr.LearnedLogs[0].LearnedAt.After(expr.LearnedLogs[1].LearnedAt.Time) || expr.LearnedLogs[0].LearnedAt.Equal(expr.LearnedLogs[1].LearnedAt.Time))
		assert.True(t, expr.ReverseLogs[0].LearnedAt.After(expr.ReverseLogs[1].LearnedAt.Time) || expr.ReverseLogs[0].LearnedAt.Equal(expr.ReverseLogs[1].LearnedAt.Time))
	})
}

func TestValidator_FixLearningNotesStructure_CrossSceneMergeReverseLogs(t *testing.T) {
	validator := &Validator{}

	t.Run("preserves EasinessFactor when duplicate has only ReverseLogs", func(t *testing.T) {
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
										Expression: "break the ice",
										LearnedLogs: []LearningRecord{
											{Status: LearnedStatusUnderstood, LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)), Quality: 4},
										},
									},
								},
							},
							{
								Metadata: LearningSceneMetadata{Title: "Scene B"},
								Expressions: []LearningHistoryExpression{
									{
										Expression: "break the ice",
										ReverseLogs: []LearningRecord{
											{Status: LearnedStatusCanBeUsed, LearnedAt: NewDate(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)), Quality: 5},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		result := &ValidationResult{}
		fixed := validator.fixLearningNotesStructure(files, result)

		// Expression should be merged into Scene A
		require.Len(t, fixed[0].contents[0].Scenes[0].Expressions, 1)
		expr := fixed[0].contents[0].Scenes[0].Expressions[0]
		assert.Len(t, expr.LearnedLogs, 1)
		// ReverseLogs should be merged
		assert.Len(t, expr.ReverseLogs, 1)

		// Scene B should be empty
		assert.Len(t, fixed[0].contents[0].Scenes[1].Expressions, 0)
	})

	t.Run("preserves ReverseEasinessFactor when duplicate has only LearnedLogs", func(t *testing.T) {
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
										Expression: "lose one's temper",
										ReverseLogs: []LearningRecord{
											{Status: LearnedStatusUnderstood, LearnedAt: NewDate(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)), Quality: 4},
										},
									},
								},
							},
							{
								Metadata: LearningSceneMetadata{Title: "Scene B"},
								Expressions: []LearningHistoryExpression{
									{
										Expression: "lose one's temper",
										LearnedLogs: []LearningRecord{
											{Status: LearnedStatusCanBeUsed, LearnedAt: NewDate(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)), Quality: 5},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		result := &ValidationResult{}
		fixed := validator.fixLearningNotesStructure(files, result)

		// Expression should be merged into Scene A
		require.Len(t, fixed[0].contents[0].Scenes[0].Expressions, 1)
		expr := fixed[0].contents[0].Scenes[0].Expressions[0]
		assert.Len(t, expr.ReverseLogs, 1)
		// LearnedLogs should be merged and EasinessFactor recalculated
		assert.Len(t, expr.LearnedLogs, 1)

		// Scene B should be empty
		assert.Len(t, fixed[0].contents[0].Scenes[1].Expressions, 0)
	})
}

func TestValidator_backfillQuizType(t *testing.T) {
	now := NewDate(time.Now())
	v := &Validator{}

	tests := []struct {
		name  string
		input []learningHistoryFile
		// want is a map from expression name to expected QuizType for the
		// first log of that expression, across all histories and scenes.
		want map[string]string
		// wantWarnings lists expected warning substrings.
		wantWarnings []string
	}{
		{
			name: "usable log without quiz_type gets freeform",
			input: []learningHistoryFile{{
				path: "test.yml",
				contents: []LearningHistory{{
					Metadata: LearningHistoryMetadata{Title: "Episode 1"},
					Scenes: []LearningScene{{
						Metadata: LearningSceneMetadata{Title: "Scene 1"},
						Expressions: []LearningHistoryExpression{{
							Expression:  "break the ice",
							LearnedLogs: []LearningRecord{{Status: LearnedStatusCanBeUsed, LearnedAt: now}},
						}},
					}},
				}},
			}},
			want:         map[string]string{"break the ice": string(QuizTypeFreeform)},
			wantWarnings: []string{"Backfilled quiz_type=freeform"},
		},
		{
			name: "usable log with existing quiz_type is unchanged",
			input: []learningHistoryFile{{
				path: "test.yml",
				contents: []LearningHistory{{
					Metadata: LearningHistoryMetadata{Title: "Episode 1"},
					Scenes: []LearningScene{{
						Metadata: LearningSceneMetadata{Title: "Scene 1"},
						Expressions: []LearningHistoryExpression{{
							Expression:  "break the ice",
							LearnedLogs: []LearningRecord{{Status: LearnedStatusCanBeUsed, LearnedAt: now, QuizType: "notebook"}},
						}},
					}},
				}},
			}},
			want: map[string]string{"break the ice": "notebook"},
		},
		{
			name: "non-usable log without quiz_type stays empty",
			input: []learningHistoryFile{{
				path: "test.yml",
				contents: []LearningHistory{{
					Metadata: LearningHistoryMetadata{Title: "Episode 1"},
					Scenes: []LearningScene{{
						Metadata: LearningSceneMetadata{Title: "Scene 1"},
						Expressions: []LearningHistoryExpression{{
							Expression:  "break the ice",
							LearnedLogs: []LearningRecord{{Status: LearnedStatusUnderstood, LearnedAt: now}},
						}},
					}},
				}},
			}},
			want: map[string]string{"break the ice": ""},
		},
		{
			name: "flashcard style (top-level expressions) is handled",
			input: []learningHistoryFile{{
				path: "test.yml",
				contents: []LearningHistory{{
					Metadata: LearningHistoryMetadata{Title: "Unit 1", Type: "flashcard"},
					Expressions: []LearningHistoryExpression{{
						Expression:  "lose one's temper",
						LearnedLogs: []LearningRecord{{Status: LearnedStatusCanBeUsed, LearnedAt: now}},
					}},
				}},
			}},
			want:         map[string]string{"lose one's temper": string(QuizTypeFreeform)},
			wantWarnings: []string{"Backfilled quiz_type=freeform"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ValidationResult{}
			fixed := v.backfillQuizType(tt.input, result)

			got := make(map[string]string)
			for _, f := range fixed {
				for _, h := range f.contents {
					for _, e := range h.Expressions {
						if len(e.LearnedLogs) > 0 {
							got[e.Expression] = e.LearnedLogs[0].QuizType
						}
					}
					for _, s := range h.Scenes {
						for _, e := range s.Expressions {
							if len(e.LearnedLogs) > 0 {
								got[e.Expression] = e.LearnedLogs[0].QuizType
							}
						}
					}
				}
			}
			assert.Equal(t, tt.want, got)

			for _, want := range tt.wantWarnings {
				found := false
				for _, w := range result.Warnings {
					if strings.Contains(w.Message, want) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected warning containing %q", want)
			}
			if len(tt.wantWarnings) == 0 {
				assert.Empty(t, result.Warnings)
			}
		})
	}
}

// TestValidator_Fix_RecalculatesIntervalsAcrossAllSlots pins the rule
// the user asked for: --fix replays each log series through the SR
// calculator (RecalculateAll) so interval_days reflects the prior-state
// chain. It touches all four slots (learned/reverse/etymology_breakdown/
// etymology_assembly), not just vocabulary; etymology had been writing
// intervals without consulting prior state under a since-fixed bug, and
// --fix is the path that corrects that stored data.
//
// The fixture seeds an interval value (999) that the fixed-interval
// algorithm would never produce. After --fix it should be replaced with
// a value the calculator actually computes for the chain.
func TestValidator_Fix_RecalculatesIntervalsAcrossAllSlots(t *testing.T) {
	dir := t.TempDir()
	learningNotesDir := filepath.Join(dir, "learning_notes")
	storiesDir := filepath.Join(dir, "stories")
	require.NoError(t, os.MkdirAll(learningNotesDir, 0o755))
	require.NoError(t, os.MkdirAll(storiesDir, 0o755))

	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.AddDate(0, 0, 3) // 3 days later — well under any seeded interval
	corruptInterval := 999    // not in DefaultFixedIntervals

	require.NoError(t, WriteYamlFile(filepath.Join(storiesDir, "demo.yml"), []StoryNotebook{
		{
			Event:    "Demo Episode",
			Metadata: Metadata{Series: "Demo", Season: 1, Episode: 1},
			Scenes: []StoryScene{
				{
					Title: "Scene 1",
					Definitions: []Note{
						{Expression: "demo-word"},
					},
				},
			},
		},
	}))

	makeLogs := func() []LearningRecord {
		return []LearningRecord{
			{
				Status:       LearnedStatusUnderstood,
				LearnedAt:    NewDate(t1),
				Quality:      4,
				IntervalDays: corruptInterval,
			},
			{
				Status:       LearnedStatusUnderstood,
				LearnedAt:    NewDate(t0),
				Quality:      4,
				IntervalDays: corruptInterval,
			},
		}
	}

	require.NoError(t, WriteYamlFile(filepath.Join(learningNotesDir, "demo.yml"), []LearningHistory{
		{
			Metadata: LearningHistoryMetadata{NotebookID: "demo", Title: "Demo Episode"},
			Scenes: []LearningScene{
				{
					Metadata: LearningSceneMetadata{Title: "Scene 1"},
					Expressions: []LearningHistoryExpression{
						{
							Expression:             "demo-word",
							LearnedLogs:            makeLogs(),
							ReverseLogs:            makeLogs(),
							EtymologyBreakdownLogs: makeLogs(),
							EtymologyAssemblyLogs:  makeLogs(),
						},
					},
				},
			},
		},
	}))

	calc := NewIntervalCalculator("fixed", nil) // DefaultFixedIntervals
	v := NewValidator(learningNotesDir, []string{storiesDir}, nil, nil, nil, "", calc)
	result, err := v.Fix()
	require.NoError(t, err)

	var histories []LearningHistory
	readYAMLForTest(t, filepath.Join(learningNotesDir, "demo.yml"), &histories)
	require.Len(t, histories, 1)
	require.Len(t, histories[0].Scenes, 1)
	require.Len(t, histories[0].Scenes[0].Expressions, 1)
	expr := histories[0].Scenes[0].Expressions[0]

	// All four slots must lose the corrupt 999 — the calculator output
	// is in DefaultFixedIntervals so 999 cannot survive.
	for _, slot := range []struct {
		name string
		logs []LearningRecord
	}{
		{"learned_logs", expr.LearnedLogs},
		{"reverse_logs", expr.ReverseLogs},
		{"etymology_breakdown_logs", expr.EtymologyBreakdownLogs},
		{"etymology_assembly_logs", expr.EtymologyAssemblyLogs},
	} {
		require.Len(t, slot.logs, 2, slot.name)
		for _, log := range slot.logs {
			assert.NotEqual(t, corruptInterval, log.IntervalDays,
				"%s still carries the bogus interval — recalc pass didn't run on this slot", slot.name)
		}
	}

	// The pass must emit one warning per recalculated log so audits show
	// what changed. 4 slots × 2 logs each = 8 expected warnings (at minimum).
	recalcCount := 0
	for _, w := range result.Warnings {
		if strings.Contains(w.Message, "Recalculated interval_days") {
			recalcCount++
		}
	}
	assert.GreaterOrEqual(t, recalcCount, 8,
		"expected at least one warning per recalculated log across all four slots")
}

// readYAMLForTest is a tiny helper used by the recalc test so it does
// not depend on the package-private readYAMLHelper in the learning
// package.
func readYAMLForTest(t *testing.T, path string, out interface{}) {
	t.Helper()
	histories, err := NewLearningHistories(filepath.Dir(path))
	require.NoError(t, err)
	notebookID := strings.TrimSuffix(filepath.Base(path), ".yml")
	got, ok := histories[notebookID]
	require.True(t, ok, "notebook %s not found in loaded histories", notebookID)
	dst, ok := out.(*[]LearningHistory)
	require.True(t, ok, "readYAMLForTest only handles *[]LearningHistory")
	*dst = got
}

// TestValidator_Fix_MergesDefinitionsIndexScene pins the migration that
// repairs definitions-book learning history split across two scene-key
// conventions. The quiz now reads definitions scenes by their human
// title (e.g. "verto (to turn)"), but older data also wrote the same
// scene under "__index_0". A word's skip could live under the human
// title while its quiz logs lived under "__index_0", so the quiz (now
// human-title-keyed) saw the skip but a sibling word's logs got
// orphaned. --fix renames "__index_N" to the human title and merges,
// so skip + logs reunite under one scene.
func TestValidator_Fix_MergesDefinitionsIndexScene(t *testing.T) {
	dir := t.TempDir()
	learningDir := filepath.Join(dir, "learning_notes")
	defsDir := filepath.Join(dir, "definitions")
	bookDir := filepath.Join(defsDir, "demo-book")
	require.NoError(t, os.MkdirAll(learningDir, 0o755))
	require.NoError(t, os.MkdirAll(bookDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "index.yml"), []byte(`id: demo-book
notebooks:
  - ./session1.yml
`), 0o644))
	// Scene index 0 has human title "verto (to turn)" with two words.
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "session1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
        title: "verto (to turn)"
      expressions:
        - expression: "extrovert"
          meaning: "outgoing person"
        - expression: "ambivert"
          meaning: "balanced person"
`), 0o644))

	// Learning history split across two scene keys for the SAME scene:
	//   - "verto (to turn)": extrovert SKIP (no logs)
	//   - "__index_0":       extrovert + ambivert quiz logs
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "demo-book.yml"), []byte(`- metadata:
    notebook_id: demo-book
    title: "Session 1"
  scenes:
    - metadata:
        title: "verto (to turn)"
      expressions:
        - expression: "extrovert"
          learned_logs: []
          skipped_at:
            notebook: "2026-05-24T20:28:42-07:00"
    - metadata:
        title: "__index_0"
      expressions:
        - expression: "extrovert"
          learned_logs:
            - status: "understood"
              learned_at: "2026-05-25T17:39:13-07:00"
              quality: 3
              quiz_type: "notebook"
              interval_days: 7
        - expression: "ambivert"
          learned_logs:
            - status: "misunderstood"
              learned_at: "2026-05-25T17:39:15-07:00"
              quality: 1
              quiz_type: "notebook"
              interval_days: 1
`), 0o644))

	v := NewValidator(learningDir, nil, nil, []string{defsDir}, nil, "", nil)
	_, err := v.Fix()
	require.NoError(t, err)

	var histories []LearningHistory
	readYAMLForTest(t, filepath.Join(learningDir, "demo-book.yml"), &histories)
	require.Len(t, histories, 1)

	// The two scenes must collapse into one titled "verto (to turn)";
	// the "__index_0" key must be gone.
	require.Len(t, histories[0].Scenes, 1, "the split scenes must merge into one")
	scene := histories[0].Scenes[0]
	assert.Equal(t, "verto (to turn)", scene.Metadata.Title)

	byExpr := map[string]LearningHistoryExpression{}
	for _, e := range scene.Expressions {
		byExpr[e.Expression] = e
	}

	// extrovert: skip preserved AND quiz log preserved (the reunion).
	extrovert, ok := byExpr["extrovert"]
	require.True(t, ok, "extrovert must survive the merge")
	assert.Contains(t, extrovert.SkippedAt, "notebook",
		"extrovert's skip (written under the human title) must survive")
	assert.NotEmpty(t, extrovert.LearnedLogs,
		"extrovert's quiz log (written under __index_0) must merge in, not be orphaned")

	// ambivert: its logs lived only under __index_0; they must not be lost.
	ambivert, ok := byExpr["ambivert"]
	require.True(t, ok, "ambivert must survive the merge")
	assert.NotEmpty(t, ambivert.LearnedLogs,
		"ambivert's __index_0 logs must be preserved under the human title")
}

// TestValidator_Fix_ConsolidatesSplitEtymologyOrigin reproduces the
// "multiple gamos records" bug: one etymology origin ends up with logs
// under two scene titles in the same session because the derived scene
// title drifted over time. --fix must merge them into one scene (the
// canonically-derived one when present) with logs + skip unioned.
//
// Generic Greek-root data (gamos/marriage, misein/hate) — the shape, not
// the user's exact notebook contents, is what's under test.
func TestValidator_Fix_ConsolidatesSplitEtymologyOrigin(t *testing.T) {
	dir := t.TempDir()
	learningDir := filepath.Join(dir, "learning_notes")
	etymDir := filepath.Join(dir, "etymology")
	defsDir := filepath.Join(dir, "definitions")
	etymBook := filepath.Join(etymDir, "demo-book")
	defsBook := filepath.Join(defsDir, "demo-book")
	require.NoError(t, os.MkdirAll(learningDir, 0o755))
	require.NoError(t, os.MkdirAll(etymBook, 0o755))
	require.NoError(t, os.MkdirAll(defsBook, 0o755))

	// Etymology session: gamos is a standalone origin (legacy flat shape).
	require.NoError(t, os.WriteFile(filepath.Join(etymBook, "index.yml"), []byte(`id: demo-book
kind: Etymology
name: Demo Book
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etymBook, "session1.yml"), []byte(`metadata:
  title: "Session 1"
origins:
  - origin: gamos
    language: Greek
    meaning: marriage
`), 0o644))

	// Definitions book: gamos is referenced ONLY from the "gamos (marriage)"
	// scene, so the canonical derived scene is "gamos (marriage)".
	require.NoError(t, os.WriteFile(filepath.Join(defsBook, "index.yml"), []byte(`id: demo-book
notebooks:
  - ./session1.yml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(defsBook, "session1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
        title: "gamos (marriage)"
      expressions:
        - expression: monogamy
          meaning: marriage to one
          origin_parts:
            - origin: gamos
              language: Greek
`), 0o644))

	// Learning history: gamos split across "misein (to hate)" (old logs)
	// and "gamos (marriage)" (newer log).
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "demo-book.yml"), []byte(`- metadata:
    notebook_id: demo-book
    title: "Session 1"
  scenes:
    - metadata:
        title: "misein (to hate)"
      expressions:
        - expression: gamos
          learned_logs: []
          etymology_breakdown_logs:
            - status: understood
              learned_at: "2026-04-25T14:21:42-07:00"
              quality: 4
              quiz_type: etymology_breakdown
              interval_days: 30
    - metadata:
        title: "gamos (marriage)"
      expressions:
        - expression: gamos
          type: origin
          learned_logs: []
          etymology_breakdown_logs:
            - status: understood
              learned_at: "2026-05-27T06:18:17-07:00"
              quality: 5
              quiz_type: etymology_breakdown
              interval_days: 30
`), 0o644))

	v := NewValidator(learningDir, nil, nil, []string{defsDir}, []string{etymDir}, "", nil)
	_, err := v.Fix()
	require.NoError(t, err)

	var histories []LearningHistory
	readYAMLForTest(t, filepath.Join(learningDir, "demo-book.yml"), &histories)
	require.Len(t, histories, 1)

	// gamos must appear under exactly ONE scene now — the canonical
	// "gamos (marriage)" — with both breakdown logs merged in.
	var gamosScenes []string
	var gamosLogCount int
	for _, scene := range histories[0].Scenes {
		for _, e := range scene.Expressions {
			if e.Expression == "gamos" {
				gamosScenes = append(gamosScenes, scene.Metadata.Title)
				gamosLogCount = len(e.EtymologyBreakdownLogs)
			}
		}
	}
	require.Len(t, gamosScenes, 1, "gamos must live under exactly one scene after consolidation")
	assert.Equal(t, "gamos (marriage)", gamosScenes[0],
		"consolidation target must be the canonically-derived scene")
	assert.Equal(t, 2, gamosLogCount,
		"both scenes' breakdown logs must be unioned onto the surviving entry")

	// The now-empty "misein (to hate)" scene must be dropped.
	for _, scene := range histories[0].Scenes {
		assert.NotEqual(t, "misein (to hate)", scene.Metadata.Title,
			"the emptied source scene must be removed")
	}
}

// TestValidator_Fix_ConsolidatesMisScenedVocabWord reproduces the
// "multiple dexterity records" bug: an old vocab path wrote a word's
// logs under a synthetic scene named after the SESSION instead of the
// word's real definitions scene. --fix must move each such word to its
// definitions scene, merging when it already has an entry there
// (duplicate) and relocating when it doesn't (mis-scened single).
//
// Generic idiom data, not the user's notebook.
func TestValidator_Fix_ConsolidatesMisScenedVocabWord(t *testing.T) {
	dir := t.TempDir()
	learningDir := filepath.Join(dir, "learning_notes")
	defsDir := filepath.Join(dir, "definitions")
	defsBook := filepath.Join(defsDir, "demo-book")
	require.NoError(t, os.MkdirAll(learningDir, 0o755))
	require.NoError(t, os.MkdirAll(defsBook, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(defsBook, "index.yml"), []byte(`id: demo-book
notebooks:
  - ./session1.yml
`), 0o644))
	// Two real scenes; "break the ice" lives in scene A, "lose your temper"
	// in scene B.
	require.NoError(t, os.WriteFile(filepath.Join(defsBook, "session1.yml"), []byte(`- metadata:
    title: "Session 1"
  scenes:
    - metadata:
        index: 0
        title: "social cues"
      expressions:
        - expression: break the ice
          meaning: to start a conversation
    - metadata:
        index: 1
        title: "emotions"
      expressions:
        - expression: lose your temper
          meaning: to get angry
`), 0o644))

	// Learning history: a synthetic "Session 1" scene holds both words'
	// old logs. "break the ice" is ALSO under its real scene "social cues"
	// (duplicate); "lose your temper" is ONLY under the synthetic scene
	// (mis-scened single).
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "demo-book.yml"), []byte(`- metadata:
    notebook_id: demo-book
    title: "Session 1"
  scenes:
    - metadata:
        title: "social cues"
      expressions:
        - expression: break the ice
          learned_logs:
            - status: understood
              learned_at: "2026-05-20T10:00:00Z"
              quality: 4
              quiz_type: notebook
              interval_days: 7
    - metadata:
        title: "Session 1"
      expressions:
        - expression: break the ice
          learned_logs:
            - status: understood
              learned_at: "2026-04-06T10:00:00Z"
              quality: 4
              quiz_type: freeform
              interval_days: 7
        - expression: lose your temper
          learned_logs:
            - status: misunderstood
              learned_at: "2026-04-06T10:05:00Z"
              quality: 1
              quiz_type: freeform
              interval_days: 1
`), 0o644))

	v := NewValidator(learningDir, nil, nil, []string{defsDir}, nil, "", nil)
	_, err := v.Fix()
	require.NoError(t, err)

	var histories []LearningHistory
	readYAMLForTest(t, filepath.Join(learningDir, "demo-book.yml"), &histories)
	require.Len(t, histories, 1)

	scenesByTitle := map[string][]LearningHistoryExpression{}
	for _, s := range histories[0].Scenes {
		scenesByTitle[s.Metadata.Title] = s.Expressions
	}

	// The synthetic "Session 1" scene must be gone.
	_, hasSynthetic := scenesByTitle["Session 1"]
	assert.False(t, hasSynthetic, "synthetic session-named scene must be removed after consolidation")

	// "break the ice": its two entries (social cues + synthetic) merge into
	// a single entry under "social cues" carrying both logs.
	social := scenesByTitle["social cues"]
	var bti *LearningHistoryExpression
	count := 0
	for i := range social {
		if social[i].Expression == "break the ice" {
			bti = &social[i]
			count++
		}
	}
	require.Equal(t, 1, count, "break the ice must be a single entry under its real scene")
	require.NotNil(t, bti)
	assert.Len(t, bti.LearnedLogs, 2, "both the duplicate's logs must be unioned")

	// "lose your temper": relocated from the synthetic scene to "emotions".
	emotions := scenesByTitle["emotions"]
	var found bool
	for _, e := range emotions {
		if e.Expression == "lose your temper" {
			found = true
			assert.NotEmpty(t, e.LearnedLogs, "relocated word keeps its logs")
		}
	}
	assert.True(t, found, "mis-scened single must be moved to its definitions scene")
}
