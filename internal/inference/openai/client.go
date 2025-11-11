package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/avast/retry-go"
	"resty.dev/v3"
)

type Client struct {
	httpClient       *resty.Client
	model            string
	maxRetryAttempts uint
}

func NewClient(apiKey, model string, retryAttempts uint) *Client {
	client := resty.New()
	client.SetBaseURL("https://api.openai.com/v1")
	client.SetHeader("Authorization", "Bearer "+apiKey)
	client.SetHeader("Content-Type", "application/json")

	return &Client{
		httpClient:       client,
		model:            model,
		maxRetryAttempts: retryAttempts,
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

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Retry on JSON parsing errors as they might be due to incomplete responses
	errStr := err.Error()
	if strings.Contains(errStr, "json.Unmarshal") || strings.Contains(errStr, "unexpected end of JSON input") {
		return true
	}

	// Retry on network-related errors
	if strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "i/o timeout") {
		return true
	}

	// Retry on 5xx errors (server errors)
	if strings.Contains(errStr, "response error 5") {
		return true
	}

	// Retry on rate limiting (429)
	if strings.Contains(errStr, "response error 429") {
		return true
	}

	return false
}

// AnswerMeanings implements the inference.Client interface
func (client *Client) AnswerMeanings(
	ctx context.Context,
	params inference.AnswerMeaningsRequest,
) (inference.AnswerMeaningsResponse, error) {
	var result inference.AnswerMeaningsResponse
	if err := retry.Do(
		func() error {
			response, err := client.answerMeanings(ctx, params)
			if err != nil {
				if !isRetryableError(err) {
					return retry.Unrecoverable(err)
				}
				return err
			}
			result = response
			return nil
		},
		retry.Context(ctx),
		retry.Attempts(client.maxRetryAttempts+1),
		retry.DelayType(func(n uint, err error, config *retry.Config) time.Duration {
			return retry.BackOffDelay(n, err, config)
		}),
	); err != nil {
		return inference.AnswerMeaningsResponse{}, err
	}
	return result, nil
}

