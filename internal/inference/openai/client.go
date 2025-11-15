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
- Each context may include a "reference_definition" field - this is ONLY a rough hint from a notebook and may be incomplete, incorrect, or empty.
- DO NOT blindly trust or compare against the reference_definition. You must independently determine the true meaning from the context itself.
- The reference_definition is provided only as a potential starting point for your analysis - always verify it matches the actual usage in context.
- Each context may include a "usage" field showing the actual inflected form of the expression as it appears in that specific context.
- Example: For expression "run", the usage might be "ran", "running", or "runs" depending on how it appears in the context sentence.
- The usage field helps you identify which word form to focus on, but you should still normalize inflections when comparing meanings.
- If usage is provided, use it to locate the expression in the context; if not provided, search for any inflected form.

EVALUATION RULES
1) Determine TRUE meaning per context INDEPENDENTLY
   - Read the context carefully and determine the meaning YOURSELF by analyzing how the expression is actually used in the sentence.
   - CRITICAL: DO NOT rely on the reference_definition field. It is often WRONG or misleading. Ignore it completely when determining the true meaning.
   - The reference_definition may give a literal/dictionary meaning when the context uses a metaphorical/idiomatic sense, or vice versa.
   - You must determine the meaning ONLY from the context itself, as if the reference_definition didn't exist.
   - Treat the target expression as a UNIT (not word-by-word), accounting for idioms, phrasal verbs, or fixed phrases.
   - If a "usage" field is provided, it indicates the specific inflected form in that context (e.g., "ran" for "run").
   - Normalize inflection/tense/number (e.g., "broke" ↔ "break", "runs" ↔ "run") and ignore punctuation/markup.
   - Identify the sense and part-of-speech actually used (e.g., "run" = operate/manage vs move quickly).
   - Be aware of metaphorical, idiomatic, and figurative uses (e.g., "disturbance in the wind" may mean "sensing danger" not "weather phenomenon").
   - Consider collocations and common phrases (e.g., "for the cause" means "for a principle/ideal", not the literal "cause and effect").
   - If multiple senses are possible, choose the one most supported by the context; if still ambiguous, choose the most common idiomatic reading for that context.

