package notebook

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// NoteSkipFlagRecord mirrors a row of note_skip_flags. Replaces the
// SkippedAtMap that used to live on LearningHistoryExpression in YAML.
type NoteSkipFlagRecord struct {
	ID        int64     `db:"id"`
	NoteID    int64     `db:"note_id"`
	QuizType  string    `db:"quiz_type"`
	SkippedAt time.Time `db:"skipped_at"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// OriginSkipFlagRecord mirrors a row of origin_skip_flags.
type OriginSkipFlagRecord struct {
	ID        int64     `db:"id"`
	OriginID  int64     `db:"origin_id"`
	QuizType  string    `db:"quiz_type"`
	SkippedAt time.Time `db:"skipped_at"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// SkipFlagRepository owns per-quiz-type skip state for both vocabulary
// notes and etymology origins. The YAML SkippedAtMap is gone; everything
// flows through these tables.
type SkipFlagRepository interface {
	// FindNoteFlags returns every skip flag for the given note IDs.
	FindNoteFlags(ctx context.Context, noteIDs []int64) ([]NoteSkipFlagRecord, error)
	// FindOriginFlags returns every skip flag for the given origin IDs.
	FindOriginFlags(ctx context.Context, originIDs []int64) ([]OriginSkipFlagRecord, error)
	// SkipNote inserts or updates the skip flag for (note_id, quiz_type).
	SkipNote(ctx context.Context, noteID int64, quizType string, at time.Time) error
	// ResumeNote removes the skip flag for (note_id, quiz_type). No-op
	// when the row doesn't exist.
	ResumeNote(ctx context.Context, noteID int64, quizType string) error
	// SkipOrigin / ResumeOrigin are the etymology equivalents.
	SkipOrigin(ctx context.Context, originID int64, quizType string, at time.Time) error
	ResumeOrigin(ctx context.Context, originID int64, quizType string) error
}

// DBSkipFlagRepository is the PostgreSQL-backed implementation.
type DBSkipFlagRepository struct {
	db *sqlx.DB
}

// NewDBSkipFlagRepository constructs the repository.
func NewDBSkipFlagRepository(db *sqlx.DB) *DBSkipFlagRepository {
	return &DBSkipFlagRepository{db: db}
}

// FindNoteFlags returns the skip rows for the given note IDs.
func (r *DBSkipFlagRepository) FindNoteFlags(ctx context.Context, noteIDs []int64) ([]NoteSkipFlagRecord, error) {
	if len(noteIDs) == 0 {
		return nil, nil
	}
	query, args, err := sqlx.In(
		`SELECT id, note_id, quiz_type, skipped_at, created_at, updated_at
		 FROM note_skip_flags WHERE note_id IN (?)`,
		noteIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("build note skip flags query: %w", err)
	}
	var rows []NoteSkipFlagRecord
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("select note skip flags: %w", err)
	}
	return rows, nil
}

// FindOriginFlags returns the skip rows for the given origin IDs.
func (r *DBSkipFlagRepository) FindOriginFlags(ctx context.Context, originIDs []int64) ([]OriginSkipFlagRecord, error) {
	if len(originIDs) == 0 {
		return nil, nil
	}
	query, args, err := sqlx.In(
		`SELECT id, origin_id, quiz_type, skipped_at, created_at, updated_at
		 FROM origin_skip_flags WHERE origin_id IN (?)`,
		originIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("build origin skip flags query: %w", err)
	}
	var rows []OriginSkipFlagRecord
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("select origin skip flags: %w", err)
	}
	return rows, nil
}

// SkipNote upserts the flag for (note_id, quiz_type).
func (r *DBSkipFlagRepository) SkipNote(ctx context.Context, noteID int64, quizType string, at time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO note_skip_flags (note_id, quiz_type, skipped_at) VALUES ($1, $2, $3)
		 ON CONFLICT (note_id, quiz_type) DO UPDATE SET skipped_at = EXCLUDED.skipped_at`,
		noteID, quizType, at,
	); err != nil {
		return fmt.Errorf("upsert note skip flag: %w", err)
	}
	return nil
}

// ResumeNote drops the flag for (note_id, quiz_type).
func (r *DBSkipFlagRepository) ResumeNote(ctx context.Context, noteID int64, quizType string) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM note_skip_flags WHERE note_id = $1 AND quiz_type = $2`,
		noteID, quizType,
	); err != nil {
		return fmt.Errorf("delete note skip flag: %w", err)
	}
	return nil
}

// SkipOrigin upserts the flag for (origin_id, quiz_type).
func (r *DBSkipFlagRepository) SkipOrigin(ctx context.Context, originID int64, quizType string, at time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO origin_skip_flags (origin_id, quiz_type, skipped_at) VALUES ($1, $2, $3)
		 ON CONFLICT (origin_id, quiz_type) DO UPDATE SET skipped_at = EXCLUDED.skipped_at`,
		originID, quizType, at,
	); err != nil {
		return fmt.Errorf("upsert origin skip flag: %w", err)
	}
	return nil
}

// ResumeOrigin drops the flag for (origin_id, quiz_type).
func (r *DBSkipFlagRepository) ResumeOrigin(ctx context.Context, originID int64, quizType string) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM origin_skip_flags WHERE origin_id = $1 AND quiz_type = $2`,
		originID, quizType,
	); err != nil {
		return fmt.Errorf("delete origin skip flag: %w", err)
	}
	return nil
}
