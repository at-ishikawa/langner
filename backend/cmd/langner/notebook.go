package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/at-ishikawa/langner/internal/analytics"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/at-ishikawa/langner/internal/quizreview"
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

// resolveNotebookOwner validates userID as the notebook's owner and returns it,
// or nil when userID is 0 ("unowned"). It REJECTS a nonexistent id and a tooling
// account (google_sub 'e2e-test|...', minted by the e2e seed / provisioning), so
// a production notebook's ownership can never be assigned to a synthetic account
// nobody signs in as — the owner must be a real Google account. There is no
// email lookup by design: accounts store no email (no PII), so the owner is
// named by id (sign in with Google first, then `langner auth backfill` with no
// flag lists the ids).
func resolveNotebookOwner(ctx context.Context, db *sqlx.DB, userID int64) (*int64, error) {
	if userID == 0 {
		return nil, nil
	}
	var sub string
	if err := db.GetContext(ctx, &sub, `SELECT google_sub FROM users WHERE id = $1`, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("no account with id %d — sign in with Google first, then run `langner auth backfill` with no flag to list ids", userID)
		}
		return nil, fmt.Errorf("look up user %d: %w", userID, err)
	}
	if strings.HasPrefix(sub, toolingSubPrefix) {
		return nil, fmt.Errorf("id %d is a tooling account, not a Google sign-in — pass the id of your real account (run `langner auth backfill` with no flag to find it)", userID)
	}
	return &userID, nil
}

// newNotebooksSetOwnerCommand assigns a notebook's public/private visibility and,
// for a private notebook, its owner — a REAL account named by --user-id. Auth
// must be enabled (owners are user accounts in Postgres). A private notebook is
// visible only to its owner; a public notebook is visible to every logged-in
// user (owner optional). No --user-id leaves the notebook unowned (public only).
func newNotebooksSetOwnerCommand() *cobra.Command {
	var notebookID, visibility string
	var userID int64
	cmd := &cobra.Command{
		Use:   "set-owner",
		Short: "Assign a notebook's public/private visibility and (for private) its owner",
		Long: `Assign a notebook's visibility, and for a private notebook its owner.

The owner is a REAL Google account named by --user-id (accounts store no email,
so there is no --owner-email): sign in with Google first so the account exists,
then run 'langner auth backfill' with no flag to list account ids. A private
notebook is visible only to its owner; a public notebook is visible to every
logged-in user. Omitting --user-id leaves the notebook unowned (public only).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if notebookID == "" {
				return fmt.Errorf("--notebook-id is required")
			}
			if visibility != notebook.VisibilityPublic && visibility != notebook.VisibilityPrivate {
				return fmt.Errorf("--visibility must be %q or %q", notebook.VisibilityPublic, notebook.VisibilityPrivate)
			}
			if visibility == notebook.VisibilityPrivate && userID == 0 {
				return fmt.Errorf("--user-id is required for a private notebook (it is visible only to its owner); run `langner auth backfill` with no flag to list account ids")
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

			ownerID, err := resolveNotebookOwner(ctx, db, userID)
			if err != nil {
				return err
			}
			if err := notebook.NewNotebookACLRepository(db).UpsertOwnership(ctx, notebookID, ownerID, visibility); err != nil {
				return err
			}
			owner := "none"
			if ownerID != nil {
				owner = fmt.Sprintf("user id %d", *ownerID)
			}
			fmt.Printf("Set notebook %q visibility=%s owner=%s\n", notebookID, visibility, owner)
			return nil
		},
	}
	cmd.Flags().StringVar(&notebookID, "notebook-id", "", "notebook id to assign")
	cmd.Flags().Int64Var(&userID, "user-id", 0, "id of the real (Google) account that owns the notebook (required for private)")
	cmd.Flags().StringVar(&visibility, "visibility", notebook.VisibilityPublic, "public or private")
	return cmd
}
