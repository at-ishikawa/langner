package notebook

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// GrammarCorrectionRecord is a row of the grammar_corrections table — the
// stable identity (notebook_id, sense_id) of one grammar correction. The
// correction's reference content (its mistaken span, the fix, the note) stays
// in the read-only grammars notebooks; this table exists only to key the
// correction's learning-log series, mirroring EtymologyOriginRecord for
// origins.
type GrammarCorrectionRecord struct {
	ID         int64     `db:"id"`
	NotebookID string    `db:"notebook_id"`
	SenseID    string    `db:"sense_id"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

// GrammarCorrectionRepository is the storage interface for grammar corrections.
type GrammarCorrectionRepository interface {
	// FindAll returns every grammar_corrections row.
	FindAll(ctx context.Context) ([]GrammarCorrectionRecord, error)
	// FindOrCreate returns the row for (notebookID, senseID), inserting it
	// when absent. Idempotent, so re-seeding is a no-op.
	FindOrCreate(ctx context.Context, notebookID, senseID string) (GrammarCorrectionRecord, error)
}

// DBGrammarCorrectionRepository is a PostgreSQL-backed implementation.
type DBGrammarCorrectionRepository struct {
	db *sqlx.DB
}

// NewDBGrammarCorrectionRepository constructs the repository.
func NewDBGrammarCorrectionRepository(db *sqlx.DB) *DBGrammarCorrectionRepository {
	return &DBGrammarCorrectionRepository{db: db}
}

// FindAll returns every grammar correction row.
func (r *DBGrammarCorrectionRepository) FindAll(ctx context.Context) ([]GrammarCorrectionRecord, error) {
	var rows []GrammarCorrectionRecord
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT id, notebook_id, sense_id, created_at, updated_at FROM grammar_corrections`); err != nil {
		return nil, fmt.Errorf("select grammar_corrections: %w", err)
	}
	return rows, nil
}

// FindOrCreate upserts by the (notebook_id, sense_id) unique key and returns
// the row. The no-op SET (assigning a column to its own EXCLUDED value) is the
// standard Postgres idiom for "RETURNING the row whether it was inserted or
// already existed" — same pattern as ensureNoteExists.
func (r *DBGrammarCorrectionRepository) FindOrCreate(ctx context.Context, notebookID, senseID string) (GrammarCorrectionRecord, error) {
	var rec GrammarCorrectionRecord
	if err := r.db.GetContext(ctx, &rec, `
		INSERT INTO grammar_corrections (notebook_id, sense_id) VALUES ($1, $2)
		ON CONFLICT (notebook_id, sense_id) DO UPDATE SET sense_id = EXCLUDED.sense_id
		RETURNING id, notebook_id, sense_id, created_at, updated_at`,
		notebookID, senseID); err != nil {
		return GrammarCorrectionRecord{}, fmt.Errorf("upsert grammar_correction: %w", err)
	}
	return rec, nil
}
