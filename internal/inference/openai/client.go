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
- "answers": an array with one object per context: {"correct": boolean, "context": "<original context>", "reason": "<brief explanation>"}

STRICT OUTPUT: No text outside the JSON. Booleans are true/false lowercase. You MUST process ALL expressions provided in the input array.

UNDERSTANDING THE INPUT
- Each context may include a "reference_definition" field - this is ONLY a rough hint from a notebook and may be incomplete, incorrect, or empty.
- When context is PROVIDED: You must determine the true meaning from the context itself. The reference_definition is just a hint that may be wrong.
- When context is EMPTY/MISSING: You MUST rely on the reference_definition and user's meaning. If they match exactly or nearly exactly, mark CORRECT.
- DO NOT blindly trust the reference_definition when you have context. Always verify it matches actual usage.
- Each context may include a "usage" field showing the actual inflected form of the expression as it appears in that specific context.
- If usage is provided, use it to locate the expression in the context; if not provided, search for any inflected form.

**PRIORITY RULE: EXACT/NEAR-EXACT MATCH WITH REFERENCE DEFINITION**
If the user's meaning EXACTLY or NEARLY EXACTLY matches the reference_definition (even with minor wording differences), you should strongly consider marking it CORRECT, especially when:
- Context is empty/missing
- The reference_definition appears to be a standard/dictionary definition
- The user's meaning captures the same core concept as reference_definition
- Synonymous phrases that convey the same essential concept with different wording
This rule applies BEFORE other checks, but still reject if it's a fatal error (self-definition, opposite meaning, etc.)

EVALUATION RULES

**CRITICAL: CONSISTENCY ACROSS CONTEXTS**
For the SAME expression with the SAME user meaning, you MUST give the SAME correctness result across ALL contexts.
Do NOT mark one context as correct and another as incorrect if the user meaning and expression are identical.

**MANDATORY FIRST CHECK - SELF-DEFINITION**
Before ANY other evaluation, check: Does the user's meaning contain the expression itself (or a simple inflection)?
- If user meaning = expression (e.g., "kicky" → "kicky"), mark INCORRECT immediately
- If user meaning contains the expression as the main definition, mark INCORRECT
- This check MUST happen FIRST, before any semantic analysis

1) Determine TRUE meaning per context INDEPENDENTLY
   - Read the context carefully and determine the meaning YOURSELF.
   - CRITICAL: DO NOT rely on the reference_definition field when context is provided.
   - If context is empty, use reference_definition and general knowledge.
   - Treat the target expression as a UNIT (not word-by-word), accounting for idioms, phrasal verbs, or fixed phrases.
   - Be aware of slang meanings that differ from literal meanings (e.g., "go commando" = not wearing underwear, NOT related to military).
   - **CRITICAL NEGATION RULE**: When evaluating the user's meaning, you MUST focus ONLY on the EXPRESSION's inherent meaning, COMPLETELY IGNORING any negation in the context.
     * Negation words like "no", "not", "isn't", "never" appearing BEFORE the expression in context DO NOT change the expression's meaning
     * Example: "This is no walk in the park" - if user says the expression means "easy", that's CORRECT because "a walk in the park" DOES mean "easy"
     * The negation in the sentence doesn't change what the expression itself means - user defines the EXPRESSION, not the full sentence
     * ALWAYS evaluate the expression's positive/affirmative meaning, regardless of how it's used in context
     * If the user defines an expression correctly but the context negates it, mark CORRECT

