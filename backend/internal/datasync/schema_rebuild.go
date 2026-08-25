package datasync

// LegacyManagedTables lists langner tables that a PAST migration created but a
// LATER migration dropped, so they are NOT part of the current schema (they are
// intentionally absent from DataTablesInDependencyOrder, whose completeness
// guard subtracts every dropped table). They may still physically exist on a
// database whose migration history stopped at an in-between version.
//
// The from-scratch rebuild (sync-db / reset-db) MUST drop them too: the
// migrations re-run from version 0 and every CREATE TABLE is a plain
// CREATE TABLE (no IF NOT EXISTS), so a leftover from an older version would
// collide with the re-migration. Dropping them IF EXISTS is a no-op on a
// database that never had them.
//
// relearn_clears: created by migration 017, dropped by 018.
func LegacyManagedTables() []string {
	return []string{"relearn_clears"}
}

// ManagedTablesForRebuild returns every langner-owned table to DROP before a
// from-scratch re-migration, ordered children-before-parents (FK-safe), then
// the legacy transient tables, then schema_migrations last.
//
// It is deliberately SCOPED: the list is built only from langner's own known
// tables (DataTablesInDependencyOrder + LegacyManagedTables) plus
// schema_migrations. It NEVER contains any other database object, so the
// rebuild can drop each table individually (DROP TABLE IF EXISTS ... CASCADE)
// without ever issuing DROP SCHEMA — safe on a shared database (Supabase
// auth/storage, other applications' tables are untouched).
//
// schema_migrations is dropped so golang-migrate resets to version 0 and any
// DIRTY (half-applied) state is cleared; the following Migrate then re-applies
// the whole chain onto an empty table set.
func ManagedTablesForRebuild() []string {
	data := DataTablesInDependencyOrder()
	legacy := LegacyManagedTables()
	out := make([]string, 0, len(data)+len(legacy)+1)
	out = append(out, data...)
	out = append(out, legacy...)
	out = append(out, "schema_migrations")
	return out
}
