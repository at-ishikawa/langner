package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/schemas"
)

// Migration 041 makes Project Gutenberg books per-user library books and keeps
// every existing book with the users who already have it: whoever highlighted
// or studied a book keeps it, and a book nobody used stays with every user.
// Other catalog notebooks are unchanged.
func TestUserLibraryMigration_KeepsExistingBooks_LivePostgres_Integration(t *testing.T) {
	db := openDriftTestDB(t)
	require.NoError(t, MigrateTo(db, schemas.Migrations, "migrations", 40))

	var reader, student, other int64
	for _, u := range []struct {
		sub string
		id  *int64
	}{{"reader", &reader}, {"student", &student}, {"other", &other}} {
		require.NoError(t, db.Get(u.id, `INSERT INTO users (google_sub, username) VALUES ($1, $1) RETURNING id`, u.sub))
	}
	_, err := db.Exec(`INSERT INTO notebooks (notebook_id) VALUES ('gutenberg-1'), ('gutenberg-2'), ('gutenberg-3'), ('idioms-book')`)
	require.NoError(t, err)
	var noteID int64
	require.NoError(t, db.Get(&noteID, `INSERT INTO notes ("usage", entry, meaning) VALUES ('break the ice', 'break the ice', 'start a conversation') RETURNING id`))
	_, err = db.Exec(`INSERT INTO user_notebook_definitions (user_id, notebook_id, note_id) VALUES ($1, 'gutenberg-1', $2)`, reader, noteID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO learning_logs (user_id, note_id, status, learned_at, quiz_type, source_notebook_id)
		VALUES ($1, $2, 'understood', now(), 'notebook', 'gutenberg-2')`, student, noteID)
	require.NoError(t, err)

	require.NoError(t, Migrate(db, schemas.Migrations, "migrations"))

	sources := map[string]string{}
	var rows []struct {
		NotebookID string `db:"notebook_id"`
		Source     string `db:"source"`
	}
	require.NoError(t, db.Select(&rows, `SELECT notebook_id, source FROM notebooks`))
	for _, r := range rows {
		sources[r.NotebookID] = r.Source
	}
	assert.Equal(t, map[string]string{
		"gutenberg-1": "library", "gutenberg-2": "library", "gutenberg-3": "library", "idioms-book": "shipped",
	}, sources)

	library := func(notebookID string) []int64 {
		var ids []int64
		require.NoError(t, db.Select(&ids, `SELECT user_id FROM user_library WHERE notebook_id = $1 ORDER BY user_id`, notebookID))
		return ids
	}
	assert.Equal(t, []int64{reader}, library("gutenberg-1"), "a highlighted book stays with who highlighted it")
	assert.Equal(t, []int64{student}, library("gutenberg-2"), "a studied book stays with who studied it")
	assert.Equal(t, []int64{reader, student, other}, library("gutenberg-3"), "an unused book stays with every user")
	assert.Empty(t, library("idioms-book"))
}
