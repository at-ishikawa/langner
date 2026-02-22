package datasync

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/at-ishikawa/langner/internal/dictionary"
)

type exportDictionaryEntry struct {
	Word       string `yaml:"word"`
	SourceType string `yaml:"source_type"`
	SourceURL  string `yaml:"source_url,omitempty"`
	Response   string `yaml:"response"`
}

// YAMLDictionarySink writes dictionary entries to a YAML file.
type YAMLDictionarySink struct {
	outputDir string
}

// NewYAMLDictionarySink creates a new YAMLDictionarySink.
func NewYAMLDictionarySink(outputDir string) *YAMLDictionarySink {
	return &YAMLDictionarySink{outputDir: outputDir}
}

// WriteAll writes dictionary entries to dictionary_entries.yml.
func (s *YAMLDictionarySink) WriteAll(entries []dictionary.DictionaryEntry) error {
	if err := os.MkdirAll(s.outputDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	exportEntries := make([]exportDictionaryEntry, len(entries))
	for i, e := range entries {
		exportEntries[i] = exportDictionaryEntry{
			Word:       e.Word,
			SourceType: e.SourceType,
			SourceURL:  e.SourceURL,
			Response:   string(e.Response),
		}
	}

	if err := writeYAML(filepath.Join(s.outputDir, "dictionary_entries.yml"), exportEntries); err != nil {
		return fmt.Errorf("write dictionary_entries.yml: %w", err)
	}
	return nil
}
