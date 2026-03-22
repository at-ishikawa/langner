package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// ExtractDefinitions extracts definitions from story notebooks into separate definition files
// and removes {{ }} markers from the story text.
func ExtractDefinitions(storiesDirectories []string, definitionsDirectory string) error {
	markerPattern := regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

	for _, storiesDir := range storiesDirectories {
		if storiesDir == "" {
			continue
		}
		if _, err := os.Stat(storiesDir); os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(storiesDir)
		if err != nil {
			return fmt.Errorf("read stories directory %s: %w", storiesDir, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			storyID := entry.Name()
			storyDir := filepath.Join(storiesDir, storyID)

			if err := extractDefinitionsFromStory(storyDir, storyID, definitionsDirectory, markerPattern); err != nil {
				return fmt.Errorf("extract definitions from %s: %w", storyID, err)
			}
		}
	}

	fmt.Println("Extraction complete!")
	return nil
}

// extractDefinitionsFromStory processes a single story directory
func extractDefinitionsFromStory(storyDir, storyID, definitionsDirectory string, markerPattern *regexp.Regexp) error {
	files, err := os.ReadDir(storyDir)
	if err != nil {
		return fmt.Errorf("read directory %s: %w", storyDir, err)
	}

	var allDefinitions []notebook.Definitions
	hasDefinitions := false

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if filepath.Ext(file.Name()) != ".yml" {
			continue
		}
		if file.Name() == "index.yml" {
			continue
		}

		filePath := filepath.Join(storyDir, file.Name())
		defs, modified, err := processStoryFile(filePath, file.Name(), markerPattern)
		if err != nil {
			fmt.Printf("  Skipping %s/%s: %v\n", storyID, file.Name(), err)
			continue
		}

		if len(defs) > 0 {
			allDefinitions = append(allDefinitions, defs...)
			hasDefinitions = true
		}

		if modified {
			fmt.Printf("  Processed: %s/%s\n", storyID, file.Name())
		}
	}

	if !hasDefinitions {
		return nil
	}

	// Check if definitions directory already exists
	defsDir := filepath.Join(definitionsDirectory, "stories", storyID)
	if _, err := os.Stat(defsDir); err == nil {
		fmt.Printf("  WARNING: definitions directory already exists for %s, skipping\n", storyID)
		return nil
	}

	// Create definitions directory and files
	if err := os.MkdirAll(defsDir, 0755); err != nil {
		return fmt.Errorf("create definitions directory %s: %w", defsDir, err)
	}

	// Write index.yml
	indexData := definitionsIndexFile{
		ID:        storyID,
		Notebooks: []string{"./definitions.yml"},
	}
	if err := writeYamlToFile(filepath.Join(defsDir, "index.yml"), indexData); err != nil {
		return fmt.Errorf("write index.yml: %w", err)
	}

	// Write definitions.yml
	if err := writeYamlToFile(filepath.Join(defsDir, "definitions.yml"), allDefinitions); err != nil {
		return fmt.Errorf("write definitions.yml: %w", err)
	}

	fmt.Printf("  Created definitions for: %s\n", storyID)
	return nil
}

// processStoryFile reads a story file, extracts definitions, removes markers and definitions sections.
// Returns one Definitions entry per event (StoryNotebook) and whether the file was modified.
// Each event's definitions are keyed by the event title so MergeDefinitionsIntoNotebooks can match them.
func processStoryFile(filePath, filename string, markerPattern *regexp.Regexp) ([]notebook.Definitions, bool, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, false, fmt.Errorf("read file %s: %w", filePath, err)
	}

	var notebooks []notebook.StoryNotebook
	if err := yaml.Unmarshal(data, &notebooks); err != nil {
		return nil, false, fmt.Errorf("unmarshal %s: %w", filePath, err)
	}

	var allDefs []notebook.Definitions
	modified := false

	for i := range notebooks {
		defs := notebook.Definitions{
			Metadata: notebook.DefinitionsMetadata{
				Title: notebooks[i].Event,
			},
		}

		for j := range notebooks[i].Scenes {
			scene := &notebooks[i].Scenes[j]

			if len(scene.Definitions) > 0 {
				defScene := notebook.DefinitionsScene{
					Metadata: notebook.DefinitionsSceneMetadata{
						Index: j,
						Title: scene.Title,
					},
					Expressions: scene.Definitions,
				}
				defs.Scenes = append(defs.Scenes, defScene)

				// Remove definitions from the scene
				scene.Definitions = nil
				modified = true
			}

			// Remove {{ }} markers from conversations
			for k := range scene.Conversations {
				replaced := markerPattern.ReplaceAllString(scene.Conversations[k].Quote, "$1")
				if replaced != scene.Conversations[k].Quote {
					scene.Conversations[k].Quote = replaced
					modified = true
				}
			}

			// Remove {{ }} markers from statements
			for k := range scene.Statements {
				replaced := markerPattern.ReplaceAllString(scene.Statements[k], "$1")
				if replaced != scene.Statements[k] {
					scene.Statements[k] = replaced
					modified = true
				}
			}
		}

		if len(defs.Scenes) > 0 {
			allDefs = append(allDefs, defs)
		}
	}

	if modified {
		if err := writeYamlToFile(filePath, notebooks); err != nil {
			return nil, false, fmt.Errorf("write modified file %s: %w", filePath, err)
		}
	}

	return allDefs, modified, nil
}

// definitionsIndexFile represents the index.yml structure for definitions
type definitionsIndexFile struct {
	ID        string   `yaml:"id"`
	Notebooks []string `yaml:"notebooks"`
}

// writeYamlToFile writes data to a YAML file
func writeYamlToFile(path string, data interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file %s: %w", path, err)
	}
	defer func() {
		_ = file.Close()
	}()

	encoder := yaml.NewEncoder(file)
	encoder.SetIndent(2)
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("encode yaml to %s: %w", path, err)
	}
	return encoder.Close()
}
