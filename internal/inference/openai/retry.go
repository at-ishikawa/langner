package openai

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/at-ishikawa/langner/internal/inference"
)

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Retry on JSON parsing errors as they might be due to incomplete responses
	errStr := err.Error()
	if contains(errStr, "json.Unmarshal") || contains(errStr, "unexpected end of JSON input") {
		return true
	}

	// Retry on network-related errors
	if contains(errStr, "connection refused") || contains(errStr, "i/o timeout") {
		return true
	}

	// Retry on 5xx errors (server errors)
	if contains(errStr, "response error 5") {
		return true
	}

	// Retry on rate limiting (429)
	if contains(errStr, "response error 429") {
		return true
	}

	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && len(substr) > 0 &&
		(s[:len(substr)] == substr || contains(s[1:], substr)))
}

// AnswerQuestions implements the inference.Client interface
func (client *Client) AnswerMeanings(
	ctx context.Context,
	params inference.AnswerMeaningsRequest,
) (inference.AnswerMeaningsResponse, error) {
	var lastErr error
	retryConfig := client.retryConfig
	backoff := retryConfig.InitialBackoff

	for attempt := 0; attempt <= retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			slog.Default().Info("Retrying OpenAI API call",
				"attempt", attempt,
				"backoff", backoff,
				"expressionCount", len(params.Expressions),
				"lastError", lastErr)

			select {
			case <-ctx.Done():
				return inference.AnswerMeaningsResponse{}, ctx.Err()
			case <-time.After(backoff):
				// Continue with retry
			}

			// Calculate next backoff with exponential increase
			backoff = time.Duration(float64(backoff) * retryConfig.BackoffFactor)
			if backoff > retryConfig.MaxBackoff {
				backoff = retryConfig.MaxBackoff
			}
		}

		response, err := client.answerMeanings(ctx, params)
		if err == nil {
			if attempt > 0 {
				slog.Default().Info("OpenAI API call succeeded after retry",
					"attempt", attempt,
					"expressionCount", len(params.Expressions))
			}
			return response, nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			slog.Default().Debug("Non-retryable error encountered",
				"error", err,
				"expressionCount", len(params.Expressions))
			return response, err
		}

		// For JSON parsing errors, log the problematic content
		if contains(err.Error(), "json.Unmarshal") {
			slog.Default().Warn("JSON parsing error, will retry",
				"attempt", attempt,
				"error", err,
				"expressionCount", len(params.Expressions))
		}
	}

	return inference.AnswerMeaningsResponse{}, fmt.Errorf("failed after %d retries: %w", retryConfig.MaxRetries+1, lastErr)
}