2) Compare user's meaning to the actual meaning

   Determine if the user's meaning is CORRECT or INCORRECT:

   **Mark INCORRECT if:**
   - The user simply repeats the expression itself instead of defining it
   - The meaning is opposite or contradictory to the actual meaning
   - The meaning is completely unrelated or from a different semantic field
   - Key details or attributes are fundamentally wrong
   - **CRITICAL IS vs USES/DOES DISTINCTION**: The user confuses what something IS (its nature/identity) with what it DOES, USES, or is USED FOR
     * Example: If something IS a liquid, don't accept "something that takes liquid" (wrong category - container vs content)
     * Example: If something IS a procedure, don't accept the tool used in that procedure
     * The user must identify the correct category of thing
   - The meaning is in the wrong category (e.g., describing one type of thing when it's another)
   - **SUFFERING vs ENDURING**: Distinguish between passively receiving harm ("get damages", "receive beating") and actively withstanding it ("endure", "withstand")

   **Mark CORRECT if:**
   - The user's meaning matches or nearly matches the reference_definition
   - The meaning is semantically equivalent even with different wording
   - Synonyms are used that convey the same core concept
   - Only minor differences in intensifiers or word order exist
   - For multi-sense words, the user captured at least one valid sense
   - **The meaning captures the essential concept even if not perfectly worded or missing specific details from reference**
   - User's phrasing is different but the fundamental idea is the same

   **When uncertain, default to INCORRECT**

3) is_expression_input handling
   - If is_expression_input = true: the typed expression may contain typos; judge what the USER INTENDED.
   - If is_expression_input = false: treat the expression as correct/canonical.

4) Canonical meaning field
   - Set "meaning" to the best, short canonical gloss (≈3–8 words per sense).

OUTPUT FORMAT:
[
  {
    "expression": "…",
    "is_expression_input": false,
    "meaning": "…",
    "answers": [
      {"correct": true,  "context": "…", "reason": "user meaning matches: both mean X"},
      {"correct": false, "context": "…", "reason": "user said X but it means Y - unrelated concepts"}
    ]
  }
]

