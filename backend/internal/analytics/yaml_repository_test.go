package analytics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const sampleHistoryYAML = `- metadata:
    id: flashcards
    title: Flashcards
    type: flashcard
  expressions:
    - expression: ephemeral
      learned_logs:
        - status: misunderstood
          learned_at: "2026-06-05"
          quality: 1
          quiz_type: notebook
        - status: misunderstood
          learned_at: "2026-05-30"
          quality: 1
          quiz_type: notebook
        - status: understood
          learned_at: "2026-05-20"
          quality: 4
          quiz_type: notebook
    - expression: thrilled
      learned_logs:
        - status: misunderstood
          learned_at: "2026-06-05"
          quality: 1
          quiz_type: notebook
        - status: understood
          learned_at: "2026-06-01"
          quality: 4
          quiz_type: notebook
        - status: understood
          learned_at: "2026-05-25"
          quality: 4
          quiz_type: notebook
      reverse_logs:
        - status: understood
          learned_at: "2026-06-04"
          quality: 5
          quiz_type: reverse
`

func writeSampleHistory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "flashcards.yml"), []byte(sampleHistoryYAML), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	return dir
}

func TestYAMLRepository_DailySummaries(t *testing.T) {
	dir := writeSampleHistory(t)
	repo := NewYAMLRepository(dir)
	got, err := repo.DailySummaries(context.Background(), 0, Filters{})
	if err != nil {
		t.Fatalf("DailySummaries: %v", err)
	}
	// 5 distinct dates: Jun 5, Jun 4, Jun 1, May 30, May 25, May 20.
	if len(got) != 6 {
		t.Fatalf("got %d days, want 6 (%v)", len(got), got)
	}
	// Newest first.
	if got[0].Date.Format("2006-01-02") != "2026-06-05" {
		t.Fatalf("first day: got %s, want 2026-06-05", got[0].Date.Format("2006-01-02"))
	}
	// Jun 5 has 2 wrong attempts (ephemeral + thrilled), 2 total.
	if got[0].WrongCount != 2 || got[0].TotalCount != 2 {
		t.Fatalf("Jun 5: got wrong=%d total=%d, want 2/2", got[0].WrongCount, got[0].TotalCount)
	}
}

func TestYAMLRepository_DayDetail(t *testing.T) {
	dir := writeSampleHistory(t)
	repo := NewYAMLRepository(dir)
	day, _ := time.Parse("2006-01-02", "2026-06-05")
	got, err := repo.DayDetail(context.Background(), day, Filters{})
	if err != nil {
		t.Fatalf("DayDetail: %v", err)
	}
	if len(got.WrongWords) != 2 {
		t.Fatalf("got %d wrong words, want 2", len(got.WrongWords))
	}
	// ephemeral wrong streak is 2 (Jun 5 and May 30), thrilled wrong streak is 1.
	for _, w := range got.WrongWords {
		switch w.Expression {
		case "ephemeral":
			if w.CurrentWrongStreak != 2 {
				t.Errorf("ephemeral wrong streak: got %d, want 2", w.CurrentWrongStreak)
			}
			if w.PreviousCorrectStreak != 1 {
				t.Errorf("ephemeral prev correct: got %d, want 1", w.PreviousCorrectStreak)
			}
		case "thrilled":
			if w.CurrentWrongStreak != 1 {
				t.Errorf("thrilled wrong streak: got %d, want 1", w.CurrentWrongStreak)
			}
			if w.PreviousCorrectStreak != 2 {
				t.Errorf("thrilled prev correct: got %d, want 2", w.PreviousCorrectStreak)
			}
		}
	}
	// Previous day with activity is Jun 4 (thrilled reverse) — but Jun 4 has activity.
	if got.PreviousDate.Format("2006-01-02") != "2026-06-04" {
		t.Errorf("prev date: got %s, want 2026-06-04", got.PreviousDate.Format("2006-01-02"))
	}
	if !got.NextDate.IsZero() {
		t.Errorf("next date: got %s, want zero", got.NextDate)
	}
}

func TestYAMLRepository_WordHistory(t *testing.T) {
	dir := writeSampleHistory(t)
	repo := NewYAMLRepository(dir)
	got, err := repo.WordHistory(context.Background(), WordRef{
		NotebookID: "flashcards",
		Expression: "ephemeral",
		QuizType:   "notebook",
	})
	if err != nil {
		t.Fatalf("WordHistory: %v", err)
	}
	if len(got.Attempts) != 3 {
		t.Fatalf("got %d attempts, want 3", len(got.Attempts))
	}
	if got.CurrentWrongStreak != 2 {
		t.Errorf("got wrong streak %d, want 2", got.CurrentWrongStreak)
	}
	if got.Attempts[0].Date.Format("2006-01-02") != "2026-06-05" {
		t.Errorf("newest attempt date: got %s, want 2026-06-05", got.Attempts[0].Date.Format("2006-01-02"))
	}
	if got.Attempts[2].StreakBeforeCorrect != 0 || got.Attempts[2].StreakBeforeWrong != 0 {
		t.Errorf("oldest streak: got w=%d c=%d, want 0/0", got.Attempts[2].StreakBeforeWrong, got.Attempts[2].StreakBeforeCorrect)
	}
}

func TestYAMLRepository_NotebookFilter(t *testing.T) {
	dir := writeSampleHistory(t)
	repo := NewYAMLRepository(dir)
	got, err := repo.DailySummaries(context.Background(), 0, Filters{NotebookID: "no-such-notebook"})
	if err != nil {
		t.Fatalf("DailySummaries with filter: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected zero rows for unknown notebook, got %d", len(got))
	}
}
