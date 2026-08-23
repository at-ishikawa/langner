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

	// This test CLEARS and RESTORES every table, so it must own the database.
	// In CI it runs in its OWN workflow step against a freshly-recreated
	// (empty, table-less) public schema — never sharing a `go test` invocation
	// with the quiz-integration packages, whose interleaved DROP SCHEMA /
	// TRUNCATE resets against the one shared Postgres corrupted this test's
	// state before (a stale-rows duplicate-key one run, a missing
	// schema_migrations the next). Reset here is therefore just:
	//   - database.Migrate: create schema_migrations + every table, and
	//   - clearAllDataTables: TRUNCATE ... RESTART IDENTITY CASCADE
	//     (belt-and-suspenders; a no-op on the already-empty fresh schema).
	// clearAllDataTables must NOT touch schema_migrations — it doesn't:
	// DataTablesInDependencyOrder excludes it (guarded by
	// TestClearAllDataTablesCoversAllSchemaTables).
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))
	require.NoError(t, clearAllDataTables(ctx, db))

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
	// with the same all-false options `langner migrate import-db` uses. On the
	// now-empty tables this creates the same 37 notes the CI import-db step
	// reports, exercising the full load path (per verify-data-features-with-
	// example-notebooks): notes, notebook_notes, learning_logs, dictionary,
	// and the etymology/semantic tables the notebook export never covered.
	importer := newImporterFromConfig(loaded, db, io.Discard)
	_, err = importer.ImportAll(ctx, datasync.ImportOptions{})
	require.NoError(t, err)

	// Seed the DB-only-state tables the same way `import-db` does (PR #26):
	// definitions_sessions/scenes, flashcard_decks, note/origin skip flags,
	// grammar_corrections + grammar logs. Runs after ImportAll so its FK
	// parents (notes, etymology_origins) already exist.
	if seeder := newStateSeederFromConfig(loaded, db, io.Discard); seeder != nil {
		_, err = seeder.SeedAll(ctx)
		require.NoError(t, err)
	}

	// Exercise the columns / tables the example config leaves empty so ALL 20
	// tables carry data through the round trip:
	//   - a non-NULL skipped_at (nullable timestamp) on one note;
	//   - note_images / note_references / etymology_origin_forms;
	//   - definition_concepts / definition_concept_members (no `concepts:`
	//     blocks in the example); and
	//   - the DB-only-state tables that stay empty on this example: the skip
	//     flags (nothing is excluded) and grammar_corrections + a grammar
	//     learning_log (config.example.yml has no `type: grammar` history), so
	//     learning_logs.correction_id round-trips non-NULL too.
	// Seed FK-safe: parents before children.
	_, err = db.ExecContext(ctx, `UPDATE notes SET skipped_at = CURRENT_TIMESTAMP WHERE id = (SELECT MIN(id) FROM notes)`)
	require.NoError(t, err)
	seedIfEmpty(ctx, t, db, "note_images",
		`INSERT INTO note_images (note_id, url, sort_order) SELECT MIN(id), 'https://example.com/ice.png', 0 FROM notes`)
	seedIfEmpty(ctx, t, db, "note_references",
		`INSERT INTO note_references (note_id, link, description, sort_order) SELECT MIN(id), 'https://example.com/ref', 'reference', 0 FROM notes`)
	seedIfEmpty(ctx, t, db, "etymology_origin_forms",
		`INSERT INTO etymology_origin_forms (origin_id, form, role, note, sort_order) SELECT MIN(id), 'seed-form', 'variant', '', 0 FROM etymology_origins`)
	seedIfEmpty(ctx, t, db, "definition_concepts",
		`INSERT INTO definition_concepts (notebook_id, head, meaning) VALUES ('roundtrip-seed', 'seed-head', 'seed meaning')`)
	seedIfEmpty(ctx, t, db, "definition_concept_members",
		`INSERT INTO definition_concept_members (concept_id, expression, session_title) SELECT MIN(id), 'seed-expression', '' FROM definition_concepts`)
	// definitions_sessions/scenes + flashcard_decks: normally seeded by
	// SeedAll from the example books; seed-if-empty is a safety net.
	seedIfEmpty(ctx, t, db, "definitions_sessions",
		`INSERT INTO definitions_sessions (notebook_id, title, notebook_file, sort_order) VALUES ('roundtrip-seed', 'seed session', '', 0)`)
	seedIfEmpty(ctx, t, db, "definitions_scenes",
		`INSERT INTO definitions_scenes (session_id, title, scene_index, sort_order) SELECT MIN(id), 'seed scene', 0, 0 FROM definitions_sessions`)
	seedIfEmpty(ctx, t, db, "flashcard_decks",
		`INSERT INTO flashcard_decks (notebook_id, title, description, sort_order) VALUES ('roundtrip-seed', 'seed deck', '', 0)`)
	seedIfEmpty(ctx, t, db, "note_skip_flags",
		`INSERT INTO note_skip_flags (note_id, quiz_type, skipped_at) SELECT MIN(id), 'notebook', CURRENT_TIMESTAMP FROM notes`)
	seedIfEmpty(ctx, t, db, "origin_skip_flags",
		`INSERT INTO origin_skip_flags (origin_id, quiz_type, skipped_at) SELECT MIN(id), 'etymology_origin', CURRENT_TIMESTAMP FROM etymology_origins`)
	seedIfEmpty(ctx, t, db, "grammar_corrections",
		`INSERT INTO grammar_corrections (notebook_id, sense_id) VALUES ('roundtrip-seed', 'seed-correction')`)
	// A grammar learning_log (note_id NULL, correction_id set) so the new
	// learning_logs.correction_id column carries a non-NULL value through the
	// dump -> restore -> dump round trip.
	_, err = db.ExecContext(ctx,
		`INSERT INTO learning_logs (correction_id, status, learned_at, quiz_type, source_notebook_id)
		 SELECT MIN(id), 'understood', CURRENT_TIMESTAMP, 'grammar', 'roundtrip-seed' FROM grammar_corrections`)
	require.NoError(t, err)

	// Every table must actually carry rows so the round trip is meaningful —
	// especially the tables the notebook-shaped ExportAll never exported.
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

// findRepoRoot is shared with import_db_pipeline_integration_test.go (same
// package): it walks up from the test working directory to the directory that
// contains config.example.yml.

func chdir(t *testing.T, dir string) func() {
	t.Helper()
	old, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	return func() { _ = os.Chdir(old) }
}
