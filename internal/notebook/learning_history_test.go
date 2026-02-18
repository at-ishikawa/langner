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
		{
			name: "duplicate expressions - first empty, second has logs",
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
								Expression:  "break the ice",
								LearnedLogs: []LearningRecord{},
							},
							{
								Expression: "break someone's ice",
								LearnedLogs: []LearningRecord{
									{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(fixedTime), Quality: 4, IntervalDays: 1},
								},
							},
						},
					},
				},
			},
			notebookTitle: "Test Notebook",
			sceneTitle:    "Scene 1",
			definition:    Note{Expression: "break the ice", Definition: "break someone's ice"},
			expected: []LearningRecord{
				{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(fixedTime), Quality: 4, IntervalDays: 1},
			},
		},
		{
			name: "flashcard type - duplicate expressions - first empty, second has logs",
			history: LearningHistory{
				Metadata: LearningHistoryMetadata{
					NotebookID: "test-id",
					Title:      "flashcards",
					Type:       "flashcard",
				},
				Expressions: []LearningHistoryExpression{
					{
						Expression:  "lose one's temper",
						LearnedLogs: []LearningRecord{},
					},
					{
						Expression: "lose someone's temper",
						LearnedLogs: []LearningRecord{
							{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(fixedTime), Quality: 4, IntervalDays: 1},
						},
					},
				},
			},
			notebookTitle: "flashcards",
			sceneTitle:    "",
			definition:    Note{Expression: "lose one's temper", Definition: "lose someone's temper"},
			expected: []LearningRecord{
				{Status: learnedStatusCanBeUsed, LearnedAt: NewDate(fixedTime), Quality: 4, IntervalDays: 1},
			},
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

func TestLearningHistoryExpression_GetLatestStatus(t *testing.T) {
	tests := []struct {
		name       string
		expression LearningHistoryExpression
		want       LearnedStatus
	}{
		{
			name: "no logs returns learning status",
			expression: LearningHistoryExpression{
				LearnedLogs: []LearningRecord{},
			},
			want: learnedStatusLearning,
		},
		{
			name: "returns first log status",
			expression: LearningHistoryExpression{
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusUnderstood},
					{Status: LearnedStatusMisunderstood},
				},
			},
			want: learnedStatusUnderstood,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.expression.GetLatestStatus()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLearningHistoryExpression_GetLogsForQuizType(t *testing.T) {
	reverseLogs := []LearningRecord{{Status: LearnedStatusMisunderstood, Quality: 1}}
	learnedLogs := []LearningRecord{{Status: learnedStatusUnderstood, Quality: 4}}

	expr := LearningHistoryExpression{
		LearnedLogs: learnedLogs,
		ReverseLogs: reverseLogs,
	}

	t.Run("reverse quiz type returns reverse logs", func(t *testing.T) {
		got := expr.GetLogsForQuizType(QuizTypeReverse)
		assert.Equal(t, reverseLogs, got)
	})

	t.Run("freeform quiz type returns learned logs", func(t *testing.T) {
		got := expr.GetLogsForQuizType(QuizTypeFreeform)
		assert.Equal(t, learnedLogs, got)
	})

	t.Run("notebook quiz type returns learned logs", func(t *testing.T) {
		got := expr.GetLogsForQuizType(QuizTypeNotebook)
		assert.Equal(t, learnedLogs, got)
	})
}

func TestLearningHistoryExpression_GetEasinessFactorForQuizType(t *testing.T) {
	tests := []struct {
		name       string
		expression LearningHistoryExpression
		quizType   QuizType
		want       float64
	}{
		{
			name: "reverse with set factor",
			expression: LearningHistoryExpression{
				ReverseEasinessFactor: 2.1,
				EasinessFactor:        2.3,
			},
			quizType: QuizTypeReverse,
			want:     2.1,
		},
		{
			name: "reverse with zero factor returns default",
			expression: LearningHistoryExpression{
				ReverseEasinessFactor: 0,
				EasinessFactor:        2.3,
			},
			quizType: QuizTypeReverse,
			want:     DefaultEasinessFactor,
		},
		{
			name: "freeform with set factor",
			expression: LearningHistoryExpression{
				EasinessFactor:        2.3,
				ReverseEasinessFactor: 2.1,
			},
			quizType: QuizTypeFreeform,
			want:     2.3,
		},
		{
			name: "freeform with zero factor returns default",
			expression: LearningHistoryExpression{
				EasinessFactor: 0,
			},
			quizType: QuizTypeFreeform,
			want:     DefaultEasinessFactor,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.expression.GetEasinessFactorForQuizType(tt.quizType)
			assert.InDelta(t, tt.want, got, 0.001)
		})
	}
}

func TestLearningHistoryExpression_AddRecordWithQualityForReverse(t *testing.T) {
	tests := []struct {
		name           string
		expression     LearningHistoryExpression
		isCorrect      bool
		isKnownWord    bool
		quality        int
		responseTimeMs int64
		wantStatus     LearnedStatus
	}{
		{
			name: "correct known word",
			expression: LearningHistoryExpression{
				Expression:            "hello",
				ReverseEasinessFactor: DefaultEasinessFactor,
			},
			isCorrect:      true,
			isKnownWord:    true,
			quality:        int(QualityCorrect),
			responseTimeMs: 3000,
			wantStatus:     learnedStatusUnderstood,
		},
		{
			name: "correct unknown word",
			expression: LearningHistoryExpression{
				Expression:            "hello",
				ReverseEasinessFactor: DefaultEasinessFactor,
			},
			isCorrect:      true,
			isKnownWord:    false,
			quality:        int(QualityCorrect),
			responseTimeMs: 5000,
			wantStatus:     learnedStatusCanBeUsed,
		},
		{
			name: "incorrect",
			expression: LearningHistoryExpression{
				Expression:            "hello",
				ReverseEasinessFactor: DefaultEasinessFactor,
			},
			isCorrect:      false,
			isKnownWord:    false,
			quality:        int(QualityWrong),
			responseTimeMs: 10000,
			wantStatus:     LearnedStatusMisunderstood,
		},
		{
			name: "zero easiness factor gets set to default",
			expression: LearningHistoryExpression{
				Expression:            "hello",
				ReverseEasinessFactor: 0,
			},
			isCorrect:      true,
			isKnownWord:    true,
			quality:        int(QualityCorrect),
			responseTimeMs: 3000,
			wantStatus:     learnedStatusUnderstood,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := tt.expression
			exp.AddRecordWithQualityForReverse(tt.isCorrect, tt.isKnownWord, tt.quality, tt.responseTimeMs)

			assert.Len(t, exp.ReverseLogs, 1)
			assert.Equal(t, tt.wantStatus, exp.ReverseLogs[0].Status)
			assert.Equal(t, tt.quality, exp.ReverseLogs[0].Quality)
			assert.Equal(t, tt.responseTimeMs, exp.ReverseLogs[0].ResponseTimeMs)
			assert.Equal(t, string(QuizTypeReverse), exp.ReverseLogs[0].QuizType)
			assert.NotZero(t, exp.ReverseEasinessFactor)
		})
	}
}

