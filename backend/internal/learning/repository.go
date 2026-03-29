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
		return nil, fmt.Errorf("load all learning logs: %w", err)
	}
	return logs, nil
}

// Create inserts a single learning log.
// If NoteID is 0 and Expression is set, it will find or create the note on demand.
func (r *DBLearningRepository) Create(ctx context.Context, log *LearningLog) error {
	if log.NoteID == 0 && log.Expression != "" {
		noteID, err := r.ensureNoteExists(ctx, log)
		if err != nil {
			return fmt.Errorf("ensure note exists: %w", err)
		}
		log.NoteID = noteID
	}

	query := `INSERT INTO learning_logs (note_id, status, learned_at, quality, response_time_ms, quiz_type, interval_days, source_notebook_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query,
		log.NoteID, log.Status, log.LearnedAt, log.Quality, log.ResponseTimeMs, log.QuizType, log.IntervalDays, log.SourceNotebookID)
	if err != nil {
		return fmt.Errorf("insert learning log: %w", err)
	}
	return nil
}

// ensureNoteExists finds an existing note by usage/entry or creates one.
// Uses Definition as entry if set, otherwise Expression. Stores Expression as usage.
func (r *DBLearningRepository) ensureNoteExists(ctx context.Context, log *LearningLog) (int64, error) {
	entry := log.OriginalExpression
	if entry == "" {
		entry = log.Expression
	}
	usage := log.Expression

	// Try to find existing note
	var noteID int64
	err := r.db.GetContext(ctx, &noteID, "SELECT id FROM notes WHERE `usage` = ? AND entry = ?", usage, entry)
	if err == nil {
		return noteID, nil
	}

	// Create the note
	result, err := r.db.ExecContext(ctx,
		"INSERT INTO notes (`usage`, entry, meaning) VALUES (?, ?, ?)",
		usage, entry, "")
	if err != nil {
		return 0, fmt.Errorf("insert note: %w", err)
	}
	noteID, err = result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get note insert ID: %w", err)
	}
	return noteID, nil
}

// BatchCreate inserts multiple learning logs in a single transaction using multi-row INSERTs.
// Rows are chunked to stay under MySQL's 65535 placeholder limit.
func (r *DBLearningRepository) BatchCreate(ctx context.Context, logs []*LearningLog) error {
	if len(logs) == 0 {
		return nil
	}

	columns := []string{"note_id", "status", "learned_at", "quality", "response_time_ms", "quiz_type", "interval_days", "source_notebook_id"}
	const chunkSize = 5000 // 5000 * 8 columns = 40000 placeholders, well under 65535

	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		for i := 0; i < len(logs); i += chunkSize {
			end := i + chunkSize
			if end > len(logs) {
				end = len(logs)
			}
			chunk := logs[i:end]

			query := database.BuildMultiRowInsert("learning_logs", columns, len(chunk))
			var args []interface{}
			for _, l := range chunk {
				args = append(args, l.NoteID, l.Status, l.LearnedAt, l.Quality, l.ResponseTimeMs, l.QuizType, l.IntervalDays, l.SourceNotebookID)
			}
			if _, err := tx.ExecContext(ctx, query, args...); err != nil {
				return fmt.Errorf("insert learning logs: %w", err)
			}
		}
		return nil
	})
}
