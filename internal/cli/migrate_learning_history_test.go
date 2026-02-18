package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateLearningHistory(t *testing.T) {
	tests := []struct {
		name       string
		setupFiles map[string]string
		wantErr    bool
	}{
		{
			name: "migrate expressions without easiness factor",
			setupFiles: map[string]string{
				"notebook1.yml": `- metadata:
    id: test-id
    title: Test Notebook
  scenes:
    - metadata:
        title: Scene 1
      expressions:
        - expression: hello
          learned_logs:
            - status: understood
              learned_at: "2025-06-01"`,
			},
			wantErr: false,
		},
		{
			name: "migrate flashcard expressions",
			setupFiles: map[string]string{
				"flashcard1.yml": `- metadata:
    id: flash-id
    title: Flashcards
    type: flashcard
  expressions:
    - expression: hello
      learned_logs:
        - status: understood
          learned_at: "2025-06-01"`,
			},
			wantErr: false,
		},
		{
			name: "already migrated - no changes",
			setupFiles: map[string]string{
				"notebook1.yml": `- metadata:
    id: test-id
    title: Test Notebook
  scenes:
    - metadata:
        title: Scene 1
      expressions:
        - expression: hello
          easiness_factor: 2.5
          learned_logs:
            - status: understood
              learned_at: "2025-06-01"
              quality: 4
              interval_days: 3`,
			},
			wantErr: false,
		},
		{
			name:       "empty directory",
			setupFiles: map[string]string{},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			for filename, content := range tt.setupFiles {
				err := os.WriteFile(filepath.Join(tempDir, filename), []byte(content), 0644)
				require.NoError(t, err)
			}

			err := MigrateLearningHistory(tempDir)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMigrateLearningHistory_NonexistentDirectory(t *testing.T) {
	err := MigrateLearningHistory("/nonexistent/directory")
	assert.Error(t, err)
}

func TestCalculateEasinessFactor(t *testing.T) {
	tests := []struct {
		name string
		logs []notebook.LearningRecord
		want float64
	}{
		{
			name: "empty logs returns default",
			logs: nil,
			want: notebook.DefaultEasinessFactor,
		},
		{
			name: "single correct log",
			logs: []notebook.LearningRecord{
				{Status: "understood", Quality: int(notebook.QualityCorrect)},
			},
			want: func() float64 {
				return notebook.UpdateEasinessFactor(notebook.DefaultEasinessFactor, int(notebook.QualityCorrect), 0)
			}(),
		},
		{
			name: "single misunderstood log",
			logs: []notebook.LearningRecord{
				{Status: notebook.LearnedStatusMisunderstood, Quality: int(notebook.QualityWrong)},
			},
			want: func() float64 {
				return notebook.UpdateEasinessFactor(notebook.DefaultEasinessFactor, int(notebook.QualityWrong), 0)
			}(),
		},
		{
			name: "logs without quality field infer from status - misunderstood",
			logs: []notebook.LearningRecord{
				{Status: notebook.LearnedStatusMisunderstood},
			},
			want: func() float64 {
				return notebook.UpdateEasinessFactor(notebook.DefaultEasinessFactor, int(notebook.QualityWrong), 0)
			}(),
		},
		{
			name: "logs without quality field infer from status - understood",
			logs: []notebook.LearningRecord{
				{Status: "understood"},
			},
			want: func() float64 {
				return notebook.UpdateEasinessFactor(notebook.DefaultEasinessFactor, int(notebook.QualityCorrect), 0)
			}(),
		},
		{
			name: "multiple logs processed oldest to newest",
			logs: []notebook.LearningRecord{
				{Status: "understood", Quality: int(notebook.QualityCorrect)},
				{Status: "understood", Quality: int(notebook.QualityCorrect)},
			},
			want: func() float64 {
				// Logs are processed from oldest (index 1) to newest (index 0)
				// Process log at index 1 first (oldest)
				ef := notebook.UpdateEasinessFactor(notebook.DefaultEasinessFactor, int(notebook.QualityCorrect), 0)
				// Process log at index 0 (newest), with correctStreak counting from index 1 onward
				ef = notebook.UpdateEasinessFactor(ef, int(notebook.QualityCorrect), 1)
				return ef
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateEasinessFactor(tt.logs)
			assert.InDelta(t, tt.want, got, 0.001)
		})
	}
}

func TestCountCorrectFromIndex(t *testing.T) {
	tests := []struct {
		name      string
		logs      []notebook.LearningRecord
		fromIndex int
		want      int
	}{
		{
			name:      "empty logs",
			logs:      nil,
			fromIndex: 0,
			want:      0,
		},
		{
			name: "single log - counts from next index",
			logs: []notebook.LearningRecord{
				{Status: "understood"},
			},
			fromIndex: 0,
			want:      0,
		},
		{
			name: "consecutive correct after index",
			logs: []notebook.LearningRecord{
				{Status: "understood"},
				{Status: "understood"},
				{Status: "understood"},
			},
			fromIndex: 0,
			want:      2,
		},
		{
			name: "stops at misunderstood",
			logs: []notebook.LearningRecord{
				{Status: "understood"},
				{Status: "understood"},
				{Status: notebook.LearnedStatusMisunderstood},
				{Status: "understood"},
			},
			fromIndex: 0,
			want:      1,
		},
		{
			name: "skips empty status",
			logs: []notebook.LearningRecord{
				{Status: "understood"},
				{Status: ""},
				{Status: "understood"},
			},
			fromIndex: 0,
			want:      1,
		},
		{
			name: "from middle index",
			logs: []notebook.LearningRecord{
				{Status: "understood"},
				{Status: "understood"},
				{Status: "understood"},
				{Status: "understood"},
			},
			fromIndex: 2,
			want:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countCorrectFromIndex(tt.logs, tt.fromIndex)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCalculateLegacyInterval(t *testing.T) {
	tests := []struct {
		name     string
		logIndex int
		logs     []notebook.LearningRecord
		want     int
	}{
		{
			name:     "no correct answers returns zero threshold",
			logIndex: 0,
			logs: []notebook.LearningRecord{
				{Status: notebook.LearnedStatusMisunderstood},
			},
			want: 0,
		},
		{
			name:     "one correct answer",
			logIndex: 0,
			logs: []notebook.LearningRecord{
				{Status: "understood"},
			},
			want: notebook.GetThresholdDaysFromCount(1),
		},
		{
			name:     "skips empty and misunderstood status",
			logIndex: 0,
			logs: []notebook.LearningRecord{
				{Status: "understood"},
				{Status: ""},
				{Status: notebook.LearnedStatusMisunderstood},
				{Status: "understood"},
			},
			want: notebook.GetThresholdDaysFromCount(2),
		},
		{
			name:     "counts from given index",
			logIndex: 1,
			logs: []notebook.LearningRecord{
				{Status: "understood"},
				{Status: "understood"},
				{Status: "understood"},
			},
			want: notebook.GetThresholdDaysFromCount(2),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateLegacyInterval(tt.logIndex, tt.logs)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMigrateExpression(t *testing.T) {
	tests := []struct {
		name string
		exp  notebook.LearningHistoryExpression
		want bool // whether modified
	}{
		{
			name: "no logs and no EF - sets default EF",
			exp: notebook.LearningHistoryExpression{
				Expression: "break the ice",
			},
			want: true,
		},
		{
			name: "already has EF and all logs have quality and interval - no change",
			exp: notebook.LearningHistoryExpression{
				Expression:     "lose one's temper",
				EasinessFactor: 2.5,
				LearnedLogs: []notebook.LearningRecord{
					{
						Status:       "understood",
						Quality:      int(notebook.QualityCorrect),
						IntervalDays: 7,
					},
				},
			},
			want: false,
		},
		{
			name: "logs missing quality - sets quality from status",
			exp: notebook.LearningHistoryExpression{
				Expression:     "hit the road",
				EasinessFactor: 2.5,
				LearnedLogs: []notebook.LearningRecord{
					{Status: notebook.LearnedStatusMisunderstood, IntervalDays: 1},
					{Status: "understood", IntervalDays: 3},
				},
			},
			want: true,
		},
		{
			name: "logs missing interval - sets interval from legacy calculation",
			exp: notebook.LearningHistoryExpression{
				Expression:     "piece of cake",
				EasinessFactor: 2.5,
				LearnedLogs: []notebook.LearningRecord{
					{Status: "understood", Quality: int(notebook.QualityCorrect)},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := tt.exp
			got := migrateExpression(&exp)
			assert.Equal(t, tt.want, got)

			if tt.want {
				// Verify EF was set
				assert.NotZero(t, exp.EasinessFactor)

				// Verify all logs have quality and interval
				for _, log := range exp.LearnedLogs {
					assert.NotZero(t, log.Quality)
					assert.NotZero(t, log.IntervalDays)
				}
			}
		})
	}
}
