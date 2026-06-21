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
	"github.com/at-ishikawa/langner/internal/analytics"
	"github.com/at-ishikawa/langner/internal/bootstrap"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/inference/mock"
	"github.com/at-ishikawa/langner/internal/inference/openai"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
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

	var inferenceClient inference.Client
	switch cfg.Inference.Mode {
	case "mock":
		inferenceClient = mock.NewClient()
		slog.Info("using mock inference client (substring grader)")
	default:
		if cfg.OpenAI.APIKey != "" {
			openaiClient := openai.NewClient(cfg.OpenAI.APIKey, cfg.OpenAI.Model, inference.DefaultMaxRetryAttempts)
			defer func() {
				_ = openaiClient.Close()
			}()
			inferenceClient = openaiClient
		} else {
			slog.Warn("OPENAI_API_KEY is not set; quiz grading features will be unavailable")
		}
	}

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

	// The server is DB-only: state (notes, learning logs, skip flags,
	// definition / flashcard structure, dictionary cache) lives in MySQL.
	// YAML files under examples/* stay as the shared content library
	// (stories, ebook chapters, etymology reference notebooks) and are
	// still read by notebook.Reader at request time.
	if cfg.Database.Host == "" || cfg.Database.Password == "" {
		return fmt.Errorf("database is required: set database.host and DB_PASSWORD")
	}
	db, err := database.Open(cfg.Database)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	app.AddShutdownHook(func(ctx context.Context) error {
		return db.Close()
	})

	learningRepo := learning.NewDBLearningRepository(db)
	noteRepo := notebook.NewDBNoteRepository(db)
	originRepo := notebook.NewDBEtymologyOriginRepository(db)
	skipFlagRepo := notebook.NewDBSkipFlagRepository(db)
	historyStore := learning.NewDBHistoryStore(noteRepo, learningRepo, originRepo, skipFlagRepo)

	dbAnalyticsRepo := analytics.NewDBRepository(db)
	if reader, err := notebook.NewReader(
		cfg.Notebooks.StoriesDirectories,
		cfg.Notebooks.FlashcardsDirectories,
		cfg.Notebooks.BooksDirectories,
		cfg.Notebooks.DefinitionsDirectories,
		cfg.Notebooks.EtymologyDirectories,
		dictionaryMap,
	); err != nil {
		slog.Warn("analytics meaning lookup disabled — notebook reader init failed", "error", err)
	} else {
		dbAnalyticsRepo = dbAnalyticsRepo.WithMetadataResolver(analytics.NewNotebookMetadataResolver(reader))
	}
	analyticsRepo := analytics.Repository(dbAnalyticsRepo)

	svc := quiz.NewService(cfg.Notebooks, inferenceClient, dictionaryMap, learningRepo, cfg.Quiz)
	svc.WithDBState(historyStore, originRepo, skipFlagRepo, noteRepo)

	dictConfig := dictionary.Config{
		RapidAPIHost: cfg.Dictionaries.RapidAPI.Host,
		RapidAPIKey:  cfg.Dictionaries.RapidAPI.Key,
	}
	dictReader := dictionary.NewReader(cfg.Dictionaries.RapidAPI.CacheDirectory, dictConfig)
	definitionsRepo := notebook.NewDBDefinitionsRepository(db)
	notebookHandler := server.NewNotebookHandler(cfg.Notebooks, cfg.Templates, dictionaryMap, dictReader, inferenceClient, noteRepo)
	notebookHandler.WithHistoryStore(historyStore)
	notebookHandler.WithDefinitionsRepo(definitionsRepo)

	handler := server.NewQuizHandler(svc)
	handler.SetNoteRepository(noteRepo)
	analyticsHandler := server.NewAnalyticsHandler(analyticsRepo)
	path, h := apiv1connect.NewQuizServiceHandler(handler, errorLogger)
	notebookPath, notebookH := apiv1connect.NewNotebookServiceHandler(notebookHandler, errorLogger)
	analyticsPath, analyticsH := apiv1connect.NewAnalyticsServiceHandler(analyticsHandler, errorLogger)

	mux := http.NewServeMux()
	mux.Handle(path, h)
	mux.Handle(notebookPath, notebookH)
	mux.Handle(analyticsPath, analyticsH)

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
