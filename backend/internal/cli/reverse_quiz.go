package cli

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/fatih/color"
)

// ReverseQuizCLI manages the interactive CLI session for reverse quiz
// (shows meaning, user produces the word)
type ReverseQuizCLI struct {
	*InteractiveQuizCLI
	notebookName       string
	cards              []*WordOccurrence
	listMissingContext bool
}

// NewReverseQuizCLI creates a new reverse quiz interactive CLI
func NewReverseQuizCLI(
	notebookName string,
	notebooksConfig config.NotebooksConfig,
	dictionaryCacheDir string,
	openaiClient inference.Client,
	listMissingContext bool,
) (*ReverseQuizCLI, error) {
	// Initialize base CLI
	baseCLI, reader, err := initializeQuizCLI(notebooksConfig, dictionaryCacheDir, openaiClient)
	if err != nil {
		return nil, err
	}

	var cards []*WordOccurrence

	if notebookName == "" {
		// Load all notebooks
		allStoryNotebooks, err := reader.ReadAllStoryNotebooksMap()
		if err != nil {
			return nil, fmt.Errorf("ReadAllStoryNotebooksMap() > %w", err)
		}

		for notebookID, stories := range allStoryNotebooks {
			learningHistory, ok := baseCLI.learningHistories[notebookID]
			if !ok {
				continue
			}

			notebookCards := extractReverseQuizCards(notebookID, stories, learningHistory, listMissingContext)
			cards = append(cards, notebookCards...)
		}

		// Also load flashcard notebooks
		for flashcardID := range reader.GetFlashcardIndexes() {
			flashcardNotebooks, err := reader.ReadFlashcardNotebooks(flashcardID)
			if err != nil {
				continue
			}

			learningHistory, ok := baseCLI.learningHistories[flashcardID]
			if !ok {
				continue
			}

			notebookCards := extractReverseQuizCardsFromFlashcards(flashcardID, flashcardNotebooks, learningHistory, listMissingContext)
			cards = append(cards, notebookCards...)
		}
	} else {
		// Load specific notebook
		_, isStory := reader.GetStoryIndexes()[notebookName]
		_, isFlashcard := reader.GetFlashcardIndexes()[notebookName]

		if !isStory && !isFlashcard {
			return nil, fmt.Errorf("notebook %q not found in stories or flashcards", notebookName)
		}

		learningHistory, ok := baseCLI.learningHistories[notebookName]
		if !ok {
			return nil, fmt.Errorf("no learning note for %s hasn't been supported yet", notebookName)
		}

		if isFlashcard {
			flashcardNotebooks, err := reader.ReadFlashcardNotebooks(notebookName)
			if err != nil {
				return nil, fmt.Errorf("ReadFlashcardNotebooks(%s) > %w", notebookName, err)
			}
			cards = extractReverseQuizCardsFromFlashcards(notebookName, flashcardNotebooks, learningHistory, listMissingContext)
		} else {
			stories, err := reader.ReadStoryNotebooks(notebookName)
			if err != nil {
				return nil, fmt.Errorf("ReadStoryNotebooks(%s) > %w", notebookName, err)
			}
			cards = extractReverseQuizCards(notebookName, stories, learningHistory, listMissingContext)
		}
	}

	// Deduplicate cards by expression to avoid asking the same word multiple times
	cards = deduplicateCardsByExpression(cards)

	// Sort cards so that words without context come first
	// This way users are notified upfront about missing context
	cards = sortCardsByContextAvailability(cards)

	return &ReverseQuizCLI{
		InteractiveQuizCLI: baseCLI,
		notebookName:       notebookName,
		cards:              cards,
		listMissingContext: listMissingContext,
	}, nil
}

