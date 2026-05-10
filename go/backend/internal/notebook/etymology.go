package notebook

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EtymologyOrigin represents a single etymology origin (root, prefix, or suffix).
type EtymologyOrigin struct {
	Origin   string `yaml:"origin"`
	Type     string `yaml:"type"`     // prefix, suffix, root
	Language string `yaml:"language"` // Latin, Greek, etc.
	Meaning  string `yaml:"meaning"`

	// SessionTitle is the parent session's metadata.title. Set at read time
	// from etymologySessionFile.Metadata.Title; not serialised.
	SessionTitle string `yaml:"-"`
}

// EtymologyIndex represents an index file for etymology directories.
type EtymologyIndex struct {
	ID            string   `yaml:"id"`
	Kind          string   `yaml:"kind"` // "Etymology"
	Name          string   `yaml:"name"`
	NotebookPaths []string `yaml:"notebooks"`

	// internal fields
	Path       string            `yaml:"-"`
	Origins    []EtymologyOrigin `yaml:"-"`
	LatestDate time.Time         `yaml:"-"` // max session-file `date`, used to sort notebooks
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
	SessionTitle string          `yaml:"-"` // set at read time from session metadata.title
}

// GetExpression returns the expression, falling back to the definition field.
func (e EtymologyDefinitionEntry) GetExpression() string {
	if e.Expression != "" {
		return e.Expression
	}
	return e.Definition
}

// EtymologySessionMetadata contains required metadata for an etymology session
// YAML file. Title is used as the disambiguator for multi-sense origins (the
// same origin string can appear in multiple sessions with different meanings)
// and is the binding key for definitions referencing those origins.
type EtymologySessionMetadata struct {
	Title string `yaml:"title"`
}

// etymologySessionFile is the wrapped format for etymology session YAML files.
// Required structure:
//
//	metadata:
//	  title: "Session 13"
//	date: 2025-01-15  # optional
//	origins:
//	  - origin: ...
//	  - ...
//	definitions:        # optional
//	  - ...
//
// metadata.title is required — readers reject files missing it.
type etymologySessionFile struct {
	Metadata    EtymologySessionMetadata   `yaml:"metadata"`
	Origins     []EtymologyOrigin          `yaml:"origins"`
	Definitions []EtymologyDefinitionEntry `yaml:"definitions"`
	// Date is optional. Used to sort etymology notebooks on the quiz start
	// page. The latest date across all session files wins.
	Date time.Time `yaml:"date,omitempty"`
}

// ReadEtymologyNotebook reads the origins from an etymology notebook.
//
// Each session file is required to use the wrapped format with metadata.title.
// Origins are tagged with their session's title via SessionTitle, which is the
// disambiguator used by quizzes and the etymology page when the same origin
// string appears in multiple sessions with different meanings.
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
		wrapped, err := readYamlFile[etymologySessionFile](path)
		if err != nil {
			return nil, fmt.Errorf("read etymology session %s: %w", path, err)
		}
		title := strings.TrimSpace(wrapped.Metadata.Title)
		if title == "" {
			return nil, fmt.Errorf("etymology session %s missing required metadata.title", path)
		}
		for _, o := range wrapped.Origins {
			o.SessionTitle = title
			allOrigins = append(allOrigins, o)
		}
	}

	index.Origins = allOrigins
	r.etymologyIndexes[etymologyID] = index
	return allOrigins, nil
}

// ReadAllEtymologyDefinitions reads definitions with origin_parts from ALL
// notebook session files: etymology notebooks, story notebooks, and flashcard notebooks.
//
// SessionTitle on each returned entry is set from the session file's
// metadata.title — the binding key used by Phase 5 to disambiguate which sense
// of an origin a definition refers to.
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
			sessionTitle := strings.TrimSpace(wrapped.Metadata.Title)
			for _, def := range wrapped.Definitions {
				if len(def.OriginParts) > 0 {
					def.NotebookName = notebookName
					def.SessionTitle = sessionTitle
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

		// Read the `date` field from each session file and keep the latest.
		// Each session file must have metadata.title — fail fast otherwise so
		// the operator notices missing metadata at startup rather than at quiz
		// time when origins disappear from the deduplicated set.
		for _, nbPath := range index.NotebookPaths {
			sessionPath := filepath.Join(dir, nbPath)
			wrapped, err := readYamlFile[etymologySessionFile](sessionPath)
			if err != nil {
				return fmt.Errorf("read etymology session %s: %w", sessionPath, err)
			}
			if strings.TrimSpace(wrapped.Metadata.Title) == "" {
				return fmt.Errorf("etymology session %s missing required metadata.title", sessionPath)
			}
			if wrapped.Date.After(index.LatestDate) {
				index.LatestDate = wrapped.Date
			}
		}

		indexMap[index.ID] = index
		return nil
	})
}
