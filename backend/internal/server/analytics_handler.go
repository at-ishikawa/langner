package server

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/analytics"
)

// AnalyticsHandler exposes the analytics views over Connect RPC.
type AnalyticsHandler struct {
	repo analytics.Repository
}

// NewAnalyticsHandler returns a handler that serves analytics requests
// from the given repository (DB or YAML).
func NewAnalyticsHandler(repo analytics.Repository) *AnalyticsHandler {
	return &AnalyticsHandler{repo: repo}
}

// GetDailySummaries returns one row per day.
func (h *AnalyticsHandler) GetDailySummaries(
	ctx context.Context,
	req *connect.Request[apiv1.GetDailySummariesRequest],
) (*connect.Response[apiv1.GetDailySummariesResponse], error) {
	filters := unpackFilters(req.Msg.Filters)
	days, err := h.repo.DailySummaries(ctx, int(req.Msg.RangeDays), filters)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	out := make([]*apiv1.DailySummary, len(days))
	for i, d := range days {
		out[i] = dailySummaryToProto(d)
	}
	return connect.NewResponse(&apiv1.GetDailySummariesResponse{Days: out}), nil
}

// GetDayDetail returns the per-day rollup and wrong words for one day.
func (h *AnalyticsHandler) GetDayDetail(
	ctx context.Context,
	req *connect.Request[apiv1.GetDayDetailRequest],
) (*connect.Response[apiv1.GetDayDetailResponse], error) {
	day, err := time.Parse("2006-01-02", req.Msg.Date)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("date: %w", err))
	}
	detail, err := h.repo.DayDetail(ctx, day, unpackFilters(req.Msg.Filters))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	wrongs := make([]*apiv1.WrongWord, len(detail.WrongWords))
	for i, w := range detail.WrongWords {
		wrongs[i] = wrongWordToProto(w)
	}
	resp := &apiv1.GetDayDetailResponse{
		Summary:      dailySummaryToProto(detail.Summary),
		WrongWords:   wrongs,
		PreviousDate: formatDate(detail.PreviousDate),
		NextDate:     formatDate(detail.NextDate),
	}
	return connect.NewResponse(resp), nil
}

// GetWordHistory returns every attempt for one (word, quiz_type) pair.
func (h *AnalyticsHandler) GetWordHistory(
	ctx context.Context,
	req *connect.Request[apiv1.GetWordHistoryRequest],
) (*connect.Response[apiv1.GetWordHistoryResponse], error) {
	hist, err := h.repo.WordHistory(ctx, analytics.WordRef{
		NoteID:     req.Msg.NoteId,
		ID:         req.Msg.GetSenseId(),
		NotebookID: req.Msg.NotebookId,
		Expression: req.Msg.Expression,
		QuizType:   req.Msg.QuizType,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	attempts := make([]*apiv1.AttemptEntry, len(hist.Attempts))
	for i, a := range hist.Attempts {
		attempts[i] = &apiv1.AttemptEntry{
			Date:                 formatDate(a.Date),
			QuizType:             a.QuizType,
			Result:               a.Result,
			Quality:              int32(a.Quality),
			StreakBeforeWrong:    int32(a.StreakBeforeWrong),
			StreakBeforeCorrect:  int32(a.StreakBeforeCorrect),
		}
	}
	return connect.NewResponse(&apiv1.GetWordHistoryResponse{
		Expression:         hist.Expression,
		NotebookId:         hist.NotebookID,
		NotebookTitle:      hist.NotebookTitle,
		CurrentStatus:      hist.CurrentStatus,
		CurrentWrongStreak: int32(hist.CurrentWrongStreak),
		Attempts:           attempts,
		SenseId:            hist.ID,
	}), nil
}

// GetTrends returns the activity metrics over time for the Trends overview.
func (h *AnalyticsHandler) GetTrends(
	ctx context.Context,
	req *connect.Request[apiv1.GetTrendsRequest],
) (*connect.Response[apiv1.GetTrendsResponse], error) {
	q := analytics.TrendsQuery{
		Granularity: granularityFromProto(req.Msg.Granularity),
		GroupBy:     groupByFromProto(req.Msg.GroupBy),
		Filters:     unpackFilters(req.Msg.Filters),
	}
	if req.Msg.StartDate != "" {
		start, err := time.Parse("2006-01-02", req.Msg.StartDate)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("start_date: %w", err))
		}
		q.Start = start
	}
	if req.Msg.EndDate != "" {
		end, err := time.Parse("2006-01-02", req.Msg.EndDate)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("end_date: %w", err))
		}
		q.End = end
	}
	res, err := h.repo.Trends(ctx, q)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	buckets := make([]*apiv1.TrendBucket, len(res.Buckets))
	for i, b := range res.Buckets {
		series := make([]*apiv1.TrendSeries, len(b.Series))
		for j, s := range b.Series {
			series[j] = &apiv1.TrendSeries{
				GroupKey:     s.GroupKey,
				GroupLabel:   s.GroupLabel,
				Attempts:     int32(s.Attempts),
				WordsTested:  int32(s.WordsTested),
				WordsLearned: int32(s.WordsLearned),
				LevelUps:     int32(s.LevelUps),
				Lapses:       int32(s.Lapses),
			}
		}
		buckets[i] = &apiv1.TrendBucket{Period: formatDate(b.Period), Series: series}
	}
	return connect.NewResponse(&apiv1.GetTrendsResponse{
		Buckets: buckets,
		Summary: &apiv1.TrendsSummary{
			Attempts:     int32(res.Summary.Attempts),
			WordsTested:  int32(res.Summary.WordsTested),
			WordsLearned: int32(res.Summary.WordsLearned),
			LevelUps:     int32(res.Summary.LevelUps),
			Lapses:       int32(res.Summary.Lapses),
		},
		Backlog: &apiv1.BacklogSnapshot{
			NeverCorrect: int32(res.Backlog.NeverCorrect),
			InProgress:   int32(res.Backlog.InProgress),
			Mastered:     int32(res.Backlog.Mastered),
		},
	}), nil
}

