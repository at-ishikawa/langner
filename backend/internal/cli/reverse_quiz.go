package cli

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/fatih/color"
)

// ReverseQuizCLI manages the interactive CLI session for reverse quiz
// (shows meaning, user produces the word)
type ReverseQuizCLI struct {
	*InteractiveQuizCLI
	svc                *quiz.Service
	notebookName       string
	cards              []quiz.ReverseCard
	listMissingContext bool
}

// NewReverseQuizCLI creates a new reverse quiz interactive CLI
func NewReverseQuizCLI(
	notebookName string,
	notebooksConfig config.NotebooksConfig,
	dictionaryCacheDir string,
	openaiClient inference.Client,
	listMissingContext bool,
	quizCfg config.QuizConfig,
) (*ReverseQuizCLI, error) {
	baseCLI, reader, err := initializeQuizCLI(notebooksConfig, dictionaryCacheDir, openaiClient)
	if err != nil {
		return nil, err
	}

	calculator := notebook.NewIntervalCalculator(quizCfg.Algorithm, quizCfg.ExponentialBase)
	svc := quiz.NewService(notebooksConfig, openaiClient, baseCLI.dictionaryMap, learning.NewYAMLLearningRepository(notebooksConfig.LearningNotesDirectory, calculator), quizCfg)

	var notebookIDs []string
	if notebookName == "" {
		// Collect all notebook IDs that have learning history
		for notebookID := range reader.GetStoryIndexes() {
			if _, ok := baseCLI.learningHistories[notebookID]; ok {
				notebookIDs = append(notebookIDs, notebookID)
			}
		}
		for notebookID := range reader.GetFlashcardIndexes() {
			if _, ok := baseCLI.learningHistories[notebookID]; ok {
				notebookIDs = append(notebookIDs, notebookID)
			}
		}
	} else {
		notebookIDs = []string{notebookName}
	}

	cards, err := svc.LoadReverseCards(notebookIDs, listMissingContext)
	if err != nil {
		return nil, fmt.Errorf("failed to load reverse cards: %w", err)
	}

	cards = sortReverseCardsByContextAvailability(cards)

	return &ReverseQuizCLI{
		InteractiveQuizCLI: baseCLI,
		svc:                svc,
		notebookName:       notebookName,
		cards:              cards,
		listMissingContext: listMissingContext,
	}, nil
}

// sortReverseCardsByContextAvailability sorts cards so that words without context come first.
func sortReverseCardsByContextAvailability(cards []quiz.ReverseCard) []quiz.ReverseCard {
	var withoutContext, withContext []quiz.ReverseCard
	for _, card := range cards {
		if len(card.Contexts) == 0 {
			withoutContext = append(withoutContext, card)
		} else {
			withContext = append(withContext, card)
		}
	}

	result := make([]quiz.ReverseCard, 0, len(cards))
	result = append(result, withoutContext...)
	result = append(result, withContext...)
	return result
}

// ShuffleCards shuffles the quiz cards within each context-availability group,
// preserving the partition where cards without context come first.
func (r *ReverseQuizCLI) ShuffleCards() {
	boundary := 0
	for boundary < len(r.cards) && len(r.cards[boundary].Contexts) == 0 {
		boundary++
	}

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
func (r *ReverseQuizCLI) getNextCard() *quiz.ReverseCard {
	if len(r.cards) == 0 {
		return nil
	}
	return &r.cards[0]
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

	question := FormatReverseQuestion(currentCard)
	fmt.Print(question)
	_, _ = r.bold.Printf("Word: ")

	userAnswer, err := r.stdinReader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading input: %w", err)
	}
	userAnswer = strings.TrimSpace(userAnswer)

	responseTimeMs := time.Since(startTime).Milliseconds()

	grade, err := r.gradeWithSynonymRetry(ctx, currentCard, userAnswer, responseTimeMs, false)
	if err != nil {
		return fmt.Errorf("grade answer: %w", err)
	}

	r.displayReverseResult(currentCard, userAnswer, grade)

	if err := r.svc.SaveReverseResult(ctx, *currentCard, grade, responseTimeMs); err != nil {
		return err
	}

	r.removeCurrentCard()
	return nil
}

