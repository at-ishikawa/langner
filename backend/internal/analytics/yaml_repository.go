package analytics

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// YAMLRepository reads quiz history from on-disk learning_notes YAML
// files. It walks every history once per call and caches nothing — the
// directory is small enough that a fresh read per request is fine, and
// it keeps the implementation simple.
//
// The optional resolver populates WrongWord.Meaning / ExampleSentence /
// NotebookKind from the source notebooks. Without it the YAML repo
// still works — meanings are simply blank in the response.
type YAMLRepository struct {
	directory string
	resolver  MetadataResolver
}

// NewYAMLRepository returns an analytics Repository backed by the YAML
// learning history files under directory.
func NewYAMLRepository(directory string) *YAMLRepository {
	return &YAMLRepository{directory: directory, resolver: NoMetadataResolver()}
}

// WithMetadataResolver returns a copy of the repository that consults
// the given resolver when building wrong-word cards. Use
// notebook.NewMetadataResolver(reader) in production wiring.
func (r *YAMLRepository) WithMetadataResolver(m MetadataResolver) *YAMLRepository {
	if m == nil {
		m = NoMetadataResolver()
	}
	cp := *r
	cp.resolver = m
	return &cp
}

// yamlAttempt carries one record from the YAML files with enough context
// to filter by notebook/quiz type and look up the parent expression for
// the per-day detail view.
type yamlAttempt struct {
	NotebookID     string
	NotebookTitle  string
	SceneTitle     string
	Expression     string
	ExpressionType string
	QuizType       string
	// Skipped is true when the expression's SkippedAt map has a non-empty
	// timestamp for this attempt's QuizType. Captured at load time so the
	// per-day detail builder doesn't need to re-walk the history files to
	// answer "is this card currently excluded".
	Skipped bool
	Attempt Attempt
}

// allAttempts loads every record from every history file, flattens them,
// and applies notebook/quiz-type filters. The result is unsorted.
func (r *YAMLRepository) allAttempts(filters Filters) ([]yamlAttempt, error) {
	histories, err := notebook.NewLearningHistories(r.directory)
	if err != nil {
		return nil, fmt.Errorf("load learning histories: %w", err)
	}
	var out []yamlAttempt
	for _, list := range histories {
		for _, h := range list {
			notebookID := h.Metadata.NotebookID
			if filters.NotebookID != "" && filters.NotebookID != notebookID {
				continue
			}
			collectExpressions(h, notebookID, h.Metadata.Title, filters.QuizType, &out)
		}
	}
	return out, nil
}

func collectExpressions(
	h notebook.LearningHistory,
	notebookID, notebookTitle, quizTypeFilter string,
	out *[]yamlAttempt,
) {
	// Flashcards have expressions at the top level; stories nest them under scenes.
	for _, exp := range h.Expressions {
		appendExpressionAttempts(exp, notebookID, notebookTitle, "", quizTypeFilter, out)
	}
	for _, scene := range h.Scenes {
		for _, exp := range scene.Expressions {
			appendExpressionAttempts(exp, notebookID, notebookTitle, scene.Metadata.Title, quizTypeFilter, out)
		}
	}
}

func appendExpressionAttempts(
	exp notebook.LearningHistoryExpression,
	notebookID, notebookTitle, sceneTitle, quizTypeFilter string,
	out *[]yamlAttempt,
) {
	tracks := map[string][]notebook.LearningRecord{
		string(notebook.QuizTypeNotebook):           exp.LearnedLogs,
		string(notebook.QuizTypeReverse):            exp.ReverseLogs,
		string(notebook.QuizTypeEtymologyStandard):  exp.EtymologyBreakdownLogs,
		string(notebook.QuizTypeEtymologyReverse):   exp.EtymologyAssemblyLogs,
	}
	for quizType, records := range tracks {
		if quizTypeFilter != "" && quizTypeFilter != quizType {
			continue
		}
		skipped := exp.SkippedAt.IsSkipped(notebook.QuizType(quizType))
		for _, rec := range records {
			if rec.LearnedAt.IsZero() {
				continue
			}
			*out = append(*out, yamlAttempt{
				NotebookID:     notebookID,
				NotebookTitle:  notebookTitle,
				SceneTitle:     sceneTitle,
				Expression:     exp.Expression,
				ExpressionType: exp.Type,
				QuizType:       quizType,
				Skipped:        skipped,
				Attempt: Attempt{
					LearnedAt: rec.LearnedAt.Time,
					QuizType:  quizType,
					IsWrong:   rec.Status == notebook.LearnedStatusMisunderstood,
					Quality:   rec.Quality,
					Status:    string(rec.Status),
				},
			})
		}
	}
}

