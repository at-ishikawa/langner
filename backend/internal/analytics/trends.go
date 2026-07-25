package analytics

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// Granularity is the width of one Trends bucket.
type Granularity int

const (
	GranularityDay Granularity = iota
	GranularityWeek
	GranularityMonth
	GranularityYear
)

// TrendGroupBy is the dimension each bucket's stack is split by.
type TrendGroupBy int

const (
	TrendGroupByNone TrendGroupBy = iota
	TrendGroupByQuizType
	TrendGroupByNotebook
	TrendGroupByStatus
	// TrendGroupByLevel groups each attempt by the spaced-repetition box it
	// landed in (the 1 / 7 / 30 / 90 / … day intervals). Pairing it with
	// the level-ups metric is how the UI shows level-ups per box.
	TrendGroupByLevel
)

// TrendsQuery bundles the arguments of Repository.Trends.
type TrendsQuery struct {
	Granularity Granularity
	// Start / End bound the emitted buckets (inclusive). Zero Start means
	// "from the first attempt"; zero End means "through today". Both are
	// day-granular — End includes the whole end day.
	Start   time.Time
	End     time.Time
	GroupBy TrendGroupBy
	Filters Filters
}

// TrendAttempt is one attempt fed into ComputeTrends. Both repositories
// map their rows (YAML records / DB learning_logs) to this shape so the
// aggregation logic lives in exactly one place — the read side of the L2
// symmetric-read invariant.
type TrendAttempt struct {
	// ID is the entry's stable source-entry identity (notebook.Note.ID).
	// Empty for legacy id-less entries, which fall back to keying by
	// Expression. Two ids sharing a spelling count as two distinct words.
	ID            string
	Expression    string
	NotebookID    string
	NotebookTitle string
	QuizType      string
	Status        string
	Quality       int
	IntervalDays  int
	LearnedAt     time.Time
}

// TrendSeries is one split of a bucket.
type TrendSeries struct {
	GroupKey     string
	GroupLabel   string
	Attempts     int
	WordsTested  int
	WordsLearned int
	LevelUps     int
	Lapses       int
}

// TrendBucket is one period (day / week / month) of activity.
type TrendBucket struct {
	Period time.Time
	Series []TrendSeries
}

// TrendsSummary is the range total for each metric. Distinct-count metrics
// (WordsTested, WordsLearned) de-duplicate over the whole range, so they do
// not equal the sum of the per-bucket values.
type TrendsSummary struct {
	Attempts     int
	WordsTested  int
	WordsLearned int
	LevelUps     int
	Lapses       int
}

// Backlog is the end-of-range state snapshot: where the user's words stand
// today, not what flowed over the period. Counts are distinct words.
type Backlog struct {
	NeverCorrect int
	InProgress   int
	Mastered     int
}

// TrendsResult bundles Repository.Trends output.
type TrendsResult struct {
	Buckets []TrendBucket
	Summary TrendsSummary
	Backlog Backlog
}

// Status values a learning record can carry. The understood/usable/
// intuitive constants are unexported in the notebook package, so the
// analytics read path names them here.
const (
	statusMisunderstood = "misunderstood"
	statusUnderstood    = "understood"
	statusUsable        = "usable"
	statusIntuitive     = "intuitive"
)

// isRetained reports whether a status counts as "learned" — the word has
// been recalled at least once, so it sits in the understood-or-better zone.
func isRetained(status string) bool {
	return status == statusUnderstood || status == statusUsable || status == statusIntuitive
}

// isMastered reports whether a status is at the top of the ladder.
func isMastered(status string) bool {
	return status == statusUsable || status == statusIntuitive
}

// isCorrect reports whether an attempt was answered correctly. Anything
// other than misunderstood (or the empty learning status) is correct,
// matching Attempt.IsWrong elsewhere in the package.
func isCorrect(status string) bool {
	return status != statusMisunderstood && status != ""
}

