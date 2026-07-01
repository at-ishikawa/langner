package notebook

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/database"
)

// EtymologyOriginFormRecord mirrors a row of the etymology_origin_forms
// table. The (OriginID, Role, Form) tuple is unique — same shape as the
// YAML side, where (form, role) identifies a variant within an origin.
type EtymologyOriginFormRecord struct {
	ID        int64     `db:"id"`
	OriginID  int64     `db:"origin_id"`
	Form      string    `db:"form"`
	Role      string    `db:"role"`
	Note      string    `db:"note"`
	SortOrder int       `db:"sort_order"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// EtymologyOriginFormRepository is the storage interface for etymology
// origin forms. Mirrors EtymologyOriginRepository: importer reads existing
// rows for dedup, writes new ones, updates Note/SortOrder when the YAML
// changes, and deletes rows the YAML no longer claims.
type EtymologyOriginFormRepository interface {
	FindAll(ctx context.Context) ([]EtymologyOriginFormRecord, error)
	BatchCreate(ctx context.Context, records []*EtymologyOriginFormRecord) error
	BatchUpdate(ctx context.Context, records []*EtymologyOriginFormRecord) error
	BatchDelete(ctx context.Context, ids []int64) error
}

// DBEtymologyOriginFormRepository is a PostgreSQL-backed implementation.
type DBEtymologyOriginFormRepository struct {
	db *sqlx.DB
}

// NewDBEtymologyOriginFormRepository constructs the repository.
func NewDBEtymologyOriginFormRepository(db *sqlx.DB) *DBEtymologyOriginFormRepository {
	return &DBEtymologyOriginFormRepository{db: db}
}

// FindAll returns every etymology origin form row.
func (r *DBEtymologyOriginFormRepository) FindAll(ctx context.Context) ([]EtymologyOriginFormRecord, error) {
	var rows []EtymologyOriginFormRecord
	if err := r.db.SelectContext(ctx, &rows, `SELECT id, origin_id, form, role, note, sort_order, created_at, updated_at FROM etymology_origin_forms`); err != nil {
		return nil, fmt.Errorf("select etymology_origin_forms: %w", err)
	}
	return rows, nil
}

// BatchCreate inserts new form rows in one statement and writes back the
// auto-generated IDs by re-reading the unique (origin_id, role, form)
// tuple. The inserts go in a transaction so partial failure can't leave
// the DB half-populated.
func (r *DBEtymologyOriginFormRepository) BatchCreate(ctx context.Context, records []*EtymologyOriginFormRecord) error {
	if len(records) == 0 {
		return nil
	}
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		query := database.BuildMultiRowInsert("etymology_origin_forms",
			[]string{"origin_id", "form", "role", "note", "sort_order"},
			len(records))
		args := make([]any, 0, len(records)*5)
		for _, rec := range records {
			args = append(args, rec.OriginID, rec.Form, rec.Role, rec.Note, rec.SortOrder)
		}
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("insert etymology_origin_forms: %w", err)
		}
		var inserted []EtymologyOriginFormRecord
		if err := tx.SelectContext(ctx, &inserted, `SELECT id, origin_id, form, role FROM etymology_origin_forms`); err != nil {
			return fmt.Errorf("reload etymology_origin_forms after insert: %w", err)
		}
		idByKey := make(map[string]int64, len(inserted))
		for _, row := range inserted {
			idByKey[etymologyOriginFormKey(row.OriginID, row.Role, row.Form)] = row.ID
		}
		for _, rec := range records {
			rec.ID = idByKey[etymologyOriginFormKey(rec.OriginID, rec.Role, rec.Form)]
		}
		return nil
	})
}

// BatchUpdate writes back Note and SortOrder on existing rows. The
// (origin_id, role, form) tuple is immutable — those changes go via
// delete-then-insert, not update.
func (r *DBEtymologyOriginFormRepository) BatchUpdate(ctx context.Context, records []*EtymologyOriginFormRecord) error {
	if len(records) == 0 {
		return nil
	}
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		for _, rec := range records {
			if _, err := tx.ExecContext(ctx,
				`UPDATE etymology_origin_forms SET note = $1, sort_order = $2 WHERE id = $3`,
				rec.Note, rec.SortOrder, rec.ID,
			); err != nil {
				return fmt.Errorf("update etymology_origin_form %d: %w", rec.ID, err)
			}
		}
		return nil
	})
}

// BatchDelete removes rows whose IDs are listed. Used by the reconcile
// pass when YAML no longer declares a form. note_origin_parts.origin_form_id
// is ON DELETE SET NULL, so dependent rows lose only the form pin.
func (r *DBEtymologyOriginFormRepository) BatchDelete(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	query, args, err := sqlx.In(`DELETE FROM etymology_origin_forms WHERE id IN (?)`, ids)
	if err != nil {
		return fmt.Errorf("build delete etymology_origin_forms: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("delete etymology_origin_forms: %w", err)
	}
	return nil
}

func etymologyOriginFormKey(originID int64, role, form string) string {
	return fmt.Sprintf("%d\x00%s\x00%s", originID, role, form)
}
