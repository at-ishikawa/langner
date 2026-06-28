package main

import (
	"net"
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/testutil"
	"github.com/stretchr/testify/assert"
)

// skipIfNoDB short-circuits tests that require a reachable MySQL.
// SetupTestConfig points the CLI at localhost:3306; in environments
// without MySQL (sandbox, CI without service container) the test would
// only ever exercise the connection-refused path.
func skipIfNoDB(t *testing.T) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", "127.0.0.1:3306", 200*time.Millisecond)
	if err != nil {
		t.Skip("MySQL not reachable on 127.0.0.1:3306 — skipping DB-backed CLI smoke test")
	}
	_ = conn.Close()
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
	skipIfNoDB(t)
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
