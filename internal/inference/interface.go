package inference

import (
	"context"
	"time"
)

//go:generate mockgen -source=interface.go -destination=../mocks/inference/mock_client.go -package=mock_inference

// Client interface defines the methods for AI inference operations
type Client interface {
	AnswerExpressionWithSingleContext(ctx context.Context, params AnswerExpressionWithSingleContextParams) (AnswerQuestionResponse, error)
	AnswerExpressionWithMultipleContexts(ctx context.Context, params AnswerExpressionWithMultipleContextsParams) (MultipleAnswerQuestionResponse, error)
}

// AnswerExpressionWithSingleContextParams holds parameters for answering questions with a single context
type AnswerExpressionWithSingleContextParams struct {
	Expression        string
	Meaning           string
	Statements        []string
	IsExpressionInput bool // true for FreeformQuiz (user inputs expression), false for NotebookQuiz (user inputs meaning)
}

// AnswerExpressionWithMultipleContextsParams holds parameters for answering questions with multiple contexts
type AnswerExpressionWithMultipleContextsParams struct {
	Expression        string
	Meaning           string
	Contexts          [][]string // Multiple sets of contexts (one per occurrence)
	IsExpressionInput bool
}

// AnswerQuestionResponse represents a single answer result
type AnswerQuestionResponse struct {
	Correct    bool   `json:"correct"`
	Expression string `json:"expression"`
	Meaning    string `json:"meaning"`
}

// MultipleAnswerQuestionResponse represents multiple answer results
type MultipleAnswerQuestionResponse struct {
	Expression        string   `json:"expression"`
	IsExpressionInput bool     `json:"is_expression_input"`
	Meaning           string   `json:"meaning"`
	AnswersForContext []Answer `json:"answers"`
}

// Answer represents a single answer for a specific context
type Answer struct {
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
