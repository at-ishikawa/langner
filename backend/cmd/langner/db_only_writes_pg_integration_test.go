package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/bootstrap"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/datasync"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/at-ishikawa/langner/schemas"
)

// copyLearningNotes copies every *.yml from src into a fresh temp dir and
// returns it. The temp dir stands in for the runtime learning_notes directory
// so the test can assert the running server never rewrites it in DB mode.
func copyLearningNotes(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	require.NoError(t, err)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, rerr := os.ReadFile(filepath.Join(src, e.Name()))
		require.NoError(t, rerr)
		require.NoError(t, os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644))
	}
	return dst
}

// snapshotDir hashes every file under dir (path -> content) so a test can
// assert the directory is byte-for-byte unchanged after an operation.
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		out[path] = string(b)
		return nil
	})
	require.NoError(t, err)
	return out
}

// candidateNote is a note that carries a notebook membership but has no
// learning logs yet — a clean target so the attempt this test writes becomes
// the word's latest (LearnedLogs[0]) status without the DB-ordering caveat
// that trips consumers reading logs[0] on a word with prior imported logs.
type candidateNote struct {
	ID         int64  `db:"id"`
	Usage      string `db:"usage"`
	Entry      string `db:"entry"`
	SenseID    string `db:"sense_id"`
	NotebookID string `db:"notebook_id"`
}

// findExpressionInHistories locates the reconstructed learning-history
// expression for a word across the story-scene, flashcard, and flat shapes the
// DBHistoryStore emits. The expression is keyed by the note's entry (or usage
// when entry is empty), matching newExpressionFromNote.
func findExpressionInHistories(histories []notebook.LearningHistory, usage, entry string) (notebook.LearningHistoryExpression, bool) {
	match := func(e notebook.LearningHistoryExpression) bool {
		return e.Expression == entry || e.Expression == usage
	}
	for _, h := range histories {
		for _, e := range h.Expressions {
			if match(e) {
				return e, true
			}
		}
		for _, sc := range h.Scenes {
			for _, e := range sc.Expressions {
				if match(e) {
					return e, true
				}
			}
		}
	}
	return notebook.LearningHistoryExpression{}, false
}

