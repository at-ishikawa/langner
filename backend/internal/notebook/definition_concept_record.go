package notebook

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/database"
)

// DefinitionConceptRecord mirrors a row of the definition_concepts table.
// The (NotebookID, Head) pair is unique. Meaning is the umbrella sense
// shared by every member expression; it comes from the first declaration
// in the book (later declarations of the same head merge their member
// lists but do not overwrite meaning).
type DefinitionConceptRecord struct {
	ID         int64     `db:"id"`
	NotebookID string    `db:"notebook_id"`
	Head       string    `db:"head"`
	Meaning    string    `db:"meaning"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

// DefinitionConceptMemberRecord mirrors a row of definition_concept_members.
// (ConceptID, Expression) is unique. SessionTitle records which session
// contributed the membership — concepts can be declared across multiple
// sessions of the same book.
type DefinitionConceptMemberRecord struct {
	ID           int64     `db:"id"`
	ConceptID    int64     `db:"concept_id"`
	Expression   string    `db:"expression"`
	SessionTitle string    `db:"session_title"`
	CreatedAt    time.Time `db:"created_at"`
}

// DefinitionConceptRepository is the storage interface for definition
// concepts and their members. The ingestion layer reconciles both: rows
// the YAML still claims are inserted or updated, rows no longer present
// are deleted.
type DefinitionConceptRepository interface {
	ListDefinitionConceptsByNotebook(ctx context.Context, notebookID string) ([]DefinitionConceptRecord, error)
	ListAllDefinitionConcepts(ctx context.Context) ([]DefinitionConceptRecord, error)
	ListDefinitionConceptMembersByConceptIDs(ctx context.Context, ids []int64) ([]DefinitionConceptMemberRecord, error)
	BatchCreateConcepts(ctx context.Context, records []*DefinitionConceptRecord) error
	BatchUpdateConcepts(ctx context.Context, records []*DefinitionConceptRecord) error
	BatchDeleteConcepts(ctx context.Context, ids []int64) error
	BatchCreateMembers(ctx context.Context, records []*DefinitionConceptMemberRecord) error
	BatchDeleteMembers(ctx context.Context, ids []int64) error
}

// DBDefinitionConceptRepository is a PostgreSQL-backed implementation.
type DBDefinitionConceptRepository struct {
	db *sqlx.DB
}

// NewDBDefinitionConceptRepository constructs the repository.
func NewDBDefinitionConceptRepository(db *sqlx.DB) *DBDefinitionConceptRepository {
	return &DBDefinitionConceptRepository{db: db}
}

// ListDefinitionConceptsByNotebook returns every concept row for a notebook.
func (r *DBDefinitionConceptRepository) ListDefinitionConceptsByNotebook(ctx context.Context, notebookID string) ([]DefinitionConceptRecord, error) {
	var rows []DefinitionConceptRecord
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT id, notebook_id, head, meaning, created_at, updated_at
		 FROM definition_concepts WHERE notebook_id = $1`, notebookID,
	); err != nil {
		return nil, fmt.Errorf("select definition_concepts: %w", err)
	}
	return rows, nil
}

