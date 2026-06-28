package dictionary

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/database"
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

// BatchUpsert inserts or updates multiple dictionary entries in
// chunked multi-row INSERT … ON DUPLICATE KEY UPDATE statements. The
// previous implementation issued one round-trip per entry under
// autocommit, which on a remote backend (TiDB Cloud) dominated the
// sync-db wall clock — `response` carries the largest JSON blob in
// the schema, so chunk size is conservative to keep each statement
// well under MySQL's default max_allowed_packet.
func (r *DBDictionaryRepository) BatchUpsert(ctx context.Context, entries []*DictionaryEntry) error {
	if len(entries) == 0 {
		return nil
	}
	const chunkSize = 200
	for i := 0; i < len(entries); i += chunkSize {
		end := i + chunkSize
		if end > len(entries) {
			end = len(entries)
		}
		chunk := entries[i:end]
		placeholders := strings.Repeat("(?, ?, ?), ", len(chunk)-1) + "(?, ?, ?)"
		query := "INSERT INTO dictionary_entries (word, source_type, response) VALUES " +
			placeholders +
			" ON DUPLICATE KEY UPDATE source_type = VALUES(source_type), response = VALUES(response)"
		args := make([]interface{}, 0, len(chunk)*3)
		for _, e := range chunk {
			args = append(args, e.Word, e.SourceType, e.Response)
		}
		if err := database.ExecWithRetry(ctx, r.db, query, args...); err != nil {
			return fmt.Errorf("upsert dictionary entries (%d rows): %w", len(chunk), err)
		}
	}
	return nil
}
