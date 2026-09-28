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
}
