package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

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

// Types are now defined in the inference package

func (client Client) createUserMessage(expression, meaning, context string) Message {
	if context == "" {
		return Message{
			Role:    RoleUser,
			Content: fmt.Sprintf(`{"expression": "%s", "meaning": "%s"}`, expression, meaning),
		}
	}
	return Message{
		Role:    RoleUser,
		Content: fmt.Sprintf(`{"expression": "%s", "meaning": "%s", "context": "%s"}`, expression, meaning, context),
	}
}

func (client Client) createAssistantMessage(expression string, isCorrect bool, meaning string) Message {
	return Message{
		Role:    RoleAssistant,
		Content: fmt.Sprintf(`{"expression": "%s", "correct": %t, "meaning": "%s"}`, expression, isCorrect, meaning),
	}
}

// getCommonEvaluationRules returns the shared evaluation rules for both single and multiple context validation
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

// getUserInputPrompt returns the prompt describing what fields are user input
func getUserInputPrompt(isExpressionInput bool) string {
	if isExpressionInput {
		return `
USER INPUT:
- The expression is what the user typed (may contain typos or misspellings)
- The meaning is what the user typed
- Focus on: Does the meaning match what the user INTENDED to express?
- IMPORTANT: If the expression has a typo (e.g., "set of" instead of "set off"), check if the meaning matches the INTENDED expression based on context
- Ignore typos in the expression when evaluating correctness`
	}
	return `
USER INPUT:
- The expression is correct (from our database)
- The meaning is what the user typed (may contain typos or paraphrases)
- Evaluate if the user's meaning matches the expression's actual meaning`
}

