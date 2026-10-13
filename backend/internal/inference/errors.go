package inference

import (
	"errors"
	"fmt"
)

// ErrNoCredential is returned by a client resolver when the user has not
// registered an LLM provider + API key. Grade handlers map it to a
// FailedPrecondition telling the user to add a key in Settings, rather than an
// opaque internal error.
var ErrNoCredential = errors.New("no llm credential configured")

// ProviderError is a typed error for a non-2xx response from an upstream LLM
// provider. It preserves the HTTP status and the provider's machine-readable
// error code (e.g. "insufficient_quota", "invalid_api_key") so callers can map
// a failure to an actionable message instead of surfacing a stringified HTTP
// body as a blanket internal error.
type ProviderError struct {
	Provider   string // e.g. "openai"
	StatusCode int    // HTTP status code of the failed response
	Code       string // provider error code / type (e.g. "insufficient_quota")
	Message    string // provider error message, if any
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("%s provider error %d (%s): %s", e.Provider, e.StatusCode, e.Code, e.Message)
}

// IsQuota reports whether the error is an out-of-credit / quota failure.
func (e *ProviderError) IsQuota() bool {
	return e.Code == "insufficient_quota" || e.StatusCode == 402
}

// IsInvalidKey reports whether the error is an authentication / invalid-key
// failure.
func (e *ProviderError) IsInvalidKey() bool {
	return e.Code == "invalid_api_key" || e.StatusCode == 401
}
