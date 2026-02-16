package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/fatih/color"
)

// FreeformQuizCLI manages the interactive CLI session for freeform quiz
type FreeformQuizCLI struct {
	*InteractiveQuizCLI
	allStories     map[string][]notebook.StoryNotebook
	allFlashcards  map[string][]notebook.FlashcardNotebook
}

// NewFreeformQuizCLI creates a new freeform quiz interactive CLI
func NewFreeformQuizCLI(
	notebooksConfig config.NotebooksConfig,
	dictionaryCacheDir string,
	openaiClient inference.Client,
) (*FreeformQuizCLI, error) {
	// Initialize base CLI
	baseCLI, reader, err := initializeQuizCLI(notebooksConfig, dictionaryCacheDir, openaiClient)
	if err != nil {
		return nil, err
	}

	// Read all story notebooks for context lookup (only from story indexes)
	allStories := make(map[string][]notebook.StoryNotebook)
	for notebookName := range reader.GetStoryIndexes() {
		stories, err := reader.ReadStoryNotebooks(notebookName)
		if err != nil {
			fmt.Printf("Warning: could not read stories for %s: %v\n", notebookName, err)
			continue
		}
		allStories[notebookName] = stories
	}

	// Read all flashcard notebooks for context lookup (only from flashcard indexes)
	allFlashcards := make(map[string][]notebook.FlashcardNotebook)
	for notebookName := range reader.GetFlashcardIndexes() {
		flashcards, err := reader.ReadFlashcardNotebooks(notebookName)
		if err != nil {
			fmt.Printf("Warning: could not read flashcards for %s: %v\n", notebookName, err)
			continue
		}
		allFlashcards[notebookName] = flashcards
	}

	return &FreeformQuizCLI{
		InteractiveQuizCLI: baseCLI,
		allStories:         allStories,
		allFlashcards:      allFlashcards,
	}, nil
}

func (r *FreeformQuizCLI) Session(ctx context.Context) error {
	startTime := time.Now()

	fmt.Print("Word: ")
	wordInput, err := r.stdinReader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading word input: %w", err)
	}
	word := strings.TrimSpace(wordInput)

	if word == "quit" || word == "exit" {
		fmt.Println("Practice session ended.")
		return nil
	}

	fmt.Print("Meaning: ")
	meaningInput, err := r.stdinReader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading meaning input: %w", err)
	}
	meaning := strings.TrimSpace(meaningInput)

	if err := ValidateInput(word, meaning); err != nil {
		fmt.Printf("Invalid input: %v\n", err)
		return nil
	}

	responseTimeMs := time.Since(startTime).Milliseconds()
	allWordContexts := r.findAllWordContexts(word)

	if len(allWordContexts) == 0 {
		fmt.Printf("No context found for word '%s' in story conversations.\n", word)
		// Even without context, we still validate the answer
	} else {
		fmt.Printf("Found %d occurrences of '%s' across notebooks\n", len(allWordContexts), word)
	}

	needsLearning := r.findOccurrencesNeedingLearning(allWordContexts, word)
	fmt.Printf("Found %d occurrences that need to be learned\n", len(needsLearning))
	if len(needsLearning) == 0 {
		// No occurrences needed learning - all are already mastered
		fmt.Println("All occurrences of this word have already been mastered!")
		return nil
	}

	var contexts []inference.Context
	for _, occurrence := range needsLearning {
		cleanContexts := occurrence.GetCleanContexts()
		for i, ctx := range occurrence.Contexts {
			contexts = append(contexts, inference.Context{
				Context:             cleanContexts[i],              // Use cleaned context without markers
				ReferenceDefinition: occurrence.Definition.Meaning, // Include meaning from notebook as hint
				Usage:               ctx.Usage,                     // Include the actual form used in context
			})
		}
	}

	results, err := r.openaiClient.AnswerMeanings(ctx, inference.AnswerMeaningsRequest{
		Expressions: []inference.Expression{
			{
				Expression:        word,
				Meaning:           meaning,
				Contexts:          contexts,
				IsExpressionInput: true,           // FreeformQuiz: user inputs the expression
				ResponseTimeMs:    responseTimeMs, // For quality assessment
			},
		},
	})
	if err != nil {
		return fmt.Errorf("openaiClient.AnswerMeanings() > %w", err)
	}

	if len(results.Answers) == 0 {
		return fmt.Errorf("no results returned from OpenAI")
	}
	result := results.Answers[0]

	contextToOccurrence := make(map[string]int)
	for i, occurrence := range needsLearning {
		for _, ctx := range occurrence.Contexts {
			contextToOccurrence[ctx.Context] = i
		}
	}

	isCorrect := false
	for _, answer := range result.AnswersForContext {
		if answer.Correct {
			isCorrect = true
			break
		}
	}

	quality := 1
	if len(result.AnswersForContext) > 0 {
		quality = result.AnswersForContext[0].Quality
		if quality == 0 {
			// Fallback if OpenAI didn't return quality
			if isCorrect {
				quality = 4
			} else {
				quality = 1
			}
		}
	}

	var firstCorrectOccurrenceIdx = -1
	var matchingContext string
	var matchingReason string
	for _, answer := range result.AnswersForContext {
		if answer.Correct {
			if occIdx, found := contextToOccurrence[answer.Context]; found {
				firstCorrectOccurrenceIdx = occIdx
				matchingContext = answer.Context
				matchingReason = answer.Reason
				break
			}
		}
	}

	// Get occurrence for display - use first correct one if answer is right,
	// otherwise use first occurrence to show correct meaning
	var displayOccurrence *WordOccurrence
	if firstCorrectOccurrenceIdx >= 0 {
		displayOccurrence = needsLearning[firstCorrectOccurrenceIdx]
	} else if len(needsLearning) > 0 {
		// Use first occurrence for incorrect answers
		displayOccurrence = needsLearning[0]
		// Show the first context from the first occurrence
		if len(displayOccurrence.Contexts) > 0 {
			matchingContext = displayOccurrence.Contexts[0].Context
		}
		// Get reason from first answer context for incorrect answers
		if len(result.AnswersForContext) > 0 {
			matchingReason = result.AnswersForContext[0].Reason
		}
	}

	// Build answer for display
	answer := AnswerResponse{
		Correct:    isCorrect,
		Expression: result.Expression,
		Meaning:    result.Meaning,
		Context:    matchingContext,
		Reason:     matchingReason,
	}

	// Show result
	if err := r.displayResult(answer, displayOccurrence); err != nil {
		return err
	}

	// Update learning history for all answers (correct and incorrect)
	occurrenceToUpdate := displayOccurrence
	if firstCorrectOccurrenceIdx >= 0 {
		occurrenceToUpdate = needsLearning[firstCorrectOccurrenceIdx]
	}
	if occurrenceToUpdate != nil {
		if err := r.updateLearningHistory(occurrenceToUpdate, word, answer, quality, responseTimeMs); err != nil {
			return err
		}
	}

	fmt.Println()
	return nil
}

