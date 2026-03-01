package rapidapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReader(t *testing.T) {
	reader := NewReader()
	assert.NotNil(t, reader)
}

func TestReader_Read(t *testing.T) {
	tests := []struct {
		name        string
		setupFiles  map[string]string // filename -> content
		wantCount   int
		wantWords   []string
		wantErr     bool
	}{
		{
			name: "single valid file",
			setupFiles: map[string]string{
				"hello.json": `{"word": "hello", "results": [{"definition": "a greeting", "partOfSpeech": "noun"}]}`,
			},
			wantCount: 1,
			wantWords: []string{"hello"},
		},
		{
			name: "multiple valid files",
			setupFiles: map[string]string{
				"apple.json":  `{"word": "apple", "results": [{"definition": "a fruit", "partOfSpeech": "noun"}]}`,
				"banana.json": `{"word": "banana", "results": [{"definition": "a yellow fruit", "partOfSpeech": "noun"}]}`,
			},
			wantCount: 2,
			wantWords: []string{"apple", "banana"},
		},
		{
			name: "skips .gitignore file",
			setupFiles: map[string]string{
				"word.json":  `{"word": "word", "results": []}`,
				".gitignore": "*.tmp",
			},
			wantCount: 1,
			wantWords: []string{"word"},
		},
		{
			name:       "empty directory",
			setupFiles: map[string]string{},
			wantCount:  0,
		},
		{
			name: "invalid JSON",
			setupFiles: map[string]string{
				"bad.json": `{invalid json`,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			for filename, content := range tt.setupFiles {
				err := os.WriteFile(filepath.Join(tempDir, filename), []byte(content), 0644)
				require.NoError(t, err)
			}

			reader := NewReader()
			got, err := reader.Read(tempDir)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, got, tt.wantCount)

			if tt.wantWords != nil {
				words := make([]string, len(got))
				for i, r := range got {
					words[i] = r.Word
				}
				for _, w := range tt.wantWords {
					assert.Contains(t, words, w)
				}
			}
		})
	}
}

func TestReader_Read_NonexistentDirectory(t *testing.T) {
	reader := NewReader()
	_, err := reader.Read("/nonexistent/directory")
	assert.Error(t, err)
}

func TestJSONDictionaryRepository_WriteAll(t *testing.T) {
	tests := []struct {
		name      string
		entries   []DictionaryExportEntry
		wantFiles map[string]string // relative path -> content
		wantErr   bool
	}{
		{
			name: "writes single entry",
			entries: []DictionaryExportEntry{
				{Word: "hello", Response: json.RawMessage(`{"word":"hello","results":[]}`)},
			},
			wantFiles: map[string]string{
				"dictionaries/rapidapi/hello.json": `{"word":"hello","results":[]}`,
			},
		},
		{
			name: "writes multiple entries",
			entries: []DictionaryExportEntry{
				{Word: "apple", Response: json.RawMessage(`{"word":"apple"}`)},
				{Word: "banana", Response: json.RawMessage(`{"word":"banana"}`)},
			},
			wantFiles: map[string]string{
				"dictionaries/rapidapi/apple.json":  `{"word":"apple"}`,
				"dictionaries/rapidapi/banana.json": `{"word":"banana"}`,
			},
		},
		{
			name:    "empty entries writes nothing",
			entries: []DictionaryExportEntry{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			repo := NewJSONDictionaryRepositoryWriter(tempDir)

			err := repo.WriteAll(tt.entries)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			for relPath, wantContent := range tt.wantFiles {
				got, err := os.ReadFile(filepath.Join(tempDir, relPath))
				require.NoError(t, err, "expected file %s to exist", relPath)
				assert.Equal(t, wantContent, string(got))
			}
		})
	}
}
