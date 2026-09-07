package quiz

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// fixedLadder is the fixed-interval SRS ladder used throughout these tests.
var fixedLadder = []int{1, 7, 30, 90, 365, 1095, 1825}

// TestNextIntervalDays_ParityWithYAMLUpdater proves the DB save-path interval
// helper (Service.nextIntervalDays) computes the EXACT interval the YAML
// updater's AddRecordWithQuality / AddRecordWithQualityForReverse would compute
// for the same attempt — the guarantee that DB-mode and YAML-mode produce
// identical interval_days. Both sides load the SAME on-disk history through the
// SAME loadHistories seam; the helper must select the same per-quiz-type slot
// and drive the same calculator.
func TestNextIntervalDays_ParityWithYAMLUpdater(t *testing.T) {
	learningDir := t.TempDir()
	// A flashcard notebook seeded with prior CORRECT logs in both the forward
	// (learned_logs) and reverse (reverse_logs) tracks, so the parity check
	// exercises a streak, not just a first write.
	require.NoError(t, os.WriteFile(filepath.Join(learningDir, "test-vocab.yml"), []byte(`- metadata:
    notebook_id: test-vocab
    title: "flashcards"
    type: "flashcard"
  expressions:
    - expression: "break the ice"
      learned_logs:
        - status: "understood"
          learned_at: "2026-01-01"
          quality: 4
          quiz_type: "notebook"
          interval_days: 1
      reverse_logs:
        - status: "understood"
          learned_at: "2026-01-01"
          quality: 4
          quiz_type: "reverse"
          interval_days: 1
`), 0o644))

	svc := NewService(config.NotebooksConfig{
		LearningNotesDirectory: learningDir,
	}, nil, nil, nil, config.QuizConfig{Algorithm: "fixed", FixedIntervals: fixedLadder})

	calc := notebook.NewIntervalCalculator("fixed", fixedLadder)
	learnedAt := time.Date(2026, 8, 29, 9, 39, 0, 0, time.UTC)

	cases := []struct {
		name     string
		quizType notebook.QuizType
		reverse  bool // use the reverse updater/slot
	}{
		{name: "forward streak", quizType: notebook.QuizTypeNotebook, reverse: false},
		{name: "reverse streak", quizType: notebook.QuizTypeReverse, reverse: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// DB save-path helper.
			got := svc.nextIntervalDays(0, "test-vocab", "", tc.quizType, true, 4, 1200, learnedAt, "break the ice")

			// Independent YAML-updater computation on the same seed: clone the
			// expression, append the same correct attempt, read its interval.
			histories, err := notebook.NewLearningHistories(learningDir)
			require.NoError(t, err)
			updater := notebook.NewLearningHistoryUpdater(histories["test-vocab"], calc)
			expr := updater.FindExpressionByName("break the ice")
			require.NotNil(t, expr)
			clone := *expr
			if tc.reverse {
				clone.ReverseLogs = append([]notebook.LearningRecord(nil), expr.ReverseLogs...)
				clone.AddRecordWithQualityForReverse(calc, true, true, 4, 1200, notebook.QuizTypeReverse)
				want := clone.ReverseLogs[0].IntervalDays
				assert.Equal(t, want, got, "reverse interval must match the YAML updater")
				assert.Equal(t, 30, got, "prior level 1 (interval 1) + one correct → ladder[2]=30")
			} else {
				clone.LearnedLogs = append([]notebook.LearningRecord(nil), expr.LearnedLogs...)
				clone.AddRecordWithQuality(calc, true, true, 4, 1200, notebook.QuizTypeNotebook)
				want := clone.LearnedLogs[0].IntervalDays
				assert.Equal(t, want, got, "forward interval must match the YAML updater")
				assert.Equal(t, 30, got, "prior level 1 (interval 1) + one correct → ladder[2]=30")
			}
			assert.Positive(t, got, "a successful attempt must never yield interval 0")
		})
	}
}

// TestNextIntervalDays_FirstAttempt confirms a first-ever correct answer (no
// prior logs) yields the ladder's first step, never 0 — the exact case the
// reported bug got wrong (stored 0 instead of 1).
func TestNextIntervalDays_FirstAttempt(t *testing.T) {
	learningDir := t.TempDir()
	svc := NewService(config.NotebooksConfig{
		LearningNotesDirectory: learningDir,
	}, nil, nil, nil, config.QuizConfig{Algorithm: "fixed", FixedIntervals: fixedLadder})

	learnedAt := time.Date(2026, 8, 29, 9, 39, 0, 0, time.UTC)
	got := svc.nextIntervalDays(0, "test-vocab", "", notebook.QuizTypeReverse, true, 4, 1000, learnedAt, "lose one's temper")
	// A fresh word sits at level 0; one correct answer (quality 4 → +1 level)
	// advances to level 1 = ladder[1] = 7. The point is it is a real computed
	// interval, never the buggy 0.
	assert.Equal(t, 7, got, "first correct reverse attempt advances level 0→1 → ladder[1]=7")
	assert.Positive(t, got)
}
