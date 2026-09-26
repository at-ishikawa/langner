package server

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/bootstrap"
	"github.com/at-ishikawa/langner/internal/clicmd"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference/mock"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/schemas"
)

// This is the mandatory real-config integration test for the id-preserving
// filesystem->Postgres import (design §13.3/§13.5). It exercises the exact
// path-divergence the repo rule targets: import the SHIPPED example catalog via
// the REAL import command core (clicmd.ImportFilesystemNotebooks), then rebuild
// the quiz Service with an EMPTY filesystem catalog + DBContentSource — the
// Vercel/no-filesystem serving shape — and prove the COMPOSITE notebook
// `latin-roots-book` (one id spanning the definitions AND etymology families)
// round-trips: its definitions words load AND resolve an origin whose meaning
// comes only from the etymology sibling under the same id. Also proves the id is
// preserved (no nb_ minting, so learning histories keyed by it survive) and a
// pre-existing private ownership row is not clobbered. Postgres-only.
func TestImportFilesystem_ServesCompositeCatalogFromDB_Integration(t *testing.T) {
	dsn := os.Getenv(integrationDBEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping live-Postgres filesystem-import coverage", integrationDBEnv)
	}
	root := repoRootForVisibilityTest(t)
	ctx := context.Background()

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	var userID int64
	require.NoError(t, db.Get(&userID,
		`INSERT INTO users (google_sub, username) VALUES ('import-fs-owner','import-fs-owner') RETURNING id`))

	// A pre-existing private ownership row on the composite id — the import must
	// PRESERVE it (design §13.3: an operator's `set-owner` is never clobbered).
	_, err = db.Exec(
		`INSERT INTO notebooks (notebook_id, owner_user_id, visibility, source) VALUES ($1, $2, 'private', 'shipped')`,
		"latin-roots-book", userID)
	require.NoError(t, err)

	// --- Import phase: config points at examples/, run the REAL import core ---
	ex := filepath.Join(root, "examples")
	importCfg := &config.Config{Notebooks: config.NotebooksConfig{
		StoriesDirectories:     []string{filepath.Join(ex, "stories")},
		JournalsDirectories:    []string{filepath.Join(ex, "journals")},
		FlashcardsDirectories:  []string{filepath.Join(ex, "flashcards")},
		BooksDirectories:       []string{filepath.Join(ex, "books")},
		DefinitionsDirectories: []string{filepath.Join(ex, "definitions")},
		EtymologyDirectories:   []string{filepath.Join(ex, "etymology")},
		GrammarsDirectories:    []string{filepath.Join(ex, "grammars")},
		LearningNotesDirectory: t.TempDir(),
	}}
	fileRepo := notebook.NewNotebookFileRepository(db)

	n, err := clicmd.ImportFilesystemNotebooks(ctx, importCfg, fileRepo, io.Discard)
	require.NoError(t, err)
	require.Greater(t, n, 0, "import must find and store filesystem notebooks")

	// --- Storage: the composite id is stored under BOTH families, id preserved ---
	var fams []string
	require.NoError(t, db.Select(&fams,
		`SELECT DISTINCT family FROM notebook_files WHERE notebook_id = $1 ORDER BY family`, "latin-roots-book"))
	assert.Equal(t, []string{"definitions", "etymology"}, fams,
		"one composite id must store blobs under each of its families")

	var source string
	require.NoError(t, db.Get(&source, `SELECT source FROM notebooks WHERE notebook_id = $1`, "latin-roots-book"))
	assert.Equal(t, "shipped", source)

	// Ownership preserved — not reset to public/NULL by the import.
	var vis string
	var owner sql.NullInt64
	require.NoError(t, db.QueryRow(
		`SELECT visibility, owner_user_id FROM notebooks WHERE notebook_id = $1`, "latin-roots-book").Scan(&vis, &owner))
	assert.Equal(t, "private", vis, "import must not clobber a pre-existing private ownership row")
	assert.True(t, owner.Valid && owner.Int64 == userID, "import must preserve the pre-existing owner")

	// No nb_ ids minted for the shipped catalog — ids are preserved so existing
	// learning_logs.source_notebook_id keeps resolving.
	var minted int
	require.NoError(t, db.Get(&minted, `SELECT COUNT(*) FROM notebook_files WHERE notebook_id LIKE 'nb\_%'`))
	assert.Equal(t, 0, minted, "the filesystem import must never mint nb_ ids")

	// --- Serve phase: EMPTY filesystem catalog + DBContentSource (Vercel shape) ---
	serveCfg := config.NotebooksConfig{LearningNotesDirectory: t.TempDir()} // every family dir empty
	quizCfg := config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}, DisableShuffle: true}
	repos := bootstrap.BuildStateRepositories(serveCfg, quizCfg, db)
	svc := quiz.NewService(serveCfg, mock.NewClient(), make(map[string]rapidapi.Response), repos.Learning, quizCfg)
	svc.SetHistoryStore(repos.HistoryStore)
	svc.SetSkipStores(repos.SkipFlags, repos.Note, repos.Origin)
	svc.SetNotebookACL(repos.ACL)
	svc.SetContentSource(notebook.NewDBContentSource(fileRepo))

	// The owner (the private row's owner) can load the composite notebook by its
	// ORIGINAL id — served entirely from Postgres, no filesystem.
	cards, err := svc.LoadCards(userID, []string{"latin-roots-book"}, true, nil)
	require.NoError(t, err)
	require.NotEmpty(t, cards, "the composite notebook is quizzable from the DB under its preserved id")

	// Composite proof: a definitions word resolves an origin whose MEANING comes
	// ONLY from the etymology notebook of the SAME id. If the etymology family
	// had not loaded from the DB, buildOriginMap would be empty and the meaning
	// would be blank — so a non-empty origin meaning proves both families of one
	// id materialized and linked through the DB-served reader.
	var originMeaning string
	for _, c := range cards {
		for _, p := range c.WordDetail.OriginParts {
			if p.Meaning != "" {
				originMeaning = p.Meaning
			}
		}
	}
	require.NotEmpty(t, originMeaning,
		"a latin-roots-book word must carry an etymology-sourced origin meaning — proving the definitions+etymology composite round-tripped through the DB")

	// A non-owner cannot see this private notebook (visibility still enforced).
	var otherID int64
	require.NoError(t, db.Get(&otherID,
		`INSERT INTO users (google_sub, username) VALUES ('import-fs-other','import-fs-other') RETURNING id`))
	_, err = svc.LoadCards(otherID, []string{"latin-roots-book"}, true, nil)
	require.Error(t, err, "a non-owner must not load the private imported notebook")
}
