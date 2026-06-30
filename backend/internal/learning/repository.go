// Package learning provides learning log storage and retrieval.
package learning

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/database"
)

// UpdateLogInput identifies a single learning log entry and the
// override to apply. Different implementations use different subsets:
// the YAML repository looks up by (NotebookName + StoryTitle/SceneTitle
// + Expression/OriginalExpression + QuizType + LearnedAt); the DB
// repository looks up by (NoteID + QuizType + LearnedAt). Pass
// everything you have — unused fields are ignored.
//
// MarkCorrect carries the user's intent for a fresh override. When
// MirrorValues is non-nil the repo skips its own
// markCorrect→status/quality derivation and writes exactly what's
// inside — used by MultiLearningRepository so the secondary store
// applies the same bytes the primary just wrote.
type UpdateLogInput struct {
	NoteID             int64
	NotebookName       string
	StoryTitle         string
	SceneTitle         string
	Expression         string
	OriginalExpression string
	QuizType           string
	LearnedAt          time.Time
	MarkCorrect        *bool
	MirrorValues       *UpdateLogMirror
}

// UpdateLogMirror carries the already-computed new values when a
// secondary store should byte-match a primary's override.
type UpdateLogMirror struct {
	Status       string
	Quality      int
	IntervalDays int
}

// UpdateLogResult reports the pre-change values and the recomputed
// next-review date. Found is false when no row/entry matched the
// lookup keys; callers must treat that as a no-op (matches the
// rows-affected=0 semantics of SQL UPDATE).
type UpdateLogResult struct {
	OriginalQuality      int
	OriginalStatus       string
	OriginalIntervalDays int
	NewQuality           int
	NewStatus            string
	NewIntervalDays      int
	NewNextReviewDate    string
	Found                bool
}

// LearningRepository defines operations for managing learning logs.
type LearningRepository interface {
	FindAll(ctx context.Context) ([]LearningLog, error)
	BatchCreate(ctx context.Context, logs []*LearningLog) error
	Create(ctx context.Context, log *LearningLog) error
	// UpdateLog rewrites the log identified by in's lookup keys
	// according to in.MarkCorrect. Used by OverrideAnswer. The DB
	// implementation can fall back to mirroring values computed
	// upstream when in.NewStatus/NewQuality are pre-filled (used by
	// MultiLearningRepository so YAML and DB stay byte-identical).
	UpdateLog(ctx context.Context, in UpdateLogInput) (UpdateLogResult, error)
	// BatchDelete removes rows whose IDs appear in ids. Used by the
	// reconcile pass to drop DB-only logs that no longer exist in YAML.
	BatchDelete(ctx context.Context, ids []int64) error
}