// deduplicateCardsByExpression removes duplicate cards with the same expression,
// keeping the one with the most context sentences
func deduplicateCardsByExpression(cards []*WordOccurrence) []*WordOccurrence {
	seen := make(map[string]*WordOccurrence)

	for _, card := range cards {
		expr := strings.ToLower(card.GetExpression())
		existing, ok := seen[expr]
		if !ok {
			seen[expr] = card
		} else {
			// Keep the card with more contexts
			if len(card.Contexts) > len(existing.Contexts) {
				seen[expr] = card
			}
		}
	}

	result := make([]*WordOccurrence, 0, len(seen))
	for _, card := range seen {
		result = append(result, card)
	}
	return result
}

// sortCardsByContextAvailability sorts cards so that words without context come first.
// This allows users to see which words are missing context before proceeding to words with context.
func sortCardsByContextAvailability(cards []*WordOccurrence) []*WordOccurrence {
	// Partition cards into two groups: without context and with context
	var withoutContext, withContext []*WordOccurrence
	for _, card := range cards {
		if len(card.Contexts) == 0 {
			withoutContext = append(withoutContext, card)
		} else {
			withContext = append(withContext, card)
		}
	}

	// Return cards without context first, then cards with context
	result := make([]*WordOccurrence, 0, len(cards))
	result = append(result, withoutContext...)
	result = append(result, withContext...)
	return result
}

// ShuffleCards shuffles the quiz cards within each context-availability group,
// preserving the partition where cards without context come first.
func (r *ReverseQuizCLI) ShuffleCards() {
	// Find the boundary between no-context and with-context groups
	boundary := 0
	for boundary < len(r.cards) && len(r.cards[boundary].Contexts) == 0 {
		boundary++
	}

	// Shuffle each group independently
	noContext := r.cards[:boundary]
	withContext := r.cards[boundary:]
	rand.Shuffle(len(noContext), func(i, j int) {
		noContext[i], noContext[j] = noContext[j], noContext[i]
	})
	rand.Shuffle(len(withContext), func(i, j int) {
		withContext[i], withContext[j] = withContext[j], withContext[i]
	})
}

// getNextCard returns the next card or nil if no more cards
func (r *ReverseQuizCLI) getNextCard() *WordOccurrence {
	if len(r.cards) == 0 {
		return nil
	}
	return r.cards[0]
}

// removeCurrentCard removes the current card from the session
func (r *ReverseQuizCLI) removeCurrentCard() {
	if len(r.cards) > 0 {
		r.cards = r.cards[1:]
	}
}

// GetCardCount returns the number of remaining cards
func (r *ReverseQuizCLI) GetCardCount() int {
	return len(r.cards)
}

// Session runs a single quiz session
func (r *ReverseQuizCLI) Session(ctx context.Context) error {
	currentCard := r.getNextCard()
	if currentCard == nil {
		fmt.Println("No more cards to practice!")
		return errEnd
	}

	startTime := time.Now()

	// Format and display the question
	question := FormatReverseQuestion(currentCard)
	fmt.Print(question)
	_, _ = r.bold.Printf("Word: ")

	userAnswer, err := r.stdinReader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading input: %w", err)
	}
	userAnswer = strings.TrimSpace(userAnswer)

	responseTimeMs := time.Since(startTime).Milliseconds()

	// Validate the answer
	isCorrect, quality, reason, err := r.validateAnswer(ctx, currentCard, userAnswer, responseTimeMs, false)
	if err != nil {
		return fmt.Errorf("validateAnswer > %w", err)
	}

	// Display result
	r.displayResult(currentCard, userAnswer, isCorrect, reason)

	// Update learning history
	if err := r.updateReverseHistory(currentCard, isCorrect, quality, responseTimeMs); err != nil {
		return err
	}

	r.removeCurrentCard()
	return nil
}

