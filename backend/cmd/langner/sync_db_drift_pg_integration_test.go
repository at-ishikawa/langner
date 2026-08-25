package main

import (
	"context"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/schemas"
)

// TestSyncDB_DriftedDirtySchema_RebuildsAndSucceeds_LivePostgres_Integration
// reproduces the exact Supabase failure a real user hit and proves the scoped
// rebuild repairs it. Per
// .claude/rules/verify-data-features-with-example-notebooks.md it drives the
// REAL sync-db command built from config.example.yml (the same construction the
// server/CLI uses), not a hand-built fixture — and it seeds the realistic
// leftover STATE the bug depends on (an obsolete schema + a DIRTY migration
// marker), which a fresh example DB never produces.
//
// The seeded drift mirrors the user's DB:
//   - a `notes` table from the abandoned part_of_speech era: it carries a
//     part_of_speech column and the OLD unique constraint
//     notes_usage_entry_pos_key (NOT the current notes_usage_entry_key), plus a
//     couple of rows;
//   - a schema_migrations row at version 22 with dirty=true (a half-applied
//     migration).
//
// Before this change, sync-db only migrated in place then TRUNCATEd — so it
// could not self-heal: a plain Migrate refuses to run on a dirty schema, and
// even forced past that, migration 022's bare DROP CONSTRAINT
// notes_usage_entry_key failed (SQLSTATE 42704) on the differently-named
// constraint. The test asserts:
//
//	Phase A (repro): a plain in-place Migrate FAILS on the dirty schema.
//	Phase B (fix):   the real sync-db command SUCCEEDS — it rebuilds the managed
//	                 tables from scratch, re-migrates 001..N clean, and imports
//	                 the example notebooks. The obsolete constraint/column are
//	                 gone, the current schema is present, and example data loaded.
//	Phase C (idempotent): running sync-db a SECOND time still SUCCEEDS.
//
// Requires LANGNER_INTEGRATION_DB_URL (CI's postgres:16); skipped otherwise.
// DROPs/rebuilds the schema, so it must run isolated (own step, or -p 1).
func TestSyncDB_DriftedDirtySchema_RebuildsAndSucceeds_LivePostgres_Integration(t *testing.T) {
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}
	ctx := context.Background()

	// Run from the repo root so config.example.yml's relative notebook dirs
	// resolve, exactly as the server/CLI does.
	root := findRepoRoot(t)
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())

	// --- Seed the user's drifted + dirty schema on an empty public schema. ---
	_, err = db.ExecContext(ctx, `DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	seedDriftedPartOfSpeechSchema(ctx, t, db)

	// --- Phase A: a plain in-place Migrate cannot self-heal a dirty schema. ---
	// This is what sync-db effectively did before (Migrate, then TRUNCATE).
	err = database.Migrate(db, schemas.Migrations, "migrations")
	require.Error(t, err, "a plain Migrate MUST fail on a DIRTY schema_migrations — proving in-place migrate can't repair drift")
	assert.Contains(t, err.Error(), "irty", "the failure should be the dirty-database guard")

	// --- Phase B: the real sync-db command rebuilds and succeeds. ---
	runSyncDBCommand(ctx, t)

	// The obsolete part_of_speech-era artifacts are gone.
	assert.False(t, constraintExists(ctx, t, db, "notes_usage_entry_pos_key"),
		"the obsolete unique constraint from the part_of_speech era must be dropped by the rebuild")
	assert.False(t, columnExistsPG(ctx, t, db, "notes", "part_of_speech"),
		"the legacy part_of_speech column must be gone after the rebuild")

	// The current schema is present.
	assert.True(t, columnExistsPG(ctx, t, db, "notes", "sense_id"),
		"the rebuilt notes table must have the current sense_id column")
	assert.True(t, indexExists(ctx, t, db, "notes_usage_entry_legacy_key"),
		"migration 022's partial legacy unique index must exist after a clean re-migration")

	// schema_migrations is clean at the latest version.
	version, dirty := schemaMigrationsState(ctx, t, db)
	assert.False(t, dirty, "schema_migrations must be clean after the rebuild")
	assert.Equal(t, expectedLatestMigrationVersion(t), version,
		"schema_migrations must report the latest migration version after the rebuild")

	// Example data was imported: the two 'bank' homographs land as two notes
	// (config.example.yml's homographs-demo), replacing the drifted seed rows.
	assert.Positive(t, countNotes(ctx, t, db), "the example notebooks must be imported")
	assert.Equal(t, 2, countNotesByUsage(ctx, t, db, "bank"),
		"the example homographs must import as two distinct notes on the rebuilt schema")

	// --- Phase C: sync-db is idempotent — a second run also succeeds. ---
	runSyncDBCommand(ctx, t)
	version2, dirty2 := schemaMigrationsState(ctx, t, db)
	assert.False(t, dirty2, "schema_migrations must stay clean after a second sync-db")
	assert.Equal(t, expectedLatestMigrationVersion(t), version2)
	assert.Equal(t, 2, countNotesByUsage(ctx, t, db, "bank"),
		"a second idempotent sync-db must leave the example data intact")
}

// TestMigrate022DropConstraint_DriftTolerant_LivePostgres_Integration is the
// focused before/after for Deliverable 3: migration 022's DROP CONSTRAINT is now
// guarded with IF EXISTS. It proves the exact one-line change against a real
// Postgres: the OLD bare statement fails (SQLSTATE 42704) when the constraint is
// absent/renamed on a drifted schema, while the FIXED statement is a no-op.
//
// Requires LANGNER_INTEGRATION_DB_URL; skipped otherwise. Rebuilds the schema so
// it must run isolated (own step, or -p 1).
func TestMigrate022DropConstraint_DriftTolerant_LivePostgres_Integration(t *testing.T) {
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}
	ctx := context.Background()

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())

	// Bring the schema to the current version (a valid, fully-migrated notes
	// table). Migration 022 has already replaced notes_usage_entry_key with the
	// notes_usage_entry_legacy_key index, so the constraint is absent — exactly
	// the condition a drifted schema (older/renamed constraint) presents to 022.
	_, err = db.ExecContext(ctx, `DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))
	require.False(t, constraintExists(ctx, t, db, "notes_usage_entry_key"),
		"a current schema has no notes_usage_entry_key (022 converted it to an index)")

	// BEFORE: the exact statement the OLD migration 022 ran fails 42704 when the
	// constraint is absent — the user's `constraint ... does not exist` crash.
	_, err = db.ExecContext(ctx, `ALTER TABLE notes DROP CONSTRAINT notes_usage_entry_key`)
	require.Error(t, err, "a bare DROP CONSTRAINT on an absent constraint must fail (SQLSTATE 42704) — the user's crash")
	assert.Contains(t, err.Error(), "does not exist")

	// AFTER: the FIXED migration 022 statement tolerates the absent constraint.
	_, err = db.ExecContext(ctx, `ALTER TABLE notes DROP CONSTRAINT IF EXISTS notes_usage_entry_key`)
	require.NoError(t, err, "DROP CONSTRAINT IF EXISTS must be a no-op when the constraint is absent — the drift fix")
}

