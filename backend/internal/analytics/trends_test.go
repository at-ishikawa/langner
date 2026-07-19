package analytics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ta builds one attempt at midnight UTC on the given YYYY-MM-DD date.
func ta(expr, quiz, status string, quality, interval int, date string) TrendAttempt {
	d, _ := time.Parse("2006-01-02", date)
	return TrendAttempt{
		Expression:    expr,
		NotebookID:    "nb",
		NotebookTitle: "NB",
		QuizType:      quiz,
		Status:        status,
		Quality:       quality,
		IntervalDays:  interval,
		LearnedAt:     d,
	}
}

// seriesByKey finds the series with the given group key in a bucket.
func seriesByKey(t *testing.T, b TrendBucket, key string) TrendSeries {
	t.Helper()
	for _, s := range b.Series {
		if s.GroupKey == key {
			return s
		}
	}
	t.Fatalf("series %q not found in bucket %s", key, b.Period.Format("2006-01-02"))
	return TrendSeries{}
}

func TestComputeTrends_DistinctWordsPerBucket(t *testing.T) {
	// The same word drilled ten times in one month is ten attempts but one
	// word tested — the invariant the user called out explicitly.
	var attempts []TrendAttempt
	for i := 0; i < 10; i++ {
		day := time.Date(2026, 6, 1+i, 9, 0, 0, 0, time.UTC).Format("2006-01-02")
		attempts = append(attempts, ta("bite the bullet", "notebook", "understood", 4, 7, day))
	}
	res := ComputeTrends(attempts, TrendsQuery{Granularity: GranularityMonth})

	require.Len(t, res.Buckets, 1)
	b := res.Buckets[0]
	assert.Equal(t, "2026-06-01", b.Period.Format("2006-01-02"))
	require.Len(t, b.Series, 1)
	assert.Equal(t, 10, b.Series[0].Attempts)
	assert.Equal(t, 1, b.Series[0].WordsTested)
	assert.Equal(t, 10, res.Summary.Attempts)
	assert.Equal(t, 1, res.Summary.WordsTested)
}

func TestComputeTrends_WordsLearnedOnCrossing(t *testing.T) {
	// A word crosses from misunderstood into understood: learned once, in
	// the bucket of the crossing attempt. Re-answering while already
	// retained does not re-count it.
	attempts := []TrendAttempt{
		ta("break the ice", "notebook", "misunderstood", 1, 1, "2026-06-02"),
		ta("break the ice", "notebook", "understood", 4, 7, "2026-06-10"),
		ta("break the ice", "notebook", "understood", 4, 30, "2026-06-20"),
	}
	res := ComputeTrends(attempts, TrendsQuery{Granularity: GranularityMonth})

	require.Len(t, res.Buckets, 1)
	assert.Equal(t, 1, res.Buckets[0].Series[0].WordsLearned)
	assert.Equal(t, 1, res.Summary.WordsLearned)
}

func TestComputeTrends_WordLearnedInTwoModesCountsOnce(t *testing.T) {
	// The same word crosses into a retained status in BOTH the standard
	// (notebook) quiz and the reverse quiz in June. "Words learned" counts
	// the word, not the word × mode: with no split it is one, and the range
	// KPI is one — but split by quiz type it shows once under each mode
	// (so the two bars sum to two while the KPI stays one).
	attempts := []TrendAttempt{
		ta("candor", "notebook", "misunderstood", 1, 1, "2026-06-02"),
		ta("candor", "notebook", "understood", 4, 7, "2026-06-10"),
		ta("candor", "reverse", "misunderstood", 1, 1, "2026-06-03"),
		ta("candor", "reverse", "understood", 4, 7, "2026-06-11"),
	}

	noSplit := ComputeTrends(attempts, TrendsQuery{Granularity: GranularityMonth})
	require.Len(t, noSplit.Buckets, 1)
	require.Len(t, noSplit.Buckets[0].Series, 1)
	assert.Equal(t, 1, noSplit.Buckets[0].Series[0].WordsLearned, "no split: one word learned, not two")
	assert.Equal(t, 1, noSplit.Summary.WordsLearned)

	byQuiz := ComputeTrends(attempts, TrendsQuery{Granularity: GranularityMonth, GroupBy: TrendGroupByQuizType})
	require.Len(t, byQuiz.Buckets, 1)
	assert.Equal(t, 1, seriesByKey(t, byQuiz.Buckets[0], "notebook").WordsLearned)
	assert.Equal(t, 1, seriesByKey(t, byQuiz.Buckets[0], "reverse").WordsLearned)
	// Deduplicated across modes for the headline total.
	assert.Equal(t, 1, byQuiz.Summary.WordsLearned)
}

