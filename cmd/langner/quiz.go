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
	quizCommand.AddCommand(newQuizReverseCommand())

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

	command := &cobra.Command{
		Use:   "notebook",
		Short: "Quiz from notebooks (shows word, you provide meaning). By default, quizzes from all story notebooks with learning history",
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
		},
	}

	command.Flags().BoolVar(&includeNoCorrectAnswers, "include-no-correct-answers", false, "Include words that have never had a correct answer")
	command.Flags().StringVarP(&notebookName, "notebook", "n", "", "Quiz from a specific notebook (empty for all story notebooks)")

	return command
}

func newQuizReverseCommand() *cobra.Command {
	var notebookName string
	var listMissingContext bool

	command := &cobra.Command{
		Use:   "reverse",
		Short: "Reverse quiz (shows meaning, you provide the word). Tests productive vocabulary",
		Long: `Reverse quiz mode shows you the meaning and asks you to provide the word.
This tests productive vocabulary - a fundamentally different cognitive skill from recognition.

The quiz validates your answer using:
1. Exact match with the expected word (case-insensitive)
2. Match with alternate forms (e.g., "ran" for "run")
3. If you provide a synonym, you get one retry to provide the exact word`,
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

			// Create reverse quiz CLI
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
		},
	}

	command.Flags().StringVarP(&notebookName, "notebook", "n", "", "Quiz from a specific notebook (empty for all notebooks)")
	command.Flags().BoolVar(&listMissingContext, "list-missing-context", false, "List words that don't have context sentences for reverse quiz")

	return command
}
