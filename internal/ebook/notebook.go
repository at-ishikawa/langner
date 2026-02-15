package ebook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// NotebookIndex represents the index.yml structure
type NotebookIndex struct {
	ID            string   `yaml:"id"`
	Name          string   `yaml:"name"`
	NotebookPaths []string `yaml:"notebooks"`
}

// NotebookEntry represents a story notebook entry
type NotebookEntry struct {
	Event  string          `yaml:"event"`
	Date   time.Time       `yaml:"date"`
	Scenes []NotebookScene `yaml:"scenes"`
}

// NotebookScene represents a scene in the notebook
type NotebookScene struct {
	Scene      string   `yaml:"scene"`
	Type       string   `yaml:"type,omitempty"`
	Statements []string `yaml:"statements,omitempty"`
}

// GenerateNotebooks creates notebook files from parsed chapters
func GenerateNotebooks(repo Repository, chapters []Chapter, booksDir string) error {
	// Create book directory
	bookDir := filepath.Join(booksDir, repo.ID)
	if err := os.MkdirAll(bookDir, 0755); err != nil {
		return fmt.Errorf("create book directory: %w", err)
	}

	// Generate notebook entries (one per chapter, numbered for ordering)
	var notebookPaths []string
	for i, chapter := range chapters {
		// Use numbered prefix to preserve order: 001-introduction.yml, 002-preface.yml, etc.
		baseName := strings.TrimSuffix(chapter.Filename, ".xhtml")
		notebookFile := fmt.Sprintf("%03d-%s.yml", i+1, baseName)
		notebookPath := filepath.Join(bookDir, notebookFile)

		// Create scenes from paragraphs (one scene per paragraph)
		scenes := make([]NotebookScene, 0, len(chapter.Paragraphs))
		for _, para := range chapter.Paragraphs {
			scene := NotebookScene{
				Scene:      "",
				Statements: para.Sentences,
			}
			if para.InBlockquote {
				scene.Type = "blockquote"
			}
			scenes = append(scenes, scene)
		}

		entry := NotebookEntry{
			Event:  chapter.Title,
			Date:   time.Now(),
			Scenes: scenes,
		}

		// Write as a list with single entry
		entries := []NotebookEntry{entry}
		data, err := yaml.Marshal(entries)
		if err != nil {
			return fmt.Errorf("marshal notebook: %w", err)
		}

		if err := os.WriteFile(notebookPath, data, 0644); err != nil {
			return fmt.Errorf("write notebook %s: %w", notebookPath, err)
		}

		notebookPaths = append(notebookPaths, notebookFile)
	}

	// Create index.yml
	index := NotebookIndex{
		ID:            repo.ID,
		Name:          fmt.Sprintf("%s by %s", repo.Title, repo.Author),
		NotebookPaths: notebookPaths,
	}

	indexData, err := yaml.Marshal(index)
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}

	indexPath := filepath.Join(bookDir, "index.yml")
	if err := os.WriteFile(indexPath, indexData, 0644); err != nil {
		return fmt.Errorf("write index: %w", err)
	}

	fmt.Printf("Generated %d notebook files in %s\n", len(chapters), bookDir)
	return nil
}
