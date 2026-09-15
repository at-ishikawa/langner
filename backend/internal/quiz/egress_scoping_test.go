package quiz

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// spyHistoryStore records which read seam each caller uses so the egress
// invariants can be asserted without a live Postgres: there is no unbounded
// whole-history read (HistoryStore has no LoadAll — this file wouldn't compile
// if it did), the Relearn pool reads only its date window, and a batch submit's
// interval computation reuses a single preloaded read per notebook.
type spyHistoryStore struct {
	notebookCalls    int
	notebookIDsCalls int
	dateRangeCalls   int
	lastFrom         time.Time
	lastTo           time.Time
}

func (s *spyHistoryStore) LoadForNotebooks(_ context.Context, _ []string) (map[string][]notebook.LearningHistory, error) {
	s.notebookCalls++
	return map[string][]notebook.LearningHistory{}, nil
}

func (s *spyHistoryStore) LoadForDateRange(_ context.Context, from, to time.Time) (map[string][]notebook.LearningHistory, error) {
	s.dateRangeCalls++
	s.lastFrom, s.lastTo = from, to
	return map[string][]notebook.LearningHistory{}, nil
}

func (s *spyHistoryStore) NotebookIDs(_ context.Context) ([]string, error) {
	s.notebookIDsCalls++
	return nil, nil
}

func newSpyService(t *testing.T) (*Service, *spyHistoryStore) {
	t.Helper()
	svc := NewService(config.NotebooksConfig{}, nil, nil, nil,
		config.QuizConfig{Algorithm: "fixed", FixedIntervals: fixedLadder})
	spy := &spyHistoryStore{}
	svc.SetHistoryStore(spy)
	return svc, spy
}

// TestLoadRelearnPool_ReadsOnlyDateWindow pins that the Relearn pool reads its
// recent-miss window via LoadForDateRange and never issues a whole-history read
// (neither an all-notebooks LoadForNotebooks nor the NotebookIDs enumeration).
func TestLoadRelearnPool_ReadsOnlyDateWindow(t *testing.T) {
	svc, spy := newSpyService(t)
	window := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	_, err := svc.LoadRelearnPool(window)
	require.NoError(t, err)

	require.Equal(t, 1, spy.dateRangeCalls, "Relearn must read via LoadForDateRange")
	require.Equal(t, window, spy.lastFrom, "the window's start must be pushed into the read")
	require.True(t, spy.lastTo.IsZero(), "the upper bound stays open (now)")
	require.Zero(t, spy.notebookCalls, "Relearn must not load every notebook")
	require.Zero(t, spy.notebookIDsCalls, "Relearn must not enumerate all notebooks")
}

// TestNextIntervalDays_ReusesPreloadedHistories pins the batch-submit
// optimization: a preloaded per-notebook history (installed by
// WithPreloadedHistories) is reused, so grading N cards of one notebook issues
// ONE scoped read, not one per card. Without a preload it still self-loads
// (single-answer submits keep working).
func TestNextIntervalDays_ReusesPreloadedHistories(t *testing.T) {
	svc, spy := newSpyService(t)
	learnedAt := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

	// No preload → one scoped read per call.
	svc.nextIntervalDays(context.Background(), "nb", "", notebook.QuizTypeNotebook, true, 4, 1000, learnedAt, "word")
	require.Equal(t, 1, spy.notebookCalls, "an un-preloaded notebook is read via LoadForNotebooks")

	// Preloaded → no further store read for that notebook.
	ctx := WithPreloadedHistories(context.Background(), map[string][]notebook.LearningHistory{"nb": {}})
	svc.nextIntervalDays(ctx, "nb", "", notebook.QuizTypeNotebook, true, 4, 1000, learnedAt, "word")
	svc.nextIntervalDays(ctx, "nb", "", notebook.QuizTypeReverse, true, 4, 1000, learnedAt, "word")
	require.Equal(t, 1, spy.notebookCalls, "a preloaded notebook must not trigger another read")

	// A notebook absent from the preload still falls back to a scoped read.
	svc.nextIntervalDays(ctx, "other", "", notebook.QuizTypeNotebook, true, 4, 1000, learnedAt, "word")
	require.Equal(t, 2, spy.notebookCalls, "a notebook missing from the preload is read on demand")
}
