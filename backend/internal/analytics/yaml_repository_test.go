package analytics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

// TestYAMLRepository_DayBoundaryUTC reproduces the day-boundary bug. When the
// quiz is answered late at night in a westward timezone the YAML learned_at
// crosses the UTC midnight, but the Quiz Complete deep link in the frontend
// computes `new Date().toISOString().slice(0,10)` — i.e. today in UTC. If the
// analytics repo groups by the time's own zone, the entry lands on yesterday-
// local and the UTC-keyed Day Detail request comes back empty.
func TestYAMLRepository_DayBoundaryUTC(t *testing.T) {
	dir := t.TempDir()
	// 11pm PDT on 2026-06-08 == 06:00 UTC on 2026-06-09. The analytics repo
	// should bucket this under the UTC date so the frontend, which always
	// uses UTC today for its deep link, can find it.
	body := `- metadata:
    id: word-power-made-easy
    title: "Word Power Made Easy"
  scenes:
    - metadata:
        title: "Session 1"
      expressions:
        - expression: tele
          type: origin
          etymology_assembly_logs:
            - status: misunderstood
              learned_at: "2026-06-08T23:00:00-07:00"
              quality: 1
              quiz_type: etymology_assembly
              interval_days: 0
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "word-power-made-easy.yml"), []byte(body), 0o600))

	repo := NewYAMLRepository(dir)
	utcDay, _ := time.Parse("2006-01-02", "2026-06-09")
	detail, err := repo.DayDetail(context.Background(), utcDay, Filters{})
	require.NoError(t, err)
	require.Len(t, detail.WrongWords, 1,
		"expected the etymology reverse failure to surface under the UTC date %s",
		"2026-06-09")
}

// TestYAMLRepository_EtymologyReverseToday is the reproduction for the bug
// reported when a user answers an etymology reverse quiz and then opens
// Analytics: the failure should show up under today's date. Etymology
// reverse writes to etymology_assembly_logs with quiz_type=etymology_assembly;
// the repo must include that track in DailySummaries / DayDetail.
func TestYAMLRepository_EtymologyReverseToday(t *testing.T) {
	dir := t.TempDir()
	today := time.Now().UTC().Format("2006-01-02")
	// Fixture mirrors what AddRecordWithQualityForEtymology writes for a
	// misunderstood etymology reverse answer on the word-power-made-easy
	// notebook.
	body := `- metadata:
    id: word-power-made-easy
    title: "Word Power Made Easy"
  scenes:
    - metadata:
        title: "Session 1"
      expressions:
        - expression: tele
          type: origin
          etymology_assembly_logs:
            - status: misunderstood
              learned_at: "` + today + `T15:30:00Z"
              quality: 1
              quiz_type: etymology_assembly
              interval_days: 0
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "word-power-made-easy.yml"), []byte(body), 0o600))

	repo := NewYAMLRepository(dir)

	// Day List should include today.
	days, err := repo.DailySummaries(context.Background(), 0, Filters{})
	require.NoError(t, err)
	require.NotEmpty(t, days, "expected today's day row, got none")
	require.Equal(t, today, days[0].Date.Format("2006-01-02"))
	require.Equal(t, 1, days[0].WrongCount)

	// Day Detail should list the etymology origin.
	dayTime, _ := time.Parse("2006-01-02", today)
	detail, err := repo.DayDetail(context.Background(), dayTime, Filters{})
	require.NoError(t, err)
	require.Len(t, detail.WrongWords, 1, "expected the etymology origin to appear under today")
	w := detail.WrongWords[0]
	require.Equal(t, "tele", w.Expression)
	require.Equal(t, "etymology_assembly", w.QuizType)
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
