package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/datasync"
	"github.com/at-ishikawa/langner/schemas"
)

// TestTableDumpRoundTrip_LivePostgres_Integration is the lossless ROUND-TRIP
// proof for the complete per-table export, run against a real Postgres.
//
// Flow (per .claude/rules/verify-data-features-with-example-notebooks.md — it
// loads the SAME example notebooks through the SAME config.example.yml the
// running server uses, not a hand-built fixture):
//
//	config.example.yml YAML
//	  --import-db (real Importer)-->        DB   (every table populated)
//	  --export-db (TableDumpExporter)-->    dir A/tables/*.yml
//	  --clear + import (TableDumpImporter)->DB'  (restored verbatim)
//	  --export-db (TableDumpExporter)-->    dir B/tables/*.yml
//
// Then it asserts:
//  1. every table's dumped row count equals SELECT COUNT(*) — the export
//     captured every row of every table (nothing silently skipped);
//  2. dir A == dir B byte-for-byte for every table file — clear+restore
//     reproduced the database exactly, so the dump loses no column or row;
//  3. post-restore row counts equal pre-restore counts.
//
// Runs only when LANGNER_INTEGRATION_DB_URL is set (CI's postgres:16 service).
func TestTableDumpRoundTrip_LivePostgres_Integration(t *testing.T) {
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}
	ctx := context.Background()

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	require.NoError(t, db.Ping())

	// Fresh schema so the test is independent of ordering/leftover state.
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	// The example config's notebook paths are relative to the repo root
	// (where config.example.yml lives). Run from there so the reader resolves
	// examples/ correctly, exactly like `./langner ... --config config.example.yml`.
	repoRoot := findRepoRoot(t)
	restoreWD := chdir(t, repoRoot)
	defer restoreWD()

	cfg, err := config.NewConfigLoader("config.example.yml")
	require.NoError(t, err)
	loaded, err := cfg.Load()
	require.NoError(t, err)

	// Seed the DB across every content-bearing table via the REAL importer,
	// using the SAME options `langner migrate import-db` uses (all-false: no
	// dry-run, no update-existing) — the proven path that imports this exact
	// example config into a fresh DB cleanly. Seeding with UpdateExisting=true
	// instead tripped the notes (usage, entry) unique key on this data; that
	// import-mode quirk is orthogonal to what this round-trip test exercises
	// (the faithful table dump + verbatim restore), so we seed the canonical
	// way rather than probe it here.
	importer := newImporterFromConfig(loaded, db, io.Discard)
	_, err = importer.ImportAll(ctx, datasync.ImportOptions{})
	require.NoError(t, err)

	// Exercise the awkward columns the example may not populate:
	//   - a non-NULL skipped_at (nullable timestamp) on one note;
	//   - at least one row in note_images / note_references / etymology_origin_forms
	//     so all 14 tables carry data through the round trip.
	_, err = db.ExecContext(ctx, `UPDATE notes SET skipped_at = CURRENT_TIMESTAMP WHERE id = (SELECT MIN(id) FROM notes)`)
	require.NoError(t, err)
	seedIfEmpty(ctx, t, db, "note_images",
		`INSERT INTO note_images (note_id, url, sort_order) SELECT MIN(id), 'https://example.com/ice.png', 0 FROM notes`)
	seedIfEmpty(ctx, t, db, "note_references",
		`INSERT INTO note_references (note_id, link, description, sort_order) SELECT MIN(id), 'https://example.com/ref', 'reference', 0 FROM notes`)
	seedIfEmpty(ctx, t, db, "etymology_origin_forms",
		`INSERT INTO etymology_origin_forms (origin_id, form, role, note, sort_order) SELECT MIN(id), 'seed-form', 'variant', '', 0 FROM etymology_origins`)

	// Every table must actually carry rows so the round trip is meaningful —
	// especially the six the notebook-shaped ExportAll never exported.
	for _, table := range datasync.DataTablesInDependencyOrder() {
		assert.Positivef(t, countRows(ctx, t, db, table), "table %q must be populated for a meaningful round trip", table)
	}
	before := countByTable(ctx, t, db)

	// (1) Export A + assert every row was captured.
	dirA := t.TempDir()
	resA, err := datasync.NewTableDumpExporter(db, dirA, io.Discard).ExportTables(ctx)
	require.NoError(t, err)
	for table, dbCount := range before {
		assert.Equalf(t, dbCount, resA.RowsByTable[table],
			"exported row count for %q must equal SELECT COUNT(*) — the dump must capture every row", table)
	}

	// (2) Clear, restore from A, export B, and assert A == B byte-for-byte.
	require.NoError(t, clearAllDataTables(ctx, db))
	_, err = datasync.NewTableDumpImporter(db, dirA, io.Discard).ImportTables(ctx)
	require.NoError(t, err)

	dirB := t.TempDir()
	_, err = datasync.NewTableDumpExporter(db, dirB, io.Discard).ExportTables(ctx)
	require.NoError(t, err)

	for _, table := range datasync.DataTablesInDependencyOrder() {
		a, err := os.ReadFile(filepath.Join(dirA, "tables", table+".yml"))
		require.NoError(t, err)
		b, err := os.ReadFile(filepath.Join(dirB, "tables", table+".yml"))
		require.NoError(t, err)
		assert.Equalf(t, string(a), string(b),
			"round trip changed table %q — export or restore lost/altered data (this is the losslessness proof)", table)
	}

	// (3) Row counts survive the clear+restore.
	after := countByTable(ctx, t, db)
	assert.Equal(t, before, after, "row counts must be identical after clear + restore from the dump")
}

func seedIfEmpty(ctx context.Context, t *testing.T, db *sqlx.DB, table, insert string) {
	t.Helper()
	if countRows(ctx, t, db, table) > 0 {
		return
	}
	_, err := db.ExecContext(ctx, insert)
	require.NoErrorf(t, err, "seed %s", table)
}

func countRows(ctx context.Context, t *testing.T, db *sqlx.DB, table string) int {
	t.Helper()
	var n int
	// table is from the internal allowlist, never user input.
	require.NoError(t, db.GetContext(ctx, &n, "SELECT COUNT(*) FROM "+table)) //nolint:gosec // internal allowlist
	return n
}

func countByTable(ctx context.Context, t *testing.T, db *sqlx.DB) map[string]int {
	t.Helper()
	out := make(map[string]int)
	for _, table := range datasync.DataTablesInDependencyOrder() {
		out[table] = countRows(ctx, t, db, table)
	}
	return out
}

// findRepoRoot walks up from the test working directory to the directory that
// contains config.example.yml.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	for path := wd; path != "/" && path != "."; path = filepath.Dir(path) {
		if _, err := os.Stat(filepath.Join(path, "config.example.yml")); err == nil {
			return path
		}
	}
	t.Fatalf("config.example.yml not found above %s", wd)
	return ""
}

func chdir(t *testing.T, dir string) func() {
	t.Helper()
	old, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	return func() { _ = os.Chdir(old) }
}
