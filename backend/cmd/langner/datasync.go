package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

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

func newValidateDBCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate-db",
		Short: "Validate database import/export round-trip by comparing source YAML against exported YAML",
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

			// Step 1: Import
			fmt.Println("Step 1: Importing data...")
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

			importer := datasync.NewImporter(noteRepo, learningRepo, yamlRepo, yamlLearningRepo, jsonDictRepo, dictRepo, io.Discard)
			opts := datasync.ImportOptions{UpdateExisting: true}
			if _, err := importer.ImportNotes(ctx, opts); err != nil {
				return fmt.Errorf("import notes: %w", err)
			}
			if _, err := importer.ImportLearningLogs(ctx, opts); err != nil {
				return fmt.Errorf("import learning logs: %w", err)
			}
			if _, err := importer.ImportDictionary(ctx, opts); err != nil {
				return fmt.Errorf("import dictionary: %w", err)
			}
			fmt.Println("  Import complete.")

			// Step 2: Export to temp dir
			exportDir, err := os.MkdirTemp("", "langner-validate-*")
			if err != nil {
				return fmt.Errorf("create temp dir: %w", err)
			}
			defer func() { _ = os.RemoveAll(exportDir) }()

			fmt.Printf("Step 2: Exporting data to %s...\n", exportDir)
			noteSink := notebook.NewYAMLNoteRepositoryWriter(exportDir)
			learningSink := learning.NewYAMLLearningRepositoryWriter(exportDir)
			dictSink := rapidapi.NewJSONDictionaryRepositoryWriter(exportDir)
			exporter := datasync.NewExporter(noteRepo, learningRepo, dictRepo, noteSink, learningSink, dictSink, io.Discard)

			if _, err := exporter.ExportNotes(ctx); err != nil {
				return fmt.Errorf("export notes: %w", err)
			}
			if _, err := exporter.ExportLearningLogs(ctx); err != nil {
				return fmt.Errorf("export learning logs: %w", err)
			}
			if _, err := exporter.ExportDictionary(ctx); err != nil {
				return fmt.Errorf("export dictionary: %w", err)
			}
			fmt.Println("  Export complete.")

			// Step 3: Read source data
			fmt.Println("Step 3: Validating round-trip...")
			sourceReader, err := notebook.NewReader(
				cfg.Notebooks.StoriesDirectories,
				cfg.Notebooks.FlashcardsDirectories,
				cfg.Notebooks.BooksDirectories,
				cfg.Notebooks.DefinitionsDirectories,
				nil,
			)
			if err != nil {
				return fmt.Errorf("create source reader: %w", err)
			}
			sourceYAML := notebook.NewYAMLNoteRepository(sourceReader)
			sourceNotes, err := sourceYAML.FindAll(ctx)
			if err != nil {
				return fmt.Errorf("read source notes: %w", err)
			}

			// Read exported notes
			exportedReader, err := notebook.NewReader(
				[]string{filepath.Join(exportDir, "stories")},
				[]string{filepath.Join(exportDir, "flashcards")},
				[]string{filepath.Join(exportDir, "books")},
				nil,
				nil,
			)
			if err != nil {
				return fmt.Errorf("create exported reader: %w", err)
			}
			exportedYAML := notebook.NewYAMLNoteRepository(exportedReader)
			exportedNotes, err := exportedYAML.FindAll(ctx)
			if err != nil {
				return fmt.Errorf("read exported notes: %w", err)
			}

			// Read source learning logs by notebook
			sourceLearningByNotebook := make(map[string][]notebook.LearningHistoryExpression)
			sourceLearningYAML := learning.NewYAMLLearningRepository(cfg.Notebooks.LearningNotesDirectory)
			for _, nbID := range extractNotebookIDs(sourceNotes) {
				exprs, err := sourceLearningYAML.FindByNotebookID(nbID)
				if err != nil {
					return fmt.Errorf("read source learning for %s: %w", nbID, err)
				}
				if len(exprs) > 0 {
					sourceLearningByNotebook[nbID] = exprs
				}
			}

			// Read exported learning logs by notebook
			exportedLearningByNotebook := make(map[string][]notebook.LearningHistoryExpression)
			exportedLearningYAML := learning.NewYAMLLearningRepository(filepath.Join(exportDir, "learning_notes"))
			for _, nbID := range extractNotebookIDs(exportedNotes) {
				exprs, err := exportedLearningYAML.FindByNotebookID(nbID)
				if err != nil {
					return fmt.Errorf("read exported learning for %s: %w", nbID, err)
				}
				if len(exprs) > 0 {
					exportedLearningByNotebook[nbID] = exprs
				}
			}

			// Read dictionary counts
			sourceDictCount := 0
			if cfg.Dictionaries.RapidAPI.CacheDirectory != "" {
				responses, err := jsonDictRepo.ReadAll()
				if err == nil {
					sourceDictCount = len(responses)
				}
			}

			exportedDictCount := 0
			exportedDictDir := filepath.Join(exportDir, "dictionaries", "rapidapi")
			if _, statErr := os.Stat(exportedDictDir); statErr == nil {
				exportedDictRepo := rapidapi.NewJSONDictionaryRepository(exportedDictDir)
				responses, err := exportedDictRepo.ReadAll()
				if err == nil {
					exportedDictCount = len(responses)
				}
			}

			// Run validation
			validResult := datasync.ValidateRoundTrip(
				sourceNotes, exportedNotes,
				sourceLearningByNotebook, exportedLearningByNotebook,
				sourceDictCount, exportedDictCount,
				os.Stdout,
			)

			if validResult.HasMismatches() {
				return fmt.Errorf("validation failed with %d mismatch(es)", len(validResult.Mismatches))
			}

			return nil
		},
	}

	return cmd
}

func extractNotebookIDs(notes []notebook.NoteRecord) []string {
	seen := make(map[string]bool)
	for _, n := range notes {
		for _, nn := range n.NotebookNotes {
			seen[nn.NotebookID] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