// validateAnswer validates the user's answer using three-tier validation
func (r *ReverseQuizCLI) validateAnswer(
	ctx context.Context,
	card *WordOccurrence,
	userAnswer string,
	responseTimeMs int64,
	isRetry bool,
) (isCorrect bool, quality int, reason string, err error) {
	expectedWord := card.GetExpression()
	meaning := card.GetMeaning()

	// Handle empty answer
	if userAnswer == "" {
		return false, int(notebook.QualityWrong), "empty answer", nil
	}

	// Tier 1: Exact match (case-insensitive) with the displayed expression
	if strings.EqualFold(userAnswer, expectedWord) {
		quality = r.calculateQuality(responseTimeMs, isRetry)
		return true, quality, "exact match", nil
	}

	// Tier 1b: Check if matches the Expression field (when Definition is different)
	// GetExpression() returns Definition if set, so we also accept the original Expression
	if card.Definition.Definition != "" && strings.EqualFold(userAnswer, card.Definition.Expression) {
		quality = r.calculateQuality(responseTimeMs, isRetry)
		return true, quality, "matches expression", nil
	}

	// Tier 2: OpenAI classification
	contextStr := ""
	if len(card.Contexts) > 0 {
		contextStr = card.Contexts[0].Context
	}

	validation, err := r.openaiClient.ValidateWordForm(ctx, inference.ValidateWordFormRequest{
		Expected:   expectedWord,
		UserAnswer: userAnswer,
		Meaning:    meaning,
		Context:    contextStr,
	})
	if err != nil {
		return false, int(notebook.QualityWrong), fmt.Sprintf("validation error: %v", err), nil
	}

	switch validation.Classification {
	case inference.ClassificationSameWord:
		quality = r.calculateQuality(responseTimeMs, isRetry)
		return true, quality, validation.Reason, nil

	case inference.ClassificationSynonym:
		if !isRetry {
			// Allow one retry for synonym
			fmt.Printf("\n%s That's a valid synonym! But we're looking for a specific word.\n", r.bold.Sprint("Hint:"))
			fmt.Printf("Your word \"%s\" means the same thing. Try the exact word we're looking for.\n\n", userAnswer)
			_, _ = r.bold.Printf("Word: ")

			retryAnswer, err := r.stdinReader.ReadString('\n')
			if err != nil {
				return false, int(notebook.QualityWrong), "error reading retry input", nil
			}
			retryAnswer = strings.TrimSpace(retryAnswer)

			// Calculate response time for retry
			retryResponseMs := time.Since(time.Now().Add(-time.Duration(responseTimeMs) * time.Millisecond)).Milliseconds()
			return r.validateAnswer(ctx, card, retryAnswer, retryResponseMs, true)
		}
		// On retry, synonym is still acceptable but with lower quality
		return true, int(notebook.QualityCorrectSlow), validation.Reason + " (accepted on retry)", nil

	case inference.ClassificationWrong:
		return false, int(notebook.QualityWrong), validation.Reason, nil

	default:
		return false, int(notebook.QualityWrong), "unknown classification", nil
	}
}

// calculateQuality determines quality based on response time
func (r *ReverseQuizCLI) calculateQuality(responseTimeMs int64, isRetry bool) int {
	if isRetry {
		return int(notebook.QualityCorrectSlow)
	}

	// Quality based on response time:
	// < 3 seconds: Q5 (instant recall)
	// 3-10 seconds: Q4 (normal)
	// > 10 seconds: Q3 (struggled)
	switch {
	case responseTimeMs < 3000:
		return int(notebook.QualityCorrectFast)
	case responseTimeMs < 10000:
		return int(notebook.QualityCorrect)
	default:
		return int(notebook.QualityCorrectSlow)
	}
}

// displayResult shows the result of the quiz
func (r *ReverseQuizCLI) displayResult(card *WordOccurrence, userAnswer string, isCorrect bool, reason string) {
	expectedWord := card.GetExpression()
	meaning := card.GetMeaning()

	fmt.Println()
	if isCorrect {
		fmt.Print("\u2705 ")
		color.Green(`Correct! The word is "%s" - %s`,
			r.bold.Sprint(expectedWord),
			r.italic.Sprint(meaning),
		)
	} else {
		fmt.Print("\u274C ")
		color.Red(`Incorrect. The word is "%s" - %s`,
			r.bold.Sprint(expectedWord),
			r.italic.Sprint(meaning),
		)
		fmt.Printf("   You answered: %s\n", userAnswer)
	}
	if reason != "" {
		fmt.Printf("   Reason: %s\n", reason)
	}

	// Show context sentences (unmasked) to help understand the phrase in context
	if len(card.Contexts) > 0 {
		fmt.Println()
		fmt.Println("   Context:")
		cleanContexts := card.GetCleanContexts()
		for i, cleanContext := range cleanContexts {
			fmt.Printf("   %d. %s\n", i+1, cleanContext)
		}
	}
	fmt.Println()
}

