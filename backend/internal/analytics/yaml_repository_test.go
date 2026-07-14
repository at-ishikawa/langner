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

// TestYAMLRepository_TrendsGroupsByNotebookFile guards the fix for the
// "notebook split shows episode titles" bug: a story's episode title
// (Metadata.Title) must NOT become the notebook group — the notebook
// dimension is the learning-history filename.
func TestYAMLRepository_TrendsGroupsByNotebookFile(t *testing.T) {
	dir := t.TempDir()
	storyYAML := `- metadata:
    id: friends
    title: "Episode 1 - The Pilot"
  scenes:
    - metadata:
        title: "Central Perk"
      expressions:
        - expression: break the ice
          learned_logs:
            - status: understood
              learned_at: "2026-06-05"
              quality: 4
              quiz_type: notebook
`
	if err := os.WriteFile(filepath.Join(dir, "friends.yml"), []byte(storyYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := NewYAMLRepository(dir)
	res, err := repo.Trends(context.Background(), TrendsQuery{
		Granularity: GranularityMonth,
		GroupBy:     TrendGroupByNotebook,
	})
	if err != nil {
		t.Fatalf("Trends: %v", err)
	}
	if len(res.Buckets) != 1 || len(res.Buckets[0].Series) != 1 {
		t.Fatalf("got %d buckets", len(res.Buckets))
	}
	s := res.Buckets[0].Series[0]
	if s.GroupKey != "friends" || s.GroupLabel != "friends" {
		t.Errorf("notebook group: got key=%q label=%q, want the filename \"friends\" (not the episode title)", s.GroupKey, s.GroupLabel)
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

// TestYAMLRepository_DayBoundaryLocalZone pins the day-bucketing semantics
// the user expects: an entry written at 5pm PDT on Monday belongs to
// Monday — i.e. the date in the record's stored zone — not Tuesday UTC.
// Forcing UTC there was a regression (the user reported "analytics shows
// Tuesday for today's answer though I answered the quiz on Monday before
// 6pm PT"). The frontend's Quiz Complete deep link mirrors this by also
// computing the local YYYY-MM-DD instead of using toISOString().
func TestYAMLRepository_DayBoundaryLocalZone(t *testing.T) {
	dir := t.TempDir()
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
              learned_at: "2026-06-08T17:30:00-07:00"
              quality: 1
              quiz_type: etymology_assembly
              interval_days: 0
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "word-power-made-easy.yml"), []byte(body), 0o600))

	repo := NewYAMLRepository(dir)

	// Should appear under Monday (2026-06-08), the local-zone date.
	monday, _ := time.Parse("2006-01-02", "2026-06-08")
	monDetail, err := repo.DayDetail(context.Background(), monday, Filters{})
	require.NoError(t, err)
	require.Len(t, monDetail.WrongWords, 1, "expected the entry on Monday (its stored zone)")

	// And must NOT appear under Tuesday UTC.
	tuesday, _ := time.Parse("2006-01-02", "2026-06-09")
	tueDetail, err := repo.DayDetail(context.Background(), tuesday, Filters{})
	require.NoError(t, err)
	require.Empty(t, tueDetail.WrongWords, "Monday-local entries must not leak into Tuesday UTC")
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

// TestYAMLRepository_DayDetailExposesSkipped pins the analytics-card
// "Excluded" badge data path: when a wrong attempt's underlying
// expression has skipped_at set for the matching quiz type, the
// resulting WrongWord must carry Skipped=true so the frontend can
// render the badge. The skip is per-quiz-type, so a skip on `notebook`
// must not bleed into a wrong attempt on `reverse`.
func TestYAMLRepository_DayDetailExposesSkipped(t *testing.T) {
	dir := t.TempDir()
	body := `- metadata:
    id: flashcards
    title: Flashcards
    type: flashcard
  expressions:
    - expression: lose-temper
      skipped_at:
        notebook: "2026-06-08T12:00:00Z"
      learned_logs:
        - status: misunderstood
          learned_at: "2026-06-09T10:00:00Z"
          quality: 1
          quiz_type: notebook
      reverse_logs:
        - status: misunderstood
          learned_at: "2026-06-09T10:30:00Z"
          quality: 1
          quiz_type: reverse
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "flashcards.yml"), []byte(body), 0o600))

	repo := NewYAMLRepository(dir)
	day, _ := time.Parse("2006-01-02", "2026-06-09")
	got, err := repo.DayDetail(context.Background(), day, Filters{})
	require.NoError(t, err)
	require.Len(t, got.WrongWords, 2, "both wrong attempts must surface")

	bySlot := make(map[string]WrongWord, len(got.WrongWords))
	for _, w := range got.WrongWords {
		bySlot[w.QuizType] = w
	}
	require.True(t, bySlot["notebook"].Skipped,
		"notebook card must carry Skipped=true because skipped_at.notebook is set")
	require.False(t, bySlot["reverse"].Skipped,
		"reverse card must carry Skipped=false because skipped_at has no reverse entry — skips are per quiz type")
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
