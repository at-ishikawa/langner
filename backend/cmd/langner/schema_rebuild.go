package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/datasync"
)

// printPreflightBanner reports the resolved connection target and the schema
// state BEFORE any migration runs, and turns the two failure classes that
// otherwise surface as cryptic golang-migrate errors into one clear message up
// front:
//
//   - an empty search_path (current_schema() is NULL) — the symptom of
//     connecting through the Supabase transaction pooler (port 6543), where
//     golang-migrate's pgx init scans current_schema() into a plain string and
//     fails with "converting NULL to string";
//   - the count of langner-managed tables already present, so the operator can
//     see whether they are about to rebuild a populated or an empty database.
//
// It runs right after opening the DB and BEFORE Migrate so the real cause is
// printed before the deep, cryptic error. The queries are NULL-guarded
// (coalesce) so the banner itself never panics on the pooler connection.
func printPreflightBanner(ctx context.Context, cfg *config.Config, db *sqlx.DB, out io.Writer) error {
	var info struct {
		Schema     string `db:"cs"`
		SearchPath string `db:"sp"`
		Version    string `db:"ver"`
	}
	if err := db.GetContext(ctx, &info,
		`SELECT coalesce(current_schema(), '') AS cs,
		        coalesce(current_setting('search_path', true), '') AS sp,
		        version() AS ver`); err != nil {
		return fmt.Errorf("read connection preflight info: %w", err)
	}

	tableCount, err := managedTableCount(ctx, db)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "Connection preflight:")
	_, _ = fmt.Fprintf(out, "  host:            %s\n", cfg.Database.Host)
	_, _ = fmt.Fprintf(out, "  port:            %d\n", cfg.Database.Port)
	_, _ = fmt.Fprintf(out, "  database:        %s\n", cfg.Database.Database)
	_, _ = fmt.Fprintf(out, "  current_schema:  %s\n", orNone(info.Schema))
	_, _ = fmt.Fprintf(out, "  search_path:     %s\n", orNone(info.SearchPath))
	_, _ = fmt.Fprintf(out, "  server_version:  %s\n", info.Version)
	_, _ = fmt.Fprintf(out, "  langner tables:  %d already present\n", tableCount)

	if info.Schema == "" {
		return fmt.Errorf("empty search_path: current_schema() returned NULL, so schema migrations cannot resolve a target schema. " +
			"This happens on the Supabase transaction pooler (port 6543); connect through the session pooler or a direct connection (port 5432) instead")
	}
	return nil
}

// managedTableCount returns how many of langner's managed tables already exist
// in the current schema. It compares the live table list against the known
// allowlist (DataTablesInDependencyOrder) in Go so it needs no array binding
// and can never count a non-langner table.
func managedTableCount(ctx context.Context, db *sqlx.DB) (int, error) {
	var present []string
	if err := db.SelectContext(ctx, &present,
		`SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema()`); err != nil {
		return 0, fmt.Errorf("list existing tables: %w", err)
	}
	managed := make(map[string]bool)
	for _, name := range datasync.DataTablesInDependencyOrder() {
		managed[name] = true
	}
	count := 0
	for _, name := range present {
		if managed[name] {
			count++
		}
	}
	return count, nil
}

// rebuildManagedSchema drops every langner-managed table (children before
// parents, CASCADE) plus schema_migrations, leaving an empty table set for the
// subsequent Migrate to re-apply the whole migration chain onto. This is what
// makes sync-db / reset-db drift- and dirty-proof:
//
//   - a database built by an OLDER or renumbered migration chain (e.g. one
//     whose notes table still carries the abandoned part_of_speech unique
//     constraint) is reset to a clean slate the current migrations rebuild
//     deterministically, instead of a bare in-place Migrate failing on a
//     DROP CONSTRAINT that names a constraint the drifted schema spells
//     differently;
//   - dropping schema_migrations resets golang-migrate to version 0 and clears
//     any DIRTY (half-applied) marker, so the next Migrate re-runs 001..N
//     cleanly.
//
// It is SCOPED (datasync.ManagedTablesForRebuild is only langner's own tables +
// schema_migrations) and NEVER issues DROP SCHEMA, so it is safe on a shared
// database — Supabase's auth/storage schemas and any other application's tables
// are untouched. The one non-table object the migrations create, the
// set_updated_at() trigger function, is left in place: migration 001 (re)creates
// it with CREATE OR REPLACE FUNCTION, so re-migration is idempotent, and every
// trigger that uses it is dropped together with its table by DROP TABLE CASCADE.
func rebuildManagedSchema(ctx context.Context, db *sqlx.DB, out io.Writer) error {
	tables := datasync.ManagedTablesForRebuild()
	_, _ = fmt.Fprintf(out, "Rebuilding langner-managed tables from scratch (drop + migrate): %s\n", strings.Join(tables, ", "))
	for _, table := range tables {
		// table comes only from the hard-coded allowlist, never user input.
		stmt := fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", table) //nolint:gosec // table is an internal allowlist constant
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("drop table %s: %w", table, err)
		}
	}
	return nil
}

// orNone renders an empty schema/search_path value as "(none)" so the banner is
// readable when the pooler hands back an empty search_path.
func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
