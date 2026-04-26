package notebook

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func readYamlFile[T any](path string) (T, error) {
	var result T

	file, err := os.Open(path)
	if err != nil {
		return result, fmt.Errorf("os.Open(%s)> %w", path, err)
	}
	defer func() {
		_ = file.Close()
	}()

	if err := yaml.NewDecoder(file).Decode(&result); err != nil {
		return result, fmt.Errorf("yaml.NewDecoder().Decode()> %w", err)
	}
	return result, nil
}

// WriteYamlFile atomically replaces the file at path with the YAML encoding
// of data. The encoded bytes are written to a sibling temp file and then
// renamed over the destination. This prevents two racing writers (or a
// crashed writer) from leaving a half-written, garbage-interleaved file —
// either the rename completes and readers see the new content, or it
// doesn't and they see the previous content. A previous version used
// os.Create + streaming yaml.Encoder, which truncated the file in place
// and corrupted it under concurrent writes.
func WriteYamlFile[T any](path string, data T) error {
	encoded, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("yaml.Marshal(%s)> %w", path, err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("os.CreateTemp(%s)> %w", dir, err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmp.Write(encoded); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write(%s)> %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close(%s)> %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("rename(%s -> %s)> %w", tmpPath, path, err)
	}
	return nil
}

// yamlFile represents a YAML file with its path and contents
type yamlFile[T any] struct {
	path     string
	contents T
}

// loadYamlFiles is a generic function to load YAML files from a directory
func loadYamlFiles[T any](dir string, filter func(path string, info os.FileInfo) bool) ([]yamlFile[T], error) {
	var files []yamlFile[T]

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !filter(path, info) {
			return nil
		}

		contents, err := readYamlFile[T](path)
		if err != nil {
			return fmt.Errorf("readYamlFile(%s) > %w", path, err)
		}

		files = append(files, yamlFile[T]{
			path:     path,
			contents: contents,
		})
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("filepath.Walk(%s) > %w", dir, err)
	}

	return files, nil
}

// loadYamlFilesAsMap is a generic function to load YAML files from a directory and return them as a map
// where the key is the basename (without extension) of each file
// loadYamlFilesSkipErrors is like loadYamlFiles but silently skips files that
// fail to parse (e.g., definition-only files in a story notebook directory).
func loadYamlFilesSkipErrors[T any](dir string, filter func(path string, info os.FileInfo) bool) ([]yamlFile[T], error) {
	var files []yamlFile[T]

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !filter(path, info) {
			return nil
		}

		contents, err := readYamlFile[T](path)
		if err != nil {
			// Skip files that can't be parsed as T (e.g., definition-only files)
			return nil
		}

		files = append(files, yamlFile[T]{
			path:     path,
			contents: contents,
		})
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("filepath.Walk(%s) > %w", dir, err)
	}

	return files, nil
}

func loadYamlFilesAsMap[T any](dir string, filter func(path string, info os.FileInfo) bool) (map[string]T, error) {
	yamlFiles, err := loadYamlFiles[T](dir, filter)
	if err != nil {
		return nil, fmt.Errorf("loadYamlFiles(%s) > %w", dir, err)
	}
	filesMap := make(map[string]T)
	for _, file := range yamlFiles {
		extension := filepath.Ext(file.path)
		basename := filepath.Base(file.path)
		basename = basename[:len(basename)-len(extension)]
		filesMap[basename] = file.contents
	}
	return filesMap, nil
}
