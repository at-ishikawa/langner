package notebook_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/schemas"
)

// openContentTestDB opens the integration database with the server's query
// mode (exec, text-format parameters) on a freshly migrated schema.
func openContentTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err := sqlx.Open("pgx", dsn+sep+"default_query_exec_mode=exec")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))
	return db
}

func storeContent(t *testing.T, db *sqlx.DB, contents []notebook.NotebookContent) {
	t.Helper()
	ctx := context.Background()
	repo := notebook.NewContentRepository(db)
	for _, nc := range contents {
		require.NoError(t, database.RunInTx(ctx, db, func(ctx context.Context, tx *sqlx.Tx) error {
			if _, err := tx.ExecContext(ctx, `INSERT INTO notebooks (notebook_id) VALUES ($1) ON CONFLICT DO NOTHING`, nc.ID); err != nil {
				return err
			}
			return repo.Store(ctx, tx, nc)
		}))
	}
}

// Every example notebook stored in Postgres reads back as the same content,
// and a Reader over Postgres reads the catalog the way the directory Reader
// does.
func TestContentRepository_StoreAndLoad_LivePostgres_Integration(t *testing.T) {
	db := openContentTestDB(t)
	ctx := context.Background()
	dirs := exampleDirs(t)
	contents, err := notebook.LoadDirectoryContent(dirs)
	require.NoError(t, err)
	storeContent(t, db, contents)
	// Storing again replaces the rows instead of failing on duplicates.
	storeContent(t, db, contents)

	repo := notebook.NewContentRepository(db)
	ids := make([]string, len(contents))
	for i, nc := range contents {
		ids[i] = nc.ID
	}
	loaded, err := repo.Load(ctx, ids)
	require.NoError(t, err)
	require.Len(t, loaded, len(contents))
	for i, nc := range contents {
		want, err := nc.Bundle()
		require.NoError(t, err)
		got, err := loaded[i].Bundle()
		require.NoError(t, err)
		require.Equal(t, len(want), len(got), nc.ID)
		for j := range want {
			assert.Equal(t, want[j].Family+"/"+want[j].Path, got[j].Family+"/"+got[j].Path)
			assert.Equal(t, string(want[j].Content), string(got[j].Content), "%s %s/%s", nc.ID, want[j].Family, want[j].Path)
		}
	}

	memCatalog, err := notebook.NewMemoryContent(contents).Catalog(ctx)
	require.NoError(t, err)
	pgCatalog, err := repo.Catalog(ctx)
	require.NoError(t, err)
	for i := range pgCatalog {
		require.NotEmpty(t, pgCatalog[i].Version)
		pgCatalog[i].Version = ""
	}
	assert.ElementsMatch(t, memCatalog, pgCatalog)

	fromDisk := newDirectoryReader(t, dirs)
	fromPG, err := notebook.NewContentReader(ctx, repo, nil)
	require.NoError(t, err)
	for _, id := range sortedKeys(fromDisk.GetStoryIndexes()) {
		want, err := fromDisk.ReadStoryNotebooks(id)
		require.NoError(t, err)
		got, err := fromPG.ReadStoryNotebooks(id)
		require.NoError(t, err)
		wantYAML, gotYAML := mustYAML(t, want), mustYAML(t, got)
		assert.Equal(t, wantYAML, gotYAML, id)
	}
	for _, id := range sortedKeys(fromDisk.GetEtymologyIndexes()) {
		want, err := fromDisk.ReadEtymologyNotebook(id)
		require.NoError(t, err)
		got, err := fromPG.ReadEtymologyNotebook(id)
		require.NoError(t, err)
		assert.Equal(t, want, got, id)
	}

	// Deleting the notebooks row removes its content.
	_, err = db.Exec(`DELETE FROM notebooks WHERE notebook_id = $1`, ids[0])
	require.NoError(t, err)
	var n int
	require.NoError(t, db.Get(&n, `SELECT count(*) FROM content_files WHERE notebook_id = $1`, ids[0]))
	assert.Zero(t, n)
}

// An outline load is the full load without story line text: each scene that
// has lines keeps one empty statement, everything else is identical.
func TestContentRepository_LoadOutline_LivePostgres_Integration(t *testing.T) {
	db := openContentTestDB(t)
	ctx := context.Background()
	contents, err := notebook.LoadDirectoryContent(exampleDirs(t))
	require.NoError(t, err)
	storeContent(t, db, contents)

	repo := notebook.NewContentRepository(db)
	ids := make([]string, len(contents))
	for i, nc := range contents {
		ids[i] = nc.ID
	}
	full, err := repo.Load(ctx, ids)
	require.NoError(t, err)
	outline, err := repo.LoadOutline(ctx, ids)
	require.NoError(t, err)

	scenesWithLines := 0
	for i := range full {
		for _, fc := range full[i].Families {
			for _, f := range fc.Files {
				for _, nb := range f.Stories {
					for j := range nb.Scenes {
						sc := &nb.Scenes[j]
						if len(sc.Statements) > 0 || len(sc.Conversations) > 0 {
							scenesWithLines++
							sc.Statements, sc.Conversations = []string{""}, nil
						}
					}
				}
			}
		}
	}
	require.Positive(t, scenesWithLines, "the examples must have story lines")
	assert.Equal(t, full, outline)

	var lines int
	require.NoError(t, db.Get(&lines, `SELECT count(*) FROM content_lines`))
	assert.Greater(t, lines, scenesWithLines, "the examples must have scenes with several lines")
}

// A word whose meaning comes from the dictionary (dictionary_number) resolves
// through the stored dictionary entries fetched with its notebook.
func TestContentReader_DictionaryWords_LivePostgres_Integration(t *testing.T) {
	db := openContentTestDB(t)
	ctx := context.Background()
	nc, err := notebook.ParseBundle("dictionary-words", []notebook.NotebookFile{
		{Family: notebook.FamilyFlashcards, Path: "index.yml", Content: []byte("id: dictionary-words\nname: Dictionary words\nnotebooks:\n  - ./cards.yml\n")},
		{Family: notebook.FamilyFlashcards, Path: "cards.yml", Content: []byte("- title: Words\n  cards:\n    - id: nothing\n      expression: nothing\n      dictionary_number: 1\n")},
	})
	require.NoError(t, err)
	storeContent(t, db, []notebook.NotebookContent{nc})
	_, err = db.Exec(`INSERT INTO dictionary_entries (word, source_type, response) VALUES ('nothing', 'rapidapi', $1),  ('unrelated', 'rapidapi', $1)`,
		`{"word":"nothing","results":[{"definition":"Something that has no existence","partOfSpeech":"noun"}]}`)
	require.NoError(t, err)

	r, err := notebook.NewContentReader(ctx, notebook.NewContentRepository(db), dictionary.NewDBDictionaryRepository(db))
	require.NoError(t, err)
	decks, err := r.ReadFlashcardNotebooks("dictionary-words")
	require.NoError(t, err)
	card := decks[0].Cards[0]
	require.NoError(t, card.SetDetails(r.Dictionary(), ""))
	assert.Equal(t, "Something that has no existence", card.Meaning)
	assert.Equal(t, []string{"nothing"}, sortedKeys(r.Dictionary()), "only the notebook's dictionary words are read")
}
