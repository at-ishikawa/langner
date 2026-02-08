package inference

import (
	"context"
)

//go:generate mockgen -source=interface.go -destination=../mocks/inference/mock_client.go -package=mock_inference

// Client interface defines the methods for AI inference operations
type Client interface {
	AnswerMeanings(ctx context.Context, params AnswerMeaningsRequest) (AnswerMeaningsResponse, error)
}

// Expression represents a single expression with its contexts
type Expression struct {
	Expression        string    `json:"expression"`
	Meaning           string    `json:"meaning"`
	Contexts          []Context `json:"contexts,omitempty"`
	IsExpressionInput bool      `json:"is_expression_input"`
	ResponseTimeMs    int64     `json:"response_time_ms,omitempty"` // For quality assessment
}

// Context represents a single example of an expression with its meaning from a registered notebook
type Context struct {
	Context string `json:"context"`
	Usage   string `json:"usage,omitempty"` // Optional: actual form of expression used in context (e.g., "ran" for "run")

	// Unused for now. TODO: Improve the prompt to use this.
	ReferenceDefinition string `json:"reference_definition,omitempty"` // Optional: hint/reference meaning from notebook, may be incomplete or incorrect
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
	Reason  string `json:"reason"`  // Explanation of why the answer is correct or incorrect
	Quality int    `json:"quality"` // 1-5 based on correctness + response time
}

const (
	DefaultMaxRetryAttempts = 3
)
