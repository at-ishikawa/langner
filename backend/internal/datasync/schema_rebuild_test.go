package datasync

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestManagedTablesForRebuildIsScopedAndComplete guards the from-scratch
// rebuild drop list (sync-db / reset-db). Two properties must hold together:
//
//  1. SCOPED — it contains ONLY langner-owned tables plus schema_migrations,
//     never any other database object. This is what lets the rebuild drop each
//     table individually instead of DROP SCHEMA, so it is safe on a shared
//     Supabase database (auth/storage and other apps are untouched).
//  2. COMPLETE — it drops every table ANY migration CREATEs, including a table
//     that a later migration dropped (relearn_clears: created by 017, dropped by
//     018). The migrations re-run from version 0 with plain CREATE TABLE (no IF
//     NOT EXISTS), so a leftover from an in-between version would collide unless
//     the rebuild dropped it first.
func TestManagedTablesForRebuildIsScopedAndComplete(t *testing.T) {
	rebuild := ManagedTablesForRebuild()

	// The allowlist of names the rebuild is permitted to contain: langner's
	// current tables + its legacy transient tables + schema_migrations. Nothing
	// else may appear (property 1 — scoped).
	allowed := make(map[string]bool)
	for _, name := range DataTablesInDependencyOrder() {
		allowed[name] = true
	}
	for _, name := range LegacyManagedTables() {
		allowed[name] = true
	}
	allowed["schema_migrations"] = true

	got := make(map[string]bool, len(rebuild))
	for _, name := range rebuild {
		assert.Truef(t, allowed[name],
			"rebuild drop list contains %q, which is NOT a langner-owned table or schema_migrations — "+
				"the scoped rebuild must never touch a non-langner object (it would be unsafe on a shared DB)", name)
		got[name] = true
	}

	// schema_migrations MUST be dropped so golang-migrate resets to version 0
	// and any DIRTY (half-applied) marker is cleared.
	assert.True(t, got["schema_migrations"],
		"rebuild drop list must include schema_migrations so a dirty/older migration state is reset")

	// Every current data table must be dropped (matches DataTablesInDependencyOrder).
	for _, name := range DataTablesInDependencyOrder() {
		assert.Truef(t, got[name], "rebuild drop list is missing current table %q", name)
	}

	// Legacy transient tables must NOT overlap the current schema — they are, by
	// definition, tables a later migration dropped.
	current := make(map[string]bool)
	for _, name := range DataTablesInDependencyOrder() {
		current[name] = true
	}
	for _, name := range LegacyManagedTables() {
		assert.Falsef(t, current[name],
			"legacy table %q is also a current table — LegacyManagedTables is only for tables a later migration DROPPED", name)
	}

	// Property 2 — complete: the rebuild must drop every table ANY up-migration
	// creates, whether or not a later migration dropped it, so re-migration from
	// scratch never collides with a leftover.
	created := allTablesEverCreatedByMigrations(t)
	for name := range created {
		if name == "schema_migrations" {
			continue // owned by the migration tool, recreated by Migrate
		}
		assert.Truef(t, got[name],
			"table %q is created by a migration but is not in the rebuild drop list — a database left at an "+
				"in-between version could still have it, and the plain CREATE TABLE on re-migration would collide. "+
				"Add it to LegacyManagedTables if a later migration dropped it, or to DataTablesInDependencyOrder otherwise", name)
	}
}

// allTablesEverCreatedByMigrations returns every table name a CREATE TABLE in an
// up-migration produces, WITHOUT subtracting later DROP TABLEs — the complete
// historical set, which is what the rebuild must be able to drop. (The sibling
// tablesFromMigrations in table_export_test.go subtracts drops to get the
// CURRENT schema; this one keeps them.)
func allTablesEverCreatedByMigrations(t *testing.T) map[string]bool {
	t.Helper()
	dir := findMigrationsDir(t)
	createPattern := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?` + "`?" + `(\w+)` + "`?")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	out := make(map[string]bool)
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		for _, m := range createPattern.FindAllStringSubmatch(string(body), -1) {
			out[m[1]] = true
		}
	}
	return out
}
