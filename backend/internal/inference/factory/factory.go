// Package factory builds inference clients from a provider name + credentials
// and resolves the per-user client from stored credentials. It lives in its own
// package (not internal/inference) because it imports the concrete
// internal/inference/openai client, which itself imports internal/inference —
// keeping Build here avoids an import cycle.
package factory

import (
	"context"
	"fmt"

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

// Resolver resolves a user's inference client from their stored credential. It
// implements inference.ClientResolver. A user with no credential resolves to
// inference.ErrNoCredential.
type Resolver struct {
	credentials *auth.CredentialsRepository
}

// NewResolver builds a per-user client resolver backed by the credentials repo.
func NewResolver(credentials *auth.CredentialsRepository) *Resolver {
	return &Resolver{credentials: credentials}
}

// ResolveClient loads the user's credentials (primary first), decrypts the
// keys, and builds the client. With one provider it returns that client; with a
// second provider configured it wraps the primary in a rate-limit/quota
// fallback to the other provider. Returns inference.ErrNoCredential when the
// user has registered no provider key.
func (r *Resolver) ResolveClient(ctx context.Context, userID int64) (inference.Client, error) {
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