// levelBox returns the spaced-repetition box index for a stored interval:
// the highest box whose interval is still <= intervalDays. Mirrors
// FixedLevelCalculator.levelFromInterval so a level here means the same
// thing it means on the quiz side.
func levelBox(intervalDays int) int {
	box := 0
	for i, iv := range notebook.DefaultFixedIntervals {
		if iv <= intervalDays {
			box = i
		}
	}
	return box
}

func boxIntervalDays(box int) int {
	if box < 0 || box >= len(notebook.DefaultFixedIntervals) {
		return 0
	}
	return notebook.DefaultFixedIntervals[box]
}

// seriesKey identifies one word × mode log series. NotebookID is part of
// the key so two notebooks that happen to share an expression keep
// independent progressions; per the L1 invariant a word's logs live under
// one canonical notebook anyway.
type seriesKey struct {
	notebookID string
	// discriminator is the entry's stable id when present, else its
	// expression (see seriesDiscriminator) — so two same-spelling ids form
	// two series while legacy id-less entries key by spelling as before.
	discriminator string
	quizType      string
}

// flaggedAttempt carries an attempt with the per-attempt events derived
// from its position in the series: whether it crossed up into a retained
// status, and whether its box moved up or down versus the previous attempt.
type flaggedAttempt struct {
	attempt  TrendAttempt
	crossing bool
	levelUp  bool
	lapse    bool
}

// ComputeTrends aggregates a flat list of attempts into per-bucket series,
// range totals, and the end-of-range backlog. attempts must contain every
// attempt of every series in scope (already notebook/quiz filtered but NOT
// date filtered) — transition metrics need each series' history before the
// range start to know its prior state.
func ComputeTrends(attempts []TrendAttempt, q TrendsQuery) TrendsResult {
	series := map[seriesKey][]TrendAttempt{}
	for _, a := range attempts {
		k := seriesKey{a.NotebookID, seriesDiscriminator(a.ID, a.Expression), a.QuizType}
		series[k] = append(series[k], a)
	}

	endExcl := time.Time{}
	if !q.End.IsZero() {
		endExcl = time.Date(q.End.Year(), q.End.Month(), q.End.Day(), 0, 0, 0, 0, q.End.Location()).AddDate(0, 0, 1)
	}

	var flags []flaggedAttempt
	backlog := computeBacklog(series, endExcl, &flags, q)

	result := TrendsResult{Backlog: backlog}
	result.Buckets, result.Summary = aggregateBuckets(flags, q, endExcl)
	return result
}

// computeBacklog walks each series oldest-first to (a) fill the flags slice
// with per-attempt events and (b) determine each series' state as of the
// end of the range, then rolls series state up per word into the backlog.
func computeBacklog(series map[seriesKey][]TrendAttempt, endExcl time.Time, flags *[]flaggedAttempt, _ TrendsQuery) Backlog {
	// Per-word roll-up of series state. A word is mastered if any of its
	// series is mastered; never-correct if it has attempts but none correct.
	type wordState struct {
		hasAttempt  bool
		everCorrect bool
		mastered    bool
	}
	words := map[string]*wordState{}

	for _, list := range series {
		sort.Slice(list, func(i, j int) bool {
			if !list[i].LearnedAt.Equal(list[j].LearnedAt) {
				return list[i].LearnedAt.Before(list[j].LearnedAt)
			}
			return list[i].Quality < list[j].Quality
		})

		var lastStatus string
		var sawInRange bool
		for i, a := range list {
			retNow := isRetained(a.Status)
			retPrev := i > 0 && isRetained(list[i-1].Status)
			f := flaggedAttempt{attempt: a, crossing: retNow && !retPrev}
			if i > 0 {
				box := levelBox(a.IntervalDays)
				prevBox := levelBox(list[i-1].IntervalDays)
				f.levelUp = box > prevBox
				f.lapse = box < prevBox
			}
			*flags = append(*flags, f)

			if endExcl.IsZero() || a.LearnedAt.Before(endExcl) {
				sawInRange = true
				lastStatus = a.Status
				wid := seriesDiscriminator(a.ID, a.Expression)
				ws := words[wid]
				if ws == nil {
					ws = &wordState{}
					words[wid] = ws
				}
				ws.hasAttempt = true
				if isCorrect(a.Status) {
					ws.everCorrect = true
				}
			}
		}
		if sawInRange && isMastered(lastStatus) {
			// list's last in-range status maps to the series' latest word.
			last := list[len(list)-1]
			words[seriesDiscriminator(last.ID, last.Expression)].mastered = true
		}
	}

	var b Backlog
	for _, ws := range words {
		switch {
		case ws.mastered:
			b.Mastered++
		case !ws.everCorrect:
			b.NeverCorrect++
		default:
			b.InProgress++
		}
	}
	return b
}

