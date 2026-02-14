package main

import (
	"context"
	"fmt"

	"github.com/at-ishikawa/langner/internal/cli"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/inference/openai"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/spf13/cobra"
)

func newQuizCommand() *cobra.Command {
	quizCommand := &cobra.Command{
		Use:   "quiz",
		Short: "Quiz commands for testing vocabulary knowledge",
	}

	quizCommand.AddCommand(newQuizNotebookCommand())
	quizCommand.AddCommand(newQuizFreeformCommand())

	return quizCommand
}

func newQuizFreeformCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "freeform",
		Short: "Freeform quiz where you provide both word and meaning from memory",
		RunE: func(cmd *cobra.Command, args []string) error {
			loader, err := config.NewConfigLoader(configFile)
			if err != nil {
				return fmt.Errorf("failed to create config loader: %w", err)
			}
			cfg, err := loader.Load()
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			// Create OpenAI client
			if cfg.OpenAI.APIKey == "" {
				return fmt.Errorf("OPENAI_API_KEY environment variable is required")
			}
			fmt.Printf("Using OpenAI provider (model: %s)\n", cfg.OpenAI.Model)
			openaiClient := openai.NewClient(cfg.OpenAI.APIKey, cfg.OpenAI.Model, inference.DefaultMaxRetryAttempts)
			defer func() {
				_ = openaiClient.Close()
			}()

			// Create interactive CLI
			freeformCLI, err := cli.NewFreeformQuizCLI(
				cfg.Notebooks.StoriesDirectories,
				cfg.Notebooks.FlashcardsDirectories,
				cfg.Notebooks.LearningNotesDirectory,
				cfg.Dictionaries.RapidAPI.CacheDirectory,
				openaiClient,
			)
			if err != nil {
				return err
			}

			fmt.Println("Interactive word practice session started!")
			fmt.Println("Enter word and meaning pairs. Type 'quit' to exit.")
			fmt.Println()
			return freeformCLI.Run(context.Background(), freeformCLI)
		},
	}

	return command
}

func newQuizNotebookCommand() *cobra.Command {
	var includeNoCorrectAnswers bool
	var notebookName string
	var quizType string
	var listMissingContext bool

	command := &cobra.Command{
		Use:   "notebook",
		Short: "Quiz from notebooks. Use --type to select quiz mode",
		Long: `Quiz from notebooks with different modes:

  recognition (default): Shows word, you provide meaning
  reverse: Shows meaning, you provide the word (tests productive vocabulary)

Examples:
  langner quiz notebook                           # Recognition quiz from all notebooks
  langner quiz notebook --type=reverse            # Reverse quiz from all notebooks
  langner quiz notebook -n friends --type=reverse # Reverse quiz from specific notebook`,
		RunE: func(cmd *cobra.Command, args []string) error {
			loader, err := config.NewConfigLoader(configFile)
			if err != nil {
				return fmt.Errorf("failed to create config loader: %w", err)
			}
			cfg, err := loader.Load()
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			// Create OpenAI client
			if cfg.OpenAI.APIKey == "" {
				return fmt.Errorf("OPENAI_API_KEY environment variable is required")
			}
			fmt.Printf("Using OpenAI provider (model: %s)\n", cfg.OpenAI.Model)
			openaiClient := openai.NewClient(cfg.OpenAI.APIKey, cfg.OpenAI.Model, inference.DefaultMaxRetryAttempts)
			defer func() {
				_ = openaiClient.Close()
			}()

			// Handle reverse quiz mode
			if quizType == "reverse" {
				return runReverseQuiz(cfg, openaiClient, notebookName, listMissingContext)
			}

			// Default: recognition quiz
			return runRecognitionQuiz(cfg, openaiClient, notebookName, includeNoCorrectAnswers)
		},
	}

	command.Flags().BoolVar(&includeNoCorrectAnswers, "include-no-correct-answers", false, "Include words that have never had a correct answer (recognition mode only)")
	command.Flags().StringVarP(&notebookName, "notebook", "n", "", "Quiz from a specific notebook (empty for all notebooks)")
	command.Flags().StringVarP(&quizType, "type", "t", "recognition", "Quiz type: recognition (default) or reverse")
	command.Flags().BoolVar(&listMissingContext, "list-missing-context", false, "List words without context sentences (reverse mode only)")

	return command
}

