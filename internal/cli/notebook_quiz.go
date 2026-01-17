package cli

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/fatih/color"
)

// NotebookQuizCLI manages the interactive CLI session for a specific notebook
// Handles both story notebooks and flashcard notebooks
type NotebookQuizCLI struct {
	*InteractiveQuizCLI
	notebookName string
	cards        []*WordOccurrence
}

// NewNotebookQuizCLI creates a new notebook quiz interactive CLI for story notebooks
func NewNotebookQuizCLI(
	notebookName string,
	storiesDirs []string,
	learningNotesDir string,
	dictionaryCacheDir string,
	openaiClient inference.Client,
	includeNoCorrectAnswers bool,
) (*NotebookQuizCLI, error) {
	// Initialize base CLI
	baseCLI, reader, err := newInteractiveQuizCLI(storiesDirs, learningNotesDir, dictionaryCacheDir, openaiClient)
	if err != nil {
		return nil, err
	}

	var cards []*WordOccurrence

	// If notebookName is empty, load all notebooks
	if notebookName == "" {
		allNotebooks, err := reader.ReadAllStoryNotebooksMap()
		if err != nil {
			return nil, fmt.Errorf("ReadAllStoryNotebooksMap() > %w", err)
		}

		for notebookID, stories := range allNotebooks {
			learningHistory, ok := baseCLI.learningHistories[notebookID]
			if !ok {
				// Skip notebooks without learning history
				continue
			}

			filteredStories, err := notebook.FilterStoryNotebooks(stories, learningHistory, baseCLI.dictionaryMap, false, false, includeNoCorrectAnswers)
			if err != nil {
				return nil, fmt.Errorf("notebook.FilterStoryNotebooks > %w", err)
			}

			notebookCards := extractWordOccurrences(notebookID, filteredStories)
			cards = append(cards, notebookCards...)
		}
	} else {
		// Load stories for the specific notebook
		stories, err := reader.ReadStoryNotebooks(notebookName)
		if err != nil {
			return nil, fmt.Errorf("ReadStoryNotebooks(%s) > %w", notebookName, err)
		}

		// Get learning history for this notebook
		learningHistory, ok := baseCLI.learningHistories[notebookName]
		if !ok {
			return nil, fmt.Errorf("no learning note for %s hasn't been supported yet", notebookName)
		}

		// Filter stories based on learning history (without conversion)
		stories, err = notebook.FilterStoryNotebooks(stories, learningHistory, baseCLI.dictionaryMap, false, false, includeNoCorrectAnswers)
		if err != nil {
			return nil, fmt.Errorf("notebook.FilterStoryNotebooks > %w", err)
		}

		cards = extractWordOccurrences(notebookName, stories)
	}

	return &NotebookQuizCLI{
		InteractiveQuizCLI: baseCLI,
		notebookName:       notebookName,
		cards:              cards,
	}, nil
}

// NewFlashcardQuizCLI creates a new notebook quiz interactive CLI for flashcard notebooks
func NewFlashcardQuizCLI(
	notebookName string,
	flashcardsDirs []string,
	learningNotesDir string,
	dictionaryCacheDir string,
	openaiClient inference.Client,
) (*NotebookQuizCLI, error) {
	// Initialize base CLI for flashcards
	baseCLI, reader, err := initializeQuizCLI(nil, flashcardsDirs, learningNotesDir, dictionaryCacheDir, openaiClient)
	if err != nil {
		return nil, err
	}

	// Load flashcard notebooks for the specific notebook
	notebooks, err := reader.ReadFlashcardNotebooks(notebookName)
	if err != nil {
		return nil, fmt.Errorf("ReadFlashcardNotebooks(%s) > %w", notebookName, err)
	}

	// Get learning history for this notebook
	learningHistory, ok := baseCLI.learningHistories[notebookName]
	if !ok {
		return nil, fmt.Errorf("no learning note for %s hasn't been supported yet", notebookName)
	}

	// Filter notebooks based on learning history
	notebooks, err = notebook.FilterFlashcardNotebooks(notebooks, learningHistory, baseCLI.dictionaryMap, false, 0)
	if err != nil {
		return nil, fmt.Errorf("notebook.FilterFlashcardNotebooks > %w", err)
	}

	cards := extractWordOccurrencesFromFlashcards(notebookName, notebooks)
	return &NotebookQuizCLI{
		InteractiveQuizCLI: baseCLI,
		notebookName:       notebookName,
		cards:              cards,
	}, nil
}