// DailySummaries aggregates per-day rollups. rangeDays == 0 means
// "all time"; otherwise records older than now-rangeDays are dropped.
func (r *YAMLRepository) DailySummaries(_ context.Context, rangeDays int, filters Filters) ([]DailySummary, error) {
	attempts, err := r.allAttempts(filters)
	if err != nil {
		return nil, err
	}
	cutoff := rangeCutoff(rangeDays)

	type bucket struct {
		total     int
		wrong     int
		notebooks map[string]struct{}
		quizTypes map[string]struct{}
	}
	buckets := map[string]*bucket{}
	for _, a := range attempts {
		if !cutoff.IsZero() && a.Attempt.LearnedAt.Before(cutoff) {
			continue
		}
		key := dayKey(a.Attempt.LearnedAt)
		b, ok := buckets[key]
		if !ok {
			b = &bucket{
				notebooks: map[string]struct{}{},
				quizTypes: map[string]struct{}{},
			}
			buckets[key] = b
		}
		b.total++
		if a.Attempt.IsWrong {
			b.wrong++
		}
		b.notebooks[a.NotebookID] = struct{}{}
		b.quizTypes[a.QuizType] = struct{}{}
	}

	out := make([]DailySummary, 0, len(buckets))
	for key, b := range buckets {
		date, _ := time.Parse("2006-01-02", key)
		out = append(out, DailySummary{
			Date:          date,
			WrongCount:    b.wrong,
			TotalCount:    b.total,
			NotebookCount: len(b.notebooks),
			QuizTypes:     sortedKeys(b.quizTypes),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out, nil
}

// DayDetail returns the wrong words on day plus prev/next day pointers.
func (r *YAMLRepository) DayDetail(ctx context.Context, day time.Time, filters Filters) (DayDetail, error) {
	attempts, err := r.allAttempts(filters)
	if err != nil {
		return DayDetail{}, err
	}

	dayStr := dayKey(day)
	// Index attempts by (notebookID, expression, quizType) so we can find each
	// word's full log to compute streak/pattern.
	type wordKey struct {
		notebookID string
		expression string
		quizType   string
	}
	byWord := map[wordKey][]yamlAttempt{}
	dayHas := map[string]bool{} // YYYY-MM-DD -> seen activity
	summary := DailySummary{Date: truncToDay(day)}
	notebooks := map[string]struct{}{}
	quizTypes := map[string]struct{}{}

	for _, a := range attempts {
		key := dayKey(a.Attempt.LearnedAt)
		dayHas[key] = true
		byWord[wordKey{a.NotebookID, a.Expression, a.QuizType}] = append(
			byWord[wordKey{a.NotebookID, a.Expression, a.QuizType}],
			a,
		)
		if key == dayStr {
			summary.TotalCount++
			if a.Attempt.IsWrong {
				summary.WrongCount++
			}
			notebooks[a.NotebookID] = struct{}{}
			quizTypes[a.QuizType] = struct{}{}
		}
	}
	summary.NotebookCount = len(notebooks)
	summary.QuizTypes = sortedKeys(quizTypes)

	// Collect wrong words for the day.
	var wrong []WrongWord
	for _, records := range byWord {
		// records ordered as encountered; sort by LearnedAt desc to keep
		// newest-first invariant required by streak helpers.
		sort.Slice(records, func(i, j int) bool {
			return records[i].Attempt.LearnedAt.After(records[j].Attempt.LearnedAt)
		})
		// Find the attempt on `day` (latest one on that day).
		var hit *yamlAttempt
		var hitIdx int
		for i := range records {
			if dayKey(records[i].Attempt.LearnedAt) == dayStr && records[i].Attempt.IsWrong {
				hit = &records[i]
				hitIdx = i
				break
			}
		}
		if hit == nil {
			continue
		}
		// Compute streak from this attempt's perspective: treat the matched
		// attempt as the newest. Records older than the day-of-attempt are
		// the "before" run.
		fromHit := make([]Attempt, 0, len(records)-hitIdx)
		for i := hitIdx; i < len(records); i++ {
			fromHit = append(fromHit, records[i].Attempt)
		}
		meta := r.resolver.Resolve(ctx, hit.NotebookID, hit.Expression, hit.ExpressionType)
		wrong = append(wrong, WrongWord{
			Expression:            hit.Expression,
			NotebookID:            hit.NotebookID,
			NotebookTitle:         hit.NotebookTitle,
			SceneTitle:            hit.SceneTitle,
			QuizType:              hit.QuizType,
			RecentPattern:         RecentPattern(fromHit),
			CurrentWrongStreak:    CurrentWrongStreak(fromHit),
			PreviousCorrectStreak: PreviousCorrectStreak(fromHit),
			CurrentStatus:         hit.Attempt.Status,
			LearnedAt:             hit.Attempt.LearnedAt,
			Meaning:               meta.Meaning,
			ExampleSentence:       meta.ExampleSentence,
			NotebookKind:          meta.NotebookKind,
			Skipped:               hit.Skipped,
		})
	}
	// Newest failure first. Ties (rare — same word + quiz type wrong twice on
	// the same exact instant) fall back to expression for stability.
	sort.Slice(wrong, func(i, j int) bool {
		if !wrong[i].LearnedAt.Equal(wrong[j].LearnedAt) {
			return wrong[i].LearnedAt.After(wrong[j].LearnedAt)
		}
		return wrong[i].Expression < wrong[j].Expression
	})

	// Find prev/next dates with activity (regardless of correct/wrong) matching filters.
	prev, next := adjacentDates(dayHas, day)

	return DayDetail{
		Summary:      summary,
		WrongWords:   wrong,
		PreviousDate: prev,
		NextDate:     next,
	}, nil
}

// WordHistory returns every attempt for a single (notebook, expression, quiz_type) triple.
func (r *YAMLRepository) WordHistory(_ context.Context, ref WordRef) (WordHistory, error) {
	attempts, err := r.allAttempts(Filters{NotebookID: ref.NotebookID, QuizType: ref.QuizType})
	if err != nil {
		return WordHistory{}, err
	}
	var matched []yamlAttempt
	var notebookTitle string
	for _, a := range attempts {
		if a.Expression != ref.Expression {
			continue
		}
		matched = append(matched, a)
		notebookTitle = a.NotebookTitle
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Attempt.LearnedAt.After(matched[j].Attempt.LearnedAt)
	})
	flat := make([]Attempt, len(matched))
	for i, m := range matched {
		flat[i] = m.Attempt
	}
	entries := make([]AttemptEntry, len(matched))
	for i, m := range matched {
		streakWrong, streakCorrect := StreakBeforeAttempt(flat, i)
		result := PatternCorrect
		if m.Attempt.IsWrong {
			result = PatternWrong
		}
		entries[i] = AttemptEntry{
			Date:                m.Attempt.LearnedAt,
			QuizType:            m.QuizType,
			Result:              result,
			Quality:             m.Attempt.Quality,
			StreakBeforeWrong:   streakWrong,
			StreakBeforeCorrect: streakCorrect,
		}
	}

	currentStatus := ""
	if len(matched) > 0 {
		currentStatus = matched[0].Attempt.Status
	}
	return WordHistory{
		Expression:         ref.Expression,
		NotebookID:         ref.NotebookID,
		NotebookTitle:      notebookTitle,
		CurrentStatus:      currentStatus,
		CurrentWrongStreak: CurrentWrongStreak(flat),
		Attempts:           entries,
	}, nil
}

func rangeCutoff(rangeDays int) time.Time {
	if rangeDays <= 0 {
		return time.Time{}
	}
	return time.Now().UTC().AddDate(0, 0, -rangeDays)
}

// dayKey returns the YYYY-MM-DD bucket for an instant, using the time's
// stored zone. LearningRecord.LearnedAt is written via time.Now() so the
// record carries the server's local zone — the user thinks of an
// evening answer as "today" in their zone, not whatever UTC was when
// they hit submit. The frontend renders the URL date with the same
// local-zone semantics; see frontend/src/app/quiz/complete/page.tsx for
// the matching "today" computation on the Quiz Complete deep link.
func dayKey(t time.Time) string {
	return t.Format("2006-01-02")
}

func truncToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func adjacentDates(have map[string]bool, target time.Time) (prev, next time.Time) {
	targetKey := dayKey(target)
	keys := make([]string, 0, len(have))
	for k := range have {
		if k == targetKey {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys) // ascending
	for _, k := range keys {
		if k < targetKey {
			d, _ := time.Parse("2006-01-02", k)
			if prev.IsZero() || d.After(prev) {
				prev = d
			}
		} else {
			d, _ := time.Parse("2006-01-02", k)
			if next.IsZero() || d.Before(next) {
				next = d
			}
		}
	}
	return prev, next
}
