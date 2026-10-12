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
	"github.com/at-ishikawa/langner/internal/dbseed"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
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
		id, err := dbseed.SeedUser(context.Background(), db, sub, "")
		require.NoError(t, err)
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
	svc := quiz.NewService(nbCfg, inference.StaticResolver(mock.NewClient()), make(map[string]rapidapi.Response), repos.Learning, quizCfg)
	svc.SetHistoryStore(repos.HistoryStore)
	svc.SetSkipStores(repos.SkipFlags, repos.Note, repos.Origin)
	svc.SetNotebookACL(repos.ACL)

	notebookHandler := NewNotebookHandler(nbCfg, config.TemplatesConfig{}, make(map[string]rapidapi.Response), nil, inference.StaticResolver(mock.NewClient()), repos.Note)
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

// compositeRootsBundle reads the shipped composite example user notebook
// (examples/user-notebooks/roots-mini: a definitions family + an etymology
// family under one id) as a composite push request carries it — each file's
// path family-prefixed ("definitions/index.yml", "etymology/origins.yml").
func compositeRootsBundle(t *testing.T) []*apiv1.NotebookFile {
	t.Helper()
	root := repoRootForVisibilityTest(t)
	base := filepath.Join(root, "examples", "user-notebooks", "roots-mini")
	rels := []string{
		"definitions/index.yml", "definitions/definitions.yml",
		"etymology/index.yml", "etymology/origins.yml",
	}
	var files []*apiv1.NotebookFile
	for _, rel := range rels {
		content, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(rel)))
		require.NoError(t, err)
		files = append(files, &apiv1.NotebookFile{Path: rel, Content: content})
	}
	return files
}

// TestPushNotebook_Composite_LivePostgres_Integration drives a COMPOSITE push —
// one notebook id spanning the definitions AND etymology families — end to end
// against a live Postgres, then serves it from the DB and proves the two
// families round-trip under a single minted id: the definitions word resolves an
// origin meaning that lives ONLY in the etymology family (§13.6).
func TestPushNotebook_Composite_LivePostgres_Integration(t *testing.T) {
	f := newPushFixture(t)
	ctx := context.Background()
	ownerCtx := testutil.WithTestUser(ctx, f.ownerID)

	pushResp, err := f.notebookHandler.PushNotebook(ownerCtx, connect.NewRequest(&apiv1.PushNotebookRequest{
		Kind:  "composite", // the reserved sentinel: files carry family-prefixed paths
		Name:  "Roots Mini",
		Files: compositeRootsBundle(t),
	}))
	require.NoError(t, err)
	nbID := pushResp.Msg.NotebookId
	require.True(t, strings.HasPrefix(nbID, notebook.NotebookIDPrefix), "server mints an nb_ id, got %q", nbID)

	// Blobs stored under BOTH families for the one minted id.
	var fams []string
	require.NoError(t, f.db.Select(&fams,
		`SELECT DISTINCT family FROM notebook_files WHERE notebook_id = $1 ORDER BY family`, nbID))
	assert.Equal(t, []string{"definitions", "etymology"}, fams,
		"a composite push stores its definitions and etymology files under distinct families of one id")

	// Served from the DB: the definitions word loads and resolves an origin whose
	// meaning comes ONLY from the etymology family of the same id.
	cards, err := f.svc.LoadCards(f.ownerID, []string{nbID}, true, nil)
	require.NoError(t, err)
	require.NotEmpty(t, cards, "the composite notebook is quizzable from the DB")

	var originMeaning string
	for _, c := range cards {
		for _, p := range c.WordDetail.OriginParts {
			if p.Meaning != "" {
				originMeaning = p.Meaning
			}
		}
	}
	require.NotEmpty(t, originMeaning,
		"a composite-pushed definitions word must carry an etymology-sourced origin meaning — proving both families round-tripped under one minted id")

	// Listing regressions: with includeUnstudied=false nothing is due for a fresh
	// push, which used to (a) skip the definitions-book summary entirely and (b)
	// name it by the raw nb_ id. Assert the Books summary still lists and is named
	// from the etymology sibling index, not the id.
	summaries, err := f.svc.LoadNotebookSummaries(f.ownerID, false)
	require.NoError(t, err)
	var defsSummary *quiz.NotebookSummary
	for i := range summaries {
		if summaries[i].NotebookID == nbID && summaries[i].Kind == "Books" {
			defsSummary = &summaries[i]
			break
		}
	}
	require.NotNil(t, defsSummary, "a 0-due composite book must still be listed, not skipped")
	assert.NotEqual(t, nbID, defsSummary.Name, "the summary must not display the raw nb_ id as the name")
	assert.Contains(t, defsSummary.Name, "Roots Mini", "the name resolves from the etymology sibling index")

	// Private by default: a non-owner cannot load it.
	_, err = f.svc.LoadCards(f.nonOwnerID, []string{nbID}, true, nil)
	require.Error(t, err, "a composite push is private to its owner")
}

