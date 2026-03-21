package notebook

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// EtymologyOrigin represents a single etymology origin (root, prefix, or suffix).
type EtymologyOrigin struct {
	Origin   string `yaml:"origin"`
	Type     string `yaml:"type"`     // prefix, suffix, root
	Language string `yaml:"language"` // Latin, Greek, etc.
	Meaning  string `yaml:"meaning"`
}

// EtymologyIndex represents an index file for etymology directories.
type EtymologyIndex struct {
	ID            string   `yaml:"id"`
	Kind          string   `yaml:"kind"` // "Etymology"
	Name          string   `yaml:"name"`
	NotebookPaths []string `yaml:"notebooks"`

	// internal fields
	Path    string            `yaml:"-"`
	Origins []EtymologyOrigin `yaml:"-"`
}

// EtymologyDefinitionEntry is a definition with origin_parts in an etymology session file.
type EtymologyDefinitionEntry struct {
	Definition   string          `yaml:"definition"`
	Expression   string          `yaml:"expression"`
	Meaning      string          `yaml:"meaning"`
	PartOfSpeech string          `yaml:"part_of_speech"`
	Note         string          `yaml:"note"`
	OriginParts  []OriginPartRef `yaml:"origin_parts"`
	NotebookName string          `yaml:"-"` // set at read time
}

// GetExpression returns the expression, falling back to the definition field.
func (e EtymologyDefinitionEntry) GetExpression() string {
	if e.Expression != "" {
		return e.Expression
	}
	return e.Definition
}

// etymologySessionFile supports both flat list and wrapped formats:
//
//	Flat:    [{ origin: tele, ... }, ...]
//	Wrapped: { origins: [...], definitions: [...] }
type etymologySessionFile struct {
	Origins     []EtymologyOrigin           `yaml:"origins"`
	Definitions []EtymologyDefinitionEntry  `yaml:"definitions"`
}

// ReadEtymologyNotebook reads the origins from an etymology notebook.
func (r *Reader) ReadEtymologyNotebook(etymologyID string) ([]EtymologyOrigin, error) {
	index, ok := r.etymologyIndexes[etymologyID]
	if !ok {
		return nil, fmt.Errorf("etymology notebook %s not found", etymologyID)
	}

	if len(index.Origins) > 0 {
		return index.Origins, nil
	}

	var allOrigins []EtymologyOrigin
	for _, notebookPath := range index.NotebookPaths {
		path := filepath.Join(index.Path, notebookPath)
		// Try flat list first (e.g., [{ origin: tele, ... }])
		origins, flatErr := readYamlFile[[]EtymologyOrigin](path)
		if flatErr == nil {
			allOrigins = append(allOrigins, origins...)
			continue
		}
		// Try wrapped format (e.g., origins: [{ origin: tele, ... }])
		wrapped, wrappedErr := readYamlFile[etymologySessionFile](path)
		if wrappedErr != nil {
			return nil, fmt.Errorf("readYamlFile(%s) > %w", path, flatErr)
		}
		allOrigins = append(allOrigins, wrapped.Origins...)
	}

	index.Origins = allOrigins
	r.etymologyIndexes[etymologyID] = index
	return allOrigins, nil
}

// ReadAllEtymologyDefinitions reads definitions with origin_parts from ALL
// notebook session files: etymology notebooks, story notebooks, and flashcard notebooks.
func (r *Reader) ReadAllEtymologyDefinitions() []EtymologyDefinitionEntry {
	var allDefs []EtymologyDefinitionEntry
	seen := make(map[string]bool) // track scanned paths to avoid duplicates

	scanSessionFiles := func(dir string, paths []string, notebookName string) {
		for _, nbPath := range paths {
			path := filepath.Join(dir, nbPath)
			if seen[path] {
				continue
			}
			seen[path] = true
			wrapped, err := readYamlFile[etymologySessionFile](path)
			if err != nil {
				continue
			}
			for _, def := range wrapped.Definitions {
				if len(def.OriginParts) > 0 {
					def.NotebookName = notebookName
					allDefs = append(allDefs, def)
				}
			}
		}
	}

	// Scan etymology notebook session files
	for _, index := range r.etymologyIndexes {
		scanSessionFiles(index.Path, index.NotebookPaths, index.Name)
	}

	// Scan story notebook session files
	for _, index := range r.indexes {
		scanSessionFiles(index.Path, index.NotebookPaths, index.Name)
	}

	// Scan flashcard notebook session files
	for _, index := range r.flashcardIndexes {
		scanSessionFiles(index.Path, index.NotebookPaths, index.Name)
	}

	return allDefs
}

// GetEtymologyIndexes returns the etymology indexes map.
func (r *Reader) GetEtymologyIndexes() map[string]EtymologyIndex {
	return r.etymologyIndexes
}

// sessionHasOriginsKey checks if a session YAML file has a top-level "origins:" key,
// indicating it defines etymology origins (not just references them via origin_parts).
func sessionHasOriginsKey(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// Check for "origins:" at the start of a line (top-level YAML key)
	for _, line := range bytes.Split(data, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if bytes.Equal(trimmed, []byte("origins:")) || bytes.HasPrefix(trimmed, []byte("origins: ")) {
			return true
		}
	}
	return false
}

// walkEtymologyIndexFiles walks a directory and loads etymology index.yml files.
// It loads indexes with kind: Etymology, and also indexes without a kind if their
// session files contain origin_parts (indicating etymology data).
func walkEtymologyIndexFiles(rootDir string, indexMap map[string]EtymologyIndex) error {
	if rootDir == "" {
		return nil
	}

	if _, err := os.Stat(rootDir); os.IsNotExist(err) {
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

		index, err := readYamlFile[EtymologyIndex](path)
		if err != nil {
			return err
		}

		dir := filepath.Dir(path)
		isEtymology := index.Kind == "Etymology"

		// For indexes without kind: Etymology, check if session files define origins
		if !isEtymology && index.Kind == "" {
			for _, nbPath := range index.NotebookPaths {
				if sessionHasOriginsKey(filepath.Join(dir, nbPath)) {
					isEtymology = true
					break
				}
			}
		}

		if !isEtymology {
			return nil
		}

		index.Path = dir
		indexMap[index.ID] = index
		return nil
	})
}
