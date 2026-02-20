package ebook

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Repository represents a cloned ebook repository
type Repository struct {
	ID        string `yaml:"id"`
	RepoPath  string `yaml:"repo_path"`
	SourceURL string `yaml:"source_url"`
	WebURL    string `yaml:"web_url"`
	Title     string `yaml:"title"`
	Author    string `yaml:"author"`
}

// RepositoriesConfig stores the list of cloned repositories
type RepositoriesConfig struct {
	Repositories []Repository `yaml:"repositories"`
}

// Manager handles ebook repository operations
type Manager struct {
	repoDirectory    string
	repositoriesFile string
	booksDirectory   string
}

// NewManager creates a new ebook Manager
func NewManager(repoDirectory, repositoriesFile, booksDirectory string) *Manager {
	return &Manager{
		repoDirectory:    repoDirectory,
		repositoriesFile: repositoriesFile,
		booksDirectory:   booksDirectory,
	}
}

// Clone clones a Standard Ebook repository from the given URL
func (m *Manager) Clone(inputURL string) error {
	// Derive URLs from input
	repoName, sourceURL, webURL, err := deriveURLs(inputURL)
	if err != nil {
		return fmt.Errorf("deriveURLs(%s): %w", inputURL, err)
	}

	// Create repo directory if it doesn't exist
	if err := os.MkdirAll(m.repoDirectory, 0755); err != nil {
		return fmt.Errorf("create repo directory %s: %w", m.repoDirectory, err)
	}

	// Clone the repository
	repoPath := filepath.Join(m.repoDirectory, repoName)
	if _, err := os.Stat(repoPath); err == nil {
		return fmt.Errorf("repository already exists at %s", repoPath)
	}

	cmd := exec.Command("git", "clone", "--depth", "1", sourceURL, repoPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone %s: %w", sourceURL, err)
	}

	// Parse metadata from content.opf
	title, author, err := parseOPF(repoPath)
	if err != nil {
		fmt.Printf("Warning: could not parse OPF metadata: %v\n", err)
	}

	// Create repository entry
	repo := Repository{
		ID:        deriveID(repoName),
		RepoPath:  repoPath,
		SourceURL: sourceURL,
		WebURL:    webURL,
		Title:     title,
		Author:    author,
	}

	// Parse chapters and generate notebooks before adding to config
	chapters, err := ParseChapters(repoPath)
	if err != nil {
		fmt.Printf("Warning: could not parse chapters: %v\n", err)
	} else if len(chapters) > 0 {
		if err := GenerateNotebooks(repo, chapters, m.booksDirectory); err != nil {
			fmt.Printf("Warning: could not generate notebooks: %v\n", err)
		}
	}

	// Add to repositories file after successful notebook generation
	if err := m.addRepository(repo); err != nil {
		return fmt.Errorf("add repository to config: %w", err)
	}

	fmt.Printf("Successfully cloned %s to %s\n", repoName, repoPath)
	return nil
}

// List returns all cloned repositories
func (m *Manager) List() ([]Repository, error) {
	config, err := m.loadRepositoriesConfig()
	if err != nil {
		if os.IsNotExist(err) {
			return []Repository{}, nil
		}
		return nil, fmt.Errorf("load repositories config: %w", err)
	}
	return config.Repositories, nil
}

// Remove removes a cloned ebook repository by ID
func (m *Manager) Remove(id string) error {
	config, err := m.loadRepositoriesConfig()
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no repositories configured")
		}
		return fmt.Errorf("load repositories config: %w", err)
	}

	// Find and remove the repository
	var found *Repository
	newRepos := make([]Repository, 0, len(config.Repositories))
	for i := range config.Repositories {
		if config.Repositories[i].ID == id {
			found = &config.Repositories[i]
		} else {
			newRepos = append(newRepos, config.Repositories[i])
		}
	}

	if found == nil {
		return fmt.Errorf("repository with ID %q not found", id)
	}

	// Remove the ebook repository directory
	if err := os.RemoveAll(found.RepoPath); err != nil {
		return fmt.Errorf("remove directory %s: %w", found.RepoPath, err)
	}

	// Remove the notebooks directory
	notebooksDir := filepath.Join(m.booksDirectory, id)
	if err := os.RemoveAll(notebooksDir); err != nil {
		fmt.Printf("Warning: could not remove notebooks directory %s: %v\n", notebooksDir, err)
	}

	// Update config
	config.Repositories = newRepos
	if err := m.saveRepositoriesConfig(config); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("Removed ebook %q from %s\n", id, found.RepoPath)
	return nil
}

func (m *Manager) loadRepositoriesConfig() (*RepositoriesConfig, error) {
	data, err := os.ReadFile(m.repositoriesFile)
	if err != nil {
		return nil, err
	}

	var config RepositoriesConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("unmarshal repositories config: %w", err)
	}
	return &config, nil
}

func (m *Manager) saveRepositoriesConfig(config *RepositoriesConfig) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal repositories config: %w", err)
	}

	if err := os.WriteFile(m.repositoriesFile, data, 0644); err != nil {
		return fmt.Errorf("write repositories file: %w", err)
	}
	return nil
}

func (m *Manager) addRepository(repo Repository) error {
	config, err := m.loadRepositoriesConfig()
	if err != nil {
		if os.IsNotExist(err) {
			config = &RepositoriesConfig{}
		} else {
			return fmt.Errorf("load repositories config: %w", err)
		}
	}

	// Check if repository already exists
	for _, r := range config.Repositories {
		if r.ID == repo.ID {
			return fmt.Errorf("repository with ID %s already exists", repo.ID)
		}
	}

	config.Repositories = append(config.Repositories, repo)
	return m.saveRepositoriesConfig(config)
}
