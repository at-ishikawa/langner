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
	"github.com/at-ishikawa/langner/internal/inference/openai"
)

// defaultOpenAIModel is used when a user registers an OpenAI key without naming
// a model. It matches the server's historical openai.model default.
const defaultOpenAIModel = "gpt-4o-mini"

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
	// case "gemini":
	//	// Enable once the gemini client package lands (#63):
	//	//   return gemini.NewClient(apiKey, model), nil
	//	// Until then "gemini" is not in inference.AvailableProviders(), so the
	//	// credentials repository rejects it and this case is unreachable.
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

// ResolveClient loads the user's credential, decrypts the key, and builds the
// matching client. It returns inference.ErrNoCredential when the user has not
// registered a provider + key.
func (r *Resolver) ResolveClient(ctx context.Context, userID int64) (inference.Client, error) {
	cred, ok, err := r.credentials.GetCredential(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get credential: %w", err)
	}
	if !ok {
		return nil, inference.ErrNoCredential
	}
	return Build(cred.Provider, cred.APIKey, cred.Model)
}
