package main

import (
	"fmt"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type SortFlag string

// Set implements pflag.Value.
func (s *SortFlag) Set(v string) error {
	switch v {
	case string(SortDescending):
		*s = SortDescending
	case string(SortAscending):
		*s = SortAscending
	default:
		return fmt.Errorf("invalid value %q, valid values are %q or %q", v, SortDescending, SortAscending)
	}
	return nil
}

// String implements pflag.Value.
func (s *SortFlag) String() string {
	if s == nil {
		return ""
	}
	return string(*s)
}

// Type implements pflag.Value.
func (s *SortFlag) Type() string {
	return "SortFlag"
}

var (
	_ pflag.Value = (*SortFlag)(nil)
)

const (
	SortDescending SortFlag = "desc"
	SortAscending  SortFlag = "asc"
)

func newNotebookCommand() *cobra.Command {
	notebookCommands := &cobra.Command{
		Use: "notebooks",
	}
	sortFlag := SortDescending
	flags := notebookCommands.PersistentFlags()
	flags.Var(&sortFlag, "sort", "Sort order for the output. Options: asc, desc")

	var generatePDF bool
	storiesCmd := &cobra.Command{
		Use:  "stories <notebook id>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			loader, err := config.NewConfigLoader(configFile)
			if err != nil {
				return fmt.Errorf("failed to create config loader: %w", err)
			}
			cfg, err := loader.Load()
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			storyID := args[0]

			response, err := rapidapi.NewReader().Read(cfg.Dictionaries.RapidAPI.CacheDirectory)
			if err != nil {
				return fmt.Errorf("rapidapi.NewReader().Read() > %w", err)
			}
			dictionaryMap := rapidapi.FromResponsesToMap(response)

			reader, err := notebook.NewReader(cfg.Notebooks.StoriesDirectory, "", dictionaryMap)
			if err != nil {
				return fmt.Errorf("textbook.NewFlashcardReader() > %w", err)
			}
			learningHistories, err := notebook.NewLearningHistories(cfg.Notebooks.LearningNotesDirectory)
			if err != nil {
				return fmt.Errorf("textbook.NewFlashcardReader() > %w", err)
			}

			writer := notebook.NewStoryNotebookWriter(reader, cfg.Templates.StoryNotebookTemplate)
			if err := writer.OutputStoryNotebooks(storyID, dictionaryMap, learningHistories, sortFlag == SortDescending, cfg.Outputs.StoryDirectory, generatePDF); err != nil {
				return fmt.Errorf("notebooks.OutputStoryNotebooks > %w", err)
			}
			return nil
		},
	}
	storiesCmd.Flags().BoolVar(&generatePDF, "pdf", false, "Generate PDF output in addition to markdown")

	notebookCommands.AddCommand(storiesCmd)

	var flashcardGeneratePDF bool
	flashcardsCmd := &cobra.Command{
		Use:   "flashcards <flashcard id>",
		Short: "Generate markdown/PDF output from flashcard notebooks",
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

			flashcardID := args[0]

			response, err := rapidapi.NewReader().Read(cfg.Dictionaries.RapidAPI.CacheDirectory)
			if err != nil {
				return fmt.Errorf("rapidapi.NewReader().Read() > %w", err)
			}
			dictionaryMap := rapidapi.FromResponsesToMap(response)

			reader, err := notebook.NewReader("", cfg.Notebooks.FlashcardsDirectory, dictionaryMap)
			if err != nil {
				return fmt.Errorf("notebook.NewReader() > %w", err)
			}
			learningHistories, err := notebook.NewLearningHistories(cfg.Notebooks.LearningNotesDirectory)
			if err != nil {
				return fmt.Errorf("notebook.NewLearningHistories() > %w", err)
			}

			writer := notebook.NewFlashcardNotebookWriter(reader, cfg.Templates.FlashcardNotebookTemplate)
			if err := writer.OutputFlashcardNotebooks(flashcardID, dictionaryMap, learningHistories, sortFlag == SortDescending, cfg.Outputs.FlashcardDirectory, flashcardGeneratePDF); err != nil {
				return fmt.Errorf("writer.OutputFlashcardNotebooks > %w", err)
			}
			return nil
		},
	}
	flashcardsCmd.Flags().BoolVar(&flashcardGeneratePDF, "pdf", false, "Generate PDF output in addition to markdown")

	notebookCommands.AddCommand(flashcardsCmd)

	return notebookCommands
}