func (r *FreeformQuizCLI) findOccurrencesNeedingLearning(allWordContexts []*WordOccurrence, word string) []*WordOccurrence {
	needsLearning := make([]*WordOccurrence, 0)

	for _, wordCtx := range allWordContexts {
		// Get learning history for this occurrence
		learningHistory, ok := r.learningHistories[wordCtx.NotebookName]
		if !ok {
			// No learning history for this notebook, so it needs learning
			needsLearning = append(needsLearning, wordCtx)
			continue
		}

		// Check if this specific expression has been answered correctly
		hasCorrectAnswer := r.hasCorrectAnswer(learningHistory, wordCtx, word)
		if !hasCorrectAnswer {
			needsLearning = append(needsLearning, wordCtx)
		}
	}

	return needsLearning
}

func (r *FreeformQuizCLI) hasCorrectAnswer(learningHistory []notebook.LearningHistory, wordCtx *WordOccurrence, word string) bool {
	// For flashcards (Story is nil), use "flashcards" as story title
	storyTitle := "flashcards"
	sceneTitle := ""
	if wordCtx.Story != nil {
		storyTitle = wordCtx.Story.Event
	}
	if wordCtx.Scene != nil {
		sceneTitle = wordCtx.Scene.Title
	}

	for _, hist := range learningHistory {
		if hist.Metadata.Title != storyTitle {
			continue
		}

		logs := hist.GetLogs(storyTitle, sceneTitle, *wordCtx.Definition)
		if len(logs) == 0 {
			continue
		}

		// For flashcard type, check expressions directly
		if hist.Metadata.Type == "flashcard" {
			for _, expr := range hist.Expressions {
				matchFound := r.isExpressionMatch(expr, wordCtx, word)
				if matchFound {
					status := expr.GetLatestStatus()
					// If status is understood, usable, or intuitive, check if threshold has passed
					if status == notebook.LearnedStatus("understood") ||
						status == notebook.LearnedStatus("usable") ||
						status == notebook.LearnedStatus("intuitive") {
						// Check if the spaced repetition threshold has passed
						if r.hasThresholdPassed(expr.LearnedLogs) {
							// Threshold has passed, needs re-learning
							return false
						}
						// Threshold has not passed, still mastered
						return true
					}
				}
			}
			continue
		}

		// For story type, check the latest status in scenes
		for _, scene := range hist.Scenes {
			if scene.Metadata.Title != sceneTitle {
				continue
			}
			for _, expr := range scene.Expressions {
				matchFound := r.isExpressionMatch(expr, wordCtx, word)
				if matchFound {
					status := expr.GetLatestStatus()
					// If status is understood, usable, or intuitive, check if threshold has passed
					if status == notebook.LearnedStatus("understood") ||
						status == notebook.LearnedStatus("usable") ||
						status == notebook.LearnedStatus("intuitive") {
						// Check if the spaced repetition threshold has passed
						if r.hasThresholdPassed(expr.LearnedLogs) {
							// Threshold has passed, needs re-learning
							return false
						}
						// Threshold has not passed, still mastered
						return true
					}
				}
			}
		}
	}
	return false
}

