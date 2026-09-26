package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/gen-protos/api/v1/apiv1connect"
	"github.com/at-ishikawa/langner/internal/analytics"
	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/bootstrap"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/datasync"
	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/inference/gemini"
	"github.com/at-ishikawa/langner/internal/inference/mock"
	"github.com/at-ishikawa/langner/internal/inference/openai"
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
	case "gemini":
		if cfg.Gemini.APIKey != "" {
			geminiClient := gemini.NewClient(cfg.Gemini.APIKey, cfg.Gemini.Model, inference.DefaultMaxRetryAttempts)
			defer func() {
				_ = geminiClient.Close()
			}()
			inferenceClient = geminiClient
			slog.Info("using Gemini inference provider", "model", cfg.Gemini.Model)
		} else {
			slog.Warn("GEMINI_API_KEY is not set; quiz grading features will be unavailable")
		}
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

	loggingInterceptor := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			if err != nil {
				slog.Error("rpc error", "procedure", req.Spec().Procedure, "error", err)
			}
			return resp, err
		}
	})

	// Open the database when one is configured. A nil db means YAML-only dev
	// mode; a non-nil db switches the server to DB-only user-state storage.
	var db *sqlx.DB
	if cfg.Database.Host != "" && cfg.Database.Password != "" {
		opened, err := database.Open(cfg.Database)
		if err != nil {
			slog.Warn("failed to open database, running with YAML-only storage", "error", err)
		} else {
			db = opened
			app.AddShutdownHook(func(ctx context.Context) error {
				return db.Close()
			})
		}
	}

	// Google-OAuth sign-in. Enabled as soon as a session signing key is
	// configured; then EVERY connect RPC is gated behind a valid session
	// cookie and the /auth/* endpoints are served plainly. Without a signing
	// key the server runs ungated (YAML-only dev), exactly as before.
	var authSetup *authComponents
	if cfg.Auth.Enabled() {
		authSetup, err = buildAuth(cfg.Auth, db)
		if err != nil {
			return fmt.Errorf("set up auth: %w", err)
		}
		slog.Info("google-oauth sign-in enabled; all RPCs require a valid session cookie")
	} else {
		slog.Warn("auth disabled (no SESSION_SIGNING_KEY); RPCs are ungated")
	}

	// CLI device-flow / bearer auth. Additive and independent of the cookie: it
	// mounts as soon as TOKEN_SIGNING_KEY is set (mirroring how the cookie mounts
	// on SESSION_SIGNING_KEY). Bearer is accepted ALONGSIDE the cookie — it never
	// weakens or replaces the cookie path. A DB is required (device/refresh rows
	// live in Postgres).
	var deviceSetup *deviceComponents
	if cfg.Auth.TokenEnabled() {
		deviceSetup, err = buildDeviceAuth(cfg.Auth, db)
		if err != nil {
			return fmt.Errorf("set up device auth: %w", err)
		}
		slog.Info("cli device-flow + bearer auth enabled; Authorization: Bearer accepted alongside the session cookie")
	}

	// User-STATE repositories. When a database is configured, writes go to the
	// DB ONLY — the runtime never rewrites the on-disk learning_notes YAML
	// (frozen at import time, regenerated by `langner export-db`) — and reads
	// are served from the DB via historyStore. Without a database both reads
	// and writes use the on-disk YAML, exactly as before.
	repos := bootstrap.BuildStateRepositories(cfg.Notebooks, cfg.Quiz, db)
	learningRepo := repos.Learning
	noteRepo := repos.Note
	historyStore := repos.HistoryStore

	// Analytics reads through the same learning-history seam the quiz service
	// uses: the DB-backed store when a database is connected, else the on-disk
	// YAML learning_notes files. The store is applied to yamlAnalyticsRepo
	// below via WithHistoryStore; here we start from the YAML reader.
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

	// historyStore is the DB-backed READ side for learning history, non-nil
	// only when a database is connected. It reconstructs the same per-notebook
	// LearningHistory shape the YAML reader produced (notes + learning_logs +
	// skip flags + etymology origins), keyed by the SAME canonical storage key
	// the write path used, so reads stay symmetric with writes (learning-
	// history invariant L2). In YAML-only mode it is nil and the quiz service,
	// notebook handler, and analytics fall back to the on-disk files.
	if historyStore != nil {
		yamlAnalyticsRepo = yamlAnalyticsRepo.WithHistoryStore(historyStore)
		slog.Info("database connected; user-state writes go to the database only (learning_notes YAML frozen at runtime), learning-history reads served from DB")
	}
	analyticsRepo := analytics.Repository(yamlAnalyticsRepo)

	svc := quiz.NewService(cfg.Notebooks, inferenceClient, dictionaryMap, learningRepo, cfg.Quiz)
	// Swap the quiz service's learning-history reads to the DB store when one
	// was built above; a nil store keeps the YAML fallback.
	svc.SetHistoryStore(historyStore)
	// Route the deliberate Exclude action (SkipWord/ResumeWord) to the DB
	// skip-flag tables in DB mode instead of the on-disk learning_notes YAML;
	// nil skip-flag repo keeps the YAML skip path.
	svc.SetSkipStores(repos.SkipFlags, repos.Note, repos.Origin)
	// DB mode: install the ensure-on-serve hook so a notebook whose YAML gained
	// units without a fresh `migrate import-db` still gets its notes rows
	// created additively when its cards are served — self-healing exclude/
	// SkipWord and every other note-keyed DB write. Reuses the same YAML note
	// source + DB note repo the CLI importer uses, but never runs the
	// destructive reconcile pass. YAML-only mode leaves the hook nil.
	if db != nil {
		if ensureReader, rerr := notebook.NewReader(
			cfg.Notebooks.StoriesDirectories,
			cfg.Notebooks.FlashcardsDirectories,
			cfg.Notebooks.BooksDirectories,
			cfg.Notebooks.DefinitionsDirectories,
			cfg.Notebooks.EtymologyDirectories,
			dictionaryMap,
		); rerr != nil {
			slog.Warn("ensure-on-serve disabled — notebook reader init failed", "error", rerr)
		} else {
			noteSource := notebook.NewYAMLNoteRepository(ensureReader)
			ensurer := datasync.NewImporter(noteRepo, nil, noteSource, nil, nil, nil, io.Discard)
			svc.SetNoteEnsurer(ensurer)
		}
	}
	// Enforce notebook public/private visibility on every quiz read path (auth
	// Phase 3). Nil (no DB) keeps every notebook visible.
	svc.SetNotebookACL(repos.ACL)

	dictConfig := dictionary.Config{
		RapidAPIHost: cfg.Dictionaries.RapidAPI.Host,
		RapidAPIKey:  cfg.Dictionaries.RapidAPI.Key,
	}
	dictReader := dictionary.NewReader(cfg.Dictionaries.RapidAPI.CacheDirectory, dictConfig)
	notebookHandler := server.NewNotebookHandler(cfg.Notebooks, cfg.Templates, dictionaryMap, dictReader, inferenceClient, noteRepo)
	// Serve the Learn page's learning-history reads (status / next-review /
	// exclusion badges) from the DB too, so they stay fresh once the on-disk
	// learning_notes YAML is frozen in DB mode. Nil keeps the YAML fallback.
	notebookHandler.SetHistoryStore(historyStore)
	// Hard-reject a private notebook the requesting user can't see on the Learn
	// page's detail / etymology / PDF paths (auth Phase 3). Nil keeps every
	// notebook visible (no DB).
	notebookHandler.SetNotebookACL(repos.ACL)

	handler := server.NewQuizHandler(svc)
	handler.SetNoteRepository(noteRepo)
	analyticsHandler := server.NewAnalyticsHandler(analyticsRepo)

	// The auth interceptor rejects any RPC lacking a verified session
	// (populated in request ctx by authCookieMiddleware). It is only added when
	// auth is enabled so ungated YAML-only dev keeps working.
	interceptors := []connect.Interceptor{loggingInterceptor}
	if authSetup != nil || deviceSetup != nil {
		interceptors = append(interceptors, server.NewAuthInterceptor())
	}
	handlerOpts := connect.WithInterceptors(interceptors...)
	path, h := apiv1connect.NewQuizServiceHandler(handler, handlerOpts)
	notebookPath, notebookH := apiv1connect.NewNotebookServiceHandler(notebookHandler, handlerOpts)
	analyticsPath, analyticsH := apiv1connect.NewAnalyticsServiceHandler(analyticsHandler, handlerOpts)

	mux := http.NewServeMux()
	mux.Handle(path, h)
	mux.Handle(notebookPath, notebookH)
	mux.Handle(analyticsPath, analyticsH)

	// Auth endpoints are plain HTTP, registered OUTSIDE the connect
	// interceptor so an unauthenticated user can sign in.
	if authSetup != nil {
		mux.HandleFunc("/auth/google/login", authSetup.handler.Login)
		mux.HandleFunc("/auth/google/callback", authSetup.handler.Callback)
		mux.HandleFunc("/auth/logout", authSetup.handler.Logout)
		mux.HandleFunc("/auth/me", authSetup.handler.Me)
	}
	// Device-flow + refresh/revoke endpoints, plain HTTP outside the interceptor
	// so an unauthenticated CLI can reach them.
	if deviceSetup != nil {
		mux.HandleFunc("/auth/device/code", deviceSetup.handler.RequestCode)
		mux.HandleFunc("/auth/device/token", deviceSetup.handler.Token)
		mux.HandleFunc("/auth/device/approve", deviceSetup.handler.Approve)
		mux.HandleFunc("/auth/device/deny", deviceSetup.handler.Deny)
		mux.HandleFunc("/auth/device", deviceSetup.handler.ShowApprovalPage)
		mux.HandleFunc("/auth/token/refresh", deviceSetup.handler.Refresh)
		mux.HandleFunc("/auth/token/revoke", deviceSetup.handler.Revoke)
	}

	rootHandler := h2c.NewHandler(mux, &http2.Server{})
	// Bearer acceptance runs INSIDE (after) the cookie middleware so a valid
	// cookie wins when both are present; bearer only fills the context when the
	// cookie left it empty. Both never reject — the interceptor gates RPCs.
	if deviceSetup != nil {
		rootHandler = server.AuthBearerMiddleware(rootHandler, deviceSetup.tokens)
	}
	if authSetup != nil {
		// Lifts a verified session cookie into the request context for both the
		// connect interceptor and /auth/me. Does NOT reject — /auth/* and CORS
		// preflight must pass through unauthenticated.
		rootHandler = server.AuthCookieMiddleware(rootHandler, authSetup.sessions)
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: corsMiddleware(rootHandler, cfg.Server.CORS.AllowedOrigins),
	}
	app.AddShutdownHook(srv.Shutdown)

	// Lightweight periodic sweep of expired device-authorization rows (design
	// §5). Cheap: the expires_at index supports it. Stops with ctx.
	if deviceSetup != nil {
		go sweepExpiredDeviceCodes(ctx, deviceSetup.deviceCodes)
	}

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

