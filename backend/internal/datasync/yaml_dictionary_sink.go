package datasync

import (
	"fmt"
	"path/filepath"

	"github.com/at-ishikawa/langner/internal/dictionary"
)

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
	if err := writeYAML(filepath.Join(s.outputDir, "dictionary_entries.yml"), entries); err != nil {
		return fmt.Errorf("write dictionary_entries.yml: %w", err)
	}
	return nil
}