// TestPushNotebook_UpdatesExistingOwnedId_LivePostgres_Integration pins the
// create-or-update policy: a fresh push mints a new nb_ id, but when the bundle
// declares an id that ALREADY EXISTS and the caller OWNS it, push updates it in
// place — preserving the id (so learning history stays attached) and its source
// ('shipped' stays 'shipped'), not minting a new notebook.
func TestPushNotebook_UpdatesExistingOwnedId_LivePostgres_Integration(t *testing.T) {
	f := newPushFixture(t)
	ctx := context.Background()
	ownerCtx := testutil.WithTestUser(ctx, f.ownerID)

	// Simulate a prior filesystem import: a shipped notebook under a human id
	// the owner claimed (private, source='shipped').
	require.NoError(t, dbseed.SetNotebookOwner(ctx, f.db, "roots-mini", &f.ownerID, "private", "shipped"))

	// Push the composite roots-mini bundle (its index.yml declares id roots-mini).
	resp, err := f.notebookHandler.PushNotebook(ownerCtx, connect.NewRequest(&apiv1.PushNotebookRequest{
		Kind:  "composite",
		Name:  "Roots Mini",
		Files: compositeRootsBundle(t),
	}))
	require.NoError(t, err)

	// Updated in place under the SAME human id — NOT minted as a new nb_.
	assert.Equal(t, "roots-mini", resp.Msg.NotebookId, "an owned declared id updates in place, not minting a new nb_")
	assert.False(t, strings.HasPrefix(resp.Msg.NotebookId, notebook.NotebookIDPrefix), "must not mint an nb_ id when updating an existing owned notebook")

	// Source preserved: a re-pushed shipped notebook stays 'shipped'.
	var source string
	require.NoError(t, f.db.Get(&source, `SELECT source FROM notebooks WHERE notebook_id = 'roots-mini'`))
	assert.Equal(t, "shipped", source, "updating a shipped notebook must not reclassify it as user")

	// Blobs were (re)written under the preserved id.
	var blobs int
	require.NoError(t, f.db.Get(&blobs, `SELECT COUNT(*) FROM notebook_files WHERE notebook_id = 'roots-mini'`))
	assert.Greater(t, blobs, 0, "the push stored blobs under the preserved id")

	// "My notebooks" = everything owned: the owned source='shipped' notebook is
	// listed by ListUserNotebooks, not filtered out for not being source='user'.
	owned, lerr := f.notebookHandler.fileRepo.ListUserNotebooks(ctx, f.ownerID)
	require.NoError(t, lerr)
	var ownedIDs []string
	for _, n := range owned {
		ownedIDs = append(ownedIDs, n.NotebookID)
	}
	assert.Contains(t, ownedIDs, "roots-mini", "an owned source='shipped' notebook must be listed among the owner's notebooks")

	// A different user pushing that same declared id is rejected (ownership).
	_, err = f.notebookHandler.PushNotebook(testutil.WithTestUser(ctx, f.nonOwnerID), connect.NewRequest(&apiv1.PushNotebookRequest{
		Kind:  "composite",
		Name:  "Roots Mini",
		Files: compositeRootsBundle(t),
	}))
	require.Error(t, err, "a non-owner cannot update someone else's notebook by declaring its id")
}
