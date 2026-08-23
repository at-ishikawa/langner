package database

import (
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/schemas"
)

// openDriftTestDB opens the shared CI Postgres and rebuilds a FRESH public
// schema so this destructive test never sees rows other integration tests
// inserted. Each subtest rebuilds again, so ordering does not matter. Skipped
// unless LANGNER_INTEGRATION_DB_URL is set (CI's postgres:16).
func openDriftTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}
	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	return db
}

// TestVerifySchema_FreshMigrated_Passes is the happy path: a database created
// by the current migrations passes the pre-flight cleanly.
func TestVerifySchema_FreshMigrated_LivePostgres_Integration(t *testing.T) {
	db := openDriftTestDB(t)
	require.NoError(t, Migrate(db, schemas.Migrations, "migrations"))
	assert.NoError(t, VerifySchema(db, schemas.Migrations, "migrations"),
		"a schema built by the current migrations must pass the pre-flight check")
}

// TestVerifySchema_NeverMigrated_ClearError reproduces a target DB that has
// never had migrations run (no schema_migrations table). The pre-flight must
// return a clear, actionable error — NOT let a later scan crash first.
func TestVerifySchema_NeverMigrated_LivePostgres_Integration(t *testing.T) {
	db := openDriftTestDB(t)
	err := VerifySchema(db, schemas.Migrations, "migrations")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no schema_migrations table")
	assert.Contains(t, err.Error(), "fresh empty database")
}

// TestVerifySchema_MissingSenseID_ClearError reproduces bug #2's core: a DB
// whose schema_migrations reports the CURRENT version (so golang-migrate and
// the version check both think it is up to date) but whose notes table never
// got sense_id — exactly what a renumbered migration chain leaves behind. The
// pre-flight's explicit column check must catch it with a clear message naming
// sense_id, instead of the import crashing deep in a column scan.
func TestVerifySchema_MissingSenseID_LivePostgres_Integration(t *testing.T) {
	db := openDriftTestDB(t)
	require.NoError(t, Migrate(db, schemas.Migrations, "migrations"))

	// Simulate the renumbered-chain drift: the version stays at the max but
	// the sense_id column is gone (the migration that would have added it was
	// considered "already applied" and skipped). Dropping the dependent index
	// first mirrors a real rollback.
	_, err := db.Exec(`DROP INDEX IF EXISTS notes_sense_id_key`)
	require.NoError(t, err)
	_, err = db.Exec(`ALTER TABLE notes DROP COLUMN sense_id`)
	require.NoError(t, err)

	err = VerifySchema(db, schemas.Migrations, "migrations")
	require.Error(t, err, "a notes table missing sense_id must be rejected before import")
	assert.Contains(t, err.Error(), "sense_id")
	assert.Contains(t, err.Error(), "fresh empty database",
		"the error must tell the user what to do, not just what is wrong")
}

// TestVerifySchema_LegacyPartOfSpeech_ClearError reproduces the leftover
// part_of_speech column from the abandoned homograph approach. Even though the
// reads are now drift-resilient, its presence signals a pre-current schema, so
// the pre-flight flags it explicitly rather than importing silently.
func TestVerifySchema_LegacyPartOfSpeech_LivePostgres_Integration(t *testing.T) {
	db := openDriftTestDB(t)
	require.NoError(t, Migrate(db, schemas.Migrations, "migrations"))
	_, err := db.Exec(`ALTER TABLE notes ADD COLUMN part_of_speech TEXT`)
	require.NoError(t, err)

	err = VerifySchema(db, schemas.Migrations, "migrations")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "part_of_speech")
	assert.Contains(t, err.Error(), "fresh empty database")
}

// TestVerifySchema_StaleVersion_ClearError reproduces the schema_migrations
// version being behind the binary's expectation (an older migration chain).
func TestVerifySchema_StaleVersion_LivePostgres_Integration(t *testing.T) {
	db := openDriftTestDB(t)
	require.NoError(t, Migrate(db, schemas.Migrations, "migrations"))
	// Force the recorded version to a stale number without touching columns.
	_, err := db.Exec(`UPDATE schema_migrations SET version = 1, dirty = false`)
	require.NoError(t, err)

	err = VerifySchema(db, schemas.Migrations, "migrations")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migration version 1")
	assert.Contains(t, err.Error(), "fresh empty database")
}
