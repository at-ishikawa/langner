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

// commonWords are words that don't contribute to semantic meaning comparison
var commonWords = map[string]bool{
	"a": true, "an": true, "the": true, "to": true, "be": true, "is": true,
	"of": true, "for": true, "in": true, "on": true, "at": true, "by": true,
	"it": true, "i": true, "you": true, "we": true, "they": true, "he": true, "she": true,
	"my": true, "your": true, "our": true, "their": true, "his": true, "her": true,
	"this": true, "that": true, "these": true, "those": true,
	"and": true, "or": true, "but": true, "if": true, "so": true,
	"all": true, "too": true, "well": true, "very": true,
	"yourself": true, "myself": true, "ourselves": true, "themselves": true,
}

// isMissingActionVerbPattern checks if shorter is missing the action verb from longer
// Returns true only if longer is "to [verb] X" and shorter is just "X" (missing the verb)
// Returns false if shorter has the same verb (e.g., "save X" vs "to save X")
func isMissingActionVerbPattern(shorter, longer string) bool {
	if !strings.HasPrefix(longer, "to ") {
		return false
	}
	if strings.HasPrefix(shorter, "to ") {
		return false
	}

	// Extract verb from longer
	longerWithoutTo := strings.TrimPrefix(longer, "to ")
	longerWords := strings.Fields(longerWithoutTo)
	if len(longerWords) == 0 {
		return false
	}
	verb := longerWords[0]

	// Check if shorter starts with the same verb
	shorterWords := strings.Fields(shorter)
	if len(shorterWords) > 0 && shorterWords[0] == verb {
		return false // Has the verb, just missing "to"
	}

	return true // Missing the actual verb
}

// normalizeText applies general normalization for comparison
func normalizeText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// Remove common subject prefixes
	for _, prefix := range []string{"i ", "you ", "we ", "they ", "it "} {
		if strings.HasPrefix(s, prefix) {
			s = strings.TrimPrefix(s, prefix)
			break
		}
	}
	// Normalize common synonyms (general linguistic equivalences)
	s = strings.ReplaceAll(s, "listen to", "hear")
	s = strings.ReplaceAll(s, "listen", "hear")
	return s
}

// referenceMatchesUser checks if user meaning matches reference_definition.
// This is a general, model-agnostic check.
func referenceMatchesUser(userMeaning, refDef string) bool {
	if refDef == "" {
		return false
	}
	userLower := strings.ToLower(strings.TrimSpace(userMeaning))
	refLower := strings.ToLower(strings.TrimSpace(refDef))

	// Split reference on "but in" to get the base definition (handles notebook annotations)
	if idx := strings.Index(refLower, ", but in"); idx > 0 {
		refLower = strings.TrimSpace(refLower[:idx])
	} else if idx := strings.Index(refLower, "; but in"); idx > 0 {
		refLower = strings.TrimSpace(refLower[:idx])
	}

	// Handle semicolon-separated multiple definitions
	refParts := strings.Split(refLower, ";")

	// Exact match against any part
	for _, part := range refParts {
		part = strings.TrimSpace(part)
		if part == userLower {
			return true
		}
	}

	// User contains full reference or a ref part
	if strings.Contains(userLower, refLower) {
		return true
	}
	for _, part := range refParts {
		part = strings.TrimSpace(part)
		if part != "" && strings.Contains(userLower, part) {
			return true
		}
	}

	// Check with normalized text (handles listen/hear equivalence)
	userNorm := normalizeText(userMeaning)
	for _, part := range refParts {
		partNorm := normalizeText(part)
		if partNorm != "" && (userNorm == partNorm || strings.Contains(userNorm, partNorm) || strings.Contains(partNorm, userNorm)) {
			return true
		}
	}

	// Reference contains user meaning, but guard against missing action verbs
	// "hardship" should NOT match "to endure hardship"
	// But "save a lot of money" SHOULD match "to save a lot of money"
	if strings.Contains(refLower, userLower) {
		if isMissingActionVerbPattern(userLower, refLower) {
			return false
		}
		// If lengths are close enough, it's a match
		if len(userLower) >= len(refLower)-15 {
			return true
		}
		return false
	}

	return false
}

