package clicmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/schemas"
)

// These tests pin that a notebook's content rows always match its stored
// bundle: the example flashcard notebook (examples/flashcards/vocabulary, loaded
// through config.example.yml) is stored first as an earlier edition without
// `origin:` (v1) and then as the shipped edition, where some cards carry an
// origin (v2). Whatever path stored v2, the quiz must read the origins.

const vocabularyID = "vocabulary"

var originLine = regexp.MustCompile(`(?m)^\s*origin:.*\n`)

// contentSyncFixture is a fresh database plus a config whose only notebook is a
// copy of the example vocabulary notebook that the test rewrites.
type contentSyncFixture struct {
	db  *sqlx.DB
	cfg *config.Config
	dir string // the vocabulary notebook dir
	v1  []notebook.NotebookFile
	v2  []notebook.NotebookFile
}

func newContentSyncFixture(t *testing.T, migrateTo uint) contentSyncFixture {
	t.Helper()
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
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	if migrateTo == 0 {
		require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))
	} else {
		require.NoError(t, database.MigrateTo(db, schemas.Migrations, "migrations", migrateTo))
	}

	loader, err := config.NewConfigLoader("config.example.yml")
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	flashcards := t.TempDir()
	dir := filepath.Join(flashcards, vocabularyID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	c := *cfg
	c.Notebooks = config.NotebooksConfig{FlashcardsDirectories: []string{flashcards}}

	f := contentSyncFixture{db: db, cfg: &c, dir: dir}
	for _, name := range []string{"index.yml", "cards.yml"} {
		content, err := os.ReadFile(filepath.Join(root, "examples", "flashcards", vocabularyID, name))
		require.NoError(t, err)
		f.v2 = append(f.v2, notebook.NotebookFile{NotebookID: vocabularyID, Family: notebook.FamilyFlashcards, Path: name, Content: content})
		f.v1 = append(f.v1, notebook.NotebookFile{NotebookID: vocabularyID, Family: notebook.FamilyFlashcards, Path: name, Content: originLine.ReplaceAll(content, nil)})
	}
	require.NotEqual(t, notebook.HashBundle(f.v1), notebook.HashBundle(f.v2))
	return f
}

// writeEdition puts an edition of the notebook on disk, as the import reads it.
func (f contentSyncFixture) writeEdition(t *testing.T, files []notebook.NotebookFile) {
	t.Helper()
	for _, file := range files {
		require.NoError(t, os.WriteFile(filepath.Join(f.dir, file.Path), file.Content, 0o644))
	}
}

// sync runs the catalog import and returns how many notebooks it updated.
func (f contentSyncFixture) sync(t *testing.T, force ...string) int {
	t.Helper()
	n, err := ImportShippedContent(context.Background(), f.cfg, f.db, io.Discard, force...)
	require.NoError(t, err)
	return n
}

// origins returns the stored origin of every card with one, by expression.
func (f contentSyncFixture) origins(t *testing.T) map[string]string {
	t.Helper()
	var rows []struct {
		Expression string `db:"expression"`
		Origin     string `db:"origin"`
	}
	require.NoError(t, f.db.Select(&rows,
		`SELECT expression, origin FROM content_notes WHERE notebook_id = $1 AND origin <> ''`, vocabularyID))
	out := map[string]string{}
	for _, r := range rows {
		out[r.Expression] = r.Origin
	}
	return out
}

// wantOrigins is what v2 declares.
var wantOrigins = map[string]string{
	"serendipity": "coined by Horace Walpole in 1754, from the Persian fairy tale 'The Three Princes of Serendip'",
	"ameliorate":  "from Latin 'melior' meaning 'better'",
}

// noteIDs is the set of notes rows; rebuilding content must keep it.
func (f contentSyncFixture) noteIDs(t *testing.T) []int64 {
	t.Helper()
	var ids []int64
	require.NoError(t, f.db.Select(&ids, `SELECT id FROM notes ORDER BY id`))
	return ids
}

// writeLegacyHash records a bundle's hash on the notebooks row without
// rebuilding its content rows, as servers before migration 038 did when they
// stored a push as raw files only.
func (f contentSyncFixture) writeLegacyHash(t *testing.T, files []notebook.NotebookFile) {
	t.Helper()
	_, err := f.db.Exec(`UPDATE notebooks SET content_hash = $2 WHERE notebook_id = $1`, vocabularyID, notebook.HashBundle(files))
	require.NoError(t, err)
}

func TestImportShippedContent_RebuildsChangedNotebook_LivePostgres_Integration(t *testing.T) {
	f := newContentSyncFixture(t, 0)

	f.writeEdition(t, f.v1)
	assert.Equal(t, 1, f.sync(t))
	assert.Empty(t, f.origins(t))
	notes := f.noteIDs(t)
	require.NotEmpty(t, notes)

	f.writeEdition(t, f.v2)
	assert.Equal(t, 1, f.sync(t), "a changed bundle is updated")
	assert.Equal(t, wantOrigins, f.origins(t))
	assert.Equal(t, notes, f.noteIDs(t), "rebuilding content keeps the notes")

	// Unchanged: nothing is reported and no row is rewritten.
	type version struct {
		Rows      string `db:"rows"`
		UpdatedAt string `db:"updated_at"`
	}
	read := func() version {
		var v version
		require.NoError(t, f.db.Get(&v,
			`SELECT (SELECT string_agg(xmin::text, ',' ORDER BY file_no, entry_no, scene_no, note_no) FROM content_notes WHERE notebook_id = $1) AS rows,
			        updated_at::text AS updated_at
			 FROM notebooks WHERE notebook_id = $1`, vocabularyID))
		return v
	}
	before := read()
	assert.Equal(t, 0, f.sync(t), "an unchanged notebook is skipped")
	assert.Equal(t, before, read(), "an unchanged notebook's rows are not rewritten")

	// --force rebuilds a named notebook even when unchanged.
	assert.Equal(t, 1, f.sync(t, vocabularyID))
	assert.NotEqual(t, before.Rows, read().Rows)
	assert.Equal(t, wantOrigins, f.origins(t))
}

// A server that predates the content tables stores v2 as files + hash only,
// after the rows were built from v1. The next sync must not take the matching
// hash as "unchanged".
func TestImportShippedContent_RebuildsRowsBehindLegacyFileWrite_LivePostgres_Integration(t *testing.T) {
	f := newContentSyncFixture(t, 0)

	f.writeEdition(t, f.v1)
	assert.Equal(t, 1, f.sync(t))
	f.writeLegacyHash(t, f.v2)
	assert.Empty(t, f.origins(t), "precondition: rows are still v1")

	f.writeEdition(t, f.v2)
	assert.Equal(t, 1, f.sync(t))
	assert.Equal(t, wantOrigins, f.origins(t))
	assert.Equal(t, 0, f.sync(t))
}
