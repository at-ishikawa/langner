package learning

import "time"

// LearningLog represents a learning history entry. Exactly one of
// NoteID and OriginID is non-zero: vocabulary quizzes target a note
// (NoteID set, OriginID 0); etymology quizzes target an origin
// (OriginID set, NoteID 0). Migration 016 made note_id nullable and
// added origin_id so both quiz kinds share one logs table; the
// repository converts zero values to SQL NULL on write.
type LearningLog struct {
	ID             int64     `db:"id"`
	NoteID         int64     `db:"note_id"`
	OriginID       int64     `db:"origin_id"`
	Status         string    `db:"status"`
	LearnedAt      time.Time `db:"learned_at"`
	Quality        int       `db:"quality"`
	ResponseTimeMs int       `db:"response_time_ms"`
	QuizType       string    `db:"quiz_type"`
	IntervalDays     int       `db:"interval_days"`
	// ConceptKey is the head expression of the definitions concept this
	// log belongs to (denormalised cache of notes.concept_key). Set at
	// log-write time so "all logs for a concept" is a single index
	// lookup, with no join required.
	ConceptKey       string    `db:"concept_key"`
	EasinessFactor   *float64  `db:"easiness_factor"` // kept for DB compatibility; derived from logs at runtime
	SourceNotebookID string    `db:"source_notebook_id"`
	CreatedAt        time.Time `db:"created_at"`
	UpdatedAt        time.Time `db:"updated_at"`

	NotebookName       string `db:"-"`
	StoryTitle         string `db:"-"`
	SceneTitle         string `db:"-"`
	Expression         string `db:"-"`
	OriginalExpression string `db:"-"`
	IsCorrect          bool   `db:"-"`
	LearningNotesDir   string `db:"-"`
}
