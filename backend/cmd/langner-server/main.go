package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/gen-protos/api/v1/apiv1connect"
	"github.com/at-ishikawa/langner/internal/bootstrap"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/inference/openai"
	"github.com/at-ishikawa/langner/internal/server"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
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

	if cfg.OpenAI.APIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY environment variable is required")
	}

	openaiClient := openai.NewClient(cfg.OpenAI.APIKey, cfg.OpenAI.Model, inference.DefaultMaxRetryAttempts)
	defer func() {
		_ = openaiClient.Close()
	}()

	dictionaryMap, err := loadDictionaryMap(cfg.Dictionaries.RapidAPI.CacheDirectory)
	if err != nil {
		log.Printf("Warning: failed to load dictionary cache: %v", err)
		dictionaryMap = make(map[string]rapidapi.Response)
	}

	handler := server.NewQuizHandler(cfg, openaiClient, dictionaryMap)
	path, h := apiv1connect.NewQuizServiceHandler(handler)

	mux := http.NewServeMux()
	mux.Handle(path, h)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: corsMiddleware(h2c.NewHandler(mux, &http2.Server{}), cfg.Server.CORS.AllowedOrigins),
	}
	app.AddShutdownHook(srv.Shutdown)

	return app.Run(ctx, func(ctx context.Context) error {
		log.Printf("Starting server on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
}

func loadConfig() (*config.Config, error) {
	loader, err := config.NewConfigLoader(configFile)
	if err != nil {
		return nil, fmt.Errorf("config.NewConfigLoader() > %w", err)
	}
	return loader.Load()
}

func loadDictionaryMap(cacheDir string) (map[string]rapidapi.Response, error) {
	responses, err := rapidapi.NewReader().Read(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("rapidapi.NewReader().Read() > %w", err)
	}
	return rapidapi.FromResponsesToMap(responses), nil
}

func corsMiddleware(next http.Handler, allowedOrigins []string) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Connect-Protocol-Version")
		w.Header().Set("Access-Control-Max-Age", "3600")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
