package datasync

import (
	"fmt"
	"path/filepath"

	"github.com/at-ishikawa/langner/internal/learning"
)

// YAMLLearningSink writes learning log records to a YAML file.
type YAMLLearningSink struct {
	outputDir string
}

// NewYAMLLearningSink creates a new YAMLLearningSink.
func NewYAMLLearningSink(outputDir string) *YAMLLearningSink {
	return &YAMLLearningSink{outputDir: outputDir}
}

// WriteAll writes learning logs to learning_logs.yml.
func (s *YAMLLearningSink) WriteAll(logs []learning.LearningLog) error {
	if err := writeYAML(filepath.Join(s.outputDir, "learning_logs.yml"), logs); err != nil {
		return fmt.Errorf("write learning_logs.yml: %w", err)
	}
	return nil
}
