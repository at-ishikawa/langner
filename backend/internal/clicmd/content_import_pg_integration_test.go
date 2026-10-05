package clicmd

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/bootstrap"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/dbseed"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference/mock"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/schemas"
)

// TestImportShippedContent_ServesFromDB_LivePostgres_Integration is the
// mandatory real-config integration test for DB-served notebook content (see
// .claude/rules/verify-data-features-with-example-notebooks.md). It loads the
// REAL example catalog through config.example.yml, imports it into notebook_files
// with ImportShippedContent (the step reset-db / seed-e2e run), then builds the
// quiz Service the SAME way the server does — but with every filesystem serving
// directory BLANKED — so a notebook that appears in the listing can ONLY have
// come from Postgres. This is the end-to-end proof that dev/e2e/prod serve
// content from the DB with no filesystem read.
//
// It also pins the awkward condition the importer must get right: a COMPOSITE
// notebook (the same id under two family dirs) folds into ONE notebook_files
// entry carrying files under BOTH families — latin-roots-book is both a
// definitions notebook and an etymology notebook in the example catalog.
//
// Requires LANGNER_INTEGRATION_DB_URL (CI's postgres:16); skipped otherwise.
// DROPs and recreates the public schema, so it must run isolated (-p 1 / its own
// workflow step).
func TestImportShippedContent_ServesFromDB_LivePostgres_Integration(t *testing.T) {
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}

	// config.example.yml's notebook dirs are relative to the repo root, so run
	// from there exactly as the server does.
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

	ctx := context.Background()

	// Import the shipped catalog into notebook_files.
	n, err := ImportShippedContent(ctx, cfg, db, io.Discard)
	require.NoError(t, err)
	require.Greater(t, n, 0, "expected to import at least one shipped notebook")

	// Composite correctness: one id, files under BOTH families (L1/L4 — one log
	// series per word, metadata selects the series; it must not fork two ids).
	var fams []string
	require.NoError(t, db.SelectContext(ctx, &fams,
		`SELECT DISTINCT family FROM notebook_files WHERE notebook_id = $1 ORDER BY family`, "latin-roots-book"))
	require.ElementsMatch(t, []string{"definitions", "etymology"}, fams,
		"a composite notebook must store its files under every family it belongs to, under one id")

	// A plain single-family notebook imports under its own family.
	var flashcardFiles int
	require.NoError(t, db.GetContext(ctx, &flashcardFiles,
		`SELECT count(*) FROM notebook_files WHERE notebook_id = $1 AND family = $2`, "vocabulary", "flashcards"))
	require.Positive(t, flashcardFiles, "the flashcard notebook must be imported under the flashcards family")

	// Build the Service the way the server does, but with ALL filesystem serving
	// directories blanked: with a DB content source installed, contentDirs()
	// returns DB dirs only, so any notebook listed below is served from Postgres.
	servingCfg := cfg.Notebooks
	servingCfg.StoriesDirectories = nil
	servingCfg.JournalsDirectories = nil
	servingCfg.FlashcardsDirectories = nil
	servingCfg.BooksDirectories = nil
	servingCfg.DefinitionsDirectories = nil
	servingCfg.EtymologyDirectories = nil
	servingCfg.GrammarsDirectories = nil
	servingCfg.LearningNotesDirectory = t.TempDir()

	repos := bootstrap.BuildStateRepositories(servingCfg, cfg.Quiz, db)
	svc := quiz.NewService(servingCfg, mock.NewClient(), make(map[string]rapidapi.Response), repos.Learning, cfg.Quiz)
	svc.SetHistoryStore(repos.HistoryStore)
	svc.SetSkipStores(repos.SkipFlags, repos.Note, repos.Origin)
	svc.SetContentSource(notebook.NewDBContentSource(notebook.NewNotebookFileRepository(db)))

	userID, err := dbseed.SeedUser(ctx, db, "content-import-reader", "")
	require.NoError(t, err)

	summaries, err := svc.LoadNotebookSummaries(userID, false)
	require.NoError(t, err)
	served := map[string]bool{}
	for _, s := range summaries {
		served[s.NotebookID] = true
	}
	require.True(t, served["vocabulary"], "flashcard notebook must be served from the DB (filesystem dirs are blank)")
	require.True(t, served["latin-roots-book"], "composite definitions/etymology notebook must be served from the DB")
}
