package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
)

type Reader struct {
	indexes       map[string]Index
	dictionaryMap map[string]rapidapi.Response
}

func NewReader(
	rootStoryNotebookDirectory string,
	dictionaryMap map[string]rapidapi.Response,
) (*Reader, error) {
	indexes := make(map[string]Index, 0)
	err := filepath.Walk(rootStoryNotebookDirectory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) != "index.yml" {
			return nil
		}

		index, err := readYamlFile[Index](path)
		if err != nil {
			return err
		}
		index.path = filepath.Dir(path)
		indexes[index.ID] = index
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("filepath.Walk() > %w", err)
	}

	return &Reader{
		indexes:       indexes,
		dictionaryMap: dictionaryMap,
	}, nil
}

func (f Reader) ReadAllStoryNotebooks() (map[string]Index, error) {
	for _, index := range f.indexes {
		_, err := f.ReadStoryNotebooks(index.ID)
		if err != nil {
			return nil, fmt.Errorf("readStoryNotebook() > %w", err)
		}
	}
	return f.indexes, nil

}
func (f Reader) ReadAllNotes(storyID string, learningHistories map[string][]LearningHistory) ([]Note, error) {
	notebooks, err := f.ReadStoryNotebooks(storyID)
	if err != nil {
		return nil, fmt.Errorf("readStoryNotebooks() > %w", err)
	}
	learningHistory := learningHistories[storyID]

	flashCards := make([]Note, 0)
	for _, notebook := range notebooks {
		for _, scene := range notebook.Scenes {
			for _, definition := range scene.Definitions {
				for _, h := range learningHistory {
					logs := h.GetLogs(
						notebook.Event,
						scene.Title,
						definition,
					)
					if len(logs) == 0 {
						continue
					}

					// todo: Fix this!! temporary mitigation
					definition.LearnedLogs = logs
				}

				if !definition.needsToLearnInFlashcard(30) {
					continue
				}
				if err := definition.setDetails(f.dictionaryMap, ""); err != nil {
					return nil, fmt.Errorf("definition.setDetails() > %w", err)
				}
				if definition.Meaning == "" && len(definition.Images) == 0 {
					continue
				}

				definition.notebookDate = notebook.Date
				flashCards = append(flashCards, definition)
			}
		}
	}
	sort.Slice(flashCards, func(i, j int) bool {
		return flashCards[i].getLearnScore() < flashCards[j].getLearnScore()
	})
	return flashCards, nil
}
