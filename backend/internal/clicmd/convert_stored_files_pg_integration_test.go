package clicmd

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/schemas"
)

// TestConvertStoredFiles_LivePostgres_Integration recreates a database as the
// previous release left it — migration 036, every notebook stored as raw YAML
// files in notebook_files — with the example catalog (including a composite
// definitions + etymology notebook) plus a learner's private pushed notebook
// and a file that doesn't parse. It then runs the upgrade (`migrate schema`:
// migrations + conversion) and checks every notebook reads back from the
// content tables exactly as its files parse, with its registry row kept.
func TestConvertStoredFiles_LivePostgres_Integration(t *testing.T) {
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
	require.NoError(t, database.MigrateTo(db, schemas.Migrations, "migrations", 36))

	loader, err := config.NewConfigLoader("config.example.yml")
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)
	bundles, err := notebook.CollectBundles(configContentDirs(cfg))
	require.NoError(t, err)

	ctx := context.Background()
	var ownerID int64
	require.NoError(t, db.GetContext(ctx, &ownerID,
		`INSERT INTO users (google_sub, username) VALUES ('convert-test', 'convert-test') RETURNING id`))

	// The old storage: a registry row plus one notebook_files row per file.
	storeOld := func(id, source, visibility string, owner *int64, files []notebook.NotebookFile) {
		_, err := db.ExecContext(ctx,
			`INSERT INTO notebooks (notebook_id, owner_user_id, visibility, source, kind, display_name, content_hash)
			 VALUES ($1, $2, $3, $4, 'composite', $5, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')`, id, owner, visibility, source, id)
		require.NoError(t, err)
		for _, f := range files {
			_, err := db.ExecContext(ctx,
				`INSERT INTO notebook_files (notebook_id, family, path, content, content_sha256, byte_size)
				 VALUES ($1, $2, $3, $4, 'sha', $5)`, id, f.Family, f.Path, f.Content, len(f.Content))
			require.NoError(t, err)
		}
	}
	want := map[string]notebook.NotebookContent{}
	for _, b := range bundles {
		storeOld(b.ID, "shipped", "public", nil, b.Files)
		nc, err := notebook.ParseBundle(b.ID, b.Files)
		require.NoError(t, err)
		want[b.ID] = nc
	}
	private := []notebook.NotebookFile{
		{Family: notebook.FamilyFlashcards, Path: "index.yml", Content: []byte("id: nb_private\nname: Private idioms\nnotebooks:\n  - ./cards.yml\n")},
		{Family: notebook.FamilyFlashcards, Path: "cards.yml", Content: []byte("- title: Idioms\n  cards:\n    - id: break-the-ice\n      expression: break the ice\n      meaning: to start a conversation\n")},
	}
	storeOld("nb_private", "user", "private", &ownerID, private)
	nc, err := notebook.ParseBundle("nb_private", private)
	require.NoError(t, err)
	want["nb_private"] = nc
	storeOld("nb_broken", "user", "private", &ownerID, []notebook.NotebookFile{
		{Family: notebook.FamilyFlashcards, Path: "index.yml", Content: []byte("id: nb_broken\nnotebooks:\n  - ./missing.yml\n")},
	})

	// The upgrade: what `migrate schema` runs.
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))
	n, err := ConvertStoredFiles(ctx, db, io.Discard)
	require.Error(t, err, "the notebook that doesn't parse is reported")
	assert.Contains(t, err.Error(), "nb_broken")
	assert.Equal(t, len(want), n)

	repo := notebook.NewContentRepository(db)
	for id, nc := range want {
		got, err := repo.Load(ctx, []string{id})
		require.NoError(t, err)
		require.Len(t, got, 1, id)
		wantFiles, err := nc.Bundle()
		require.NoError(t, err)
		gotFiles, err := got[0].Bundle()
		require.NoError(t, err)
		assert.Equal(t, wantFiles, gotFiles, id)
	}

	// Registry rows are kept as they were.
	var row struct {
		Owner      *int64 `db:"owner_user_id"`
		Visibility string `db:"visibility"`
		Source     string `db:"source"`
		Hash       string `db:"content_hash"`
		Bytes      int    `db:"content_bytes"`
	}
	require.NoError(t, db.GetContext(ctx, &row,
		`SELECT owner_user_id, visibility, source, content_hash, content_bytes FROM notebooks WHERE notebook_id = 'nb_private'`))
	require.NotNil(t, row.Owner)
	assert.Equal(t, ownerID, *row.Owner)
	assert.Equal(t, "private", row.Visibility)
	assert.Equal(t, "user", row.Source)
	assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", row.Hash)
	assert.Equal(t, notebook.BundleBytes(private), row.Bytes)

	// Converted notebooks have their notes; the stored files stay as a backup.
	var notes int
	require.NoError(t, db.GetContext(ctx, &notes, `SELECT count(*) FROM notebook_notes WHERE notebook_id = 'nb_private'`))
	assert.Equal(t, 1, notes)
	var kept int
	require.NoError(t, db.GetContext(ctx, &kept, `SELECT count(DISTINCT notebook_id) FROM notebook_files`))
	assert.Equal(t, len(want)+1, kept)

	// Running the upgrade again converts nothing more (only the broken one is
	// retried, and fails again).
	n, err = ConvertStoredFiles(ctx, db, io.Discard)
	require.Error(t, err)
	assert.Zero(t, n)

	// The composite example notebook reads through the server's Reader with
	// both its families.
	reader, err := notebook.NewContentReader(ctx, repo, nil)
	require.NoError(t, err)
	_, hasDefs := reader.GetDefinitionsBook("latin-roots-book")
	assert.True(t, hasDefs)
	_, hasEtymology := reader.GetEtymologyIndexes()["latin-roots-book"]
	assert.True(t, hasEtymology)
}
