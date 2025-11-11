package inference

import (
	"context"
	"time"
)

//go:generate mockgen -source=interface.go -destination=../mocks/inference/mock_client.go -package=mock_inference

// Client interface defines the methods for AI inference operations
type Client interface {
	AnswerMeanings(ctx context.Context, params AnswerMeaningsRequest) (AnswerMeaningsResponse, error)
}

// Expression represents a single expression with its contexts
type Expression struct {
	Expression        string     `json:"expression"`
	Meaning           string     `json:"meaning"`
	Contexts          [][]string `json:"contexts,omitempty"`
	IsExpressionInput bool       `json:"is_expression_input"`
}

// AnswerMeaningsRequest holds parameters for answering multiple expressions
type AnswerMeaningsRequest struct {
	Expressions []Expression `json:"expressions"`
}

type AnswerMeaningsResponse struct {
	Answers []AnswerMeaning
}

// AnswerMeaning represents a single answer result
type AnswerMeaning struct {
	Expression        string              `json:"expression"`
	Meaning           string              `json:"meaning"`
	AnswersForContext []AnswersForContext `json:"answers"`
}

// AnswersForContext represents a single answer for a specific context
type AnswersForContext struct {
	Correct bool   `json:"correct"`
	Context string `json:"context"`
}

// RetryConfig holds configuration for retry logic
type RetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	BackoffFactor  float64
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		BackoffFactor:  2.0,
	}
}