2) Compare to the user's meaning - BE STRICT
   - STEP 1: Write down the TRUE meaning you determined from context (let's call this "TRUE_MEANING")
   - STEP 2: Write down the user's provided meaning (let's call this "USER_MEANING")
   - STEP 3: Compare these TWO meanings WORD BY WORD:
     • Does USER_MEANING contain ALL the key concepts from TRUE_MEANING?
     • Does USER_MEANING add any concepts NOT in TRUE_MEANING?
     • Are they 100% semantically equivalent and interchangeable?
   - STEP 4: If ANY answer above is "no", mark INCORRECT
   - DEFAULT TO INCORRECT: Only mark CORRECT if you are absolutely certain the user's meaning is semantically equivalent to the TRUE meaning.
   - Mark CORRECT only in these specific cases:
     • The user's wording is a direct paraphrase expressing the EXACT same concept (e.g., "cease trying" = "stop trying", "depart quickly" = "leave in a hurry")
     • The user uses a perfect synonym that could replace the expression in context (e.g., "wealthy" = "rich", "begin" = "start")
     • The ONLY differences are grammar/spelling, not meaning
   - Mark INCORRECT in ALL other cases, including:
     • Related but distinct concepts, even if they seem similar (e.g., "miserable" ≠ "unpleasant", "annoying" ≠ "mysterious", "tasty" ≠ "spicy")
     • Simplified definitions that lose important nuance (e.g., "coating" ≠ "protective layer formed by reaction", "place" ≠ "specific location")
     • Over-generalizations (e.g., "song or place" when it's specifically "a particular song title")
     • Opposite or contradictory meanings
     • Different senses of polysemous words (e.g., "bank" as financial institution vs river edge)
     • Partially correct but incomplete (e.g., "complete" when full meaning is "plan, organize, and complete")
     • Missing critical attributes (e.g., "continuous sound" vs just "sound")
     • Wrong attributes added (e.g., "pleasant sound" when there's no pleasantness implied)
     • Confusing similar-sounding words (e.g., "accept" ≠ "except", user might be thinking of wrong word)
     • Getting idiom meaning wrong (e.g., "hit the road" = leave/depart, not "strike the pavement")
     • Defining only part of a multi-word expression
     • Vague or unclear descriptions
   - CRITICAL STRICTNESS RULES - FOLLOW THESE EXACTLY:
     • WORD-BY-WORD CHECK: If user's meaning contains even ONE word that doesn't match the true meaning, immediately mark INCORRECT. For example:
       - "sad and annoying" vs "sad and mysterious" → INCORRECT (annoying ≠ mysterious)
       - "dark and cheerful" vs "dark and gloomy" → INCORRECT (cheerful ≠ gloomy)
     • SIMPLIFICATION CHECK: If user removed ANY important detail from the definition, mark INCORRECT:
       - "loud noise" vs "sudden explosive sound" → INCORRECT (missing "sudden", missing "explosive")
      - "powder" vs "fine substance that creates powder" → INCORRECT (oversimplified, missing process)
       - "red coating" vs "oxidized iron compound" → INCORRECT (missing chemical process)
     • PROCESS/MECHANISM CHECK: If true meaning involves a process or mechanism (like "produces", "causes", "creates"), user must include it:
       - User says "growth" but meaning is "organism that produces growth" → INCORRECT
       - User says "container" but meaning is "vessel that holds liquid" → INCORRECT
     • PARTIAL CORRECTNESS = INCORRECT: If user only captures part of a multi-part definition, mark INCORRECT:
       - "complete" vs "plan, organize, and complete" → INCORRECT (missing "plan" and "organize")
      - "accomplish" vs "prepare, execute, and accomplish" → INCORRECT (only final step, missing earlier steps)
       - "finish" vs "design, build, and finish" → INCORRECT (missing earlier steps)
     • SAME SEMANTIC FIELD ≠ CORRECT: Even if meanings are related or in same category, if not equivalent, mark INCORRECT:
       - "annoying" and "mysterious" are both negative descriptors but DIFFERENT → INCORRECT
       - "miserable" and "unpleasant" are both negative feelings but DIFFERENT → INCORRECT
     • THE RESULT IS NOT THE THING: If true meaning is "X that produces/creates Y", user saying just "Y" is INCORRECT:
       - User: "heat" vs True: "device that generates heat" → INCORRECT
   - CRITICAL: Your job is to CATCH MISTAKES, not to accept similar answers. Be a STRICT grader.
   - FINAL CHECK: Before marking CORRECT, ask yourself EXPLICITLY:
     1. "Did I determine the TRUE_MEANING from context?"
     2. "Does USER_MEANING contain EVERY key concept from TRUE_MEANING?"
     3. "Are these definitions 100% interchangeable in a dictionary?"
     4. If ANY answer is "no" or "unsure", mark INCORRECT.

3) is_expression_input handling
   - If is_expression_input = true: the typed expression may contain typos; judge what the USER INTENDED to say by that expression in the given context. Ignore typos in the expression itself; still grade the user's MEANING against the actual meaning.
   - If is_expression_input = false: treat the expression as correct/canonical and just grade the user's MEANING.

4) Canonical meaning field
   - Set "meaning" to the best, short canonical gloss that fits the expression across its contexts (e.g., for "run" with both senses present, use a concise multi-sense gloss like "to move quickly by foot; to operate/manage", otherwise a single-sense gloss).
   - Keep it short (≈3–8 words per sense). Use semicolons to separate multiple senses if needed.