// gradeWithSynonymRetry grades the answer using the service and handles synonym retry (CLI-specific).
func (r *ReverseQuizCLI) gradeWithSynonymRetry(
	ctx context.Context,
	card *quiz.ReverseCard,
	userAnswer string,
	responseTimeMs int64,
	isRetry bool,
) (quiz.GradeResult, error) {
	if userAnswer == "" {
		return quiz.GradeResult{
			Correct: false,
			Reason:  "empty answer",
			Quality: int(notebook.QualityWrong),
		}, nil
	}

	grade, err := r.svc.GradeReverseAnswer(ctx, *card, userAnswer, responseTimeMs)
	if err != nil {
		return quiz.GradeResult{
			Correct: false,
			Reason:  fmt.Sprintf("validation error: %v", err),
			Quality: int(notebook.QualityWrong),
		}, nil
	}

	if isRetry {
		grade.Quality = int(notebook.QualityCorrectSlow)
	}

	switch grade.Classification {
	case string(inference.ClassificationSynonym):
		if !isRetry {
			fmt.Printf("\n%s That's a valid synonym! But we're looking for a specific word.\n", r.bold.Sprint("Hint:"))
			fmt.Printf("Your word \"%s\" means the same thing. Try the exact word we're looking for.\n\n", userAnswer)
			_, _ = r.bold.Printf("Word: ")

			retryAnswer, err := r.stdinReader.ReadString('\n')
			if err != nil {
				return quiz.GradeResult{
					Correct: false,
					Reason:  "error reading retry input",
					Quality: int(notebook.QualityWrong),
				}, nil
			}
			retryAnswer = strings.TrimSpace(retryAnswer)

			retryResponseMs := time.Since(time.Now().Add(-time.Duration(responseTimeMs) * time.Millisecond)).Milliseconds()
			return r.gradeWithSynonymRetry(ctx, card, retryAnswer, retryResponseMs, true)
		}
		// On retry, synonym is still acceptable but with lower quality
		return quiz.GradeResult{
			Correct:        true,
			Reason:         grade.Reason + " (accepted on retry)",
			Quality:        int(notebook.QualityCorrectSlow),
			Classification: grade.Classification,
		}, nil
	}

	return grade, nil
}

// displayReverseResult shows the result of the quiz
func (r *ReverseQuizCLI) displayReverseResult(card *quiz.ReverseCard, userAnswer string, grade quiz.GradeResult) {
	fmt.Println()
	if grade.Correct {
		fmt.Print("\u2705 ")
		color.Green(`Correct! The word is "%s" - %s`,
			r.bold.Sprint(card.Expression),
			r.italic.Sprint(card.Meaning),
		)
	} else {
		fmt.Print("\u274C ")
		color.Red(`Incorrect. The word is "%s" - %s`,
			r.bold.Sprint(card.Expression),
			r.italic.Sprint(card.Meaning),
		)
		fmt.Printf("   You answered: %s\n", userAnswer)
	}
	if grade.Reason != "" {
		fmt.Printf("   Reason: %s\n", grade.Reason)
	}

	if len(card.Contexts) > 0 {
		fmt.Println()
		fmt.Println("   Context:")
		for i, ctx := range card.Contexts {
			fmt.Printf("   %d. %s\n", i+1, ctx.Context)
		}
	}
	fmt.Println()
}

// FormatReverseQuestion formats a reverse quiz question
// Shows meaning and masked context
func FormatReverseQuestion(card *quiz.ReverseCard) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Meaning: %s\n", card.Meaning))

	if len(card.Contexts) > 0 {
		sb.WriteString("Context:\n")
		for i, ctx := range card.Contexts {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, ctx.MaskedContext))
		}
	} else {
		sb.WriteString("Context: (no context available - this word may be difficult to answer)\n")
	}

	sb.WriteString("\n")
	return sb.String()
}

// ListMissingContext returns cards that are missing context
func (r *ReverseQuizCLI) ListMissingContext() {
	if len(r.cards) == 0 {
		fmt.Println("No words with missing context found.")
		return
	}

	byNotebook := make(map[string][]quiz.ReverseCard)
	for _, card := range r.cards {
		byNotebook[card.NotebookName] = append(byNotebook[card.NotebookName], card)
	}

	fmt.Printf("Found %d words with missing context:\n\n", len(r.cards))

	for notebookName, cards := range byNotebook {
		fmt.Printf("%s (%d words):\n", r.bold.Sprint(notebookName), len(cards))
		for _, card := range cards {
			fmt.Printf("  - %s: %s\n", card.Expression, card.Meaning)
		}
		fmt.Println()
	}
}
