package notebook

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Definitions represents a definitions file for a book/story
type Definitions struct {
	Metadata DefinitionsMetadata  `yaml:"metadata"`
	Scenes   []DefinitionsScene   `yaml:"scenes"`
	Concepts []DefinitionConcept  `yaml:"concepts,omitempty"`
}

// DefinitionsMetadata contains metadata about which notebook the definitions apply to
// Either Notebook (filename) or Title (event name) can be used to identify the notebook
type DefinitionsMetadata struct {
	Notebook string    `yaml:"notebook,omitempty"` // e.g., "005-letter-1.yml"
	Title    string    `yaml:"title,omitempty"`    // e.g., "Letter I" (matches notebook.Event)
	Date     time.Time `yaml:"date,omitempty"`     // optional; used to sort notebooks on the quiz start page
}

// DefinitionsScene represents definitions for a specific scene
type DefinitionsScene struct {
	Metadata    DefinitionsSceneMetadata `yaml:"metadata"`
	Expressions []Note                   `yaml:"expressions"`
}

// DefinitionsSceneMetadata contains metadata to identify a scene
type DefinitionsSceneMetadata struct {
	Index int    `yaml:"index"`           // 0-based scene index
	Scene *int   `yaml:"scene,omitempty"` // alternative field name for index (pointer to distinguish unset from zero)
	Title string `yaml:"title,omitempty"` // scene title for human readability
}

// GetIndex returns the scene index, preferring Scene if set, otherwise Index
func (m DefinitionsSceneMetadata) GetIndex() int {
	if m.Scene != nil {
		return *m.Scene
	}
	return m.Index
}

// DefinitionConcept groups related vocabulary expressions in a definitions
// book under one umbrella meaning. Members are expression strings declared
// in the same book (across any session); the head doubles as the canonical
// display anchor and the database key. The same head may be declared in
// multiple sessions of the same book — the validator unifies them and
// enforces meaning agreement.
//
// Used downstream for quiz card collapse (one card per concept), learning
// log merging (logs flow to the head's entry), and skip propagation (a
// skip on any member applies to the whole concept).
type DefinitionConcept struct {
	Head        string   `yaml:"head"`
	Meaning     string   `yaml:"meaning"`
	Expressions []string `yaml:"expressions"`

	// SessionTitle records which session declared this concept block; set
	// at read time from the parent Definitions.Metadata. Not serialised.
	SessionTitle string `yaml:"-"`
}

// ReadDefinitionsFromBytes parses a YAML byte slice into a slice of Definitions.
func ReadDefinitionsFromBytes(data []byte) ([]Definitions, error) {
	var result []Definitions
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&result); err != nil {
		return nil, fmt.Errorf("yaml.Decode: %w", err)
	}
	return result, nil
}

// DefinitionsMap is a map of book ID -> notebook file -> scene key -> definitions
// Scene keys use index-based format (__index_N) to avoid duplication when
// multiple scenes share the same title (e.g., "In Monica's apartment").
type DefinitionsMap map[string]map[string]map[string][]Note

// definitionsIndex represents an index.yml for a definitions directory.
type definitionsIndex struct {
	ID        string   `yaml:"id"`
	Notebooks []string `yaml:"notebooks"`
}

// DefinitionsIndex is the public view of a definitions directory's index.yml.
// Used by tooling (migrators, validators) that need the book ID and ordered
// notebook file list without going through the full DefinitionsMap loader.
type DefinitionsIndex struct {
	ID        string
	Notebooks []string
}

// ReadDefinitionsIndex loads one definitions index.yml into a DefinitionsIndex.
func ReadDefinitionsIndex(path string) (DefinitionsIndex, error) {
	idx, err := readYamlFile[definitionsIndex](path)
	if err != nil {
		return DefinitionsIndex{}, err
	}
	return DefinitionsIndex{ID: idx.ID, Notebooks: idx.Notebooks}, nil
}

