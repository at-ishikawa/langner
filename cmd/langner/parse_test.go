package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewParseCommand(t *testing.T) {
	cmd := newParseCommand()

	assert.Equal(t, "parse", cmd.Use)
	assert.True(t, cmd.HasSubCommands())
}

func TestNewParseCommand_Friends_RunE(t *testing.T) {
	tmpDir := t.TempDir()
	inputFile := filepath.Join(tmpDir, "transcript.txt")
	content := "[Scene: Central Perk]\nRoss: Hi everyone.\nRachel: Hey Ross!\n\n[End of Scene]\nJoey: How you doing?\n"
	require.NoError(t, os.WriteFile(inputFile, []byte(content), 0644))

	cmd := newParseCommand()
	cmd.SetArgs([]string{"friends", inputFile})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestNewParseCommand_Friends_RunE_FileNotFound(t *testing.T) {
	cmd := newParseCommand()
	cmd.SetArgs([]string{"friends", "/nonexistent/file.txt"})
	err := cmd.Execute()
	assert.Error(t, err)
}
