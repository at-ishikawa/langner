package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/at-ishikawa/langner/internal/inference"
	"resty.dev/v3"
)

type Client struct {
	httpClient  *resty.Client
	model       string
	retryConfig inference.RetryConfig
}

func NewClient(apiKey, model string, retryConfig inference.RetryConfig) *Client {
	client := resty.New()
	client.SetBaseURL("https://api.openai.com/v1")
	client.SetHeader("Authorization", "Bearer "+apiKey)
	client.SetHeader("Content-Type", "application/json")

	return &Client{
		httpClient:  client,
		model:       model,
		retryConfig: retryConfig,
	}
}

func (client Client) Close() error {
	return client.httpClient.Close()
}

// GetModel returns the model name configured for this client
func (client Client) GetModel() string {
	return client.model
}

type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float32   `json:"temperature,omitempty"`
}

type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int           `json:"index"`
	Message      ChoiceMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type ChoiceMessage struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// getCommonEvaluationRules returns the shared evaluation rules for context validation
func getCommonEvaluationRules() string {
	return `You are an expert in evaluating whether a user's understanding of an English expression is correct.

EVALUATION RULES:
1. SEMANTIC EQUIVALENCE - Accept synonyms and paraphrases:
   - "trigger" = "activate" = "cause" = "initiate" = "start" = "set off"
   - "to reject" = "to refuse" = "to decline"
   - Accept different wording if the core meaning is the same

2. Mark as CORRECT if:
   - The meaning is semantically equivalent (uses synonyms or paraphrases)
   - The core concept is captured correctly
   - Minor spelling/grammar issues don't change the meaning

3. Mark as INCORRECT if:
   - The meaning is opposite (e.g., "start" when it should be "stop")
   - The meaning is completely unrelated
   - The meaning describes a different sense of the word`
}

// answerMeanings validates multiple expressions, each with multiple contexts
func (client *Client) answerMeanings(
	ctx context.Context,
	args inference.AnswerMeaningsRequest,
) (inference.AnswerMeaningsResponse, error) {
	if len(args.Expressions) == 0 {
		return inference.AnswerMeaningsResponse{}, nil
	}

	// Use shared prompt components
	commonRules := getCommonEvaluationRules()

	// Multiple expressions output format with per-expression is_expression_input
	systemPrompt := commonRules + `

MULTIPLE EXPRESSIONS:
You will be given multiple expressions, each with multiple contexts.
Each expression has an "is_expression_input" field that indicates the input mode:
- If "is_expression_input" is true: The expression is from user input (may contain typos). The meaning is what the user typed. Focus on: Does the meaning match what the user INTENDED to express? Ignore typos in the expression.
- If "is_expression_input" is false: The expression is correct (from our database). The meaning is what the user typed (may contain typos or paraphrases). Evaluate if the user's meaning matches the expression's actual meaning.

Evaluate each expression's meaning against its contexts separately based on its input mode.

OUTPUT FORMAT:
You MUST respond with ONLY a valid JSON array containing one object per expression:
[
  {
    "expression": "expression1",
    "is_expression_input": boolean,
    "meaning": "correct meaning",
    "answers": [
      {"correct": boolean, "context": "context string"}
    ]
  },
  {
    "expression": "expression2",
    "is_expression_input": boolean,
    "meaning": "correct meaning",
    "answers": [
      {"correct": boolean, "context": "context string"}
    ]
  }
]

No explanations, no additional text before or after the JSON array.`

	// Build user message with all expressions
	userContent := bytes.NewBuffer(nil)
	if err := json.NewEncoder(userContent).Encode(args.Expressions); err != nil {
		return inference.AnswerMeaningsResponse{}, fmt.Errorf("failed to marshal expressions: %w", err)
	}

	body := ChatCompletionRequest{
		Model:       client.model,
		Temperature: 0.3,
		Messages: []Message{
			{
				Role:    RoleSystem,
				Content: systemPrompt,
			},
			{
				Role:    RoleUser,
				Content: userContent.String(),
			},
		},
	}

	response, err := client.httpClient.R().
		SetContext(ctx).
		SetBody(body).
		SetResult(&ChatCompletionResponse{}).
		Post("/chat/completions")
	if err != nil {
		return inference.AnswerMeaningsResponse{}, fmt.Errorf("httpClient.Post > %w", err)
	}
	if response.IsError() {
		return inference.AnswerMeaningsResponse{}, fmt.Errorf("response error %d: %s", response.StatusCode(), response.String())
	}

	responseBody := response.Result().(*ChatCompletionResponse)
	if responseBody == nil || len(responseBody.Choices) == 0 {
		return inference.AnswerMeaningsResponse{}, fmt.Errorf("empty response body or choices: %s", response.String())
	}

	content := responseBody.Choices[0].Message.Content
	if content == "" {
		return inference.AnswerMeaningsResponse{}, fmt.Errorf("empty response content: %s", response.String())
	}
	slog.Default().Debug("openai response content", "content", content)

	var decoded []inference.AnswerMeaning
	if err := json.NewDecoder(strings.NewReader(content)).Decode(&decoded); err != nil {
		slog.Default().Error("Failed to parse OpenAI response as JSON",
			"request", body,
			"prompt", content,
			"userRequest", userContent.String(),
			"expressionCount", len(args.Expressions),
			"error", err)
		return inference.AnswerMeaningsResponse{}, fmt.Errorf("json.Unmarshal(%s) > %w", content, err)
	}
	return inference.AnswerMeaningsResponse{Answers: decoded}, nil
}
