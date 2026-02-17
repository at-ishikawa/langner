package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewAnalyzeCommand(t *testing.T) {
	cmd := newAnalyzeCommand()

	assert.Equal(t, "analyze", cmd.Use)
	assert.Equal(t, "Analyze learning progress and statistics", cmd.Short)
	assert.True(t, cmd.HasSubCommands())
}

func TestNewAnalyzeReportCommand(t *testing.T) {
	cmd := newAnalyzeReportCommand()

	assert.Equal(t, "report", cmd.Use)
	assert.Equal(t, "Show monthly/yearly report of learning statistics", cmd.Short)
	assert.NotNil(t, cmd.RunE)

	// Verify flags
	yearFlag := cmd.Flags().Lookup("year")
	assert.NotNil(t, yearFlag)
	assert.Equal(t, "0", yearFlag.DefValue)

	monthFlag := cmd.Flags().Lookup("month")
	assert.NotNil(t, monthFlag)
	assert.Equal(t, "0", monthFlag.DefValue)
}

func TestNewAnalyzeReportCommand_MonthWithoutYear(t *testing.T) {
	cmd := newAnalyzeReportCommand()
	cmd.SetArgs([]string{"--month", "3"})

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--month requires --year")
}

func TestNewAnalyzeReportCommand_InvalidMonth(t *testing.T) {
	cmd := newAnalyzeReportCommand()
	cmd.SetArgs([]string{"--year", "2025", "--month", "13"})

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--month must be between 1 and 12")
}

func TestNewAnalyzeReportCommand_RunE_WithConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newAnalyzeReportCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestNewAnalyzeReportCommand_RunE_WithYearMonth(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := setupTestConfigFile(t, tmpDir)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newAnalyzeReportCommand()
	cmd.SetArgs([]string{"--year", "2025", "--month", "6"})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestNewAnalyzeReportCommand_RunE_NegativeMonth(t *testing.T) {
	cmd := newAnalyzeReportCommand()
	cmd.SetArgs([]string{"--year", "2025", "--month", "-1"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--month must be between 1 and 12")
}

func TestNewAnalyzeReportCommand_RunE_InvalidConfig(t *testing.T) {
	cfgPath := setupBrokenConfigFile(t)
	oldConfigFile := configFile
	configFile = cfgPath
	defer func() { configFile = oldConfigFile }()

	cmd := newAnalyzeReportCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configuration")
}
