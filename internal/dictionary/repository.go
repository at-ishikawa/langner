package dictionary

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// DictionaryEntry represents a cached dictionary API response.
type DictionaryEntry struct {
	Word       string          `db:"word" yaml:"word"`
	SourceType string          `db:"source_type" yaml:"source_type"`
	SourceURL  string          `db:"source_url" yaml:"source_url"`
	Response   json.RawMessage `db:"response" yaml:"response"`
	CreatedAt  time.Time       `db:"created_at" yaml:"created_at"`
	UpdatedAt  time.Time       `db:"updated_at" yaml:"updated_at"`
}

// MarshalYAML serializes DictionaryEntry with Response as a JSON string.
func (d DictionaryEntry) MarshalYAML() (interface{}, error) {
	return &struct {
		Word       string    `yaml:"word"`
		SourceType string    `yaml:"source_type"`
		SourceURL  string    `yaml:"source_url"`
		Response   string    `yaml:"response"`
		CreatedAt  time.Time `yaml:"created_at"`
		UpdatedAt  time.Time `yaml:"updated_at"`
	}{
		Word:       d.Word,
		SourceType: d.SourceType,
		SourceURL:  d.SourceURL,
		Response:   string(d.Response),
		CreatedAt:  d.CreatedAt,
		UpdatedAt:  d.UpdatedAt,
	}, nil
}

// DictionaryRepository defines operations for managing dictionary entries.
type DictionaryRepository interface {
	FindAll(ctx context.Context) ([]DictionaryEntry, error)
	FindByWord(ctx context.Context, word string) (*DictionaryEntry, error)
	Upsert(ctx context.Context, entry *DictionaryEntry) error
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
