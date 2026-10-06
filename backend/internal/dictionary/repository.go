package dictionary

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

// FindAll returns all dictionary entries. Columns are listed explicitly (not
// SELECT *) so a drifted DB with an extra/unknown column doesn't crash the
// strict sqlx scan — the same drift-resilience the notes reads need.
func (r *DBDictionaryRepository) FindAll(ctx context.Context) ([]DictionaryEntry, error) {
	var entries []DictionaryEntry
	if err := r.db.SelectContext(ctx, &entries,
		"SELECT word, source_type, source_url, response, created_at, updated_at FROM dictionary_entries ORDER BY word"); err != nil {
		return nil, fmt.Errorf("load all dictionary entries: %w", err)
	}
	return entries, nil
}

// FindResponseByWord returns the stored RapidAPI response bytes for a word, and
// whether it exists. It is the DB-backed lookup the live reader consults before
// calling the external API (so a serverless deploy never touches the filesystem
// cache). A missing word is (nil, false, nil), not an error.
func (r *DBDictionaryRepository) FindResponseByWord(ctx context.Context, word string) (json.RawMessage, bool, error) {
	var resp json.RawMessage
	err := r.db.GetContext(ctx, &resp, "SELECT response FROM dictionary_entries WHERE word = $1", word)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find dictionary entry %q: %w", word, err)
	}
	return resp, true, nil
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
