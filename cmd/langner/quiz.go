package main

import (
	"context"
	"fmt"

	"github.com/at-ishikawa/langner/internal/cli"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/inference/openai"
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
			cfg, err := config.Load(configFile)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			// Create OpenAI client
			if cfg.OpenAI.APIKey == "" {
				return fmt.Errorf("OPENAI_API_KEY environment variable is required")
			}
			fmt.Printf("Using OpenAI provider (model: %s)\n", cfg.OpenAI.Model)
			openaiClient := openai.NewClient(cfg.OpenAI.APIKey, cfg.OpenAI.Model, inference.DefaultRetryConfig())
			defer func() {
				_ = openaiClient.Close()
			}()

			// Create interactive CLI
			freeformCLI, err := cli.NewFreeformQuizCLI(
				cfg.Notebooks.StoriesDirectory,
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
	command := &cobra.Command{
		Use:   "notebook",
		Short: "Quiz from a specific notebook (shows word, you provide meaning)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configFile)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			notebookName := args[0]

			// Create OpenAI client
			if cfg.OpenAI.APIKey == "" {
				return fmt.Errorf("OPENAI_API_KEY environment variable is required")
			}
			fmt.Printf("Using OpenAI provider (model: %s)\n", cfg.OpenAI.Model)
			openaiClient := openai.NewClient(cfg.OpenAI.APIKey, cfg.OpenAI.Model, inference.DefaultRetryConfig())
			defer func() {
				_ = openaiClient.Close()
			}()

			// Create interactive CLI
			notebookCLI, err := cli.NewNotebookQuizCLI(
				notebookName,
				cfg.Notebooks.StoriesDirectory,
				cfg.Notebooks.LearningNotesDirectory,
				cfg.Dictionaries.RapidAPI.CacheDirectory,
				openaiClient,
			)
			if err != nil {
				return err
			}
			notebookCLI.ShuffleCards()
			fmt.Printf("Starting Q&A session with %d cards\n\n", notebookCLI.GetCardCount())

			return notebookCLI.Run(context.Background(), notebookCLI)
		},
	}

	return command
}
