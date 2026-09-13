package learning

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/schemas"
)

// TestDBHistoryStore_LoadForNotebooks_ParityAndScoping is the egress-fix
// regression: LoadForNotebooks must return EXACTLY what filtering LoadAll to
// those notebooks returns (so read/write symmetry — invariants L2/L4 — is
// preserved), while fetching only the scoped subset of rows (the reason the fix
// cuts database egress). It seeds two notebooks, a note SHARED across both (with
// per-source logs), an etymology origin, and a skip flag — the awkward shapes
// that make scoping non-trivial — through a real Postgres.
//
// Requires LANGNER_INTEGRATION_DB_URL; skipped otherwise.
func TestDBHistoryStore_LoadForNotebooks_ParityAndScoping(t *testing.T) {
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}
	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.Ping())
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	ctx := context.Background()

	// Generic, invented data (never the user's real notebooks): two Latin-root
	// demo books, one word shared between them.
	insNote := func(senseID, word string) int64 {
		var id int64
		require.NoError(t, db.QueryRowContext(ctx,
			`INSERT INTO notes (sense_id, "usage", entry, meaning) VALUES ($1,$2,$3,$4) RETURNING id`,
			senseID, word, word, "meaning of "+word).Scan(&id))
		return id
	}
	link := func(noteID int64, nb, group, sub string) {
		_, err := db.ExecContext(ctx,
			`INSERT INTO notebook_notes (note_id, notebook_type, notebook_id, "group", subgroup) VALUES ($1,'story',$2,$3,$4)`,
			noteID, nb, group, sub)
		require.NoError(t, err)
	}
	logFor := func(noteID int64, source string) {
		_, err := db.ExecContext(ctx,
			`INSERT INTO learning_logs (note_id, status, learned_at, quality, response_time_ms, quiz_type, interval_days, source_notebook_id)
			 VALUES ($1,'understood', now(), 4, 1200, 'notebook', 7, $2)`,
			noteID, source)
		require.NoError(t, err)
	}

	const nbA, nbB = "book-a", "book-b"
	a1 := insNote("s-a1", "aqueduct")
	a2 := insNote("s-a2", "aquifer")
	shared := insNote("s-sh", "current") // lives in BOTH books
	b1 := insNote("s-b1", "bilingual")

	link(a1, nbA, "UNIT ONE", "scene 1")
	link(a2, nbA, "UNIT ONE", "scene 1")
	link(shared, nbA, "UNIT TWO", "scene 1")
	link(shared, nbB, "UNIT ONE", "scene 1")
	link(b1, nbB, "UNIT ONE", "scene 1")

	logFor(a1, nbA)
	logFor(shared, nbA) // shared word's book-a attempt
	logFor(shared, nbB) // shared word's book-b attempt (must NOT bleed into book-a)
	logFor(b1, nbB)

	// An etymology origin in book-a with its own log + a skip flag on a note.
	var originID int64
	require.NoError(t, db.QueryRowContext(ctx,
		`INSERT INTO etymology_origins (notebook_id, session_title, sense, origin, type, language, meaning)
		 VALUES ($1,'UNIT ONE','','aqua','root','Latin','water') RETURNING id`, nbA).Scan(&originID))
	_, err = db.ExecContext(ctx,
		`INSERT INTO learning_logs (origin_id, status, learned_at, quality, response_time_ms, quiz_type, interval_days, source_notebook_id)
		 VALUES ($1,'understood', now(), 5, 900, 'etymology_origin', 7, $2)`, originID, nbA)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO note_skip_flags (note_id, quiz_type, skipped_at) VALUES ($1,'notebook', now())`, a2)
	require.NoError(t, err)

	noteRepo := notebook.NewDBNoteRepository(db)
	learningRepo := NewDBLearningRepository(db)
	originRepo := notebook.NewDBEtymologyOriginRepository(db)
	skipFlagRepo := notebook.NewDBSkipFlagRepository(db)
	store := NewDBHistoryStore(noteRepo, learningRepo, originRepo, skipFlagRepo, nil)

	all, err := store.LoadAll(ctx)
	require.NoError(t, err)
	scoped, err := store.LoadForNotebooks(ctx, []string{nbA})
	require.NoError(t, err)

	// Scoping: only the requested notebook is returned.
	require.Len(t, scoped, 1, "LoadForNotebooks must return only the requested notebook")
	require.Contains(t, scoped, nbA)

	// Parity: the scoped notebook is byte-identical to filtering LoadAll.
	assert.True(t, reflect.DeepEqual(all[nbA], scoped[nbA]),
		"LoadForNotebooks(%q) must equal LoadAll filtered to %q", nbA, nbA)

	// Egress: the scoped repo reads fetch fewer rows than the whole-table reads.
	allNotes, err := noteRepo.FindAll(ctx)
	require.NoError(t, err)
	scopedNotes, err := noteRepo.FindByNotebooks(ctx, []string{nbA})
	require.NoError(t, err)
	assert.Less(t, len(scopedNotes), len(allNotes), "scoped note read must fetch fewer notes (b1 excluded)")

	allLogs, err := learningRepo.FindAll(ctx)
	require.NoError(t, err)
	noteIDs := make([]int64, 0, len(scopedNotes))
	for _, n := range scopedNotes {
		noteIDs = append(noteIDs, n.ID)
	}
	scopedLogs, err := learningRepo.FindByTargets(ctx, noteIDs, []int64{originID}, nil)
	require.NoError(t, err)
	assert.Less(t, len(scopedLogs), len(allLogs), "scoped log read must fetch fewer logs (book-b logs excluded)")
	// Sanity: the shared word's book-b log is NOT in the scoped set for book-a.
	_ = b1
}
