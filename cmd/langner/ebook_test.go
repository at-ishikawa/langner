package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/at-ishikawa/langner/internal/testutil"
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

func TestNewEbookListCommand_RunE(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := testutil.SetupTestConfig(t, tmpDir)
	setConfigFile(t, cfgPath)

	cmd := newEbookListCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestNewEbookListCommand_RunE_WithRepos(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := testutil.SetupTestConfig(t, tmpDir)
	setConfigFile(t, cfgPath)

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
	cfgPath := testutil.SetupTestConfig(t, tmpDir)
	setConfigFile(t, cfgPath)

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
	cfgPath := testutil.SetupTestConfig(t, tmpDir)
	setConfigFile(t, cfgPath)

	cmd := newEbookCloneCommand()
	cmd.SetArgs([]string{"not-a-valid-url"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestEbookCommands_InvalidConfig(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	setConfigFile(t, cfgPath)

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
