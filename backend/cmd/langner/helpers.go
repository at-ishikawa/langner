package main

import (
	"context"
	"fmt"

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
		_ = db.Close()
		return nil, nil, fmt.Errorf("load learning histories from DB: %w", err)
	}
	return histories, db, nil
}