// seriesAgg accumulates one bucket+group's metrics. tested/learned are sets
// of expressions so a word drilled repeatedly counts once per bucket.
type seriesAgg struct {
	label    string
	attempts int
	tested   map[string]struct{}
	learned  map[string]struct{}
	levelUps int
	lapses   int
}

func aggregateBuckets(flags []flaggedAttempt, q TrendsQuery, endExcl time.Time) ([]TrendBucket, TrendsSummary) {
	// Bucket by the canonical date string, not the time.Time value: two
	// attempts in the same period whose LearnedAt carries different zone
	// offsets (a date-only record stored as UTC vs. a timestamped record in
	// the server's local zone) would otherwise produce two distinct map
	// keys — and two bars — for one calendar period.
	byBucket := map[string]map[string]*seriesAgg{}
	keyLabel := map[string]string{}
	sumTested := map[string]struct{}{}
	sumLearned := map[string]struct{}{}
	var summary TrendsSummary

	for _, f := range flags {
		a := f.attempt
		if !q.Start.IsZero() && a.LearnedAt.Before(q.Start) {
			continue
		}
		if !endExcl.IsZero() && !a.LearnedAt.Before(endExcl) {
			continue
		}
		bucketKey := bucketStart(a.LearnedAt, q.Granularity).Format("2006-01-02")
		key, label := groupOf(a, q.GroupBy)
		keyLabel[key] = label

		groups := byBucket[bucketKey]
		if groups == nil {
			groups = map[string]*seriesAgg{}
			byBucket[bucketKey] = groups
		}
		g := groups[key]
		if g == nil {
			g = &seriesAgg{label: label, tested: map[string]struct{}{}, learned: map[string]struct{}{}}
			groups[key] = g
		}
		wid := seriesDiscriminator(a.ID, a.Expression)
		g.attempts++
		g.tested[wid] = struct{}{}
		summary.Attempts++
		sumTested[wid] = struct{}{}
		if f.crossing {
			g.learned[wid] = struct{}{}
			sumLearned[wid] = struct{}{}
		}
		if f.levelUp {
			g.levelUps++
			summary.LevelUps++
		}
		if f.lapse {
			g.lapses++
			summary.Lapses++
		}
	}
	summary.WordsTested = len(sumTested)
	summary.WordsLearned = len(sumLearned)

	order := orderedGroupKeys(q.GroupBy, keyLabel)
	buckets := make([]TrendBucket, 0, len(byBucket))
	for periodKey, groups := range byBucket {
		period, _ := time.Parse("2006-01-02", periodKey)
		tb := TrendBucket{Period: period}
		for _, key := range order {
			g, ok := groups[key]
			if !ok {
				continue
			}
			tb.Series = append(tb.Series, TrendSeries{
				GroupKey:     key,
				GroupLabel:   g.label,
				Attempts:     g.attempts,
				WordsTested:  len(g.tested),
				WordsLearned: len(g.learned),
				LevelUps:     g.levelUps,
				Lapses:       g.lapses,
			})
		}
		buckets = append(buckets, tb)
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Period.Before(buckets[j].Period) })
	return buckets, summary
}

