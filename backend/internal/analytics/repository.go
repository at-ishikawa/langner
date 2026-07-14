package analytics

import (
	"context"
	"time"
)

// Repository is the read-only data source for the analytics views. The
// langner-server picks a concrete implementation at startup: when a
// PostgreSQL database is configured the DB repository is used;
// otherwise the YAML implementation falls back to the on-disk learning
// history files.
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

	// Trends returns the activity metrics over time for the Trends
	// overview: per-bucket series, range totals, and the end-of-range
	// backlog snapshot. Implementations load every in-scope attempt and
	// delegate the aggregation to ComputeTrends so the YAML and DB paths
	// stay identical.
	Trends(ctx context.Context, q TrendsQuery) (TrendsResult, error)
}

// DayDetail bundles the response of Repository.DayDetail.
type DayDetail struct {
	Summary      DailySummary
	WrongWords   []WrongWord
	PreviousDate time.Time
	NextDate     time.Time
}

// MetadataResolver hydrates a wrong word with the canonical meaning,
// one example sentence, and the source notebook's kind. The analytics
// repository walks learning history files which only know expression
// strings and notebook IDs; the resolver bridges to wherever meanings
// live (story / flashcard YAML for vocab, etymology session YAML for
// origins).
//
// Implementations may return zero values when no match is found; the
// frontend hides empty fields rather than failing.
type MetadataResolver interface {
	// Resolve returns the meaning + example for an expression on the
	// analytics card. quizType is the LearningRecord.QuizType of the
	// attempt that produced the card; resolvers use it to pick between
	// vocabulary and etymology-origin lookups when the expression
	// collides across both senses (e.g. "gauche" is both an English
	// adjective meaning "clumsy" AND a French origin meaning "left").
	// expressionType comes from the learning-history record's `type`
	// field and is the fallback signal when quizType is ambiguous.
	Resolve(ctx context.Context, notebookID, expression, expressionType, quizType string) WordMetadata
}

// noMetadataResolver is the default no-op resolver. The handler uses it
// when no real resolver is wired so the YAML path still works in tests
// and YAML-only test setups.
type noMetadataResolver struct{}

// NoMetadataResolver returns a resolver that yields empty metadata for
// every lookup. Useful in tests and when the server lacks the notebook
// fixtures needed to populate meanings.
func NoMetadataResolver() MetadataResolver { return noMetadataResolver{} }

func (noMetadataResolver) Resolve(_ context.Context, _, _, _, _ string) WordMetadata {
	return WordMetadata{}
}
