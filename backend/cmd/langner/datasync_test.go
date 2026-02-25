package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewExportDBCommand(t *testing.T) {
	cmd := newExportDBCommand()

	assert.Equal(t, "export-db", cmd.Use)
	assert.Equal(t, "Export database to YAML files", cmd.Short)
	assert.NotNil(t, cmd.RunE)

	// Verify required flag
	flag := cmd.Flags().Lookup("output")
	assert.NotNil(t, flag)
}

func TestNewExportDBCommand_RunE_ConfigError(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	setConfigFile(t, cfgPath)

	cmd := newExportDBCommand()
	cmd.SetArgs([]string{"--output", t.TempDir()})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load config")
}

func TestNewMigrateImportDBCommand(t *testing.T) {
	cmd := newMigrateImportDBCommand()

	assert.Equal(t, "import-db", cmd.Use)
	assert.Equal(t, "Import notebook data into the database", cmd.Short)
	assert.NotNil(t, cmd.RunE)
}

func TestNewMigrateImportDBCommand_RunE_ConfigError(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	setConfigFile(t, cfgPath)

	cmd := newMigrateImportDBCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load config")
}
