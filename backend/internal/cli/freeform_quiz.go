package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quiz"
	"github.com/fatih/color"
)

// FreeformQuizCLI manages the interactive CLI session for freeform quiz
type FreeformQuizCLI struct {
	*InteractiveQuizCLI
	svc           *quiz.Service
	freeformCards []quiz.FreeformCard
}

// NewFreeformQuizCLI creates a new freeform quiz interactive CLI
func NewFreeformQuizCLI(
	notebooksConfig config.NotebooksConfig,
	dictionaryCacheDir string,
	openaiClient inference.Client,
	quizCfg config.QuizConfig,
) (*FreeformQuizCLI, error) {
	baseCLI, _, err := initializeQuizCLI(notebooksConfig, dictionaryCacheDir, openaiClient)
	if err != nil {
		return nil, err
	}

	calculator := notebook.NewIntervalCalculator(quizCfg.Algorithm, quizCfg.ExponentialBase)
	svc := quiz.NewService(notebooksConfig, openaiClient, baseCLI.dictionaryMap, learning.NewYAMLLearningRepository(notebooksConfig.LearningNotesDirectory, calculator), quizCfg)

	cards, err := svc.LoadAllWords()
	if err != nil {
		return nil, fmt.Errorf("failed to load all words: %w", err)
	}

	return &FreeformQuizCLI{
		InteractiveQuizCLI: baseCLI,
		svc:                svc,
		freeformCards:      cards,
	}, nil
}

// WordCount returns the total number of word definitions loaded.
func (r *FreeformQuizCLI) WordCount() int {
	return len(r.freeformCards)
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

	grade, err := r.svc.GradeFreeformAnswer(ctx, word, meaning, responseTimeMs, r.freeformCards)
	if err != nil {
		return fmt.Errorf("grade answer: %w", err)
	}

	r.displayFreeformResult(grade)

	if grade.MatchedCard != nil {
		if err := r.svc.SaveFreeformResult(ctx, *grade.MatchedCard, grade, responseTimeMs); err != nil {
			return err
		}
	}

	fmt.Println()
	return nil
}

func (r *FreeformQuizCLI) displayFreeformResult(grade quiz.FreeformGradeResult) {
	if grade.Correct {
		if _, err := fmt.Fprint(r.stdoutWriter, "✅ "); err != nil {
			return
		}
		green := color.New(color.FgGreen)
		if _, err := green.Fprintf(r.stdoutWriter, `It's correct. The meaning of %s is "%s"`,
			r.bold.Sprintf("%s", grade.Word),
			r.italic.Sprintf("%s", grade.Meaning),
		); err != nil {
			return
		}
		if _, err := fmt.Fprintln(r.stdoutWriter); err != nil {
			return
		}
	} else {
		if _, err := fmt.Fprint(r.stdoutWriter, "❌ "); err != nil {
			return
		}
		red := color.New(color.FgRed)
		if _, err := red.Fprintf(r.stdoutWriter, `It's wrong. The meaning of %s is "%s"`,
			r.bold.Sprintf("%s", grade.Word),
			r.italic.Sprintf("%s", grade.Meaning),
		); err != nil {
			return
		}
		if _, err := fmt.Fprintln(r.stdoutWriter); err != nil {
			return
		}
	}

	if grade.Reason != "" {
		if _, err := fmt.Fprintf(r.stdoutWriter, "   Reason: %s\n", grade.Reason); err != nil {
			return
		}
	}

	if grade.Context != "" {
		if _, err := fmt.Fprintln(r.stdoutWriter); err != nil {
			return
		}
		if _, err := fmt.Fprintf(r.stdoutWriter, "  Context: %s\n", grade.Context); err != nil {
			return
		}
	}
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
