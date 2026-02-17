package main

import (
	"testing"

	"github.com/at-ishikawa/langner/internal/testutil"
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
}

func TestNewAnalyzeReportCommand_RunE(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "month without year",
			args:    []string{"--month", "3"},
			wantErr: "--month requires --year",
		},
		{
			name:    "invalid month too high",
			args:    []string{"--year", "2025", "--month", "13"},
			wantErr: "--month must be between 1 and 12",
		},
		{
			name:    "negative month",
			args:    []string{"--year", "2025", "--month", "-1"},
			wantErr: "--month must be between 1 and 12",
		},
		{
			name:    "invalid config",
			args:    []string{},
			wantErr: "configuration",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "invalid config" {
				cfgPath := setupBrokenConfigFile(t)
				setConfigFile(t, cfgPath)
			}
			cmd := newAnalyzeReportCommand()
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestNewAnalyzeReportCommand_RunE_WithConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := testutil.SetupTestConfig(t, tmpDir)
	setConfigFile(t, cfgPath)

	tests := []struct {
		name string
		args []string
	}{
		{name: "no args", args: []string{}},
		{name: "with year and month", args: []string{"--year", "2025", "--month", "6"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newAnalyzeReportCommand()
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			assert.NoError(t, err)
		})
	}
}
