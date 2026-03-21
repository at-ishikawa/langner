package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/jmoiron/sqlx"
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

			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			importer := newImporterFromConfig(cfg, db, os.Stdout)
			opts := datasync.ImportOptions{
				DryRun:         dryRun,
				UpdateExisting: updateExisting,
			}

			result, err := importer.ImportAll(ctx, opts)
			if err != nil {
				return err
			}

			fmt.Println("\nImport Summary:")
			if opts.DryRun {
				fmt.Println("  (dry-run mode — no changes made)")
			}
			fmt.Printf("  Notes:              %d new, %d skipped, %d updated\n", result.Notes.NotesNew, result.Notes.NotesSkipped, result.Notes.NotesUpdated)
			fmt.Printf("  Notebook notes:     %d new, %d skipped\n", result.Notes.NotebookNew, result.Notes.NotebookSkipped)
			fmt.Printf("  Learning logs:      %d new, %d skipped\n", result.Learning.LearningNew, result.Learning.LearningSkipped)
			fmt.Printf("  Dictionary entries: %d new, %d skipped, %d updated\n", result.Dictionary.DictionaryNew, result.Dictionary.DictionarySkipped, result.Dictionary.DictionaryUpdated)

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

			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			exporter := newExporterFromConfig(cfg, db, outputDir, os.Stdout)
			result, err := exporter.ExportAll(ctx)
			if err != nil {
				return err
			}

			fmt.Println("\nExport Summary:")
			fmt.Printf("  Notes exported:              %d\n", result.Notes.NotesExported)
			fmt.Printf("  Learning logs exported:      %d\n", result.Learning.LogsExported)
			fmt.Printf("  Dictionary entries exported: %d\n", result.Dictionary.EntriesExported)

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

			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			// Step 0: Clear existing data for a clean round-trip test
			fmt.Println("Clearing database for clean round-trip test...")
			for _, table := range []string{"learning_logs", "notebook_notes", "note_images", "note_references", "notes", "dictionary_entries"} {
				if _, err := db.ExecContext(ctx, "DELETE FROM "+table); err != nil {
					return fmt.Errorf("clear table %s: %w", table, err)
				}
			}

			// Step 1: Import
			fmt.Println("Step 1: Importing data...")
			importer := newImporterFromConfig(cfg, db, io.Discard)
			if _, err := importer.ImportAll(ctx, datasync.ImportOptions{UpdateExisting: true}); err != nil {
				return err
			}
			fmt.Println("  Import complete.")

			// Step 2: Export to temp dir
			exportDir, err := os.MkdirTemp("", "langner-validate-*")
			if err != nil {
				return fmt.Errorf("create temp dir: %w", err)
			}
			defer func() { _ = os.RemoveAll(exportDir) }()

			fmt.Printf("Step 2: Exporting data to %s...\n", exportDir)
			exporter := newExporterFromConfig(cfg, db, exportDir, io.Discard)
			if _, err := exporter.ExportAll(ctx); err != nil {
				return err
			}
			fmt.Println("  Export complete.")

			// Step 3: Compare source vs exported
			fmt.Println("Step 3: Validating round-trip...")

			sourceNotes, err := readNotesFromDirs(ctx, cfg.Notebooks.StoriesDirectories, cfg.Notebooks.FlashcardsDirectories, cfg.Notebooks.BooksDirectories, cfg.Notebooks.DefinitionsDirectories)
			if err != nil {
				return fmt.Errorf("read source notes: %w", err)
			}
			exportedNotes, err := readNotesFromDirs(ctx,
				[]string{filepath.Join(exportDir, "stories")},
				[]string{filepath.Join(exportDir, "flashcards")},
				[]string{filepath.Join(exportDir, "books")},
				nil,
			)
			if err != nil {
				return fmt.Errorf("read exported notes: %w", err)
			}

			sourceLearning := readLearningByNotebook(sourceNotes, cfg.Notebooks.LearningNotesDirectory)
			exportedLearning := readLearningByNotebook(exportedNotes, filepath.Join(exportDir, "learning_notes"))

			sourceDictCount := countDictEntries(cfg.Dictionaries.RapidAPI.CacheDirectory)
			exportedDictCount := countDictEntries(filepath.Join(exportDir, "dictionaries", "rapidapi"))

			validResult := datasync.ValidateRoundTrip(
				sourceNotes, exportedNotes,
				sourceLearning, exportedLearning,
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

func openConfigAndDB() (*config.Config, *sqlx.DB, error) {
	loader, err := config.NewConfigLoader(configFile)
	if err != nil {
		return nil, nil, fmt.Errorf("load config loader: %w", err)
	}
	cfg, err := loader.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	db, err := database.Open(cfg.Database)
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}
	return cfg, db, nil
}

