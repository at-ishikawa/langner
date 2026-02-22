package datasync

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/at-ishikawa/langner/internal/notebook"
)

type exportNote struct {
	ID               int64  `yaml:"id"`
	Usage            string `yaml:"usage"`
	Entry            string `yaml:"entry"`
	Meaning          string `yaml:"meaning"`
	Level            string `yaml:"level,omitempty"`
	DictionaryNumber int    `yaml:"dictionary_number,omitempty"`
}

type exportNotebookNote struct {
	NoteID       int64  `yaml:"note_id"`
	NotebookType string `yaml:"notebook_type"`
	NotebookID   string `yaml:"notebook_id"`
	Group        string `yaml:"group"`
	Subgroup     string `yaml:"subgroup,omitempty"`
}

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

	exportNotes := make([]exportNote, len(notes))
	var allNNs []exportNotebookNote
	for i, n := range notes {
		exportNotes[i] = exportNote{
			ID:               n.ID,
			Usage:            n.Usage,
			Entry:            n.Entry,
			Meaning:          n.Meaning,
			Level:            n.Level,
			DictionaryNumber: n.DictionaryNumber,
		}
		for _, nn := range n.NotebookNotes {
			allNNs = append(allNNs, exportNotebookNote{
				NoteID:       nn.NoteID,
				NotebookType: nn.NotebookType,
				NotebookID:   nn.NotebookID,
				Group:        nn.Group,
				Subgroup:     nn.Subgroup,
			})
		}
	}

	if err := writeYAML(filepath.Join(s.outputDir, "notes.yml"), exportNotes); err != nil {
		return fmt.Errorf("write notes.yml: %w", err)
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