// updateReverseHistory updates the learning history for reverse quiz
func (r *ReverseQuizCLI) updateReverseHistory(card *WordOccurrence, isCorrect bool, quality int, responseTimeMs int64) error {
	// Use GetExpression() to match learning history entries consistently with forward quiz
	// GetExpression() returns Definition.Definition if set, otherwise Definition.Expression
	// This ensures both forward and reverse quiz use the same expression key
	wordToRecord := card.GetExpression()
	cardNotebookName := card.NotebookName

	storyTitle := "flashcards"
	sceneTitle := ""
	if card.Story != nil {
		storyTitle = card.Story.Event
		sceneTitle = card.Scene.Title
	}

	updater := notebook.NewLearningHistoryUpdater(r.learningHistories[cardNotebookName])
	updater.UpdateOrCreateExpressionWithQualityForReverse(
		cardNotebookName,
		storyTitle,
		sceneTitle,
		wordToRecord,
		isCorrect,
		true, // always mark as known word for reverse quiz
		quality,
		responseTimeMs,
	)
	learningHistory := updater.GetHistory()

	// Save learning history
	notePath := r.learningNotesDir + "/" + cardNotebookName + ".yml"
	if err := notebook.WriteYamlFile(notePath, learningHistory); err != nil {
		return fmt.Errorf("failed to write a file %s > %w", notePath, err)
	}

	r.learningHistories[cardNotebookName] = learningHistory
	return nil
}

// FormatReverseQuestion formats a reverse quiz question
// Shows meaning and masked context
func FormatReverseQuestion(occurrence *WordOccurrence) string {
	meaning := occurrence.GetMeaning()
	expression := occurrence.GetExpression()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Meaning: %s\n", meaning))

	if len(occurrence.Contexts) > 0 {
		sb.WriteString("Context:\n")
		for i, ctx := range occurrence.Contexts {
			maskedContext := maskWordInContext(ctx.Context, expression, occurrence.Definition.Definition, ctx.Usage)
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, maskedContext))
		}
	} else {
		sb.WriteString("Context: (no context available - this word may be difficult to answer)\n")
	}

	sb.WriteString("\n")
	return sb.String()
}

// maskWordInContext replaces occurrences of the word with blanks
// It uses the Expression field (actual form in context) and Definition field (base form)
func maskWordInContext(context, expression, definition, usage string) string {
	// First, remove any {{ }} markers
	markerPattern := regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)
	context = markerPattern.ReplaceAllString(context, "$1")

	// Collect all word forms to mask
	wordsToMask := make(map[string]bool)
	if expression != "" {
		wordsToMask[strings.ToLower(expression)] = true
	}
	if definition != "" {
		wordsToMask[strings.ToLower(definition)] = true
	}
	if usage != "" {
		wordsToMask[strings.ToLower(usage)] = true
	}

	for word := range wordsToMask {
		// Escape special regex characters in the word
		escapedWord := regexp.QuoteMeta(word)

		// Build pattern with smart word boundaries:
		// - Use \b for word characters (letters, digits, underscore)
		// - Skip \b for punctuation (which doesn't have word boundaries)
		var patternStr string
		if len(word) > 0 {
			firstChar := word[0]
			lastChar := word[len(word)-1]

			// Check if first char is a word character (letter, digit, underscore)
			// Note: word is always lowercase (from strings.ToLower), so no need to check 'A'-'Z'
			isFirstWordChar := (firstChar >= 'a' && firstChar <= 'z') ||
				(firstChar >= '0' && firstChar <= '9') ||
				firstChar == '_'

			// Check if last char is a word character
			isLastWordChar := (lastChar >= 'a' && lastChar <= 'z') ||
				(lastChar >= '0' && lastChar <= '9') ||
				lastChar == '_'

			// Build pattern with appropriate boundaries
			if isFirstWordChar {
				patternStr = `(?i)\b` + escapedWord
			} else {
				patternStr = `(?i)` + escapedWord
			}
			if isLastWordChar {
				patternStr += `\b`
			}
		} else {
			continue // Skip empty words
		}

		pattern := regexp.MustCompile(patternStr)
		context = pattern.ReplaceAllString(context, "______")
	}

	return context
}