func TestLearningHistoryExpression_AddRecordWithQuality(t *testing.T) {
	tests := []struct {
		name           string
		expression     LearningHistoryExpression
		isCorrect      bool
		isKnownWord    bool
		quality        int
		responseTimeMs int64
		quizType       QuizType
		wantStatus     LearnedStatus
	}{
		{
			name: "correct known word",
			expression: LearningHistoryExpression{
				Expression:     "hello",
				EasinessFactor: DefaultEasinessFactor,
			},
			isCorrect:      true,
			isKnownWord:    true,
			quality:        int(QualityCorrect),
			responseTimeMs: 3000,
			quizType:       QuizTypeFreeform,
			wantStatus:     learnedStatusUnderstood,
		},
		{
			name: "correct unknown word",
			expression: LearningHistoryExpression{
				Expression:     "hello",
				EasinessFactor: DefaultEasinessFactor,
			},
			isCorrect:      true,
			isKnownWord:    false,
			quality:        int(QualityCorrect),
			responseTimeMs: 5000,
			quizType:       QuizTypeNotebook,
			wantStatus:     learnedStatusCanBeUsed,
		},
		{
			name: "incorrect",
			expression: LearningHistoryExpression{
				Expression:     "hello",
				EasinessFactor: DefaultEasinessFactor,
			},
			isCorrect:      false,
			isKnownWord:    false,
			quality:        int(QualityWrong),
			responseTimeMs: 10000,
			quizType:       QuizTypeFreeform,
			wantStatus:     LearnedStatusMisunderstood,
		},
		{
			name: "zero easiness factor gets set to default",
			expression: LearningHistoryExpression{
				Expression:     "hello",
				EasinessFactor: 0,
			},
			isCorrect:      true,
			isKnownWord:    true,
			quality:        int(QualityCorrect),
			responseTimeMs: 3000,
			quizType:       QuizTypeFreeform,
			wantStatus:     learnedStatusUnderstood,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := tt.expression
			exp.AddRecordWithQuality(tt.isCorrect, tt.isKnownWord, tt.quality, tt.responseTimeMs, tt.quizType)

			assert.Len(t, exp.LearnedLogs, 1)
			assert.Equal(t, tt.wantStatus, exp.LearnedLogs[0].Status)
			assert.Equal(t, tt.quality, exp.LearnedLogs[0].Quality)
			assert.Equal(t, string(tt.quizType), exp.LearnedLogs[0].QuizType)
			assert.NotZero(t, exp.EasinessFactor)
		})
	}
}