func loadDictionaryMap(cacheDir string) (map[string]rapidapi.Response, error) {
	responses, err := rapidapi.NewReader().Read(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("rapidapi.NewReader().Read() > %w", err)
	}
	return rapidapi.FromResponsesToMap(responses), nil
}

// authComponents holds the pieces main wires when auth is enabled.
type authComponents struct {
	handler  *server.AuthHandler
	sessions *auth.SessionSigner
}

// buildAuth constructs the OAuth handler and session signer. It fails fast when
// a required secret is missing, so a misconfigured auth deployment never starts
// silently ungated.
func buildAuth(cfg config.AuthConfig, db *sqlx.DB) (*authComponents, error) {
	if db == nil {
		return nil, errors.New("auth is enabled but no database is configured (users are stored in Postgres)")
	}
	sessions, err := auth.NewSessionSigner(auth.DecodeKey(cfg.SessionSigningKey))
	if err != nil {
		return nil, fmt.Errorf("session signing key: %w", err)
	}
	state, err := auth.NewStateSigner(auth.DecodeKey(cfg.SessionSigningKey))
	if err != nil {
		return nil, fmt.Errorf("state signing key: %w", err)
	}
	users := auth.NewUserRepository(db)
	authenticator := auth.NewOAuthClient(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.RedirectURL, nil)
	handler := server.NewAuthHandler(server.AuthHandlerConfig{
		Authenticator:  authenticator,
		Sessions:       sessions,
		State:          state,
		Users:          users,
		AllowedEmails:  cfg.AllowedEmails,
		FrontendURL:    cfg.FrontendURL,
		CookieSecure:   cfg.CookieSecure,
		CookieSameSite: parseSameSite(cfg.CookieSameSite),
	})
	return &authComponents{handler: handler, sessions: sessions}, nil
}

