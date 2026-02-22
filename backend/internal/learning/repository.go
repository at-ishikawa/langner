// Package learning provides learning log storage and retrieval.
package learning

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/database"
)

// LearningRepository defines operations for managing learning logs.
type LearningRepository interface {
	FindAll(ctx context.Context) ([]LearningLog, error)
	BatchCreate(ctx context.Context, logs []*LearningLog) error
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
		return nil, fmt.Errorf("load all learning logs: %w", err)
	}
	return logs, nil
}

// BatchCreate inserts multiple learning logs in a single transaction using a multi-row INSERT.
func (r *DBLearningRepository) BatchCreate(ctx context.Context, logs []*LearningLog) error {
	if len(logs) == 0 {
		return nil
	}

	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		columns := []string{"note_id", "status", "learned_at", "quality", "response_time_ms", "quiz_type", "interval_days", "easiness_factor"}
		query := database.BuildMultiRowInsert("learning_logs", columns, len(logs))

		var args []interface{}
		for _, l := range logs {
			args = append(args, l.NoteID, l.Status, l.LearnedAt, l.Quality, l.ResponseTimeMs, l.QuizType, l.IntervalDays, l.EasinessFactor)
		}
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("insert learning logs: %w", err)
		}
		return nil
	})
}