func TestLearningHistoryExpression_Validate(t *testing.T) {
	fixedTime := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	olderTime := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		expr       LearningHistoryExpression
		wantErrors int
		wantMsg    string
	}{
		{
			name: "valid expression with logs",
			expr: LearningHistoryExpression{
				Expression: "hello",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(fixedTime)},
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDate(olderTime)},
				},
			},
			wantErrors: 0,
		},
		{
			name: "empty expression",
			expr: LearningHistoryExpression{
				Expression: "",
			},
			wantErrors: 1,
			wantMsg:    "expression field is empty",
		},
		{
			name: "no learned logs is valid",
			expr: LearningHistoryExpression{
				Expression:  "hello",
				LearnedLogs: []LearningRecord{},
			},
			wantErrors: 0,
		},
		{
			name: "invalid status",
			expr: LearningHistoryExpression{
				Expression: "hello",
				LearnedLogs: []LearningRecord{
					{Status: "invalid_status", LearnedAt: NewDate(fixedTime)},
				},
			},
			wantErrors: 1,
			wantMsg:    "invalid status",
		},
		{
			name: "missing learned_at",
			expr: LearningHistoryExpression{
				Expression: "hello",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusUnderstood},
				},
			},
			wantErrors: 1,
			wantMsg:    "learned_at is required",
		},
		{
			name: "logs not in chronological order",
			expr: LearningHistoryExpression{
				Expression: "hello",
				LearnedLogs: []LearningRecord{
					{Status: learnedStatusUnderstood, LearnedAt: NewDate(olderTime)},
					{Status: LearnedStatusMisunderstood, LearnedAt: NewDate(fixedTime)},
				},
			},
			wantErrors: 1,
			wantMsg:    "not in chronological order",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := tt.expr.Validate("test-location")
			assert.Len(t, errors, tt.wantErrors)
			if tt.wantMsg != "" && len(errors) > 0 {
				assert.Contains(t, errors[0].Message, tt.wantMsg)
			}
		})
	}
}