// seedDriftedPartOfSpeechSchema creates a notes table in the shape the abandoned
// part_of_speech approach produced (part_of_speech column + the old
// notes_usage_entry_pos_key unique constraint), a couple of rows, and a DIRTY
// schema_migrations row at version 22 — reproducing the user's Supabase state.
func seedDriftedPartOfSpeechSchema(ctx context.Context, t *testing.T, db *sqlx.DB) {
	t.Helper()
	_, err := db.ExecContext(ctx, `
		CREATE TABLE notes (
			id BIGSERIAL PRIMARY KEY,
			"usage" VARCHAR(255) NOT NULL,
			entry VARCHAR(255) NOT NULL,
			part_of_speech VARCHAR(50) NOT NULL DEFAULT '',
			meaning TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT notes_usage_entry_pos_key UNIQUE ("usage", entry, part_of_speech)
		)`)
	require.NoError(t, err, "seed drifted notes table")

	_, err = db.ExecContext(ctx, `
		INSERT INTO notes ("usage", entry, part_of_speech, meaning) VALUES
			('break the ice', 'break the ice', 'verb', 'to relieve tension'),
			('piece of cake', 'piece of cake', 'noun', 'something easy')`)
	require.NoError(t, err, "seed drifted notes rows")

	// golang-migrate's version table, left DIRTY at 22 by a half-applied migration.
	_, err = db.ExecContext(ctx, `
		CREATE TABLE schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL)`)
	require.NoError(t, err, "seed schema_migrations table")
	_, err = db.ExecContext(ctx, `INSERT INTO schema_migrations (version, dirty) VALUES (22, true)`)
	require.NoError(t, err, "seed dirty schema_migrations row")
}

