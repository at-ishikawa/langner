package server

import (
	"context"
	"errors"
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
