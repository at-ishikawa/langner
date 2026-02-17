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