// loadDefinitionsFile loads a single definitions YAML file into the result map
// and updates the dates map with the latest `date` across all definitions in
// the file for the given bookID. raw, when non-nil, accumulates the parsed
// Definitions slice per book so callers that need the original scene titles
// (lost in the index-keyed result map) can recover them.
func loadDefinitionsFile(path string, bookID string, result DefinitionsMap, raw map[string][]Definitions, dates map[string]time.Time) error {
	definitions, err := readYamlFile[[]Definitions](path)
	if err != nil {
		return fmt.Errorf("readYamlFile(%s): %w", path, err)
	}

	if result[bookID] == nil {
		result[bookID] = make(map[string]map[string][]Note)
	}
	if raw != nil {
		raw[bookID] = append(raw[bookID], definitions...)
	}

	for _, def := range definitions {
		if def.Metadata.Date.After(dates[bookID]) {
			dates[bookID] = def.Metadata.Date
		}

		key := def.Metadata.Notebook
		if key == "" {
			key = def.Metadata.Title
		}
		if key == "" {
			continue
		}

		if result[bookID][key] == nil {
			result[bookID][key] = make(map[string][]Note)
		}

		for _, scene := range def.Scenes {
			sceneKey := fmt.Sprintf("__index_%d", scene.Metadata.GetIndex())
			result[bookID][key][sceneKey] = append(
				result[bookID][key][sceneKey],
				scene.Expressions...,
			)
		}
	}

	return nil
}

// NewDefinitionsMap loads definitions from the given directories. Returns the
// index-keyed result map (used by quiz code), a raw per-book Definitions
// slice (preserves scene titles for the notebook-detail view), and a per-book
// `latest date` map (populated from each definition file's metadata.date —
// the max wins per book).
func NewDefinitionsMap(directories []string) (DefinitionsMap, map[string][]Definitions, map[string]time.Time, error) {
	result := make(DefinitionsMap)
	raw := make(map[string][]Definitions)
	dates := make(map[string]time.Time)

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
				if err := loadDefinitionsFile(nbFullPath, idx.ID, result, raw, dates); err != nil {
					return err
				}
			}

			return nil
		})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("walk definitions directory %s (index pass): %w", dir, err)
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
			return loadDefinitionsFile(path, bookID, result, raw, dates)
		})

		if err != nil {
			return nil, nil, nil, fmt.Errorf("walk definitions directory %s: %w", dir, err)
		}
	}

	return result, raw, dates, nil
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

		// Merge definitions into each scene by index
		for j := range notebook.Scenes {
			sceneKey := fmt.Sprintf("__index_%d", j)
			sceneDefs, ok := notebookDefs[sceneKey]
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

// GetDefinitionsNotes returns the definitions for a given book ID from the definitions map.
// The returned map is keyed by title/notebook name, then by scene index key (__index_N).
func (r Reader) GetDefinitionsNotes(bookID string) (map[string]map[string][]Note, bool) {
	defs, ok := r.definitionsMap[bookID]
	return defs, ok
}

// GetDefinitionsNotesByTitle returns the same nested shape as
// GetDefinitionsNotes but keyed by the HUMAN scene title
// (scene.Metadata.Title) instead of the __index_N index key. Built from
// the raw definitions so it matches exactly what the notebook-detail RPC
// renders and what the skip/learning-history YAML stores.
//
// The quiz (loaders, counts, section summaries) reads through this so a
// definitions word's scene title is identical across the quiz, the
// detail page, and the learning-history file — otherwise a skip written
// under "verto (to turn)" is invisible to a quiz that looks up
// "__index_0", and the word's logs and skip end up split across two
// scene entries. The __index_N map stays in place for the story-merge
// path, which needs positional keys because story scene titles repeat.
func (r Reader) GetDefinitionsNotesByTitle(bookID string) (map[string]map[string][]Note, bool) {
	defs, ok := r.definitionsRaw[bookID]
	if !ok || len(defs) == 0 {
		return nil, false
	}
	result := make(map[string]map[string][]Note)
	for _, def := range defs {
		session := def.Metadata.Notebook
		if session == "" {
			session = def.Metadata.Title
		}
		if session == "" {
			continue
		}
		if result[session] == nil {
			result[session] = make(map[string][]Note)
		}
		for _, scene := range def.Scenes {
			sceneTitle := scene.Metadata.Title
			result[session][sceneTitle] = append(result[session][sceneTitle], scene.Expressions...)
		}
	}
	return result, true
}

// GetDefinitionsBookIDs returns all book IDs that have definitions in the definitions map.
func (r Reader) GetDefinitionsBookIDs() []string {
	var ids []string
	for id := range r.definitionsMap {
		ids = append(ids, id)
	}
	return ids
}

// GetDefinitionsLatestDate returns the latest `date` across all definition
// files for the given book ID, or the zero time if no dates are set.
func (r Reader) GetDefinitionsLatestDate(bookID string) time.Time {
	return r.definitionsDates[bookID]
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
