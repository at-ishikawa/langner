package dictionary

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type FileCache struct {
	rootDir string
}

func NewFileCache(cacheDirectory string) *FileCache {
	return &FileCache{
		rootDir: cacheDirectory,
	}
}

func (f *FileCache) filePath(expression string) string {
	return filepath.Join(f.rootDir, expression+".json")
}

func (cache *FileCache) cache(expression string, f func() ([]byte, error)) ([]byte, error) {
	localFilePath := cache.filePath(expression)
	if _, err := os.Stat(localFilePath); err == nil {
		contents, err := cache.read(expression)
		if err != nil {
			return nil, fmt.Errorf("cache.read > %w", err)
		}
		return contents, nil
	}

	contents, err := f()
	if err != nil {
		return nil, fmt.Errorf("storeWord for RapidAPI > %w", err)
	}

	file, err := os.Create(localFilePath)
	if err != nil {
		return nil, fmt.Errorf("os.Create > %w", err)
	}
	defer func() {
		_ = file.Close()
	}()
	if _, err := file.Write(contents); err != nil {
		return contents, fmt.Errorf("file.Write > %w", err)
	}
	return contents, nil
}

func (cache *FileCache) read(expression string) ([]byte, error) {
	file, err := os.Open(cache.filePath(expression))
	if err != nil {
		return nil, fmt.Errorf("os.Open > %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	contents, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("io.ReadAll > %w", err)
	}
	return contents, nil
}
