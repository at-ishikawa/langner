package dictionary

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
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