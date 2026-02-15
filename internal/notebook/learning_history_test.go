package notebook

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLearningHistory_GetLogs(t *testing.T) {
	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		history       LearningHistory
		notebookTitle string
		sceneTitle    string
		definition    Note
		expected      []LearningRecord
	}{
		{
			name: "matching notebook, scene, and expression",
			history: LearningHistory{
				Metadata: LearningHistoryMetadata{
					NotebookID: "test-id",
					Title:      "Test Notebook",
				},
				Scenes: []LearningScene{
					{
						Metadata: LearningSceneMetadata{Title: "Scene 1"},
						Expressions: []LearningHistoryExpression{
							{
								Expression: "hello",
								LearnedLogs: []LearningRecord{
									{Status: "understood", LearnedAt: NewDate(fixedTime)},
								},
							},
						},
					},
				},
			},
			notebookTitle: "Test Notebook",
			sceneTitle:    "Scene 1",
			definition:    Note{Expression: "hello", Definition: "greeting"},
			expected: []LearningRecord{
				{Status: "understood", LearnedAt: NewDate(fixedTime)},
			},
		},
		{
			name: "matching notebook, scene, and definition",
			history: LearningHistory{
				Metadata: LearningHistoryMetadata{
					NotebookID: "test-id",
					Title:      "Test Notebook",
				},
				Scenes: []LearningScene{
					{
						Metadata: LearningSceneMetadata{Title: "Scene 1"},
						Expressions: []LearningHistoryExpression{
							{
								Expression: "greeting",
								LearnedLogs: []LearningRecord{
									{Status: "usable", LearnedAt: NewDate(fixedTime)},
								},
							},
						},
					},
				},
			},
			notebookTitle: "Test Notebook",
			sceneTitle:    "Scene 1",
			definition:    Note{Expression: "hello", Definition: "greeting"},
			expected: []LearningRecord{
				{Status: "usable", LearnedAt: NewDate(fixedTime)},
			},
		},
		{
			name: "wrong notebook title",
			history: LearningHistory{
				Metadata: LearningHistoryMetadata{
					NotebookID: "test-id",
					Title:      "Different Notebook",
				},
				Scenes: []LearningScene{
					{
						Metadata: LearningSceneMetadata{Title: "Scene 1"},
						Expressions: []LearningHistoryExpression{
							{
								Expression: "hello",
								LearnedLogs: []LearningRecord{
									{Status: "understood", LearnedAt: NewDate(fixedTime)},
								},
							},
						},
					},
				},
			},
			notebookTitle: "Test Notebook",
			sceneTitle:    "Scene 1",
			definition:    Note{Expression: "hello", Definition: "greeting"},
			expected:      nil,
		},
		{
			name: "wrong scene title",
			history: LearningHistory{
				Metadata: LearningHistoryMetadata{
					NotebookID: "test-id",
					Title:      "Test Notebook",
				},
				Scenes: []LearningScene{
					{
						Metadata: LearningSceneMetadata{Title: "Different Scene"},
						Expressions: []LearningHistoryExpression{
							{
								Expression: "hello",
								LearnedLogs: []LearningRecord{
									{Status: "understood", LearnedAt: NewDate(fixedTime)},
								},
							},
						},
					},
				},
			},
			notebookTitle: "Test Notebook",
			sceneTitle:    "Scene 1",
			definition:    Note{Expression: "hello", Definition: "greeting"},
			expected:      nil,
		},
		{
			name: "expression not found",
			history: LearningHistory{
				Metadata: LearningHistoryMetadata{
					NotebookID: "test-id",
					Title:      "Test Notebook",
				},
				Scenes: []LearningScene{
					{
						Metadata: LearningSceneMetadata{Title: "Scene 1"},
						Expressions: []LearningHistoryExpression{
							{
								Expression: "different",
								LearnedLogs: []LearningRecord{
									{Status: "understood", LearnedAt: NewDate(fixedTime)},
								},
							},
						},
					},
				},
			},
			notebookTitle: "Test Notebook",
			sceneTitle:    "Scene 1",
			definition:    Note{Expression: "hello", Definition: "greeting"},
			expected:      nil,
		},
		{
			name: "multiple scenes, find in second",
			history: LearningHistory{
				Metadata: LearningHistoryMetadata{
					NotebookID: "test-id",
					Title:      "Test Notebook",
				},
				Scenes: []LearningScene{
					{
						Metadata: LearningSceneMetadata{Title: "Scene 1"},
						Expressions: []LearningHistoryExpression{
							{Expression: "other", LearnedLogs: []LearningRecord{}},
						},
					},
					{
						Metadata: LearningSceneMetadata{Title: "Scene 2"},
						Expressions: []LearningHistoryExpression{
							{
								Expression: "hello",
								LearnedLogs: []LearningRecord{
									{Status: "intuitive", LearnedAt: NewDate(fixedTime)},
								},
							},
						},
					},
				},
			},
			notebookTitle: "Test Notebook",
			sceneTitle:    "Scene 2",
			definition:    Note{Expression: "hello", Definition: "greeting"},
			expected: []LearningRecord{
				{Status: "intuitive", LearnedAt: NewDate(fixedTime)},
			},
		},
		{
			name: "flashcard type - find expression",
			history: LearningHistory{
				Metadata: LearningHistoryMetadata{
					NotebookID: "test-id",
					Title:      "flashcards",
					Type:       "flashcard",
				},
				Expressions: []LearningHistoryExpression{
					{
						Expression: "hello",
						LearnedLogs: []LearningRecord{
							{Status: "understood", LearnedAt: NewDate(fixedTime)},
						},
					},
				},
			},
			notebookTitle: "flashcards",
			sceneTitle:    "",
			definition:    Note{Expression: "hello", Definition: "greeting"},
			expected: []LearningRecord{
				{Status: "understood", LearnedAt: NewDate(fixedTime)},
			},
		},
		{
			name: "flashcard type - expression not found",
			history: LearningHistory{
				Metadata: LearningHistoryMetadata{
					NotebookID: "test-id",
					Title:      "flashcards",
					Type:       "flashcard",
				},
				Expressions: []LearningHistoryExpression{
					{
						Expression: "different",
						LearnedLogs: []LearningRecord{
							{Status: "understood", LearnedAt: NewDate(fixedTime)},
						},
					},
				},
			},
			notebookTitle: "flashcards",
			sceneTitle:    "",
			definition:    Note{Expression: "hello", Definition: "greeting"},
			expected:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.history.GetLogs(tt.notebookTitle, tt.sceneTitle, tt.definition)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewLearningHistories(t *testing.T) {
	tests := []struct {
		name          string
		setupFiles    map[string]string // filename -> content
		expectedKeys  []string
		expectedError bool
	}{
		{
			name: "single valid file",
			setupFiles: map[string]string{
				"test.yml": `- metadata:
    id: test-id
    title: Test Title
  scenes:
    - metadata:
        title: Scene 1
      expressions:
        - expression: hello
          learned_logs: []`,
			},
			expectedKeys:  []string{"test"},
			expectedError: false,
		},
		{
			name: "multiple valid files",
			setupFiles: map[string]string{
				"vocab1.yml": `- metadata:
    id: vocab1
    title: Unit 1
  scenes:
    - metadata:
        title: Lesson 1
      expressions: []`,
				"vocab2.yml": `- metadata:
    id: vocab2
    title: Unit 1
  scenes:
    - metadata:
        title: Lesson 1
      expressions: []`,
			},
			expectedKeys:  []string{"vocab1", "vocab2"},
			expectedError: false,
		},
		{
			name: "ignore non-yml files",
			setupFiles: map[string]string{
				"test.yml": `- metadata:
    id: test-id
    title: Test Title
  scenes: []`,
				"readme.txt":  "This is not a yaml file",
				"config.json": `{"key": "value"}`,
			},
			expectedKeys:  []string{"test"},
			expectedError: false,
		},
		{
			name: "invalid yaml",
			setupFiles: map[string]string{
				"invalid.yml": "invalid yaml content: [",
			},
			expectedKeys:  nil,
			expectedError: true,
		},
		{
			name:          "empty directory",
			setupFiles:    map[string]string{},
			expectedKeys:  []string{},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tempDir, err := os.MkdirTemp("", "learning_history_test")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			// Create test files
			for filename, content := range tt.setupFiles {
				filePath := filepath.Join(tempDir, filename)
				err := os.WriteFile(filePath, []byte(content), 0644)
				require.NoError(t, err)
			}

			// Test the function
			result, err := NewLearningHistories(tempDir)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Len(t, result, len(tt.expectedKeys))

			for _, expectedKey := range tt.expectedKeys {
				assert.Contains(t, result, expectedKey)
				assert.NotEmpty(t, result[expectedKey])
			}
		})
	}
}

func TestNewLearningHistories_NonexistentDirectory(t *testing.T) {
	result, err := NewLearningHistories("/nonexistent/directory")
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestLearningHistoryExpression_NeedsReverseReview(t *testing.T) {
	// Use actual timestamps for elapsed time calculation
	// The function calculates elapsed days from the stored timestamp
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)
	oneDayAgo := now.Add(-25 * time.Hour)   // 25 hours = 1 day elapsed
	twoDaysAgo := now.Add(-49 * time.Hour)  // 49 hours = 2 days elapsed

	tests := []struct {
		name       string
		expression LearningHistoryExpression
		want       bool
	}{
		{
			name: "No reverse logs - needs review",
			expression: LearningHistoryExpression{
				Expression:  "test",
				ReverseLogs: []LearningRecord{},
			},
			want: true,
		},
		{
			name: "Misunderstood status - needs review",
			expression: LearningHistoryExpression{
				Expression: "test",
				ReverseLogs: []LearningRecord{
					{
						Status:       LearnedStatusMisunderstood,
						LearnedAt:    NewDate(oneHourAgo),
						IntervalDays: 1,
					},
				},
			},
			want: true,
		},
		{
			name: "Answered recently with 1 day interval - should NOT need review",
			expression: LearningHistoryExpression{
				Expression: "test",
				ReverseLogs: []LearningRecord{
					{
						Status:       "usable",
						LearnedAt:    NewDate(oneHourAgo), // 0 days elapsed
						IntervalDays: 1,
					},
				},
			},
			want: false, // 0 < 1, no review needed
		},
		{
			name: "Answered 1 day ago with 1 day interval - should need review",
			expression: LearningHistoryExpression{
				Expression: "test",
				ReverseLogs: []LearningRecord{
					{
						Status:       "usable",
						LearnedAt:    NewDate(oneDayAgo), // 1 day elapsed
						IntervalDays: 1,
					},
				},
			},
			want: true, // 1 >= 1, needs review
		},
		{
			name: "Answered 2 days ago with 6 day interval - should NOT need review",
			expression: LearningHistoryExpression{
				Expression: "test",
				ReverseLogs: []LearningRecord{
					{
						Status:       "usable",
						LearnedAt:    NewDate(twoDaysAgo), // 2 days elapsed
						IntervalDays: 6,
					},
				},
			},
			want: false, // 2 < 6, no review needed
		},
		{
			name: "IntervalDays is 0 - fallback to count-based threshold",
			expression: LearningHistoryExpression{
				Expression: "test",
				ReverseLogs: []LearningRecord{
					{
						Status:       "usable",
						LearnedAt:    NewDate(oneHourAgo), // 0 days elapsed
						IntervalDays: 0,                   // No interval stored
					},
				},
			},
			// With 1 correct answer, threshold should be 1 day (from GetThresholdDaysFromCount)
			// 0 days elapsed < 1 day threshold, so should NOT need review
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.expression.NeedsReverseReview()
			assert.Equal(t, tt.want, got, "NeedsReverseReview() = %v, want %v", got, tt.want)
		})
	}
}

