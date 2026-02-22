package learning

import (
	"fmt"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// YAMLLearningRepository reads learning history from YAML files.
type YAMLLearningRepository struct {
	directory string
}

// NewYAMLLearningRepository creates a new YAMLLearningRepository.
func NewYAMLLearningRepository(directory string) *YAMLLearningRepository {
	return &YAMLLearningRepository{directory: directory}
}

// FindByNotebookID reads learning YAML files, filters by notebook ID, and
// returns flattened expressions. Handles flashcard (top-level) vs story (scene) types.
func (r *YAMLLearningRepository) FindByNotebookID(notebookID string) ([]notebook.LearningHistoryExpression, error) {
	histories, err := notebook.NewLearningHistories(r.directory)
	if err != nil {
		return nil, fmt.Errorf("load learning histories: %w", err)
	}

	var result []notebook.LearningHistoryExpression
	for _, fileHistories := range histories {
		for _, h := range fileHistories {
			if h.Metadata.NotebookID != notebookID {
				continue
			}
			if h.Metadata.Type == "flashcard" {
				result = append(result, h.Expressions...)
				continue
			}
			for _, scene := range h.Scenes {
				result = append(result, scene.Expressions...)
			}
		}
	}

	return result, nil
}