// DBLearningRepository implements LearningRepository using PostgreSQL.
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

	query := `INSERT INTO learning_logs (note_id, status, learned_at, quality, response_time_ms, quiz_type, interval_days, source_notebook_id, concept_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := r.db.ExecContext(ctx, query,
		log.NoteID, log.Status, log.LearnedAt, log.Quality, log.ResponseTimeMs, log.QuizType, log.IntervalDays, log.SourceNotebookID, log.ConceptKey)
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
	err := r.db.GetContext(ctx, &noteID, `SELECT id FROM notes WHERE "usage" = $1 AND entry = $2`, usage, entry)
	if err == nil {
		return noteID, nil
	}

	// Create the note
	if err := r.db.GetContext(ctx, &noteID,
		`INSERT INTO notes ("usage", entry, meaning) VALUES ($1, $2, $3) RETURNING id`,
		usage, entry, ""); err != nil {
		return 0, fmt.Errorf("insert note: %w", err)
	}
	return noteID, nil
}

// BatchCreate inserts multiple learning logs in a single transaction using multi-row INSERTs.
// Rows are chunked to stay under PostgreSQL's 65535 parameter limit.
func (r *DBLearningRepository) BatchCreate(ctx context.Context, logs []*LearningLog) error {
	if len(logs) == 0 {
		return nil
	}

	columns := []string{"note_id", "status", "learned_at", "quality", "response_time_ms", "quiz_type", "interval_days", "source_notebook_id", "concept_key"}
	const chunkSize = 5000 // 5000 * 9 columns = 45000 placeholders, well under 65535

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
				args = append(args, l.NoteID, l.Status, l.LearnedAt, l.Quality, l.ResponseTimeMs, l.QuizType, l.IntervalDays, l.SourceNotebookID, l.ConceptKey)
			}
			if _, err := tx.ExecContext(ctx, query, args...); err != nil {
				return fmt.Errorf("insert learning logs: %w", err)
			}
		}
		return nil
	})
}

// UpdateLog rewrites a single learning_logs row identified by
// (note_id, quiz_type, learned_at) according to in.MarkCorrect (or
// in.MirrorValues when set). Returns the pre-update values plus the
// recomputed next-review date so the handler can hand them back to the
// frontend's Undo workflow.
//
// Lookup matches by exact learned_at when LearnedAt is non-zero; the
// frontend always sends the YYYY-MM-DD or RFC3339 string of the
// specific log the user clicked, so a same-day match is sufficient. If
// no row matches, the call returns Found=false (no SQL no-op error).
func (r *DBLearningRepository) UpdateLog(ctx context.Context, in UpdateLogInput) (UpdateLogResult, error) {
	if in.NoteID == 0 || in.QuizType == "" || in.LearnedAt.IsZero() {
		return UpdateLogResult{}, nil
	}

	// Find the row + read originals. DATE() lets the frontend pass a
	// YYYY-MM-DD without losing matches to clock skew between the
	// answer write and the override click.
	type currentRow struct {
		ID           int64     `db:"id"`
		Status       string    `db:"status"`
		Quality      int       `db:"quality"`
		IntervalDays int       `db:"interval_days"`
		LearnedAt    time.Time `db:"learned_at"`
	}
	var cur currentRow
	err := r.db.GetContext(ctx, &cur, `
		SELECT id, status, quality, interval_days, learned_at
		FROM learning_logs
		WHERE note_id = $1 AND quiz_type = $2 AND DATE(learned_at) = $3::date
		ORDER BY learned_at DESC LIMIT 1`,
		in.NoteID, in.QuizType, in.LearnedAt.Format("2006-01-02"))
	if err != nil {
		// No row matches — treat as soft no-op so callers in
		// MultiLearningRepository don't fail the whole override when
		// the YAML side succeeded but the DB doesn't have the log yet
		// (e.g. fresh import that hasn't replayed the latest quiz).
		return UpdateLogResult{}, nil
	}

	newStatus, newQuality, newIntervalDays := computeOverrideValues(in, cur.Status, cur.Quality, cur.IntervalDays)

	if _, err := r.db.ExecContext(ctx, `
		UPDATE learning_logs
		SET status = $1, quality = $2, interval_days = $3
		WHERE id = $4`,
		newStatus, newQuality, newIntervalDays, cur.ID); err != nil {
		return UpdateLogResult{}, fmt.Errorf("update learning_logs: %w", err)
	}

	return UpdateLogResult{
		OriginalQuality:      cur.Quality,
		OriginalStatus:       cur.Status,
		OriginalIntervalDays: cur.IntervalDays,
		NewQuality:           newQuality,
		NewStatus:            newStatus,
		NewIntervalDays:      newIntervalDays,
		NewNextReviewDate:    cur.LearnedAt.AddDate(0, 0, newIntervalDays).Format("2006-01-02"),
		Found:                true,
	}, nil
}

// computeOverrideValues derives the post-override status/quality/
// interval. If the caller provided MirrorValues (the YAML primary's
// already-computed values, in the Multi flow), those win — keeping
// YAML and DB byte-identical. Otherwise we apply the markCorrect
// shorthand directly: quality 1/misunderstood or 4/understood with
// interval reset to 1 on misunderstood, otherwise carry the prior
// interval. The DB doesn't have the calculator's full SM-2 chain at
// hand; for the rare DB-only deployment that's a small drift the
// next quiz answer will replay through the YAML calculator anyway.
func computeOverrideValues(in UpdateLogInput, curStatus string, curQuality, curInterval int) (status string, quality, intervalDays int) {
	if in.MirrorValues != nil {
		return in.MirrorValues.Status, in.MirrorValues.Quality, in.MirrorValues.IntervalDays
	}
	if in.MarkCorrect == nil {
		return curStatus, curQuality, curInterval
	}
	if *in.MarkCorrect {
		newStatus := "understood"
		if in.QuizType == "freeform" || in.QuizType == "etymology_freeform" {
			newStatus = "usable"
		}
		return newStatus, 4, max(curInterval, 1)
	}
	return "misunderstood", 1, 1
}

// BatchDelete removes the rows whose IDs are in the slice. Used by the
// importer's reconcile pass to drop DB-only logs whose YAML counterpart
// has disappeared.
func (r *DBLearningRepository) BatchDelete(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	const chunkSize = 5000
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		for i := 0; i < len(ids); i += chunkSize {
			end := i + chunkSize
			if end > len(ids) {
				end = len(ids)
			}
			chunk := ids[i:end]
			query, args, err := sqlx.In("DELETE FROM learning_logs WHERE id IN (?)", chunk)
			if err != nil {
				return fmt.Errorf("build delete query: %w", err)
			}
			if _, err := tx.ExecContext(ctx, tx.Rebind(query), args...); err != nil {
				return fmt.Errorf("delete learning logs: %w", err)
			}
		}
		return nil
	})
}