// ListAllDefinitionConcepts returns every concept row across all books.
// Used by the exporter to round-trip concepts back to YAML.
func (r *DBDefinitionConceptRepository) ListAllDefinitionConcepts(ctx context.Context) ([]DefinitionConceptRecord, error) {
	var rows []DefinitionConceptRecord
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT id, notebook_id, head, meaning, created_at, updated_at FROM definition_concepts`,
	); err != nil {
		return nil, fmt.Errorf("select all definition_concepts: %w", err)
	}
	return rows, nil
}

// ListDefinitionConceptMembersByConceptIDs returns members for the given
// concept IDs. Returns an empty slice when ids is empty.
func (r *DBDefinitionConceptRepository) ListDefinitionConceptMembersByConceptIDs(ctx context.Context, ids []int64) ([]DefinitionConceptMemberRecord, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query, args, err := sqlx.In(
		`SELECT id, concept_id, expression, session_title, created_at
		 FROM definition_concept_members WHERE concept_id IN (?)`, ids,
	)
	if err != nil {
		return nil, fmt.Errorf("build select definition_concept_members: %w", err)
	}
	var rows []DefinitionConceptMemberRecord
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("select definition_concept_members: %w", err)
	}
	return rows, nil
}

// BatchCreateConcepts inserts new concept rows and writes back the
// auto-generated IDs by re-reading the unique (notebook_id, head) tuple.
func (r *DBDefinitionConceptRepository) BatchCreateConcepts(ctx context.Context, records []*DefinitionConceptRecord) error {
	if len(records) == 0 {
		return nil
	}
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		query := database.BuildMultiRowInsert("definition_concepts",
			[]string{"notebook_id", "head", "meaning"},
			len(records))
		args := make([]any, 0, len(records)*3)
		for _, rec := range records {
			args = append(args, rec.NotebookID, rec.Head, rec.Meaning)
		}
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("insert definition_concepts: %w", err)
		}
		var inserted []DefinitionConceptRecord
		if err := tx.SelectContext(ctx, &inserted,
			`SELECT id, notebook_id, head FROM definition_concepts`,
		); err != nil {
			return fmt.Errorf("reload definition_concepts after insert: %w", err)
		}
		idByKey := make(map[string]int64, len(inserted))
		for _, row := range inserted {
			idByKey[definitionConceptKey(row.NotebookID, row.Head)] = row.ID
		}
		for _, rec := range records {
			rec.ID = idByKey[definitionConceptKey(rec.NotebookID, rec.Head)]
		}
		return nil
	})
}

// BatchUpdateConcepts overwrites meaning for existing concept rows. The
// (notebook_id, head) tuple is treated as immutable.
func (r *DBDefinitionConceptRepository) BatchUpdateConcepts(ctx context.Context, records []*DefinitionConceptRecord) error {
	if len(records) == 0 {
		return nil
	}
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		for _, rec := range records {
			if _, err := tx.ExecContext(ctx,
				`UPDATE definition_concepts SET meaning = $1 WHERE id = $2`,
				rec.Meaning, rec.ID,
			); err != nil {
				return fmt.Errorf("update definition_concept %d: %w", rec.ID, err)
			}
		}
		return nil
	})
}

// BatchDeleteConcepts removes concept rows by ID. ON DELETE CASCADE drops
// dependent definition_concept_members rows.
func (r *DBDefinitionConceptRepository) BatchDeleteConcepts(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	query, args, err := sqlx.In(`DELETE FROM definition_concepts WHERE id IN (?)`, ids)
	if err != nil {
		return fmt.Errorf("build delete definition_concepts: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("delete definition_concepts: %w", err)
	}
	return nil
}

// BatchCreateMembers inserts member rows. Existing pairs are skipped via
// the unique (concept_id, expression) constraint — callers pre-filter.
func (r *DBDefinitionConceptRepository) BatchCreateMembers(ctx context.Context, records []*DefinitionConceptMemberRecord) error {
	if len(records) == 0 {
		return nil
	}
	query := database.BuildMultiRowInsert("definition_concept_members",
		[]string{"concept_id", "expression", "session_title"},
		len(records))
	args := make([]any, 0, len(records)*3)
	for _, rec := range records {
		args = append(args, rec.ConceptID, rec.Expression, rec.SessionTitle)
	}
	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("insert definition_concept_members: %w", err)
	}
	return nil
}

// BatchDeleteMembers removes member rows by ID.
func (r *DBDefinitionConceptRepository) BatchDeleteMembers(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	query, args, err := sqlx.In(`DELETE FROM definition_concept_members WHERE id IN (?)`, ids)
	if err != nil {
		return fmt.Errorf("build delete definition_concept_members: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("delete definition_concept_members: %w", err)
	}
	return nil
}

func definitionConceptKey(notebookID, head string) string {
	return notebookID + "\x00" + head
}