// extractReverseQuizCards extracts cards for reverse quiz from story notebooks
func extractReverseQuizCards(
	notebookName string,
	stories []notebook.StoryNotebook,
	learningHistory []notebook.LearningHistory,
	listMissingContext bool,
) []*WordOccurrence {
	var occurrences []*WordOccurrence

	for i := range stories {
		story := &stories[i]
		for j := range story.Scenes {
			scene := &story.Scenes[j]
			for k := range scene.Definitions {
				definition := &scene.Definitions[k]

				// Extract contexts first (needed for both regular quiz and listMissingContext)
				contexts := extractContextsFromConversations(scene, definition.Expression, definition.Definition)

				// If listMissingContext mode, skip the review check - we're just looking for missing context
				if listMissingContext {
					if len(contexts) > 0 {
						continue // Skip words with context
					}
				} else {
					// Regular quiz mode: check if word needs reverse review
					reverseNeedsReview := needsReverseReviewForStory(
						learningHistory,
						story.Event,
						scene.Title,
						definition,
					)

					if !reverseNeedsReview {
						continue
					}
				}

				occurrence := &WordOccurrence{
					NotebookName: notebookName,
					Story:        story,
					Scene:        scene,
					Definition:   definition,
					Contexts:     contexts,
				}

				occurrences = append(occurrences, occurrence)
			}
		}
	}

	return occurrences
}

// extractReverseQuizCardsFromFlashcards extracts cards for reverse quiz from flashcard notebooks
func extractReverseQuizCardsFromFlashcards(
	notebookName string,
	flashcards []notebook.FlashcardNotebook,
	learningHistory []notebook.LearningHistory,
	listMissingContext bool,
) []*WordOccurrence {
	var occurrences []*WordOccurrence

	for i := range flashcards {
		flashcard := &flashcards[i]
		for j := range flashcard.Cards {
			card := &flashcard.Cards[j]

			// Convert string examples to WordOccurrenceContext
			contexts := make([]WordOccurrenceContext, 0, len(card.Examples))
			for _, example := range card.Examples {
				// Check if word appears in example
				if strings.Contains(strings.ToLower(example), strings.ToLower(card.Expression)) {
					contexts = append(contexts, WordOccurrenceContext{
						Context: example,
						Usage:   card.Expression,
					})
				}
			}

			// If listMissingContext mode, skip the review check - we're just looking for missing context
			hasMeaningfulContext := len(contexts) > 0
			if listMissingContext {
				if hasMeaningfulContext {
					continue // Skip words with context
				}
			} else {
				// Regular quiz mode: check if card needs reverse review
				reverseNeedsReview := needsReverseReviewForFlashcard(
					learningHistory,
					flashcard.Title,
					card,
				)

				if !reverseNeedsReview {
					continue
				}
			}

			occurrence := &WordOccurrence{
				NotebookName: notebookName,
				Story:        nil,
				Scene:        nil,
				Definition:   card,
				Contexts:     contexts,
			}

			occurrences = append(occurrences, occurrence)
		}
	}

	return occurrences
}

// normalizeTitle normalizes a title for comparison by trimming whitespace
// and normalizing internal whitespace (newlines, multiple spaces -> single space)
func normalizeTitle(s string) string {
	s = strings.TrimSpace(s)
	// Replace all whitespace sequences (including newlines) with single space
	return strings.Join(strings.Fields(s), " ")
}

