package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/internal/datasync"
	"github.com/at-ishikawa/langner/internal/dictionary"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/note"
	"github.com/at-ishikawa/langner/internal/notebook"
)

func newDbCommand() *cobra.Command {
	dbCmd := &cobra.Command{
		Use:   "db",
		Short: "Database operations",
	}
	dbCmd.AddCommand(newDbImportCommand())
	dbCmd.AddCommand(newDbExportCommand())
	return dbCmd
}

func newDbImportCommand() *cobra.Command {
	var dryRun bool
	var updateExisting bool

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import notebook data into the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			loader, err := config.NewConfigLoader(configFile)
			if err != nil {
				return fmt.Errorf("config.NewConfigLoader() > %w", err)
			}
			cfg, err := loader.Load()
			if err != nil {
				return fmt.Errorf("loader.Load() > %w", err)
			}

			db, err := database.Open(cfg.Database)
			if err != nil {
				return fmt.Errorf("database.Open() > %w", err)
			}
			if err := database.RunMigrations(db); err != nil {
				return fmt.Errorf("database.RunMigrations() > %w", err)
			}

			noteRepo := note.NewDBNoteRepository(db)
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
				return fmt.Errorf("notebook.NewReader() > %w", err)
			}

			storyIndexes, err := reader.ReadAllStoryNotebooks()
			if err != nil {
				return fmt.Errorf("reader.ReadAllStoryNotebooks() > %w", err)
			}

			flashcardIndexes, err := reader.ReadAllFlashcardNotebooks()
			if err != nil {
				return fmt.Errorf("reader.ReadAllFlashcardNotebooks() > %w", err)
			}

			learningHistories, err := notebook.NewLearningHistories(cfg.Notebooks.LearningNotesDirectory)
			if err != nil {
				return fmt.Errorf("notebook.NewLearningHistories() > %w", err)
			}

			dictResponses, err := rapidapi.NewReader().Read(cfg.Dictionaries.RapidAPI.CacheDirectory)
			if err != nil {
				return fmt.Errorf("rapidapi.Reader.Read() > %w", err)
			}

			importer := datasync.NewImporter(noteRepo, learningRepo, dictRepo, os.Stdout)
			opts := datasync.ImportOptions{
				DryRun:         dryRun,
				UpdateExisting: updateExisting,
			}

			noteResult, err := importer.ImportNotes(ctx, storyIndexes, flashcardIndexes, opts)
			if err != nil {
				return fmt.Errorf("importer.ImportNotes() > %w", err)
			}

			learningResult, err := importer.ImportLearningLogs(ctx, learningHistories, opts)
			if err != nil {
				return fmt.Errorf("importer.ImportLearningLogs() > %w", err)
			}

			dictResult, err := importer.ImportDictionary(ctx, dictResponses, opts)
			if err != nil {
				return fmt.Errorf("importer.ImportDictionary() > %w", err)
			}

			fmt.Println("\nImport Summary:")
			if opts.DryRun {
				fmt.Println("  (dry-run mode — no changes made)")
			}
			fmt.Printf("  Notes:              %d new, %d skipped, %d updated\n", noteResult.NotesNew, noteResult.NotesSkipped, noteResult.NotesUpdated)
			fmt.Printf("  Notebook notes:     %d new, %d skipped\n", noteResult.NotebookNew, noteResult.NotebookSkipped)
			fmt.Printf("  Learning logs:      %d new, %d skipped, %d warnings\n", learningResult.LearningNew, learningResult.LearningSkipped, learningResult.LearningWarnings)
			fmt.Printf("  Dictionary entries: %d new, %d skipped, %d updated\n", dictResult.DictionaryNew, dictResult.DictionarySkipped, dictResult.DictionaryUpdated)

			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without modifying the database")
	cmd.Flags().BoolVar(&updateExisting, "update-existing", false, "Update existing records with new data")
	return cmd
}

func newDbExportCommand() *cobra.Command {
	var outputDir string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export database data to YAML files",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			loader, err := config.NewConfigLoader(configFile)
			if err != nil {
				return fmt.Errorf("config.NewConfigLoader() > %w", err)
			}
			cfg, err := loader.Load()
			if err != nil {
				return fmt.Errorf("loader.Load() > %w", err)
			}

			db, err := database.Open(cfg.Database)
			if err != nil {
				return fmt.Errorf("database.Open() > %w", err)
			}
			if err := database.RunMigrations(db); err != nil {
				return fmt.Errorf("database.RunMigrations() > %w", err)
			}

			noteRepo := note.NewDBNoteRepository(db)
			learningRepo := learning.NewDBLearningRepository(db)
			dictRepo := dictionary.NewDBDictionaryRepository(db)

			exporter := datasync.NewExporter(noteRepo, learningRepo, dictRepo)
			data, err := exporter.Export(ctx)
			if err != nil {
				return fmt.Errorf("exporter.Export() > %w", err)
			}

			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("os.MkdirAll(%s) > %w", outputDir, err)
			}

			writeYAML := func(path string, data any) error {
				f, err := os.Create(path)
				if err != nil {
					return fmt.Errorf("os.Create(%s) > %w", path, err)
				}
				defer f.Close()
				return yaml.NewEncoder(f).Encode(data)
			}

			if err := writeYAML(filepath.Join(outputDir, "notes.yml"), data.Notes); err != nil {
				return fmt.Errorf("writeYAML(notes) > %w", err)
			}
			if err := writeYAML(filepath.Join(outputDir, "learning_logs.yml"), data.LearningLogs); err != nil {
				return fmt.Errorf("writeYAML(learning_logs) > %w", err)
			}
			if err := writeYAML(filepath.Join(outputDir, "dictionary_entries.yml"), data.DictionaryEntries); err != nil {
				return fmt.Errorf("writeYAML(dictionary_entries) > %w", err)
			}

			fmt.Printf("Exported to %s:\n", outputDir)
			fmt.Printf("  Notes:              %d\n", len(data.Notes))
			fmt.Printf("  Learning logs:      %d\n", len(data.LearningLogs))
			fmt.Printf("  Dictionary entries: %d\n", len(data.DictionaryEntries))

			return nil
		},
	}

	cmd.Flags().StringVar(&outputDir, "output", "./export", "Output directory for exported YAML files")
	return cmd
}