func runRecognitionQuiz(cfg *config.Config, openaiClient inference.Client, notebookName string, includeNoCorrectAnswers bool) error {
	// If no notebook specified, quiz from all story notebooks
	if notebookName == "" {
		notebookCLI, err := cli.NewNotebookQuizCLI(
			"",
			cfg.Notebooks.StoriesDirectories,
			cfg.Notebooks.LearningNotesDirectory,
			cfg.Dictionaries.RapidAPI.CacheDirectory,
			openaiClient,
			includeNoCorrectAnswers,
		)
		if err != nil {
			return err
		}
		notebookCLI.ShuffleCards()
		fmt.Printf("Starting Q&A session with all notebooks with %d cards\n\n", notebookCLI.GetCardCount())

		return notebookCLI.Run(context.Background(), notebookCLI)
	}

	// Create a reader to detect notebook type
	reader, err := notebook.NewReader(cfg.Notebooks.StoriesDirectories, cfg.Notebooks.FlashcardsDirectories, nil)
	if err != nil {
		return fmt.Errorf("failed to create notebook reader: %w", err)
	}

	// Check if notebook exists in story or flashcard indexes
	_, isStory := reader.GetStoryIndexes()[notebookName]
	_, isFlashcard := reader.GetFlashcardIndexes()[notebookName]

	if !isStory && !isFlashcard {
		return fmt.Errorf("notebook %q not found in stories or flashcards", notebookName)
	}

	// Create interactive CLI based on detected type
	if isFlashcard {
		flashcardCLI, err := cli.NewFlashcardQuizCLI(
			notebookName,
			cfg.Notebooks.FlashcardsDirectories,
			cfg.Notebooks.LearningNotesDirectory,
			cfg.Dictionaries.RapidAPI.CacheDirectory,
			openaiClient,
		)
		if err != nil {
			return err
		}
		flashcardCLI.ShuffleCards()
		fmt.Printf("Starting flashcard Q&A session with %d cards\n\n", flashcardCLI.GetCardCount())

		return flashcardCLI.Run(context.Background(), flashcardCLI)
	}

	// Story notebook
	notebookCLI, err := cli.NewNotebookQuizCLI(
		notebookName,
		cfg.Notebooks.StoriesDirectories,
		cfg.Notebooks.LearningNotesDirectory,
		cfg.Dictionaries.RapidAPI.CacheDirectory,
		openaiClient,
		includeNoCorrectAnswers,
	)
	if err != nil {
		return err
	}
	notebookCLI.ShuffleCards()
	fmt.Printf("Starting Q&A session for notebook %s with %d cards\n\n", notebookName, notebookCLI.GetCardCount())

	return notebookCLI.Run(context.Background(), notebookCLI)
}

func runReverseQuiz(cfg *config.Config, openaiClient inference.Client, notebookName string, listMissingContext bool) error {
	reverseCLI, err := cli.NewReverseQuizCLI(
		notebookName,
		cfg.Notebooks.StoriesDirectories,
		cfg.Notebooks.FlashcardsDirectories,
		cfg.Notebooks.LearningNotesDirectory,
		cfg.Dictionaries.RapidAPI.CacheDirectory,
		openaiClient,
		listMissingContext,
	)
	if err != nil {
		return err
	}

	// If listing missing context, just output and exit
	if listMissingContext {
		reverseCLI.ListMissingContext()
		return nil
	}

	if reverseCLI.GetCardCount() == 0 {
		fmt.Println("No cards need reverse quiz review.")
		return nil
	}

	reverseCLI.ShuffleCards()
	if notebookName == "" {
		fmt.Printf("Starting reverse quiz session with %d cards from all notebooks\n\n", reverseCLI.GetCardCount())
	} else {
		fmt.Printf("Starting reverse quiz session for notebook %s with %d cards\n\n", notebookName, reverseCLI.GetCardCount())
	}

	return reverseCLI.Run(context.Background(), reverseCLI)
}

