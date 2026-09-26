package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/at-ishikawa/langner/gen-protos/api/v1"
	"github.com/at-ishikawa/langner/internal/bootstrap"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference/mock"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/internal/testutil"
	"github.com/at-ishikawa/langner/schemas"
)

// This test drives the WHOLE per-user-notebooks path end to end against a live
// Postgres, through the SAME Service/NotebookHandler construction the server
// uses (bootstrap.BuildStateRepositories + SetContentSource/SetPushDeps): push
// the example bundle examples/user-notebooks/idioms → mint nb_ id → store blobs
// → import notes → read via the reader/quiz → grade an attempt → prove it's
// hidden from another user → pull round-trips. It is the mandatory real-config
// integration test required by
// .claude/rules/verify-data-features-with-example-notebooks.md. Postgres-only,
// so it skips without LANGNER_INTEGRATION_DB_URL.

type pushFixture struct {
	svc             *quiz.Service
	notebookHandler *NotebookHandler
	db              *sqlx.DB
	ownerID         int64
	nonOwnerID      int64
}

func newPushFixture(t *testing.T) pushFixture {
	t.Helper()
	dsn := os.Getenv(integrationDBEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping live-Postgres per-user-notebooks coverage", integrationDBEnv)
	}
	root := repoRootForVisibilityTest(t)

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())

	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	seedUser := func(sub string) int64 {
		var id int64
		require.NoError(t, db.Get(&id, `INSERT INTO users (google_sub, username) VALUES ($1, $1) RETURNING id`, sub))
		return id
	}
	ownerID := seedUser("push-owner")
	nonOwnerID := seedUser("push-nonowner")

	ex := filepath.Join(root, "examples")
	nbCfg := config.NotebooksConfig{
		StoriesDirectories:     []string{filepath.Join(ex, "stories")},
		JournalsDirectories:    []string{filepath.Join(ex, "journals")},
		FlashcardsDirectories:  []string{filepath.Join(ex, "flashcards")},
		BooksDirectories:       []string{filepath.Join(ex, "books")},
		DefinitionsDirectories: []string{filepath.Join(ex, "definitions")},
		EtymologyDirectories:   []string{filepath.Join(ex, "etymology")},
		GrammarsDirectories:    []string{filepath.Join(ex, "grammars")},
		LearningNotesDirectory: t.TempDir(),
	}
	quizCfg := config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}, DisableShuffle: true}

	// The exact DB-mode wiring cmd/langner-server/main.go performs, plus the
	// per-user-notebook seams this feature adds.
	repos := bootstrap.BuildStateRepositories(nbCfg, quizCfg, db)
	svc := quiz.NewService(nbCfg, mock.NewClient(), make(map[string]rapidapi.Response), repos.Learning, quizCfg)
	svc.SetHistoryStore(repos.HistoryStore)
	svc.SetSkipStores(repos.SkipFlags, repos.Note, repos.Origin)
	svc.SetNotebookACL(repos.ACL)

	notebookHandler := NewNotebookHandler(nbCfg, config.TemplatesConfig{}, make(map[string]rapidapi.Response), nil, mock.NewClient(), repos.Note)
	notebookHandler.SetHistoryStore(repos.HistoryStore)
	notebookHandler.SetNotebookACL(repos.ACL)

	fileRepo := notebook.NewNotebookFileRepository(db)
	contentSource := notebook.NewDBContentSource(fileRepo)
	svc.SetContentSource(contentSource)
	notebookHandler.SetPushDeps(db, fileRepo, contentSource)

	return pushFixture{svc: svc, notebookHandler: notebookHandler, db: db, ownerID: ownerID, nonOwnerID: nonOwnerID}
}

// exampleIdiomBundle reads the shipped example user-notebook bundle as the push
// request would carry it (index.yml + cards.yml, bundle-relative paths).
func exampleIdiomBundle(t *testing.T) []*apiv1.NotebookFile {
	t.Helper()
	root := repoRootForVisibilityTest(t)
	dir := filepath.Join(root, "examples", "user-notebooks", "idioms")
	var files []*apiv1.NotebookFile
	for _, rel := range []string{"index.yml", "cards.yml"} {
		content, err := os.ReadFile(filepath.Join(dir, rel))
		require.NoError(t, err)
		files = append(files, &apiv1.NotebookFile{Path: rel, Content: content})
	}
	return files
}