func granularityFromProto(g apiv1.Granularity) analytics.Granularity {
	switch g {
	case apiv1.Granularity_GRANULARITY_WEEK:
		return analytics.GranularityWeek
	case apiv1.Granularity_GRANULARITY_MONTH:
		return analytics.GranularityMonth
	case apiv1.Granularity_GRANULARITY_YEAR:
		return analytics.GranularityYear
	default:
		return analytics.GranularityDay
	}
}

func groupByFromProto(g apiv1.TrendGroupBy) analytics.TrendGroupBy {
	switch g {
	case apiv1.TrendGroupBy_TREND_GROUP_BY_QUIZ_TYPE:
		return analytics.TrendGroupByQuizType
	case apiv1.TrendGroupBy_TREND_GROUP_BY_NOTEBOOK:
		return analytics.TrendGroupByNotebook
	case apiv1.TrendGroupBy_TREND_GROUP_BY_STATUS:
		return analytics.TrendGroupByStatus
	case apiv1.TrendGroupBy_TREND_GROUP_BY_LEVEL:
		return analytics.TrendGroupByLevel
	default:
		return analytics.TrendGroupByNone
	}
}

func unpackFilters(f *apiv1.AnalyticsFilters) analytics.Filters {
	if f == nil {
		return analytics.Filters{}
	}
	return analytics.Filters{NotebookID: f.NotebookId, QuizType: f.QuizType}
}

func dailySummaryToProto(d analytics.DailySummary) *apiv1.DailySummary {
	return &apiv1.DailySummary{
		Date:          formatDate(d.Date),
		WrongCount:    int32(d.WrongCount),
		TotalCount:    int32(d.TotalCount),
		NotebookCount: int32(d.NotebookCount),
		QuizTypes:     d.QuizTypes,
	}
}

func wrongWordToProto(w analytics.WrongWord) *apiv1.WrongWord {
	related := make([]*apiv1.RelatedGroup, 0, len(w.RelatedGroups))
	for _, g := range w.RelatedGroups {
		related = append(related, &apiv1.RelatedGroup{
			Kind:    g.Kind,
			Label:   g.Label,
			Members: g.Members,
		})
	}
	return &apiv1.WrongWord{
		NoteId:                w.NoteID,
		SenseId:               w.ID,
		Expression:            w.Expression,
		NotebookId:            w.NotebookID,
		NotebookTitle:         w.NotebookTitle,
		SceneTitle:            w.SceneTitle,
		QuizType:              w.QuizType,
		RecentPattern:         w.RecentPattern,
		CurrentWrongStreak:    int32(w.CurrentWrongStreak),
		PreviousCorrectStreak: int32(w.PreviousCorrectStreak),
		CurrentStatus:         w.CurrentStatus,
		Meaning:               w.Meaning,
		ExampleSentence:       w.ExampleSentence,
		NotebookKind:          w.NotebookKind,
		Skipped:               w.Skipped,
		RelatedGroups:         related,
	}
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}
