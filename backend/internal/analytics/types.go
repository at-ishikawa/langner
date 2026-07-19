// Package analytics computes day-level and per-word views of the user's
// quiz history. The package contains pure aggregation logic and a
// repository interface; the concrete YAML and DB implementations live in
// sibling files.
package analytics

import "time"

// PatternCorrect / PatternWrong / PatternNone are the three string values
// used in the WrongWord.RecentPattern slice and AttemptEntry.Result.
const (
	PatternCorrect = "correct"
	PatternWrong   = "wrong"
	PatternNone    = "none"
)

// RecentPatternLength is how many of the most recent attempts (per quiz
// type) are shown as glyphs on the Day Detail page.
const RecentPatternLength = 5

// Attempt is a single past quiz attempt for one (note, quiz_type) pair,
// represented in a quiz-type-neutral way so streak logic doesn't care
// where the record came from.
type Attempt struct {
	LearnedAt time.Time
	QuizType  string
	// IsWrong is true when the attempt was marked misunderstood. Anything
	// else (understood, usable, intuitive) counts as correct.
	IsWrong bool
	// Quality is the SM-2 quality grade (0-5) attached to the attempt.
	Quality int
	// Status is the raw LearnedStatus string ("misunderstood", "understood", …).
	Status string
}

// Filters mirrors api.v1.AnalyticsFilters. Empty fields mean "no filter".
type Filters struct {
	NotebookID string
	QuizType   string
}

// DailySummary is one row on the Day List page.
type DailySummary struct {
	Date          time.Time
	WrongCount    int
	TotalCount    int
	NotebookCount int
	QuizTypes     []string
}

// WordRef identifies one (notebook, expression, quiz_type) record for the
// repository to look up. NoteID is used when available; the fallback path
// uses NotebookID + Expression instead.
type WordRef struct {
	NoteID     int64
	NotebookID string
	Expression string
	QuizType   string
	// PartOfSpeech is the sense discriminator for a homograph (e.g. "record"
	// the noun vs the verb). Empty means "any sense" — the legacy behavior
	// that commingles every same-spelling series, so a request that predates
	// the sense-aware frontend still resolves. With Expression it selects the
	// one series to return; see MatchesSense in the notebook package.
	PartOfSpeech string
}

// WrongWord is the per-card payload on the Day Detail page.
type WrongWord struct {
	NoteID     int64
	Expression string
	// PartOfSpeech is the sense of the expression (e.g. "noun" / "verb").
	// Empty for single-sense words and legacy (pre-migration) entries. Two
	// homograph senses produce two WrongWord cards; the frontend surfaces
	// this so "record" the noun and "record" the verb read as distinct.
	PartOfSpeech          string
	NotebookID            string
	NotebookTitle         string
	SceneTitle            string
	QuizType              string
	RecentPattern         []string
	CurrentWrongStreak    int
	PreviousCorrectStreak int
	CurrentStatus         string
	// LearnedAt is the timestamp of the wrong attempt on the requested day.
	// Used to sort cards reverse-chronologically — newest failure on top.
	LearnedAt time.Time
	// Meaning is the canonical definition of the expression. Empty when
	// the metadata resolver could not match the expression in any source
	// notebook (typical for legacy entries with stale notebook IDs).
	Meaning string
	// ExampleSentence is one illustrative usage. Empty for etymology
	// origins and for entries whose source notebook has no examples.
	ExampleSentence string
	// NotebookKind is "story", "flashcard", or "etymology". The frontend
	// uses this to pick the right Learn-page route for the deep link.
	NotebookKind string
	// Skipped is true when the expression's SkippedAt map has a non-empty
	// timestamp for this card's QuizType — i.e. the user has explicitly
	// excluded the word from this quiz mode. The analytics card renders an
	// "Excluded" badge so a wrong attempt that won't re-surface in future
	// quizzes is visibly distinguished from one that will.
	Skipped bool
	// RelatedGroups places the word in its concept graph: same-concept
	// siblings, sibling origins under the same etymology concept, and
	// concepts connected via etymology relations (antonym / synonym /
	// hyponym / …). Empty when the source notebook declares no concepts.
	RelatedGroups []RelatedGroup
}

// RelatedGroup is one cluster of related entries returned alongside a
// WrongWord. The kind names the cluster ("concept", "origin_family",
// "antonym", "synonym", "hyponym", …); the label is a human-readable
// header and the members are already-formatted display strings.
type RelatedGroup struct {
	Kind    string
	Label   string
	Members []string
}

// WordMetadata is what a MetadataResolver returns for a single
// (notebookID, expression, expressionType) lookup.
type WordMetadata struct {
	Meaning         string
	ExampleSentence string
	NotebookKind    string
	// RelatedGroups carries the concept-graph context for the word: the
	// definitions-book concept it shares with sibling expressions, the
	// etymology concept its origin belongs to (and sibling origins
	// under it), plus etymology relations from that concept (antonym /
	// synonym / hyponym / …). Empty when the notebook has no concepts.
	RelatedGroups []RelatedGroup
}

// expressionType is the LearningHistoryExpression.Type value carried into
// the resolver: "" or "vocabulary" for a vocab note, "origin" for an
// etymology origin. Defined here so callers don't have to import the
// notebook package just to construct lookups.
const (
	ExpressionTypeVocabulary = ""
	ExpressionTypeOrigin     = "origin"
)


// WordHistory is the payload returned by GetWordHistory.
type WordHistory struct {
	Expression string
	// PartOfSpeech echoes the resolved sense of the series (empty for
	// single-sense / legacy words), so the frontend header can disambiguate a
	// homograph's history from its other sense.
	PartOfSpeech       string
	NotebookID         string
	NotebookTitle      string
	CurrentStatus      string
	CurrentWrongStreak int
	Attempts           []AttemptEntry
}

// AttemptEntry is one row on the expanded word panel.
type AttemptEntry struct {
	Date                time.Time
	QuizType            string
	Result              string // PatternCorrect or PatternWrong
	Quality             int
	StreakBeforeWrong   int
	StreakBeforeCorrect int
}
