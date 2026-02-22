package datasync

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/at-ishikawa/langner/internal/learning"
)

type exportLearningLog struct {
	ID             int64   `yaml:"id"`
	NoteID         int64   `yaml:"note_id"`
	Status         string  `yaml:"status"`
	LearnedAt      string  `yaml:"learned_at"`
	Quality        int     `yaml:"quality"`
	ResponseTimeMs int     `yaml:"response_time_ms"`
	QuizType       string  `yaml:"quiz_type"`
	IntervalDays   int     `yaml:"interval_days"`
	EasinessFactor float64 `yaml:"easiness_factor"`
}

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
	if err := os.MkdirAll(s.outputDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	out := make([]exportLearningLog, len(logs))
	for i, l := range logs {
		out[i] = exportLearningLog{
			ID:             l.ID,
			NoteID:         l.NoteID,
			Status:         l.Status,
			LearnedAt:      l.LearnedAt.Format("2006-01-02"),
			Quality:        l.Quality,
			ResponseTimeMs: l.ResponseTimeMs,
			QuizType:       l.QuizType,
			IntervalDays:   l.IntervalDays,
			EasinessFactor: l.EasinessFactor,
		}
	}

	if err := writeYAML(filepath.Join(s.outputDir, "learning_logs.yml"), out); err != nil {
		return fmt.Errorf("write learning_logs.yml: %w", err)
	}
	return nil
}
