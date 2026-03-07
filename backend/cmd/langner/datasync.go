package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

func newExportDBCommand() *cobra.Command {
	var outputDir string

	cmd := &cobra.Command{
		Use:   "export-db",
		Short: "Export database to YAML files",
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
			noteSink := notebook.NewYAMLNoteRepositoryWriter(outputDir)
			learningSink := learning.NewYAMLLearningRepositoryWriter(outputDir)
			dictSink := rapidapi.NewJSONDictionaryRepositoryWriter(outputDir)
			exporter := datasync.NewExporter(noteRepo, learningRepo, dictRepo, noteSink, learningSink, dictSink, os.Stdout)

			noteResult, err := exporter.ExportNotes(ctx)
			if err != nil {
				return fmt.Errorf("export notes: %w", err)
			}

			learningResult, err := exporter.ExportLearningLogs(ctx)
			if err != nil {
				return fmt.Errorf("export learning logs: %w", err)
			}

			dictResult, err := exporter.ExportDictionary(ctx)
			if err != nil {
				return fmt.Errorf("export dictionary: %w", err)
			}

			fmt.Println("\nExport Summary:")
			fmt.Printf("  Notes exported:              %d\n", noteResult.NotesExported)
			fmt.Printf("  Learning logs exported:      %d\n", learningResult.LogsExported)
			fmt.Printf("  Dictionary entries exported: %d\n", dictResult.EntriesExported)

			return nil
		},
	}

	cmd.Flags().StringVar(&outputDir, "output", "", "Output directory for exported files")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}

func newValidateDatasyncCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate-datasync",
		Short: "Import data, export to temp directory, and validate exported files against sources",
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

			// Build source reader
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

			// Step 1: Import
			fmt.Println("Step 1: Importing data into database...")
			importer := datasync.NewImporter(noteRepo, learningRepo, yamlRepo, yamlLearningRepo, jsonDictRepo, dictRepo, os.Stdout)
			opts := datasync.ImportOptions{UpdateExisting: true}

			noteResult, err := importer.ImportNotes(ctx, opts)
			if err != nil {
				return fmt.Errorf("import notes: %w", err)
			}
			fmt.Printf("  Notes: %d new, %d skipped, %d updated\n", noteResult.NotesNew, noteResult.NotesSkipped, noteResult.NotesUpdated)

			learningResult, err := importer.ImportLearningLogs(ctx, opts)
			if err != nil {
				return fmt.Errorf("import learning logs: %w", err)
			}
			fmt.Printf("  Learning logs: %d new, %d skipped\n", learningResult.LearningNew, learningResult.LearningSkipped)

			dictResult, err := importer.ImportDictionary(ctx, opts)
			if err != nil {
				return fmt.Errorf("import dictionary: %w", err)
			}
			fmt.Printf("  Dictionary entries: %d new, %d skipped, %d updated\n", dictResult.DictionaryNew, dictResult.DictionarySkipped, dictResult.DictionaryUpdated)

			// Step 2: Export to temp directory
			tmpDir, err := os.MkdirTemp("", "langner-validate-datasync-*")
			if err != nil {
				return fmt.Errorf("create temp directory: %w", err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			fmt.Printf("\nStep 2: Exporting database to %s...\n", tmpDir)
			noteSink := notebook.NewYAMLNoteRepositoryWriter(tmpDir)
			learningSink := learning.NewYAMLLearningRepositoryWriter(tmpDir)
			dictSink := rapidapi.NewJSONDictionaryRepositoryWriter(tmpDir)
			exporter := datasync.NewExporter(noteRepo, learningRepo, dictRepo, noteSink, learningSink, dictSink, os.Stdout)

			exportNoteResult, err := exporter.ExportNotes(ctx)
			if err != nil {
				return fmt.Errorf("export notes: %w", err)
			}
			fmt.Printf("  Notes exported: %d\n", exportNoteResult.NotesExported)

			exportLearningResult, err := exporter.ExportLearningLogs(ctx)
			if err != nil {
				return fmt.Errorf("export learning logs: %w", err)
			}
			fmt.Printf("  Learning logs exported: %d\n", exportLearningResult.LogsExported)

			exportDictResult, err := exporter.ExportDictionary(ctx)
			if err != nil {
				return fmt.Errorf("export dictionary: %w", err)
			}
			fmt.Printf("  Dictionary entries exported: %d\n", exportDictResult.EntriesExported)

			// Step 3: Validate exported files against sources
			fmt.Println("\nStep 3: Validating exported files against sources...")
			exportedReader, err := notebook.NewReader(
				[]string{filepath.Join(tmpDir, "stories")},
				[]string{filepath.Join(tmpDir, "flashcards")},
				[]string{filepath.Join(tmpDir, "books")},
				nil,
				nil,
			)
			if err != nil {
				return fmt.Errorf("create exported reader: %w", err)
			}
			exportedYamlRepo := notebook.NewYAMLNoteRepository(exportedReader)

			validator := datasync.NewValidator(os.Stdout)
			validationResult, err := validator.ValidateNotes(ctx, yamlRepo, exportedYamlRepo)
			if err != nil {
				return fmt.Errorf("validate notes: %w", err)
			}

			fmt.Printf("\nValidation Summary:\n")
			fmt.Printf("  Source notes:   %d\n", validationResult.SourceNoteCount)
			fmt.Printf("  Exported notes: %d\n", validationResult.ExportedNoteCount)

			if len(validationResult.Errors) == 0 {
				fmt.Println("  All validations passed!")
				return nil
			}

			fmt.Printf("  Errors: %d\n\n", len(validationResult.Errors))
			for _, e := range validationResult.Errors {
				fmt.Printf("  [%s] %s: source=%q export=%q\n", e.NoteKey, e.Field, e.Source, e.Export)
			}
			return errors.New("validation failed")
		},
	}

	return cmd
}
