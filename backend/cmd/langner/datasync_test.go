package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMigrateExportDBCommand(t *testing.T) {
	cmd := newMigrateExportDBCommand()

	assert.Equal(t, "export-db", cmd.Use)
	assert.Equal(t, "Export database data to YAML files", cmd.Short)
	assert.NotNil(t, cmd.RunE)

	flag := cmd.Flags().Lookup("output")
	assert.NotNil(t, flag)
	assert.Equal(t, "./export", flag.DefValue)
}

func TestNewMigrateExportDBCommand_RunE_configError(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	setConfigFile(t, cfgPath)

	cmd := newMigrateExportDBCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load config")
}

func TestNewMigrateImportDBCommand(t *testing.T) {
	cmd := newMigrateImportDBCommand()

	assert.Equal(t, "import-db", cmd.Use)
	assert.Equal(t, "Import notebook data into the database", cmd.Short)
	assert.NotNil(t, cmd.RunE)

	dryRunFlag := cmd.Flags().Lookup("dry-run")
	assert.NotNil(t, dryRunFlag)
	assert.Equal(t, "false", dryRunFlag.DefValue)

	updateFlag := cmd.Flags().Lookup("update-existing")
	assert.NotNil(t, updateFlag)
	assert.Equal(t, "false", updateFlag.DefValue)
}

func TestNewMigrateImportDBCommand_RunE_configError(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	setConfigFile(t, cfgPath)

	cmd := newMigrateImportDBCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load config")
}
