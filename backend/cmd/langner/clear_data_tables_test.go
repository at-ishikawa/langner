package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/datasync"
)

// dataTablesInDeletionOrder is the persisted-data table list (children before
// parents, FK-safe) shared by the round-trip and guard tests. Production sync-db
// / reset-db no longer TRUNCATE — they DROP + re-migrate the managed tables
// (see rebuildManagedSchema) — so this clear primitive is now used only by the
// table-dump round-trip test, which must empty the tables in place (keeping the
// schema) so it can restore them verbatim.
func dataTablesInDeletionOrder() []string {
	return datasync.DataTablesInDependencyOrder()
}

// clearAllDataTables wipes every persisted-data table in one TRUNCATE.
// CASCADE truncates dependent-referenced rows so the FK graph doesn't have to be
// walked in a specific order; RESTART IDENTITY resets the BIGSERIAL sequences so
// restored rows get the same IDs. It deliberately does NOT touch
// schema_migrations (DataTablesInDependencyOrder excludes it), so the round-trip
// test keeps its migrated schema.
func clearAllDataTables(ctx context.Context, db *sqlx.DB) error {
	tables := dataTablesInDeletionOrder()
	sql := "TRUNCATE " + strings.Join(tables, ", ") + " RESTART IDENTITY CASCADE"
	if _, err := db.ExecContext(ctx, sql); err != nil {
		return fmt.Errorf("truncate data tables: %w", err)
	}
	return nil
}
