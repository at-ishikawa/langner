package dictionary

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// DictionaryRepository defines operations for managing dictionary entries.
type DictionaryRepository interface {
	FindAll(ctx context.Context) ([]DictionaryEntry, error)
	BatchUpsert(ctx context.Context, entries []*DictionaryEntry) error
}

// DBDictionaryRepository implements DictionaryRepository using PostgreSQL.
type DBDictionaryRepository struct {
	db *sqlx.DB
}

// NewDBDictionaryRepository creates a new DBDictionaryRepository.
func NewDBDictionaryRepository(db *sqlx.DB) *DBDictionaryRepository {
	return &DBDictionaryRepository{db: db}
}

// FindAll returns all dictionary entries.
func (r *DBDictionaryRepository) FindAll(ctx context.Context) ([]DictionaryEntry, error) {
	var entries []DictionaryEntry
	if err := r.db.SelectContext(ctx, &entries, "SELECT * FROM dictionary_entries ORDER BY word"); err != nil {
		return nil, fmt.Errorf("load all dictionary entries: %w", err)
	}
	return entries, nil
}

// BatchUpsert inserts or updates multiple dictionary entries.
func (r *DBDictionaryRepository) BatchUpsert(ctx context.Context, entries []*DictionaryEntry) error {
	for _, e := range entries {
		_, err := r.db.ExecContext(ctx,
			`INSERT INTO dictionary_entries (word, source_type, response) VALUES ($1, $2, $3)
			 ON CONFLICT (word) DO UPDATE SET source_type = EXCLUDED.source_type, response = EXCLUDED.response`,
			e.Word, e.SourceType, e.Response)
		if err != nil {
			return fmt.Errorf("upsert dictionary entry: %w", err)
		}
	}
	return nil
}
