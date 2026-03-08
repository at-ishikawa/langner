package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"connectrpc.com/connect"

	"github.com/at-ishikawa/langner/gen-protos/api/v1/apiv1connect"
	"github.com/at-ishikawa/langner/internal/bootstrap"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/inference/openai"
	"github.com/at-ishikawa/langner/internal/quiz"
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
		slog.Warn("failed to load dictionary cache", "error", err)
		dictionaryMap = make(map[string]rapidapi.Response)
	}

	errorLogger := connect.WithInterceptors(connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			if err != nil {
				slog.Error("rpc error", "procedure", req.Spec().Procedure, "error", err)
			}
			return resp, err
		}
	}))

	svc := quiz.NewService(cfg.Notebooks, openaiClient, dictionaryMap)
	handler := server.NewQuizHandler(svc)
	path, h := apiv1connect.NewQuizServiceHandler(handler, errorLogger)

	notebookHandler := server.NewNotebookHandler(cfg.Notebooks, cfg.Templates, dictionaryMap)
	notebookPath, notebookH := apiv1connect.NewNotebookServiceHandler(notebookHandler, errorLogger)

	mux := http.NewServeMux()
	mux.Handle(path, h)
	mux.Handle(notebookPath, notebookH)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: corsMiddleware(h2c.NewHandler(mux, &http2.Server{}), cfg.Server.CORS.AllowedOrigins),
	}
	app.AddShutdownHook(srv.Shutdown)

	return app.Run(ctx, func(ctx context.Context) error {
		slog.Info("starting server", "addr", srv.Addr)
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
	allowAll := false
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o == "*" {
			allowAll = true
			break
		}
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowAll || allowed[origin]) {
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
