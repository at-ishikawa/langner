package cli

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/fatih/color"
)

// FlashcardQuizCLI manages the interactive CLI session for flashcard notebooks
type FlashcardQuizCLI struct {
	*InteractiveQuizCLI
	notebookName string
	notebooks    []notebook.FlashcardNotebook
	cards        []*WordOccurrence
}

// newFlashcardInteractiveQuizCLI creates the base CLI with shared initialization for flashcards
func newFlashcardInteractiveQuizCLI(
	flashcardsDir string,
	learningNotesDir string,
	dictionaryCacheDir string,
	openaiClient inference.Client,
) (*InteractiveQuizCLI, *notebook.Reader, error) {
	return initializeQuizCLI("", flashcardsDir, learningNotesDir, dictionaryCacheDir, openaiClient)
}

// NewFlashcardQuizCLI creates a new flashcard quiz interactive CLI
func NewFlashcardQuizCLI(
	notebookName string,
	flashcardsDir string,
	learningNotesDir string,
	dictionaryCacheDir string,
	openaiClient inference.Client,
) (*FlashcardQuizCLI, error) {
	// Initialize base CLI for flashcards
	baseCLI, reader, err := newFlashcardInteractiveQuizCLI(flashcardsDir, learningNotesDir, dictionaryCacheDir, openaiClient)
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
	return &FlashcardQuizCLI{
		InteractiveQuizCLI: baseCLI,
		notebookName:       notebookName,
		notebooks:          notebooks,
		cards:              cards,
	}, nil
}

// ShuffleCards shuffles the QA cards
func (r *FlashcardQuizCLI) ShuffleCards() {
	rand.Shuffle(len(r.cards), func(i, j int) {
		r.cards[i], r.cards[j] = r.cards[j], r.cards[i]
	})
}

// getNextCard returns the next card or nil if no more cards
func (r *FlashcardQuizCLI) getNextCard() *WordOccurrence {
	if len(r.cards) == 0 {
		return nil
	}
	return r.cards[0]
}

// removeCurrentCard removes the current card from the session
func (r *FlashcardQuizCLI) removeCurrentCard() {
	if len(r.cards) > 0 {
		r.cards = r.cards[1:]
	}
}

// GetCardCount returns the number of remaining cards
func (r *FlashcardQuizCLI) GetCardCount() int {
	return len(r.cards)
}

func (r *FlashcardQuizCLI) Session(ctx context.Context) error {
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
	cleanContexts := currentCard.GetCleanContexts()
	var contexts []inference.Context
	for i, ctx := range currentCard.Contexts {
		contexts = append(contexts, inference.Context{
			Context:             cleanContexts[i],
			ReferenceDefinition: currentCard.Definition.Meaning,
			Usage:               ctx.Usage,
		})
	}

	results, err := r.openaiClient.AnswerMeanings(ctx, inference.AnswerMeaningsRequest{
		Expressions: []inference.Expression{
			{
				Expression:        currentCard.GetExpression(),
				Meaning:           userAnswer,
				Contexts:          contexts,
				IsExpressionInput: false, // FlashcardQuiz: user inputs the meaning
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

	// Update learning history - flashcard quiz always records even if status doesn't change
	// For flashcards, we use a generic scene title since there are no scenes
	wordToRecord := currentCard.GetExpression()

	learningHistory, err := r.updateLearningHistory(
		r.notebookName,
		r.learningHistories[r.notebookName],
		r.notebookName,
		"flashcards", // Generic story title for flashcards
		"",           // No scene for flashcards
		wordToRecord,
		answer.Correct,
		true, // flashcard quiz always marks correct answers as known words
		true, // flashcard quiz always records even if status doesn't change
	)
	if err != nil {
		return err
	}

	r.learningHistories[r.notebookName] = learningHistory

	// Remove the card from the session
	r.removeCurrentCard()
	return nil
}
