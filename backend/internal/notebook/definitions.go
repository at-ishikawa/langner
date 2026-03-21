package notebook

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Definitions represents a definitions file for a book/story
type Definitions struct {
	Metadata DefinitionsMetadata  `yaml:"metadata"`
	Scenes   []DefinitionsScene   `yaml:"scenes"`
}

// DefinitionsMetadata contains metadata about which notebook the definitions apply to
// Either Notebook (filename) or Title (event name) can be used to identify the notebook
type DefinitionsMetadata struct {
	Notebook string `yaml:"notebook,omitempty"` // e.g., "005-letter-1.yml"
	Title    string `yaml:"title,omitempty"`    // e.g., "Letter I" (matches notebook.Event)
}

// DefinitionsScene represents definitions for a specific scene
type DefinitionsScene struct {
	Metadata    DefinitionsSceneMetadata `yaml:"metadata"`
	Expressions []Note                   `yaml:"expressions"`
}

// DefinitionsSceneMetadata contains metadata to identify a scene
type DefinitionsSceneMetadata struct {
	Index int  `yaml:"index"` // 0-based scene index
	Scene *int `yaml:"scene"` // alternative field name for index (pointer to distinguish unset from zero)
}

// GetIndex returns the scene index, preferring Scene if set, otherwise Index
func (m DefinitionsSceneMetadata) GetIndex() int {
	if m.Scene != nil {
		return *m.Scene
	}
	return m.Index
}

// ReadDefinitionsFromBytes parses a YAML byte slice into a slice of Definitions.
func ReadDefinitionsFromBytes(data []byte) ([]Definitions, error) {
	var result []Definitions
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&result); err != nil {
		return nil, fmt.Errorf("yaml.Decode: %w", err)
	}
	return result, nil
}

// DefinitionsMap is a map of book ID -> notebook file -> scene index -> definitions
type DefinitionsMap map[string]map[string]map[int][]Note

// definitionsIndex represents an index.yml for a definitions directory.
type definitionsIndex struct {
	ID        string   `yaml:"id"`
	Notebooks []string `yaml:"notebooks"`
}

// loadDefinitionsFile loads a single definitions YAML file into the result map.
func loadDefinitionsFile(path string, bookID string, result DefinitionsMap) error {
	definitions, err := readYamlFile[[]Definitions](path)
	if err != nil {
		return fmt.Errorf("readYamlFile(%s): %w", path, err)
	}

	if result[bookID] == nil {
		result[bookID] = make(map[string]map[int][]Note)
	}

	for _, def := range definitions {
		key := def.Metadata.Notebook
		if key == "" {
			key = def.Metadata.Title
		}
		if key == "" {
			continue
		}

		if result[bookID][key] == nil {
			result[bookID][key] = make(map[int][]Note)
		}

		for _, scene := range def.Scenes {
			sceneIndex := scene.Metadata.GetIndex()
			result[bookID][key][sceneIndex] = append(
				result[bookID][key][sceneIndex],
				scene.Expressions...,
			)
		}
	}

	return nil
}

// NewDefinitionsMap loads definitions from the given directories
func NewDefinitionsMap(directories []string) (DefinitionsMap, error) {
	result := make(DefinitionsMap)

	for _, dir := range directories {
		if dir == "" {
			continue
		}

		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		// Track directories with index.yml so we skip them in the walk
		indexedDirs := make(map[string]bool)

		// First pass: find directories with index.yml
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || filepath.Base(path) != "index.yml" {
				return nil
			}

			idx, err := readYamlFile[definitionsIndex](path)
			if err != nil || idx.ID == "" {
				return nil // skip invalid index files
			}

			indexDir := filepath.Dir(path)
			indexedDirs[indexDir] = true

			for _, nbPath := range idx.Notebooks {
				nbFullPath := filepath.Join(indexDir, nbPath)
				if err := loadDefinitionsFile(nbFullPath, idx.ID, result); err != nil {
					return err
				}
			}

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk definitions directory %s (index pass): %w", dir, err)
		}

		// Second pass: load standalone .yml files (not in indexed directories)
		err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".yml" || filepath.Base(path) == "index.yml" {
				return nil
			}

			// Skip files in directories that have index.yml
			if indexedDirs[filepath.Dir(path)] {
				return nil
			}

			// Book ID from filename
			bookID := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			return loadDefinitionsFile(path, bookID, result)
		})

		if err != nil {
			return nil, fmt.Errorf("walk definitions directory %s: %w", dir, err)
		}
	}

	return result, nil
}

// MergeDefinitionsIntoNotebooks merges definitions from the definitions map into story notebooks
func MergeDefinitionsIntoNotebooks(
	bookID string,
	notebooks []StoryNotebook,
	notebookPaths []string,
	definitionsMap DefinitionsMap,
) []StoryNotebook {
	bookDefs, ok := definitionsMap[bookID]
	if !ok {
		return notebooks
	}

	for i, notebook := range notebooks {
		// Get the notebook filename
		if i >= len(notebookPaths) {
			continue
		}
		notebookFile := notebookPaths[i]

		// Try to match by notebook filename first, then by title (Event)
		notebookDefs, ok := bookDefs[notebookFile]
		if !ok {
			// Try matching by title (notebook.Event)
			notebookDefs, ok = bookDefs[notebook.Event]
			if !ok {
				continue
			}
		}

		// Merge definitions into each scene
		for j := range notebook.Scenes {
			sceneDefs, ok := notebookDefs[j]
			if !ok {
				continue
			}

			// Add definitions to scene and add {{ }} markers to statements
			for _, note := range sceneDefs {
				notebooks[i].Scenes[j].Definitions = append(notebooks[i].Scenes[j].Definitions, note)

				// Add markers to all statements containing this expression
				for k := range notebooks[i].Scenes[j].Statements {
					notebooks[i].Scenes[j].Statements[k] = addExpressionMarker(
						notebooks[i].Scenes[j].Statements[k],
						note.Expression,
					)
				}
			}
		}
	}

	return notebooks
}

// addExpressionMarker adds {{ }} markers around an expression in text (case-insensitive)
func addExpressionMarker(text, expression string) string {
	// Skip if already has markers (case-insensitive check to match replacement behavior)
	if strings.Contains(text, "{{") && strings.Contains(text, "}}") {
		// Check if this specific expression already has markers (case-insensitive)
		markerPattern := regexp.MustCompile(`(?i)\{\{\s*` + regexp.QuoteMeta(expression) + `\s*\}\}`)
		if markerPattern.MatchString(text) {
			return text
		}
	}

	// Case-insensitive replacement
	pattern := regexp.MustCompile(`(?i)\b(` + regexp.QuoteMeta(expression) + `)\b`)
	return pattern.ReplaceAllString(text, "{{ $1 }}")
}
