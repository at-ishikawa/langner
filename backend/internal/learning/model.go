package learning

import "time"

// LearningLog represents a learning history entry for a note.
type LearningLog struct {
	ID             int64     `db:"id"`
	NoteID         int64     `db:"note_id"`
	Status         string    `db:"status"`
	LearnedAt      time.Time `db:"learned_at"`
	Quality        int       `db:"quality"`
	ResponseTimeMs int       `db:"response_time_ms"`
	QuizType       string    `db:"quiz_type"`
	IntervalDays   int       `db:"interval_days"`
	EasinessFactor float64   `db:"easiness_factor"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}