REASON FORMAT:
- For CORRECT: briefly explain why the meanings match (e.g., "exact match", "synonymous", "captures the core meaning")
- For INCORRECT: explain what's wrong in plain language (e.g., "user repeated the word", "opposite meaning", "unrelated concept", "it IS X, not something that does X")`

	// promptExample to demonstrate correct evaluation patterns
	type promptExample struct {
		description     string                    // What this example demonstrates
		userRequest     []inference.Expression    // The user's input
		assistantAnswer []inference.AnswerMeaning // The correct evaluation
	}

	examples := []promptExample{
		{
			description: "INCORRECT - Self-definition: user uses the same word to define itself",
			userRequest: []inference.Expression{
				{
					Expression: "snazzy",
					Meaning:    "snazzy",
					Contexts: []inference.Context{
						{Context: "He wore a snazzy new suit to the interview.", ReferenceDefinition: "stylish and attractive", Usage: "snazzy"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "snazzy",
					Meaning:    "stylish and attractive",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Context: "He wore a snazzy new suit to the interview.", Reason: "user repeated the word 'snazzy' instead of defining it"},
					},
				},
			},
		},
		{
			description: "INCORRECT - Opposite meaning: user says 'wear' but slang means 'not wearing'",
			userRequest: []inference.Expression{
				{
					Expression: "freeball",
					Meaning:    "to wear loose underwear",
					Contexts: []inference.Context{
						{Context: "He decided to freeball at the gym today.", ReferenceDefinition: "to not wear underwear", Usage: "freeball"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "freeball",
					Meaning:    "to not wear underwear",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Context: "He decided to freeball at the gym today.", Reason: "user said 'wear loose underwear' but the expression means the opposite - 'not wearing underwear'"},
					},
				},
			},
		},
		{
			description: "INCORRECT - Completely unrelated meaning from wrong semantic field (food vs personality)",
			userRequest: []inference.Expression{
				{
					Expression: "spunky",
					Meaning:    "salty",
					Contexts: []inference.Context{
						{Context: "The spunky little dog barked at the mailman.", ReferenceDefinition: "courageous and determined; spirited", Usage: "spunky"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "spunky",
					Meaning:    "courageous and spirited",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Context: "The spunky little dog barked at the mailman.", Reason: "user said 'salty' which is about food/taste, but the expression describes personality traits - completely different concepts"},
					},
				},
			},
		},
		{
			description: "INCORRECT - Wrong attribute: user says 'mountain' but it's 'river'",
			userRequest: []inference.Expression{
				{
					Expression: "the Thames",
					Meaning:    "a mountain in England",
					Contexts: []inference.Context{
						{Context: "We took a boat ride along the Thames.", ReferenceDefinition: "a major river flowing through London", Usage: "Thames"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "the Thames",
					Meaning:    "a major river flowing through London",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Context: "We took a boat ride along the Thames.", Reason: "user said 'mountain' but the Thames is a river - wrong geographic feature"},
					},
				},
			},
		},
		{
			description: "INCORRECT - IS vs USES error: user describes what it's used for, not what it IS",
			userRequest: []inference.Expression{
				{
					Expression: "saline",
					Meaning:    "a container for salt water",
					Contexts: []inference.Context{
						{Context: "The nurse injected saline into the IV.", ReferenceDefinition: "a solution of salt in water", Usage: "saline"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "saline",
					Meaning:    "a solution of salt in water",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Context: "The nurse injected saline into the IV.", Reason: "user said 'container' but saline IS the liquid solution itself, not the container - wrong category (container vs content)"},
					},
				},
			},
		},
		{
			description: "INCORRECT - Suffering vs Enduring: user says passive harm reception when it means active withstanding",
			userRequest: []inference.Expression{
				{
					Expression: "stand one's ground",
					Meaning:    "to get injured while staying in place",
					Contexts: []inference.Context{
						{Context: "Despite the criticism, she stood her ground on the issue.", ReferenceDefinition: "to maintain one's position; to refuse to retreat", Usage: "stood her ground"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "stand one's ground",
					Meaning:    "to maintain one's position; to refuse to retreat",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Context: "Despite the criticism, she stood her ground on the issue.", Reason: "user said 'get injured' which is passive suffering, but the expression means 'maintain position' which is actively resisting - different concepts"},
					},
				},
			},
		},
		{
			description: "CORRECT - Semantically identical with minor word difference",
			userRequest: []inference.Expression{
				{
					Expression: "simple",
					Meaning:    "easy",
					Contexts: []inference.Context{
						{Context: "This is a simple task.", ReferenceDefinition: "easily done or understood", Usage: "simple"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "simple",
					Meaning:    "easy; not complicated",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: true, Context: "This is a simple task.", Reason: "'easy' and 'simple' are synonymous - both convey the same core meaning"},
					},
				},
			},
		},
		{
			description: "CORRECT - Core concept captured with different phrasing",
			userRequest: []inference.Expression{
				{
					Expression: "hit the hay",
					Meaning:    "to start sleeping",
					Contexts: []inference.Context{
						{Context: "I'm exhausted. Time to hit the hay.", ReferenceDefinition: "to go to bed", Usage: "hit the hay"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "hit the hay",
					Meaning:    "to go to bed",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: true, Context: "I'm exhausted. Time to hit the hay.", Reason: "user said 'start sleeping' and reference says 'go to bed' - these capture the same essential concept"},
					},
				},
			},
		},
		{
			description: "CORRECT - Expression meaning remains same even when negated in context",
			userRequest: []inference.Expression{
				{
					Expression: "a piece of cake",
					Meaning:    "something easy to do",
					Contexts: []inference.Context{
						{Context: "This test is no piece of cake.", ReferenceDefinition: "something very easy to do", Usage: "piece of cake"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "a piece of cake",
					Meaning:    "something very easy to do",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: true, Context: "This test is no piece of cake.", Reason: "the expression itself means 'easy' - the negation in context doesn't change what the expression means"},
					},
				},
			},
		},
		{
			description: "CORRECT - Success concept matches even with different wording",
			userRequest: []inference.Expression{
				{
					Expression: "nail it",
					Meaning:    "to accomplish something perfectly",
					Contexts: []inference.Context{
						{Context: "You really nailed that presentation!", ReferenceDefinition: "to succeed at something; to do something very well", Usage: "nailed"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "nail it",
					Meaning:    "to succeed at something; to do something very well",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: true, Context: "You really nailed that presentation!", Reason: "user said 'accomplish perfectly' which captures the success concept - same essential meaning as 'succeed' and 'do very well'"},
					},
				},
			},
		},
		{
			description: "CORRECT - Empty context with matching reference definition",
			userRequest: []inference.Expression{
				{
					Expression: "cavalry",
					Meaning:    "soldiers on horseback",
					Contexts: []inference.Context{
						{Context: "", ReferenceDefinition: "soldiers who fight on horseback", Usage: "cavalry"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "cavalry",
					Meaning:    "soldiers who fight on horseback",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: true, Context: "", Reason: "user meaning matches the reference definition - both refer to soldiers on horseback"},
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
