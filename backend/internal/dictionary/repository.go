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

// DBDictionaryRepository implements DictionaryRepository using MySQL.
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
		_, err := r.db.NamedExecContext(ctx,
			"INSERT INTO dictionary_entries (word, source_type, response) VALUES (:word, :source_type, :response) ON DUPLICATE KEY UPDATE source_type = VALUES(source_type), response = VALUES(response)",
			e)
		if err != nil {
			return fmt.Errorf("upsert dictionary entry: %w", err)
		}
	}
	return nil
}
