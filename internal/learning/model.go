package learning

import "time"

// LearningLog represents a learning history entry for a note.
type LearningLog struct {
	ID             int64     `db:"id" yaml:"id"`
	NoteID         int64     `db:"note_id" yaml:"note_id"`
	Status         string    `db:"status" yaml:"status"`
	LearnedAt      time.Time `db:"learned_at" yaml:"learned_at"`
	Quality        int       `db:"quality" yaml:"quality"`
	ResponseTimeMs int       `db:"response_time_ms" yaml:"response_time_ms"`
	QuizType       string    `db:"quiz_type" yaml:"quiz_type"`
	IntervalDays   int       `db:"interval_days" yaml:"interval_days"`
	EasinessFactor float64   `db:"easiness_factor" yaml:"easiness_factor"`
	CreatedAt      time.Time `db:"created_at" yaml:"created_at"`
	UpdatedAt      time.Time `db:"updated_at" yaml:"updated_at"`
}
