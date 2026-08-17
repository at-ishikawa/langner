package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

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

	// Set up repositories with dual storage when DB is configured
	calculator := notebook.NewIntervalCalculator(cfg.Quiz.Algorithm, cfg.Quiz.FixedIntervals)
	yamlLearningRepo := learning.NewYAMLLearningRepository(cfg.Notebooks.LearningNotesDirectory, calculator)
	var learningRepo learning.LearningRepository = yamlLearningRepo
	var noteRepo notebook.NoteRepository
	var defsDir string
	if len(cfg.Notebooks.DefinitionsDirectories) > 0 && cfg.Notebooks.DefinitionsDirectories[0] != "" {
		defsDir = cfg.Notebooks.DefinitionsDirectories[0]
	}
	yamlNoteRepo := notebook.NewYAMLNoteRepositoryWithDefsDir(defsDir)
	noteRepo = yamlNoteRepo

	// Analytics reads through the same learning-history seam the quiz service
	// uses: the DB-backed store when a database is connected (see the
	// historyStore built in the DB branch below), else the on-disk YAML
	// learning_notes files. The store is applied to yamlAnalyticsRepo inside
	// that branch via WithHistoryStore; here we start from the YAML reader.
	yamlAnalyticsRepo := analytics.NewYAMLRepository(cfg.Notebooks.LearningNotesDirectory)
	// Journals are read alongside stories (they share the story format, see
	// quiz.Service.newReader) so a grammar attempt's notebook — a journal —
	// resolves here too; LoadGrammars then attaches the corrections
	// themselves, which resolveGrammar needs to turn a correction id back
	// into its mistake/fix text.
	analyticsStoryDirectories := append(append([]string{}, cfg.Notebooks.StoriesDirectories...), cfg.Notebooks.JournalsDirectories...)
	if reader, err := notebook.NewReader(
		analyticsStoryDirectories,
		cfg.Notebooks.FlashcardsDirectories,
		cfg.Notebooks.BooksDirectories,
		cfg.Notebooks.DefinitionsDirectories,
		cfg.Notebooks.EtymologyDirectories,
		dictionaryMap,
	); err != nil {
		slog.Warn("analytics meaning lookup disabled — notebook reader init failed", "error", err)
	} else if err := reader.LoadGrammars(cfg.Notebooks.GrammarsDirectories); err != nil {
		slog.Warn("analytics grammar meaning lookup disabled — LoadGrammars failed", "error", err)
	} else {
		yamlAnalyticsRepo = yamlAnalyticsRepo.WithMetadataResolver(analytics.NewNotebookMetadataResolver(reader))
	}

	// historyStore is the DB-backed READ side for learning history, wired only
	// when a database is connected. It reconstructs the same per-notebook
	// LearningHistory shape the YAML reader produced (notes + learning_logs +
	// skip flags + etymology origins), keyed by the SAME canonical storage key
	// the write path used (note_id / origin_id, quiz_type→slot), so reads stay
	// symmetric with writes (learning-history invariant L2). Nil in YAML-only
	// mode, in which case the quiz service and analytics fall back to reading
	// the on-disk learning_notes files.
	var historyStore learning.HistoryStore
	if cfg.Database.Host != "" && cfg.Database.Password != "" {
		db, err := database.Open(cfg.Database)
		if err != nil {
			slog.Warn("failed to open database, running with YAML-only storage", "error", err)
		} else {
			app.AddShutdownHook(func(ctx context.Context) error {
				return db.Close()
			})
			dbLearningRepo := learning.NewDBLearningRepository(db)
			dbNoteRepo := notebook.NewDBNoteRepository(db)
			learningRepo = learning.NewMultiLearningRepository(yamlLearningRepo, dbLearningRepo)
			noteRepo = notebook.NewMultiNoteRepository(yamlNoteRepo, dbNoteRepo)
			// Reads resolve straight from the DB repositories (source of
			// truth), not the Multi wrappers used for dual-write.
			historyStore = learning.NewDBHistoryStore(
				dbNoteRepo,
				dbLearningRepo,
				notebook.NewDBEtymologyOriginRepository(db),
				notebook.NewDBSkipFlagRepository(db),
			).WithGrammarYAMLDir(cfg.Notebooks.LearningNotesDirectory)
			yamlAnalyticsRepo = yamlAnalyticsRepo.WithHistoryStore(historyStore)
			slog.Info("database connected, dual storage enabled; learning-history reads served from DB")
		}
	}
	analyticsRepo := analytics.Repository(yamlAnalyticsRepo)

	svc := quiz.NewService(cfg.Notebooks, inferenceClient, dictionaryMap, learningRepo, cfg.Quiz)
	// Swap the quiz service's learning-history reads to the DB store when one
	// was built above; a nil store keeps the YAML fallback.
	svc.SetHistoryStore(historyStore)

	dictConfig := dictionary.Config{
		RapidAPIHost: cfg.Dictionaries.RapidAPI.Host,
		RapidAPIKey:  cfg.Dictionaries.RapidAPI.Key,
	}
	dictReader := dictionary.NewReader(cfg.Dictionaries.RapidAPI.CacheDirectory, dictConfig)
	notebookHandler := server.NewNotebookHandler(cfg.Notebooks, cfg.Templates, dictionaryMap, dictReader, inferenceClient, noteRepo)

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
