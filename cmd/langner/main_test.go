package main

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetupLogger(t *testing.T) {
	tests := []struct {
		name      string
		debugMode bool
		wantLevel slog.Level
	}{
		{
			name:      "debug mode enabled",
			debugMode: true,
			wantLevel: slog.LevelDebug,
		},
		{
			name:      "debug mode disabled",
			debugMode: false,
			wantLevel: slog.LevelInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupLogger(tt.debugMode)
			// Verify the logger was set (no panic)
			logger := slog.Default()
			assert.NotNil(t, logger)
			assert.Equal(t, tt.wantLevel <= slog.LevelDebug, logger.Enabled(nil, slog.LevelDebug))
		})
	}
}

func TestNewMigrateCommand(t *testing.T) {
	cmd := newMigrateCommand()

	assert.Equal(t, "migrate", cmd.Use)
	assert.Equal(t, "Migration commands", cmd.Short)
	assert.True(t, cmd.HasSubCommands())
}

func TestNewMigrateLearningHistoryCommand(t *testing.T) {
	cmd := newMigrateLearningHistoryCommand()

	assert.Equal(t, "learning-history", cmd.Use)
	assert.Equal(t, "Migrate learning history files to new SM-2 format", cmd.Short)
	assert.NotNil(t, cmd.RunE)
}
