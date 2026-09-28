package dbseed_test

import (
	"context"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/dbseed"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/schemas"
)

const integrationDBEnv = "LANGNER_INTEGRATION_DB_URL"

// Verifies the fixtures library end-to-end against a live Postgres: SeedUser +
// SetNotebookOwner + SeedLearningLog (which goes through the REAL write path,
// resolving/creating the note) produce a well-formed, queryable learning log.
func TestDBSeed_LearningLog_ThroughRealWritePath(t *testing.T) {
	dsn := os.Getenv(integrationDBEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping live-Postgres dbseed coverage", integrationDBEnv)
	}
	ctx := context.Background()
	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	userID, err := dbseed.SeedUser(ctx, db, "dbseed-user", "")
	require.NoError(t, err)
	require.NoError(t, dbseed.SetNotebookOwner(ctx, db, "roots-mini", &userID, "private", "shipped"))

	when := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	require.NoError(t, dbseed.SeedLearningLog(ctx, db, dbseed.LearningLogSpec{
		UserID:     userID,
		NotebookID: "roots-mini",
		Group:      "Common Idioms",
		Expression: "break the ice",
		Status:     "misunderstood",
		Quality:    1,
		LearnedAt:  when,
	}))

	// The log is queryable and joins to a note carrying the seeded expression —
	// exactly what the analytics Day Detail reads.
	var got struct {
		Entry      string `db:"entry"`
		UserID     int64  `db:"user_id"`
		QuizType   string `db:"quiz_type"`
		SourceNB   string `db:"source_notebook_id"`
		LearnedDay string `db:"learned_day"`
	}
	require.NoError(t, db.Get(&got, `
		SELECT n.entry, l.user_id, l.quiz_type, l.source_notebook_id,
		       to_char(l.learned_at, 'YYYY-MM-DD') AS learned_day
		FROM learning_logs l JOIN notes n ON n.id = l.note_id
		WHERE l.source_notebook_id = 'roots-mini'`))
	assert.Equal(t, "break the ice", got.Entry)
	assert.Equal(t, userID, got.UserID)
	assert.Equal(t, "notebook", got.QuizType)
	assert.Equal(t, "2025-01-02", got.LearnedDay, "the seeded date is preserved (analytics Day Detail keys on it)")

	// Regression guard: a content-free seed must ALSO register the note's
	// notebook_notes membership, or the DB history reconstruction
	// (DBNoteRepository.FindByNotebooks → DBHistoryStore.LoadForNotebooks)
	// finds no vocab for the notebook and the word silently vanishes from the
	// Learn page, the quiz due/reverse counts, and analytics Day Detail. The
	// membership's title (`group`) MUST equal the on-disk notebook title so the
	// quiz loaders — which match content cards to history BY TITLE — attach.
	var mem struct {
		NotebookType string `db:"notebook_type"`
		Group        string `db:"group"`
	}
	require.NoError(t, db.Get(&mem, `
		SELECT nn.notebook_type, nn."group"
		FROM notebook_notes nn JOIN notes n ON n.id = nn.note_id
		WHERE nn.notebook_id = 'roots-mini' AND n.entry = 'break the ice'`))
	assert.Equal(t, "flashcard", mem.NotebookType)
	assert.Equal(t, "Common Idioms", mem.Group, "membership title must match the on-disk notebook title so quiz loaders attach")

	// Drive the REAL reconstruction the running server reads through: the word
	// must reappear under its notebook with its log, keyed by the same title.
	store := learning.NewDBHistoryStore(
		notebook.NewDBNoteRepository(db),
		learning.NewDBLearningRepository(db),
		notebook.NewDBEtymologyOriginRepository(db),
		notebook.NewDBSkipFlagRepository(db),
		notebook.NewDBGrammarCorrectionRepository(db),
	)
	histories, err := store.LoadForNotebooks(ctx, []string{"roots-mini"}, userID)
	require.NoError(t, err)
	var found bool
	for _, h := range histories["roots-mini"] {
		if h.Metadata.Title != "Common Idioms" {
			continue
		}
		for _, e := range h.Expressions {
			if e.Expression == "break the ice" && len(e.LearnedLogs) > 0 {
				found = true
			}
		}
	}
	assert.True(t, found, "reconstruction must surface the seeded vocab word under its titled notebook with its log")
}
