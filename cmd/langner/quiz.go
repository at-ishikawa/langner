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
				cfg.Notebooks.StoriesDirectory,
				cfg.Notebooks.FlashcardsDirectory,
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

	command := &cobra.Command{
		Use:   "notebook <notebook-id>",
		Short: "Quiz from a specific notebook (shows word, you provide meaning)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			loader, err := config.NewConfigLoader(configFile)
			if err != nil {
				return fmt.Errorf("failed to create config loader: %w", err)
			}
			cfg, err := loader.Load()
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			notebookID := args[0]

			// Create a reader to detect notebook type
			reader, err := notebook.NewReader(cfg.Notebooks.StoriesDirectory, cfg.Notebooks.FlashcardsDirectory, nil)
			if err != nil {
				return fmt.Errorf("failed to create notebook reader: %w", err)
			}

			// Check if notebook exists in story or flashcard indexes
			_, isStory := reader.GetStoryIndexes()[notebookID]
			_, isFlashcard := reader.GetFlashcardIndexes()[notebookID]

			if !isStory && !isFlashcard {
				return fmt.Errorf("notebook %q not found in stories or flashcards", notebookID)
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

			// Create interactive CLI based on detected type
			if isFlashcard {
				flashcardCLI, err := cli.NewFlashcardQuizCLI(
					notebookID,
					cfg.Notebooks.FlashcardsDirectory,
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
				notebookID,
				cfg.Notebooks.StoriesDirectory,
				cfg.Notebooks.LearningNotesDirectory,
				cfg.Dictionaries.RapidAPI.CacheDirectory,
				openaiClient,
				includeNoCorrectAnswers,
			)
			if err != nil {
				return err
			}
			notebookCLI.ShuffleCards()
			fmt.Printf("Starting story Q&A session with %d cards\n\n", notebookCLI.GetCardCount())

			return notebookCLI.Run(context.Background(), notebookCLI)
		},
	}

	command.Flags().BoolVar(&includeNoCorrectAnswers, "include-no-correct-answers", false, "Include words that have never had a correct answer")

	return command
}