func (client *Client) answerExpressionWithSingleContext(
	ctx context.Context,
	expression string,
	meaning string,
	statements string,
	isExpressionInput bool,
) (inference.AnswerQuestionResponse, error) {
	var result inference.AnswerQuestionResponse

	// Use shared prompt components
	commonRules := getCommonEvaluationRules()
	userInputPrompt := getUserInputPrompt(isExpressionInput)

	// Single-context specific output format
	var outputFormat string
	if isExpressionInput {
		outputFormat = `
OUTPUT FORMAT:
You MUST respond with ONLY a valid JSON object containing exactly three fields:
- "expression": the exact expression from user input (unchanged, even if it has typos)
- "correct": boolean indicating if the user's meaning is correct
- "meaning": the correct meaning based on context (if provided) or most common meaning (if no context)

No explanations, no additional text before or after the JSON.`
	} else {
		outputFormat = `
OUTPUT FORMAT:
You MUST respond with ONLY a valid JSON object containing exactly three fields:
- "expression": the exact expression provided (this is always correct)
- "correct": boolean indicating if the user's meaning matches the contextual usage
- "meaning": the correct meaning based on context (if provided) or most common meaning (if no context)

No explanations, no additional text before or after the JSON.`
	}

	systemPrompt := commonRules + userInputPrompt + outputFormat

	// Build example messages based on mode
	var exampleMessages []Message
	if isExpressionInput {
		// FreeformQuiz examples: expression may have typos
		exampleMessages = []Message{
			// Example 1: Expression with typo, but meaning is correct
			client.createUserMessage("set of", "to trigger or activate something", "you're going to set of my thorns"),
			client.createAssistantMessage("set of", true, "to trigger or activate something (likely meant 'set off')"),

			// Example 2: Semantic equivalence - different wording, same meaning
			client.createUserMessage("run into", "to meet someone by chance", "I ran into an old friend at the store"),
			client.createAssistantMessage("run into", true, "to encounter someone unexpectedly"),

			// Example 3: Wrong meaning
			client.createUserMessage("chat", "person to talk", "I'm going to chat with my friends"),
			client.createAssistantMessage("chat", false, "to have an informal conversation"),
		}
	} else {
		// NotebookQuiz examples: expression is always correct
		exampleMessages = []Message{
			// Example 1: Wrong meaning, context shows it's different
			client.createUserMessage("chat", "person to talk", "I'm going to chat with my friends"),
			client.createAssistantMessage("chat", false, "to have an informal conversation"),

			// Example 2: Semantic equivalence - different wording, same meaning
			client.createUserMessage("run into", "to meet someone by chance", "I ran into an old friend at the store"),
			client.createAssistantMessage("run into", true, "to encounter someone unexpectedly"),

			// Example 3: Context determines meaning (phrasal verb with multiple meanings)
			client.createUserMessage("turn down", "to reject or refuse", "She turned down the job offer"),
			client.createAssistantMessage("turn down", true, "to decline or refuse something"),
		}
	}

	messages := []Message{
		{
			Role:    RoleSystem,
			Content: systemPrompt,
		},
	}
	messages = append(messages, exampleMessages...)
	messages = append(messages, client.createUserMessage(expression, meaning, statements))

	body := ChatCompletionRequest{
		Model:       client.model,
		Temperature: 0.3, // Lower temperature for more consistent responses
		Messages:    messages,
	}

	response, err := client.httpClient.R().
		SetContext(ctx).
		SetBody(body).
		SetResult(&ChatCompletionResponse{}).
		Post("/chat/completions")
	if err != nil {
		return result, fmt.Errorf("httpClient.Post > %w", err)
	}
	if response.IsError() {
		return result, fmt.Errorf("response error %d: %s", response.StatusCode(), response.String())
	}

	responseBody := response.Result().(*ChatCompletionResponse)
	if responseBody == nil || len(responseBody.Choices) == 0 {
		return result, fmt.Errorf("empty response body or choices: %s", response.String())
	}

	content := responseBody.Choices[0].Message.Content
	if content == "" {
		return result, fmt.Errorf("empty response content: %s", response.String())
	}
	slog.Default().Debug("openai response content", "content", content)

	var unmarshaled inference.AnswerQuestionResponse
	if err := json.Unmarshal([]byte(content), &unmarshaled); err != nil {
		// Try to parse with partial JSON fix
		fixed, fixErr := tryParsePartialJSON(content)
		if fixErr != nil {
			slog.Default().Error("Failed to parse OpenAI response as JSON",
				"content", content,
				"expression", expression,
				"error", err,
				"fixError", fixErr)
			return result, fmt.Errorf("json.Unmarshal(%s) > %w (also failed with fix: %v)", content, err, fixErr)
		}
		slog.Default().Warn("Successfully parsed after fixing JSON format",
			"original", content,
			"fixed", fixed,
			"expression", expression)
		return *fixed, nil
	}

	return unmarshaled, nil
}

