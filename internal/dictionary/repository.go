package dictionary

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// DictionaryRepository defines operations for managing dictionary entries.
type DictionaryRepository interface {
	FindAll(ctx context.Context) ([]DictionaryEntry, error)
	FindByWord(ctx context.Context, word string) (*DictionaryEntry, error)
	Upsert(ctx context.Context, entry *DictionaryEntry) error
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
		return nil, fmt.Errorf("db.SelectContext(dictionary_entries) > %w", err)
	}
	return entries, nil
}

// FindByWord returns a dictionary entry by word, or nil if not found.
func (r *DBDictionaryRepository) FindByWord(ctx context.Context, word string) (*DictionaryEntry, error) {
	var entry DictionaryEntry
	err := r.db.GetContext(ctx, &entry, "SELECT * FROM dictionary_entries WHERE word = ?", word)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db.GetContext(dictionary_entry) > %w", err)
	}
	return &entry, nil
}

// Upsert inserts or updates a dictionary entry.
func (r *DBDictionaryRepository) Upsert(ctx context.Context, entry *DictionaryEntry) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO dictionary_entries (word, source_type, source_url, response)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE source_type = VALUES(source_type), source_url = VALUES(source_url), response = VALUES(response)`,
		entry.Word, entry.SourceType, entry.SourceURL, entry.Response)
	if err != nil {
		return fmt.Errorf("db.ExecContext(upsert dictionary_entry) > %w", err)
	}
	return nil
}

// BatchUpsert inserts or updates multiple dictionary entries.
func (r *DBDictionaryRepository) BatchUpsert(ctx context.Context, entries []*DictionaryEntry) error {
	if len(entries) == 0 {
		return nil
	}
	const batchSize = 50
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		query := "INSERT INTO dictionary_entries (word, source_type, source_url, response) VALUES "
		args := make([]interface{}, 0, len(batch)*4)
		for j, e := range batch {
			if j > 0 {
				query += ", "
			}
			query += "(?, ?, ?, ?)"
			args = append(args, e.Word, e.SourceType, e.SourceURL, e.Response)
		}
		query += " ON DUPLICATE KEY UPDATE source_type = VALUES(source_type), source_url = VALUES(source_url), response = VALUES(response)"
		if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("db.ExecContext(batch upsert dictionary_entries) > %w", err)
		}
	}
	return nil
}
