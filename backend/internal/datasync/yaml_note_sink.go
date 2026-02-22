package datasync

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// YAMLNoteSink writes note records to YAML files.
type YAMLNoteSink struct {
	outputDir string
}

// NewYAMLNoteSink creates a new YAMLNoteSink.
func NewYAMLNoteSink(outputDir string) *YAMLNoteSink {
	return &YAMLNoteSink{outputDir: outputDir}
}

// WriteAll writes notes and notebook_notes to separate YAML files.
func (s *YAMLNoteSink) WriteAll(notes []notebook.NoteRecord) error {
	if err := os.MkdirAll(s.outputDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	if err := writeYAML(filepath.Join(s.outputDir, "notes.yml"), notes); err != nil {
		return fmt.Errorf("write notes.yml: %w", err)
	}

	var allNNs []notebook.NotebookNote
	for _, n := range notes {
		allNNs = append(allNNs, n.NotebookNotes...)
	}
	if err := writeYAML(filepath.Join(s.outputDir, "notebook_notes.yml"), allNNs); err != nil {
		return fmt.Errorf("write notebook_notes.yml: %w", err)
	}

	return nil
}

func writeYAML(path string, data interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	enc := yaml.NewEncoder(f)
	defer func() { _ = enc.Close() }()
	return enc.Encode(data)
}
