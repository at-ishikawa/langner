package dictionary

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReader(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "valid config",
			config: Config{
				RapidAPIHost: "test.rapidapi.com",
				RapidAPIKey:  "test-key",
			},
		},
		{
			name: "empty config",
			config: Config{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := NewReader(t.TempDir(), tt.config)
			assert.NotNil(t, reader)
			assert.Equal(t, tt.config, reader.config)
			assert.NotNil(t, reader.fileCache)
		})
	}
}

func TestReader_LookupFromCache(t *testing.T) {
	// Note: This test focuses on the cache behavior rather than API calls
	// since API calls require external dependencies and credentials

	tests := []struct {
		name        string
		config      Config
		word        string
		expectError bool
	}{
		{
			name: "lookup non-existent word",
			config: Config{
				RapidAPIHost: "test.rapidapi.com",
				RapidAPIKey:  "test-key",
			},
			word:        "nonexistentword123456",
			expectError: true, // Will fail due to cache miss and invalid API
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := NewReader(t.TempDir(), tt.config)
			ctx := context.Background()

			result, err := reader.Lookup(ctx, tt.word)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, result.Word)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.word, result.Word)
			}
		})
	}
}

func TestReader_LookupFromCacheHit(t *testing.T) {
	tempDir := t.TempDir()

	// Pre-populate cache with a valid JSON file
	cacheContent := `{"word": "hello", "results": [{"definition": "a greeting", "partOfSpeech": "noun", "examples": ["hello there"]}], "pronunciation": {"all": "həˈloʊ"}}`
	err := os.WriteFile(filepath.Join(tempDir, "hello.json"), []byte(cacheContent), 0644)
	require.NoError(t, err)

	reader := NewReader(tempDir, Config{
		RapidAPIHost: "test.rapidapi.com",
		RapidAPIKey:  "test-key",
	})

	ctx := context.Background()
	result, err := reader.Lookup(ctx, "hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", result.Word)
	assert.Len(t, result.Results, 1)
	assert.Equal(t, "a greeting", result.Results[0].Definition)
}

func TestReader_LookupInvalidJSON(t *testing.T) {
	tempDir := t.TempDir()

	// Pre-populate cache with invalid JSON
	err := os.WriteFile(filepath.Join(tempDir, "bad.json"), []byte(`{invalid}`), 0644)
	require.NoError(t, err)

	reader := NewReader(tempDir, Config{})

	ctx := context.Background()
	_, err = reader.Lookup(ctx, "bad")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "json.Unmarshal")
}

func TestReader_Show(t *testing.T) {
	reader := NewReader(t.TempDir(), Config{})

	response := rapidapi.Response{
		Word: "hello",
		Results: []rapidapi.Result{
			{
				PartOfSpeech: "noun",
				Definition:   "a greeting",
				Synonyms:     []string{"hi", "hey"},
			},
			{
				PartOfSpeech: "interjection",
				Definition:   "used as a greeting",
			},
		},
	}

	// Show should not panic
	reader.Show(response)
}