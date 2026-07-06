package learning

import "time"

// LearningLog represents a learning history entry for a note.
type LearningLog struct {
	ID             int64     `db:"id"`
	NoteID         int64     `db:"note_id"`
	// OriginID targets an etymology_origins row instead of a note.
	// Exactly one of NoteID / OriginID is non-zero: vocab logs set
	// NoteID, etymology-origin logs set OriginID. Migration 017 made
	// note_id nullable and added origin_id so both quiz kinds share
	// one logs table.
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
