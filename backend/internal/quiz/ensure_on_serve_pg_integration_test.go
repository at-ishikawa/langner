package quiz_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/bootstrap"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/datasync"
	"github.com/at-ishikawa/langner/internal/dbseed"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/inference/mock"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/schemas"
)

// These tests reproduce and pin the DB-mode drift bug: a notebook whose YAML
// declares words the DB has no notes row for (the operator added units without
// re-running `migrate import-db`). The words are quizzed straight from YAML, so
// any note-keyed DB write — the deliberate Exclude (SkipWord) is the reported
// one — fails with "no matching note or origin in notebook ...". ensure-on-serve
// creates the missing notes additively when the cards are loaded, self-healing
// it. Everything runs through the real config.example.yml construction + a live
// Postgres, per verify-data-features-with-example-notebooks (no hand-built
// NoteRecord/Card fixtures — those bypass the load path that actually diverges).
//
// Requires a reachable Postgres; the suite self-skips when LANGNER_INTEGRATION_DB_URL
// is unset. The fixture is the shipped example notebook examples/definitions/
// latin-roots-book (id "latin-roots-book"), which has a fixed set of definition
// words — never the operator's real vocabulary.
const ensureIntegrationDBEnv = "LANGNER_INTEGRATION_DB_URL"

const ensureBookID = "latin-roots-book"

// ensureFixture holds the wiring shared by the ensure-on-serve tests: a freshly
// migrated DB, the DB state repos, and the notebook config from
// config.example.yml (with CWD moved to the repo root so its relative notebook
// dirs resolve exactly as the server sees them).
type ensureFixture struct {
	db     *sqlx.DB
	repos  bootstrap.StateRepositories
	cfg    *config.Config
	userID int64
}

func setupEnsureFixture(t *testing.T) ensureFixture {
	t.Helper()
	dsn := os.Getenv(ensureIntegrationDBEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping Postgres-backed ensure-on-serve test", ensureIntegrationDBEnv)
	}

	// Move to the repo root so config.example.yml's relative notebook
	// directories (examples/…) resolve the same way they do for the server.
	root := findRepoRootForEnsure(t)
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

	repos := bootstrap.BuildStateRepositories(cfg.Notebooks, cfg.Quiz, db)
	userID, err := dbseed.SeedUser(context.Background(), db, "ensure-notes-test", "")
	require.NoError(t, err)
	return ensureFixture{db: db, repos: repos, cfg: cfg, userID: userID}
}

// newImporter builds the same notes-only importer main.go injects as the
// ensure-on-serve hook: the DB note repo + a YAML note source over the example
// notebooks. Nil for every phase the note path doesn't touch.
func (f ensureFixture) newImporter(t *testing.T) *datasync.Importer {
	t.Helper()
	reader, err := notebook.NewReader(
		f.cfg.Notebooks.StoriesDirectories,
		f.cfg.Notebooks.FlashcardsDirectories,
		f.cfg.Notebooks.BooksDirectories,
		f.cfg.Notebooks.DefinitionsDirectories,
		f.cfg.Notebooks.EtymologyDirectories,
		nil,
	)
	require.NoError(t, err)
	noteSource := notebook.NewYAMLNoteRepository(reader)
	return datasync.NewImporter(f.repos.Note, nil, noteSource, nil, nil, nil, io.Discard)
}

// newService builds the quiz.Service as the server does: notebooks are read
// from the Postgres content tables.
func (f ensureFixture) newService(t *testing.T) *quiz.Service {
	t.Helper()
	svc := quiz.NewService(f.cfg.Notebooks, inference.StaticResolver(mock.NewClient()), map[string]rapidapi.Response{}, f.repos.Learning, f.cfg.Quiz)
	svc.SetHistoryStore(f.repos.HistoryStore)
	svc.SetSkipStores(f.repos.SkipFlags, f.repos.Note, f.repos.Origin)
	svc.SetContent(notebook.NewContentRepository(f.db), nil)
	return svc
}

