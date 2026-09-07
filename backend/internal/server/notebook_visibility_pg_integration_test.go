package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/inference/mock"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/internal/testutil"
	"github.com/at-ishikawa/langner/schemas"
)

// These tests cover auth Phase 3: notebook public/private visibility. The
// example books examples/definitions/visibility-{public,private}-demo are loaded
// through the SAME Service/NotebookHandler construction the server uses
// (bootstrap.BuildStateRepositories → SetHistoryStore/SetSkipStores/
// SetNotebookACL), so the assertions exercise the real read paths, not a
// hand-built card (see .claude/rules/verify-data-features-with-example-notebooks.md).
//
// The private book is owned by the "owner" user; a second "non-owner" user must
// never see it — in quiz options, notebook detail, or the freeform pool — while
// both users see the public book. The DB path (the notebooks overlay table) is
// Postgres-only, so the end-to-end test skips without LANGNER_INTEGRATION_DB_URL
// (CI provides it via the postgres:16 service container).

const (
	visPublicNotebook  = "visibility-public-demo"
	visPrivateNotebook = "visibility-private-demo"
)

// repoRootForVisibilityTest walks up to the directory holding config.example.yml
// so the test loads the shipped example notebooks (the real on-disk format).
func repoRootForVisibilityTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for dir != "/" {
		if _, err := os.Stat(filepath.Join(dir, "config.example.yml")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Skip("repo root (config.example.yml) not found")
	return ""
}

// TestNotebookOwnership_ExampleConfigParses pins that config.example.yml wires
// the two example books through notebook_ownership as intended (no DB needed).
// It proves the config block the server reads matches the on-disk example books,
// so the DB provisioning has the right input.
func TestNotebookOwnership_ExampleConfigParses(t *testing.T) {
	root := repoRootForVisibilityTest(t)
	loader, err := config.NewConfigLoader(filepath.Join(root, "config.example.yml"))
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	byID := make(map[string]config.NotebookOwnership, len(cfg.NotebookOwnership))
	for _, o := range cfg.NotebookOwnership {
		byID[o.NotebookID] = o
	}

	priv, ok := byID[visPrivateNotebook]
	require.True(t, ok, "config.example.yml must wire the private example book under notebook_ownership")
	assert.Equal(t, notebook.VisibilityPrivate, priv.Visibility)
	assert.Equal(t, "admin@example.com", priv.OwnerEmail)

	pub, ok := byID[visPublicNotebook]
	require.True(t, ok, "config.example.yml must wire the public example book under notebook_ownership")
	assert.Equal(t, notebook.VisibilityPublic, pub.Visibility)
}

// fakeVisibility hides one notebook from everyone except ownerID. It stands in
// for the DB-backed NotebookACLRepository so the handler hard-reject path is
// exercised in CI without a database (the notebooks overlay table is
// Postgres-only). The notebooks themselves are the real shipped example books.
type fakeVisibility struct {
	privateNotebook string
	ownerID         int64
}

func (f fakeVisibility) VisibleNotebookIDs(_ context.Context, userID int64) (notebook.VisibilityPredicate, error) {
	return func(notebookID string) bool {
		if notebookID == f.privateNotebook {
			return userID == f.ownerID
		}
		return true
	}, nil
}

// TestNotebookVisibility_GetNotebookDetail_HardRejects drives GetNotebookDetail
// over the real example books with a fake ACL (no DB): the owner loads the
// private book, a non-owner gets CodeNotFound, and both load the public book.
func TestNotebookVisibility_GetNotebookDetail_HardRejects(t *testing.T) {
	const ownerID, nonOwnerID = int64(1), int64(2)
	root := repoRootForVisibilityTest(t)
	ex := filepath.Join(root, "examples")
	nbCfg := config.NotebooksConfig{
		StoriesDirectories:     []string{filepath.Join(ex, "stories")},
		JournalsDirectories:    []string{filepath.Join(ex, "journals")},
		FlashcardsDirectories:  []string{filepath.Join(ex, "flashcards")},
		BooksDirectories:       []string{filepath.Join(ex, "books")},
		DefinitionsDirectories: []string{filepath.Join(ex, "definitions")},
		EtymologyDirectories:   []string{filepath.Join(ex, "etymology")},
		LearningNotesDirectory: t.TempDir(),
	}
	h := NewNotebookHandler(nbCfg, config.TemplatesConfig{}, make(map[string]rapidapi.Response), nil, inference.StaticResolver(mock.NewClient()), nil)
	h.SetNotebookACL(fakeVisibility{privateNotebook: visPrivateNotebook, ownerID: ownerID})

	detail := func(userID int64, nbID string) error {
		_, err := h.GetNotebookDetail(
			testutil.WithTestUser(context.Background(), userID),
			connect.NewRequest(&apiv1.GetNotebookDetailRequest{NotebookId: nbID}),
		)
		return err
	}

	require.NoError(t, detail(ownerID, visPrivateNotebook), "owner loads their private book")
	require.NoError(t, detail(ownerID, visPublicNotebook), "owner loads the public book")
	require.NoError(t, detail(nonOwnerID, visPublicNotebook), "non-owner loads the public book")

	err := detail(nonOwnerID, visPrivateNotebook)
	require.Error(t, err, "non-owner must be rejected from the private book")
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr), "want a connect error, got %T", err)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code(), "hidden notebook must 404, not disclose existence")
}