// evaluateReverseReviewNeed checks if matching expressions indicate a need for reverse review.
// Returns false if:
// - No matching expressions exist (word hasn't been practiced yet)
// - No matching expression has correct answers in forward quiz
// - Any matching expression with reverse_logs was recently reviewed
// Returns true if the word has been learned and needs reverse review.
func evaluateReverseReviewNeed(matchingExpressions []notebook.LearningHistoryExpression) bool {
	if len(matchingExpressions) == 0 {
		return false
	}

	// Check if ANY matching expression has correct answers in forward quiz
	hasCorrectAnswers := false
	for _, expr := range matchingExpressions {
		if expr.HasAnyCorrectAnswer() {
			hasCorrectAnswers = true
			break
		}
	}

	if !hasCorrectAnswers {
		return false
	}

	// Check if ANY matching expression with reverse_logs doesn't need review
	// (meaning it was recently reviewed under any key)
	for _, expr := range matchingExpressions {
		if len(expr.ReverseLogs) > 0 && !expr.NeedsReverseReview() {
			return false
		}
	}

	// If we get here, either:
	// - No expression has reverse_logs (never reviewed) -> needs review
	// - All expressions with reverse_logs need review -> needs review
	return true
}

// needsReverseReviewForStory checks if a word needs reverse review based on learning history.
//
// Note: This function checks ALL matching expressions (both Expression and Definition matches)
// to handle cases where learning history has duplicate entries from inconsistent recording.
func needsReverseReviewForStory(
	learningHistory []notebook.LearningHistory,
	storyTitle, sceneTitle string,
	definition *notebook.Note,
) bool {
	normalizedSceneTitle := normalizeTitle(sceneTitle)

	var matchingExpressions []notebook.LearningHistoryExpression
	for _, h := range learningHistory {
		if h.Metadata.Title != storyTitle {
			continue
		}

		for _, scene := range h.Scenes {
			if normalizeTitle(scene.Metadata.Title) != normalizedSceneTitle {
				continue
			}

			for _, expr := range scene.Expressions {
				if expr.Expression != definition.Expression && expr.Expression != definition.Definition {
					continue
				}
				matchingExpressions = append(matchingExpressions, expr)
			}
		}
	}

	return evaluateReverseReviewNeed(matchingExpressions)
}

// needsReverseReviewForFlashcard checks if a flashcard needs reverse review.
//
// Note: This function checks ALL matching expressions (both Expression and Definition matches)
// to handle cases where learning history has duplicate entries from inconsistent recording.
func needsReverseReviewForFlashcard(
	learningHistory []notebook.LearningHistory,
	flashcardTitle string,
	card *notebook.Note,
) bool {
	var matchingExpressions []notebook.LearningHistoryExpression
	for _, h := range learningHistory {
		if h.Metadata.Title != flashcardTitle {
			continue
		}

		for _, expr := range h.Expressions {
			if expr.Expression != card.Expression && expr.Expression != card.Definition {
				continue
			}
			matchingExpressions = append(matchingExpressions, expr)
		}
	}

	return evaluateReverseReviewNeed(matchingExpressions)
}

// ListMissingContext returns cards that are missing context
func (r *ReverseQuizCLI) ListMissingContext() {
	if len(r.cards) == 0 {
		fmt.Println("No words with missing context found.")
		return
	}

	// Group by notebook
	byNotebook := make(map[string][]*WordOccurrence)
	for _, card := range r.cards {
		byNotebook[card.NotebookName] = append(byNotebook[card.NotebookName], card)
	}

	fmt.Printf("Found %d words with missing context:\n\n", len(r.cards))

	for notebookName, cards := range byNotebook {
		fmt.Printf("%s (%d words):\n", r.bold.Sprint(notebookName), len(cards))
		for _, card := range cards {
			expression := card.GetExpression()
			meaning := card.GetMeaning()
			fmt.Printf("  - %s: %s\n", expression, meaning)
		}
		fmt.Println()
	}
}