// answerExpressionWithMultipleContexts validates a user's meaning against multiple context groups
func (client *Client) answerExpressionWithMultipleContexts(
	ctx context.Context,
	expression string,
	meaning string,
	contexts [][]string,
	isExpressionInput bool,
) (inference.MultipleAnswerQuestionResponse, error) {
	var result inference.MultipleAnswerQuestionResponse

	// Use shared prompt components
	commonRules := getCommonEvaluationRules()
	userInputPrompt := getUserInputPrompt(isExpressionInput)

	// Multiple-context specific output format
	outputFormat := `
MULTIPLE CONTEXTS:
You will be given multiple contexts, each representing a different usage/occurrence of the expression.
Evaluate the user's meaning against EACH context separately.

OUTPUT FORMAT:
You MUST respond with ONLY a valid JSON object with these fields:
- "expression": the exact expression provided
- "is_expression_input": boolean indicating if expression is from user input
- "meaning": the correct meaning based on the contexts
- "answers": array of objects, one per context, each with:
  - "correct": boolean indicating if user's meaning matches this context
  - "context": the context string

Example:
{"expression": "run", "is_expression_input": true, "meaning": "to move quickly", "answers": [{"correct": true, "context": "I run every morning"}, {"correct": false, "context": "I run a business"}]}

No explanations, no additional text before or after the JSON.`

	systemPrompt := commonRules + userInputPrompt + outputFormat

	// Build user message with all contexts flattened
	userContent := fmt.Sprintf(`{"expression": "%s", "meaning": "%s"`, expression, meaning)
	if len(contexts) > 0 {
		userContent += `, "contexts": [`
		first := true
		for _, contextGroup := range contexts {
			for _, ctx := range contextGroup {
				if !first {
					userContent += `, `
				}
				first = false
				userContent += fmt.Sprintf(`"%s"`, ctx)
			}
		}
		userContent += `]`
	}
	userContent += `}`

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
				Content: userContent,
			},
		},
	}

	response, err := client.httpClient.R().
		SetContext(ctx).
		SetBody(body).
		SetResult(&ChatCompletionResponse{}).
		Post("/chat/completions")
	if err != nil {
		return result, fmt.Errorf("httpClient.Post > %w", err)
	}
	if response.IsError() {
		return result, fmt.Errorf("response error %d: %s", response.StatusCode(), response.String())
	}

	responseBody := response.Result().(*ChatCompletionResponse)
	if responseBody == nil || len(responseBody.Choices) == 0 {
		return result, fmt.Errorf("empty response body or choices: %s", response.String())
	}

	content := responseBody.Choices[0].Message.Content
	if content == "" {
		return result, fmt.Errorf("empty response content: %s", response.String())
	}
	slog.Default().Debug("openai response content", "content", content)

	var unmarshaled inference.MultipleAnswerQuestionResponse
	if err := json.Unmarshal([]byte(content), &unmarshaled); err != nil {
		slog.Default().Error("Failed to parse OpenAI response as JSON",
			"content", content,
			"expression", expression,
			"error", err)
		return result, fmt.Errorf("json.Unmarshal(%s) > %w", content, err)
	}

	// Ensure IsExpressionInput is set correctly from the input parameter
	// rather than relying on OpenAI to return it accurately
	unmarshaled.IsExpressionInput = isExpressionInput

	return unmarshaled, nil
}

// GetWordMeaning uses OpenAI to get the meaning of a word
// This is an alternative to using RapidAPI dictionary
func (client *Client) GetWordMeaning(ctx context.Context, word string) (string, error) {
	body := ChatCompletionRequest{
		Model:       client.model,
		Temperature: 0.3,
		Messages: []Message{
			{
				Role:    RoleSystem,
				Content: "You are an expert English dictionary. Provide a clear, concise definition for the given word. Include the part of speech and primary meaning. Keep the response under 100 words.",
			},
			{
				Role:    RoleUser,
				Content: fmt.Sprintf("Define the word: %s", word),
			},
		},
	}

	response, err := client.httpClient.R().
		SetContext(ctx).
		SetBody(body).
		SetResult(&ChatCompletionResponse{}).
		Post("/chat/completions")
	if err != nil {
		return "", fmt.Errorf("httpClient.Post > %w", err)
	}
	if response.IsError() {
		return "", fmt.Errorf("response error %d: %s", response.StatusCode(), response.String())
	}

	responseBody := response.Result().(*ChatCompletionResponse)
	if responseBody == nil || len(responseBody.Choices) == 0 {
		return "", fmt.Errorf("empty response body or choices")
	}

	content := responseBody.Choices[0].Message.Content
	if content == "" {
		return "", fmt.Errorf("empty response content")
	}

	return content, nil
}

// GetDictionaryEntry uses OpenAI to get structured dictionary information
func (client *Client) GetDictionaryEntry(ctx context.Context, request ChatCompletionRequest) (string, error) {
	response, err := client.httpClient.R().
		SetContext(ctx).
		SetBody(request).
		SetResult(&ChatCompletionResponse{}).
		Post("/chat/completions")
	if err != nil {
		return "", fmt.Errorf("httpClient.Post > %w", err)
	}
	if response.IsError() {
		return "", fmt.Errorf("response error %d: %s", response.StatusCode(), response.String())
	}

	responseBody := response.Result().(*ChatCompletionResponse)
	if responseBody == nil || len(responseBody.Choices) == 0 {
		return "", fmt.Errorf("empty response body or choices")
	}

	content := responseBody.Choices[0].Message.Content
	if content == "" {
		return "", fmt.Errorf("empty response content")
	}

	return content, nil
}