func TestPushNotebook_EndToEnd_LivePostgres_Integration(t *testing.T) {
	f := newPushFixture(t)
	ctx := context.Background()
	ownerCtx := testutil.WithTestUser(ctx, f.ownerID)

	// --- Push (create) as the owner ---
	pushResp, err := f.notebookHandler.PushNotebook(ownerCtx, connect.NewRequest(&apiv1.PushNotebookRequest{
		Kind:  "Flashcard",
		Name:  "Common English Idioms",
		Files: exampleIdiomBundle(t),
	}))
	require.NoError(t, err)
	nbID := pushResp.Msg.NotebookId
	require.True(t, strings.HasPrefix(nbID, notebook.NotebookIDPrefix), "server must mint an nb_ id, got %q", nbID)
	assert.Greater(t, pushResp.Msg.ImportedRows["notes"], int32(0), "push must import notes")

	// --- Stored in notebook_files (2 blobs) + a private notebooks row ---
	var blobCount int
	require.NoError(t, f.db.Get(&blobCount, `SELECT COUNT(*) FROM notebook_files WHERE notebook_id = $1`, nbID))
	assert.Equal(t, 2, blobCount, "both bundle files are stored as blobs")

	var visibility, source string
	var owner int64
	require.NoError(t, f.db.QueryRow(
		`SELECT visibility, source, owner_user_id FROM notebooks WHERE notebook_id = $1`, nbID).
		Scan(&visibility, &source, &owner))
	assert.Equal(t, notebook.VisibilityPrivate, visibility)
	assert.Equal(t, "user", source)
	assert.Equal(t, f.ownerID, owner)

	// --- Imported into notes / notebook_notes under the nb_ id ---
	var noteLinks int
	require.NoError(t, f.db.Get(&noteLinks, `SELECT COUNT(*) FROM notebook_notes WHERE notebook_id = $1`, nbID))
	assert.Equal(t, 5, noteLinks, "all five idiom cards imported as notebook_notes")

	// --- ListMyNotebooks: owner sees it, non-owner does not ---
	ownerList, err := f.notebookHandler.ListMyNotebooks(ownerCtx, connect.NewRequest(&apiv1.ListMyNotebooksRequest{}))
	require.NoError(t, err)
	require.Len(t, ownerList.Msg.Notebooks, 1)
	assert.Equal(t, nbID, ownerList.Msg.Notebooks[0].NotebookId)
	assert.Equal(t, "Common English Idioms", ownerList.Msg.Notebooks[0].Name)

	nonOwnerList, err := f.notebookHandler.ListMyNotebooks(testutil.WithTestUser(ctx, f.nonOwnerID), connect.NewRequest(&apiv1.ListMyNotebooksRequest{}))
	require.NoError(t, err)
	assert.Empty(t, nonOwnerList.Msg.Notebooks, "a user's list must not include another user's notebooks")

	// --- Quizzable: the owner loads a card and a graded attempt persists ---
	cards, err := f.svc.LoadCards(f.ownerID, []string{nbID}, true, nil)
	require.NoError(t, err)
	require.NotEmpty(t, cards, "the pushed notebook is quizzable for its owner")
	require.NoError(t, f.svc.SaveResult(ctx, f.ownerID, cards[0], quiz.GradeResult{Correct: true, Quality: 5}, 1200))

	var ownerLogs int
	require.NoError(t, f.db.Get(&ownerLogs,
		`SELECT COUNT(*) FROM learning_logs l JOIN notebook_notes nn ON nn.note_id = l.note_id
		 WHERE nn.notebook_id = $1 AND l.user_id = $2`, nbID, f.ownerID))
	assert.Equal(t, 1, ownerLogs, "the graded attempt persists to learning_logs under the owner's user id")

	// --- Hidden from a different user (private via VisibleNotebookIDs) ---
	_, err = f.svc.LoadCards(f.nonOwnerID, []string{nbID}, true, nil)
	require.Error(t, err, "a non-owner must not be able to load a private notebook's cards")
	var notFound *quiz.NotFoundError
	assert.True(t, errors.As(err, &notFound), "hidden notebook is a NotFoundError, got %T", err)

	nonOwnerSummaries := summaryIDs(t, f.svc, f.nonOwnerID)
	assert.False(t, nonOwnerSummaries[nbID], "the private notebook must not appear in a non-owner's quiz options")

	// --- Pull round-trips the stored blobs (owner-only) ---
	pull, err := f.notebookHandler.PullNotebook(ownerCtx, connect.NewRequest(&apiv1.PullNotebookRequest{NotebookId: nbID}))
	require.NoError(t, err)
	require.Len(t, pull.Msg.Files, 2)
	byPath := map[string][]byte{}
	for _, pf := range pull.Msg.Files {
		byPath[pf.Path] = pf.Content
	}
	// cards.yml round-trips byte-for-byte; index.yml carries the minted nb_ id.
	orig := exampleIdiomBundle(t)
	var origCards []byte
	for _, of := range orig {
		if of.Path == "cards.yml" {
			origCards = of.Content
		}
	}
	assert.Equal(t, origCards, byPath["cards.yml"], "cards.yml pulls back byte-identical")
	assert.Contains(t, string(byPath["index.yml"]), nbID, "the stored index.yml carries the minted nb_ id")

	// --- A non-owner cannot pull it ---
	_, err = f.notebookHandler.PullNotebook(testutil.WithTestUser(ctx, f.nonOwnerID), connect.NewRequest(&apiv1.PullNotebookRequest{NotebookId: nbID}))
	require.Error(t, err, "a non-owner must not be able to pull a private notebook")
}
