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
	allStories map[string][]notebook.StoryNotebook
}

// NewFreeformQuizCLI creates a new freeform quiz interactive CLI
func NewFreeformQuizCLI(
	storiesDir string,
	learningNotesDir string,
	dictionaryCacheDir string,
	openaiClient inference.Client,
) (*FreeformQuizCLI, error) {
	// Initialize base CLI
	baseCLI, reader, err := newInteractiveQuizCLI(storiesDir, learningNotesDir, dictionaryCacheDir, openaiClient)
	if err != nil {
		return nil, err
	}

	// Read all story notebooks for context lookup
	allStories := make(map[string][]notebook.StoryNotebook)
	for notebookName := range baseCLI.learningHistories {
		stories, err := reader.ReadStoryNotebooks(notebookName)
		if err != nil {
			fmt.Printf("Warning: could not read stories for %s: %v\n", notebookName, err)
			continue
		}
		allStories[notebookName] = stories
	}

	return &FreeformQuizCLI{
		InteractiveQuizCLI: baseCLI,
		allStories:         allStories,
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
	contextGroups := make([][]string, 0, len(needsLearning))
	for _, occurrence := range needsLearning {
		contextGroups = append(contextGroups, occurrence.Contexts)
	}

	// Validate meaning against all context groups in a single API call
	result, err := r.openaiClient.AnswerExpressionWithMultipleContexts(ctx, inference.AnswerExpressionWithMultipleContextsParams{
		Expression:        word,
		Meaning:           meaning,
		Contexts:          contextGroups,
		IsExpressionInput: true, // FreeformQuiz: user inputs the expression
	})
	if err != nil {
		return fmt.Errorf("openaiClient.AnswerExpressionWithMultipleContexts() > %w", err)
	}

	// Build a map from context string to occurrence index
	contextToOccurrence := make(map[string]int)
	for i, occurrence := range needsLearning {
		for _, ctx := range occurrence.Contexts {
			contextToOccurrence[ctx] = i
		}
	}

	// Find the first occurrence that has at least one correct context
	var firstCorrectOccurrenceIdx = -1
	var matchingContext string
	for _, answer := range result.AnswersForContext {
		if answer.Correct {
			if occIdx, found := contextToOccurrence[answer.Context]; found {
				firstCorrectOccurrenceIdx = occIdx
				matchingContext = answer.Context
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
			matchingContext = displayOccurrence.Contexts[0]
		}
	}

	// Build answer for display
	answer := AnswerResponse{
		Correct:    firstCorrectOccurrenceIdx >= 0,
		Expression: result.Expression,
		Meaning:    result.Meaning,
		Context:    matchingContext,
	}

	// Show result
	if err := r.displayResult(answer, displayOccurrence); err != nil {
		return err
	}

	// Update learning history only when answer is correct
	if firstCorrectOccurrenceIdx >= 0 {
		if err := r.updateLearningHistory(needsLearning[firstCorrectOccurrenceIdx], word, answer); err != nil {
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
	for _, hist := range learningHistory {
		if hist.Metadata.Title != wordCtx.Story.Event {
			continue
		}

		logs := hist.GetLogs(wordCtx.Story.Event, wordCtx.Scene.Title, *wordCtx.Definition)
		if len(logs) == 0 {
			continue
		}

		// Check the latest status
		for _, scene := range hist.Scenes {
			if scene.Metadata.Title != wordCtx.Scene.Title {
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

	// Show the matching context if available
	if answer.Context != "" && occurrence != nil {
		if _, err := fmt.Fprintln(r.stdoutWriter); err != nil {
			return fmt.Errorf("failed to write to stdout: %w", err)
		}
		// Convert markers to show only the target expression in bold
		convertedContext := notebook.ConvertMarkersInText(
			answer.Context,
			occurrence.Scene.Definitions,
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

	// Update learning history with "usable" status for correct answers
	var err error
	learningHistory, err = r.InteractiveQuizCLI.updateLearningHistory(
		occurrence.NotebookName,
		learningHistory,
		occurrence.NotebookName,
		occurrence.Story.Event,
		occurrence.Scene.Title,
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

// findAllWordContexts searches for ALL occurrences of a word across all story notebooks
func (r *FreeformQuizCLI) findAllWordContexts(word string) []*WordOccurrence {
	var allContexts []*WordOccurrence

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
					contexts := extractContextsFromConversations(scene, word, definition.Expression)
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
