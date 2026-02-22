package rapidapi

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type Reader struct {
}

func NewReader() *Reader {
	return &Reader{}
}

// JSONDictionaryRepository binds a cache directory and exposes ReadAll.
type JSONDictionaryRepository struct {
	cacheDir string
}

// NewJSONDictionaryRepository creates a new JSONDictionaryRepository.
func NewJSONDictionaryRepository(cacheDir string) *JSONDictionaryRepository {
	return &JSONDictionaryRepository{cacheDir: cacheDir}
}

// ReadAll reads all cached dictionary responses from the directory.
func (r *JSONDictionaryRepository) ReadAll() ([]Response, error) {
	return NewReader().Read(r.cacheDir)
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