func newImporterFromConfig(cfg *config.Config, db *sqlx.DB, writer io.Writer) *datasync.Importer {
	noteRepo := notebook.NewDBNoteRepository(db)
	learningRepo := learning.NewDBLearningRepository(db)
	dictRepo := dictionary.NewDBDictionaryRepository(db)

	reader, err := notebook.NewReader(
		cfg.Notebooks.StoriesDirectories,
		cfg.Notebooks.FlashcardsDirectories,
		cfg.Notebooks.BooksDirectories,
		cfg.Notebooks.DefinitionsDirectories,
		cfg.Notebooks.EtymologyDirectories,
		nil,
	)
	if err != nil {
		// Reader creation only fails if directories are invalid, which is a config issue.
		// The caller will get the error when ImportAll is called.
		return datasync.NewImporter(noteRepo, learningRepo, nil, nil, nil, dictRepo, writer)
	}

	yamlRepo := notebook.NewYAMLNoteRepository(reader)
	yamlLearningRepo := learning.NewYAMLLearningRepository(cfg.Notebooks.LearningNotesDirectory, nil)
	jsonDictRepo := rapidapi.NewJSONDictionaryRepository(cfg.Dictionaries.RapidAPI.CacheDirectory)

	return datasync.NewImporter(noteRepo, learningRepo, yamlRepo, yamlLearningRepo, jsonDictRepo, dictRepo, writer)
}

func newExporterFromConfig(cfg *config.Config, db *sqlx.DB, outputDir string, writer io.Writer) *datasync.Exporter {
	noteRepo := notebook.NewDBNoteRepository(db)
	learningRepo := learning.NewDBLearningRepository(db)
	dictRepo := dictionary.NewDBDictionaryRepository(db)
	noteSink := notebook.NewYAMLNoteRepositoryWriter(outputDir)
	learningSink := learning.NewYAMLLearningRepositoryWriter(outputDir)
	dictSink := rapidapi.NewJSONDictionaryRepositoryWriter(outputDir)

	return datasync.NewExporter(noteRepo, learningRepo, dictRepo, noteSink, learningSink, dictSink, writer)
}

func readNotesFromDirs(ctx context.Context, storyDirs, flashcardDirs, bookDirs, definitionDirs []string) ([]notebook.NoteRecord, error) {
	reader, err := notebook.NewReader(storyDirs, flashcardDirs, bookDirs, definitionDirs, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create reader: %w", err)
	}
	yamlRepo := notebook.NewYAMLNoteRepository(reader)
	return yamlRepo.FindAll(ctx)
}

func readLearningByNotebook(notes []notebook.NoteRecord, learningDir string) map[string][]notebook.LearningHistoryExpression {
	result := make(map[string][]notebook.LearningHistoryExpression)
	repo := learning.NewYAMLLearningRepository(learningDir, nil)
	for _, nbID := range extractNotebookIDs(notes) {
		exprs, err := repo.FindByNotebookID(nbID)
		if err != nil || len(exprs) == 0 {
			continue
		}
		result[nbID] = exprs
	}
	return result
}

func countDictEntries(dir string) int {
	if dir == "" {
		return 0
	}
	if _, err := os.Stat(dir); err != nil {
		return 0
	}
	repo := rapidapi.NewJSONDictionaryRepository(dir)
	responses, err := repo.ReadAll()
	if err != nil {
		return 0
	}
	unique := make(map[string]struct{}, len(responses))
	for _, r := range responses {
		unique[r.Word] = struct{}{}
	}
	return len(unique)
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