// deviceComponents holds the pieces main wires when TOKEN_SIGNING_KEY is set.
type deviceComponents struct {
	handler     *server.DeviceAuthHandler
	tokens      *auth.TokenSigner
	deviceCodes *auth.CLIDeviceCodeRepository
}

// buildDeviceAuth constructs the device-flow handler, the access-token signer,
// and the device/refresh repositories. It fails fast when the DB or key is
// missing, so a misconfigured bearer deployment never starts silently.
func buildDeviceAuth(cfg config.AuthConfig, db *sqlx.DB) (*deviceComponents, error) {
	if db == nil {
		return nil, errors.New("bearer auth is enabled but no database is configured (device/refresh rows live in Postgres)")
	}
	tokens, err := auth.NewTokenSigner(auth.DecodeKey(cfg.TokenSigningKey))
	if err != nil {
		return nil, fmt.Errorf("token signing key: %w", err)
	}
	deviceCodes := auth.NewCLIDeviceCodeRepository(db)
	refreshTokens := auth.NewCLIRefreshTokenRepository(db)
	handler := server.NewDeviceAuthHandler(server.DeviceAuthHandlerConfig{
		DeviceCodes:     deviceCodes,
		RefreshTokens:   refreshTokens,
		Tokens:          tokens,
		FrontendURL:     cfg.FrontendURL,
		VerificationURI: deviceVerificationURI(cfg.RedirectURL),
	})
	return &deviceComponents{handler: handler, tokens: tokens, deviceCodes: deviceCodes}, nil
}

// deviceVerificationURI derives the browser approval URL (…/auth/device) from
// the OAuth redirect_url's origin. The redirect_url is the API host the browser
// already returns to, so its scheme+host is the right base. Falls back to a
// bare path if the redirect_url can't be parsed.
func deviceVerificationURI(redirectURL string) string {
	u, err := url.Parse(redirectURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "/auth/device"
	}
	return u.Scheme + "://" + u.Host + "/auth/device"
}

func parseSameSite(s string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

// corsMiddleware serves explicit, credentialed CORS. It reflects the request
// Origin (never the literal "*", which is invalid with credentials) when the
// Origin is configured or when "*" is configured as a wildcard, and always
// sends Access-Control-Allow-Credentials so the session cookie is accepted
// cross-origin (frontend 3100 ↔ backend 8080).
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
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Connect-Protocol-Version, Authorization")
		w.Header().Set("Access-Control-Max-Age", "3600")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
