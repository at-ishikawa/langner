package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

func loadConfig() (*config.Config, error) {
	loader, err := config.NewConfigLoader(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create config loader: %w", err)
	}
	return loader.Load()
}

// openDB opens a database connection using the provided config.
func openDB(cfg *config.Config) (*sqlx.DB, error) {
	return database.Open(cfg.Database)
}

// loadLearningHistoriesFromDB opens the DB, builds a DBHistoryStore and
// returns the per-notebook learning histories. After migration 016 the
// langner-server stopped writing user state to YAML, so the CLI's
// exports / reports must read from the DB to match what the user did in
// the web UI. Callers receive the *sqlx.DB so they can close it.
//
// A DB whose schema hasn't been migrated yet (fresh install, CI sandbox)
// surfaces as Error 1146 'Table … doesn't exist'. Treat that as "no
// history" rather than a fatal error so `langner notebooks stories X`
// still renders the notebook with no learned annotations — same as the
// pre-cutover behaviour against an empty learning_notes directory.
func loadLearningHistoriesFromDB(ctx context.Context, cfg *config.Config) (map[string][]notebook.LearningHistory, *sqlx.DB, error) {
	db, err := openDB(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}
	store := learning.NewDBHistoryStore(
		notebook.NewDBNoteRepository(db),
		learning.NewDBLearningRepository(db),
		notebook.NewDBEtymologyOriginRepository(db),
		notebook.NewDBSkipFlagRepository(db),
	)
	histories, err := store.LoadAll(ctx)
	if err != nil {
		if isTableMissing(err) {
			return map[string][]notebook.LearningHistory{}, db, nil
		}
		_ = db.Close()
		return nil, nil, fmt.Errorf("load learning histories from DB: %w", err)
	}
	return histories, db, nil
}

// isTableMissing returns true when err carries MySQL/TiDB error 1146
// (ER_NO_SUCH_TABLE). Used to distinguish "schema isn't migrated" from
// real query failures.
func isTableMissing(err error) bool {
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1146 {
		return true
	}
	return false
}
