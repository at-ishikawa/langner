package inference

import "context"

// Provider identifiers for the LLM backends a user may register a key for.
const (
	// ProviderOpenAI is the only functional provider today. Gemini slots in
	// once its client package lands (a commented case in inference/factory).
	ProviderOpenAI = "openai"
)

// AvailableProviders returns the providers the server can build a client for.
// The Settings UI is driven by this list (advertised via /auth/me), and the
// credentials repository validates a registered provider against it. Only
// "openai" is functional on this base; add "gemini" here when its client lands.
func AvailableProviders() []string {
	return []string{ProviderOpenAI}
}

// IsAvailableProvider reports whether provider is one AvailableProviders lists.
func IsAvailableProvider(provider string) bool {
	for _, p := range AvailableProviders() {
		if p == provider {
			return true
		}
	}
	return false
}

// ClientResolver resolves the inference Client to use for one user's request.
// Grading resolves the client per call (rather than holding a boot-time
// singleton) so each user's own provider + API key backs their quizzes.
type ClientResolver interface {
	ResolveClient(ctx context.Context, userID int64) (Client, error)
}

// staticResolver returns a single fixed client regardless of user. It backs the
// mock-grader mode (e2e) and the single-user CLI, where there is no per-user
// credential lookup. A nil client resolves to ErrNoCredential so a missing
// client surfaces the same actionable failure as a missing per-user key.
type staticResolver struct {
	client Client
}

// StaticResolver wraps a single client as a ClientResolver. Used for the mock
// grader (e2e) and the single-user CLI/tests.
func StaticResolver(client Client) ClientResolver {
	return staticResolver{client: client}
}

func (r staticResolver) ResolveClient(context.Context, int64) (Client, error) {
	if r.client == nil {
		return nil, ErrNoCredential
	}
	return r.client, nil
}
