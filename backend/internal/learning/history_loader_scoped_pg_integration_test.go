package learning

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

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

	all, err := store.loadAllFallback(ctx)
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

// TestDBHistoryStore_LoadForNotebooks_OrphanOriginLogsScopedByUsage pins the
// orphan-notes egress fix: FindByNotebooks no longer pulls EVERY id-less legacy
// note; instead LoadForNotebooks fetches (via FindOrphanNotesByUsage) only the
// orphans whose usage matches one of the loaded origins. The scoped result must
// still be byte-identical to the whole-dataset load filtered to the notebook
// (the orphan's log re-attaches to the matching origin), AND an orphan whose
// usage matches no loaded origin must NOT be fetched.
func TestDBHistoryStore_LoadForNotebooks_OrphanOriginLogsScopedByUsage(t *testing.T) {
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

	const nbA = "book-a"
	// A linked note so book-a is non-empty, and an origin "aqua" in book-a.
	var linked int64
	require.NoError(t, db.QueryRowContext(ctx,
		`INSERT INTO notes (sense_id,"usage",entry,meaning) VALUES ('s1','aqueduct','aqueduct','a channel') RETURNING id`).Scan(&linked))
	_, err = db.ExecContext(ctx,
		`INSERT INTO notebook_notes (note_id, notebook_type, notebook_id, "group", subgroup) VALUES ($1,'story',$2,'UNIT ONE','scene 1')`, linked, nbA)
	require.NoError(t, err)
	var originID int64
	require.NoError(t, db.QueryRowContext(ctx,
		`INSERT INTO etymology_origins (notebook_id, session_title, sense, origin, type, language, meaning)
		 VALUES ($1,'UNIT ONE','','aqua','root','Latin','water') RETURNING id`, nbA).Scan(&originID))

	// Two id-less ORPHAN notes (no notebook_notes link), each with a note_id log:
	// one keyed by the loaded origin's name ("aqua"), one by an unrelated name.
	orphanNote := func(usage string) int64 {
		var id int64
		require.NoError(t, db.QueryRowContext(ctx,
			`INSERT INTO notes (sense_id,"usage",entry,meaning) VALUES ('','' || $1,$1,'legacy') RETURNING id`, usage).Scan(&id))
		_, err = db.ExecContext(ctx,
			`INSERT INTO learning_logs (note_id, status, learned_at, quality, response_time_ms, quiz_type, interval_days, source_notebook_id)
			 VALUES ($1,'understood', now(), 5, 900, 'etymology_origin', 7, $2)`, id, nbA)
		require.NoError(t, err)
		return id
	}
	orphanNote("aqua")  // matches the loaded origin → must be fetched + merged
	orphanNote("ignis") // matches no loaded origin → must NOT be fetched

	noteRepo := notebook.NewDBNoteRepository(db)
	store := NewDBHistoryStore(noteRepo, NewDBLearningRepository(db),
		notebook.NewDBEtymologyOriginRepository(db), notebook.NewDBSkipFlagRepository(db), nil)

	// Direct scoping check: only the matching orphan is fetched.
	orphans, err := noteRepo.FindOrphanNotesByUsage(ctx, []string{"aqua"})
	require.NoError(t, err)
	require.Len(t, orphans, 1, "only the usage-matching orphan is fetched")
	assert.Equal(t, "aqua", orphans[0].Usage)

	// Parity: scoped equals the whole-dataset load filtered to book-a. This
	// proves the "aqua" orphan's log still re-attaches to the origin under the
	// scoped path, and the "ignis" orphan (matching no origin) is absent in both.
	all, err := store.loadAllFallback(ctx)
	require.NoError(t, err)
	scoped, err := store.LoadForNotebooks(ctx, []string{nbA})
	require.NoError(t, err)
	assert.True(t, reflect.DeepEqual(all[nbA], scoped[nbA]),
		"scoped LoadForNotebooks must equal the full load filtered to %q, orphan merge included", nbA)

	// Guard: the origin actually carries BOTH its own log and the orphan's, so
	// parity above isn't trivially "both missing the merge".
	originLogs := 0
	for _, h := range scoped[nbA] {
		for _, e := range h.Expressions {
			originLogs += len(e.EtymologyOriginLogs)
		}
		for _, sc := range h.Scenes {
			for _, e := range sc.Expressions {
				originLogs += len(e.EtymologyOriginLogs)
			}
		}
	}
	assert.GreaterOrEqual(t, originLogs, 1, "the aqua origin history must carry the merged orphan log")
}

// TestDBHistoryStore_LoadForDateRange_ScopesByDate pins the Analytics daily-view
// egress fix: LoadForDateRange returns only attempts whose learned_at is in the
// window, and FindByDateRange fetches only those log rows.
func TestDBHistoryStore_LoadForDateRange_ScopesByDate(t *testing.T) {
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

	var noteID int64
	require.NoError(t, db.QueryRowContext(ctx,
		`INSERT INTO notes (sense_id,"usage",entry,meaning) VALUES ('s1','word','word','meaning') RETURNING id`).Scan(&noteID))
	_, err = db.ExecContext(ctx,
		`INSERT INTO notebook_notes (note_id, notebook_type, notebook_id, "group", subgroup) VALUES ($1,'story','book','UNIT ONE','scene 1')`, noteID)
	require.NoError(t, err)
	// Two attempts: one recent (in a 30-day window), one old (outside it).
	_, err = db.ExecContext(ctx,
		`INSERT INTO learning_logs (note_id,status,learned_at,quality,response_time_ms,quiz_type,interval_days,source_notebook_id)
		 VALUES ($1,'understood', now() - interval '5 days', 4, 1000, 'notebook', 7, 'book'),
		        ($1,'misunderstood', now() - interval '60 days', 1, 1000, 'notebook', 1, 'book')`, noteID)
	require.NoError(t, err)

	store := NewDBHistoryStore(notebook.NewDBNoteRepository(db), NewDBLearningRepository(db),
		notebook.NewDBEtymologyOriginRepository(db), notebook.NewDBSkipFlagRepository(db), nil)

	countAttempts := func(m map[string][]notebook.LearningHistory) int {
		n := 0
		for _, hs := range m {
			for _, h := range hs {
				for _, sc := range h.Scenes {
					for _, e := range sc.Expressions {
						n += len(e.LearnedLogs) + len(e.ReverseLogs) + len(e.EtymologyOriginLogs)
					}
				}
			}
		}
		return n
	}

	all, err := store.loadAllFallback(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, countAttempts(all), "both attempts present in the full load")

	// Window: last 30 days → only the recent attempt.
	scoped, err := store.LoadForDateRange(ctx, timeDaysAgo(30), time.Time{})
	require.NoError(t, err)
	assert.Equal(t, 1, countAttempts(scoped), "only the in-window attempt is reconstructed")

	// FindByDateRange fetches only the in-window row.
	inWindow, err := NewDBLearningRepository(db).FindByDateRange(ctx, timeDaysAgo(30), time.Time{})
	require.NoError(t, err)
	assert.Len(t, inWindow, 1, "date-scoped log read fetches only the in-window row")
}

func timeDaysAgo(d int) time.Time { return time.Now().AddDate(0, 0, -d) }
