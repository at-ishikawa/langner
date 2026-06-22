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
	"github.com/at-ishikawa/langner/schemas"
)

// newMigrateUpCommand applies pending schema migrations against the
// configured database without importing or re-importing notebook
// content. Use this after upgrading the binary when migration files
// have been added (e.g. 016_db_only_state) but you don't want
// `import-db` to walk the YAML again.
func newMigrateUpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Apply pending schema migrations (no content import)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			db, err := openDB(cfg)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer func() { _ = db.Close() }()

			if err := database.Migrate(db, schemas.Migrations, "migrations"); err != nil {
				return fmt.Errorf("apply schema migrations: %w", err)
			}
			fmt.Println("Schema migrations applied.")
			return nil
		},
	}
	return cmd
}

func newMigrateImportDBCommand() *cobra.Command {
	var dryRun bool
	var updateExisting bool
	var skipMigrate bool

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

			// Auto-apply schema migrations before import. The embedded
			// migration files always match the binary version, so we can
			// safely run them every time. --skip-migrate is the escape
			// hatch for the rare case where a downgraded binary needs to
			// import against a newer schema without rolling back.
			if !skipMigrate {
				if err := database.Migrate(db, schemas.Migrations, "migrations"); err != nil {
					return fmt.Errorf("apply schema migrations: %w", err)
				}
			}

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
			fmt.Printf("  Notes:              %d new, %d skipped, %d updated, %d deleted\n", result.Notes.NotesNew, result.Notes.NotesSkipped, result.Notes.NotesUpdated, result.Notes.NotesDeleted)
			fmt.Printf("  Notebook notes:     %d new, %d skipped, %d deleted\n", result.Notes.NotebookNew, result.Notes.NotebookSkipped, result.Notes.NotebookNotesDeleted)
			fmt.Printf("  Learning logs:      %d new, %d skipped, %d deleted\n", result.Learning.LearningNew, result.Learning.LearningSkipped, result.Learning.LearningDeleted)
			fmt.Printf("  Dictionary entries: %d new, %d skipped, %d updated\n", result.Dictionary.DictionaryNew, result.Dictionary.DictionarySkipped, result.Dictionary.DictionaryUpdated)
			if result.Etymology != nil {
				fmt.Printf("  Etymology origins:  %d new, %d skipped\n", result.Etymology.OriginsNew, result.Etymology.OriginsSkipped)
				fmt.Printf("  Note origin parts:  %d new, %d skipped\n", result.Etymology.PartsNew, result.Etymology.PartsSkipped)
			}

			// Phase 2: seed the migration-016 state tables from the same
			// YAML the importer just consumed. The state seeder is
			// idempotent so re-runs only insert what's missing.
			if !opts.DryRun {
				seeder := newStateSeederFromConfig(cfg, db, os.Stdout)
				if seeder == nil {
					return nil
				}
				stateResult, err := seeder.SeedAll(ctx)
				if err != nil {
					return fmt.Errorf("seed db-only state: %w", err)
				}
				fmt.Println("\nState Seed Summary:")
				fmt.Printf("  Definitions sessions: %d new\n", stateResult.DefinitionsSessionsCreated)
				fmt.Printf("  Definitions scenes:   %d new\n", stateResult.DefinitionsScenesCreated)
				fmt.Printf("  Flashcard decks:      %d new\n", stateResult.FlashcardDecksCreated)
				fmt.Printf("  Note skip flags:      %d new\n", stateResult.NoteSkipFlagsCreated)
				fmt.Printf("  Origin skip flags:    %d new\n", stateResult.OriginSkipFlagsCreated)
				fmt.Printf("  Etymology logs:       %d new\n", stateResult.EtymologyLogsCreated)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without modifying the database")
	cmd.Flags().BoolVar(&updateExisting, "update-existing", false, "Update existing records with new data")
	cmd.Flags().BoolVar(&skipMigrate, "skip-migrate", false, "Skip applying schema migrations before import")
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
		Short: "Compare current database state against source YAML files (read-only)",
		Long: `Export the database's current state to a temporary directory and compare
it against the source YAML notebooks. Reports any mismatches between
the two and exits non-zero when divergence is found.

This command is read-only: it never writes to the database. To re-sync
the database from YAML when divergence is found, run "migrate sync-db".`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			return runRoundTripDiff(ctx, cfg, db, os.Stdout)
		},
	}

	return cmd
}

func newSyncDBCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync-db",
		Short: "Drop schema, re-apply migrations, re-import from source YAML (destructive)",
		Long: `Make the database match the source YAML files from a clean slate.
This is a destructive operation:

  1. DROP every table in the database (including schema_migrations).
     This also clears any half-applied migration state, which is the
     safe path to recover from a migration that failed mid-flight on
     a backend that doesn't roll back DDL (MySQL, TiDB).
  2. Apply every schema migration from scratch.
  3. Import all source YAML notebooks into the now-empty database.
  4. Export the database back to a temporary directory and diff
     against the source YAML to verify the roundtrip is lossless.

Use this command when the database has drifted from the YAML and you
want YAML to win, when a migration failed and the schema is in a
partially-applied state, or after upgrading the binary. To check
current divergence WITHOUT modifying the database, use
"migrate validate-db" instead.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			fmt.Println("Step 1: Dropping every table in the database...")
			if err := dropAllTables(ctx, db, cfg.Database.Database); err != nil {
				return err
			}
			fmt.Println("  Drop complete.")

			fmt.Println("Step 2: Applying schema migrations from scratch...")
			if err := database.Migrate(db, schemas.Migrations, "migrations"); err != nil {
				return fmt.Errorf("apply schema migrations: %w", err)
			}
			fmt.Println("  Migrations applied.")

			fmt.Println("Step 3: Importing source YAML into the empty database...")
			importer := newImporterFromConfig(cfg, db, io.Discard)
			if _, err := importer.ImportAll(ctx, datasync.ImportOptions{UpdateExisting: true}); err != nil {
				return err
			}
			fmt.Println("  Import complete.")

			if seeder := newStateSeederFromConfig(cfg, db, io.Discard); seeder != nil {
				fmt.Println("Step 4: Seeding DB-only state tables from YAML...")
				if _, err := seeder.SeedAll(ctx); err != nil {
					return fmt.Errorf("seed db-only state: %w", err)
				}
				fmt.Println("  Seed complete.")
			}

			fmt.Println("Step 5: Verifying the roundtrip is lossless...")
			return runRoundTripDiff(ctx, cfg, db, os.Stdout)
		},
	}

	return cmd
}

// dropAllTables drops every base table in the given schema, regardless
// of FK direction, on a sticky connection with foreign_key_checks off.
// Required because sync-db needs to remove tables that may not exist in
// the current migration set (post-rollback or after the schema drifted)
// as well as schema_migrations itself, which Migrate would otherwise
// see as "we're at head, nothing to do."
func dropAllTables(ctx context.Context, db *sqlx.DB, schema string) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	rows, err := conn.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = ? AND table_type = 'BASE TABLE'
	`, schema)
	if err != nil {
		return fmt.Errorf("list tables in %s: %w", schema, err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan table name: %w", err)
		}
		names = append(names, name)
	}
	_ = rows.Close()
	if len(names) == 0 {
		return nil
	}

	if _, err := conn.ExecContext(ctx, "SET foreign_key_checks = 0"); err != nil {
		return fmt.Errorf("disable foreign_key_checks: %w", err)
	}
	for _, name := range names {
		if _, err := conn.ExecContext(ctx, "DROP TABLE IF EXISTS `"+name+"`"); err != nil {
			return fmt.Errorf("drop table %s: %w", name, err)
		}
	}
	if _, err := conn.ExecContext(ctx, "SET foreign_key_checks = 1"); err != nil {
		return fmt.Errorf("re-enable foreign_key_checks: %w", err)
	}
	return nil
}

