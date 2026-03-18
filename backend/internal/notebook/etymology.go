package notebook

import (
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
		origins, err := readYamlFile[[]EtymologyOrigin](path)
		if err != nil {
			return nil, fmt.Errorf("readYamlFile(%s) > %w", path, err)
		}
		allOrigins = append(allOrigins, origins...)
	}

	index.Origins = allOrigins
	r.etymologyIndexes[etymologyID] = index
	return allOrigins, nil
}

// GetEtymologyIndexes returns the etymology indexes map.
func (r *Reader) GetEtymologyIndexes() map[string]EtymologyIndex {
	return r.etymologyIndexes
}

// walkEtymologyIndexFiles walks a directory and loads etymology index.yml files.
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

		if index.Kind != "Etymology" {
			return nil
		}

		index.Path = filepath.Dir(path)
		indexMap[index.ID] = index
		return nil
	})
}
