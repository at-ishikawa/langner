package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEbookCommand(t *testing.T) {
	cmd := newEbookCommand()

	assert.Equal(t, "ebook", cmd.Use)
	assert.Equal(t, "Manage ebook repositories", cmd.Short)
	assert.True(t, cmd.HasSubCommands())
}

func TestNewEbookCloneCommand(t *testing.T) {
	cmd := newEbookCloneCommand()

	assert.Equal(t, "clone <url>", cmd.Use)
	assert.Equal(t, "Clone a Standard Ebooks repository", cmd.Short)
	assert.NotNil(t, cmd.RunE)
}

func TestNewEbookListCommand(t *testing.T) {
	cmd := newEbookListCommand()

	assert.Equal(t, "list", cmd.Use)
	assert.Equal(t, "List cloned ebook repositories", cmd.Short)
	assert.NotNil(t, cmd.RunE)
}

func TestNewEbookRemoveCommand(t *testing.T) {
	cmd := newEbookRemoveCommand()

	assert.Equal(t, "remove <id>", cmd.Use)
	assert.Equal(t, "Remove a cloned ebook repository", cmd.Short)
	assert.NotNil(t, cmd.RunE)
}

func TestNewEbookListCommand_RunE(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newEbookListCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestNewEbookListCommand_RunE_WithRepos(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	// Create repos config with entries
	reposFile := filepath.Join(tmpDir, "repos.yml")
	content := `repositories:
  - id: test-book
    repo_path: /tmp/test
    title: Test Book
    author: Test Author
`
	require.NoError(t, os.WriteFile(reposFile, []byte(content), 0644))

	cmd := newEbookListCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestNewEbookRemoveCommand_RunE_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	// Create empty repos config
	reposFile := filepath.Join(tmpDir, "repos.yml")
	require.NoError(t, os.WriteFile(reposFile, []byte("repositories: []\n"), 0644))

	cmd := newEbookRemoveCommand()
	cmd.SetArgs([]string{"nonexistent"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestNewEbookCloneCommand_RunE_InvalidURL(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newEbookCloneCommand()
	cmd.SetArgs([]string{"not-a-valid-url"})
	err := cmd.Execute()
	assert.Error(t, err)
}

// setupBrokenConfigFile creates a config file with invalid YAML that causes Load() to fail
func setupBrokenConfigFile(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("{{invalid yaml content"), 0644))
	return cfgPath
}

func TestEbookCommands_InvalidConfig(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	tests := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
	}{
		{name: "clone", cmd: newEbookCloneCommand, args: []string{"https://example.com"}},
		{name: "list", cmd: newEbookListCommand, args: nil},
		{name: "remove", cmd: newEbookRemoveCommand, args: []string{"test-id"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmd()
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "configuration")
		})
	}
}

// setupTestConfigFile creates a minimal config file for testing
func setupTestConfigFile(t *testing.T, tmpDir string) string {
	t.Helper()

	dirs := []string{
		"stories", "learning_notes", "flashcards",
		"dictionaries", "output_stories", "output_flashcards",
		"books", "definitions", "ebooks",
	}
	for _, d := range dirs {
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, d), 0755))
	}

	configContent := fmt.Sprintf(`notebooks:
  stories_directories:
    - %s
  learning_notes_directory: %s
  flashcards_directories:
    - %s
  books_directories:
    - %s
  definitions_directories:
    - %s
dictionaries:
  rapidapi:
    cache_directory: %s
outputs:
  story_directory: %s
  flashcard_directory: %s
books:
  repo_directory: %s
  repositories_file: %s
`,
		filepath.Join(tmpDir, "stories"),
		filepath.Join(tmpDir, "learning_notes"),
		filepath.Join(tmpDir, "flashcards"),
		filepath.Join(tmpDir, "books"),
		filepath.Join(tmpDir, "definitions"),
		filepath.Join(tmpDir, "dictionaries"),
		filepath.Join(tmpDir, "output_stories"),
		filepath.Join(tmpDir, "output_flashcards"),
		filepath.Join(tmpDir, "ebooks"),
		filepath.Join(tmpDir, "repos.yml"),
	)

	cfgPath := filepath.Join(tmpDir, "config.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(configContent), 0644))
	return cfgPath
}
