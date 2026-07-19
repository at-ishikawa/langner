package analytics

import (
	"context"
	"fmt"
	"sort"
	"strings"
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
	// PartOfSpeech is the learning entry's sense discriminator (normalized:
	// trimmed + lowercased). It is part of the per-word key so two same-
	// spelling senses (e.g. "record" noun vs verb) count and chart as
	// separate series. Empty for single-sense / legacy entries.
	PartOfSpeech string
	QuizType     string
	// Skipped is true when the expression's SkippedAt map has a non-empty
	// timestamp for this attempt's QuizType. Captured at load time so the
	// per-day detail builder doesn't need to re-walk the history files to
	// answer "is this card currently excluded".
	Skipped bool
	// IntervalDays is the record's stored spaced-repetition interval, used
	// by the Trends level-box aggregation.
	IntervalDays int
	Attempt      Attempt
}

// allAttempts loads every record from every history file, flattens them,
// and applies notebook/quiz-type filters. The result is unsorted.
func (r *YAMLRepository) allAttempts(filters Filters) ([]yamlAttempt, error) {
	histories, err := notebook.NewLearningHistories(r.directory)
	if err != nil {
		return nil, fmt.Errorf("load learning histories: %w", err)
	}
	var out []yamlAttempt
	// The map key is the learning-history filename (one file per notebook),
	// which is the canonical notebook identity — the same value the DB path
	// stores as source_notebook_id. Metadata.Title/id are per-episode
	// (e.g. "Friends S01E01"), so grouping the analytics "notebook"
	// dimension on them would split one notebook into many chapters.
	for notebookName, list := range histories {
		if filters.NotebookID != "" && filters.NotebookID != notebookName {
			continue
		}
		for _, h := range list {
			collectExpressions(h, notebookName, filters.QuizType, &out)
		}
	}
	return out, nil
}

func collectExpressions(
	h notebook.LearningHistory,
	notebookName, quizTypeFilter string,
	out *[]yamlAttempt,
) {
	// Flashcards have expressions at the top level; stories nest them under
	// scenes. The episode title (h.Metadata.Title) becomes the scene
	// breadcrumb for top-level expressions so the per-day card keeps its
	// context while the notebook dimension stays at the file level.
	for _, exp := range h.Expressions {
		appendExpressionAttempts(exp, notebookName, h.Metadata.Title, quizTypeFilter, out)
	}
	for _, scene := range h.Scenes {
		sceneTitle := scene.Metadata.Title
		if h.Metadata.Title != "" && sceneTitle != "" {
			sceneTitle = h.Metadata.Title + " / " + sceneTitle
		} else if sceneTitle == "" {
			sceneTitle = h.Metadata.Title
		}
		for _, exp := range scene.Expressions {
			appendExpressionAttempts(exp, notebookName, sceneTitle, quizTypeFilter, out)
		}
	}
}