// hasThresholdPassed checks if the spaced repetition threshold has passed
// based on the learning logs. Returns true if threshold has passed (needs re-learning),
// false if still within threshold (mastered).
func (r *FreeformQuizCLI) hasThresholdPassed(logs []notebook.LearningRecord) bool {
	if len(logs) == 0 {
		return true
	}

	// Count correct answers (not misunderstood or learning)
	count := 0
	for _, log := range logs {
		if log.Status == notebook.LearnedStatus("") || log.Status == notebook.LearnedStatus("misunderstood") {
			continue
		}
		count++
	}

	// Use stored interval if available, otherwise use legacy calculation
	threshold := logs[0].IntervalDays
	if threshold == 0 {
		threshold = notebook.GetThresholdDaysFromCount(count)
	}

	// Most recent log is at index 0 (logs are sorted newest first)
	lastCorrectLog := logs[0]
	now := time.Now()
	thresholdDate := lastCorrectLog.LearnedAt.Add(time.Duration(threshold) * time.Hour * 24)

	// If now is after the threshold date, threshold has passed
	return now.After(thresholdDate)
}

func (r *FreeformQuizCLI) isExpressionMatch(expr notebook.LearningHistoryExpression, wordCtx *WordOccurrence, word string) bool {
	// Direct match with the expression
	if expr.Expression == wordCtx.Definition.Expression {
		return true
	}

	// Check if user is practicing the definition and this expression matches
	if wordCtx.Definition.Definition != "" && strings.EqualFold(word, wordCtx.Definition.Definition) {
		if expr.Expression == wordCtx.Definition.Expression {
			return true
		}
	}

	// Also check if the expression in learning notes matches the definition field
	if wordCtx.Definition.Definition != "" {
		if strings.EqualFold(expr.Expression, wordCtx.Definition.Definition) {
			return true
		}
	}

	return false
}

func (r *FreeformQuizCLI) displayResult(answer AnswerResponse, occurrence *WordOccurrence) error {
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)

	if answer.Correct {
		if _, err := fmt.Fprint(r.stdoutWriter, "✅ "); err != nil {
			return fmt.Errorf("failed to write to stdout: %w", err)
		}
		if _, err := green.Fprintf(r.stdoutWriter, `It's correct. The meaning of %s is "%s"`,
			r.bold.Sprintf("%s", answer.Expression),
			r.italic.Sprintf("%s", answer.Meaning),
		); err != nil {
			return fmt.Errorf("failed to write to stdout: %w", err)
		}
		if _, err := fmt.Fprintln(r.stdoutWriter); err != nil {
			return fmt.Errorf("failed to write to stdout: %w", err)
		}
	} else {
		if _, err := fmt.Fprint(r.stdoutWriter, "❌ "); err != nil {
			return fmt.Errorf("failed to write to stdout: %w", err)
		}
		correctMeaning := answer.Meaning
		if occurrence != nil && occurrence.Definition.Meaning != "" {
			correctMeaning = occurrence.Definition.Meaning
		}
		if _, err := red.Fprintf(r.stdoutWriter, `It's wrong. The meaning of %s is "%s"`,
			r.bold.Sprintf("%s", answer.Expression),
			r.italic.Sprintf("%s", correctMeaning),
		); err != nil {
			return fmt.Errorf("failed to write to stdout: %w", err)
		}
		if _, err := fmt.Fprintln(r.stdoutWriter); err != nil {
			return fmt.Errorf("failed to write to stdout: %w", err)
		}
	}

	// Show reason if available
	if answer.Reason != "" {
		if _, err := fmt.Fprintf(r.stdoutWriter, "   Reason: %s\n", answer.Reason); err != nil {
			return fmt.Errorf("failed to write to stdout: %w", err)
		}
	}

	// Show the matching context if available
	if answer.Context != "" && occurrence != nil {
		if _, err := fmt.Fprintln(r.stdoutWriter); err != nil {
			return fmt.Errorf("failed to write to stdout: %w", err)
		}
		// Convert markers to show only the target expression in bold
		// For flashcards (Scene is nil), pass empty definitions slice
		var definitions []notebook.Note
		if occurrence.Scene != nil {
			definitions = occurrence.Scene.Definitions
		}
		convertedContext := notebook.ConvertMarkersInText(
			answer.Context,
			definitions,
			notebook.ConversionStyleTerminal,
			occurrence.Definition.Expression,
		)
		if _, err := fmt.Fprintf(r.stdoutWriter, "  Context: %s\n", convertedContext); err != nil {
			return fmt.Errorf("failed to write to stdout: %w", err)
		}
	}
	return nil
}

