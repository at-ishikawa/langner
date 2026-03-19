package cli

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/fatih/color"
)

// NotebookQuizCLI manages the interactive CLI session for a specific notebook
// Handles both story notebooks and flashcard notebooks
type NotebookQuizCLI struct {
	*InteractiveQuizCLI
	svc          *quiz.Service
	notebookName string
	cards        []quiz.Card
}

// NewNotebookQuizCLI creates a new notebook quiz interactive CLI for story notebooks
func NewNotebookQuizCLI(
	notebookName string,
	notebooksConfig config.NotebooksConfig,
	dictionaryCacheDir string,
	openaiClient inference.Client,
	includeNoCorrectAnswers bool,
) (*NotebookQuizCLI, error) {
	// Initialize base CLI
	baseCLI, reader, err := initializeQuizCLI(notebooksConfig, dictionaryCacheDir, openaiClient)
	if err != nil {
		return nil, err
	}

	svc := quiz.NewService(notebooksConfig, openaiClient, baseCLI.dictionaryMap, learning.NewYAMLLearningRepository(notebooksConfig.LearningNotesDirectory))

	var cards []quiz.Card

	// If notebookName is empty, load all notebooks with learning history
	if notebookName == "" {
		allNotebooks, err := reader.ReadAllStoryNotebooksMap()
		if err != nil {
			return nil, fmt.Errorf("failed to load story notebooks: %w", err)
		}

		var notebookIDs []string
		for notebookID := range allNotebooks {
			if _, ok := baseCLI.learningHistories[notebookID]; ok {
				notebookIDs = append(notebookIDs, notebookID)
			}
		}

		if len(notebookIDs) > 0 {
			cards, err = svc.LoadCards(notebookIDs, includeNoCorrectAnswers)
			if err != nil {
				return nil, fmt.Errorf("failed to load quiz cards: %w", err)
			}
		}
	} else {
		// Keep the "no learning note" check for backwards compatibility
		_, ok := baseCLI.learningHistories[notebookName]
		if !ok {
			return nil, fmt.Errorf("no learning note for %s hasn't been supported yet", notebookName)
		}

		cards, err = svc.LoadCards([]string{notebookName}, includeNoCorrectAnswers)
		if err != nil {
			return nil, fmt.Errorf("failed to load quiz cards: %w", err)
		}
	}

	return &NotebookQuizCLI{
		InteractiveQuizCLI: baseCLI,
		svc:                svc,
		notebookName:       notebookName,
		cards:              cards,
	}, nil
}

// NewFlashcardQuizCLI creates a new notebook quiz interactive CLI for flashcard notebooks
func NewFlashcardQuizCLI(
	notebookName string,
	notebooksConfig config.NotebooksConfig,
	dictionaryCacheDir string,
	openaiClient inference.Client,
) (*NotebookQuizCLI, error) {
	// Initialize base CLI for flashcards
	baseCLI, _, err := initializeQuizCLI(notebooksConfig, dictionaryCacheDir, openaiClient)
	if err != nil {
		return nil, err
	}

	// Keep the "no learning note" check for backwards compatibility
	_, ok := baseCLI.learningHistories[notebookName]
	if !ok {
		return nil, fmt.Errorf("no learning note for %s hasn't been supported yet", notebookName)
	}

	svc := quiz.NewService(notebooksConfig, openaiClient, baseCLI.dictionaryMap, learning.NewYAMLLearningRepository(notebooksConfig.LearningNotesDirectory))

	cards, err := svc.LoadCards([]string{notebookName}, false)
	if err != nil {
		return nil, fmt.Errorf("failed to load quiz cards: %w", err)
	}

	return &NotebookQuizCLI{
		InteractiveQuizCLI: baseCLI,
		svc:                svc,
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
func (r *NotebookQuizCLI) getNextCard() *quiz.Card {
	if len(r.cards) == 0 {
		return nil
	}
	return &r.cards[0]
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

	startTime := time.Now()

	question := FormatQuestion(currentCard)
	fmt.Print(question)
	_, _ = r.bold.Printf("%s: ", currentCard.Entry)

	userAnswer, err := r.stdinReader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading input: %w", err)
	}

	responseTimeMs := time.Since(startTime).Milliseconds()

	grade, err := r.svc.GradeNotebookAnswer(ctx, *currentCard, strings.TrimSpace(userAnswer), responseTimeMs)
	if err != nil {
		return fmt.Errorf("failed to grade answer: %w", err)
	}

	// Override correct flag for empty input
	if strings.TrimSpace(userAnswer) == "" {
		grade.Correct = false
	}

	answer := AnswerResponse{
		Correct:    grade.Correct,
		Expression: currentCard.Entry,
		Meaning:    currentCard.Meaning,
		Reason:     grade.Reason,
	}

	fmt.Printf(`Answer for %s is "%s"`,
		r.bold.Sprintf("%s", currentCard.Entry),
		r.italic.Sprintf("%s", currentCard.Meaning),
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
	fmt.Println()

	if err := r.svc.SaveResult(ctx, *currentCard, grade, responseTimeMs); err != nil {
		return err
	}

	// Remove the card from the session
	r.removeCurrentCard()
	return nil
}

// FormatQuestion formats a question for display
// For flashcards (when card.SceneTitle is empty), it shows a simpler question without scene context
func FormatQuestion(card *quiz.Card) string {
	expression := card.Entry
	if len(card.Examples) == 0 {
		return fmt.Sprintf("What does '%s' mean?\n", expression)
	}

	// Flashcard: no scene title
	if card.SceneTitle == "" {
		question := fmt.Sprintf("What does '%s' mean?\n", expression)
		question += "Examples:\n"
		for i, ex := range card.Examples {
			question += fmt.Sprintf("  %d. %s\n", i+1, ex.Text)
		}
		return question
	}

	// Story notebook: show context
	question := fmt.Sprintf("What does '%s' mean in the following context?\n", expression)
	for i, ex := range card.Examples {
		question += fmt.Sprintf("  %d. %s\n", i+1, ex.Text)
	}
	return question
}