// hasMissingActionVerb detects when user defines only the object/state
// but misses the critical action verb from the reference definition.
// E.g., user says "hardship" when reference says "to endure hardship".
// But "save a lot of money" vs "to save a lot of money" is NOT missing - user has the verb.
func hasMissingActionVerb(userMeaning, refDef string) bool {
	if refDef == "" {
		return false
	}
	userLower := strings.ToLower(strings.TrimSpace(userMeaning))
	refLower := strings.ToLower(strings.TrimSpace(refDef))

	// Only applies when reference starts with "to [verb]"
	if !strings.HasPrefix(refLower, "to ") {
		return false
	}

	// If user also starts with "to ", they included an action verb
	if strings.HasPrefix(userLower, "to ") {
		return false
	}

	// Extract the verb from reference (word after "to ")
	refWithoutTo := strings.TrimPrefix(refLower, "to ")
	refWords := strings.Fields(refWithoutTo)
	if len(refWords) == 0 {
		return false
	}
	refVerb := refWords[0]

	// If user meaning starts with the same verb, they have the action verb
	// (just missing the infinitive marker "to")
	userWords := strings.Fields(userLower)
	if len(userWords) > 0 && userWords[0] == refVerb {
		return false // User has the verb, not missing action verb
	}

	// Check if user meaning is a subset of the reference (just the object/noun part)
	// by seeing if reference contains the user's words
	if strings.Contains(refLower, userLower) {
		return true
	}

	return false
}

// isSelfDefinition checks if user meaning is actually a self-definition
// (i.e., user repeats the expression's key words instead of defining it)
// This is a general, model-agnostic check.
func isSelfDefinition(expression, userMeaning string) bool {
	exprLower := strings.ToLower(expression)
	userLower := strings.ToLower(userMeaning)

	// Extract content words from expression
	exprWords := strings.FieldsFunc(exprLower, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || r == '\'')
	})

	keyWordsRepeated := 0
	totalKeyWords := 0

	for _, word := range exprWords {
		word = strings.Trim(word, "'")
		if len(word) < 3 || commonWords[word] {
			continue
		}
		totalKeyWords++
		if strings.Contains(userLower, word) {
			keyWordsRepeated++
		}
	}

	// If no key words, can't be self-definition
	if totalKeyWords == 0 {
		return false
	}

	// If half or more key words are repeated, it's likely a self-definition
	return float64(keyWordsRepeated)/float64(totalKeyWords) >= 0.5
}

// meaningsSimilar checks if two meanings are semantically similar
// It's strict: requires high word overlap to avoid false positives
func meaningsSimilar(meaning1, meaning2 string) bool {
	if meaning1 == "" || meaning2 == "" {
		return false
	}
	m1 := normalizeText(meaning1)
	m2 := normalizeText(meaning2)

	// Exact match
	if m1 == m2 {
		return true
	}

	// If model's meaning has multiple parts (semicolon-separated),
	// check if user matches ANY part exactly
	if strings.Contains(m2, ";") {
		parts := strings.Split(m2, ";")
		for _, part := range parts {
			partNorm := normalizeText(part)
			if partNorm == m1 || strings.Contains(m1, partNorm) || strings.Contains(partNorm, m1) {
				// Ensure lengths are close (avoid partial matches)
				if len(m1) >= len(partNorm)-10 && len(m1) <= len(partNorm)+10 {
					return true
				}
			}
		}
		return false
	}

	// For single meanings, check word overlap with high threshold
	words1 := extractContentWords(m1)
	words2 := extractContentWords(m2)

	if len(words1) == 0 || len(words2) == 0 {
		return false
	}

	// Count matching words
	matches := 0
	for _, w1 := range words1 {
		for _, w2 := range words2 {
			if w1 == w2 {
				matches++
				break
			}
		}
	}

	// Require high overlap (70%+) from both sides to avoid partial matches
	overlap1 := float64(matches) / float64(len(words1))
	overlap2 := float64(matches) / float64(len(words2))
	return overlap1 >= 0.7 && overlap2 >= 0.7
}

