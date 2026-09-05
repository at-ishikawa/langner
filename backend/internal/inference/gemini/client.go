// Package gemini provides an inference.Client backed by Google Gemini.
//
// Gemini exposes an OpenAI-compatible Chat Completions REST endpoint that
// accepts the same request/response schema (model, messages, temperature) with
// an "Authorization: Bearer <GEMINI_API_KEY>" header. Rather than duplicating
// the ~1100 lines of prompt-building and response-parsing logic, this package
// is a thin adapter: it constructs the shared openai.Client pointed at Gemini's
// base URL via openai.WithBaseURL. The returned *openai.Client implements
// inference.Client unchanged.
package gemini

import (
	"github.com/at-ishikawa/langner/internal/inference/openai"
)

// DefaultBaseURL is Gemini's OpenAI-compatible Chat Completions base URL.
// Requests POST to "<DefaultBaseURL>/chat/completions".
const DefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"

// NewClient builds an inference client that talks to Gemini through its
// OpenAI-compatible endpoint, reusing the shared openai.Client implementation.
func NewClient(apiKey, model string, retryAttempts uint) *openai.Client {
	return openai.NewClient(apiKey, model, retryAttempts, openai.WithBaseURL(DefaultBaseURL))
}
