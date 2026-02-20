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

func WriteYamlFile[T any](path string, data T) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("os.Create(%s)> %w", path, err)
	}
	defer func() {
		_ = file.Close()
	}()

	return yaml.NewEncoder(file).Encode(data)
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
