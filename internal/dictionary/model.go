package dictionary

import (
	"encoding/json"
	"time"
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
