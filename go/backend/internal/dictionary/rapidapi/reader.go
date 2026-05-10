package rapidapi

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// DictionaryExportEntry holds a word and its raw JSON response for export.
type DictionaryExportEntry struct {
	Word     string
	Response json.RawMessage
}

type Reader struct {
}

func NewReader() *Reader {
	return &Reader{}
}

// JSONDictionaryRepository binds a cache directory and exposes ReadAll and WriteAll.
type JSONDictionaryRepository struct {
	cacheDir  string
	outputDir string
}

// NewJSONDictionaryRepository creates a new JSONDictionaryRepository for reading.
func NewJSONDictionaryRepository(cacheDir string) *JSONDictionaryRepository {
	return &JSONDictionaryRepository{cacheDir: cacheDir}
}

// NewJSONDictionaryRepositoryWriter creates a new JSONDictionaryRepository for writing.
func NewJSONDictionaryRepositoryWriter(outputDir string) *JSONDictionaryRepository {
	return &JSONDictionaryRepository{outputDir: outputDir}
}

// ReadAll reads all cached dictionary responses from the directory.
func (r *JSONDictionaryRepository) ReadAll() ([]Response, error) {
	return NewReader().Read(r.cacheDir)
}

// WriteAll writes each DictionaryExportEntry as a JSON file under {outputDir}/dictionaries/rapidapi/{word}.json.
func (r *JSONDictionaryRepository) WriteAll(entries []DictionaryExportEntry) error {
	dir := filepath.Join(r.outputDir, "dictionaries", "rapidapi")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Word+".json")
		if err := os.WriteFile(path, entry.Response, 0644); err != nil {
			return fmt.Errorf("write dictionary file %s: %w", path, err)
		}
	}

	return nil
}

func (r *Reader) Read(dir string) ([]Response, error) {
	lookedUpWords, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("os.ReadDir > %w", err)
	}

	sort.Slice(lookedUpWords, func(i, j int) bool {
		// sort by file's timestamp in descending order

		iStat, err := os.Stat(filepath.Join(dir, lookedUpWords[i].Name()))
		if err != nil {
			return false
		}
		jStat, err := os.Stat(filepath.Join(dir, lookedUpWords[j].Name()))
		if err != nil {
			return false
		}
		return iStat.ModTime().After(jStat.ModTime())
	})
	dictionaries := make([]Response, 0, len(lookedUpWords))
	for _, word := range lookedUpWords {
		if word.Name() == ".gitignore" {
			continue
		}

		file := filepath.Join(dir, word.Name())
		f, err := os.Open(file)
		if err != nil {
			return nil, fmt.Errorf("word: %s, os.Open > %w", word, err)
		}
		defer func() {
			_ = f.Close()
		}()

		contents, err := io.ReadAll(f)
		if err != nil {
			return nil, fmt.Errorf("word: %s. io.ReadAll > %w", word, err)
		}

		var res Response
		if err := json.Unmarshal(contents, &res); err != nil {
			return nil, fmt.Errorf("word: %s. json.Unmarshal > %w", word, err)
		}
		dictionaries = append(dictionaries, res)
	}
	return dictionaries, nil
}
