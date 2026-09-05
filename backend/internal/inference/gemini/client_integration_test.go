//go:build integration

package gemini_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/inference/gemini"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClient_LiveGemini_ValidateWordForm makes a REAL call to Google Gemini's
// OpenAI-compatible Chat Completions endpoint to prove the provider wiring and
// the configured model work end-to-end (it exercises the reverse-quiz grade
// path, ValidateWordForm).
//
// It requires the GEMINI_API_KEY environment variable (provided as a GitHub
// Actions secret in CI) and SKIPS when it is absent, so pushes/PRs without
// access to the secret do not fail. The key is never stored in the repo.
//
// Run locally:
//
//	GEMINI_API_KEY=... go test -tags integration -run LiveGemini ./internal/inference/gemini
func TestClient_LiveGemini_ValidateWordForm(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set; skipping live Gemini integration test")
	}

	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-3.6-flash"
	}

	client := gemini.NewClient(apiKey, model, inference.DefaultMaxRetryAttempts)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// A learner typing the exact expected word must be graded as the same word;
	// asserting on "not wrong" keeps the test robust against LLM variance while
	// still proving the round-trip (request build → Gemini call → response parse)
	// works against the live free-tier model.
	resp, err := client.ValidateWordForm(ctx, inference.ValidateWordFormRequest{
		Expected:   "reluctant",
		UserAnswer: "reluctant",
		Meaning:    "unwilling and hesitant to do something",
	})
	require.NoError(t, err, "live Gemini ValidateWordForm call should succeed")
	assert.Contains(t, []inference.ValidateWordFormClassification{
		inference.ClassificationSameWord,
		inference.ClassificationSynonym,
		inference.ClassificationWrong,
	}, resp.Classification, "response must carry a valid classification")
	assert.NotEqual(t, inference.ClassificationWrong, resp.Classification,
		"the exact expected word must not be graded wrong")
}
