package datasync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/dictionary"
)

func TestYAMLDictionarySink_WriteAll(t *testing.T) {
	tests := []struct {
		name     string
		entries  []dictionary.DictionaryEntry
		wantYAML string
	}{
		{
			name: "dictionary entries use snake_case field names",
			entries: []dictionary.DictionaryEntry{
				{
					Word:       "resilient",
					SourceType: "rapidapi",
					SourceURL:  "https://example.com/resilient",
					Response:   json.RawMessage(`{"word":"resilient"}`),
				},
				{
					Word:       "tenacious",
					SourceType: "rapidapi",
					Response:   json.RawMessage(`{"word":"tenacious"}`),
				},
			},
			wantYAML: `- word: resilient
  source_type: rapidapi
  source_url: https://example.com/resilient
  response: '{"word":"resilient"}'
- word: tenacious
  source_type: rapidapi
  response: '{"word":"tenacious"}'
`,
		},
		{
			name:    "empty entries",
			entries: []dictionary.DictionaryEntry{},
			wantYAML: `[]
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			sink := NewYAMLDictionarySink(dir)

			err := sink.WriteAll(tt.entries)
			require.NoError(t, err)

			got, err := os.ReadFile(filepath.Join(dir, "dictionary_entries.yml"))
			require.NoError(t, err)
			assert.Equal(t, tt.wantYAML, string(got))
		})
	}

	t.Run("MkdirAll error returns error", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "blocker")
		require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o644))

		sink := NewYAMLDictionarySink(filepath.Join(filePath, "subdir"))
		err := sink.WriteAll(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "create output directory")
	})

	t.Run("writeYAML error returns error", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "dictionary_entries.yml"), 0o755))

		sink := NewYAMLDictionarySink(dir)
		err := sink.WriteAll([]dictionary.DictionaryEntry{{Word: "resilient"}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write dictionary_entries.yml")
	})
}
