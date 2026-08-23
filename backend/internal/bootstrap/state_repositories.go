package bootstrap

import (
	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// StateRepositories bundles the user-STATE persistence seams the server
// wires: the learning-log write repo, the note write repo, and the DB-backed
// learning-history read store. Extracted from main so the exact same wiring
// the running server uses is reachable from an integration test (see
// .claude/rules/verify-data-features-with-example-notebooks.md).
type StateRepositories struct {
	// Learning is the write path for quiz results (SaveResult / reverse /
	// freeform / grammar). DB-backed when a database is configured, else YAML.
	Learning learning.LearningRepository
	// Note is the write path for definition create/delete (RegisterDefinition
	// / DeleteDefinition). DB-backed when a database is configured, else YAML.
	Note notebook.NoteRepository
	// HistoryStore is the DB-backed READ side for learning history, non-nil
	// only when a database is configured. Callers install it on the quiz
	// service and the notebook handler so learning-history reads come from the
	// database; nil keeps the on-disk YAML fallback.
	HistoryStore learning.HistoryStore
}

// BuildStateRepositories wires the user-state repositories the way
// langner-server runs them.
//
// When db is non-nil (a database is configured) user-STATE writes go to the
// DATABASE ONLY: the runtime never rewrites the on-disk learning_notes YAML,
// which is frozen at import time and regenerated on demand by
// `langner export-db`. Reads are served from the database through
// HistoryStore. This completes the #26 cutover — PR #26 moved reads to the DB
// but the server kept dual-writing YAML via Multi* wrappers; wiring the DB
// repositories directly here drops the runtime YAML write.
//
// When db is nil (a DB-less dev setup) both reads and writes use the on-disk
// YAML learning_notes files, exactly as before — so a database-less checkout
// still works unchanged.
//
// The YAML repositories and the Multi* dual-write wrappers are intentionally
// NOT used here in DB mode; they remain for the no-DB path and for the
// migrate/import-db/export-db/sync-db CLI commands, which are untouched.
func BuildStateRepositories(notebooksCfg config.NotebooksConfig, quizCfg config.QuizConfig, db *sqlx.DB) StateRepositories {
	if db == nil {
		calculator := notebook.NewIntervalCalculator(quizCfg.Algorithm, quizCfg.FixedIntervals)
		var defsDir string
		if len(notebooksCfg.DefinitionsDirectories) > 0 && notebooksCfg.DefinitionsDirectories[0] != "" {
			defsDir = notebooksCfg.DefinitionsDirectories[0]
		}
		return StateRepositories{
			Learning: learning.NewYAMLLearningRepository(notebooksCfg.LearningNotesDirectory, calculator),
			Note:     notebook.NewYAMLNoteRepositoryWithDefsDir(defsDir),
		}
	}

	dbLearningRepo := learning.NewDBLearningRepository(db)
	dbNoteRepo := notebook.NewDBNoteRepository(db)
	// Reads resolve straight from the DB repositories (source of truth). The
	// canonical storage key a read looks up is the same one the write path
	// used (note_id / origin_id, quiz_type→slot), keeping reads symmetric with
	// writes (learning-history invariant L2).
	historyStore := learning.NewDBHistoryStore(
		dbNoteRepo,
		dbLearningRepo,
		notebook.NewDBEtymologyOriginRepository(db),
		notebook.NewDBSkipFlagRepository(db),
		notebook.NewDBGrammarCorrectionRepository(db),
	)
	return StateRepositories{
		Learning:     dbLearningRepo,
		Note:         dbNoteRepo,
		HistoryStore: historyStore,
	}
}
