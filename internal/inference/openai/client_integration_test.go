// go build +integration
package openai_test

import (
	"context"
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
						Contexts:   [][]string{{"I run every morning for exercise"}, {"I run a small business downtown"}},
					},
					{
						Expression:        "turn down",
						Meaning:           "to reject or refuse",
						IsExpressionInput: true,
						Contexts:          [][]string{{"She turned down the job offer"}, {"He turned down the invitation to the party"}},
					},
				},
			},
			wantCorrects: [][]bool{
				{true, false},
				{true, true},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client := openai.NewClient(apiKey, model, inference.RetryConfig{MaxRetries: 0})
			defer func() {
				_ = client.Close()
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.AnswerMeanings(ctx, tc.request)
			require.NoError(t, err)

			for i, wantCorrectForExpression := range tc.wantCorrects {
				for j, wantCorrectForContext := range wantCorrectForExpression {
					assert.Equal(t, wantCorrectForContext, result.Answers[i].AnswersForContext[j].Correct,
						"Expression %d, Context %d: expected correct=%v, got=%v",
						i, j, wantCorrectForContext, result.Answers[i].AnswersForContext[j].Correct)
				}
			}
		})
	}
}
