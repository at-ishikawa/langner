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
type NotebookQuizCLI struct {
	*InteractiveQuizCLI
	notebookName string
	stories      []notebook.StoryNotebook
	cards        []*WordOccurrence
}

// NewNotebookQuizCLI creates a new notebook quiz interactive CLI
func NewNotebookQuizCLI(
	notebookName string,
	storiesDir string,
	learningNotesDir string,
	dictionaryCacheDir string,
	openaiClient inference.Client,
) (*NotebookQuizCLI, error) {
	// Initialize base CLI
	baseCLI, reader, err := newInteractiveQuizCLI(storiesDir, learningNotesDir, dictionaryCacheDir, openaiClient)
	if err != nil {
		return nil, err
	}

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
	stories, err = notebook.FilterStoryNotebooks(stories, learningHistory, baseCLI.dictionaryMap, false, true)
	if err != nil {
		return nil, fmt.Errorf("notebook.FilterStoryNotebooks > %w", err)
	}

	cards := extractWordOccurrences(notebookName, stories)
	return &NotebookQuizCLI{
		InteractiveQuizCLI: baseCLI,
		notebookName:       notebookName,
		stories:            stories,
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

	result, err := r.openaiClient.AnswerExpressionWithSingleContext(ctx, inference.AnswerExpressionWithSingleContextParams{
		Expression:        currentCard.GetExpression(),
		Meaning:           userAnswer,
		Statements:        currentCard.Contexts,
		IsExpressionInput: false, // NotebookQuiz: user inputs the meaning
	})
	if err != nil {
		return fmt.Errorf("openaiClient.AnswerExpressionWithSingleContext() > %w", err)
	}

	answer := AnswerResponse{
		Correct:    result.Correct,
		Expression: result.Expression,
		Meaning:    result.Meaning,
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

	if len(currentCard.GetImages()) > 0 {
		fmt.Printf("\nImages: %+v\n", currentCard.GetImages())
	}
	fmt.Println()

	// Update learning history - QA always records even if status doesn't change
	// Use definition if available, otherwise use expression
	wordToRecord := currentCard.GetExpression()

	learningHistory, err := r.updateLearningHistory(
		r.notebookName,
		r.learningHistories[r.notebookName],
		r.notebookName,
		currentCard.Story.Event,
		currentCard.Scene.Title,
		wordToRecord,
		answer.Correct,
		true, // qa command always marks correct answers as known words
		true, // qa command always records even if status doesn't change
	)
	if err != nil {
		return err
	}

	r.learningHistories[r.notebookName] = learningHistory

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
func FormatQuestion(occurrence *WordOccurrence) string {
	expression := occurrence.GetExpression()
	if len(occurrence.Contexts) == 0 {
		return fmt.Sprintf("What does '%s' mean?\n", expression)
	}

	question := fmt.Sprintf("What does '%s' mean in the following context?\n", expression)

	// Only convert markers if we have scene definitions
	if occurrence.Scene != nil && len(occurrence.Scene.Definitions) > 0 {
		for i, context := range occurrence.Contexts {
			// Convert only the specific expression to bold terminal text
			convertedContext := notebook.ConvertMarkersInText(
				context,
				occurrence.Scene.Definitions,
				notebook.ConversionStyleTerminal,
				occurrence.Definition.Expression,
			)
			question += fmt.Sprintf("  %d. %s\n", i+1, convertedContext)
		}
	} else {
		// No scene definitions, just output contexts as-is
		for i, context := range occurrence.Contexts {
			question += fmt.Sprintf("  %d. %s\n", i+1, context)
		}
	}

	return question
}
