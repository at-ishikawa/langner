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
	// BatchDelete removes rows whose IDs appear in ids. Used by the
	// reconcile pass to drop DB-only logs that no longer exist in YAML.
	BatchDelete(ctx context.Context, ids []int64) error
}

// DBLearningRepository implements LearningRepository using MySQL.
type DBLearningRepository struct {
	db *sqlx.DB
}

// NewDBLearningRepository creates a new DBLearningRepository.
func NewDBLearningRepository(db *sqlx.DB) *DBLearningRepository {
	return &DBLearningRepository{db: db}
}

// FindAll returns all learning logs. note_id and origin_id are nullable
// since migration 016 — COALESCE both to zero so the int64 fields scan
// cleanly. Callers distinguish vocab vs etymology rows by which one is
// non-zero.
const selectLearningLogColumns = `SELECT id, COALESCE(note_id, 0) AS note_id, COALESCE(origin_id, 0) AS origin_id, status, learned_at, quality, response_time_ms, quiz_type, interval_days, concept_key, easiness_factor, source_notebook_id, created_at, updated_at FROM learning_logs`

func (r *DBLearningRepository) FindAll(ctx context.Context) ([]LearningLog, error) {
	var logs []LearningLog
	if err := r.db.SelectContext(ctx, &logs, selectLearningLogColumns+" ORDER BY id"); err != nil {
		return nil, fmt.Errorf("load all learning logs: %w", err)
	}
	return logs, nil
}

// Create inserts a single learning log. Vocab logs need a NoteID (it
// will be found-or-created from Expression if 0); etymology logs need
// an OriginID. Exactly one of the two must end up non-zero; NULLIF
// converts the zero side to SQL NULL for the nullable column.
func (r *DBLearningRepository) Create(ctx context.Context, log *LearningLog) error {
	if log.OriginID == 0 && log.NoteID == 0 && log.Expression != "" {
		noteID, err := r.ensureNoteExists(ctx, log)
		if err != nil {
			return fmt.Errorf("ensure note exists: %w", err)
		}
		log.NoteID = noteID
	}
	if log.NoteID == 0 && log.OriginID == 0 {
		return fmt.Errorf("learning log requires NoteID or OriginID")
	}

	query := `INSERT INTO learning_logs (note_id, origin_id, status, learned_at, quality, response_time_ms, quiz_type, interval_days, source_notebook_id, concept_key)
		VALUES (NULLIF(?, 0), NULLIF(?, 0), ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query,
		log.NoteID, log.OriginID, log.Status, log.LearnedAt, log.Quality, log.ResponseTimeMs, log.QuizType, log.IntervalDays, log.SourceNotebookID, log.ConceptKey)
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

	columns := []string{"note_id", "origin_id", "status", "learned_at", "quality", "response_time_ms", "quiz_type", "interval_days", "source_notebook_id", "concept_key"}
	// chunkSize bounds the multi-row INSERT payload so TLS round-trips
	// against TiDB Cloud don't trip `bad record MAC` mid-stream the way
	// 5000-row INSERTs did during sync-db. 500 rows × 10 columns =
	// 5000 placeholders, comfortably under MySQL's 65535 placeholder
	// cap and under the wire-frame size that triggered the failure.
	const chunkSize = 500

	// Multi-row INSERT can't use NULLIF in VALUES — overwrite zero IDs
	// with explicit *int64(nil) so sqlx passes SQL NULL. Vocab logs keep
	// NoteID set; etymology logs keep OriginID set.
	nullableID := func(id int64) interface{} {
		if id == 0 {
			return nil
		}
		return id
	}

	// Each chunk commits in its own implicit transaction (db.ExecContext
	// rather than tx.ExecContext) so a multi-thousand-row import doesn't
	// hold one pooled connection open for the entire run. Long-lived
	// connections inside one transaction surfaced as
	// `tls: bad record MAC` against TiDB Cloud — the connection went
	// stale partway through and the in-flight transaction died with no
	// recovery path. Per-chunk commits let the pool rotate connections,
	// at the cost of partial-commit visibility if the import fails mid-
	// way. Sync-db's drop-and-reimport flow tolerates that (re-running
	// drops everything anyway); incremental import-db users who care
	// about atomicity can re-run after a failure.
	//
	// Per-chunk retry handles TiDB Cloud's occasional mid-statement
	// connection death. ExecContext on autocommit means a failed INSERT
	// rolled back atomically — no rows landed — so re-running the same
	// chunk on a fresh pool connection is safe. We retry only on
	// transient connection-level errors (TLS framing, `invalid
	// connection`, driver.ErrBadConn); persistent errors like
	// duplicate-key get returned immediately so the caller sees the
	// real cause.
	for i := 0; i < len(logs); i += chunkSize {
		end := i + chunkSize
		if end > len(logs) {
			end = len(logs)
		}
		chunk := logs[i:end]

		query := database.BuildMultiRowInsert("learning_logs", columns, len(chunk))
		var args []interface{}
		for _, l := range chunk {
			args = append(args, nullableID(l.NoteID), nullableID(l.OriginID), l.Status, l.LearnedAt, l.Quality, l.ResponseTimeMs, l.QuizType, l.IntervalDays, l.SourceNotebookID, l.ConceptKey)
		}
		if err := database.ExecWithRetry(ctx, r.db, query, args...); err != nil {
			return fmt.Errorf("insert learning logs (rows %d..%d): %w", i, end, err)
		}
	}
	return nil
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
			if _, err := tx.ExecContext(ctx, query, args...); err != nil {
				return fmt.Errorf("delete learning logs: %w", err)
			}
		}
		return nil
	})
}
