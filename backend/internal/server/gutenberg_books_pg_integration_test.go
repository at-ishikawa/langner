package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
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

// This test drives the WHOLE Gutenberg-books path end to end against a live
// Postgres, through the SAME Service/NotebookHandler wiring the server uses:
// ImportBook (with an injected fake fetcher serving the committed testdata
// fixture) stores ONE shared, public, owner-less notebook; a second import is
// an idempotent no-op; a DIFFERENT user can read it; then the per-user
// highlight bridge is exercised — user A's registration becomes A's reader
// highlight + A's quiz card and B's LookupWord suggestion, while B sees neither
// the highlight nor the card, and A's delete removes only A's copy.
// Postgres-only, so it skips without LANGNER_INTEGRATION_DB_URL.

// fakeGutendexClient serves canned Gutendex metadata + plain-text so ImportBook
// runs without real network (the dev container/CI has no egress). It routes by
// URL: the catalog metadata endpoint vs the plain-text download URL.
type fakeGutendexClient struct {
	sourceID string
	raw      string
}

func (c fakeGutendexClient) Do(req *http.Request) (*http.Response, error) {
	u := req.URL.String()
	body := ""
	switch {
	case strings.Contains(u, "/books/"+c.sourceID):
		// Gutendex metadata: a single plain-text UTF-8 format pointing at the
		// download URL the fake also serves below.
		body = fmt.Sprintf(`{"id":%s,"title":"A Small Invented Tale","authors":[{"name":"Jane Placeholder"}],"languages":["en"],"formats":{"text/plain; charset=utf-8":"https://example.test/%s.txt"}}`, c.sourceID, c.sourceID)
	case strings.Contains(u, "example.test"):
		body = c.raw
	default:
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
}

type booksFixture struct {
	svc             *quiz.Service
	notebookHandler *NotebookHandler
	db              *sqlx.DB
	userA           int64
	userB           int64
}

func newBooksFixture(t *testing.T) booksFixture {
	t.Helper()
	dsn := os.Getenv(integrationDBEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping live-Postgres Gutenberg-books coverage", integrationDBEnv)
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
		id, serr := dbseed.SeedUser(context.Background(), db, sub, "")
		require.NoError(t, serr)
		return id
	}
	userA := seedUser("book-user-a")
	userB := seedUser("book-user-b")

	// Fresh books have empty content dirs; the book is served purely from the DB
	// content source once imported, so the filesystem example dirs can be empty.
	nbCfg := config.NotebooksConfig{LearningNotesDirectory: t.TempDir()}
	quizCfg := config.QuizConfig{Algorithm: "modified_sm2", FixedIntervals: []int{1, 7, 30, 90, 365, 1095, 1825}, DisableShuffle: true}

	raw, err := os.ReadFile(filepath.Join(root, "backend", "internal", "gutenberg", "testdata", "sample.txt"))
	require.NoError(t, err)

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
	userDefsRepo := notebook.NewUserDefinitionsRepository(db)
	svc.SetContentSource(contentSource)
	svc.SetUserDefinitionsRepo(userDefsRepo)
	notebookHandler.SetPushDeps(db, fileRepo, contentSource)
	notebookHandler.SetBookDeps(userDefsRepo, fakeGutendexClient{sourceID: "77", raw: string(raw)})

	return booksFixture{svc: svc, notebookHandler: notebookHandler, db: db, userA: userA, userB: userB}
}

func TestImportBook_SharedPublicAndIdempotent_LivePostgres_Integration(t *testing.T) {
	f := newBooksFixture(t)
	ctx := testutil.WithTestUser(context.Background(), f.userA)

	resp, err := f.notebookHandler.ImportBook(ctx, connect.NewRequest(&apiv1.ImportBookRequest{SourceId: "77"}))
	require.NoError(t, err)
	assert.Equal(t, "gutenberg-77", resp.Msg.NotebookId)

	// Stored ONCE as a shared, public, owner-less 'shipped' notebook.
	var visibility, source string
	var owner *int64
	require.NoError(t, f.db.QueryRow(
		`SELECT visibility, source, owner_user_id FROM notebooks WHERE notebook_id = $1`, "gutenberg-77").
		Scan(&visibility, &source, &owner))
	assert.Equal(t, "public", visibility)
	assert.Equal(t, "shipped", source)
	assert.Nil(t, owner, "shared books are owner-less")

	// Files land under the "books" family.
	var family string
	require.NoError(t, f.db.Get(&family, `SELECT family FROM notebook_files WHERE notebook_id = $1 LIMIT 1`, "gutenberg-77"))
	assert.Equal(t, "books", family)

	var blobCount int
	require.NoError(t, f.db.Get(&blobCount, `SELECT COUNT(*) FROM notebook_files WHERE notebook_id = $1`, "gutenberg-77"))

	// A second import by a DIFFERENT user is an idempotent no-op (upsert by id).
	resp2, err := f.notebookHandler.ImportBook(testutil.WithTestUser(context.Background(), f.userB),
		connect.NewRequest(&apiv1.ImportBookRequest{SourceId: "77"}))
	require.NoError(t, err)
	assert.Equal(t, "gutenberg-77", resp2.Msg.NotebookId)

	var nbCount, blobCount2 int
	require.NoError(t, f.db.Get(&nbCount, `SELECT COUNT(*) FROM notebooks WHERE notebook_id = $1`, "gutenberg-77"))
	require.NoError(t, f.db.Get(&blobCount2, `SELECT COUNT(*) FROM notebook_files WHERE notebook_id = $1`, "gutenberg-77"))
	assert.Equal(t, 1, nbCount, "one shared copy")
	assert.Equal(t, blobCount, blobCount2, "re-import replaces blobs wholesale, no duplication")

	// A different user can READ the shared book.
	detail, err := f.notebookHandler.GetNotebookDetail(testutil.WithTestUser(context.Background(), f.userB),
		connect.NewRequest(&apiv1.GetNotebookDetailRequest{NotebookId: "gutenberg-77"}))
	require.NoError(t, err)
	require.NotEmpty(t, detail.Msg.Stories, "book must be readable")
}

func TestGutenbergBook_PerUserHighlightBridge_LivePostgres_Integration(t *testing.T) {
	f := newBooksFixture(t)
	_, err := f.notebookHandler.ImportBook(testutil.WithTestUser(context.Background(), f.userA),
		connect.NewRequest(&apiv1.ImportBookRequest{SourceId: "77"}))
	require.NoError(t, err)

	ctxA := testutil.WithTestUser(context.Background(), f.userA)
	ctxB := testutil.WithTestUser(context.Background(), f.userB)

	// --- User A registers a private highlight on CHAPTER I ("river"). ---
	_, err = f.notebookHandler.RegisterDefinition(ctxA, connect.NewRequest(&apiv1.RegisterDefinitionRequest{
		NotebookId:   "gutenberg-77",
		NotebookFile: "CHAPTER I", // matches the chapter event via the merge fallback
		SceneIndex:   0,
		Expression:   "river",
		Meaning:      "a flowing body of water",
		PartOfSpeech: "noun",
	}))
	require.NoError(t, err)

	hasRiver := func(resp *connect.Response[apiv1.GetNotebookDetailResponse]) bool {
		for _, s := range resp.Msg.Stories {
			for _, scene := range s.Scenes {
				for _, d := range scene.Definitions {
					if strings.EqualFold(d.Expression, "river") {
						return true
					}
				}
			}
		}
		return false
	}

	// A's reader bolds "river"; B's reader does NOT (per-user isolation).
	detailA, err := f.notebookHandler.GetNotebookDetail(ctxA, connect.NewRequest(&apiv1.GetNotebookDetailRequest{NotebookId: "gutenberg-77"}))
	require.NoError(t, err)
	assert.True(t, hasRiver(detailA), "A should see their registered word as a definition")

	detailB, err := f.notebookHandler.GetNotebookDetail(ctxB, connect.NewRequest(&apiv1.GetNotebookDetailRequest{NotebookId: "gutenberg-77"}))
	require.NoError(t, err)
	assert.False(t, hasRiver(detailB), "B must NOT see A's private registration")

	// A's quiz yields a due card for "river"; B's does not.
	cardsA, err := f.svc.LoadCards(f.userA, []string{"gutenberg-77"}, true, nil)
	require.NoError(t, err)
	assert.True(t, cardHasEntry(cardsA, "river"), "A should get a due card for the registered word")

	cardsB, err := f.svc.LoadCards(f.userB, []string{"gutenberg-77"}, true, nil)
	require.NoError(t, err)
	assert.False(t, cardHasEntry(cardsB, "river"), "B must NOT get A's card")

	// The quiz-hub listing must surface the book for A (its ONLY word is A's
	// registration, so the summary has to count the per-user overlay or the book
	// shows 0 words and gets filtered out of the picker). B's summary must not.
	summariesA, err := f.svc.LoadNotebookSummaries(f.userA, true)
	require.NoError(t, err)
	assert.Positive(t, bookVocabCount(summariesA, "gutenberg-77"),
		"A's quiz-hub summary must count the registered word so the book is listed")
	summariesB, err := f.svc.LoadNotebookSummaries(f.userB, true)
	require.NoError(t, err)
	assert.Zero(t, bookVocabCount(summariesB, "gutenberg-77"),
		"B registered nothing, so the shared book has no words for B")

	// --- SRS persistence: a correct answer on the registered card MUST bind to
	// its scene so the word is not forever "unstudied". Regression guard for the
	// notes→content history binding: RegisterDefinition links notebook_notes with
	// Group=event title + Subgroup=scene title (resolved from content), which is
	// exactly what GetNotebookDetail's GetLogs(event, scene.Title, def) and the
	// history reconstructor key on. With the earlier Subgroup="" the log
	// reconstructed under scene "" and never matched, so LearnedLogs stayed empty
	// after a correct submit (the card was permanently unstudied). ---
	var riverCard quiz.Card
	for _, c := range cardsA {
		if strings.EqualFold(c.Entry, "river") {
			riverCard = c
			break
		}
	}
	require.True(t, strings.EqualFold(riverCard.Entry, "river"), "precondition: A has a river card to answer")
	require.NoError(t, f.svc.SaveResult(ctxA, f.userA, riverCard,
		quiz.GradeResult{Correct: true, Quality: int(notebook.QualityCorrect)}, 1200))

	detailAfterAnswer, err := f.notebookHandler.GetNotebookDetail(ctxA, connect.NewRequest(&apiv1.GetNotebookDetailRequest{NotebookId: "gutenberg-77"}))
	require.NoError(t, err)
	riverLogged := false
	for _, s := range detailAfterAnswer.Msg.Stories {
		for _, scene := range s.Scenes {
			for _, d := range scene.Definitions {
				if strings.EqualFold(d.Expression, "river") && len(d.LearnedLogs) > 0 {
					riverLogged = true
				}
			}
		}
	}
	assert.True(t, riverLogged, "a correct answer must bind to the registered word's scene so its SRS state persists")

	// B's LookupWord returns A's meaning as a shared suggestion with the count.
	look, err := f.notebookHandler.LookupWord(ctxB, connect.NewRequest(&apiv1.LookupWordRequest{
		Word: "river", NotebookId: "gutenberg-77",
	}))
	require.NoError(t, err)
	require.Len(t, look.Msg.Suggestions, 1)
	assert.Equal(t, "a flowing body of water", look.Msg.Suggestions[0].Meaning)
	assert.Equal(t, int32(1), look.Msg.Suggestions[0].SavedByCount)

	// --- A deletes their registration; only A's copy goes. ---
	_, err = f.notebookHandler.DeleteDefinition(ctxA, connect.NewRequest(&apiv1.DeleteDefinitionRequest{
		NotebookId: "gutenberg-77", Expression: "river",
	}))
	require.NoError(t, err)

	detailA2, err := f.notebookHandler.GetNotebookDetail(ctxA, connect.NewRequest(&apiv1.GetNotebookDetailRequest{NotebookId: "gutenberg-77"}))
	require.NoError(t, err)
	assert.False(t, hasRiver(detailA2), "A's registration should be gone after delete")

	// The shared book content is still fully readable for everyone.
	require.NotEmpty(t, detailB.Msg.Stories)
}

// bookVocabCount returns the VocabularyCount the quiz-hub summary reports for
// notebookID, or 0 when the notebook isn't listed at all.
func bookVocabCount(summaries []quiz.NotebookSummary, notebookID string) int {
	for _, s := range summaries {
		if s.NotebookID == notebookID {
			return int(s.VocabularyCount)
		}
	}
	return 0
}

func cardHasEntry(cards []quiz.Card, entry string) bool {
	for _, c := range cards {
		if strings.EqualFold(c.Entry, entry) || strings.EqualFold(c.OriginalEntry, entry) {
			return true
		}
	}
	return false
}