func (client *Client) getRequestBody(args inference.AnswerMeaningsRequest) (ChatCompletionRequest, error) {
	// Multiple expressions output format with per-expression is_expression_input
	systemPrompt := `You are an expert grader that judges whether a user's stated MEANING for an English expression is correct IN EACH GIVEN CONTEXT.

GOAL
Return ONLY a JSON array. For each input expression, include:
- "expression": the expression as provided
- "is_expression_input": boolean from input
- "meaning": the CANONICAL meaning (lemma/inflection normalized) that best fits the expression across its contexts (not the user's meaning)
- "answers": an array with one object per context: {"correct": boolean, "context": "<original context>"}

STRICT OUTPUT: No text outside the JSON. Booleans are true/false lowercase.

UNDERSTANDING THE INPUT
- Each context may include a "usage" field showing the actual inflected form of the expression as it appears in that specific context.
- Example: For expression "run", the usage might be "ran", "running", or "runs" depending on how it appears in the context sentence.
- The usage field helps you identify which word form to focus on, but you should still normalize inflections when comparing meanings.
- If usage is provided, use it to locate the expression in the context; if not provided, search for any inflected form.

EVALUATION RULES
1) Determine TRUE meaning per context
   - Read the context carefully.
   - Treat the target expression as a UNIT (not word-by-word), accounting for idiom, phrasal verb, or fixed phrase.
   - If a "usage" field is provided, it indicates the specific inflected form in that context (e.g., "ran" for "run").
   - Normalize inflection/tense/number (e.g., "broke" ↔ "break", "runs" ↔ "run") and ignore punctuation/markup (e.g., {{…}}).
   - Identify the sense and part-of-speech actually used (e.g., "run" = operate/manage vs move quickly).
   - If multiple senses are possible, choose the one most supported by the context; if still ambiguous, choose the most common idiomatic reading for that context.

2) Compare to the user's meaning
   - Consider the user's MEANING string as the claim being graded.
   - Accept paraphrases ONLY if semantically equivalent to the TRUE meaning (core concept must match).
   - Minor grammar/spelling doesn’t matter if the meaning is the same.
   - Mark INCORRECT if the user’s meaning is:
     • opposite or unrelated,
     • a different sense of the same word,
     • only partially correct or too vague,
     • meaning of a different phrase in the sentence.
   - Be conservative: if in doubt, mark incorrect.

3) is_expression_input handling
   - If is_expression_input = true: the typed expression may contain typos; judge what the USER INTENDED to say by that expression in the given context. Ignore typos in the expression itself; still grade the user’s MEANING strictly.
   - If is_expression_input = false: treat the expression as correct/canonical and just grade the user’s MEANING.

4) Canonical meaning field
   - Set "meaning" to the best, short canonical gloss that fits the expression across its contexts (e.g., for “run” with both senses present, use a concise multi-sense gloss like "to move quickly by foot; to operate/manage", otherwise a single-sense gloss).
   - Keep it short (≈3–8 words per sense). Use semicolons to separate multiple senses if needed.

DECISION CHECKLIST (apply before output):
- Did I identify the correct sense for THIS expression in THIS context?
- Is the user’s meaning truly equivalent (not just similar words)?
- Are all booleans present and lowercase?
- Is the top-level array valid JSON with the required fields only?

OUTPUT FORMAT (example skeleton):
[
  {
    "expression": "…",
    "is_expression_input": false,
    "meaning": "…",
    "answers": [
      {"correct": true,  "context": "…"},
      {"correct": false, "context": "…"}
    ]
  }
]`

	// promptExample to demonstrate correct evaluation patterns
	type promptExample struct {
		description     string                    // What this example demonstrates
		userRequest     []inference.Expression    // The user's input
		assistantAnswer []inference.AnswerMeaning // The correct evaluation
	}

	examples := []promptExample{
		{
			description: "MIXED - Same expression, different contexts with different meanings. " +
				"Tests: evaluating multiple contexts separately, detecting when meaning only applies to some contexts, using usage field",
			userRequest: []inference.Expression{
				{
					Expression: "run",
					Meaning:    "to move fast",
					Contexts: []inference.Context{
						{Context: "I need to run to the store before it closes.", Meaning: "to move quickly", Usage: "run"},
						{Context: "She runs a successful startup company.", Meaning: "to operate or manage", Usage: "runs"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "run",
					Meaning:    "to move quickly",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: true, Context: "I need to run to the store before it closes."},
						{Correct: false, Context: "She runs a successful startup company."},
					},
				},
			},
		},
		{
			description: "INCORRECT - User's meaning is opposite of actual meaning. " +
				"Tests: rejecting opposite meanings even when hint shows the correct answer",
			userRequest: []inference.Expression{
				{
					Expression: "piece of cake",
					Meaning:    "very difficult",
					Contexts: []inference.Context{
						{Context: "Don't worry, the test will be a piece of cake.", Meaning: "very easy"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "piece of cake",
					Meaning:    "very easy",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Context: "Don't worry, the test will be a piece of cake."},
					},
				},
			},
		},
		{
			description: "INCORRECT - User took literal meaning instead of idiomatic. " +
				"Tests: rejecting literal interpretation of idiomatic expressions, usage field with phrasal expression",
			userRequest: []inference.Expression{
				{
					Expression: "make a scene",
					Meaning:    "to create something",
					Contexts: []inference.Context{
						{Context: "Please don't make a scene in front of everyone at the restaurant.", Meaning: "to cause a public disturbance", Usage: "make a scene"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "make a scene",
					Meaning:    "to cause a public disturbance or display of emotion",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Context: "Please don't make a scene in front of everyone at the restaurant."},
					},
				},
			},
		},
	}

	// Build messages: system prompt + few-shot examples + actual request
	messages := []Message{
		{
			Role:    RoleSystem,
			Content: systemPrompt,
		},
	}

	// Add few-shot examples
	for _, example := range examples {
		// Marshal user request to JSON
		userJSON, err := json.Marshal(example.userRequest)
		if err != nil {
			return ChatCompletionRequest{}, fmt.Errorf("failed to marshal example user request: %w", err)
		}

		// Marshal assistant answer to JSON
		assistantJSON, err := json.Marshal(example.assistantAnswer)
		if err != nil {
			return ChatCompletionRequest{}, fmt.Errorf("failed to marshal example assistant answer: %w", err)
		}

		messages = append(messages,
			Message{
				Role:    RoleUser,
				Content: string(userJSON),
			},
			Message{
				Role:    RoleAssistant,
				Content: string(assistantJSON),
			},
		)
	}

	// Add actual user request
	// Build user message with all expressions

	// TOOD: With a meaning of a context, it doesn't work well.
	for i := range args.Expressions {
		for j := range args.Expressions[i].Contexts {
			args.Expressions[i].Contexts[j].Meaning = ""
		}
	}
	userContent := bytes.NewBuffer(nil)
	if err := json.NewEncoder(userContent).Encode(args.Expressions); err != nil {
		return ChatCompletionRequest{}, fmt.Errorf("failed to marshal expressions: %w", err)
	}
	messages = append(messages, Message{
		Role:    RoleUser,
		Content: userContent.String(),
	})

	body := ChatCompletionRequest{
		Model:       client.model,
		Temperature: 0.3,
		Messages:    messages,
	}

	return body, nil
}

// answerMeanings validates multiple expressions, each with multiple contexts
func (client *Client) answerMeanings(
	ctx context.Context,
	args inference.AnswerMeaningsRequest,
) (inference.AnswerMeaningsResponse, error) {
	if len(args.Expressions) == 0 {
		return inference.AnswerMeaningsResponse{}, nil
	}

	requestBody, err := client.getRequestBody(args)
	if err != nil {
		return inference.AnswerMeaningsResponse{}, fmt.Errorf("getRequestBody > %w", err)
	}

	response, err := client.httpClient.R().
		SetContext(ctx).
		SetBody(requestBody).
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
	slog.Default().Debug("openai response content",
		"request", requestBody,
		"response", responseBody,
	)

	var decoded []inference.AnswerMeaning
	if err := json.NewDecoder(strings.NewReader(content)).Decode(&decoded); err != nil {
		slog.Default().Error("Failed to parse OpenAI response as JSON",
			"request", requestBody,
			"expressionCount", len(args.Expressions),
			"error", err)
		return inference.AnswerMeaningsResponse{}, fmt.Errorf("json.Unmarshal(%s) > %w", content, err)
	}
	return inference.AnswerMeaningsResponse{Answers: decoded}, nil
}
