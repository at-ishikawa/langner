package main

import (
	"fmt"

	"github.com/at-ishikawa/langner/internal/config"
)

func loadConfig() (*config.Config, error) {
	loader, err := config.NewConfigLoader(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create config loader: %w", err)
	}
	return loader.Load()
}
