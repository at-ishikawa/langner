package cli

import (
	"context"
	"fmt"
	"strings"

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
	storiesDir string,
	flashcardsDir string,
	learningNotesDir string,
	dictionaryCacheDir string,
	openaiClient inference.Client,
) (*FreeformQuizCLI, error) {
	// Initialize base CLI
	baseCLI, reader, err := initializeQuizCLI(storiesDir, flashcardsDir, learningNotesDir, dictionaryCacheDir, openaiClient)
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
	// Get word from user
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

	// Get meaning from user
	fmt.Print("Meaning: ")
	meaningInput, err := r.stdinReader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading meaning input: %w", err)
	}
	meaning := strings.TrimSpace(meaningInput)

	// Validate inputs
	if err := ValidateInput(word, meaning); err != nil {
		fmt.Printf("Invalid input: %v\n", err)
		return nil
	}

	// Look up ALL occurrences of the word across all story notebooks
	allWordContexts := r.findAllWordContexts(word)

	if len(allWordContexts) == 0 {
		fmt.Printf("No context found for word '%s' in story conversations.\n", word)
		// Even without context, we still validate the answer
	} else {
		fmt.Printf("Found %d occurrences of '%s' across notebooks\n", len(allWordContexts), word)
	}

	// Check each occurrence to see if it needs to be learned
	needsLearning := r.findOccurrencesNeedingLearning(allWordContexts, word)
	fmt.Printf("Found %d occurrences that need to be learned\n", len(needsLearning))
	if len(needsLearning) == 0 {
		// No occurrences needed learning - all are already mastered
		fmt.Println("All occurrences of this word have already been mastered!")
		return nil
	}

	// Collect contexts from each occurrence that needs learning
	// Strip {{ }} markers from contexts before sending to inference API
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

	// Validate meaning against all context groups in a single API call
	results, err := r.openaiClient.AnswerMeanings(ctx, inference.AnswerMeaningsRequest{
		Expressions: []inference.Expression{
			{
				Expression:        word,
				Meaning:           meaning,
				Contexts:          contexts,
				IsExpressionInput: true, // FreeformQuiz: user inputs the expression
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

	// Build a map from context string to occurrence index
	contextToOccurrence := make(map[string]int)
	for i, occurrence := range needsLearning {
		for _, ctx := range occurrence.Contexts {
			contextToOccurrence[ctx.Context] = i
		}
	}

	// Check if OpenAI marked any answer as correct
	isCorrect := false
	for _, answer := range result.AnswersForContext {
		if answer.Correct {
			isCorrect = true
			break
		}
	}

	// Find the first occurrence that has at least one correct context
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

	// Update learning history when answer is correct
	if isCorrect {
		// Use the occurrence that matched the correct context if found, otherwise use the first occurrence
		occurrenceToUpdate := displayOccurrence
		if firstCorrectOccurrenceIdx >= 0 {
			occurrenceToUpdate = needsLearning[firstCorrectOccurrenceIdx]
		}
		if occurrenceToUpdate != nil {
			if err := r.updateLearningHistory(occurrenceToUpdate, word, answer); err != nil {
				return err
			}
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
					// If status is understood, usable, or intuitive, it's been answered correctly
					if status == notebook.LearnedStatus("understood") ||
						status == notebook.LearnedStatus("usable") ||
						status == notebook.LearnedStatus("intuitive") {
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
					// If status is understood, usable, or intuitive, it's been answered correctly
					if status == notebook.LearnedStatus("understood") ||
						status == notebook.LearnedStatus("usable") ||
						status == notebook.LearnedStatus("intuitive") {
						return true
					}
				}
			}
		}
	}
	return false
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
) error {
	learningHistory, ok := r.learningHistories[occurrence.NotebookName]
	if !ok {
		learningHistory = []notebook.LearningHistory{}
	}

	// Record what the user actually practiced
	expressionToRecord := occurrence.Definition.Expression
	if occurrence.Definition.Definition != "" && strings.EqualFold(word, occurrence.Definition.Definition) {
		// User practiced the base form (definition), so record that
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

	// Update learning history with "usable" status for correct answers
	var err error
	learningHistory, err = r.InteractiveQuizCLI.updateLearningHistory(
		occurrence.NotebookName,
		learningHistory,
		occurrence.NotebookName,
		storyTitle,
		sceneTitle,
		expressionToRecord,
		answer.Correct,
		false, // isKnownWord=false to get "usable" status when correct
		false, // alwaysRecord=false for freeform quiz
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
