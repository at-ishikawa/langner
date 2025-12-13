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
									{Status: "understood", LearnedAt: NewDateFromTime(fixedTime)},
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
				{Status: "understood", LearnedAt: NewDateFromTime(fixedTime)},
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
									{Status: "usable", LearnedAt: NewDateFromTime(fixedTime)},
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
				{Status: "usable", LearnedAt: NewDateFromTime(fixedTime)},
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
									{Status: "understood", LearnedAt: NewDateFromTime(fixedTime)},
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
									{Status: "understood", LearnedAt: NewDateFromTime(fixedTime)},
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
									{Status: "understood", LearnedAt: NewDateFromTime(fixedTime)},
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
									{Status: "intuitive", LearnedAt: NewDateFromTime(fixedTime)},
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
				{Status: "intuitive", LearnedAt: NewDateFromTime(fixedTime)},
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
							{Status: "understood", LearnedAt: NewDateFromTime(fixedTime)},
						},
					},
				},
			},
			notebookTitle: "flashcards",
			sceneTitle:    "",
			definition:    Note{Expression: "hello", Definition: "greeting"},
			expected: []LearningRecord{
				{Status: "understood", LearnedAt: NewDateFromTime(fixedTime)},
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
							{Status: "understood", LearnedAt: NewDateFromTime(fixedTime)},
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

func TestLearningHistoryExpression_AddRecord(t *testing.T) {
	tests := []struct {
		name              string
		initialExpression LearningHistoryExpression
		isCorrect         bool
		isKnownWord       bool
		expectedStatus    LearnedStatus
		expectedCount     int
	}{
		{
			name: "add correct record to empty expression",
			initialExpression: LearningHistoryExpression{
				Expression:  "hello",
				LearnedLogs: []LearningRecord{},
			},
			isCorrect:      true,
			isKnownWord:    true,
			expectedStatus: learnedStatusUnderstood,
			expectedCount:  1,
		},
		{
			name: "add correct record to empty expression",
			initialExpression: LearningHistoryExpression{
				Expression:  "hello",
				LearnedLogs: []LearningRecord{},
			},
			isCorrect:      true,
			isKnownWord:    false,
			expectedStatus: learnedStatusCanBeUsed,
			expectedCount:  1,
		},
		{
			name: "add incorrect record to empty expression",
			initialExpression: LearningHistoryExpression{
				Expression:  "hello",
				LearnedLogs: []LearningRecord{},
			},
			isCorrect:      false,
			isKnownWord:    true,
			expectedStatus: LearnedStatusMisunderstood,
			expectedCount:  1,
		},
		{
			name: "add record to existing logs",
			initialExpression: LearningHistoryExpression{
				Expression: "hello",
				LearnedLogs: []LearningRecord{
					{Status: "usable", LearnedAt: NewDateFromTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
				},
			},
			isCorrect:      true,
			isKnownWord:    true,
			expectedStatus: learnedStatusUnderstood,
			expectedCount:  2,
		},
		{
			name: "should not add duplicate misunderstood status",
			initialExpression: LearningHistoryExpression{
				Expression: "hello",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDateFromTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
				},
			},
			isCorrect:      false,
			isKnownWord:    false,
			expectedStatus: LearnedStatusMisunderstood,
			expectedCount:  1, // Should remain 1, not add duplicate
		},
		{
			name: "should not add duplicate usable status",
			initialExpression: LearningHistoryExpression{
				Expression: "hello",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusCanBeUsed, LearnedAt: NewDateFromTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
				},
			},
			isCorrect:      true,
			isKnownWord:    false,
			expectedStatus: learnedStatusCanBeUsed,
			expectedCount:  1, // Should remain 1, not add duplicate
		},
		{
			name: "should not add duplicate understood status",
			initialExpression: LearningHistoryExpression{
				Expression: "hello",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
				},
			},
			isCorrect:      true,
			isKnownWord:    true,
			expectedStatus: learnedStatusUnderstood,
			expectedCount:  1, // Should remain 1, not add duplicate
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Record initial count
			initialCount := len(tt.initialExpression.LearnedLogs)
			
			// Record time before calling AddRecord to ensure new record is recent
			beforeTime := time.Now()

			tt.initialExpression.AddRecord(tt.isCorrect, tt.isKnownWord)

			afterTime := time.Now()

			assert.Len(t, tt.initialExpression.LearnedLogs, tt.expectedCount)

			// Check the latest status
			assert.Equal(t, tt.expectedStatus, tt.initialExpression.GetLatestStatus())
			
			// For new records (when count increased), check that the new record is at the beginning
			if len(tt.initialExpression.LearnedLogs) > initialCount {
				newRecord := tt.initialExpression.LearnedLogs[0]
				assert.True(t, newRecord.LearnedAt.After(beforeTime) || newRecord.LearnedAt.Equal(beforeTime))
				assert.True(t, newRecord.LearnedAt.Before(afterTime) || newRecord.LearnedAt.Equal(afterTime))
			}
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

// TestAddRecordAlways verifies QA command only records correct answers
func TestAddRecordAlways(t *testing.T) {
	tests := []struct {
		name              string
		initialExpression LearningHistoryExpression
		isCorrect         bool
		isKnownWord       bool
		expectedCount     int
		expectedStatus    LearnedStatus
	}{
		{
			name: "correct answer with duplicate understood status - should add",
			initialExpression: LearningHistoryExpression{
				Expression: "test",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusUnderstood, LearnedAt: NewDateFromTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
				},
			},
			isCorrect:      true,
			isKnownWord:    true,
			expectedCount:  2, // Should add duplicate
			expectedStatus: learnedStatusUnderstood,
		},
		{
			name: "incorrect answer - should NOT add any record",
			initialExpression: LearningHistoryExpression{
				Expression: "test",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDateFromTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
				},
			},
			isCorrect:      false,
			isKnownWord:    false,
			expectedCount:  1, // Should NOT add any record
			expectedStatus: LearnedStatusMisunderstood,
		},
		{
			name: "correct answer changing from misunderstood to understood - should add",
			initialExpression: LearningHistoryExpression{
				Expression: "test",
				LearnedLogs: []LearningRecord{
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDateFromTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
				},
			},
			isCorrect:      true,
			isKnownWord:    true,
			expectedCount:  2, // Should add new status
			expectedStatus: learnedStatusUnderstood,
		},
		{
			name: "incorrect answer with empty history - should add misunderstood",
			initialExpression: LearningHistoryExpression{
				Expression:  "test",
				LearnedLogs: []LearningRecord{},
			},
			isCorrect:      false,
			isKnownWord:    false,
			expectedCount:  1, // Should add misunderstood record
			expectedStatus: LearnedStatusMisunderstood,
		},
		{
			name: "correct answer with usable status - should add understood",
			initialExpression: LearningHistoryExpression{
				Expression: "test",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusCanBeUsed, LearnedAt: NewDateFromTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
				},
			},
			isCorrect:      true,
			isKnownWord:    true,
			expectedCount:  2, // Should add new record
			expectedStatus: learnedStatusUnderstood,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.initialExpression.AddRecordAlways(tt.isCorrect, tt.isKnownWord)
			
			assert.Len(t, tt.initialExpression.LearnedLogs, tt.expectedCount)
			if tt.expectedCount > 0 {
				assert.Equal(t, tt.expectedStatus, tt.initialExpression.GetLatestStatus())
			}
		})
	}
}