// ShuffleCards shuffles the QA cards
func (r *NotebookQuizCLI) ShuffleCards() {
	rand.Shuffle(len(r.cards), func(i, j int) {
		r.cards[i], r.cards[j] = r.cards[j], r.cards[i]
	})
}

// getNextCard returns the next card or nil if no more cards
func (r *NotebookQuizCLI) getNextCard() *WordOccurrence {
	if len(r.cards) == 0 {
		return nil
	}
	return r.cards[0]
}

// removeCurrentCard removes the current card from the session
func (r *NotebookQuizCLI) removeCurrentCard() {
	if len(r.cards) > 0 {
		r.cards = r.cards[1:]
	}
}

// GetCardCount returns the number of remaining cards
func (r *NotebookQuizCLI) GetCardCount() int {
	return len(r.cards)
}

var (
	errEnd = errors.New("end")
)

func (r *NotebookQuizCLI) Session(ctx context.Context) error {
	currentCard := r.getNextCard()
	if currentCard == nil {
		fmt.Println("No more cards to practice!")
		return errEnd
	}

	// Display the question
	question := FormatQuestion(currentCard)
	fmt.Print(question)
	_, _ = r.bold.Printf("%s: ", currentCard.GetExpression())

	userAnswer, err := r.stdinReader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading input: %w", err)
	}

	// Convert contexts to Context structs with meanings and usage
	// Strip {{ }} markers from contexts before sending to inference API
	cleanContexts := currentCard.GetCleanContexts()
	var contexts []inference.Context
	for i, ctx := range currentCard.Contexts {
		contexts = append(contexts, inference.Context{
			Context:             cleanContexts[i],                // Use cleaned context without markers
			ReferenceDefinition: currentCard.Definition.Meaning, // Include meaning from notebook as hint
			Usage:               ctx.Usage,                      // Include the actual form used in context
		})
	}

	results, err := r.openaiClient.AnswerMeanings(ctx, inference.AnswerMeaningsRequest{
		Expressions: []inference.Expression{
			{
				Expression:        currentCard.GetExpression(),
				Meaning:           userAnswer,
				Contexts:          contexts,
				IsExpressionInput: false, // NotebookQuiz: user inputs the meaning
			},
		},
	})
	if err != nil {
		return fmt.Errorf("openaiClient.AnswerQuestions() > %w", err)
	}
	if len(results.Answers) == 0 {
		return fmt.Errorf("no results returned from OpenAI")
	}
	result := results.Answers[0]

	isCorrect := len(result.AnswersForContext) > 0 && result.AnswersForContext[0].Correct
	reason := ""
	if len(result.AnswersForContext) > 0 {
		reason = result.AnswersForContext[0].Reason
	}
	answer := AnswerResponse{
		Correct:    isCorrect,
		Expression: result.Expression,
		Meaning:    result.Meaning,
		Reason:     reason,
	}

	if strings.TrimSpace(userAnswer) == "" {
		answer.Correct = false
	}

	fmt.Printf(`Answer for %s is "%s"`,
		r.bold.Sprintf("%s", currentCard.GetExpression()),
		r.italic.Sprintf("%s", currentCard.GetMeaning()),
	)
	fmt.Println()

	if answer.Correct {
		fmt.Print("\u2705 ")
		color.Green(`It's correct. The meaning of %s is "%s"`,
			r.bold.Sprintf("%s", answer.Expression),
			r.italic.Sprintf("%s", answer.Meaning),
		)
	} else {
		fmt.Print("\u274C ")
		color.Red(`It's wrong. The meaning of %s is "%s"`,
			r.bold.Sprintf("%s", answer.Expression),
			r.italic.Sprintf("%s", answer.Meaning),
		)
	}
	if answer.Reason != "" {
		fmt.Printf("   Reason: %s\n", answer.Reason)
	}

	if len(currentCard.GetImages()) > 0 {
		fmt.Printf("\nImages: %+v\n", currentCard.GetImages())
	}
	fmt.Println()

	// Update learning history - QA always records even if status doesn't change
	// Use definition if available, otherwise use expression
	wordToRecord := currentCard.GetExpression()

	// Use the card's notebook name (important when loading all notebooks)
	cardNotebookName := currentCard.NotebookName

	// For flashcards (Story is nil), use generic story/scene titles
	storyTitle := "flashcards"
	sceneTitle := ""
	if currentCard.Story != nil {
		storyTitle = currentCard.Story.Event
		sceneTitle = currentCard.Scene.Title
	}

	learningHistory, err := r.updateLearningHistory(
		cardNotebookName,
		r.learningHistories[cardNotebookName],
		cardNotebookName,
		storyTitle,
		sceneTitle,
		wordToRecord,
		answer.Correct,
		true, // qa command always marks correct answers as known words
	)
	if err != nil {
		return err
	}

	r.learningHistories[cardNotebookName] = learningHistory

	// Remove the card from the session
	r.removeCurrentCard()
	return nil
}

