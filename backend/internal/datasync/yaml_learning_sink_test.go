package datasync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/learning"
)

func TestYAMLLearningSink_WriteAll(t *testing.T) {
	baseTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		logs     []learning.LearningLog
		wantYAML string
	}{
		{
			name: "learning logs use snake_case field names",
			logs: []learning.LearningLog{
				{
					ID:             1,
					NoteID:         10,
					Status:         "understood",
					LearnedAt:      baseTime,
					Quality:        4,
					ResponseTimeMs: 1500,
					QuizType:       "notebook",
					IntervalDays:   7,
					EasinessFactor: 2.5,
				},
				{
					ID:             2,
					NoteID:         20,
					Status:         "misunderstood",
					LearnedAt:      baseTime.Add(24 * time.Hour),
					Quality:        2,
					ResponseTimeMs: 3000,
					QuizType:       "reverse",
					IntervalDays:   1,
					EasinessFactor: 2.1,
				},
			},
			wantYAML: `- id: 1
  note_id: 10
  status: understood
  learned_at: "2025-01-15"
  quality: 4
  response_time_ms: 1500
  quiz_type: notebook
  interval_days: 7
  easiness_factor: 2.5
- id: 2
  note_id: 20
  status: misunderstood
  learned_at: "2025-01-16"
  quality: 2
  response_time_ms: 3000
  quiz_type: reverse
  interval_days: 1
  easiness_factor: 2.1
`,
		},
		{
			name: "empty logs",
			logs: []learning.LearningLog{},
			wantYAML: `[]
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			sink := NewYAMLLearningSink(dir)

			err := sink.WriteAll(tt.logs)
			require.NoError(t, err)

			got, err := os.ReadFile(filepath.Join(dir, "learning_logs.yml"))
			require.NoError(t, err)
			assert.Equal(t, tt.wantYAML, string(got))
		})
	}

	t.Run("MkdirAll error returns error", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "blocker")
		require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o644))

		sink := NewYAMLLearningSink(filepath.Join(filePath, "subdir"))
		err := sink.WriteAll(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "create output directory")
	})

	t.Run("writeYAML error returns error", func(t *testing.T) {
		dir := t.TempDir()
		// Create learning_logs.yml as a directory to cause os.Create to fail
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "learning_logs.yml"), 0o755))

		sink := NewYAMLLearningSink(dir)
		err := sink.WriteAll([]learning.LearningLog{{ID: 1, NoteID: 10, Status: "understood"}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write learning_logs.yml")
	})

	t.Run("creates output directory if it does not exist", func(t *testing.T) {
		dir := t.TempDir()
		subDir := filepath.Join(dir, "nested", "output")
		sink := NewYAMLLearningSink(subDir)

		err := sink.WriteAll([]learning.LearningLog{})
		require.NoError(t, err)

		info, err := os.Stat(subDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}
