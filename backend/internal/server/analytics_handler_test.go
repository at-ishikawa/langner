package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/analytics"
)

// fakeRepo lets each test specify the response without standing up a real
// YAML directory or DB. It only implements the analytics.Repository methods.
type fakeRepo struct {
	days      []analytics.DailySummary
	dayDetail analytics.DayDetail
	history   analytics.WordHistory
	wantDays  int
	wantRange int
	wantFilt  analytics.Filters
	gotDays   int
	gotRange  int
	gotFilt   analytics.Filters
	dayDetErr error
	dayCalls  int
	trends    analytics.TrendsResult
	gotTrends analytics.TrendsQuery
}

func (f *fakeRepo) DailySummaries(_ context.Context, rangeDays int, filters analytics.Filters) ([]analytics.DailySummary, error) {
	f.gotDays++
	f.gotRange = rangeDays
	f.gotFilt = filters
	return f.days, nil
}

func (f *fakeRepo) DayDetail(_ context.Context, _ time.Time, _ analytics.Filters) (analytics.DayDetail, error) {
	f.dayCalls++
	if f.dayDetErr != nil {
		return analytics.DayDetail{}, f.dayDetErr
	}
	return f.dayDetail, nil
}

func (f *fakeRepo) WordHistory(_ context.Context, _ analytics.WordRef) (analytics.WordHistory, error) {
	return f.history, nil
}

func (f *fakeRepo) Trends(_ context.Context, q analytics.TrendsQuery) (analytics.TrendsResult, error) {
	f.gotTrends = q
	return f.trends, nil
}

