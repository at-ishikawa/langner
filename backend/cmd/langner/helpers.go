package main

import (
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
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