func (r *FreeformQuizCLI) updateLearningHistory(
	occurrence *WordOccurrence,
	word string,
	answer AnswerResponse,
	quality int,
	responseTimeMs int64,
) error {
	learningHistory, ok := r.learningHistories[occurrence.NotebookName]
	if !ok {
		learningHistory = []notebook.LearningHistory{}
	}

	// Always prefer the Definition form (canonical/base form) for consistent tracking
	expressionToRecord := occurrence.Definition.Expression
	if occurrence.Definition.Definition != "" {
		expressionToRecord = occurrence.Definition.Definition
	}

	// For flashcards (Story is nil), use "flashcards" as story title
	storyTitle := "flashcards"
	sceneTitle := ""
	if occurrence.Story != nil {
		storyTitle = occurrence.Story.Event
	}
	if occurrence.Scene != nil {
		sceneTitle = occurrence.Scene.Title
	}

	var err error
	learningHistory, err = r.updateLearningHistoryWithQuality(
		occurrence.NotebookName,
		learningHistory,
		occurrence.NotebookName,
		storyTitle,
		sceneTitle,
		expressionToRecord,
		answer.Correct,
		false, // isKnownWord=false to get "usable" status when correct
		quality,
		responseTimeMs,
		notebook.QuizTypeFreeform,
	)
	if err != nil {
		return err
	}
	// Update the map with the modified learning history
	r.learningHistories[occurrence.NotebookName] = learningHistory

	return nil
}

// findAllWordContexts searches for ALL occurrences of a word across all story notebooks and flashcard notebooks
func (r *FreeformQuizCLI) findAllWordContexts(word string) []*WordOccurrence {
	var allContexts []*WordOccurrence

	// Search story notebooks
	for notebookName, stories := range r.allStories {
		for i := range stories {
			story := &stories[i]
			for j := range story.Scenes {
				scene := &story.Scenes[j]

				// Check if word exists in definitions
				for k := range scene.Definitions {
					definition := &scene.Definitions[k]
					// Check both Expression and Definition fields
					if !strings.EqualFold(definition.Expression, word) && !strings.EqualFold(definition.Definition, word) {
						continue
					}
					contexts := extractContextsFromConversations(scene, definition.Expression, definition.Definition)
					allContexts = append(allContexts, &WordOccurrence{
						NotebookName: notebookName,
						Story:        story,
						Scene:        scene,
						Definition:   definition,
						Contexts:     contexts,
					})
				}
			}
		}
	}

	// Search flashcard notebooks
	for notebookName, flashcards := range r.allFlashcards {
		for i := range flashcards {
			flashcard := &flashcards[i]
			for j := range flashcard.Cards {
				card := &flashcard.Cards[j]
				// Check both Expression and Definition fields
				if !strings.EqualFold(card.Expression, word) && !strings.EqualFold(card.Definition, word) {
					continue
				}
				// Convert string examples to WordOccurrenceContext
				contexts := make([]WordOccurrenceContext, len(card.Examples))
				for k, example := range card.Examples {
					contexts[k] = WordOccurrenceContext{
						Context: example,
						Usage:   card.Expression,
					}
				}
				allContexts = append(allContexts, &WordOccurrence{
					NotebookName: notebookName,
					Story:        nil,
					Scene:        nil,
					Definition:   card,
					Contexts:     contexts,
				})
			}
		}
	}

	return allContexts
}

// ValidateInput checks if the word and meaning inputs are valid
func ValidateInput(word, meaning string) error {
	if word == "" {
		return ErrEmptyWord
	}
	if meaning == "" {
		return ErrEmptyMeaning
	}
	return nil
}

// Errors for validation
var (
	ErrEmptyWord    = &ValidationError{Message: "Word cannot be empty"}
	ErrEmptyMeaning = &ValidationError{Message: "Meaning cannot be empty"}
)

// ValidationError represents an input validation error
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}