// extractContentWords extracts meaningful words from text
func extractContentWords(s string) []string {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || r == '\'')
	})
	var result []string
	for _, word := range words {
		word = strings.Trim(word, "'")
		if len(word) >= 3 && !commonWords[word] {
			result = append(result, word)
		}
	}
	return result
}

// postProcessAnswers applies general, model-agnostic corrections to model output.
// It validates against reference_definition OR the model's own canonical meaning:
// 1. False negatives: model rejected but user meaning matches reference/model meaning → flip to CORRECT
// 2. False positives: model accepted but user meaning is missing action verb → flip to INCORRECT
func postProcessAnswers(answers []inference.AnswerMeaning, expressions []inference.Expression) []inference.AnswerMeaning {
	// Build lookup from expression name to input data
	exprData := make(map[string]struct {
		contexts []inference.Context
		meaning  string
	})
	for _, expr := range expressions {
		exprData[expr.Expression] = struct {
			contexts []inference.Context
			meaning  string
		}{
			contexts: expr.Contexts,
			meaning:  expr.Meaning,
		}
	}

	for i, answer := range answers {
		data, ok := exprData[answer.Expression]
		if !ok {
			continue
		}

		for j, ansCtx := range answer.AnswersForContext {
			// --- Fix false negatives: model rejected but user meaning matches ---
			if !ansCtx.Correct {
				corrected := false

				// Check against reference_definition
				for _, ctx := range data.contexts {
					if referenceMatchesUser(data.meaning, ctx.ReferenceDefinition) {
						slog.Default().Info("Post-process: correcting false negative",
							"expression", answer.Expression,
							"userMeaning", data.meaning,
							"reason", "user meaning matches reference definition",
							"originalReason", ansCtx.Reason)
						answers[i].AnswersForContext[j].Correct = true
						answers[i].AnswersForContext[j].Reason = "post-processed: user meaning matches reference definition"
						corrected = true
						break
					}
				}

				// Check against model's own canonical meaning
				// Only do this when model has a single meaning (no semicolon)
				// When model has multiple meanings, user providing one meaning is partial
				// and model's per-context judgment should be respected
				if !corrected && answer.Meaning != "" && !strings.Contains(answer.Meaning, ";") {
					if meaningsSimilar(data.meaning, answer.Meaning) {
						slog.Default().Info("Post-process: correcting false negative",
							"expression", answer.Expression,
							"userMeaning", data.meaning,
							"modelMeaning", answer.Meaning,
							"reason", "user meaning matches model's canonical meaning",
							"originalReason", ansCtx.Reason)
						answers[i].AnswersForContext[j].Correct = true
						answers[i].AnswersForContext[j].Reason = "post-processed: user meaning matches model's canonical meaning"
					}
				}
			}

			// --- Fix false positives: model accepted but action verb is missing ---
			if answers[i].AnswersForContext[j].Correct {
				for _, ctx := range data.contexts {
					if hasMissingActionVerb(data.meaning, ctx.ReferenceDefinition) {
						slog.Default().Info("Post-process: correcting false positive (missing action verb)",
							"expression", answer.Expression,
							"userMeaning", data.meaning,
							"refDef", ctx.ReferenceDefinition)
						answers[i].AnswersForContext[j].Correct = false
						answers[i].AnswersForContext[j].Reason = "post-processed: user meaning missing action verb from reference definition"
						break
					}
				}
			}
		}
	}

	return answers
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
	systemPrompt := `You are an expert grader that judges whether a user's stated MEANING for an English expression is correct.

GOAL
Return ONLY a JSON array. For each input expression, include:
- "expression": the expression as provided
- "is_expression_input": boolean from input
- "meaning": the CANONICAL meaning that best fits the expression across its contexts (not the user's meaning)
- "answers": an array with one object per context: {"correct": boolean, "context": "<original context>", "reason": "<brief explanation>"}

STRICT OUTPUT: No text outside the JSON. Booleans are true/false lowercase. Process ALL expressions in the input array, including duplicates.

=== CORE RULES ===

RULE 1: REFERENCE MATCH = CORRECT
If user's meaning matches reference_definition (exactly or semantically), mark CORRECT immediately.
- Do NOT say "but in context it means X" when user matches reference
- Minor grammatical variations are acceptable

RULE 2: SELF-DEFINITION = INCORRECT
If user repeats the SAME KEY WORD from the expression, mark INCORRECT.
- Expression "fast" → meaning "fast" = INCORRECT
- Expression "beautiful" → meaning "beautiful" = INCORRECT
- BUT: Synonyms and paraphrases are NOT self-definitions (mark CORRECT)

RULE 3: CONTEXT SHOWS USAGE, NOT MEANING
Judge the EXPRESSION's meaning, not the sentence's meaning.
- Negation in context does NOT change expression meaning
- FORBIDDEN: "context implies X", "in context it means X", "but in context..."

RULE 4: CONTEXT MODIFIERS MATTER
When context has "MODIFIER + expression", use the modified meaning:
- Look for adjectives/nouns before the expression that change its meaning
- User matching the contextual modified meaning = CORRECT

RULE 5: FIGURATIVE LANGUAGE IS VALID
Phrases like "[verb] into someone" are usually figurative, not literal.
- Accept user's figurative interpretation as CORRECT
- Do NOT reject figurative meanings as "literal meaning expected"

RULE 6: MISSING ACTION VERB = INCORRECT
When reference starts with "to [verb]...", the verb is essential:
- Reference "to endure hardship" vs user "hardship" = INCORRECT (missing verb)
- Reference "to save money" vs user "save money" = CORRECT (has verb)

=== FORBIDDEN REJECTION REASONS ===
NEVER use these as reasons to mark INCORRECT:
- "too specific" or "too vague"
- "context implies/suggests/indicates X"
- "in context it means X" or "but in context..."
- "doesn't capture the broader idea"
- "which is a different concept" (for synonymous terms)

=== SEMANTIC EQUIVALENCE ===
Be generous with equivalences:
- Synonyms and near-synonyms are equivalent
- Paraphrases that capture the essence are equivalent
- If user captures ANY valid sense of a multi-sense word, mark CORRECT

is_expression_input HANDLING
- If true: expression may have typos; judge user's intended meaning
- If false: treat expression as correct/canonical; trust user more

## Response Speed Assessment

For each answer, also assess response speed quality (1-5):
- If incorrect: quality = 1
- If correct, evaluate response time relative to meaning complexity:
  - Fast (quick recall for this meaning's length/complexity): quality = 5
  - Normal (reasonable time for this meaning): quality = 4
  - Slow (took long relative to meaning complexity): quality = 3

The input will include "response_time_ms" for each expression. Use your judgment to determine what constitutes fast/normal/slow based on the meaning's length and complexity.

OUTPUT FORMAT:
[
  {
    "expression": "...",
    "is_expression_input": false,
    "meaning": "...",
    "answers": [
      {"correct": true,  "context": "...", "reason": "user meaning matches: both mean X", "quality": 5},
      {"correct": false, "context": "...", "reason": "user said X but it means Y", "quality": 1}
    ]
  }
]

REASON FORMAT:
- CORRECT: explain match (e.g., "exact match", "synonymous", "same core concept: survival", "matches contextual meaning")
- INCORRECT: explain error (e.g., "self-definition", "IS vs USES: user described container not substance", "PASSIVE vs ACTIVE: user said victim but means resilience", "missing action verb: user said X but not 'to endure X'")
- NEVER USE AS REASON: "context uses negation", "context implies X is not true", "in context it means the opposite because of negation"`

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
						{Correct: false, Context: "He wore a snazzy new suit to the interview.", Reason: "user repeated the word 'snazzy' instead of defining it", Quality: 1},
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
						{Correct: false, Context: "He decided to freeball at the gym today.", Reason: "user said 'wear loose underwear' but the expression means the opposite - 'not wearing underwear'", Quality: 1},
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
						{Correct: false, Context: "The spunky little dog barked at the mailman.", Reason: "user said 'salty' which is about food/taste, but the expression describes personality traits", Quality: 1},
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
						{Correct: false, Context: "We took a boat ride along the Thames.", Reason: "user said 'mountain' but the Thames is a river - wrong geographic feature", Quality: 1},
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
						{Correct: false, Context: "The nurse injected saline into the IV.", Reason: "user said 'container' but saline IS the liquid solution itself, not the container", Quality: 1},
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
						{Correct: false, Context: "Despite the criticism, she stood her ground on the issue.", Reason: "user said 'get injured' which is passive suffering, but the expression means 'maintain position' which is actively resisting", Quality: 1},
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
						{Correct: true, Context: "This is a simple task.", Reason: "'easy' and 'simple' are synonymous", Quality: 5},
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
						{Correct: true, Context: "I'm exhausted. Time to hit the hay.", Reason: "user said 'start sleeping' and reference says 'go to bed' - these capture the same essential concept", Quality: 4},
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
						{Correct: true, Context: "This test is no piece of cake.", Reason: "the expression itself means 'easy' - the negation in context doesn't change what the expression means", Quality: 5},
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
						{Correct: true, Context: "You really nailed that presentation!", Reason: "user said 'accomplish perfectly' which captures the success concept - same essential meaning as 'succeed' and 'do very well'", Quality: 4},
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
						{Correct: true, Context: "", Reason: "user meaning matches the reference definition - both refer to soldiers on horseback", Quality: 5},
					},
				},
			},
		},
		{
			description: "CORRECT - Survival equivalence: 'not dying' and 'surviving' are the same concept",
			userRequest: []inference.Expression{
				{
					Expression: "pull through",
					Meaning:    "to get somewhere successfully without being dead",
					Contexts: []inference.Context{
						{Context: "After the surgery, we hoped she would pull through.", ReferenceDefinition: "to survive or recover from a serious illness or injury", Usage: "pull through"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "pull through",
					Meaning:    "to survive or recover from a serious illness or injury",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: true, Context: "After the surgery, we hoped she would pull through.", Reason: "same core concept: survival - 'not being dead' and 'survive' both describe the outcome of staying alive", Quality: 4},
					},
				},
			},
		},
		{
			description: "CORRECT - Possession equivalence: 'to have' matches context showing current possession",
			userRequest: []inference.Expression{
				{
					Expression: "score something",
					Meaning:    "to have something",
					Contexts: []inference.Context{
						{Context: "He scored himself a great deal on the car.", ReferenceDefinition: "to obtain or acquire something", Usage: "scored himself"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "score something",
					Meaning:    "to obtain or acquire something",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: true, Context: "He scored himself a great deal on the car.", Reason: "context shows current possession state - 'to have' correctly describes the result of acquiring something", Quality: 5},
					},
				},
			},
		},
		{
			description: "CORRECT - Knowledge context: user correctly defines what X literally IS",
			userRequest: []inference.Expression{
				{
					Expression: "carpentry",
					Meaning:    "the craft of building with wood",
					Contexts: []inference.Context{
						{Context: "I don't know the first thing about carpentry.", ReferenceDefinition: "the skill or work of making things from wood", Usage: "carpentry"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "carpentry",
					Meaning:    "the skill or work of making things from wood",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: true, Context: "I don't know the first thing about carpentry.", Reason: "user correctly defines what carpentry IS - the context is about speaker's knowledge, not changing the word's meaning", Quality: 4},
					},
				},
			},
		},
		{
			description: "INCORRECT - IS vs USES: user describes something that takes/contains X, but expression IS X itself",
			userRequest: []inference.Expression{
				{
					Expression: "serum",
					Meaning:    "something to take fluids",
					Contexts: []inference.Context{
						{Context: "The doctor administered the serum to the patient.", ReferenceDefinition: "a fluid, especially one used in medical treatment", Usage: "serum"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "serum",
					Meaning:    "a fluid, especially one used in medical treatment",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Context: "The doctor administered the serum to the patient.", Reason: "IS vs USES error: user said 'something to take fluids' (a container/recipient) but serum IS the fluid itself", Quality: 1},
					},
				},
			},
		},
		{
			description: "INCORRECT - PASSIVE vs ACTIVE: user says receiving harm (passive) but expression means resilience (active)",
			userRequest: []inference.Expression{
				{
					Expression: "weather the storm",
					Meaning:    "to get damages from bad weather",
					Contexts: []inference.Context{
						{Context: "The company managed to weather the storm during the recession.", ReferenceDefinition: "to survive a difficult situation or period", Usage: "weather the storm"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "weather the storm",
					Meaning:    "to survive a difficult situation or period",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: false, Context: "The company managed to weather the storm during the recession.", Reason: "PASSIVE vs ACTIVE error: user said 'get damages' (passive victim) but expression means 'survive/endure' (active resilience)", Quality: 1},
					},
				},
			},
		},
		// New examples to address specific failure patterns
		{
			description: "CORRECT - User's more detailed definition captures core concept (counters 'too specific' rejection)",
			userRequest: []inference.Expression{
				{
					Expression: "clue",
					Meaning:    "a piece of information that helps solve a problem or mystery",
					Contexts: []inference.Context{
						{Context: "The detective found a crucial clue.", ReferenceDefinition: "information that helps", Usage: "clue"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "clue",
					Meaning:    "information that helps",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: true, Context: "The detective found a crucial clue.", Reason: "user's more detailed definition captures the core concept - extra detail about 'solve a problem or mystery' is fine, not 'too specific'"},
					},
				},
			},
		},
		{
			description: "CORRECT - Generic definition valid even when context uses figuratively",
			userRequest: []inference.Expression{
				{
					Expression: "sharp",
					Meaning:    "having a fine edge or point",
					Contexts: []inference.Context{
						{Context: "She has a sharp mind.", ReferenceDefinition: "having a fine edge or point", Usage: "sharp"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "sharp",
					Meaning:    "having a fine edge or point",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: true, Context: "She has a sharp mind.", Reason: "user defines the primary meaning correctly - context uses 'sharp' figuratively but base definition is valid"},
					},
				},
			},
		},
		{
			description: "CORRECT - Context modifier 'oven' overrides general reference definition",
			userRequest: []inference.Expression{
				{
					Expression: "glove",
					Meaning:    "hand covering for protection from heat",
					Contexts: []inference.Context{
						{Context: "Put on oven gloves before touching the hot pan.", ReferenceDefinition: "a covering for the hand worn for protection", Usage: "gloves"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "glove",
					Meaning:    "a covering for the hand worn for protection",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: true, Context: "Put on oven gloves before touching the hot pan.", Reason: "context says 'oven gloves' - user correctly identified the contextual meaning of heat protection"},
					},
				},
			},
		},
		{
			description: "CORRECT - Negation in context doesn't affect expression definition",
			userRequest: []inference.Expression{
				{
					Expression: "fool around",
					Meaning:    "to behave in a silly way or waste time",
					Contexts: []inference.Context{
						{Context: "Stop fooling around and focus!", ReferenceDefinition: "to behave in a silly way or waste time", Usage: "fooling around"},
					},
					IsExpressionInput: false,
				},
			},
			assistantAnswer: []inference.AnswerMeaning{
				{
					Expression: "fool around",
					Meaning:    "to behave in a silly way or waste time",
					AnswersForContext: []inference.AnswersForContext{
						{Correct: true, Context: "Stop fooling around and focus!", Reason: "user correctly defines what 'fool around' means - the command 'stop' is about the sentence, not the expression's definition"},
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

	// Post-process: fix false negatives (forbidden patterns) and false positives (missing action verbs)
	decoded = postProcessAnswers(decoded, args.Expressions)

	return inference.AnswerMeaningsResponse{Answers: decoded}, nil
}

// ValidateWordForm validates if the user's answer matches the expected word
func (client *Client) ValidateWordForm(
	ctx context.Context,
	params inference.ValidateWordFormRequest,
) (inference.ValidateWordFormResponse, error) {
	var result inference.ValidateWordFormResponse
	if err := retry.Do(
		func() error {
			response, err := client.validateWordForm(ctx, params)
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
		return inference.ValidateWordFormResponse{}, err
	}
	return result, nil
}

func (client *Client) validateWordForm(
	ctx context.Context,
	params inference.ValidateWordFormRequest,
) (inference.ValidateWordFormResponse, error) {
	systemPrompt := `You are a vocabulary quiz validator for a reverse quiz (meaning → word production).

The user was shown a MEANING and asked to produce a word with that meaning.
You must classify their answer into one of three categories.

CLASSIFICATION RULES:

1. "same_word" - The user's answer IS the expected word, just in a different form:
   - Different tense: "ran" for "run", "swimming" for "swim"
   - Different number: "boxes" for "box", "children" for "child"
   - Different case: "Hello" for "hello"
   - With/without articles: "the dog" for "dog"
   - Spelling variants: "colour" for "color"

2. "synonym" - The user's answer is NOT the expected word but IS a valid word with the same meaning:
   - "happy" when expected "glad" (both mean joyful)
   - "thrilled" when expected "excited" (both mean very happy)
   - The synonym must genuinely fit the meaning shown

3. "wrong" - The user's answer does not convey the meaning:
   - Wrong definition entirely
   - Antonym (opposite meaning)
   - Unrelated word
   - Gibberish or empty

IMPORTANT:
- Focus on whether the meaning matches, not exact word form
- If the user's word legitimately expresses the given meaning, it's either "same_word" or "synonym"
- Be generous with morphological variants of the expected word

OUTPUT FORMAT (JSON only):
{
  "classification": "same_word" | "synonym" | "wrong",
  "reason": "<brief explanation>"
}

Do NOT include any text outside the JSON.`

	contextInfo := ""
	if params.Context != "" {
		contextInfo = fmt.Sprintf("\nContext sentence: %s", params.Context)
	}

	userMessage := fmt.Sprintf(`Expected word: %s
Meaning shown: %s%s
User's answer: %s

Classify this answer.`, params.Expected, params.Meaning, contextInfo, params.UserAnswer)

	requestBody := ChatCompletionRequest{
		Model:       client.model,
		Temperature: 0.1,
		Messages: []Message{
			{Role: RoleSystem, Content: systemPrompt},
			{Role: RoleUser, Content: userMessage},
		},
	}

	response, err := client.httpClient.R().
		SetContext(ctx).
		SetBody(requestBody).
		SetResult(&ChatCompletionResponse{}).
		Post("/chat/completions")
	if err != nil {
		return inference.ValidateWordFormResponse{}, fmt.Errorf("httpClient.Post > %w", err)
	}
	if response.IsError() {
		return inference.ValidateWordFormResponse{}, fmt.Errorf("response error %d: %s", response.StatusCode(), response.String())
	}

	responseBody := response.Result().(*ChatCompletionResponse)
	if responseBody == nil || len(responseBody.Choices) == 0 {
		return inference.ValidateWordFormResponse{}, fmt.Errorf("empty response body or choices: %s", response.String())
	}

	content := responseBody.Choices[0].Message.Content
	if content == "" {
		return inference.ValidateWordFormResponse{}, fmt.Errorf("empty response content: %s", response.String())
	}

	slog.Default().Debug("validateWordForm response",
		"request", requestBody,
		"response", content,
	)

	var decoded inference.ValidateWordFormResponse
	if err := json.NewDecoder(strings.NewReader(content)).Decode(&decoded); err != nil {
		return inference.ValidateWordFormResponse{}, fmt.Errorf("json.Unmarshal(%s) > %w", content, err)
	}

	return decoded, nil
}
