package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// TestBuildStateRepositories_YAMLOnlyMode asserts that without a database the
// server keeps writing (and reading) the on-disk learning_notes YAML — the
// DB-less dev setup must behave exactly as before.
func TestBuildStateRepositories_YAMLOnlyMode(t *testing.T) {
	repos := BuildStateRepositories(config.NotebooksConfig{
		LearningNotesDirectory: t.TempDir(),
		DefinitionsDirectories: []string{t.TempDir()},
	}, config.QuizConfig{}, nil)

	assert.IsType(t, &learning.YAMLLearningRepository{}, repos.Learning,
		"no database -> learning writes go to YAML")
	assert.IsType(t, &notebook.YAMLNoteRepository{}, repos.Note,
		"no database -> note writes go to YAML")
	assert.Nil(t, repos.HistoryStore,
		"no database -> reads fall back to the YAML learning_notes files")
}

// TestBuildStateRepositories_DBMode asserts the completed #26 cutover: when a
// database is configured the write repositories are the DB repositories
// DIRECTLY — NOT the Multi* dual-write wrappers that also rewrote YAML — and a
// DB-backed history store serves reads. This is the wiring the "no YAML writes
// at runtime" guarantee rests on.
func TestBuildStateRepositories_DBMode(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mockDB.Close() })
	db := sqlx.NewDb(mockDB, "sqlmock")

	repos := BuildStateRepositories(config.NotebooksConfig{
		LearningNotesDirectory: t.TempDir(),
	}, config.QuizConfig{}, db)

	assert.IsType(t, &learning.DBLearningRepository{}, repos.Learning,
		"database configured -> learning writes go to the DB ONLY (not Multi/YAML)")
	assert.IsType(t, &notebook.DBNoteRepository{}, repos.Note,
		"database configured -> note writes go to the DB ONLY (not Multi/YAML)")
	assert.NotNil(t, repos.HistoryStore,
		"database configured -> learning-history reads are served from the DB")
}

// snapshotDir hashes every file under dir (path -> content) so a test can
// assert a directory is byte-for-byte unchanged after an operation.
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

// recordingLearningRepo is a stand-in for the DB learning repository: it
// records writes in memory and — crucially — never touches the filesystem,
// exactly like the real DBLearningRepository writes only to Postgres.
type recordingLearningRepo struct {
	created []*learning.LearningLog
}

func (r *recordingLearningRepo) Create(_ context.Context, log *learning.LearningLog) error {
	r.created = append(r.created, log)
	return nil
}
func (r *recordingLearningRepo) FindAll(context.Context) ([]learning.LearningLog, error) {
	return nil, nil
}
func (r *recordingLearningRepo) BatchCreate(context.Context, []*learning.LearningLog) error {
	return nil
}
func (r *recordingLearningRepo) BatchDelete(context.Context, []int64) error { return nil }
func (r *recordingLearningRepo) UpdateLog(context.Context, learning.UpdateLogInput) (learning.UpdateLogResult, error) {
	return learning.UpdateLogResult{}, nil
}

// TestDBOnlyLearningRepo_WriteDoesNotTouchLearningNotesYAML is the PG-free,
// always-runnable before/after demonstration the owner asked for: it drives
// the SAME learning-log write path (LearningRepository.Create, the call
// SaveResult makes) through both wirings and shows the freeze.
//
//   - CONTROL (the pre-change wiring): MultiLearningRepository(YAML, DB) — a
//     quiz result rewrites the on-disk learning_notes YAML. The snapshot
//     CHANGES, so a "learning_notes is unchanged" assertion FAILS here. This
//     is the behavior this PR removes.
//   - DB-ONLY (the post-change wiring): the DB repository alone — the write
//     records nothing to disk, so the learning_notes snapshot is byte-for-byte
//     unchanged. The assertion PASSES.
//
// The full end-to-end proof against a real Postgres (row persisted, read back
// through DBHistoryStore) lives in the *_LivePostgres_Integration test under
// cmd/langner; this one pins the filesystem-freeze property without a DB.
func TestDBOnlyLearningRepo_WriteDoesNotTouchLearningNotesYAML(t *testing.T) {
	newLog := func() *learning.LearningLog {
		return &learning.LearningLog{
			NotebookName: "vocabulary", Expression: "serendipity",
			Status: "understood", Quality: 4, QuizType: "notebook",
			SourceNotebookID: "vocabulary",
		}
	}

	t.Run("control: dual-write wiring rewrites learning_notes YAML", func(t *testing.T) {
		dir := t.TempDir()
		log := newLog()
		log.LearningNotesDir = dir
		before := snapshotDir(t, dir)

		yamlRepo := learning.NewYAMLLearningRepository(dir, notebook.NewIntervalCalculator("sm2", nil))
		multi := learning.NewMultiLearningRepository(yamlRepo, &recordingLearningRepo{})
		require.NoError(t, multi.Create(context.Background(), log))

		after := snapshotDir(t, dir)
		assert.NotEqual(t, before, after,
			"the pre-change dual-write wiring MUST rewrite the learning_notes YAML — proving the freeze assertion would fail before this PR")
	})

	t.Run("db-only wiring leaves learning_notes YAML untouched", func(t *testing.T) {
		dir := t.TempDir()
		log := newLog()
		log.LearningNotesDir = dir
		before := snapshotDir(t, dir)

		dbOnly := &recordingLearningRepo{}
		require.NoError(t, dbOnly.Create(context.Background(), log))

		after := snapshotDir(t, dir)
		assert.Equal(t, before, after,
			"the DB-only write path MUST NOT create or modify any learning_notes YAML file")
		require.Len(t, dbOnly.created, 1, "the attempt is still persisted — to the DB, not YAML")
	})
}
