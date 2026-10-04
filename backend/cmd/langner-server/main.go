package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/bootstrap"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/serverapp"
)

var configFile string

func main() {
	rootCmd := &cobra.Command{
		Use:           "langner-server",
		Short:         "Langner quiz service HTTP server",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context())
		},
	}
	rootCmd.Flags().StringVar(&configFile, "config", "", "config file path")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	app := bootstrap.New()

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("loadConfig() > %w", err)
	}

	handler, authSetup, cleanup, err := serverapp.BuildHandler(cfg)
	if err != nil {
		return err
	}
	app.AddShutdownHook(func(context.Context) error {
		cleanup()
		return nil
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: handler,
	}
	app.AddShutdownHook(srv.Shutdown)

	// Lightweight periodic sweep of expired device-authorization rows (design
	// §5). Cheap: the expires_at index supports it. Stops with ctx. This is the
	// one piece the serverless entrypoint omits (no long-running process).
	go sweepExpiredDeviceCodes(ctx, authSetup.DeviceCodes)

	return app.Run(ctx, func(ctx context.Context) error {
		slog.Info("starting server", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
}

// sweepExpiredDeviceCodes deletes expired device rows every 10 minutes until
// ctx is cancelled.
func sweepExpiredDeviceCodes(ctx context.Context, repo *auth.CLIDeviceCodeRepository) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := repo.DeleteExpired(ctx, time.Now()); err != nil {
				slog.Warn("device: sweep expired codes failed", "error", err)
			} else if n > 0 {
				slog.Debug("device: swept expired codes", "deleted", n)
			}
		}
	}
}

func loadConfig() (*config.Config, error) {
	loader, err := config.NewConfigLoader(configFile)
	if err != nil {
		return nil, fmt.Errorf("config.NewConfigLoader() > %w", err)
	}
	return loader.Load()
}