// storeBook stores the example book's content the way an import or push does,
// optionally creating its notes.
func (f ensureFixture) storeBook(t *testing.T, withNotes bool) {
	t.Helper()
	ctx := context.Background()
	contents, err := notebook.LoadDirectoryContent(notebook.ContentDirs{
		Definitions: f.cfg.Notebooks.DefinitionsDirectories,
		Etymology:   f.cfg.Notebooks.EtymologyDirectories,
	})
	require.NoError(t, err)
	for _, nc := range contents {
		if nc.ID != ensureBookID {
			continue
		}
		require.NoError(t, notebook.NewNotebookStore(f.db).ImportShippedBundle(ctx, "composite", nc.ID, "", 0, nc))
		if withNotes {
			_, err := datasync.EnsureContentNotes(ctx, f.db, nc)
			require.NoError(t, err)
		}
		return
	}
	t.Fatalf("example notebook %s not found", ensureBookID)
}

func (f ensureFixture) countNotebookNotes(t *testing.T, notebookID string) int {
	t.Helper()
	var n int
	require.NoError(t, f.db.Get(&n, `SELECT count(*) FROM notebook_notes WHERE notebook_id = $1`, notebookID))
	return n
}

func (f ensureFixture) countSkipFlags(t *testing.T, notebookID string) int {
	t.Helper()
	var n int
	require.NoError(t, f.db.Get(&n,
		`SELECT count(*) FROM note_skip_flags sf
		   JOIN notebook_notes nn ON nn.note_id = sf.note_id
		  WHERE nn.notebook_id = $1`, notebookID))
	return n
}

func findRepoRootForEnsure(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "config.example.yml")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "config.example.yml not found walking up from test dir")
		dir = parent
	}
}

// loadFirstCard loads the standard-quiz cards for the book and returns the first
// one, mirroring what StartQuiz stores in noteStore. includeUnstudied=true so a
// fresh DB (no SRS history) still yields the words.
func loadFirstCard(t *testing.T, svc *quiz.Service) quiz.Card {
	t.Helper()
	cards, err := svc.LoadCards(0, []string{ensureBookID}, true, nil)
	require.NoError(t, err)
	require.NotEmpty(t, cards, "the example definitions book must yield cards")
	return cards[0]
}

// TestStoredNotebook_HasNotes_ExcludeWorks: a notebook stored without its
// notes (the drift state: words quizzed from content with no notes rows) makes
// Exclude fail; storing it the way import/push do creates the notes, so Exclude
// works with no separate import step.
func TestStoredNotebook_HasNotes_ExcludeWorks(t *testing.T) {
	f := setupEnsureFixture(t)

	t.Run("without notes Exclude fails", func(t *testing.T) {
		f.storeBook(t, false)
		require.Equal(t, 0, f.countNotebookNotes(t, ensureBookID), "precondition: DB has no notes for the book")
		svc := f.newService(t)
		card := loadFirstCard(t, svc)
		err := svc.SkipWord(f.userID, quiz.CardInfoFromCard(card), "", []notebook.QuizType{notebook.QuizTypeNotebook})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no matching note or origin")
	})

	t.Run("storing creates the notes and Exclude succeeds", func(t *testing.T) {
		f.storeBook(t, true)
		assert.Greater(t, f.countNotebookNotes(t, ensureBookID), 0, "storing must create the book's notes")
		svc := f.newService(t)
		card := loadFirstCard(t, svc)
		require.NoError(t, svc.SkipWord(f.userID, quiz.CardInfoFromCard(card), "", []notebook.QuizType{notebook.QuizTypeNotebook}))
		assert.Equal(t, 1, f.countSkipFlags(t, ensureBookID), "the skip flag landed on the created note")
		require.NoError(t, svc.ResumeWord(f.userID, quiz.CardInfoFromCard(card), []notebook.QuizType{notebook.QuizTypeNotebook}))
		assert.Equal(t, 0, f.countSkipFlags(t, ensureBookID))
	})
}

