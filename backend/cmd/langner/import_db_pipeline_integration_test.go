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

// findRepoRoot walks up from the test working directory until it finds
// config.example.yml — the root the running server is launched from. The
// example config's notebook directories are relative to that root, so the test
// must run with it as the working directory.
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

// TestImportDB_HomographsAndMultiNotebook_LivePostgres_Integration drives the
// FULL import-db pipeline the way the server / CLI does: it builds the Importer
// from config.example.yml (newImporterFromConfig — the same construction
// import-db uses) and imports the real example notebooks into a fresh migrated
// Postgres. Hand-built fixtures can't catch this because they bypass the real
// reader/importer construction (see
// .claude/rules/verify-data-features-with-example-notebooks.md).
//
// It asserts the two awkward conditions the example config now carries:
//
//   - HOMOGRAPHS: "bank" (bank-finance / bank-river) lands as TWO distinct
//     notes. Before migration 022 this crashed with `duplicate key value
//     violates unique constraint notes_usage_entry_key` — the exact user bug.
//   - MULTI-NOTEBOOK: "migrate" (id migrate-shared), declared in both
//     homographs-demo and roots-demo, DEDUPS to ONE note with two
//     notebook_notes (learning-history invariant L1/L4), never two copies.
//
// Requires LANGNER_INTEGRATION_DB_URL (CI's postgres:16); skipped otherwise.
// This test DROPs and recreates the public schema, so it must run isolated
// from other live-PG tests (its own workflow step, or -p 1).
func TestImportDB_HomographsAndMultiNotebook_LivePostgres_Integration(t *testing.T) {
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}

	// Run from the repo root so config.example.yml's relative notebook dirs
	// resolve, exactly as the server does.
	root := findRepoRoot(t)
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())

	// Fresh schema built by the current migrations, then verify it passes the
	// pre-flight check (Deliverable 1) before importing.
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))
	require.NoError(t, database.VerifySchema(db, schemas.Migrations, "migrations"),
		"a freshly-migrated schema must pass the import-db pre-flight check")

	loader, err := config.NewConfigLoader("config.example.yml")
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	importer := newImporterFromConfig(cfg, db, io.Discard)
	_, err = importer.ImportAll(context.Background(), datasync.ImportOptions{})
	require.NoError(t, err,
		"import-db must not crash on homographs (duplicate key) or multi-notebook words")

	ctx := context.Background()

	// HOMOGRAPHS: two distinct notes share the spelling "bank".
	var banks []struct {
		SenseID string `db:"sense_id"`
		Meaning string `db:"meaning"`
	}
	require.NoError(t, db.SelectContext(ctx, &banks,
		`SELECT sense_id, meaning FROM notes WHERE "usage" = 'bank' ORDER BY sense_id`))
	require.Len(t, banks, 2, "the two 'bank' homographs must import as two distinct notes")
	assert.Equal(t, "bank-finance", banks[0].SenseID)
	assert.Equal(t, "bank-river", banks[1].SenseID)
	assert.Contains(t, banks[0].Meaning, "money")
	assert.Contains(t, banks[1].Meaning, "river")

	// MULTI-NOTEBOOK: one note for migrate-shared, carrying two notebook_notes.
	var migrateNoteID int64
	require.NoError(t, db.GetContext(ctx, &migrateNoteID,
		`SELECT id FROM notes WHERE sense_id = 'migrate-shared'`),
		"the shared word must dedup to exactly one note (GetContext errors on 0 or >1 rows)")

	var nbIDs []string
	require.NoError(t, db.SelectContext(ctx, &nbIDs,
		`SELECT notebook_id FROM notebook_notes WHERE note_id = $1 ORDER BY notebook_id`, migrateNoteID))
	assert.Equal(t, []string{"homographs-demo", "roots-demo"}, nbIDs,
		"the one note must carry a notebook_note for EACH notebook that referenced it, never a parallel copy")
}

// TestImportDB_SharedSenseAcrossNotebooks_LivePostgres_Integration reproduces
// the SECOND real-DB import-db crash: a word claimed by two notebooks under one
// sense_id but with a DIFFERENT surface entry per notebook. The example books
// shared-sense-a and shared-sense-b both declare "hypersensitive" with id
// hypersensitive-shared; shared-sense-b's `definition:` normalizes the entry to
// "hyper-sensitive". The old (sense_id, usage, entry) dedup treated them as two
// notes, so ImportNotes' BatchCreate inserted two rows with one sense_id and
// Postgres rejected the second with `duplicate key value violates unique
// constraint notes_sense_id_key` (SQLSTATE 23505) — the user's
// `import notes: batch create notes: insert note: ...` error.
//
// Post-fix (CanonicalNoteKey folds id-bearing notes by sense_id on BOTH the
// reader and importer, and BatchCreate upserts idempotently on sense_id) the
// import succeeds and the word is ONE note carrying both notebook_notes.
//
// Requires LANGNER_INTEGRATION_DB_URL (CI's postgres:16); skipped otherwise.
// DROPs and recreates the public schema, so it must run isolated (own workflow
// step, or -p 1). Reproduces the 23505 on the pre-fix code; passes after.
func TestImportDB_SharedSenseAcrossNotebooks_LivePostgres_Integration(t *testing.T) {
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}

	root := findRepoRoot(t)
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())

	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	loader, err := config.NewConfigLoader("config.example.yml")
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	importer := newImporterFromConfig(cfg, db, io.Discard)
	_, err = importer.ImportAll(context.Background(), datasync.ImportOptions{})
	require.NoError(t, err,
		"import-db must not crash with notes_sense_id_key (23505) when one sense_id is claimed by two notebooks with different surface entries")

	ctx := context.Background()

	// Exactly ONE note for the shared sense_id.
	var noteID int64
	require.NoError(t, db.GetContext(ctx, &noteID,
		`SELECT id FROM notes WHERE sense_id = 'hypersensitive-shared'`),
		"the shared sense must be one note (GetContext errors on 0 or >1 rows)")

	// It carries a notebook_note for BOTH books that declared it.
	var nbIDs []string
	require.NoError(t, db.SelectContext(ctx, &nbIDs,
		`SELECT notebook_id FROM notebook_notes WHERE note_id = $1 ORDER BY notebook_id`, noteID))
	assert.Equal(t, []string{"shared-sense-a", "shared-sense-b"}, nbIDs,
		"the one note must carry both notebooks' memberships, not split into two rows")

	// Case 2: an id-LESS word ("estuary") declared in both shared-sense books
	// with the SAME meaning must DEDUP to one note (two notebook_notes), never
	// be rejected — only id-less words with DIFFERING meanings are rejected.
	var estuaryID int64
	require.NoError(t, db.GetContext(ctx, &estuaryID,
		`SELECT id FROM notes WHERE "usage" = 'estuary' AND sense_id = ''`),
		"the id-less word must dedup to exactly one note")
	var estuaryNBs []string
	require.NoError(t, db.SelectContext(ctx, &estuaryNBs,
		`SELECT notebook_id FROM notebook_notes WHERE note_id = $1 ORDER BY notebook_id`, estuaryID))
	assert.Equal(t, []string{"shared-sense-a", "shared-sense-b"}, estuaryNBs,
		"the identical id-less word dedups to one note carrying both memberships")
}
