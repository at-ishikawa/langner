// Package learning provides learning log domain models and repository interfaces.
package learning

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

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

// LearningRepository defines operations for managing learning logs.
type LearningRepository interface {
	FindAll(ctx context.Context) ([]LearningLog, error)
	FindByNote(ctx context.Context, noteID int64, quizType string) ([]LearningLog, error)
	FindLatestByNote(ctx context.Context, noteID int64, quizType string) (*LearningLog, error)
	FindByNoteQuizTypeAndLearnedAt(ctx context.Context, noteID int64, quizType string, learnedAt time.Time) (*LearningLog, error)
	Create(ctx context.Context, log *LearningLog) error
}

// DBLearningRepository implements LearningRepository using MySQL.
type DBLearningRepository struct {
	db *sqlx.DB
}

// NewDBLearningRepository creates a new DBLearningRepository.
func NewDBLearningRepository(db *sqlx.DB) *DBLearningRepository {
	return &DBLearningRepository{db: db}
}

// FindAll returns all learning logs.
func (r *DBLearningRepository) FindAll(ctx context.Context) ([]LearningLog, error) {
	var logs []LearningLog
	if err := r.db.SelectContext(ctx, &logs, "SELECT * FROM learning_logs ORDER BY id"); err != nil {
		return nil, fmt.Errorf("db.SelectContext(learning_logs) > %w", err)
	}
	return logs, nil
}

// FindByNote returns all learning logs for a note and quiz type.
func (r *DBLearningRepository) FindByNote(ctx context.Context, noteID int64, quizType string) ([]LearningLog, error) {
	var logs []LearningLog
	if err := r.db.SelectContext(ctx, &logs,
		"SELECT * FROM learning_logs WHERE note_id = ? AND quiz_type = ? ORDER BY learned_at",
		noteID, quizType); err != nil {
		return nil, fmt.Errorf("db.SelectContext(learning_logs by note) > %w", err)
	}
	return logs, nil
}

// FindLatestByNote returns the most recent learning log for a note and quiz type, or nil if not found.
func (r *DBLearningRepository) FindLatestByNote(ctx context.Context, noteID int64, quizType string) (*LearningLog, error) {
	var log LearningLog
	err := r.db.GetContext(ctx, &log,
		"SELECT * FROM learning_logs WHERE note_id = ? AND quiz_type = ? ORDER BY learned_at DESC LIMIT 1",
		noteID, quizType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db.GetContext(latest learning_log) > %w", err)
	}
	return &log, nil
}

// FindByNoteQuizTypeAndLearnedAt returns a learning log matching the given criteria, or nil if not found.
func (r *DBLearningRepository) FindByNoteQuizTypeAndLearnedAt(ctx context.Context, noteID int64, quizType string, learnedAt time.Time) (*LearningLog, error) {
	var log LearningLog
	err := r.db.GetContext(ctx, &log,
		"SELECT * FROM learning_logs WHERE note_id = ? AND quiz_type = ? AND learned_at = ?",
		noteID, quizType, learnedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db.GetContext(learning_log) > %w", err)
	}
	return &log, nil
}

// Create inserts a new learning log.
func (r *DBLearningRepository) Create(ctx context.Context, log *LearningLog) error {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO learning_logs (note_id, status, learned_at, quality, response_time_ms, quiz_type, interval_days, easiness_factor)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		log.NoteID, log.Status, log.LearnedAt, log.Quality, log.ResponseTimeMs,
		log.QuizType, log.IntervalDays, log.EasinessFactor)
	if err != nil {
		return fmt.Errorf("db.ExecContext(insert learning_log) > %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("result.LastInsertId() > %w", err)
	}
	log.ID = id
	return nil
}