func TestLearningHistoryExpression_HasAnyCorrectAnswer(t *testing.T) {
	tests := []struct {
		name       string
		expression LearningHistoryExpression
		want       bool
	}{
		{
			name: "No logs - no correct answers",
			expression: LearningHistoryExpression{
				Expression:  "test",
				LearnedLogs: []LearningRecord{},
			},
			want: false,
		},
		{
			name: "Only misunderstood - no correct answers",
			expression: LearningHistoryExpression{
				Expression: "test",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood},
					{Status: LearnedStatusMisunderstood},
				},
			},
			want: false,
		},
		{
			name: "Has understood status - has correct answer",
			expression: LearningHistoryExpression{
				Expression: "test",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood},
					{Status: learnedStatusUnderstood},
				},
			},
			want: true,
		},
		{
			name: "Has usable status - has correct answer",
			expression: LearningHistoryExpression{
				Expression: "test",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusCanBeUsed},
				},
			},
			want: true,
		},
		{
			name: "Has intuitive status - has correct answer",
			expression: LearningHistoryExpression{
				Expression: "test",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusIntuitivelyUsed},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.expression.HasAnyCorrectAnswer()
			assert.Equal(t, tt.want, got, "HasAnyCorrectAnswer() = %v, want %v", got, tt.want)
		})
	}
}

