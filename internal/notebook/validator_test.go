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
												{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC))},
												{Status: learnedStatusLearning, LearnedAt: NewDateFromTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
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
												{Status: LearnedStatus("invalid"), LearnedAt: NewDateFromTime(time.Now())},
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
			name: "duplicate dates",
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
												{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
												{Status: learnedStatusLearning, LearnedAt: NewDateFromTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
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
			errorMessageContains: []string{"duplicate learned_at date"},
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
												{Status: learnedStatusLearning, LearnedAt: NewDateFromTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
												{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC))},
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
											{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
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
											{Status: learnedStatusCanBeUsed, LearnedAt: NewDateFromTime(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC))},
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
											{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
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
											{Status: learnedStatusCanBeUsed, LearnedAt: NewDateFromTime(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC))},
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