func TestComputeTrends_LevelUpsByBox(t *testing.T) {
	// Interval climbs 1 -> 7 -> 30, i.e. box 0 -> 1 -> 2. Grouped by level,
	// each level-up lands in the box it reached.
	attempts := []TrendAttempt{
		ta("resilient", "notebook", "understood", 4, 1, "2026-06-01"), // box 0, first attempt: no level-up
		ta("resilient", "notebook", "understood", 4, 7, "2026-06-08"), // box 1: level-up into 7-day
		ta("resilient", "notebook", "usable", 5, 30, "2026-06-20"),    // box 2: level-up into 30-day
	}
	res := ComputeTrends(attempts, TrendsQuery{Granularity: GranularityMonth, GroupBy: TrendGroupByLevel})

	require.Len(t, res.Buckets, 1)
	b := res.Buckets[0]
	// Boxes are ordered by ascending interval.
	assert.Equal(t, []string{"1", "7", "30"}, []string{b.Series[0].GroupKey, b.Series[1].GroupKey, b.Series[2].GroupKey})
	assert.Equal(t, "7-day", seriesByKey(t, b, "7").GroupLabel)
	assert.Equal(t, 0, seriesByKey(t, b, "1").LevelUps)
	assert.Equal(t, 1, seriesByKey(t, b, "7").LevelUps)
	assert.Equal(t, 1, seriesByKey(t, b, "30").LevelUps)
	assert.Equal(t, 2, res.Summary.LevelUps)
}

func TestComputeTrends_LapseOnWrong(t *testing.T) {
	// A wrong answer drops the box: that is a lapse, not a level-up.
	attempts := []TrendAttempt{
		ta("gauche", "notebook", "understood", 4, 30, "2026-06-01"),   // box 2
		ta("gauche", "notebook", "misunderstood", 1, 7, "2026-06-05"), // box 1: lapse
	}
	res := ComputeTrends(attempts, TrendsQuery{Granularity: GranularityMonth})
	require.Len(t, res.Buckets, 1)
	assert.Equal(t, 1, res.Buckets[0].Series[0].Lapses)
	assert.Equal(t, 0, res.Buckets[0].Series[0].LevelUps)
	assert.Equal(t, 1, res.Summary.Lapses)
}

func TestComputeTrends_DistinctInRangeVsBucketSum(t *testing.T) {
	// One word tested across two months is two bucket entries but one
	// distinct word in the range total — the "bars don't sum to the KPI"
	// property.
	attempts := []TrendAttempt{
		ta("serendipity", "notebook", "understood", 4, 7, "2026-05-15"),
		ta("serendipity", "notebook", "understood", 4, 30, "2026-06-15"),
	}
	res := ComputeTrends(attempts, TrendsQuery{Granularity: GranularityMonth})

	require.Len(t, res.Buckets, 2)
	assert.Equal(t, 1, res.Buckets[0].Series[0].WordsTested)
	assert.Equal(t, 1, res.Buckets[1].Series[0].WordsTested)
	// Sum of buckets is 2, but the de-duplicated range total is 1.
	assert.Equal(t, 1, res.Summary.WordsTested)
}

func TestComputeTrends_GroupByQuizType(t *testing.T) {
	attempts := []TrendAttempt{
		ta("aloof", "notebook", "understood", 4, 7, "2026-06-02"),
		ta("aloof", "reverse", "understood", 4, 7, "2026-06-02"),
		ta("candid", "notebook", "understood", 4, 7, "2026-06-03"),
	}
	res := ComputeTrends(attempts, TrendsQuery{Granularity: GranularityMonth, GroupBy: TrendGroupByQuizType})

	require.Len(t, res.Buckets, 1)
	b := res.Buckets[0]
	// Canonical order puts notebook before reverse.
	assert.Equal(t, "notebook", b.Series[0].GroupKey)
	assert.Equal(t, "reverse", b.Series[1].GroupKey)
	assert.Equal(t, 2, seriesByKey(t, b, "notebook").WordsTested)
	assert.Equal(t, 1, seriesByKey(t, b, "reverse").WordsTested)
}