// extractWordOccurrences extracts all word occurrences from stories
func extractWordOccurrences(notebookName string, stories []notebook.StoryNotebook) []*WordOccurrence {
	var occurrences []*WordOccurrence

	for i := range stories {
		story := &stories[i]
		for j := range story.Scenes {
			scene := &story.Scenes[j]
			for k := range scene.Definitions {
				definition := &scene.Definitions[k]
				// Extract contexts from conversations that contain the expression
				// Words without contexts are still included for learning
				contexts := extractContextsFromConversations(scene, definition.Expression, definition.Definition)

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

// FormatQuestion formats a question for display
// It converts {{ }} markers for the specific expression being asked
// For flashcards (when occurrence.Scene is nil), it shows a simpler question without scene context
func FormatQuestion(occurrence *WordOccurrence) string {
	expression := occurrence.GetExpression()
	if len(occurrence.Contexts) == 0 {
		return fmt.Sprintf("What does '%s' mean?\n", expression)
	}

	// For flashcards (no scene), show a simpler question with examples
	if occurrence.Scene == nil {
		question := fmt.Sprintf("What does '%s' mean?\n", expression)
		if len(occurrence.Contexts) > 0 {
			question += "Examples:\n"
			for i, context := range occurrence.Contexts {
				question += fmt.Sprintf("  %d. %s\n", i+1, context)
			}
		}
		return question
	}

	// For story notebooks, show context-based question
	question := fmt.Sprintf("What does '%s' mean in the following context?\n", expression)

	// Only convert markers if we have scene definitions
	if occurrence.Scene != nil && len(occurrence.Scene.Definitions) > 0 {
		for i, ctx := range occurrence.Contexts {
			// Convert only the specific expression to bold terminal text
			convertedContext := notebook.ConvertMarkersInText(
				ctx.Context,
				occurrence.Scene.Definitions,
				notebook.ConversionStyleTerminal,
				occurrence.Definition.Expression,
			)
			question += fmt.Sprintf("  %d. %s\n", i+1, convertedContext)
		}
	} else {
		// No scene definitions, just output contexts as-is
		for i, ctx := range occurrence.Contexts {
			question += fmt.Sprintf("  %d. %s\n", i+1, ctx.Context)
		}
	}

	return question
}
