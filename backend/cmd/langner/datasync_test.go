package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestClearAllDataTablesCoversAllSchemaTables guards against the bug
// that previously broke `langner migrate validate-db`: the clear-list
// missed `note_origin_parts`, which has a FK from notes — running the
// clear hit `Error 1451 (23000): Cannot delete or update a parent row`
// before any actual validation happened. The existing sqlmock tests
// couldn't catch this because a mock happily accepts whatever DELETE
// strings are issued without enforcing the real schema's foreign keys.
//
// This test derives the expected table set from the source-of-truth
// schema (schemas/migrations/*.up.sql) and asserts the validate-db
// clear list covers it exactly. If a future migration adds a table,
// this test fails fast — flagging that dataTablesInDeletionOrder
// needs to grow with it (and prompting the author to think about
// where the new table sits in the FK dependency order).
//
// Tables explicitly skipped:
//   - schema_migrations: managed by the migration tool, not by the
//     importer. Clearing it would force every migration to re-run.
func TestClearAllDataTablesCoversAllSchemaTables(t *testing.T) {
	migrationsDir := findMigrationsDir(t)
	expected, err := tablesFromMigrations(migrationsDir)
	require.NoError(t, err)
	delete(expected, "schema_migrations")

	got := make(map[string]bool, len(dataTablesInDeletionOrder()))
	for _, name := range dataTablesInDeletionOrder() {
		got[name] = true
	}

	var missing []string
	for table := range expected {
		if !got[table] {
			missing = append(missing, table)
		}
	}
	var extra []string
	for table := range got {
		if !expected[table] {
			extra = append(extra, table)
		}
	}
	assert.Empty(t, missing,
		"dataTablesInDeletionOrder is missing tables defined by schemas/migrations — "+
			"the validate-db clear step will hit a FK constraint error before reaching the actual validation. "+
			"Add the missing tables to the list, in FK-dependency order (children before parents).")
	assert.Empty(t, extra,
		"dataTablesInDeletionOrder lists tables that no migration defines — either the migration was reverted "+
			"without updating this list, or the table name is misspelled.")
}

// tablesFromMigrations parses every up-migration in the given
// directory and returns the set of table names introduced by
// CREATE TABLE statements. The matcher is permissive: it accepts
// "CREATE TABLE", "CREATE TABLE IF NOT EXISTS", optional backticks,
// and the table name in either bare or quoted form. Down-migrations
// are ignored — they describe rollbacks, not the canonical schema.
func tablesFromMigrations(dir string) (map[string]bool, error) {
	pattern := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?` + "`?" + `(\w+)` + "`?")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		for _, m := range pattern.FindAllStringSubmatch(string(body), -1) {
			out[m[1]] = true
		}
	}
	return out, nil
}

// findMigrationsDir walks up from the test working directory until it
// finds schemas/migrations. The cmd/langner test runs inside the
// command package; the migrations directory sits two levels up.
func findMigrationsDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	for path := wd; path != "/" && path != "."; path = filepath.Dir(path) {
		candidate := filepath.Join(path, "schemas", "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	t.Fatalf("schemas/migrations directory not found above %s", wd)
	return ""
}
