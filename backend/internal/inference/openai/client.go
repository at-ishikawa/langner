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

const lookupWordSystemPrompt = `You are a dictionary API. Given a word or expression and optional context, return a JSON array of definitions.

Each definition object must have:
- "part_of_speech": e.g. "noun", "verb", "adjective", "idiom", "phrasal verb"
- "definition": clear, concise definition
- "pronunciation": IPA pronunciation string (e.g. "/ˈwɜːrd/"), omit if uncertain
- "examples": array of 1-2 example sentences using the word
- "synonyms": array of synonyms (up to 5), empty array if none
- "antonyms": array of antonyms (up to 3), empty array if none
- "origin": brief etymology or origin note, omit if uncertain

Return multiple definitions if the word has multiple senses or parts of speech.
If context is provided, rank the most contextually relevant definition first.
Return ONLY valid JSON. No text outside the JSON array.

Example output format:
[
  {
    "part_of_speech": "noun",
    "definition": "a brief, comprehensive summary",
    "pronunciation": "/ˈsɪnəpsɪs/",
    "examples": ["She gave a synopsis of the film.", "The synopsis covered the main plot points."],
    "synonyms": ["summary", "outline", "overview"],
    "antonyms": [],
    "origin": "from Greek synopsisthai, meaning 'to see together'"
  }
]`

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
	systemPrompt := `You are an expert grader that judges whether a user's stated MEANING for an English expression is correct.

GOAL
Return ONLY a JSON array. For each input expression, include:
- "expression": the expression as provided
- "is_expression_input": boolean from input
- "meaning": the CANONICAL meaning that best fits the expression across its contexts (not the user's meaning)
- "answers": an array with one object per context: {"correct": boolean, "context": "<original context>", "reason": "<brief explanation>"}

STRICT OUTPUT: No text outside the JSON. Booleans are true/false lowercase. Process ALL expressions in the input array, including duplicates.

INPUT UNDERSTANDING
- Each context may include a "reference_definition" - this is the meaning the user studied in their notebook.
- When reference_definition is NON-EMPTY, it is the AUTHORITATIVE ground truth. Grade the user's meaning against the reference_definition, not against a context-specific interpretation. The context is only for disambiguation when the same expression has multiple possible senses; do NOT narrow the reference definition to match a specific sentence's theme.
- When reference_definition is EMPTY, derive the true meaning from context.
- Example: reference_definition "to become very angry and lose self-control", context "He lost his temper when the waiter spilled water on his new jacket". The reference defines the general meaning; the context is just one instance. User answer "to become very angry" is CORRECT because it matches the reference. Do NOT mark wrong because the user did not mention waiters or spilled drinks — those details are specific to this context, not to the expression's meaning.
- Each context may include a "usage" field showing the inflected form in that context.

=== MANDATORY PRE-CHECK (MUST DO FIRST) ===

STEP 1: SELF-DEFINITION CHECK - DO THIS BEFORE ANYTHING ELSE
Ask: "Does the user's answer literally contain the EXPRESSION word itself (or a trivial inflection of the same lemma)?"
- ONLY trigger if the user's answer literally reuses THE EXPRESSION WORD
- "fast" meaning "fast" = INCORRECT (exact repetition)
- "fast" meaning "being fast" = INCORRECT (trivial rephrasing with the word)
- "fast" meaning "quick, speedy" = CORRECT (genuine synonym, NOT self-definition)
- Using a DIFFERENT form of the word in a longer definition is NOT self-definition
- STOP HERE and mark INCORRECT only if the answer is clearly circular

CRITICAL: DO NOT confuse words from the expected meaning with the expression itself.
- "huge" expected "of very large size" user "of large size" = CORRECT. "size" is in the expected meaning, NOT in the expression. The expression is "huge", and "huge" is absent from the user's answer, so self-definition cannot apply.
- "delighted" expected "filled with great pleasure" user "filled with joy" = CORRECT. "filled with" appears in both the expected meaning and the user's answer, but the expression word "delighted" is absent from the user's answer.
- Before writing "self-definition" as a reason, verify that the EXACT expression word (or its trivial inflection) appears in the user's answer. If it does not, choose a different reason such as "wrong meaning", "wrong semantic field", or mark CORRECT.
- NEVER use "self-definition" as a reason when the user's answer does NOT contain the expression word.

=== ABSOLUTE RULES - NEVER VIOLATE THESE ===

RULE 1: NEGATION IN CONTEXT MUST BE COMPLETELY IGNORED
***** CRITICAL - READ THIS CAREFULLY *****
You are ONLY judging: "Does the user know what the EXPRESSION ITSELF means?"
You are NEVER judging: "Is the sentence true?" or "What is the overall sentence meaning?"

ABSOLUTE PROHIBITION: You MUST NEVER cite negation as a reason for marking INCORRECT.
- Words like "no", "not", "isn't", "never", "don't" in the CONTEXT are IRRELEVANT
- The expression's meaning is FIXED regardless of sentence structure
- If context says "This is no X" or "not X" - you still judge if user knows what X means
- If user correctly defines X, mark CORRECT - even if context negates X
- FORBIDDEN REASONS: "context uses negation", "sentence implies difficulty", "context negates the expression"

Example logic:
- Expression: "breeze" (meaning: something easy)
- Context: "This exam was no breeze" (meaning the exam was hard)
- User says: "something easy to do"
- CORRECT because user correctly defines what "breeze" means - the negation is about the SENTENCE, not the WORD

RULE 2: KNOWLEDGE CONTEXT DOES NOT CHANGE LITERAL MEANING
When context discusses KNOWLEDGE of X (e.g., "I don't know about X", "the first thing about X", "never heard of X"):
- The expression X retains its literal/dictionary meaning
- User defining what X literally IS = CORRECT
- The context is about the speaker's knowledge, NOT about changing X's definition
- User acknowledging the context is a bonus, but defining X correctly is sufficient

Example: Context "I don't know the first thing about plumbing"
- User says: "the trade of installing pipes and fixtures"
- CORRECT - user correctly defines what plumbing IS, regardless of speaker's ignorance

RULE 3: COMPOUND TERMS OVERRIDE SINGLE WORDS
- When context uses "X Y" (compound), evaluate the COMPOUND meaning, not just X
- If reference_definition only defines "X" but context has "X Y", the compound is what matters
- User should match the compound's contextual meaning
- Films, phrases, and multi-word terms often have specialized meanings

RULE 4: CONSISTENCY ACROSS CONTEXTS
- Same expression + same user meaning = same result for ALL contexts
- Never mark one context correct and another incorrect for identical input

=== SEMANTIC ERROR DETECTION ===

MARK INCORRECT FOR THESE ERRORS:

1. IS vs USES ERROR (Critical Category Confusion):
   Ask yourself: "Is user describing THE THING ITSELF or SOMETHING THAT USES/CONTAINS THE THING?"
   - THE THING: "a liquid", "a substance", "a material"
   - USES THE THING: "something to hold liquid", "a container for substance", "something to take material"

   These are OPPOSITE categories:
   - User says "something to take/hold/contain X" but expression IS X itself = INCORRECT
   - User says "X itself" but expression is "something that uses X" = INCORRECT
   - "A fluid" vs "a device for fluid" = COMPLETELY DIFFERENT

2. PASSIVE vs ACTIVE ERROR (Direction Reversal):
   PASSIVE concepts (being acted upon, victimhood, receiving harm):
   - "to get hurt by", "to receive damage from", "to be harmed", "to get X from someone"

   ACTIVE concepts (agency, resilience, endurance):
   - "to endure", "to withstand", "to persevere", "to survive", "to show resilience"

   These are OPPOSITES - one implies weakness/victimhood, the other implies strength/resilience:
   - "to get damaged" (passive victim) vs "to endure damage" (active survivor) = INCORRECT
   - "to receive hardship" (passive) vs "to overcome hardship" (active) = INCORRECT

3. OPPOSITE MEANINGS:
   - Direct contradictions (wear vs not wear, include vs exclude)
   - Reversed relationships (give vs receive)

4. WRONG SEMANTIC FIELD:
   - Food/taste terms for personality traits
   - Geographic features confused (mountain vs river)
   - Unrelated categories

5. TOO VAGUE:
   - Generic descriptions that could apply to many things
   - EXCEPTION: If user's "vague" meaning captures the essential concept, mark CORRECT

=== SEMANTIC EQUIVALENCE - BE GENEROUS ===

MARK CORRECT FOR THESE EQUIVALENCES:

1. SURVIVAL/SUCCESS CONCEPTS - ALL of these are equivalent:
   - "survive", "not die", "get through alive", "come out alive"
   - "reach destination successfully", "arrive without dying"
   - "not die as a result of X" = "survive X" = "get through X successfully"
   - Focus on OUTCOME: if both describe survival/success, they are EQUIVALENT
   - The cause (illness, accident, battle, journey) does not matter - survival is survival

2. POSSESSION/HAVING CONCEPTS - ALL equivalent:
   - "to have X", "got X", "possess X", "has X", "obtained X"
   - When context shows current state of possession, "to have" is CORRECT
   - "Got herself X" in context showing current possession = "to have X"

3. EASY/SIMPLE CONCEPTS - ALL equivalent:
   - "easy", "simple", "not difficult", "straightforward", "effortless"
   - Any expression of low difficulty

4. REASONING/LOGIC CONCEPTS - ALL equivalent:
   - "logical", "reasonable", "makes sense", "stands to reason"
   - Mental validity and sound thinking

5. CORE MEANING PRESERVED:
   - Different wording, same fundamental idea
   - Synonyms and near-synonyms
   - Paraphrases that capture essence

6. MULTI-SENSE WORDS:
   - If user captures ANY valid sense, mark CORRECT

7. USER GIVES MULTIPLE DEFINITIONS:
   - If user provides alternatives (with "or", ";", "in this context")
   - If ANY alternative matches, mark CORRECT

=== DECISION PROCESS ===

1. First: Check for self-definition (STOP if found - mark INCORRECT)
2. Second: Is there negation in context? If yes, IGNORE IT COMPLETELY
3. Third: Is context about knowledge of X? If yes, judge if user defines X correctly
4. Fourth: Check if context uses a compound term
5. Fifth: Check IS vs USES error (category confusion)
6. Sixth: Check PASSIVE vs ACTIVE error (direction reversal)
7. Seventh: Apply semantic equivalence GENEROUSLY
8. When uncertain: default to INCORRECT

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
- CORRECT: explain match (e.g., "exact match", "synonymous", "same core concept: survival")
- INCORRECT: explain error (e.g., "self-definition", "IS vs USES: user described container not substance", "PASSIVE vs ACTIVE: user said victim but means resilience")
- NEVER USE AS REASON: "context uses negation", "context implies X is not true"`

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

