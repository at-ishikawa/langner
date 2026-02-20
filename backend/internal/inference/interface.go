package inference

import (
	"context"
)

//go:generate mockgen -source=interface.go -destination=../mocks/inference/mock_client.go -package=mock_inference

// Client interface defines the methods for AI inference operations
type Client interface {
	AnswerMeanings(ctx context.Context, params AnswerMeaningsRequest) (AnswerMeaningsResponse, error)
	ValidateWordForm(ctx context.Context, params ValidateWordFormRequest) (ValidateWordFormResponse, error)
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

// ValidateWordFormRequest holds parameters for validating a word form in reverse quiz
type ValidateWordFormRequest struct {
	Expected   string `json:"expected"`    // The expected word
	UserAnswer string `json:"user_answer"` // The user's answer
	Meaning    string `json:"meaning"`     // The meaning that was shown to the user
	Context    string `json:"context"`     // Optional context sentence
}

// ValidateWordFormClassification represents the classification of a user's answer
type ValidateWordFormClassification string

const (
	// ClassificationSameWord means the answer is a different form of the expected word (e.g., "ran" for "run")
	ClassificationSameWord ValidateWordFormClassification = "same_word"
	// ClassificationSynonym means the answer is a valid synonym (e.g., "thrilled" for "excited")
	ClassificationSynonym ValidateWordFormClassification = "synonym"
	// ClassificationWrong means the answer doesn't match the meaning
	ClassificationWrong ValidateWordFormClassification = "wrong"
)

// ValidateWordFormResponse holds the result of word form validation
type ValidateWordFormResponse struct {
	Classification ValidateWordFormClassification `json:"classification"` // same_word, synonym, or wrong
	Reason         string                         `json:"reason"`         // Explanation of the classification
}
