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
}

// WrongWord is the per-card payload on the Day Detail page.
type WrongWord struct {
	NoteID                int64
	Expression            string
	NotebookID            string
	NotebookTitle         string
	SceneTitle            string
	QuizType              string
	RecentPattern         []string
	CurrentWrongStreak    int
	PreviousCorrectStreak int
	CurrentStatus         string
}

// WordHistory is the payload returned by GetWordHistory.
type WordHistory struct {
	Expression         string
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
