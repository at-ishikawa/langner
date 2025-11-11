// go build +integration
package openai_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/inference/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClient_AnswerMeanings tests the multiple contexts API integration
// This test requires OPENAI_API_KEY environment variable to be set
// Run with: OPENAI_API_KEY=your-key go test -v ./internal/inference/openai -run TestClient_AnswerMeanings_Evaluate
func TestClient_AnswerMeanings_Evaluate(t *testing.T) {
	t.Parallel()

	slog.SetDefault(
		slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level:     slog.LevelDebug,
			AddSource: true,
		})),
	)

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY environment variable not set, skipping integration test")
	}

	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}

	tests := []struct {
		name         string
		request      inference.AnswerMeaningsRequest
		wantCorrects [][]bool
		description  string
	}{
		{
			name: "Check if the meaning of some word or phrases is correct",
			request: inference.AnswerMeaningsRequest{
				Expressions: []inference.Expression{
					{
						Expression: "run",
						Meaning:    "to move quickly by foot",
						Contexts: []inference.Context{
							{Context: "I run every morning for exercise", Meaning: "to move quickly"},
							{Context: "I run a small business downtown", Meaning: "to operate"},
						},
					},
				},
			},
			wantCorrects: [][]bool{
				{true, false},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, len(tc.request.Expressions), len(tc.wantCorrects))
			client := openai.NewClient(apiKey, model, 1)
			defer func() {
				_ = client.Close()
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.AnswerMeanings(ctx, tc.request)
			require.NoError(t, err)

			for i, wantCorrectForExpression := range tc.wantCorrects {
				answer := result.Answers[i]
				expression := answer.Expression
				for j, wantCorrectForContext := range wantCorrectForExpression {
					context := answer.AnswersForContext[j].Context
					t.Logf("Expression: %s, Context: %s, Expected correct: %v, Got correct: %v", expression, context, wantCorrectForContext, answer.AnswersForContext[j].Correct)

					assert.Equal(t, wantCorrectForContext, answer.AnswersForContext[j].Correct,
						"Expression %d, Context %d: expected correct=%v, got=%v",
						expression, context, wantCorrectForContext, answer.AnswersForContext[j].Correct)
				}
			}
		})
	}
}
