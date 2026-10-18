// Package factory builds inference clients from a provider name + credentials
// and resolves the per-user client from stored credentials. It lives in its own
// package (not internal/inference) because it imports the concrete
// internal/inference/openai client, which itself imports internal/inference —
// keeping Build here avoids an import cycle.
package factory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/inference/fallback"
	"github.com/at-ishikawa/langner/internal/inference/gemini"
	"github.com/at-ishikawa/langner/internal/inference/openai"
)

// Default models used when a user registers a key without naming a model. They
// match the server's historical openai.model / gemini.model defaults.
const (
	defaultOpenAIModel = "gpt-4o-mini"
	defaultGeminiModel = "gemini-3.5-flash-lite"
)

// Build constructs an inference client for the given provider credentials.
// provider must be one of inference.AvailableProviders(). An empty model falls
// back to the provider's default.
func Build(provider, apiKey, model string) (inference.Client, error) {
	switch provider {
	case inference.ProviderOpenAI:
		if model == "" {
			model = defaultOpenAIModel
		}
		return openai.NewClient(apiKey, model, inference.DefaultMaxRetryAttempts), nil
	case inference.ProviderGemini:
		if model == "" {
			model = defaultGeminiModel
		}
		return gemini.NewClient(apiKey, model, inference.DefaultMaxRetryAttempts), nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", provider)
	}
}

// defaultClientCacheTTL is how long a resolved per-user client is reused before
// its credentials are re-read and decrypted. Resolving used to run on EVERY
// grade (a DB read + key decrypt + fresh HTTP-client build per quiz/relearn
// card); caching collapses that to once per TTL per user, which is the dominant
// controllable per-card overhead in a relearn session. The window is also the
// MAX staleness after a user changes their provider key: a key edit takes effect
// within this TTL (kept short for that reason). Reusing the client additionally
// lets its HTTP connection pool stay warm across cards.
const defaultClientCacheTTL = 60 * time.Second

// credentialsReader is the slice of *auth.CredentialsRepository the resolver
// needs: load + decrypt a user's provider credentials. Narrowing to an interface
// lets the cache behavior be unit-tested with a fake instead of a live DB.
type credentialsReader interface {
	DecryptedCredentials(ctx context.Context, userID int64) ([]auth.LLMCredential, error)
}

// Resolver resolves a user's inference client from their stored credential. It
// implements inference.ClientResolver. A user with no credential resolves to
// inference.ErrNoCredential. Resolved clients are cached per user for a short
// TTL so repeated grades don't re-hit the DB / re-decrypt the key every card.
type Resolver struct {
	credentials credentialsReader

	ttl   time.Duration
	now   func() time.Time // injectable for tests; defaults to time.Now
	mu    sync.Mutex
	cache map[int64]cachedClient
}

type cachedClient struct {
	client inference.Client
	expiry time.Time
}

// NewResolver builds a per-user client resolver backed by the credentials repo.
func NewResolver(credentials *auth.CredentialsRepository) *Resolver {
	return &Resolver{
		credentials: credentials,
		ttl:         defaultClientCacheTTL,
		now:         time.Now,
		cache:       make(map[int64]cachedClient),
	}
}

// ResolveClient returns the user's inference client, reusing a cached one while
// it is still fresh. On a miss it loads + decrypts credentials and builds the
// client (see buildClient), then caches it for r.ttl. Returns
// inference.ErrNoCredential when the user has registered no provider key.
func (r *Resolver) ResolveClient(ctx context.Context, userID int64) (inference.Client, error) {
	now := r.now()
	r.mu.Lock()
	if e, ok := r.cache[userID]; ok && now.Before(e.expiry) {
		client := e.client
		r.mu.Unlock()
		return client, nil
	}
	r.mu.Unlock()

	// Build outside the lock so a slow DB read for one user never blocks grades
	// for others. A rare concurrent miss for the SAME user may build twice; both
	// clients are valid and the last write wins — not worth a singleflight.
	client, err := r.buildClient(ctx, userID)
	if err != nil {
		// Do not cache failures (ErrNoCredential / transient DB errors): a user
		// who has just added a key must not be told "no credential" for a whole TTL.
		return nil, err
	}

	r.mu.Lock()
	r.cache[userID] = cachedClient{client: client, expiry: r.now().Add(r.ttl)}
	r.mu.Unlock()
	return client, nil
}

// Invalidate drops any cached client for the user so the next ResolveClient
// rebuilds from fresh credentials. Call it after a credential change to make the
// new key take effect immediately rather than waiting out the TTL.
func (r *Resolver) Invalidate(userID int64) {
	r.mu.Lock()
	delete(r.cache, userID)
	r.mu.Unlock()
}

// buildClient loads the user's credentials (primary first), decrypts the keys,
// and builds the client. With one provider it returns that client; with a second
// provider configured it wraps the primary in a rate-limit/quota fallback to the
// other provider.
func (r *Resolver) buildClient(ctx context.Context, userID int64) (inference.Client, error) {
	creds, err := r.credentials.DecryptedCredentials(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get credentials: %w", err)
	}
	if len(creds) == 0 {
		return nil, inference.ErrNoCredential
	}
	primary, err := Build(creds[0].Provider, creds[0].APIKey, creds[0].Model)
	if err != nil {
		return nil, err
	}
	// creds are ordered primary-first; a second provider becomes the automatic
	// fallback the grader retries on when the primary hits a rate-limit/quota.
	if len(creds) > 1 {
		secondary, err := Build(creds[1].Provider, creds[1].APIKey, creds[1].Model)
		if err != nil {
			return nil, err
		}
		return fallback.NewClient(primary, secondary), nil
	}
	return primary, nil
}
