package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
)

type Reader struct {
	indexes          map[string]Index
	flashcardIndexes map[string]FlashcardIndex
	dictionaryMap    map[string]rapidapi.Response
}

// walkIndexFiles walks a directory and loads index.yml files into the provided map
func walkIndexFiles[T Index | FlashcardIndex](rootDir string, indexMap map[string]T) error {
	if rootDir == "" {
		return nil
	}

	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) != "index.yml" {
			return nil
		}

		index, err := readYamlFile[T](path)
		if err != nil {
			return err
		}

		// Set the path field on the index using type assertion
		// This works because both Index and FlashcardIndex have a path field
		switch v := any(&index).(type) {
		case *Index:
			v.path = filepath.Dir(path)
			indexMap[v.ID] = any(*v).(T)
		case *FlashcardIndex:
			v.path = filepath.Dir(path)
			indexMap[v.ID] = any(*v).(T)
		}
		return nil
	})
}

func NewReader(
	storyDirectories []string,
	flashcardDirectories []string,
	dictionaryMap map[string]rapidapi.Response,
) (*Reader, error) {
	indexes := make(map[string]Index, 0)
	for _, dir := range storyDirectories {
		if err := walkIndexFiles(dir, indexes); err != nil {
			return nil, fmt.Errorf("walkIndexFiles(story, %s) > %w", dir, err)
		}
	}

	flashcardIndexes := make(map[string]FlashcardIndex, 0)
	for _, dir := range flashcardDirectories {
		if err := walkIndexFiles(dir, flashcardIndexes); err != nil {
			return nil, fmt.Errorf("walkIndexFiles(flashcard, %s) > %w", dir, err)
		}
	}

	return &Reader{
		indexes:          indexes,
		flashcardIndexes: flashcardIndexes,
		dictionaryMap:    dictionaryMap,
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

func (f Reader) ReadFlashcardNotebooks(flashcardID string) ([]FlashcardNotebook, error) {
	index, ok := f.flashcardIndexes[flashcardID]
	if !ok {
		return nil, fmt.Errorf("flashcard %s not found", flashcardID)
	}

	result := make([]FlashcardNotebook, 0)
	for _, notebookPath := range index.NotebookPaths {
		path := filepath.Join(index.path, notebookPath)

		notebooks, err := readYamlFile[[]FlashcardNotebook](path)
		if err != nil {
			return nil, fmt.Errorf("readYamlFile(%s) > %w", path, err)
		}

		index.Notebooks = append(index.Notebooks, notebooks...)
		result = append(result, notebooks...)
	}
	f.flashcardIndexes[flashcardID] = index
	return result, nil
}

func (f Reader) ReadAllFlashcardNotebooks() (map[string]FlashcardIndex, error) {
	for _, index := range f.flashcardIndexes {
		_, err := f.ReadFlashcardNotebooks(index.ID)
		if err != nil {
			return nil, fmt.Errorf("ReadFlashcardNotebooks() > %w", err)
		}
	}
	return f.flashcardIndexes, nil
}

func (f Reader) GetFlashcardIndexes() map[string]FlashcardIndex {
	return f.flashcardIndexes
}

func (f Reader) GetStoryIndexes() map[string]Index {
	return f.indexes
}
