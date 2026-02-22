package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/datasync"
	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

func newMigrateImportDBCommand() *cobra.Command {
	var dryRun bool
	var updateExisting bool

	cmd := &cobra.Command{
		Use:   "import-db",
		Short: "Import notebook data into the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			loader, err := config.NewConfigLoader(configFile)
			if err != nil {
				return fmt.Errorf("load config loader: %w", err)
			}
			cfg, err := loader.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			db, err := database.Open(cfg.Database)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer func() { _ = db.Close() }()
			noteRepo := notebook.NewDBNoteRepository(db)
			learningRepo := learning.NewDBLearningRepository(db)
			dictRepo := dictionary.NewDBDictionaryRepository(db)

			reader, err := notebook.NewReader(
				cfg.Notebooks.StoriesDirectories,
				cfg.Notebooks.FlashcardsDirectories,
				cfg.Notebooks.BooksDirectories,
				cfg.Notebooks.DefinitionsDirectories,
				nil,
			)
			if err != nil {
				return fmt.Errorf("create notebook reader: %w", err)
			}

			yamlRepo := notebook.NewYAMLNoteRepository(reader)
			yamlLearningRepo := learning.NewYAMLLearningRepository(cfg.Notebooks.LearningNotesDirectory)
			jsonDictRepo := rapidapi.NewJSONDictionaryRepository(cfg.Dictionaries.RapidAPI.CacheDirectory)

			importer := datasync.NewImporter(noteRepo, learningRepo, yamlRepo, yamlLearningRepo, jsonDictRepo, dictRepo, os.Stdout)
			opts := datasync.ImportOptions{
				DryRun:         dryRun,
				UpdateExisting: updateExisting,
			}

			noteResult, err := importer.ImportNotes(ctx, opts)
			if err != nil {
				return fmt.Errorf("import notes: %w", err)
			}

			learningResult, err := importer.ImportLearningLogs(ctx, opts)
			if err != nil {
				return fmt.Errorf("import learning logs: %w", err)
			}

			dictResult, err := importer.ImportDictionary(ctx, opts)
			if err != nil {
				return fmt.Errorf("import dictionary: %w", err)
			}

			fmt.Println("\nImport Summary:")
			if opts.DryRun {
				fmt.Println("  (dry-run mode — no changes made)")
			}
			fmt.Printf("  Notes:              %d new, %d skipped, %d updated\n", noteResult.NotesNew, noteResult.NotesSkipped, noteResult.NotesUpdated)
			fmt.Printf("  Notebook notes:     %d new, %d skipped\n", noteResult.NotebookNew, noteResult.NotebookSkipped)
			fmt.Printf("  Learning logs:      %d new, %d skipped\n", learningResult.LearningNew, learningResult.LearningSkipped)
			fmt.Printf("  Dictionary entries: %d new, %d skipped, %d updated\n", dictResult.DictionaryNew, dictResult.DictionarySkipped, dictResult.DictionaryUpdated)

			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without modifying the database")
	cmd.Flags().BoolVar(&updateExisting, "update-existing", false, "Update existing records with new data")
	return cmd
}
