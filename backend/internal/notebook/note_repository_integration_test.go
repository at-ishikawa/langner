package notebook

import (
	"context"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/schemas"
)

// TestDBNoteRepository_FindAll_SchemaDrift_LivePostgres_Integration reproduces
// the bug the user hit running `langner migrate import-db` against their real
// Postgres: their notes table carried a LEFTOVER part_of_speech column (from
// the abandoned homograph approach, replaced by sense_id) that no current
// migration creates. NoteRecord.PartOfSpeech is db:"-" (YAML-only), so a plain
// SELECT * scanned a column with no destination and sqlx crashed with
// "missing destination name part_of_speech". The reconcile reads now name their
// columns explicitly, so an extra/unknown column is simply not fetched.
//
// This is exactly the "seed the leftover state a removed feature leaves behind"
// case: a fresh exact-migrations schema (all CI ever ran) never has the drift,
// so only a test that ADDS the stray column can catch it. Requires
// LANGNER_INTEGRATION_DB_URL (CI's postgres:16); skipped otherwise. Fails
// before the explicit-column fix, passes after.
func TestDBNoteRepository_FindAll_SchemaDrift_LivePostgres_Integration(t *testing.T) {
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.Ping())

	// Fresh schema, then apply migrations.
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	// Simulate the user's real-DB drift: columns that no current migration
	// creates and that no struct field maps. A plain SELECT * would crash on
	// any of these. note_images / notebook_notes are drifted too so the
	// loadRelations reads are covered, not just the notes read.
	for _, ddl := range []string{
		`ALTER TABLE notes ADD COLUMN part_of_speech TEXT`,
		`ALTER TABLE note_images ADD COLUMN legacy_flag BOOLEAN`,
		`ALTER TABLE notebook_notes ADD COLUMN legacy_rank INT`,
	} {
		_, err = db.Exec(ddl)
		require.NoError(t, err, "drift DDL: %s", ddl)
	}

	// Insert one note with a notebook_note and an image so FindAll must scan
	// all three drifted tables.
	var noteID int64
	require.NoError(t, db.Get(&noteID,
		`INSERT INTO notes ("usage", entry, meaning, part_of_speech) VALUES ($1, $2, $3, $4) RETURNING id`,
		"break the ice", "break the ice", "to start a conversation", "phrase"))
	_, err = db.Exec(
		`INSERT INTO notebook_notes (note_id, notebook_type, notebook_id, "group", subgroup) VALUES ($1, $2, $3, $4, $5)`,
		noteID, "flashcard", "idioms", "Common Idioms", "")
	require.NoError(t, err)
	_, err = db.Exec(
		`INSERT INTO note_images (note_id, url, sort_order) VALUES ($1, $2, $3)`,
		noteID, "https://example.com/ice.png", 0)
	require.NoError(t, err)

	repo := NewDBNoteRepository(db)

	// FindAll must succeed despite the stray columns and return the note with
	// its relations fully populated.
	notes, err := repo.FindAll(context.Background())
	require.NoError(t, err, "FindAll must tolerate an unknown/extra column on a drifted DB")
	require.Len(t, notes, 1)
	assert.Equal(t, "break the ice", notes[0].Usage)
	assert.Equal(t, "break the ice", notes[0].Entry)
	require.Len(t, notes[0].NotebookNotes, 1)
	assert.Equal(t, "idioms", notes[0].NotebookNotes[0].NotebookID)
	require.Len(t, notes[0].Images, 1)
	assert.Equal(t, "https://example.com/ice.png", notes[0].Images[0].URL)
	// The drifted column is simply not read; the mapped fields are intact.
	assert.Empty(t, notes[0].PartOfSpeech, "part_of_speech is db:\"-\", never scanned from the DB")

	// FindByID must be drift-resilient too (same SELECT).
	one, err := repo.FindByID(context.Background(), noteID)
	require.NoError(t, err)
	assert.Equal(t, "break the ice", one.Usage)
}
