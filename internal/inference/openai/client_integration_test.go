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

// TestAnswerExpressionWithSingleContextIntegration tests the actual OpenAI API integration
// This test requires OPENAI_API_KEY environment variable to be set
// Run with: OPENAI_API_KEY=your-key go test -v ./internal/inference/openai -run TestAnswerExpressionWithSingleContextIntegration
func TestAnswerExpressionWithSingleContextIntegration(t *testing.T) {
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
		name              string
		expression        string
		userMeaning       string
		context           string
		isExpressionInput bool
		expectCorrect     bool
		description       string
	}{
		{
			name:              "run",
			expression:        "run",
			userMeaning:       "to move quickly by foot",
			context:           "I run every morning for exercise",
			isExpressionInput: true,
			expectCorrect:     true,
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

			result, err := client.AnswerExpressionWithSingleContext(ctx,
				inference.AnswerExpressionWithSingleContextParams{
					Expression:        tc.expression,
					Meaning:           tc.userMeaning,
					Statements:        []string{tc.context},
					IsExpressionInput: tc.isExpressionInput,
				},
			)
			require.NoError(t, err, "API call should not fail")

			assert.Equal(t, tc.expectCorrect, result.Correct,
				"Expected correct=%v for '%s' with meaning '%s'\nAI returned meaning: %s",
				tc.expectCorrect, tc.expression, tc.userMeaning, result.Meaning)

			assert.Equal(t, tc.expression, result.Expression, "Expression should be preserved")
			assert.NotEmpty(t, result.Meaning, "AI should provide a meaning")
		})
	}
}

// TestAnswerExpressionWithMultipleContextsIntegration tests the multiple contexts API integration
// This test requires OPENAI_API_KEY environment variable to be set
// Run with: OPENAI_API_KEY=your-key go test -v ./internal/inference/openai -run TestAnswerExpressionWithMultipleContextsIntegration
func TestAnswerExpressionWithMultipleContextsIntegration(t *testing.T) {
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
		name              string
		expression        string
		userMeaning       string
		contexts          [][]string
		isExpressionInput bool
		expectedCorrect   []bool // Expected correctness for each context
		description       string
	}{
		{
			name:        "run - multiple meanings, match one",
			expression:  "run",
			userMeaning: "to move quickly by foot",
			contexts: [][]string{
				{"I run every morning for exercise"},
				{"I run a small business downtown"},
			},
			isExpressionInput: true,
			expectedCorrect:   []bool{true, false}, // First context matches, second doesn't
		},
		{
			name:        "turn down - same meaning in different contexts",
			expression:  "turn down",
			userMeaning: "to reject or refuse",
			contexts: [][]string{
				{"She turned down the job offer"},
				{"He turned down the invitation to the party"},
			},
			isExpressionInput: false,
			expectedCorrect:   []bool{true, true}, // Both contexts match
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

			result, err := client.AnswerExpressionWithMultipleContexts(ctx,
				inference.AnswerExpressionWithMultipleContextsParams{
					Expression:        tc.expression,
					Meaning:           tc.userMeaning,
					Contexts:          tc.contexts,
					IsExpressionInput: tc.isExpressionInput,
				},
			)
			require.NoError(t, err, "API call should not fail")

			assert.Equal(t, tc.expression, result.Expression, "Expression should be preserved")
			assert.Equal(t, tc.isExpressionInput, result.IsExpressionInput, "IsExpressionInput should be preserved")
			assert.NotEmpty(t, result.Meaning, "AI should provide a meaning")

			// Flatten contexts to match the response format
			var allContexts []string
			for _, contextGroup := range tc.contexts {
				allContexts = append(allContexts, contextGroup...)
			}

			require.Len(t, result.AnswersForContext, len(allContexts),
				"Should have answer for each context")

			// Check each answer
			for i, answer := range result.AnswersForContext {
				assert.Equal(t, allContexts[i], answer.Context,
					"Context %d should match", i)
				assert.Equal(t, tc.expectedCorrect[i], answer.Correct,
					"Context %d: expected correct=%v for context '%s'\nUser meaning: '%s'\nAI meaning: '%s'",
					i, tc.expectedCorrect[i], answer.Context, tc.userMeaning, result.Meaning)
			}
		})
	}
}
