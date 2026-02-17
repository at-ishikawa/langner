package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewParseCommand(t *testing.T) {
	cmd := newParseCommand()

	assert.Equal(t, "parse", cmd.Use)
	assert.True(t, cmd.HasSubCommands())
}