// visibilityFixture wires the real DB-mode Service + NotebookHandler over the
// example notebooks, seeds two users, and provisions the notebooks overlay so
// the private book is owned by ownerID. Returns the two handlers, the quiz
// service (for LoadNotebookSummaries / LoadAllWords), the db, and the two user
// ids.
type visibilityFixture struct {
	svc             *quiz.Service
	notebookHandler *NotebookHandler
	db              *sqlx.DB
	ownerID         int64
	nonOwnerID      int64
}

func newVisibilityFixture(t *testing.T) visibilityFixture {
	t.Helper()

	dsn := os.Getenv(integrationDBEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping live-Postgres notebook-visibility coverage", integrationDBEnv)
	}
	root := repoRootForVisibilityTest(t)

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())

	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	// Two accounts. Encrypted PII columns are irrelevant to visibility, so seed
	// placeholder ciphertext (the same shape the other PG integration tests use).
	seedUser := func(sub, hash string) int64 {
		var id int64
		require.NoError(t, db.Get(&id,
			`INSERT INTO users (google_sub, email_encrypted, email_hash, name_encrypted)
			 VALUES ($1, '\x00'::bytea, $2, '\x00'::bytea) RETURNING id`, sub, hash))
		return id
	}
	ownerID := seedUser("vis-owner", "vis-owner-hash")
	nonOwnerID := seedUser("vis-nonowner", "vis-nonowner-hash")

	// Provision the notebooks overlay the way notebook_ownership provisioning
	// does: private book owned by ownerID, public book public. This is the exact
	// write `langner auth provision` performs (UpsertOwnership).
	acl := notebook.NewNotebookACLRepository(db)
	require.NoError(t, acl.UpsertOwnership(context.Background(), visPrivateNotebook, &ownerID, notebook.VisibilityPrivate))
	require.NoError(t, acl.UpsertOwnership(context.Background(), visPublicNotebook, &ownerID, notebook.VisibilityPublic))

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

	// The exact wiring cmd/langner-server/main.go performs in DB mode.
	repos := bootstrap.BuildStateRepositories(nbCfg, quizCfg, db)
	svc := quiz.NewService(nbCfg, inference.StaticResolver(mock.NewClient()), make(map[string]rapidapi.Response), repos.Learning, quizCfg)
	svc.SetHistoryStore(repos.HistoryStore)
	svc.SetSkipStores(repos.SkipFlags, repos.Note, repos.Origin)
	svc.SetNotebookACL(repos.ACL)

	notebookHandler := NewNotebookHandler(nbCfg, config.TemplatesConfig{}, make(map[string]rapidapi.Response), nil, inference.StaticResolver(mock.NewClient()), repos.Note)
	notebookHandler.SetHistoryStore(repos.HistoryStore)
	notebookHandler.SetNotebookACL(repos.ACL)

	return visibilityFixture{svc: svc, notebookHandler: notebookHandler, db: db, ownerID: ownerID, nonOwnerID: nonOwnerID}
}

func summaryIDs(t *testing.T, svc *quiz.Service, userID int64) map[string]bool {
	t.Helper()
	summaries, err := svc.LoadNotebookSummaries(userID, true)
	require.NoError(t, err)
	ids := make(map[string]bool, len(summaries))
	for _, s := range summaries {
		ids[s.NotebookID] = true
	}
	return ids
}

func freeformExpressions(t *testing.T, svc *quiz.Service, userID int64) map[string]bool {
	t.Helper()
	cards, err := svc.LoadAllWords(userID)
	require.NoError(t, err)
	exprs := make(map[string]bool, len(cards))
	for _, c := range cards {
		exprs[c.Expression] = true
	}
	return exprs
}

