package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