// runRoundTripDiff exports the current database state to a temp
// directory and compares it against the source YAML notebooks. Used
// by BOTH validate-db (called without any preceding writes) and
// sync-db (called after the clear+import). The function never writes
// to the database itself, so it's safe to reuse from a read-only path.
func runRoundTripDiff(ctx context.Context, cfg *config.Config, db *sqlx.DB, out io.Writer) error {
	exportDir, err := os.MkdirTemp("", "langner-validate-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(exportDir) }()

	_, _ = fmt.Fprintf(out, "Exporting current DB to %s...\n", exportDir)
	exporter := newExporterFromConfig(cfg, db, exportDir, io.Discard)
	if _, err := exporter.ExportAll(ctx); err != nil {
		return err
	}

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
		out,
	)

	if validResult.HasMismatches() {
		return fmt.Errorf("validation failed with %d mismatch(es)", len(validResult.Mismatches))
	}
	return nil
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

// newStateSeederFromConfig wires the datasync.StateSeeder for the
// migration-016 tables. Returns nil when the YAML reader can't be
// built (no notebooks configured) since no source data is available.
func newStateSeederFromConfig(cfg *config.Config, db *sqlx.DB, writer io.Writer) *datasync.StateSeeder {
	reader, err := notebook.NewReader(
		cfg.Notebooks.StoriesDirectories,
		cfg.Notebooks.FlashcardsDirectories,
		cfg.Notebooks.BooksDirectories,
		cfg.Notebooks.DefinitionsDirectories,
		cfg.Notebooks.EtymologyDirectories,
		nil,
	)
	if err != nil {
		return nil
	}
	return datasync.NewStateSeeder(
		reader,
		notebook.NewDBNoteRepository(db),
		notebook.NewDBEtymologyOriginRepository(db),
		notebook.NewDBDefinitionsRepository(db),
		notebook.NewDBFlashcardDeckRepository(db),
		notebook.NewDBSkipFlagRepository(db),
		learning.NewDBLearningRepository(db),
		learning.NewYAMLLearningRepository(cfg.Notebooks.LearningNotesDirectory, nil),
		writer,
	)
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

	imp := datasync.NewImporter(noteRepo, learningRepo, yamlRepo, yamlLearningRepo, jsonDictRepo, dictRepo, writer)
	imp = imp.WithEtymology(
		notebook.NewDBEtymologyOriginRepository(db),
		notebook.NewDBNoteOriginPartRepository(db),
		notebook.NewYAMLEtymologyOriginSource(reader),
		notebook.NewYAMLEtymologyDefinitionSource(reader),
	)
	imp = imp.WithEtymologyForms(
		notebook.NewDBEtymologyOriginFormRepository(db),
		notebook.NewYAMLEtymologyOriginFormSource(reader),
	)
	imp = imp.WithSemanticConcepts(
		notebook.NewDBSemanticConceptRepository(db),
		notebook.NewYAMLSemanticConceptSource(reader),
	)
	imp = imp.WithConceptRelations(
		notebook.NewDBConceptRelationRepository(db),
		notebook.NewYAMLConceptRelationSource(reader),
	)
	return imp.WithDefinitionConcepts(
		notebook.NewDBDefinitionConceptRepository(db),
		notebook.NewYAMLDefinitionConceptSource(reader),
	)
}

func newExporterFromConfig(cfg *config.Config, db *sqlx.DB, outputDir string, writer io.Writer) *datasync.Exporter {
	noteRepo := notebook.NewDBNoteRepository(db)
	learningRepo := learning.NewDBLearningRepository(db)
	dictRepo := dictionary.NewDBDictionaryRepository(db)
	noteSink := notebook.NewYAMLNoteRepositoryWriter(outputDir)
	learningSink := learning.NewYAMLLearningRepositoryWriter(outputDir)
	dictSink := rapidapi.NewJSONDictionaryRepositoryWriter(outputDir)

	exp := datasync.NewExporter(noteRepo, learningRepo, dictRepo, noteSink, learningSink, dictSink, writer)
	return exp.WithDefinitionConcepts(
		notebook.NewDBDefinitionConceptRepository(db),
		notebook.NewYAMLDefinitionsBookSink(outputDir),
	)
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

// dataTablesInDeletionOrder lists every persisted-data table in an
// order safe for sequential DELETE: child rows (rows whose FK points
// at another row in this list) come before their parents. The
// validate-db roundtrip clears these before re-importing. Keep in
// sync with schemas/migrations/ — a missing entry surfaces as a
// foreign-key constraint error at clear time, which is exactly the
// failure mode TestClearAllDataTablesCoversAllSchemaTables guards
// against.
//
// Order rationale:
//   - note_origin_parts depends on notes + etymology_origins + etymology_origin_forms (no CASCADE on note_id)
//   - notebook_notes, note_images, note_references, learning_logs depend on notes
//   - etymology_origin_forms depends on etymology_origins (CASCADE; listed for clarity)
//   - semantic_concept_members, concept_relations CASCADE from semantic_concepts
//   - definition_concept_members CASCADE from definition_concepts
//   - notes, etymology_origins, semantic_concepts, definition_concepts, dictionary_entries are leaf parents
func dataTablesInDeletionOrder() []string {
	return []string{
		// note_skip_flags / origin_skip_flags hang off notes / origins
		// without CASCADE on every dialect; delete them first.
		"note_skip_flags",
		"origin_skip_flags",
		"note_origin_parts",
		"notebook_notes",
		"note_images",
		"note_references",
		"learning_logs",
		"etymology_origin_forms",
		"semantic_concept_members",
		"concept_relations",
		"definition_concept_members",
		// definitions_scenes cascades from definitions_sessions, but
		// listing both explicitly keeps the FK-error story safe across
		// dialects that don't honour CASCADE in the migration.
		"definitions_scenes",
		"definitions_sessions",
		"flashcard_decks",
		"notes",
		"etymology_origins",
		"semantic_concepts",
		"definition_concepts",
		"dictionary_entries",
	}
}

// clearAllDataTables wipes every persisted-data table on a sticky
// connection with FOREIGN_KEY_CHECKS off. The SET is per-session, so
// running it on a borrowed *sqlx.DB (which pools connections) would
// leak: subsequent statements could land on a different connection
// that still has FK checks enabled. Pinning a single connection via
// db.Conn keeps the SET effective for the lifetime of the clear.
func clearAllDataTables(ctx context.Context, db *sqlx.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "SET foreign_key_checks = 0"); err != nil {
		return fmt.Errorf("disable foreign_key_checks: %w", err)
	}
	for _, table := range dataTablesInDeletionOrder() {
		if _, err := conn.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("clear table %s: %w", table, err)
		}
	}
	if _, err := conn.ExecContext(ctx, "SET foreign_key_checks = 1"); err != nil {
		return fmt.Errorf("re-enable foreign_key_checks: %w", err)
	}
	return nil
}
