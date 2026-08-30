package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/analytics"
	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quizreview"
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
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			storyID := args[0]

			response, err := rapidapi.NewReader().Read(cfg.Dictionaries.RapidAPI.CacheDirectory)
			if err != nil {
				return fmt.Errorf("rapidapi.NewReader().Read() > %w", err)
			}
			dictionaryMap := rapidapi.FromResponsesToMap(response)

			reader, err := notebook.NewReader(cfg.Notebooks.StoriesDirectories, nil, cfg.Notebooks.BooksDirectories, cfg.Notebooks.DefinitionsDirectories, cfg.Notebooks.EtymologyDirectories, dictionaryMap)
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
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			flashcardID := args[0]

			response, err := rapidapi.NewReader().Read(cfg.Dictionaries.RapidAPI.CacheDirectory)
			if err != nil {
				return fmt.Errorf("rapidapi.NewReader().Read() > %w", err)
			}
			dictionaryMap := rapidapi.FromResponsesToMap(response)

			reader, err := notebook.NewReader(nil, cfg.Notebooks.FlashcardsDirectories, nil, nil, nil, dictionaryMap)
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

	var etymologyGeneratePDF bool
	etymologyCmd := &cobra.Command{
		Use:   "etymology <etymology id>",
		Short: "Generate markdown/PDF output from etymology notebooks",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			etymologyID := args[0]

			reader, err := notebook.NewReader(nil, nil, nil, cfg.Notebooks.DefinitionsDirectories, cfg.Notebooks.EtymologyDirectories, nil)
			if err != nil {
				return fmt.Errorf("notebook.NewReader() > %w", err)
			}

			learningHistories, err := notebook.NewLearningHistories(cfg.Notebooks.LearningNotesDirectory)
			if err != nil {
				return fmt.Errorf("notebook.NewLearningHistories() > %w", err)
			}
			writer := notebook.NewEtymologyNotebookWriter(reader, cfg.Templates.EtymologyNotebookTemplate, cfg.Notebooks.DefinitionsDirectories, learningHistories)
			if err := writer.OutputEtymologyNotebook(etymologyID, cfg.Outputs.EtymologyDirectory, etymologyGeneratePDF); err != nil {
				return fmt.Errorf("writer.OutputEtymologyNotebook > %w", err)
			}
			return nil
		},
	}
	etymologyCmd.Flags().BoolVar(&etymologyGeneratePDF, "pdf", false, "Generate PDF output in addition to markdown")

	notebookCommands.AddCommand(etymologyCmd)

	var definitionsGeneratePDF bool
	definitionsCmd := &cobra.Command{
		Use:   "definitions <book id>",
		Short: "Generate markdown/PDF output from a definitions-only book (e.g. Word Power Made Easy), with concept members grouped into one row per concept",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			bookID := args[0]
			reader, err := notebook.NewReader(nil, nil, nil, cfg.Notebooks.DefinitionsDirectories, nil, nil)
			if err != nil {
				return fmt.Errorf("notebook.NewReader() > %w", err)
			}
			writer := notebook.NewDefinitionsBookWriter(reader, cfg.Templates.StoryNotebookTemplate)
			outDir := cfg.Outputs.StoryDirectory
			if err := writer.OutputDefinitionsBook(bookID, outDir, definitionsGeneratePDF); err != nil {
				return fmt.Errorf("writer.OutputDefinitionsBook > %w", err)
			}
			return nil
		},
	}
	definitionsCmd.Flags().BoolVar(&definitionsGeneratePDF, "pdf", false, "Generate PDF output in addition to markdown")
	notebookCommands.AddCommand(definitionsCmd)

	var quizReviewGeneratePDF bool
	quizReviewCmd := &cobra.Command{
		Use:   "quiz-review [YYYY-MM-DD]",
		Short: "Generate per-notebook markdown/PDF of the words and origins you got wrong on the given date (default: today).",
		Long: `Export a single study-friendly markdown file covering every notebook
where you got something wrong on the given date. Each notebook becomes a
top-level section, broken down by source session, with an entry per failed
expression carrying its meaning, an example sentence (when available), and
the concept-graph context (sibling words, sibling origins, antonym / synonym
members) — so the file can be re-read alongside the original notebooks to
drill exactly that day's failures.

The file lands at <outputs.quiz_review_directory>/quiz-review-<date>.md.
When quiz_review_directory is unset, the command falls back to
outputs.story_directory.

The date argument defaults to today in your local timezone.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			day := time.Now()
			if len(args) == 1 {
				parsed, err := time.ParseInLocation("2006-01-02", args[0], time.Local)
				if err != nil {
					return fmt.Errorf("parse date %q: %w (expected YYYY-MM-DD)", args[0], err)
				}
				day = parsed
			}

			reader, err := notebook.NewReader(
				cfg.Notebooks.StoriesDirectories,
				cfg.Notebooks.FlashcardsDirectories,
				cfg.Notebooks.BooksDirectories,
				cfg.Notebooks.DefinitionsDirectories,
				cfg.Notebooks.EtymologyDirectories,
				nil,
			)
			if err != nil {
				return fmt.Errorf("notebook.NewReader: %w", err)
			}

			repo := analytics.NewYAMLRepository(cfg.Notebooks.LearningNotesDirectory).
				WithMetadataResolver(analytics.NewNotebookMetadataResolver(reader))
			writer := quizreview.NewWriterWithSource(repo, quizreview.NewReaderSource(reader))

			outDir := cfg.Outputs.QuizReviewDirectory
			if outDir == "" {
				outDir = cfg.Outputs.StoryDirectory
			}
			if outDir == "" {
				return fmt.Errorf("no output directory configured (set outputs.quiz_review_directory or outputs.story_directory)")
			}

			written, err := writer.Output(context.Background(), day, outDir, quizReviewGeneratePDF)
			if err != nil {
				return fmt.Errorf("writer.Output: %w", err)
			}
			if written == "" {
				fmt.Printf("No wrong attempts found for %s — nothing to write.\n", day.Format("2006-01-02"))
				return nil
			}
			fmt.Printf("Wrote %s\n", written)
			if quizReviewGeneratePDF {
				fmt.Printf("Wrote %s\n", strings.TrimSuffix(written, ".md")+".pdf")
			}
			return nil
		},
	}
	quizReviewCmd.Flags().BoolVar(&quizReviewGeneratePDF, "pdf", false, "Generate PDF output in addition to markdown")
	notebookCommands.AddCommand(quizReviewCmd)

	notebookCommands.AddCommand(newNotebooksSetOwnerCommand())

	return notebookCommands
}

// newNotebooksSetOwnerCommand assigns a notebook's owner + public/private
// visibility ad hoc (the same overlay `notebook_ownership:` config provisions at
// import time). Auth must be enabled (owners are user accounts in Postgres). An
// empty --owner-email leaves the notebook unowned (valid only for public).
func newNotebooksSetOwnerCommand() *cobra.Command {
	var notebookID, ownerEmail, visibility string
	cmd := &cobra.Command{
		Use:   "set-owner",
		Short: "Assign a notebook's owner and public/private visibility",
		RunE: func(cmd *cobra.Command, args []string) error {
			if notebookID == "" {
				return fmt.Errorf("--notebook-id is required")
			}
			if visibility != notebook.VisibilityPublic && visibility != notebook.VisibilityPrivate {
				return fmt.Errorf("--visibility must be %q or %q", notebook.VisibilityPublic, notebook.VisibilityPrivate)
			}
			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			if !cfg.Auth.Enabled() {
				return fmt.Errorf("auth is not enabled in config (session_signing_key is unset)")
			}
			ctx := cmd.Context()
			var ownerID *int64
			if ownerEmail != "" {
				enc, err := auth.NewEncryptor(auth.DecodeKey(cfg.Auth.CredentialEncryptionKey))
				if err != nil {
					return fmt.Errorf("credential encryption key: %w", err)
				}
				users := auth.NewUserRepository(db, enc)
				id, err := ensureUser(ctx, users, ownerEmail, ownerEmail)
				if err != nil {
					return err
				}
				ownerID = &id
			}
			if err := notebook.NewNotebookACLRepository(db).UpsertOwnership(ctx, notebookID, ownerID, visibility); err != nil {
				return err
			}
			fmt.Printf("Set notebook %q visibility=%s owner=%q\n", notebookID, visibility, ownerEmail)
			return nil
		},
	}
	cmd.Flags().StringVar(&notebookID, "notebook-id", "", "notebook id to assign")
	cmd.Flags().StringVar(&ownerEmail, "owner-email", "", "owner account email (empty = unowned)")
	cmd.Flags().StringVar(&visibility, "visibility", notebook.VisibilityPublic, "public or private")
	return cmd
}