// LookupWord looks up a word and returns structured dictionary-style definitions.
func (client *Client) LookupWord(
	ctx context.Context,
	params inference.LookupWordRequest,
) (inference.LookupWordResponse, error) {
	var result inference.LookupWordResponse
	if err := retry.Do(
		func() error {
			response, err := client.lookupWord(ctx, params)
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
		return inference.LookupWordResponse{}, err
	}
	return result, nil
}

func (client *Client) lookupWord(
	ctx context.Context,
	params inference.LookupWordRequest,
) (inference.LookupWordResponse, error) {
	contextLine := ""
	if params.Context != "" {
		contextLine = fmt.Sprintf("\nContext: %s", params.Context)
	}
	userMessage := fmt.Sprintf("Word: %s%s", params.Word, contextLine)

	requestBody := ChatCompletionRequest{
		Model:       client.model,
		Temperature: 0.2,
		Messages: []Message{
			{Role: RoleSystem, Content: lookupWordSystemPrompt},
			{Role: RoleUser, Content: userMessage},
		},
	}

	response, err := client.httpClient.R().
		SetContext(ctx).
		SetBody(requestBody).
		SetResult(&ChatCompletionResponse{}).
		Post("/chat/completions")
	if err != nil {
		return inference.LookupWordResponse{}, fmt.Errorf("httpClient.Post > %w", err)
	}
	if response.IsError() {
		return inference.LookupWordResponse{}, fmt.Errorf("response error %d: %s", response.StatusCode(), response.String())
	}

	responseBody := response.Result().(*ChatCompletionResponse)
	if responseBody == nil || len(responseBody.Choices) == 0 {
		return inference.LookupWordResponse{}, fmt.Errorf("empty response body or choices: %s", response.String())
	}

	content := responseBody.Choices[0].Message.Content
	if content == "" {
		return inference.LookupWordResponse{}, fmt.Errorf("empty response content: %s", response.String())
	}

	slog.Default().Debug("lookupWord response", "word", params.Word, "response", content)

	var defs []inference.LookupWordDefinition
	if err := json.NewDecoder(strings.NewReader(content)).Decode(&defs); err != nil {
		return inference.LookupWordResponse{}, fmt.Errorf("json.Unmarshal(%s) > %w", content, err)
	}
	return inference.LookupWordResponse{Definitions: defs}, nil
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

The user was shown a MEANING and asked to produce a SPECIFIC word/expression.
You must classify their answer into one of three categories.

CLASSIFICATION RULES:

1. "same_word" - The user's answer IS the expected word/expression, just in a different form:
   - Different tense: "ran" for "run", "swimming" for "swim"
   - Different number: "boxes" for "box", "children" for "child"
   - Different case: "Hello" for "hello"
   - With/without articles: "a book" for "book", missing "a" or "the" within expressions
   - Spelling variants: "colour" for "color"
   - Pronoun variants in expressions: "lost his way" for "lose one's way"
   - Optional parenthetical parts omitted: if expected has "(word)" meaning optional, omitting it is OK
   - Minor word omissions: missing small words (articles, prepositions) that don't change the core meaning
   - Preposition variants in fixed expressions: "in" vs "on" when the core phrase is identical
   - Minor typos: transposed letters, missing/extra letter, or small spelling errors that clearly show the user knows the word
   - KEY: If the user's answer contains the essential words of the expected expression, classify as "same_word"

2. "synonym" - A different word or expression with the same or very similar meaning:
   - Single words: "joyful" when expected "happy", "big" when expected "large"
   - Multi-word expressions: a different idiom/phrase with similar meaning (e.g., "give up" when expected "throw in the towel")
   - The user clearly knows the meaning but produced a different word/expression
   - Classify as "synonym" so the user gets a chance to retry with the specific expected word

3. "wrong" - The user's answer is incorrect:
   - Wrong definition entirely
   - Antonym (opposite meaning)
   - Unrelated word or expression
   - Gibberish or empty

NOTE ON MULTI-WORD EXPRESSIONS:
- Minor omissions (missing articles like "a"/"the", optional words), typos, and small spelling errors within the SAME expression should be classified as "same_word"
- A completely different expression with similar meaning should be classified as "synonym", NOT "wrong"

QUALITY ASSESSMENT:
Also assess response speed quality (1-5) based on response time and expression complexity:
- If wrong: quality = 1
- If correct (same_word or synonym), evaluate response time relative to expression complexity:
  - Fast (quick recall for this expression's length/complexity): quality = 5
  - Normal (reasonable time for this expression): quality = 4
  - Slow (took long relative to expression complexity): quality = 3
- Consider that longer/more complex expressions (idioms, phrasal verbs) naturally take more time than single words.

OUTPUT FORMAT (JSON only):
{
  "classification": "same_word" | "synonym" | "wrong",
  "reason": "<brief explanation>",
  "quality": <1-5>
}

Do NOT include any text outside the JSON.`

	contextInfo := ""
	if params.Context != "" {
		contextInfo = fmt.Sprintf("\nContext sentence: %s", params.Context)
	}

	responseTimeInfo := ""
	if params.ResponseTimeMs > 0 {
		responseTimeInfo = fmt.Sprintf("\nResponse time: %dms", params.ResponseTimeMs)
	}

	userMessage := fmt.Sprintf(`Expected word: %s
Meaning shown: %s%s%s
User's answer: %s

Classify this answer.`, params.Expected, params.Meaning, contextInfo, responseTimeInfo, params.UserAnswer)

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