func TestComputeTrends_RangeFilterKeepsPriorState(t *testing.T) {
	// The May crossing is outside the range, so June shows no new learned
	// word even though the word is answered again in June — its state was
	// already retained before the range began.
	attempts := []TrendAttempt{
		ta("ephemeral", "notebook", "misunderstood", 1, 1, "2026-05-01"),
		ta("ephemeral", "notebook", "understood", 4, 7, "2026-05-10"), // crossing in May
		ta("ephemeral", "notebook", "understood", 4, 30, "2026-06-05"),
	}
	start, _ := time.Parse("2006-01-02", "2026-06-01")
	res := ComputeTrends(attempts, TrendsQuery{Granularity: GranularityMonth, Start: start})

	require.Len(t, res.Buckets, 1)
	assert.Equal(t, "2026-06-01", res.Buckets[0].Period.Format("2006-01-02"))
	assert.Equal(t, 0, res.Buckets[0].Series[0].WordsLearned)
	assert.Equal(t, 0, res.Summary.WordsLearned)
}

func TestComputeTrends_Backlog(t *testing.T) {
	attempts := []TrendAttempt{
		// mastered: latest status usable
		ta("mastered_word", "notebook", "understood", 4, 7, "2026-06-01"),
		ta("mastered_word", "notebook", "usable", 5, 30, "2026-06-10"),
		// in progress: retained but not mastered
		ta("progress_word", "notebook", "understood", 4, 7, "2026-06-05"),
		// never correct: only misunderstood
		ta("stuck_word", "notebook", "misunderstood", 1, 1, "2026-06-06"),
		ta("stuck_word", "notebook", "misunderstood", 1, 1, "2026-06-08"),
	}
	res := ComputeTrends(attempts, TrendsQuery{Granularity: GranularityMonth})

	assert.Equal(t, 1, res.Backlog.Mastered)
	assert.Equal(t, 1, res.Backlog.InProgress)
	assert.Equal(t, 1, res.Backlog.NeverCorrect)
}

func TestComputeTrends_YearBucketing(t *testing.T) {
	attempts := []TrendAttempt{
		ta("a", "notebook", "understood", 4, 7, "2025-03-01"),
		ta("b", "notebook", "understood", 4, 7, "2025-11-01"),
		ta("c", "notebook", "understood", 4, 7, "2026-02-01"),
	}
	res := ComputeTrends(attempts, TrendsQuery{Granularity: GranularityYear})
	require.Len(t, res.Buckets, 2)
	assert.Equal(t, "2025-01-01", res.Buckets[0].Period.Format("2006-01-02"))
	assert.Equal(t, "2026-01-01", res.Buckets[1].Period.Format("2006-01-02"))
	assert.Equal(t, 2, res.Buckets[0].Series[0].Attempts)
	assert.Equal(t, 1, res.Buckets[1].Series[0].Attempts)
}

func TestComputeTrends_MonthBucketMergesMixedZones(t *testing.T) {
	// Two April attempts whose timestamps carry different zone offsets — a
	// date-only record stored as UTC and a timestamped one in +09:00 — must
	// land in ONE April bucket, not two.
	utc := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	jst := time.Date(2026, 4, 20, 9, 0, 0, 0, time.FixedZone("JST", 9*3600))
	attempts := []TrendAttempt{
		{Expression: "a", NotebookID: "nb", QuizType: "notebook", Status: "understood", Quality: 4, IntervalDays: 7, LearnedAt: utc},
		{Expression: "b", NotebookID: "nb", QuizType: "notebook", Status: "understood", Quality: 4, IntervalDays: 7, LearnedAt: jst},
	}
	res := ComputeTrends(attempts, TrendsQuery{Granularity: GranularityMonth})
	require.Len(t, res.Buckets, 1)
	assert.Equal(t, "2026-04-01", res.Buckets[0].Period.Format("2006-01-02"))
	assert.Equal(t, 2, res.Buckets[0].Series[0].Attempts)
	assert.Equal(t, 2, res.Buckets[0].Series[0].WordsTested)
}

func TestComputeTrends_WeekBucketing(t *testing.T) {
	// 2026-06-01 is a Monday; 2026-06-08 the next Monday.
	attempts := []TrendAttempt{
		ta("a", "notebook", "understood", 4, 7, "2026-06-03"), // week of Jun 1
		ta("b", "notebook", "understood", 4, 7, "2026-06-09"), // week of Jun 8
	}
	res := ComputeTrends(attempts, TrendsQuery{Granularity: GranularityWeek})
	require.Len(t, res.Buckets, 2)
	assert.Equal(t, "2026-06-01", res.Buckets[0].Period.Format("2006-01-02"))
	assert.Equal(t, "2026-06-08", res.Buckets[1].Period.Format("2006-01-02"))
}