func TestLearningScene_Validate(t *testing.T) {
	fixedTime := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		scene      LearningScene
		wantErrors int
	}{
		{
			name: "valid scene",
			scene: LearningScene{
				Metadata: LearningSceneMetadata{Title: "Scene 1"},
				Expressions: []LearningHistoryExpression{
					{Expression: "hello", LearnedLogs: []LearningRecord{
						{Status: learnedStatusUnderstood, LearnedAt: NewDate(fixedTime)},
					}},
				},
			},
			wantErrors: 0,
		},
		{
			name: "scene with invalid expression",
			scene: LearningScene{
				Metadata: LearningSceneMetadata{Title: "Scene 1"},
				Expressions: []LearningHistoryExpression{
					{Expression: ""},
				},
			},
			wantErrors: 1,
		},
		{
			name: "empty scene is valid",
			scene: LearningScene{
				Metadata:    LearningSceneMetadata{Title: "Scene 1"},
				Expressions: []LearningHistoryExpression{},
			},
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := tt.scene.Validate("test-location")
			assert.Len(t, errors, tt.wantErrors)
		})
	}
}

func TestLearningHistory_Validate(t *testing.T) {
	fixedTime := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		history    LearningHistory
		wantErrors int
		wantMsg    string
	}{
		{
			name: "valid story type",
			history: LearningHistory{
				Metadata: LearningHistoryMetadata{Title: "Story 1"},
				Scenes: []LearningScene{
					{
						Metadata: LearningSceneMetadata{Title: "Scene 1"},
						Expressions: []LearningHistoryExpression{
							{Expression: "hello", LearnedLogs: []LearningRecord{
								{Status: learnedStatusUnderstood, LearnedAt: NewDate(fixedTime)},
							}},
						},
					},
				},
			},
			wantErrors: 0,
		},
		{
			name: "valid flashcard type",
			history: LearningHistory{
				Metadata: LearningHistoryMetadata{Title: "Flashcards", Type: "flashcard"},
				Expressions: []LearningHistoryExpression{
					{Expression: "hello", LearnedLogs: []LearningRecord{
						{Status: learnedStatusUnderstood, LearnedAt: NewDate(fixedTime)},
					}},
				},
			},
			wantErrors: 0,
		},
		{
			name: "flashcard with duplicate expressions",
			history: LearningHistory{
				Metadata: LearningHistoryMetadata{Title: "Flashcards", Type: "flashcard"},
				Expressions: []LearningHistoryExpression{
					{Expression: "hello"},
					{Expression: "hello"},
				},
			},
			wantErrors: 1,
			wantMsg:    "duplicate expression",
		},
		{
			name: "story with duplicate expressions across scenes",
			history: LearningHistory{
				Metadata: LearningHistoryMetadata{Title: "Story 1"},
				Scenes: []LearningScene{
					{
						Metadata:    LearningSceneMetadata{Title: "Scene 1"},
						Expressions: []LearningHistoryExpression{{Expression: "hello"}},
					},
					{
						Metadata:    LearningSceneMetadata{Title: "Scene 2"},
						Expressions: []LearningHistoryExpression{{Expression: "hello"}},
					},
				},
			},
			wantErrors: 1,
			wantMsg:    "appears in multiple scenes",
		},
		{
			name: "flashcard with invalid expression",
			history: LearningHistory{
				Metadata: LearningHistoryMetadata{Title: "Flashcards", Type: "flashcard"},
				Expressions: []LearningHistoryExpression{
					{Expression: ""},
				},
			},
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.history
			errors := h.Validate("test-location")
			assert.Len(t, errors, tt.wantErrors)
			if tt.wantMsg != "" && len(errors) > 0 {
				found := false
				for _, e := range errors {
					if assert.ObjectsAreEqual(true, true) {
						if len(e.Message) > 0 {
							if containsStr(e.Message, tt.wantMsg) {
								found = true
								break
							}
						}
					}
				}
				assert.True(t, found, "expected error containing %q", tt.wantMsg)
			}
		})
	}
}

// containsStr is a simple helper for string contains check
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && strings.Contains(s, substr))
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

