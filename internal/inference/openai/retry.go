package openai

import (
	"context"
	"encoding/json"
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

// AnswerExpressionWithSingleContext implements the inference.Client interface
func (client *Client) AnswerExpressionWithSingleContext(
	ctx context.Context,
	params inference.AnswerExpressionWithSingleContextParams,
) (inference.AnswerQuestionResponse, error) {
	retryConfig := client.retryConfig
	var lastErr error
	backoff := retryConfig.InitialBackoff

	// Join statements into a single string for the API call
	statementsStr := joinStatements(params.Statements)

	for attempt := 0; attempt <= retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			slog.Default().Info("Retrying OpenAI API call",
				"attempt", attempt,
				"backoff", backoff,
				"expression", params.Expression,
				"lastError", lastErr)

			select {
			case <-ctx.Done():
				return inference.AnswerQuestionResponse{}, ctx.Err()
			case <-time.After(backoff):
				// Continue with retry
			}

			// Calculate next backoff with exponential increase
			backoff = time.Duration(float64(backoff) * retryConfig.BackoffFactor)
			if backoff > retryConfig.MaxBackoff {
				backoff = retryConfig.MaxBackoff
			}
		}

		response, err := client.answerExpressionWithSingleContext(ctx, params.Expression, params.Meaning, statementsStr, params.IsExpressionInput)
		if err == nil {
			if attempt > 0 {
				slog.Default().Info("OpenAI API call succeeded after retry",
					"attempt", attempt,
					"expression", params.Expression)
			}
			return response, nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			slog.Default().Debug("Non-retryable error encountered",
				"error", err,
				"expression", params.Expression)
			return response, err
		}

		// For JSON parsing errors, log the problematic content
		if contains(err.Error(), "json.Unmarshal") {
			slog.Default().Warn("JSON parsing error, will retry",
				"attempt", attempt,
				"error", err,
				"expression", params.Expression)
		}
	}

	return inference.AnswerQuestionResponse{}, fmt.Errorf("failed after %d retries: %w", retryConfig.MaxRetries+1, lastErr)
}

// joinStatements joins a slice of statements with newlines
func joinStatements(statements []string) string {
	result := ""
	for i, stmt := range statements {
		if i > 0 {
			result += "\n"
		}
		result += stmt
	}
	return result
}

// AnswerExpressionWithMultipleContexts implements the inference.Client interface
func (client *Client) AnswerExpressionWithMultipleContexts(
	ctx context.Context,
	params inference.AnswerExpressionWithMultipleContextsParams,
) (inference.MultipleAnswerQuestionResponse, error) {
	var lastErr error
	retryConfig := client.retryConfig
	backoff := retryConfig.InitialBackoff

	for attempt := 0; attempt <= retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			slog.Default().Info("Retrying OpenAI API call",
				"attempt", attempt,
				"backoff", backoff,
				"expression", params.Expression,
				"lastError", lastErr)

			select {
			case <-ctx.Done():
				return inference.MultipleAnswerQuestionResponse{}, ctx.Err()
			case <-time.After(backoff):
				// Continue with retry
			}

			// Calculate next backoff with exponential increase
			backoff = time.Duration(float64(backoff) * retryConfig.BackoffFactor)
			if backoff > retryConfig.MaxBackoff {
				backoff = retryConfig.MaxBackoff
			}
		}

		response, err := client.answerExpressionWithMultipleContexts(ctx, params.Expression, params.Meaning, params.Contexts, params.IsExpressionInput)
		if err == nil {
			if attempt > 0 {
				slog.Default().Info("OpenAI API call succeeded after retry",
					"attempt", attempt,
					"expression", params.Expression)
			}
			return response, nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			slog.Default().Debug("Non-retryable error encountered",
				"error", err,
				"expression", params.Expression)
			return response, err
		}

		// For JSON parsing errors, log the problematic content
		if contains(err.Error(), "json.Unmarshal") {
			slog.Default().Warn("JSON parsing error, will retry",
				"attempt", attempt,
				"error", err,
				"expression", params.Expression)
		}
	}

	return inference.MultipleAnswerQuestionResponse{}, fmt.Errorf("failed after %d retries: %w", retryConfig.MaxRetries+1, lastErr)
}

// tryParsePartialJSON attempts to parse potentially incomplete JSON responses
func tryParsePartialJSON(content string) (*inference.AnswerQuestionResponse, error) {
	// Try to fix common JSON issues
	content = fixCommonJSONIssues(content)

	var result inference.AnswerQuestionResponse
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// fixCommonJSONIssues attempts to fix common JSON formatting problems
func fixCommonJSONIssues(content string) string {
	// First, try to extract just the JSON object if there's extra text
	// Look for the first { and find its matching }
	firstBrace := -1
	lastBrace := -1
	braceCount := 0
	inString := false
	escapeNext := false

	for i, ch := range content {
		// Handle string escaping to avoid counting braces inside strings
		if escapeNext {
			escapeNext = false
			continue
		}
		if ch == '\\' && inString {
			escapeNext = true
			continue
		}
		if ch == '"' && !escapeNext {
			inString = !inString
			continue
		}

		// Only count braces outside of strings
		if !inString {
			switch ch {
			case '{':
				if firstBrace == -1 {
					firstBrace = i
				}
				braceCount++
			case '}':
				braceCount--
				if braceCount == 0 && firstBrace != -1 {
					lastBrace = i
					// Found a complete JSON object, return just this object
					return content[firstBrace : lastBrace+1]
				}
			}
		}
	}

	// If we didn't find a complete object, return what we have
	if firstBrace >= 0 && lastBrace > firstBrace {
		return content[firstBrace : lastBrace+1]
	}

	return content
}