// TestEnsureNotesForNotebook_AdditiveNoClobber pins that ensure creates EXACTLY
// the missing notes and never deletes or mutates existing rows or DB-only state
// (a learning log / skip flag on a kept note survives).
func TestEnsureNotesForNotebook_AdditiveNoClobber(t *testing.T) {
	f := setupEnsureFixture(t)
	ctx := context.Background()
	imp := f.newImporter(t)

	// Full ensure from empty → the whole book.
	full, err := imp.EnsureNotesForNotebook(ctx, ensureBookID)
	require.NoError(t, err)
	require.Greater(t, full, 1, "book must have several notes")
	total := f.countNotebookNotes(t, ensureBookID)
	require.Equal(t, full, total)

	// Seed DB-only STATE on a note we will KEEP: a skip flag via the real path.
	f.storeBook(t, false)
	svc := f.newService(t)
	cards, err := svc.LoadCards(f.userID, []string{ensureBookID}, true, nil)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(cards), 2)
	keep, drop := cards[0], cards[1]
	require.NoError(t, svc.SkipWord(f.userID, quiz.CardInfoFromCard(keep), "", []notebook.QuizType{notebook.QuizTypeNotebook}))
	require.Equal(t, 1, f.countSkipFlags(t, ensureBookID))

	// Simulate drift: delete a DIFFERENT note's row (as if that unit was added
	// to YAML after the last import). Delete the notebook_notes link + note.
	var dropNoteID int64
	require.NoError(t, f.db.Get(&dropNoteID,
		`SELECT nn.note_id FROM notebook_notes nn JOIN notes n ON n.id = nn.note_id
		  WHERE nn.notebook_id = $1 AND (n.sense_id = $2 OR lower(n.entry) = lower($3)) LIMIT 1`,
		ensureBookID, drop.ID, drop.Entry))
	_, err = f.db.Exec(`DELETE FROM notebook_notes WHERE note_id = $1`, dropNoteID)
	require.NoError(t, err)
	_, err = f.db.Exec(`DELETE FROM notes WHERE id = $1`, dropNoteID)
	require.NoError(t, err)
	require.Equal(t, total-1, f.countNotebookNotes(t, ensureBookID), "one unit is now missing (drift)")

	// Re-ensure: recreates EXACTLY the one missing note, nothing else.
	created, err := imp.EnsureNotesForNotebook(ctx, ensureBookID)
	require.NoError(t, err)
	assert.Equal(t, 1, created, "exactly the missing note is recreated")
	assert.Equal(t, total, f.countNotebookNotes(t, ensureBookID))

	// The kept note's skip flag is untouched — ensure never deletes DB state.
	assert.Equal(t, 1, f.countSkipFlags(t, ensureBookID), "existing skip flag survived the ensure")

	// The recreated word is now excludable end-to-end.
	require.NoError(t, svc.SkipWord(f.userID, quiz.CardInfoFromCard(drop), "", []notebook.QuizType{notebook.QuizTypeNotebook}))
	assert.Equal(t, 2, f.countSkipFlags(t, ensureBookID))
}

// TestEnsureNotesForNotebook_Concurrency proves two simultaneous serves of the
// same new notebook do not double-insert (relies on the ON CONFLICT DO NOTHING
// path in BatchCreate / insertNoteReturningID).
func TestEnsureNotesForNotebook_Concurrency(t *testing.T) {
	f := setupEnsureFixture(t)
	ctx := context.Background()

	const goroutines = 6
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	// Independent importers (each with its own connection-sharing repo) mirror
	// separate concurrent sessions.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			imp := f.newImporter(t)
			_, errs[i] = imp.EnsureNotesForNotebook(ctx, ensureBookID)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		require.NoErrorf(t, err, "goroutine %d", i)
	}

	// Exactly one set of notes exists — no duplicates.
	single := f.newImporter(t)
	full, err := single.EnsureNotesForNotebook(ctx, ensureBookID)
	require.NoError(t, err)
	assert.Equal(t, 0, full, "after concurrent ensure the notebook is fully synced")

	var distinctNotes int
	require.NoError(t, f.db.Get(&distinctNotes,
		`SELECT count(DISTINCT note_id) FROM notebook_notes WHERE notebook_id = $1`, ensureBookID))
	assert.Equal(t, f.countNotebookNotes(t, ensureBookID), distinctNotes, "no duplicate notebook_notes")
}
