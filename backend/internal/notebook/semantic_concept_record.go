package notebook

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/database"
)

// SemanticConceptRecord mirrors a row of the semantic_concepts table. The
// (NotebookID, ConceptKey) pair is unique. Meaning/Note come from the first
// declaration in the book.
type SemanticConceptRecord struct {
	ID         int64     `db:"id"`
	NotebookID string    `db:"notebook_id"`
	ConceptKey string    `db:"concept_key"`
	Meaning    string    `db:"meaning"`
	Note       string    `db:"note"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

// SemanticConceptMemberRecord mirrors a row of semantic_concept_members.
// (ConceptID, OriginID) is unique. SessionTitle records which session
// declared the membership.
type SemanticConceptMemberRecord struct {
	ID           int64     `db:"id"`
	ConceptID    int64     `db:"concept_id"`
	OriginID     int64     `db:"origin_id"`
	SessionTitle string    `db:"session_title"`
	CreatedAt    time.Time `db:"created_at"`
}

// ConceptRelationRecord mirrors a row of concept_relations. Symmetric
// relations live as two rows (A->B, B->A) with IsDirected=false; directed
// relations live as a single row with IsDirected=true.
type ConceptRelationRecord struct {
	ID            int64     `db:"id"`
	NotebookID    string    `db:"notebook_id"`
	Type          string    `db:"type"`
	FromConceptID int64     `db:"from_concept_id"`
	ToConceptID   int64     `db:"to_concept_id"`
	IsDirected    bool      `db:"is_directed"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

// SemanticConceptRepository is the storage interface for semantic concepts
// and their members. Ingestion reconciles both: rows the YAML still claims
// are inserted or updated, rows no longer present are deleted.
type SemanticConceptRepository interface {
	ListSemanticConceptsByNotebook(ctx context.Context, notebookID string) ([]SemanticConceptRecord, error)
	ListSemanticConceptMembersByConceptIDs(ctx context.Context, ids []int64) ([]SemanticConceptMemberRecord, error)
	BatchCreateConcepts(ctx context.Context, records []*SemanticConceptRecord) error
	BatchUpdateConcepts(ctx context.Context, records []*SemanticConceptRecord) error
	BatchDeleteConcepts(ctx context.Context, ids []int64) error
	BatchCreateMembers(ctx context.Context, records []*SemanticConceptMemberRecord) error
	BatchDeleteMembers(ctx context.Context, ids []int64) error
}

// ConceptRelationRepository is the storage interface for concept_relations.
type ConceptRelationRepository interface {
	ListConceptRelationsByNotebook(ctx context.Context, notebookID string) ([]ConceptRelationRecord, error)
	BatchCreateRelations(ctx context.Context, records []*ConceptRelationRecord) error
	BatchDeleteRelations(ctx context.Context, ids []int64) error
}

// DBSemanticConceptRepository is a PostgreSQL-backed implementation.
type DBSemanticConceptRepository struct {
	db *sqlx.DB
}

// NewDBSemanticConceptRepository constructs the repository.
func NewDBSemanticConceptRepository(db *sqlx.DB) *DBSemanticConceptRepository {
	return &DBSemanticConceptRepository{db: db}
}

// ListSemanticConceptsByNotebook returns every concept row for a notebook.
func (r *DBSemanticConceptRepository) ListSemanticConceptsByNotebook(ctx context.Context, notebookID string) ([]SemanticConceptRecord, error) {
	var rows []SemanticConceptRecord
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT id, notebook_id, concept_key, meaning, note, created_at, updated_at
		 FROM semantic_concepts WHERE notebook_id = $1`, notebookID,
	); err != nil {
		return nil, fmt.Errorf("select semantic_concepts: %w", err)
	}
	return rows, nil
}

// ListSemanticConceptMembersByConceptIDs returns members for a set of
// concept IDs. Returns an empty slice when ids is empty.
func (r *DBSemanticConceptRepository) ListSemanticConceptMembersByConceptIDs(ctx context.Context, ids []int64) ([]SemanticConceptMemberRecord, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query, args, err := sqlx.In(
		`SELECT id, concept_id, origin_id, session_title, created_at
		 FROM semantic_concept_members WHERE concept_id IN (?)`, ids,
	)
	if err != nil {
		return nil, fmt.Errorf("build select semantic_concept_members: %w", err)
	}
	var rows []SemanticConceptMemberRecord
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("select semantic_concept_members: %w", err)
	}
	return rows, nil
}

// BatchCreateConcepts inserts new concept rows and writes back the
// auto-generated IDs by re-reading the unique (notebook_id, concept_key) tuple.
func (r *DBSemanticConceptRepository) BatchCreateConcepts(ctx context.Context, records []*SemanticConceptRecord) error {
	if len(records) == 0 {
		return nil
	}
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		query := database.BuildMultiRowInsert("semantic_concepts",
			[]string{"notebook_id", "concept_key", "meaning", "note"},
			len(records))
		args := make([]any, 0, len(records)*4)
		for _, rec := range records {
			args = append(args, rec.NotebookID, rec.ConceptKey, rec.Meaning, rec.Note)
		}
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("insert semantic_concepts: %w", err)
		}
		var inserted []SemanticConceptRecord
		if err := tx.SelectContext(ctx, &inserted,
			`SELECT id, notebook_id, concept_key FROM semantic_concepts`,
		); err != nil {
			return fmt.Errorf("reload semantic_concepts after insert: %w", err)
		}
		idByKey := make(map[string]int64, len(inserted))
		for _, row := range inserted {
			idByKey[semanticConceptKey(row.NotebookID, row.ConceptKey)] = row.ID
		}
		for _, rec := range records {
			rec.ID = idByKey[semanticConceptKey(rec.NotebookID, rec.ConceptKey)]
		}
		return nil
	})
}

// BatchUpdateConcepts overwrites meaning/note for existing concept rows.
// The (notebook_id, concept_key) tuple is treated as immutable.
func (r *DBSemanticConceptRepository) BatchUpdateConcepts(ctx context.Context, records []*SemanticConceptRecord) error {
	if len(records) == 0 {
		return nil
	}
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		for _, rec := range records {
			if _, err := tx.ExecContext(ctx,
				`UPDATE semantic_concepts SET meaning = $1, note = $2 WHERE id = $3`,
				rec.Meaning, rec.Note, rec.ID,
			); err != nil {
				return fmt.Errorf("update semantic_concept %d: %w", rec.ID, err)
			}
		}
		return nil
	})
}

// BatchDeleteConcepts removes concept rows by ID. ON DELETE CASCADE drops
// dependent semantic_concept_members and concept_relations rows.
func (r *DBSemanticConceptRepository) BatchDeleteConcepts(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	query, args, err := sqlx.In(`DELETE FROM semantic_concepts WHERE id IN (?)`, ids)
	if err != nil {
		return fmt.Errorf("build delete semantic_concepts: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("delete semantic_concepts: %w", err)
	}
	return nil
}

// BatchCreateMembers inserts member rows. Existing pairs are skipped via the
// unique (concept_id, origin_id) constraint — callers pre-filter.
func (r *DBSemanticConceptRepository) BatchCreateMembers(ctx context.Context, records []*SemanticConceptMemberRecord) error {
	if len(records) == 0 {
		return nil
	}
	query := database.BuildMultiRowInsert("semantic_concept_members",
		[]string{"concept_id", "origin_id", "session_title"},
		len(records))
	args := make([]any, 0, len(records)*3)
	for _, rec := range records {
		args = append(args, rec.ConceptID, rec.OriginID, rec.SessionTitle)
	}
	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("insert semantic_concept_members: %w", err)
	}
	return nil
}

// BatchDeleteMembers removes member rows by ID.
func (r *DBSemanticConceptRepository) BatchDeleteMembers(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	query, args, err := sqlx.In(`DELETE FROM semantic_concept_members WHERE id IN (?)`, ids)
	if err != nil {
		return fmt.Errorf("build delete semantic_concept_members: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("delete semantic_concept_members: %w", err)
	}
	return nil
}

func semanticConceptKey(notebookID, conceptKey string) string {
	return notebookID + "\x00" + conceptKey
}

// DBConceptRelationRepository is a PostgreSQL-backed implementation.
type DBConceptRelationRepository struct {
	db *sqlx.DB
}

// NewDBConceptRelationRepository constructs the repository.
func NewDBConceptRelationRepository(db *sqlx.DB) *DBConceptRelationRepository {
	return &DBConceptRelationRepository{db: db}
}

// ListConceptRelationsByNotebook returns every relation row for a notebook.
func (r *DBConceptRelationRepository) ListConceptRelationsByNotebook(ctx context.Context, notebookID string) ([]ConceptRelationRecord, error) {
	var rows []ConceptRelationRecord
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT id, notebook_id, type, from_concept_id, to_concept_id, is_directed, created_at, updated_at
		 FROM concept_relations WHERE notebook_id = $1`, notebookID,
	); err != nil {
		return nil, fmt.Errorf("select concept_relations: %w", err)
	}
	return rows, nil
}

// BatchCreateRelations inserts new relation rows.
func (r *DBConceptRelationRepository) BatchCreateRelations(ctx context.Context, records []*ConceptRelationRecord) error {
	if len(records) == 0 {
		return nil
	}
	query := database.BuildMultiRowInsert("concept_relations",
		[]string{"notebook_id", "type", "from_concept_id", "to_concept_id", "is_directed"},
		len(records))
	args := make([]any, 0, len(records)*5)
	for _, rec := range records {
		args = append(args, rec.NotebookID, rec.Type, rec.FromConceptID, rec.ToConceptID, rec.IsDirected)
	}
	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("insert concept_relations: %w", err)
	}
	return nil
}

// BatchDeleteRelations removes relation rows by ID.
func (r *DBConceptRelationRepository) BatchDeleteRelations(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	query, args, err := sqlx.In(`DELETE FROM concept_relations WHERE id IN (?)`, ids)
	if err != nil {
		return fmt.Errorf("build delete concept_relations: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("delete concept_relations: %w", err)
	}
	return nil
}