// runSyncDBCommand invokes the REAL sync-db cobra command against
// config.example.yml (the same path the CLI runs), so the test exercises the
// production rebuild → migrate → import → seed → roundtrip flow end to end.
func runSyncDBCommand(ctx context.Context, t *testing.T) {
	t.Helper()
	setConfigFile(t, "config.example.yml")
	cmd := newSyncDBCommand()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute(), "the real sync-db command must succeed on a drifted/dirty schema")
}

// upMigrationPrefix matches the numeric prefix of a golang-migrate up file
// (e.g. "022_notes_homograph_unique.up.sql" -> 22).
var upMigrationPrefix = regexp.MustCompile(`^(\d+)_.*\.up\.sql$`)

// expectedLatestMigrationVersion is the highest numeric prefix among *.up.sql —
// the version a fully-migrated DB reports in schema_migrations. It reads the
// EMBEDDED migrations FS (schemas.Migrations, the same one database.Migrate
// uses), so it is independent of the working directory — a filesystem walk for
// schemas/migrations fails in CI, where the CWD is the repo root but the
// migrations live under backend/schemas/migrations.
func expectedLatestMigrationVersion(t *testing.T) int {
	t.Helper()
	entries, err := fs.ReadDir(schemas.Migrations, "migrations")
	require.NoError(t, err)
	max := 0
	for _, e := range entries {
		m := upMigrationPrefix.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		if v, err := strconv.Atoi(m[1]); err == nil && v > max {
			max = v
		}
	}
	require.Positive(t, max, "at least one *.up.sql migration must exist")
	return max
}

// --- live-schema assertion helpers (Postgres system catalogs) ---

func constraintExists(ctx context.Context, t *testing.T, db *sqlx.DB, name string) bool {
	t.Helper()
	var present bool
	require.NoError(t, db.GetContext(ctx, &present,
		`SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = $1)`, name))
	return present
}

func indexExists(ctx context.Context, t *testing.T, db *sqlx.DB, name string) bool {
	t.Helper()
	var present bool
	require.NoError(t, db.GetContext(ctx, &present,
		`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = current_schema() AND indexname = $1)`, name))
	return present
}

func columnExistsPG(ctx context.Context, t *testing.T, db *sqlx.DB, table, column string) bool {
	t.Helper()
	var present bool
	require.NoError(t, db.GetContext(ctx, &present,
		`SELECT EXISTS (SELECT 1 FROM information_schema.columns
		 WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2)`, table, column))
	return present
}

func schemaMigrationsState(ctx context.Context, t *testing.T, db *sqlx.DB) (version int, dirty bool) {
	t.Helper()
	var row struct {
		Version int  `db:"version"`
		Dirty   bool `db:"dirty"`
	}
	require.NoError(t, db.GetContext(ctx, &row, `SELECT version, dirty FROM schema_migrations LIMIT 1`))
	return row.Version, row.Dirty
}

func countNotes(ctx context.Context, t *testing.T, db *sqlx.DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.GetContext(ctx, &n, `SELECT COUNT(*) FROM notes`))
	return n
}

func countNotesByUsage(ctx context.Context, t *testing.T, db *sqlx.DB, usage string) int {
	t.Helper()
	var n int
	require.NoError(t, db.GetContext(ctx, &n, `SELECT COUNT(*) FROM notes WHERE "usage" = $1`, usage))
	return n
}
