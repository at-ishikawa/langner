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

func TestNewValidateDBCommand(t *testing.T) {
	cmd := newValidateDBCommand()

	assert.Equal(t, "validate-db", cmd.Use)
	assert.NotNil(t, cmd.RunE)
	assert.Contains(t, cmd.Short, "read-only",
		"validate-db must advertise itself as read-only — the previous behaviour cleared every persisted-data table and silently destroyed any DB-only state, which a 'validate' command should never do")
}

func TestNewValidateDBCommand_RunE_ConfigError(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	setConfigFile(t, cfgPath)

	cmd := newValidateDBCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load config")
}

func TestNewSyncDBCommand(t *testing.T) {
	cmd := newSyncDBCommand()

	assert.Equal(t, "sync-db", cmd.Use)
	assert.NotNil(t, cmd.RunE)
	assert.Contains(t, cmd.Short, "destructive",
		"sync-db's Short MUST flag the destructive nature — the command clears every persisted-data table before re-importing, and an operator running it without that context could lose DB-only state without warning")
}

func TestNewSyncDBCommand_RunE_ConfigError(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	setConfigFile(t, cfgPath)

	cmd := newSyncDBCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load config")
}

