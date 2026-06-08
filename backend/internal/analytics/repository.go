package analytics

import (
	"context"
	"time"
)

// Repository is the read-only data source for the analytics views. The
// langner-server picks a concrete implementation at startup: when MySQL
// is configured the DB repository is used; otherwise the YAML
// implementation falls back to the on-disk learning history files.
type Repository interface {
	// DailySummaries returns one row per day with quiz activity within
	// [now-rangeDays, now]. rangeDays == 0 means "all time".
	// Days are returned newest-first.
	DailySummaries(ctx context.Context, rangeDays int, filters Filters) ([]DailySummary, error)

	// DayDetail returns the per-day rollup for the requested date plus
	// every (note × quiz_type) attempt that was wrong on that day.
	// prevDate / nextDate are the adjacent days with activity that
	// match the same filters; either may be the zero time.
	DayDetail(ctx context.Context, day time.Time, filters Filters) (DayDetail, error)

	// WordHistory returns every attempt for one (note × quiz_type) pair
	// across all time, newest-first.
	WordHistory(ctx context.Context, ref WordRef) (WordHistory, error)
}

// DayDetail bundles the response of Repository.DayDetail.
type DayDetail struct {
	Summary      DailySummary
	WrongWords   []WrongWord
	PreviousDate time.Time
	NextDate     time.Time
}