DECISION CHECKLIST (apply before output):
- Did I identify the correct sense for THIS expression in THIS context by reading the context itself?
- Does the user's meaning capture the same core concept, even if worded differently?
- Am I being fair in accepting paraphrases and synonyms that demonstrate understanding?
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
			description: "INCORRECT - User has one wrong word in compound meaning. " +
				"Tests: rejecting when ANY word in user's meaning doesn't match",
			userRequest: []inference.Expression{
				{
					Expression: "gloomy",
					Meaning:    "dark and depressing",
					Contexts: []inference.Context{
						{Context: "The gloomy forest felt mysterious and foreboding as we entered.", ReferenceDefinition: "dark and sad", Usage: "gloomy"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "gloomy",
					Meaning:    "dark and threatening; mysterious",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Context: "The gloomy forest felt mysterious and foreboding as we entered."},
					},
				},
			},
		},
		{
			description: "INCORRECT - User over-simplified, losing critical detail. " +
				"Tests: rejecting simplified definitions that remove important aspects",
			userRequest: []inference.Expression{
				{
					Expression: "rust",
					Meaning:    "orange coating",
					Contexts: []inference.Context{
						{Context: "The old car had rust all over its body.", ReferenceDefinition: "iron oxide coating formed by corrosion", Usage: "rust"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "rust",
					Meaning:    "reddish-brown coating formed by oxidation of iron",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Context: "The old car had rust all over its body."},
					},
				},
			},
		},
		{
			description: "INCORRECT - User described result instead of the organism/process that produces it. " +
				"Tests: rejecting when definition says 'X that produces Y' but user only says 'Y'",
			userRequest: []inference.Expression{
				{
					Expression: "bacteria",
					Meaning:    "germs",
					Contexts: []inference.Context{
						{Context: "Wash your hands to remove bacteria.", ReferenceDefinition: "microscopic organisms that can cause disease", Usage: "bacteria"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "bacteria",
					Meaning:    "microscopic single-celled organisms",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Context: "Wash your hands to remove bacteria."},
					},
				},
			},
		},
		{
			description: "INCORRECT - User confused similar-sounding words. " +
				"Tests: catching when user gives meaning of wrong but similar word",
			userRequest: []inference.Expression{
				{
					Expression: "hardy",
					Meaning:    "barely or scarcely",
					Contexts: []inference.Context{
						{Context: "These hardy plants can survive the winter frost.", ReferenceDefinition: "robust, capable of endurance", Usage: "hardy"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "hardy",
					Meaning:    "robust; capable of enduring difficult conditions",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Context: "These hardy plants can survive the winter frost."},
					},
				},
			},
		},
		{
			description: "INCORRECT - User got idiom meaning completely wrong. " +
				"Tests: recognizing idiomatic expressions and rejecting wrong interpretations",
			userRequest: []inference.Expression{
				{
					Expression: "hit the books",
					Meaning:    "to strike books physically",
					Contexts: []inference.Context{
						{Context: "I need to hit the books tonight to prepare for the exam.", ReferenceDefinition: "to study hard", Usage: "hit the books"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "hit the books",
					Meaning:    "to study intensively",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Context: "I need to hit the books tonight to prepare for the exam."},
					},
				},
			},
		},
		{
			description: "INCORRECT - User too general/vague about specific thing. " +
				"Tests: rejecting when user gives general category instead of specific referent",
			userRequest: []inference.Expression{
				{
					Expression: "Beatles",
					Meaning:    "a famous band or insects",
					Contexts: []inference.Context{
						{Context: "I love listening to the Beatles, especially their early albums.", ReferenceDefinition: "famous British rock band from the 1960s", Usage: "Beatles"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "Beatles",
					Meaning:    "a famous British rock band from the 1960s",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Context: "I love listening to the Beatles, especially their early albums."},
					},
				},
			},
		},
		{
			description: "CORRECT - User's meaning is precise equivalent paraphrase. " +
				"Tests: accepting when user demonstrates exact understanding with different words",
			userRequest: []inference.Expression{
				{
					Expression: "put off",
					Meaning:    "to delay or postpone",
					Contexts: []inference.Context{
						{Context: "Don't put off your homework until the last minute.", ReferenceDefinition: "to defer to a later time", Usage: "put off"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "put off",
					Meaning:    "to postpone; to defer to a later time",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: true, Context: "Don't put off your homework until the last minute."},
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