func TestAnalyticsHandler_GetDailySummaries(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-06-05")
	repo := &fakeRepo{
		days: []analytics.DailySummary{
			{Date: day, WrongCount: 2, TotalCount: 5, NotebookCount: 1, QuizTypes: []string{"notebook"}},
		},
	}
	h := NewAnalyticsHandler(repo)
	resp, err := h.GetDailySummaries(context.Background(), connect.NewRequest(&apiv1.GetDailySummariesRequest{
		RangeDays: 30,
		Filters:   &apiv1.AnalyticsFilters{NotebookId: "flashcards"},
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Days, 1)
	assert.Equal(t, "2026-06-05", resp.Msg.Days[0].Date)
	assert.EqualValues(t, 2, resp.Msg.Days[0].WrongCount)
	assert.EqualValues(t, 5, resp.Msg.Days[0].TotalCount)
	assert.Equal(t, 30, repo.gotRange)
	assert.Equal(t, "flashcards", repo.gotFilt.NotebookID)
}

func TestAnalyticsHandler_GetDayDetail(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-06-05")
	prev, _ := time.Parse("2006-01-02", "2026-06-04")
	repo := &fakeRepo{
		dayDetail: analytics.DayDetail{
			Summary: analytics.DailySummary{Date: day, WrongCount: 1, TotalCount: 3},
			WrongWords: []analytics.WrongWord{{
				Expression:            "ephemeral",
				NotebookID:            "flashcards",
				QuizType:              "notebook",
				CurrentWrongStreak:    2,
				PreviousCorrectStreak: 1,
				RecentPattern:         []string{"none", "none", "correct", "wrong", "wrong"},
				CurrentStatus:         "misunderstood",
			}},
			PreviousDate: prev,
		},
	}
	h := NewAnalyticsHandler(repo)
	resp, err := h.GetDayDetail(context.Background(), connect.NewRequest(&apiv1.GetDayDetailRequest{Date: "2026-06-05"}))
	require.NoError(t, err)
	assert.Equal(t, "2026-06-05", resp.Msg.Summary.Date)
	require.Len(t, resp.Msg.WrongWords, 1)
	w := resp.Msg.WrongWords[0]
	assert.Equal(t, "ephemeral", w.Expression)
	assert.EqualValues(t, 2, w.CurrentWrongStreak)
	assert.EqualValues(t, 1, w.PreviousCorrectStreak)
	assert.Equal(t, "2026-06-04", resp.Msg.PreviousDate)
	assert.Equal(t, "", resp.Msg.NextDate)
}

func TestAnalyticsHandler_GetDayDetail_InvalidDate(t *testing.T) {
	h := NewAnalyticsHandler(&fakeRepo{})
	_, err := h.GetDayDetail(context.Background(), connect.NewRequest(&apiv1.GetDayDetailRequest{Date: "not-a-date"}))
	require.Error(t, err)
	var ce *connect.Error
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, connect.CodeInvalidArgument, ce.Code())
}

// emptyDBRepo simulates the production DB repository in a setup where
// etymology quizzes have been answered but the rows never reached
// learning_logs — exactly what SaveEtymologyOriginResult does today,
// since it only writes YAML. The test uses this to reproduce the bug
// where the analytics handler returned no etymology results when wired
// against DB-only.
type emptyDBRepo struct{}

func (emptyDBRepo) DailySummaries(context.Context, int, analytics.Filters) ([]analytics.DailySummary, error) {
	return nil, nil
}
func (emptyDBRepo) DayDetail(context.Context, time.Time, analytics.Filters) (analytics.DayDetail, error) {
	return analytics.DayDetail{}, nil
}
func (emptyDBRepo) WordHistory(context.Context, analytics.WordRef) (analytics.WordHistory, error) {
	return analytics.WordHistory{}, nil
}
func (emptyDBRepo) Trends(context.Context, analytics.TrendsQuery) (analytics.TrendsResult, error) {
	return analytics.TrendsResult{}, nil
}

// writeEtymologyLearningHistory lays out one YAML learning history file
// with an etymology breakdown log marked misunderstood on the requested
// date. The fixture mirrors a real "word-power-made-easy" notebook with
// origin "logos" failed on 2026-06-07.
func writeEtymologyLearningHistory(t *testing.T, dir, date string) {
	t.Helper()
	body := `- metadata:
    id: word-power-made-easy
    title: "Word Power Made Easy"
  scenes:
    - metadata:
        title: "Session 1"
      expressions:
        - expression: logos
          type: origin
          etymology_breakdown_logs:
            - status: misunderstood
              learned_at: "` + date + `"
              quality: 1
              quiz_type: etymology_breakdown
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "word-power-made-easy.yml"), []byte(body), 0o600))
}

// TestAnalyticsHandler_GetDayDetail_EtymologyMissingFromDB is the bug
// reproduction. With a DB-only repo (the production wiring before this
// commit) etymology results never reach the response. With a YAML-backed
// repo (or a fallback that consults YAML) they do.
func TestAnalyticsHandler_GetDayDetail_EtymologyMissingFromDB(t *testing.T) {
	dir := t.TempDir()
	writeEtymologyLearningHistory(t, dir, "2026-06-07")

	// 1) DB-only: etymology fails to appear (this is the bug the user reported).
	dbOnly := NewAnalyticsHandler(emptyDBRepo{})
	resp, err := dbOnly.GetDayDetail(context.Background(), connect.NewRequest(&apiv1.GetDayDetailRequest{Date: "2026-06-07"}))
	require.NoError(t, err)
	require.Empty(t, resp.Msg.WrongWords, "etymology results should be missing when only DB is consulted")

	// 2) Fixed wiring: an analytics repo that can read the YAML history file
	// surfaces the misunderstood etymology breakdown attempt.
	yamlRepo := analytics.NewYAMLRepository(dir)
	fixed := NewAnalyticsHandler(yamlRepo)
	resp, err = fixed.GetDayDetail(context.Background(), connect.NewRequest(&apiv1.GetDayDetailRequest{Date: "2026-06-07"}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.WrongWords, 1, "expected one etymology origin to appear in analytics")
	w := resp.Msg.WrongWords[0]
	assert.Equal(t, "logos", w.Expression)
	assert.Equal(t, "etymology_breakdown", w.QuizType)
}

func TestAnalyticsHandler_GetTrends(t *testing.T) {
	period, _ := time.Parse("2006-01-02", "2026-06-01")
	repo := &fakeRepo{
		trends: analytics.TrendsResult{
			Buckets: []analytics.TrendBucket{{
				Period: period,
				Series: []analytics.TrendSeries{
					{GroupKey: "notebook", GroupLabel: "Notebook", Attempts: 12, WordsTested: 8, WordsLearned: 3, LevelUps: 2},
				},
			}},
			Summary: analytics.TrendsSummary{Attempts: 12, WordsTested: 8, WordsLearned: 3, LevelUps: 2},
			Backlog: analytics.Backlog{NeverCorrect: 4, InProgress: 10, Mastered: 20},
		},
	}
	h := NewAnalyticsHandler(repo)
	resp, err := h.GetTrends(context.Background(), connect.NewRequest(&apiv1.GetTrendsRequest{
		Granularity: apiv1.Granularity_GRANULARITY_MONTH,
		GroupBy:     apiv1.TrendGroupBy_TREND_GROUP_BY_QUIZ_TYPE,
		StartDate:   "2026-01-01",
		EndDate:     "2026-06-30",
		Filters:     &apiv1.AnalyticsFilters{NotebookId: "flashcards"},
	}))
	require.NoError(t, err)
	// Request was decoded into the domain query.
	assert.Equal(t, analytics.GranularityMonth, repo.gotTrends.Granularity)
	assert.Equal(t, analytics.TrendGroupByQuizType, repo.gotTrends.GroupBy)
	assert.Equal(t, "flashcards", repo.gotTrends.Filters.NotebookID)
	assert.Equal(t, "2026-01-01", repo.gotTrends.Start.Format("2006-01-02"))
	assert.Equal(t, "2026-06-30", repo.gotTrends.End.Format("2006-01-02"))
	// Response carries buckets, summary and backlog.
	require.Len(t, resp.Msg.Buckets, 1)
	assert.Equal(t, "2026-06-01", resp.Msg.Buckets[0].Period)
	require.Len(t, resp.Msg.Buckets[0].Series, 1)
	assert.Equal(t, "Notebook", resp.Msg.Buckets[0].Series[0].GroupLabel)
	assert.EqualValues(t, 8, resp.Msg.Buckets[0].Series[0].WordsTested)
	assert.EqualValues(t, 3, resp.Msg.Summary.WordsLearned)
	assert.EqualValues(t, 20, resp.Msg.Backlog.Mastered)
	assert.EqualValues(t, 4, resp.Msg.Backlog.NeverCorrect)
}

func TestAnalyticsHandler_GetWordHistory(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-06-05")
	repo := &fakeRepo{
		history: analytics.WordHistory{
			Expression:         "ephemeral",
			NotebookID:         "flashcards",
			CurrentStatus:      "misunderstood",
			CurrentWrongStreak: 2,
			Attempts: []analytics.AttemptEntry{
				{Date: day, QuizType: "notebook", Result: "wrong", Quality: 1, StreakBeforeWrong: 1},
			},
		},
	}
	h := NewAnalyticsHandler(repo)
	resp, err := h.GetWordHistory(context.Background(), connect.NewRequest(&apiv1.GetWordHistoryRequest{
		Expression: "ephemeral",
		QuizType:   "notebook",
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Attempts, 1)
	assert.Equal(t, "2026-06-05", resp.Msg.Attempts[0].Date)
	assert.Equal(t, "wrong", resp.Msg.Attempts[0].Result)
	assert.EqualValues(t, 2, resp.Msg.CurrentWrongStreak)
}
