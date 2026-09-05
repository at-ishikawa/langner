package gemini_test

import (
	"testing"

	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/inference/gemini"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	client := gemini.NewClient("test-key", "gemini-2.0-flash", 3)
	require.NotNil(t, client)
	defer func() { _ = client.Close() }()

	// The adapter must implement the shared inference.Client interface.
	var _ inference.Client = client

	// It reuses the shared openai.Client but points at Gemini's
	// OpenAI-compatible endpoint, and passes through the configured model.
	assert.Equal(t, "gemini-2.0-flash", client.GetModel())
	assert.Equal(t, gemini.DefaultBaseURL, client.BaseURL())
}
