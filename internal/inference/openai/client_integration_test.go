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

type IntegrationTestTargetWant struct {
	Correct bool
	Reason  string
}

type IntegrationTestTarget struct {
	Expression inference.Expression
	Wants      []IntegrationTestTargetWant
}

// integrationTestTargets can be set in client_integration_data_test.go to override the default tests
// This is used to test for each user's case
var integrationTestTargets []IntegrationTestTarget

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

	slog.SetDefault(
		slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level:     slog.LevelDebug,
			AddSource: true,
		})),
	)

	if integrationTestTargets == nil {
		integrationTestTargets = []IntegrationTestTarget{
			{
				Expression: inference.Expression{
					Expression: "run",
					Meaning:    "to move quickly by foot",
					Contexts: []inference.Context{
						{Context: "I run every morning for exercise", ReferenceDefinition: "to move quickly"},
						{Context: "I run a small business downtown", ReferenceDefinition: "to operate"},
					},
				},
				Wants: []IntegrationTestTargetWant{
					{Correct: true, Reason: "user meaning matches the context"},
					{Correct: false, Reason: "user meaning is about moving, not operating"},
				},
			},
		}
	}

	tests := []struct {
		name string

		testTarget []IntegrationTestTarget
	}{
		{
			name:       "Check if the meaning of some word or phrases is correct",
			testTarget: integrationTestTargets,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			request := inference.AnswerMeaningsRequest{
				Expressions: []inference.Expression{},
			}
			for _, testTarget := range tc.testTarget {
				request.Expressions = append(request.Expressions, testTarget.Expression)
			}

			require.Equal(t, len(request.Expressions), len(tc.testTarget))
			client := openai.NewClient(apiKey, model, 0)
			defer func() {
				_ = client.Close()
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			result, err := client.AnswerMeanings(ctx, request)
			require.NoError(t, err)

			for i, testTarget := range tc.testTarget {
				answer := result.Answers[i]
				expression := answer.Expression
				for j, want := range testTarget.Wants {
					context := answer.AnswersForContext[j].Context
					t.Logf("Expression: %s, Context: %s, Expected correct: %v, Got correct: %v, Got meaning: %s, Why %v is expected: %s",
						expression, context, want.Correct, answer.AnswersForContext[j].Correct, answer.Meaning, want.Correct, want.Reason)

					assert.Equal(t, want.Correct, answer.AnswersForContext[j].Correct,
						"Expression %s, Context %s: want correct=%v, got=%v, got meaning=%s, Why %v is expected: %s",
						expression, context, want.Correct, answer.AnswersForContext[j].Correct, answer.Meaning, want.Correct, want.Reason)
				}
			}
		})
	}
}