// bucketStart truncates an instant to the start of its day / ISO-week
// (Monday) / month, in the instant's own zone. LearnedAt carries the zone
// it was written in, matching the day-bucket semantics used elsewhere.
func bucketStart(t time.Time, g Granularity) time.Time {
	y, m, d := t.Date()
	loc := t.Location()
	switch g {
	case GranularityWeek:
		offset := (int(t.Weekday()) + 6) % 7 // days since Monday
		return time.Date(y, m, d, 0, 0, 0, 0, loc).AddDate(0, 0, -offset)
	case GranularityMonth:
		return time.Date(y, m, 1, 0, 0, 0, 0, loc)
	case GranularityYear:
		return time.Date(y, 1, 1, 0, 0, 0, 0, loc)
	default:
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	}
}

func groupOf(a TrendAttempt, by TrendGroupBy) (key, label string) {
	switch by {
	case TrendGroupByQuizType:
		return a.QuizType, quizTypeLabel(a.QuizType)
	case TrendGroupByNotebook:
		label := a.NotebookTitle
		if label == "" {
			label = a.NotebookID
		}
		return a.NotebookID, label
	case TrendGroupByStatus:
		s := a.Status
		if s == "" {
			s = "learning"
		}
		return s, s
	case TrendGroupByLevel:
		iv := boxIntervalDays(levelBox(a.IntervalDays))
		return strconv.Itoa(iv), fmt.Sprintf("%d-day", iv)
	default:
		return "", ""
	}
}

// orderedGroupKeys returns the group keys in a stable, meaningful display
// order for the dimension: canonical quiz-type order, the status ladder,
// ascending box interval, or alphabetical notebook label.
func orderedGroupKeys(by TrendGroupBy, keyLabel map[string]string) []string {
	keys := make([]string, 0, len(keyLabel))
	for k := range keyLabel {
		keys = append(keys, k)
	}
	switch by {
	case TrendGroupByQuizType:
		sort.Slice(keys, func(i, j int) bool { return quizTypeRank(keys[i]) < quizTypeRank(keys[j]) })
	case TrendGroupByStatus:
		sort.Slice(keys, func(i, j int) bool { return statusRank(keys[i]) < statusRank(keys[j]) })
	case TrendGroupByLevel:
		sort.Slice(keys, func(i, j int) bool { return atoiSafe(keys[i]) < atoiSafe(keys[j]) })
	default:
		sort.Slice(keys, func(i, j int) bool { return keyLabel[keys[i]] < keyLabel[keys[j]] })
	}
	return keys
}

var quizTypeOrder = []string{
	string(notebook.QuizTypeNotebook),
	string(notebook.QuizTypeReverse),
	string(notebook.QuizTypeFreeform),
	string(notebook.QuizTypeEtymologyStandard),
	string(notebook.QuizTypeEtymologyReverse),
	string(notebook.QuizTypeEtymologyFreeform),
}

var quizTypeLabels = map[string]string{
	string(notebook.QuizTypeNotebook):          "Notebook",
	string(notebook.QuizTypeReverse):           "Reverse",
	string(notebook.QuizTypeFreeform):          "Freeform",
	string(notebook.QuizTypeEtymologyStandard): "Etymology breakdown",
	string(notebook.QuizTypeEtymologyReverse):  "Etymology assembly",
	string(notebook.QuizTypeEtymologyFreeform): "Etymology freeform",
}

func quizTypeLabel(q string) string {
	if l, ok := quizTypeLabels[q]; ok {
		return l
	}
	return q
}

func quizTypeRank(q string) int {
	for i, t := range quizTypeOrder {
		if t == q {
			return i
		}
	}
	return len(quizTypeOrder)
}

var statusOrder = []string{statusMisunderstood, statusUnderstood, statusUsable, statusIntuitive, "learning"}

func statusRank(s string) int {
	for i, t := range statusOrder {
		if t == s {
			return i
		}
	}
	return len(statusOrder)
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