// TestNotebookVisibility_LivePostgres_Integration drives the whole access-control
// surface end to end: quiz options, notebook detail, and the freeform pool.
func TestNotebookVisibility_LivePostgres_Integration(t *testing.T) {
	f := newVisibilityFixture(t)

	// --- Quiz options (LoadNotebookSummaries) ---
	ownerSummaries := summaryIDs(t, f.svc, f.ownerID)
	assert.True(t, ownerSummaries[visPublicNotebook], "owner must see the public book in quiz options")
	assert.True(t, ownerSummaries[visPrivateNotebook], "owner must see their own private book in quiz options")

	nonOwnerSummaries := summaryIDs(t, f.svc, f.nonOwnerID)
	assert.True(t, nonOwnerSummaries[visPublicNotebook], "non-owner must see the public book in quiz options")
	assert.False(t, nonOwnerSummaries[visPrivateNotebook], "non-owner must NOT see the private book in quiz options")

	// --- Notebook detail (GetNotebookDetail) ---
	detail := func(userID int64, nbID string) error {
		_, err := f.notebookHandler.GetNotebookDetail(
			testutil.WithTestUser(context.Background(), userID),
			connect.NewRequest(&apiv1.GetNotebookDetailRequest{NotebookId: nbID}),
		)
		return err
	}
	require.NoError(t, detail(f.ownerID, visPrivateNotebook), "owner must load their private book's detail")
	require.NoError(t, detail(f.ownerID, visPublicNotebook), "owner must load the public book's detail")
	require.NoError(t, detail(f.nonOwnerID, visPublicNotebook), "non-owner must load the public book's detail")

	err := detail(f.nonOwnerID, visPrivateNotebook)
	require.Error(t, err, "non-owner must be rejected from the private book's detail")
	var connectErr *connect.Error
	require.True(t, errors.As(err, &connectErr), "error must be a connect error, got %T", err)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code(),
		"a hidden notebook must 404 (CodeNotFound), never disclose its existence")

	// --- Freeform pool (LoadAllWords) ---
	ownerFreeform := freeformExpressions(t, f.svc, f.ownerID)
	assert.True(t, ownerFreeform["spectate"], "owner freeform pool includes the public book's words")
	assert.True(t, ownerFreeform["portage"], "owner freeform pool includes their private book's words")
	assert.True(t, ownerFreeform["comport"], "owner freeform pool includes their private book's words")

	nonOwnerFreeform := freeformExpressions(t, f.svc, f.nonOwnerID)
	assert.True(t, nonOwnerFreeform["spectate"], "non-owner freeform pool includes the public book's words")
	assert.False(t, nonOwnerFreeform["portage"], "a private book's words must NEVER enter a non-owner's freeform pool")
	assert.False(t, nonOwnerFreeform["comport"], "a private book's words must NEVER enter a non-owner's freeform pool")
}

// TestNotebookVisibility_HistoryStaysPrivate_LivePostgres_Integration seeds real
// per-user learning-history STATE through the write path (SaveResult), then
// proves visibility gates independently of that state: the owner's private-book
// history exists in the DB, yet the non-owner never sees the notebook — its
// existence and its words stay hidden regardless of what history rows exist.
func TestNotebookVisibility_HistoryStaysPrivate_LivePostgres_Integration(t *testing.T) {
	f := newVisibilityFixture(t)

	// Owner records a real attempt on a word in their PRIVATE book (the same
	// write path a runtime quiz uses), creating a note + learning_log in the DB
	// attributed to the owner.
	ctx := context.Background()
	cards, err := f.svc.LoadCards(f.ownerID, []string{visPrivateNotebook}, true, nil)
	require.NoError(t, err)
	require.NotEmpty(t, cards, "the owner can load their private book's cards")
	require.NoError(t, f.svc.SaveResult(ctx, f.ownerID, cards[0], quiz.GradeResult{Correct: true, Quality: 5}, 1200))

	// The log really landed, attributed to the owner.
	var ownerLogs int
	require.NoError(t, f.db.Get(&ownerLogs,
		`SELECT COUNT(*) FROM learning_logs WHERE user_id = $1`, f.ownerID))
	assert.Equal(t, 1, ownerLogs, "owner's private-book attempt is persisted under their user id")

	// Non-owner also records an attempt, but only on the PUBLIC book they can see.
	pubCards, err := f.svc.LoadCards(f.nonOwnerID, []string{visPublicNotebook}, true, nil)
	require.NoError(t, err)
	require.NotEmpty(t, pubCards)
	require.NoError(t, f.svc.SaveResult(ctx, f.nonOwnerID, pubCards[0], quiz.GradeResult{Correct: true, Quality: 5}, 1200))

	// Despite the owner having private-book history in the DB, the non-owner
	// still cannot see the private book anywhere.
	nonOwnerSummaries := summaryIDs(t, f.svc, f.nonOwnerID)
	assert.False(t, nonOwnerSummaries[visPrivateNotebook], "owner's private history must not surface the book to the non-owner")

	_, err = f.svc.LoadCards(f.nonOwnerID, []string{visPrivateNotebook}, true, nil)
	require.Error(t, err, "non-owner loading the private book's cards must fail (treated as not-found)")
	var notFound *quiz.NotFoundError
	assert.True(t, errors.As(err, &notFound), "a hidden notebook is a NotFoundError, got %T", err)
}
