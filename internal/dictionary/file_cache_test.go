package dictionary

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileCache(t *testing.T) {
	tests := []struct {
		name        string
		api         string
		expectedDir string
	}{
		{
			name:        "rapidapi cache",
			api:         "rapidapi",
			expectedDir: "rapidapi",
		},
		{
			name:        "custom api cache",
			api:         "custom",
			expectedDir: "custom",
		},
		{
			name:        "empty api name",
			api:         "",
			expectedDir: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewFileCache(tt.api)
			assert.NotNil(t, cache)
			assert.Equal(t, tt.expectedDir, cache.rootDir)
		})
	}
}

func TestFileCache_filePath(t *testing.T) {
	tests := []struct {
		name       string
		api        string
		expression string
		expected   string
	}{
		{
			name:       "simple word",
			api:        "rapidapi",
			expression: "hello",
			expected:   filepath.Join("rapidapi", "hello.json"),
		},
		{
			name:       "word with spaces",
			api:        "rapidapi",
			expression: "hello world",
			expected:   filepath.Join("rapidapi", "hello world.json"),
		},
		{
			name:       "word with special characters",
			api:        "test",
			expression: "don't",
			expected:   filepath.Join("test", "don't.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewFileCache(tt.api)
			result := cache.filePath(tt.expression)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFileCache_cache(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	tests := []struct {
		name            string
		expression      string
		setupCache     bool
		cacheContent   string
		fetcherFunc    func() ([]byte, error)
		expectedResult string
		expectError    bool
	}{
		{
			name:        "cache miss - successful fetch",
			expression:  "test",
			setupCache: false,
			fetcherFunc: func() ([]byte, error) {
				return []byte(`{"word": "test"}`), nil
			},
			expectedResult: `{"word": "test"}`,
			expectError:    false,
		},
		{
			name:         "cache hit",
			expression:   "cached",
			setupCache:  true,
			cacheContent: `{"word": "cached", "source": "cache"}`,
			fetcherFunc: func() ([]byte, error) {
				return []byte(`{"word": "cached", "source": "api"}`), nil
			},
			expectedResult: `{"word": "cached", "source": "cache"}`,
			expectError:    false,
		},
		{
			name:        "cache miss - fetch error",
			expression:  "error",
			setupCache: false,
			fetcherFunc: func() ([]byte, error) {
				return nil, errors.New("API error")
			},
			expectedResult: "",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewFileCache(tempDir)

			// Setup cache file if needed
			if tt.setupCache {
				err := os.MkdirAll(cache.rootDir, 0755)
				require.NoError(t, err)
				
				filePath := cache.filePath(tt.expression)
				err = os.WriteFile(filePath, []byte(tt.cacheContent), 0644)
				require.NoError(t, err)
			}

			// Create directory for cache if testing cache miss
			if !tt.setupCache && !tt.expectError {
				err := os.MkdirAll(cache.rootDir, 0755)
				require.NoError(t, err)
			}

			// Test the cache function
			result, err := cache.cache(tt.expression, tt.fetcherFunc)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, string(result))

				// Verify file was created/exists
				filePath := cache.filePath(tt.expression)
				_, err := os.Stat(filePath)
				assert.NoError(t, err)
			}
		})
	}
}

func TestFileCache_read(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	tests := []struct {
		name           string
		expression     string
		setupFile      bool
		fileContent    string
		expectedResult string
		expectError    bool
	}{
		{
			name:           "existing file",
			expression:     "test",
			setupFile:      true,
			fileContent:    `{"word": "test", "definition": "a trial"}`,
			expectedResult: `{"word": "test", "definition": "a trial"}`,
			expectError:    false,
		},
		{
			name:        "non-existent file",
			expression:  "missing",
			setupFile:   false,
			expectError: true,
		},
		{
			name:           "empty file",
			expression:     "empty",
			setupFile:      true,
			fileContent:    "",
			expectedResult: "",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewFileCache(tempDir)

			// Setup file if needed
			if tt.setupFile {
				err := os.MkdirAll(cache.rootDir, 0755)
				require.NoError(t, err)
				
				filePath := cache.filePath(tt.expression)
				err = os.WriteFile(filePath, []byte(tt.fileContent), 0644)
				require.NoError(t, err)
			}

			// Test the read function
			result, err := cache.read(tt.expression)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, string(result))
			}
		})
	}
}