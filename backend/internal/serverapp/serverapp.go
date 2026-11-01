// Package serverapp builds and runs langner's complete HTTP server — the connect
// RPC services, the plain-HTTP /auth/* routes, bearer-auth middleware, and CORS.
// It is the single wiring point shared by the CLI entrypoint (cmd/langner-server)
// and the Vercel Go-framework entrypoint (cmd/server), so both run an identical
// stack. The server listens on cfg.Server.Port, which config binds to $PORT — the
// port Vercel's Go framework routes traffic to.
package serverapp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
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
	"github.com/at-ishikawa/langner/internal/clicmd"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/inference/factory"
	"github.com/at-ishikawa/langner/internal/inference/mock"
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
	// Credentials is the per-user LLM-key store, non-nil only when
	// CREDENTIAL_ENCRYPTION_KEY is set. It backs the `user` inference mode and
	// the /auth/llm-credential endpoints.
	Credentials *auth.CredentialsRepository
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

	// The dictionary is served from Postgres only (filled below once the DB is
	// connected). The on-disk rapidapi cache is no longer read by the server — it
	// is absent on a serverless deploy and the dictionary lives in
	// dictionary_entries.
	dictionaryMap := map[string]rapidapi.Response{}

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
			database.LogUnattributedQueries()
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

	// Resolve the inference client that backs quiz grading. There is NO
	// system-wide OpenAI/Gemini key: every learner grades with their OWN stored
	// LLM key (the multi-user model). The only mode is per-user; `mock` is kept
	// solely for e2e/tests (a deterministic grader, no key). A nil resolver
	// (CREDENTIAL_ENCRYPTION_KEY unset, or a user with no key yet) resolves to
	// ErrNoCredential, surfaced as a clear FailedPrecondition, not a crash.
	var clientResolver inference.ClientResolver
	switch cfg.Inference.Mode {
	case "mock":
		clientResolver = inference.StaticResolver(mock.NewClient())
		slog.Info("using mock inference client (substring grader; e2e/tests only)")
	default:
		if authSetup != nil && authSetup.Credentials != nil {
			resolver := factory.NewResolver(authSetup.Credentials)
			// Drop a user's cached client the moment they change their key, so a
			// rotated/added credential takes effect immediately instead of after
			// the resolver's cache TTL.
			if authSetup.Handler != nil {
				authSetup.Handler.SetCredentialInvalidator(resolver.Invalidate)
			}
			clientResolver = resolver
			slog.Info("grading with each user's own LLM credential")
		} else {
			clientResolver = inference.StaticResolver(nil)
			slog.Warn("per-user LLM credentials unavailable (set CREDENTIAL_ENCRYPTION_KEY); quiz grading will report 'no API key configured' until a user adds their key in Settings")
		}
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

	// Notebook content is stored in Postgres (the content_* tables): every
	// reader below is built per request and loads only the notebooks the
	// request reads.
	// Dictionary-backed words resolve from dictionary_entries, fetched with the
	// notebooks that use them.
	var store *notebook.NotebookStore
	var content notebook.ContentLoader
	var dictionaryEntries notebook.DictionarySource
	var userDefsRepo *notebook.UserDefinitionsRepository
	if db != nil {
		dictionaryEntries = dictionary.NewDBDictionaryRepository(db)
		store = notebook.NewNotebookStore(db)
		content = notebook.NewCachedContent(notebook.NewContentRepository(db))
		userDefsRepo = notebook.NewUserDefinitionsRepository(db)
	}

	// Analytics reads through the same learning-history seam the quiz service
	// uses, and resolves a wrong word's meaning / grammar correction from the
	// stored notebooks.
	yamlAnalyticsRepo := analytics.NewYAMLRepository(cfg.Notebooks.LearningNotesDirectory)
	if content != nil {
		yamlAnalyticsRepo = yamlAnalyticsRepo.WithMetadataResolver(analytics.NewContentMetadataResolver(
			func(ctx context.Context) (*notebook.Reader, error) {
				return notebook.NewContentReader(ctx, content, dictionaryEntries)
			}))
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

	svc := quiz.NewService(cfg.Notebooks, clientResolver, dictionaryMap, learningRepo, cfg.Quiz)
	svc.SetHistoryStore(historyStore)
	svc.SetSkipStores(repos.SkipFlags, repos.Note, repos.Origin)

	if content != nil {
		svc.SetContent(content, dictionaryEntries)
		// Per-user highlight overlay on the quiz card loaders.
		svc.SetUserDefinitionsRepo(userDefsRepo)
	}
	// Enforce notebook public/private visibility on every quiz read path (auth
	// Phase 3). Nil (no DB) keeps every notebook visible.
	svc.SetNotebookACL(repos.ACL)

	dictConfig := dictionary.Config{
		RapidAPIHost: cfg.Dictionaries.RapidAPI.Host,
		RapidAPIKey:  cfg.Dictionaries.RapidAPI.Key,
	}
	// DB-backed live lookup: a cache miss is fetched from the external API and
	// persisted to dictionary_entries — no filesystem cache. Nil without a DB
	// (the handler guards it), so no server path reads the on-disk cache.
	var dictReader *dictionary.Reader
	if db != nil {
		dictReader = dictionary.NewDBReader(dictionary.NewDBDictionaryRepository(db), dictConfig)
	}
	notebookHandler := server.NewNotebookHandler(cfg.Notebooks, cfg.Templates, dictReader, clientResolver, noteRepo)
	notebookHandler.SetHistoryStore(historyStore)
	notebookHandler.SetNotebookACL(repos.ACL)

	// Give the notebook handler the stored notebooks to read and the
	// push/pull/list store.
	if db != nil {
		notebookHandler.SetContent(content, dictionaryEntries)
		notebookHandler.SetPushDeps(db, store)
		// Per-user registrations + Gutenberg import (DB mode). A nil HTTP client
		// makes gutenberg use a default client with a timeout; real outbound HTTP
		// runs only in preview/prod.
		notebookHandler.SetBookDeps(userDefsRepo, nil)
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
	// POST/DELETE register/clear the signed-in user's own LLM API key (the
	// Settings page). Behind the same bearer middleware as /auth/me; the handler
	// itself 503s when credential storage is unconfigured.
	mux.HandleFunc("/auth/llm-credential", authSetup.Handler.LLMCredential)
	mux.HandleFunc("/auth/device/code", authSetup.DeviceHandler.RequestCode)
	mux.HandleFunc("/auth/device/token", authSetup.DeviceHandler.Token)
	// Deny is a plain redirect endpoint; the approval PAGE (GET /auth/device) is a
	// frontend route — the API renders no user-facing HTML.
	mux.HandleFunc("/auth/device/deny", authSetup.DeviceHandler.Deny)
	mux.HandleFunc("/auth/token/refresh", authSetup.DeviceHandler.Refresh)
	mux.HandleFunc("/auth/token/revoke", authSetup.DeviceHandler.Revoke)

	// Preview-only e2e seed endpoint: mounted ONLY when E2E_SEED_SECRET is set
	// (preview env; never in prod). Guarded by that shared secret, it resets the
	// deployed preview's learning state to the e2e baseline and mints an e2e
	// bearer, so CI can seed the running preview over HTTP without holding any
	// database credential or signing key. Registered OUTSIDE the connect auth
	// interceptor (like /auth/*) so the unauthenticated runner can call it.
	if cfg.Server.E2ESeedSecret != "" && db != nil {
		mux.HandleFunc("/internal/e2e/seed", e2eSeedHandler(cfg, db))
		slog.Warn("e2e seed endpoint ENABLED at POST /internal/e2e/seed — preview only, guarded by E2E_SEED_SECRET")
	}

	rootHandler := h2c.NewHandler(mux, &http2.Server{})
	// The ONLY auth path: populate the request context from the bearer access
	// token for both the connect interceptor and /auth/me. Never rejects —
	// /auth/* and CORS preflight pass through; the interceptor gates RPCs.
	rootHandler = server.AuthBearerMiddleware(rootHandler, authSetup.Tokens)
	// Logs one "db usage" line per request that queried Postgres (counts, rows,
	// bytes received, top query shapes — never data), to find endpoints that
	// pull a lot of data from the database.
	rootHandler = database.QueryStatsMiddleware(rootHandler)

	return corsMiddleware(rootHandler, cfg.Server.CORS.AllowedOrigins), authSetup, cleanup, nil
}

// e2eSeedHandler returns the preview-only seed endpoint. It requires the shared
// secret in the X-E2E-Seed-Secret header (constant-time compared), resets the DB
// to the e2e baseline (clicmd.PrepareE2EDatabase), mints an e2e bearer for the
// configured admin account, and returns {"token","email"} as JSON. It is only
// registered when E2E_SEED_SECRET is set, so prod never exposes it.
func e2eSeedHandler(cfg *config.Config, db *sqlx.DB) http.HandlerFunc {
	secret := []byte(cfg.Server.E2ESeedSecret)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-E2E-Seed-Secret")), secret) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		// Optional request body: a gzipped tar of the notebook fixture content,
		// uploaded by CI so the serverless backend (no fixtures on disk) can
		// import the catalog. Empty body ⇒ skip import (catalog assumed present).
		// Cap the body as a modest upload-bomb guard.
		var content io.Reader
		if r.ContentLength != 0 {
			content = http.MaxBytesReader(w, r.Body, 64<<20)
		}
		if err := clicmd.PrepareE2EDatabase(r.Context(), cfg, db, content); err != nil {
			slog.Error("e2e seed failed", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		token, err := clicmd.IssueTestToken(r.Context(), cfg, db, cfg.Auth.InitialAdminEmail)
		if err != nil {
			slog.Error("e2e token mint failed", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"token": token, "email": cfg.Auth.InitialAdminEmail})
	}
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

	// Per-user LLM credentials are encrypted at rest with CREDENTIAL_ENCRYPTION_KEY.
	// OPTIONAL: without the key the credentials repo stays nil, so the server still
	// starts (mock/openai/gemini modes) and per-user keys are simply disabled.
	var credentials *auth.CredentialsRepository
	if cfg.Auth.CredentialEncryptionKey != "" {
		enc, err := auth.NewEncryptor(auth.DecodeKey(cfg.Auth.CredentialEncryptionKey))
		if err != nil {
			return nil, fmt.Errorf("CREDENTIAL_ENCRYPTION_KEY invalid: %w", err)
		}
		credentials = auth.NewCredentialsRepository(db, enc)
	}

	authenticator := auth.NewOAuthClient(cfg.Auth.GoogleClientID, cfg.Auth.GoogleClientSecret, cfg.Auth.RedirectURL, auth.Endpoint{
		AuthURL:     cfg.Auth.OAuthEndpoint.AuthURL,
		TokenURL:    cfg.Auth.OAuthEndpoint.TokenURL,
		UserInfoURL: cfg.Auth.OAuthEndpoint.UserInfoURL,
	}, nil)
	handler := server.NewAuthHandler(server.AuthHandlerConfig{
		Authenticator:  authenticator,
		Tokens:         tokens,
		State:          state,
		Users:          users,
		DeviceCodes:    deviceCodes,
		RefreshTokens:  refreshTokens,
		Credentials:    credentials,
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
		VerificationURI: deviceVerificationURI(cfg.Auth.FrontendURL),
		CookieSecure:    cfg.Auth.CookieSecure,
		CookieSameSite:  parseSameSite(cfg.Auth.CookieSameSite),
	})
	return &AuthComponents{Handler: handler, DeviceHandler: deviceHandler, Tokens: tokens, DeviceCodes: deviceCodes, Credentials: credentials}, nil
}

// deviceVerificationURI derives the browser approval URL (…/auth/device) from
// the FRONTEND origin: the approval page is a frontend route (the API renders no
// user-facing HTML; it only redirects). Falls back to a bare path if it can't
// parse.
func deviceVerificationURI(frontendURL string) string {
	u, err := url.Parse(frontendURL)
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

// matchGlobOrigin matches a `*`-glob pattern against an origin, anchored at both
// ends. Only `*` is special (any run of chars); everything else is literal. Used
// so a single Preview-scope CORS entry can match this project's per-branch
// preview origins (langner-git-<slug>-at-ishikawa.vercel.app) without an open `*`.
func matchGlobOrigin(pattern, origin string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == origin
	}
	if !strings.HasPrefix(origin, parts[0]) {
		return false
	}
	rest := origin[len(parts[0]):]
	for _, mid := range parts[1 : len(parts)-1] {
		idx := strings.Index(rest, mid)
		if idx < 0 {
			return false
		}
		rest = rest[idx+len(mid):]
	}
	last := parts[len(parts)-1]
	return len(rest) >= len(last) && strings.HasSuffix(rest, last)
}

// corsMiddleware serves explicit CORS for bearer-authenticated RPCs. Auth is a
// header (`Authorization: Bearer`), not a cookie, so NO
// Access-Control-Allow-Credentials is sent. It reflects the request Origin when
// it is configured (or when "*" is configured) and always allows Authorization.
func corsMiddleware(next http.Handler, allowedOrigins []string) http.Handler {
	allowAll := false
	exact := make(map[string]bool, len(allowedOrigins))
	var patterns []string
	for _, o := range allowedOrigins {
		switch {
		case o == "*":
			allowAll = true
		case strings.Contains(o, "*"):
			// A `*`-glob entry (e.g. https://langner-git-*-at-ishikawa.vercel.app)
			// lets Preview scope allow this project's per-branch preview origins
			// without opening CORS to every site — the branch slug varies per PR
			// but the account-scoped suffix anchors it to our own deployments.
			patterns = append(patterns, o)
		default:
			exact[o] = true
		}
	}
	originAllowed := func(origin string) bool {
		if allowAll || exact[origin] {
			return true
		}
		for _, p := range patterns {
			if matchGlobOrigin(p, origin) {
				return true
			}
		}
		return false
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && originAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		// DELETE is needed for the browser "Remove key" call (DELETE
		// /auth/llm-credential); the rest of the API is GET/POST (connect RPCs).
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Connect-Protocol-Version, Authorization")
		w.Header().Set("Access-Control-Max-Age", "3600")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
