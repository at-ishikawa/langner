package dictionary

import (
	"encoding/json"
	"time"
)

// DictionaryEntry represents a cached dictionary API response.
type DictionaryEntry struct {
	Word       string          `db:"word"`
	SourceType string          `db:"source_type"`
	SourceURL  string          `db:"source_url"`
	Response   json.RawMessage `db:"response"`
	CreatedAt  time.Time       `db:"created_at"`
	UpdatedAt  time.Time       `db:"updated_at"`
}
