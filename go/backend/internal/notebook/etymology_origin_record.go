package notebook

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/database"
)

// EtymologyOriginRecord mirrors a row of the etymology_origins table.
// The (NotebookID, SessionTitle, Origin, Language) tuple is unique.
type EtymologyOriginRecord struct {
	ID           int64     `db:"id"`
	NotebookID   string    `db:"notebook_id"`
	SessionTitle string    `db:"session_title"`
	Origin       string    `db:"origin"`
	Type         string    `db:"type"`
	Language     string    `db:"language"`
	Meaning      string    `db:"meaning"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

// NoteOriginPartRecord mirrors a row of the note_origin_parts junction table.
type NoteOriginPartRecord struct {
	ID        int64     `db:"id"`
	NoteID    int64     `db:"note_id"`
	OriginID  int64     `db:"origin_id"`
	SortOrder int       `db:"sort_order"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// EtymologyOriginRepository is the storage interface for etymology origins.
// Importer code reads existing rows to deduplicate, then writes any new ones.
type EtymologyOriginRepository interface {
	FindAll(ctx context.Context) ([]EtymologyOriginRecord, error)
	BatchCreate(ctx context.Context, records []*EtymologyOriginRecord) error
}

// NoteOriginPartRepository is the storage interface for the note↔origin
// junction table. Reads use a notes-only filter so callers can build a
// "(note_id, sort_order) is taken" set before writing new rows.
type NoteOriginPartRepository interface {
	FindAll(ctx context.Context) ([]NoteOriginPartRecord, error)
	BatchCreate(ctx context.Context, records []*NoteOriginPartRecord) error
}

// DBEtymologyOriginRepository is a MySQL-backed implementation.
type DBEtymologyOriginRepository struct {
	db *sqlx.DB
}

// NewDBEtymologyOriginRepository constructs the repository.
func NewDBEtymologyOriginRepository(db *sqlx.DB) *DBEtymologyOriginRepository {
	return &DBEtymologyOriginRepository{db: db}
}

// FindAll returns every etymology origin row.
func (r *DBEtymologyOriginRepository) FindAll(ctx context.Context) ([]EtymologyOriginRecord, error) {
	var rows []EtymologyOriginRecord
	if err := r.db.SelectContext(ctx, &rows, `SELECT id, notebook_id, session_title, origin, type, language, meaning, created_at, updated_at FROM etymology_origins`); err != nil {
		return nil, fmt.Errorf("select etymology_origins: %w", err)
	}
	return rows, nil
}

// BatchCreate inserts new origin rows in one statement and writes back the
// auto-generated IDs by re-reading the unique key tuple. The inserts go in
// a transaction so partial failure can't leave the DB half-populated.
func (r *DBEtymologyOriginRepository) BatchCreate(ctx context.Context, records []*EtymologyOriginRecord) error {
	if len(records) == 0 {
		return nil
	}
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		query := database.BuildMultiRowInsert("etymology_origins",
			[]string{"notebook_id", "session_title", "origin", "type", "language", "meaning"},
			len(records))
		args := make([]any, 0, len(records)*6)
		for _, rec := range records {
			args = append(args, rec.NotebookID, rec.SessionTitle, rec.Origin, rec.Type, rec.Language, rec.Meaning)
		}
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("insert etymology_origins: %w", err)
		}
		// Reload IDs so callers can use them for note_origin_parts FKs.
		// Cheaper than per-row LAST_INSERT_ID because BatchCreate is the
		// only writer for new rows in a single import pass.
		var inserted []EtymologyOriginRecord
		if err := tx.SelectContext(ctx, &inserted, `SELECT id, notebook_id, session_title, origin, language FROM etymology_origins`); err != nil {
			return fmt.Errorf("reload etymology_origins after insert: %w", err)
		}
		idByKey := make(map[string]int64, len(inserted))
		for _, row := range inserted {
			idByKey[etymologyOriginKey(row.NotebookID, row.SessionTitle, row.Origin, row.Language)] = row.ID
		}
		for _, rec := range records {
			rec.ID = idByKey[etymologyOriginKey(rec.NotebookID, rec.SessionTitle, rec.Origin, rec.Language)]
		}
		return nil
	})
}

func etymologyOriginKey(notebookID, sessionTitle, origin, language string) string {
	return notebookID + "\x00" + sessionTitle + "\x00" + origin + "\x00" + language
}

// DBNoteOriginPartRepository is a MySQL-backed implementation.
type DBNoteOriginPartRepository struct {
	db *sqlx.DB
}

// NewDBNoteOriginPartRepository constructs the repository.
func NewDBNoteOriginPartRepository(db *sqlx.DB) *DBNoteOriginPartRepository {
	return &DBNoteOriginPartRepository{db: db}
}

// FindAll returns every note↔origin junction row.
func (r *DBNoteOriginPartRepository) FindAll(ctx context.Context) ([]NoteOriginPartRecord, error) {
	var rows []NoteOriginPartRecord
	if err := r.db.SelectContext(ctx, &rows, `SELECT id, note_id, origin_id, sort_order, created_at, updated_at FROM note_origin_parts`); err != nil {
		return nil, fmt.Errorf("select note_origin_parts: %w", err)
	}
	return rows, nil
}

// BatchCreate inserts new junction rows. Existing pairs are skipped via the
// unique (note_id, sort_order) constraint — callers are expected to pre-filter.
func (r *DBNoteOriginPartRepository) BatchCreate(ctx context.Context, records []*NoteOriginPartRecord) error {
	if len(records) == 0 {
		return nil
	}
	query := database.BuildMultiRowInsert("note_origin_parts",
		[]string{"note_id", "origin_id", "sort_order"},
		len(records))
	args := make([]any, 0, len(records)*3)
	for _, rec := range records {
		args = append(args, rec.NoteID, rec.OriginID, rec.SortOrder)
	}
	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("insert note_origin_parts: %w", err)
	}
	return nil
}
