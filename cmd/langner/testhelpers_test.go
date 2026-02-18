package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// setConfigFile sets the global configFile variable and registers a cleanup to restore it.
func setConfigFile(t *testing.T, cfgPath string) {
	t.Helper()
	oldConfigFile := configFile
	configFile = cfgPath
	t.Cleanup(func() { configFile = oldConfigFile })
}

// setupBrokenConfigFile creates a config file with invalid YAML that causes Load() to fail.
func setupBrokenConfigFile(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("{{invalid yaml content"), 0644))
	return cfgPath
}
