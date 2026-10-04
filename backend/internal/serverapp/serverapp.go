// Package serverapp builds and runs langner's complete HTTP server — the connect
// RPC services, the plain-HTTP /auth/* routes, bearer-auth middleware, and CORS.
// It is the single wiring point shared by the CLI entrypoint (cmd/langner-server)
// and the Vercel Go-framework entrypoint (cmd/server), so both run an identical
// stack. The server listens on cfg.Server.Port, which config binds to $PORT — the
// port Vercel's Go framework routes traffic to.
package serverapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"

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

// Serve loads config (from configFile, or env-only when configFile is ""), builds
// the HTTP handler, and runs the server until ctx is cancelled. It is the entry
// both cmd/langner-server and the Vercel cmd/server call.
func Serve(ctx context.Context, configFile string) error {
	app := bootstrap.New()

	loader, err := config.NewConfigLoader(configFile)
	if err != nil {
		return fmt.Errorf("config.NewConfigLoader() > %w", err)
	}
	cfg, err := loader.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	handler, authSetup, cleanup, err := BuildHandler(cfg)
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
	// §5). Cheap: the expires_at index supports it. Stops with ctx.
	go sweepExpiredDeviceCodes(ctx, authSetup.DeviceCodes)

	return app.Run(ctx, func(ctx context.Context) error {
		slog.Info("starting server", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
}

// AuthComponents holds the always-on bearer-auth pieces: the OAuth/web-token
// handler, the device-flow handler, the shared access-token signer, and the
// device-code repository (which the expiry sweeper uses).
type AuthComponents struct {
	Handler       *server.AuthHandler
	DeviceHandler *server.DeviceAuthHandler
	Tokens        *auth.TokenSigner
	DeviceCodes   *auth.CLIDeviceCodeRepository
}

// BuildHandler wires the complete HTTP handler from cfg and returns it alongside
// the auth components (for the device-code sweeper) and a cleanup func that
// releases the DB pool + inference client. A build error releases anything
// already opened before returning.
func BuildHandler(cfg *config.Config) (http.Handler, *AuthComponents, func(), error) {
	var closers []func()
	cleanup := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}

	var inferenceClient inference.Client
	switch cfg.Inference.Mode {
	case "mock":
		inferenceClient = mock.NewClient()
		slog.Info("using mock inference client (substring grader)")
	case "gemini":
		if cfg.Gemini.APIKey != "" {
			geminiClient := gemini.NewClient(cfg.Gemini.APIKey, cfg.Gemini.Model, inference.DefaultMaxRetryAttempts)
			closers = append(closers, func() { _ = geminiClient.Close() })
			inferenceClient = geminiClient
			slog.Info("using Gemini inference provider", "model", cfg.Gemini.Model)
		} else {
			slog.Warn("GEMINI_API_KEY is not set; quiz grading features will be unavailable")
		}
	default:
		if cfg.OpenAI.APIKey != "" {
			openaiClient := openai.NewClient(cfg.OpenAI.APIKey, cfg.OpenAI.Model, inference.DefaultMaxRetryAttempts)
			closers = append(closers, func() { _ = openaiClient.Close() })
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

	// A database is REQUIRED: auth is always mounted and every RPC is gated, so
	// users / per-user state / device-flow rows must live in Postgres. buildAuth
	// fails fast when db is nil, so if the connection cannot be opened the server
	// refuses to start (rather than silently running ungated).
	var db *sqlx.DB
	if cfg.Database.Host != "" && cfg.Database.Password != "" {
		opened, err := database.Open(cfg.Database)
		if err != nil {
			slog.Error("failed to open database", "error", err)
		} else {
			db = opened
			closers = append(closers, func() { _ = db.Close() })
		}
	}

	// Always-on auth (design §2). Google-OAuth sign-in mints a langner access
	// JWT for the web SPA (delivered in the URL fragment, no session cookie) and
	// the RFC 8628 device flow mints one for the CLI; EVERY connect RPC is gated
	// behind a valid `Authorization: Bearer` token. buildAuth fails fast if the
	// database or TOKEN_SIGNING_KEY is missing, so the server never starts
	// silently ungated.
	authSetup, err := buildAuth(cfg, db)
	if err != nil {
		cleanup()
		return nil, nil, nil, fmt.Errorf("set up auth: %w", err)
	}
	slog.Info("auth enabled; all RPCs require a valid bearer access token (web + CLI)")

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
	// YAML learning_notes files.
	yamlAnalyticsRepo := analytics.NewYAMLRepository(cfg.Notebooks.LearningNotesDirectory)
	// Journals are read alongside stories (they share the story format, see
	// quiz.Service.newReader) so a grammar attempt's notebook — a journal —
	// resolves here too; LoadGrammars then attaches the corrections themselves,
	// which resolveGrammar needs to turn a correction id back into its text.
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

	// historyStore is the DB-backed READ side for learning history, non-nil only
	// when a database is connected. It reconstructs the same per-notebook
	// LearningHistory shape the YAML reader produced, keyed by the SAME canonical
	// storage key the write path used, so reads stay symmetric with writes
	// (learning-history invariant L2). In YAML-only mode it is nil and the quiz
	// service, notebook handler, and analytics fall back to the on-disk files.
	if historyStore != nil {
		yamlAnalyticsRepo = yamlAnalyticsRepo.WithHistoryStore(historyStore)
		slog.Info("database connected; user-state writes go to the database only (learning_notes YAML frozen at runtime), learning-history reads served from DB")
	}
	analyticsRepo := analytics.Repository(yamlAnalyticsRepo)

	svc := quiz.NewService(cfg.Notebooks, inferenceClient, dictionaryMap, learningRepo, cfg.Quiz)
	svc.SetHistoryStore(historyStore)
	svc.SetSkipStores(repos.SkipFlags, repos.Note, repos.Origin)
	// DB mode: install the ensure-on-serve hook so a notebook whose YAML gained
	// units without a fresh `migrate import-db` still gets its notes rows created
	// additively when its cards are served. YAML-only mode leaves the hook nil.
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
	notebookHandler.SetHistoryStore(historyStore)
	notebookHandler.SetNotebookACL(repos.ACL)

	// Per-user notebooks (DB mode only): surface user-pushed content to the quiz
	// service + notebook-detail reader through the DB content source, and give
	// the notebook handler the push/pull/list dependencies. YAML-only mode leaves
	// these nil, so those RPCs return Unimplemented and the reader sees only the
	// shipped catalog.
	if db != nil {
		fileRepo := notebook.NewNotebookFileRepository(db)
		contentSource := notebook.NewDBContentSource(fileRepo)
		svc.SetContentSource(contentSource)
		notebookHandler.SetPushDeps(db, fileRepo, contentSource)
	}

	handler := server.NewQuizHandler(svc)
	handler.SetNoteRepository(noteRepo)
	analyticsHandler := server.NewAnalyticsHandler(analyticsRepo)

	// The auth interceptor rejects any RPC lacking a verified session (populated
	// in request ctx by AuthBearerMiddleware). It is appended UNCONDITIONALLY —
	// auth is always on (design §2).
	interceptors := []connect.Interceptor{loggingInterceptor, server.NewAuthInterceptor()}
	handlerOpts := connect.WithInterceptors(interceptors...)
	path, h := apiv1connect.NewQuizServiceHandler(handler, handlerOpts)
	notebookPath, notebookH := apiv1connect.NewNotebookServiceHandler(notebookHandler, handlerOpts)
	analyticsPath, analyticsH := apiv1connect.NewAnalyticsServiceHandler(analyticsHandler, handlerOpts)

	mux := http.NewServeMux()
	mux.Handle(path, h)
	mux.Handle(notebookPath, notebookH)
	mux.Handle(analyticsPath, analyticsH)

	// Auth endpoints are plain HTTP, registered OUTSIDE the connect interceptor
	// so an unauthenticated user/CLI can sign in. Device approval happens INSIDE
	// /auth/google/callback (no /auth/device/approve).
	mux.HandleFunc("/auth/google/login", authSetup.Handler.Login)
	mux.HandleFunc("/auth/google/callback", authSetup.Handler.Callback)
	mux.HandleFunc("/auth/logout", authSetup.Handler.Logout)
	mux.HandleFunc("/auth/me", authSetup.Handler.Me)
	mux.HandleFunc("/auth/device/code", authSetup.DeviceHandler.RequestCode)
	mux.HandleFunc("/auth/device/token", authSetup.DeviceHandler.Token)
	mux.HandleFunc("/auth/device/deny", authSetup.DeviceHandler.Deny)
	mux.HandleFunc("/auth/device", authSetup.DeviceHandler.ShowApprovalPage)
	mux.HandleFunc("/auth/token/refresh", authSetup.DeviceHandler.Refresh)
	mux.HandleFunc("/auth/token/revoke", authSetup.DeviceHandler.Revoke)

	rootHandler := h2c.NewHandler(mux, &http2.Server{})
	// The ONLY auth path: populate the request context from the bearer access
	// token for both the connect interceptor and /auth/me. Never rejects —
	// /auth/* and CORS preflight pass through; the interceptor gates RPCs.
	rootHandler = server.AuthBearerMiddleware(rootHandler, authSetup.Tokens)

	return corsMiddleware(rootHandler, cfg.Server.CORS.AllowedOrigins), authSetup, cleanup, nil
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

func loadDictionaryMap(cacheDir string) (map[string]rapidapi.Response, error) {
	responses, err := rapidapi.NewReader().Read(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("rapidapi.NewReader().Read() > %w", err)
	}
	return rapidapi.FromResponsesToMap(responses), nil
}

// buildAuth constructs the OAuth/web-token handler, the device-flow handler, the
// single access-token signer (TOKEN_SIGNING_KEY, which also keys the OAuth state
// signer), and the CLI repositories. It fails fast when the database or
// TOKEN_SIGNING_KEY is missing, so a misconfigured deployment never starts
// silently ungated (design §2).
func buildAuth(cfg *config.Config, db *sqlx.DB) (*AuthComponents, error) {
	if db == nil {
		return nil, errors.New("auth requires a database (users / device-flow rows live in Postgres) — set database.* and DB_PASSWORD")
	}
	key := auth.DecodeKey(cfg.Auth.TokenSigningKey)
	tokens, err := auth.NewTokenSigner(key)
	if err != nil {
		return nil, fmt.Errorf("TOKEN_SIGNING_KEY is required: %w", err)
	}
	state, err := auth.NewStateSigner(key)
	if err != nil {
		return nil, fmt.Errorf("state signing key: %w", err)
	}
	users := auth.NewUserRepository(db)
	deviceCodes := auth.NewCLIDeviceCodeRepository(db)
	refreshTokens := auth.NewCLIRefreshTokenRepository(db)
	authenticator := auth.NewOAuthClient(cfg.Auth.GoogleClientID, cfg.Auth.GoogleClientSecret, cfg.Auth.RedirectURL, nil)
	handler := server.NewAuthHandler(server.AuthHandlerConfig{
		Authenticator:  authenticator,
		Tokens:         tokens,
		State:          state,
		Users:          users,
		DeviceCodes:    deviceCodes,
		RefreshTokens:  refreshTokens,
		AllowedEmails:  cfg.Auth.AllowedEmails,
		FrontendURL:    cfg.Auth.FrontendURL,
		AllowedOrigins: cfg.Server.CORS.AllowedOrigins,
		CookieSecure:   cfg.Auth.CookieSecure,
		CookieSameSite: parseSameSite(cfg.Auth.CookieSameSite),
	})
	deviceHandler := server.NewDeviceAuthHandler(server.DeviceAuthHandlerConfig{
		DeviceCodes:     deviceCodes,
		RefreshTokens:   refreshTokens,
		Tokens:          tokens,
		FrontendURL:     cfg.Auth.FrontendURL,
		VerificationURI: deviceVerificationURI(cfg.Auth.RedirectURL),
		CookieSecure:    cfg.Auth.CookieSecure,
		CookieSameSite:  parseSameSite(cfg.Auth.CookieSameSite),
	})
	return &AuthComponents{Handler: handler, DeviceHandler: deviceHandler, Tokens: tokens, DeviceCodes: deviceCodes}, nil
}

// deviceVerificationURI derives the browser approval URL (…/auth/device) from
// the OAuth redirect_url's origin. Falls back to a bare path if it can't parse.
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

// corsMiddleware serves explicit CORS for bearer-authenticated RPCs. Auth is a
// header (`Authorization: Bearer`), not a cookie, so NO
// Access-Control-Allow-Credentials is sent. It reflects the request Origin when
// it is configured (or when "*" is configured) and always allows Authorization.
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