// TestServerDBOnlyWrites_FreezesLearningNotesYAML_LivePostgres_Integration is
// the end-to-end proof the owner asked for: with a database configured, a real
// quiz-result save driven through the quiz Service — wired the SAME way
// langner-server wires it (bootstrap.BuildStateRepositories) — persists to the
// DATABASE ONLY and never rewrites the on-disk learning_notes YAML.
//
// It asserts all three of the owner's conditions:
//
//	(a) the learning_notes directory is byte-for-byte unchanged (YAML writes
//	    stopped) — the freeze proof;
//	(b) the DB's learning_logs gained the expected row (the attempt persisted);
//	(c) reading back through DBHistoryStore reflects the attempt (writes land
//	    where reads look — learning-history invariant L2).
//
// A CONTROL section drives the same save through the PRE-CHANGE wiring
// (MultiLearningRepository = YAML + DB) and asserts that DOES rewrite YAML —
// so this test FAILS the freeze assertion on the old wiring and PASSES on the
// new one, the before/after the owner wants demonstrated, here against a real
// Postgres.
//
// Requires LANGNER_INTEGRATION_DB_URL (CI's postgres:16); skipped otherwise.
// DROPs and recreates the public schema, so it must run isolated (own workflow
// step, or -p 1).
func TestServerDBOnlyWrites_FreezesLearningNotesYAML_LivePostgres_Integration(t *testing.T) {
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}

	// Run from the repo root so config.example.yml's relative notebook dirs
	// resolve, exactly as the server does.
	root := findRepoRoot(t)
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())

	// Fresh schema, then seed the DB with the real example notebooks through
	// the SAME importer import-db uses (not a hand-built fixture).
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	loader, err := config.NewConfigLoader("config.example.yml")
	require.NoError(t, err)
	cfg, err := loader.Load()
	require.NoError(t, err)

	importer := newImporterFromConfig(cfg, db, io.Discard)
	_, err = importer.ImportAll(context.Background(), datasync.ImportOptions{})
	require.NoError(t, err)

	ctx := context.Background()

	// Two clean target notes (a notebook membership, no logs yet): one for the
	// DB-only assertion, one for the dual-write control.
	var candidates []candidateNote
	require.NoError(t, db.SelectContext(ctx, &candidates, `
		SELECT n.id, n."usage", n.entry, n.sense_id, nn.notebook_id
		FROM notes n
		JOIN notebook_notes nn ON nn.note_id = n.id
		LEFT JOIN learning_logs ll ON ll.note_id = n.id
		WHERE ll.id IS NULL
		ORDER BY n.id
		LIMIT 2`))
	require.GreaterOrEqual(t, len(candidates), 2,
		"the example notebooks must contain at least two never-quizzed words")
	target := candidates[0]
	control := candidates[1]

	cardFor := func(c candidateNote) quiz.Card {
		orig := ""
		if c.Entry != c.Usage {
			orig = c.Entry
		}
		return quiz.Card{
			ID:            c.SenseID,
			NotebookName:  c.NotebookID,
			Entry:         c.Usage,
			OriginalEntry: orig,
		}
	}
	result := quiz.GradeResult{Correct: true, Quality: 4}

	// ---- DB-only wiring (this PR): learning_notes must stay frozen. ----
	frozenDir := copyLearningNotes(t, cfg.Notebooks.LearningNotesDirectory)
	nbFrozen := cfg.Notebooks
	nbFrozen.LearningNotesDirectory = frozenDir

	repos := bootstrap.BuildStateRepositories(nbFrozen, cfg.Quiz, db)
	require.NotNil(t, repos.HistoryStore, "DB configured -> reads served from DB")
	require.IsType(t, &learning.DBLearningRepository{}, repos.Learning,
		"DB configured -> writes go to the DB only")

	svc := quiz.NewService(nbFrozen, nil, nil, repos.Learning, cfg.Quiz)
	svc.SetHistoryStore(repos.HistoryStore)

	before := snapshotDir(t, frozenDir)

	var logsBefore int
	require.NoError(t, db.GetContext(ctx, &logsBefore,
		`SELECT COUNT(*) FROM learning_logs WHERE note_id = $1`, target.ID))

	require.NoError(t, svc.SaveResult(ctx, cardFor(target), result, 1500))

	// (a) learning_notes is byte-for-byte unchanged.
	assert.Equal(t, before, snapshotDir(t, frozenDir),
		"DB-only wiring MUST NOT create or modify any learning_notes YAML file")

	// (b) the DB gained exactly the one expected row.
	var logsAfter int
	require.NoError(t, db.GetContext(ctx, &logsAfter,
		`SELECT COUNT(*) FROM learning_logs WHERE note_id = $1`, target.ID))
	assert.Equal(t, logsBefore+1, logsAfter, "the attempt must persist one learning_logs row in the DB")

	var row struct {
		Status   string `db:"status"`
		QuizType string `db:"quiz_type"`
	}
	require.NoError(t, db.GetContext(ctx, &row, `
		SELECT status, quiz_type FROM learning_logs
		WHERE note_id = $1 ORDER BY id DESC LIMIT 1`, target.ID))
	assert.Equal(t, "understood", row.Status)
	assert.Equal(t, string(notebook.QuizTypeNotebook), row.QuizType)

	// (c) reading back through DBHistoryStore reflects the attempt.
	histories, err := repos.HistoryStore.LoadAll(ctx)
	require.NoError(t, err)
	expr, ok := findExpressionInHistories(histories[target.NotebookID], target.Usage, target.Entry)
	require.True(t, ok, "the just-saved word must appear in the DB-reconstructed history")
	assert.Equal(t, notebook.LearnedStatusUnderstood, expr.GetLatestStatus(),
		"the DB read-back must reflect the correct attempt just written")

	// ---- Exclude/Resume (SkipWord/ResumeWord): freeze YAML, write DB skip flags. ----
	svc.SetSkipStores(repos.SkipFlags, repos.Note, repos.Origin)
	require.NotNil(t, repos.SkipFlags, "DB configured -> Exclude writes DB skip flags")

	beforeSkip := snapshotDir(t, frozenDir)
	skipInfo := quiz.CardInfo{NotebookName: target.NotebookID, Expression: target.Usage, NoteID: target.ID}
	require.NoError(t, svc.SkipWord(skipInfo, "", []notebook.QuizType{notebook.QuizTypeNotebook}))

	// (a) learning_notes still byte-for-byte unchanged.
	assert.Equal(t, beforeSkip, snapshotDir(t, frozenDir),
		"Exclude MUST NOT write learning_notes YAML in DB mode (the reported 'reputation' bug)")
	// (b) the DB skip-flag row was created.
	var skipCount int
	require.NoError(t, db.GetContext(ctx, &skipCount,
		`SELECT COUNT(*) FROM note_skip_flags WHERE note_id = $1 AND quiz_type = 'notebook'`, target.ID))
	assert.Equal(t, 1, skipCount, "Exclude must UPSERT a note_skip_flags row")
	// (c) the loaders' read side now sees the word excluded (L2 symmetry).
	histAfterSkip, err := repos.HistoryStore.LoadAll(ctx)
	require.NoError(t, err)
	exprSkip, ok := findExpressionInHistories(histAfterSkip[target.NotebookID], target.Usage, target.Entry)
	require.True(t, ok)
	assert.True(t, exprSkip.SkippedAt.IsSkippedAny(),
		"the DB read-back must show the word excluded — proving the skip lands where the loaders read it")

	// ResumeWord clears it, still without writing YAML.
	require.NoError(t, svc.ResumeWord(skipInfo, []notebook.QuizType{notebook.QuizTypeNotebook}))
	require.NoError(t, db.GetContext(ctx, &skipCount,
		`SELECT COUNT(*) FROM note_skip_flags WHERE note_id = $1 AND quiz_type = 'notebook'`, target.ID))
	assert.Equal(t, 0, skipCount, "Resume must DELETE the note_skip_flags row")
	assert.Equal(t, beforeSkip, snapshotDir(t, frozenDir),
		"Resume MUST NOT write learning_notes YAML in DB mode either")

	// CONTROL for Exclude: the pre-fix path wrote learning_notes YAML directly.
	// WriteYamlFile(learning_notes/<nb>.yml) is exactly what SkipWord did before
	// this change; assert that DOES modify the directory.
	skipCtlDir := copyLearningNotes(t, cfg.Notebooks.LearningNotesDirectory)
	skipCtlBefore := snapshotDir(t, skipCtlDir)
	require.NoError(t, notebook.WriteYamlFile(
		filepath.Join(skipCtlDir, target.NotebookID+".yml"),
		[]notebook.LearningHistory{{Metadata: notebook.LearningHistoryMetadata{NotebookID: target.NotebookID}}}))
	assert.NotEqual(t, skipCtlBefore, snapshotDir(t, skipCtlDir),
		"the pre-change SkipWord wrote learning_notes YAML — proving Exclude failed the freeze before this PR")

	// ---- RegisterDefinition (note write): freeze definitions YAML, write DB note. ----
	defsDir := ""
	if len(cfg.Notebooks.DefinitionsDirectories) > 0 {
		defsDir = cfg.Notebooks.DefinitionsDirectories[0]
	}
	require.NotEmpty(t, defsDir, "config.example.yml must declare a definitions directory")
	const newWord = "quixotic-dbonly-frozen"
	beforeDefs := snapshotDir(t, defsDir)
	require.NoError(t, repos.Note.Create(ctx, &notebook.NoteRecord{
		Usage: newWord, Entry: newWord, Meaning: "idealistic and impractical",
		DefinitionsDir: defsDir, NotebookFile: "roots-demo",
		NotebookNotes: []notebook.NotebookNote{{NotebookType: "book", NotebookID: "roots-demo"}},
	}))
	assert.Equal(t, beforeDefs, snapshotDir(t, defsDir),
		"RegisterDefinition (noteRepository.Create) MUST NOT write definitions YAML in DB mode")
	var defCount int
	require.NoError(t, db.GetContext(ctx, &defCount, `SELECT COUNT(*) FROM notes WHERE "usage" = $1`, newWord))
	assert.Equal(t, 1, defCount, "the new definition must persist as a DB note row")

	// CONTROL for RegisterDefinition: the pre-change dual-write note wiring
	// (MultiNoteRepository = YAML + DB) writes definitions YAML.
	defCtlDir := t.TempDir()
	defCtl := notebook.NewMultiNoteRepository(
		notebook.NewYAMLNoteRepositoryWithDefsDir(defCtlDir),
		notebook.NewDBNoteRepository(db),
	)
	defCtlBefore := snapshotDir(t, defCtlDir)
	require.NoError(t, defCtl.Create(ctx, &notebook.NoteRecord{
		Usage: "serendipitous-dbonly-ctl", Entry: "serendipitous-dbonly-ctl", Meaning: "x",
		DefinitionsDir: defCtlDir, NotebookFile: "roots-demo",
		NotebookNotes: []notebook.NotebookNote{{NotebookType: "book", NotebookID: "roots-demo"}},
	}))
	assert.NotEqual(t, defCtlBefore, snapshotDir(t, defCtlDir),
		"the pre-change dual-write note wiring MUST write definitions YAML — proving RegisterDefinition failed the freeze before this PR")

	// ---- CONTROL: the pre-change dual-write wiring rewrites YAML. ----
	controlDir := copyLearningNotes(t, cfg.Notebooks.LearningNotesDirectory)
	nbControl := cfg.Notebooks
	nbControl.LearningNotesDirectory = controlDir
	dualWrite := learning.NewMultiLearningRepository(
		learning.NewYAMLLearningRepository(controlDir, notebook.NewIntervalCalculator(cfg.Quiz.Algorithm, cfg.Quiz.FixedIntervals)),
		learning.NewDBLearningRepository(db),
	)
	svcControl := quiz.NewService(nbControl, nil, nil, dualWrite, cfg.Quiz)

	controlBefore := snapshotDir(t, controlDir)
	require.NoError(t, svcControl.SaveResult(ctx, cardFor(control), result, 1500))
	assert.NotEqual(t, controlBefore, snapshotDir(t, controlDir),
		"the pre-change dual-write wiring MUST rewrite learning_notes YAML — proving the freeze assertion fails before this PR and passes after")
}