func appendExpressionAttempts(
	exp notebook.LearningHistoryExpression,
	notebookName, sceneTitle, quizTypeFilter string,
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
				NotebookID:     notebookName,
				NotebookTitle:  notebookName,
				SceneTitle:     sceneTitle,
				Expression:     exp.Expression,
				ExpressionType: exp.Type,
				PartOfSpeech:   exp.PartOfSpeech,
				QuizType:       quizType,
				Skipped:        skipped,
				IntervalDays:   rec.IntervalDays,
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
	// Index attempts by (notebookID, expression, partOfSpeech, quizType) so we
	// can find each word's full log to compute streak/pattern. The sense is
	// part of the key so two homograph senses form two independent series and
	// surface as two cards, each with its own streak/pattern and meaning.
	type wordKey struct {
		notebookID   string
		expression   string
		partOfSpeech string
		quizType     string
	}
	keyOf := func(a yamlAttempt) wordKey {
		return wordKey{a.NotebookID, a.Expression, normalizeSense(a.PartOfSpeech), a.QuizType}
	}
	byWord := map[wordKey][]yamlAttempt{}
	dayHas := map[string]bool{} // YYYY-MM-DD -> seen activity
	summary := DailySummary{Date: truncToDay(day)}
	notebooks := map[string]struct{}{}
	quizTypes := map[string]struct{}{}

	for _, a := range attempts {
		key := dayKey(a.Attempt.LearnedAt)
		dayHas[key] = true
		wk := keyOf(a)
		byWord[wk] = append(byWord[wk], a)
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
		meta := r.resolver.Resolve(ctx, hit.NotebookID, hit.Expression, hit.ExpressionType, hit.PartOfSpeech, hit.QuizType)
		wrong = append(wrong, WrongWord{
			Expression:            hit.Expression,
			PartOfSpeech:          hit.PartOfSpeech,
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
			RelatedGroups:         meta.RelatedGroups,
		})
	}
	// Newest failure first. Ties (rare — same word + quiz type wrong twice on
	// the same exact instant) fall back to expression for stability.
	sort.Slice(wrong, func(i, j int) bool {
		if !wrong[i].LearnedAt.Equal(wrong[j].LearnedAt) {
			return wrong[i].LearnedAt.After(wrong[j].LearnedAt)
		}
		if wrong[i].Expression != wrong[j].Expression {
			return wrong[i].Expression < wrong[j].Expression
		}
		return wrong[i].PartOfSpeech < wrong[j].PartOfSpeech
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
	var resolvedSense string
	for _, a := range attempts {
		if a.Expression != ref.Expression {
			continue
		}
		if !senseMatchesRef(a.PartOfSpeech, ref.PartOfSpeech) {
			continue
		}
		matched = append(matched, a)
		notebookTitle = a.NotebookTitle
		if resolvedSense == "" {
			resolvedSense = a.PartOfSpeech
		}
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
		PartOfSpeech:       resolvedSense,
		NotebookID:         ref.NotebookID,
		NotebookTitle:      notebookTitle,
		CurrentStatus:      currentStatus,
		CurrentWrongStreak: CurrentWrongStreak(flat),
		Attempts:           entries,
	}, nil
}

// Trends loads every attempt matching the notebook/quiz filters and
// delegates to ComputeTrends. Attempts are intentionally NOT date-filtered
// here: the aggregation needs each series' full history to know its state
// before the range start.
func (r *YAMLRepository) Trends(_ context.Context, q TrendsQuery) (TrendsResult, error) {
	attempts, err := r.allAttempts(q.Filters)
	if err != nil {
		return TrendsResult{}, err
	}
	trendAttempts := make([]TrendAttempt, 0, len(attempts))
	for _, a := range attempts {
		trendAttempts = append(trendAttempts, TrendAttempt{
			Expression:    a.Expression,
			PartOfSpeech:  a.PartOfSpeech,
			NotebookID:    a.NotebookID,
			NotebookTitle: a.NotebookTitle,
			QuizType:      a.QuizType,
			Status:        a.Attempt.Status,
			Quality:       a.Attempt.Quality,
			IntervalDays:  a.IntervalDays,
			LearnedAt:     a.Attempt.LearnedAt,
		})
	}
	return ComputeTrends(trendAttempts, q), nil
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

// normalizeSense is the canonical sense token used across the analytics
// package's per-word keys: trimmed + lowercased, matching the notebook
// package's normalizePartOfSpeech so a series keys identically on both the
// write and the analytics read side (invariant L2).
func normalizeSense(pos string) string {
	return strings.ToLower(strings.TrimSpace(pos))
}

// senseMatchesRef reports whether a series entry's sense satisfies a
// WordHistory request's requested sense. An empty requested sense matches
// any series (the legacy, pre-sense-aware frontend behavior — every
// same-spelling series is returned commingled). A legacy entry with no
// sense also satisfies a specific request, so a homograph view never drops
// pre-migration history. Two distinct tagged senses stay separate.
func senseMatchesRef(entrySense, refSense string) bool {
	want := normalizeSense(refSense)
	if want == "" {
		return true
	}
	got := normalizeSense(entrySense)
	return got == "" || got == want
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
