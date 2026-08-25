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

			// Preflight BEFORE Migrate: report the connection target and fail
			// clearly on an empty search_path (Supabase transaction pooler,
			// port 6543) rather than letting golang-migrate's pgx init crash
			// with "converting NULL to string" on current_schema().
			if err := printPreflightBanner(ctx, cfg, db, os.Stdout); err != nil {
				return err
			}

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

			// Pre-flight: fail LOUDLY and CLEARLY on schema/version drift
			// before touching any data. golang-migrate trusts the integer
			// version in schema_migrations and never verifies the actual
			// columns, so a DB built by an older/renumbered migration chain
			// slips past Migrate and then crashes deep in a column scan with
			// a cryptic error (`missing destination name part_of_speech`,
			// `column sense_id does not exist`). VerifySchema turns that into
			// one actionable message naming the offending table/column.
			if err := database.VerifySchema(db, schemas.Migrations, "migrations"); err != nil {
				return err
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

			// Seed the DB-only state tables (definitions sessions/scenes,
			// flashcard decks, per-quiz-type skip flags, etymology origin
			// logs) from the same YAML the importer just consumed. The
			// seeder is idempotent so re-runs only insert what's missing.
			if !opts.DryRun {
				if seeder := newStateSeederFromConfig(cfg, db, os.Stdout); seeder != nil {
					stateResult, serr := seeder.SeedAll(ctx)
					if serr != nil {
						return fmt.Errorf("seed db-only state: %w", serr)
					}
					fmt.Println("\nState Seed Summary:")
					fmt.Printf("  Definitions sessions: %d new\n", stateResult.DefinitionsSessionsCreated)
					fmt.Printf("  Definitions scenes:   %d new\n", stateResult.DefinitionsScenesCreated)
					fmt.Printf("  Flashcard decks:      %d new\n", stateResult.FlashcardDecksCreated)
					fmt.Printf("  Note skip flags:      %d new\n", stateResult.NoteSkipFlagsCreated)
					fmt.Printf("  Origin skip flags:    %d new\n", stateResult.OriginSkipFlagsCreated)
					fmt.Printf("  Etymology logs:       %d new\n", stateResult.EtymologyLogsCreated)
					fmt.Printf("  Grammar corrections:  %d new\n", stateResult.GrammarCorrectionsCreated)
					fmt.Printf("  Grammar logs:         %d new\n", stateResult.GrammarLogsCreated)
				}
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
		Short: "Export database to YAML files (notebook shapes + a complete, lossless per-table snapshot)",
		Long: `Export the database to YAML under the --output directory.

Two things are written:

  1. The notebook-shaped YAML the app reads (stories/, books/, flashcards/,
     definitions/, learning_notes/, dictionaries/). This is convenient for
     re-import but is NOT a lossless mirror of the DB — it cannot represent
     DB-only columns (note ids, skipped_at, the etymology junction tables)
     and drops note-body fields the DB never stores.

  2. A faithful, complete per-table snapshot under tables/<table>.yml: every
     row of every persisted-data table, every column, exactly as stored.
     This is the diffable, recoverable backup to take before moving to
     DB-only state — nothing is silently lost.`,
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

			// Faithful, complete per-table snapshot under <output>/tables/.
			// The notebook-shaped export above reconstructs the app's YAML
			// formats but cannot represent DB-only columns (note ids,
			// skipped_at, the etymology junction tables) and drops note-body
			// fields the DB never stores — so it is not a lossless mirror of
			// the database. This dump captures every row of every table
			// exactly, which is what makes the data recoverable and diffable
			// before the move to DB-only state.
			tableResult, err := datasync.NewTableDumpExporter(db, outputDir, os.Stdout).ExportTables(ctx)
			if err != nil {
				return fmt.Errorf("export tables: %w", err)
			}

			fmt.Println("\nExport Summary:")
			fmt.Printf("  Notes exported:              %d\n", result.Notes.NotesExported)
			fmt.Printf("  Learning logs exported:      %d\n", result.Learning.LogsExported)
			fmt.Printf("  Dictionary entries exported: %d\n", result.Dictionary.EntriesExported)
			fmt.Printf("  Tables snapshotted:          %d (see %s)\n",
				len(tableResult.RowsByTable), filepath.Join(outputDir, "tables"))
			totalRows := 0
			for _, n := range tableResult.RowsByTable {
				totalRows += n
			}
			fmt.Printf("  Total rows snapshotted:      %d\n", totalRows)

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

func newMigrateSchemaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Apply pending schema migrations only (no data import)",
		Long: `Apply all pending schema migrations to the database configured in the
config file. Unlike "import-db", this runs ONLY the embedded schema
migrations — it does not import or reconcile any notebook data.

The database connection is read from --config, so no DATABASE_URL is
needed. Idempotent: a no-op when the schema is already up to date.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			if err := database.Migrate(db, schemas.Migrations, "migrations"); err != nil {
				return fmt.Errorf("apply schema migrations: %w", err)
			}
			fmt.Println("Schema migrations applied (or already up to date).")
			return nil
		},
	}
	return cmd
}

// newMigrateResetDBCommand rebuilds the database to the seeded baseline in
// one shot: rebuild langner's managed tables from scratch (scoped drop +
// migrate), re-import the source YAML, and re-seed the DB-only state tables.
// Unlike sync-db it skips the export/roundtrip diff — it is meant for the e2e
// harness to restore per-scenario isolation quickly (the diff would add latency
// and can be sensitive to fixtures the app has just mutated). It is the DB half
// of a reset; the harness restores the mutated learning_notes YAML separately.
// The scoped rebuild (rather than the old in-place TRUNCATE) also makes reset-db
// drift/dirty-tolerant, so a stale or half-migrated harness DB self-heals.
func newMigrateResetDBCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reset-db",
		Short: "Reset the database to the seeded baseline (scoped drop + migrate + import + seed, no roundtrip diff)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			if err := printPreflightBanner(ctx, cfg, db, os.Stdout); err != nil {
				return err
			}
			if err := rebuildManagedSchema(ctx, db, os.Stdout); err != nil {
				return err
			}
			if err := database.Migrate(db, schemas.Migrations, "migrations"); err != nil {
				return fmt.Errorf("apply schema migrations: %w", err)
			}
			importer := newImporterFromConfig(cfg, db, io.Discard)
			if _, err := importer.ImportAll(ctx, datasync.ImportOptions{UpdateExisting: true}); err != nil {
				return fmt.Errorf("import source yaml: %w", err)
			}
			if seeder := newStateSeederFromConfig(cfg, db, io.Discard); seeder != nil {
				if _, err := seeder.SeedAll(ctx); err != nil {
					return fmt.Errorf("seed db-only state: %w", err)
				}
			}
			return nil
		},
	}
}

func newSyncDBCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync-db",
		Short: "Rebuild database from source YAML (destructive: drops + re-migrates langner's managed tables first)",
		Long: `Make the database match the source YAML files. This is a destructive
operation, SCOPED to langner's own tables (safe on a shared/Supabase
database — auth/storage and other apps are never touched):

  1. REBUILD langner's managed tables from scratch: drop every managed
     table (and schema_migrations) with DROP TABLE IF EXISTS ... CASCADE,
     then re-apply all schema migrations onto the empty set. This resets
     golang-migrate to a clean version 0, so a database left DIRTY by a
     half-applied migration, or built by an OLDER/renumbered migration
     chain (drift), is repaired instead of failing in place.
  2. Import all source YAML notebooks into the now-empty database.
  3. Seed the DB-only state tables from the same YAML.
  4. Export the database back to a temporary directory and diff it against
     the source YAML to verify the roundtrip is lossless.

Because step 1 drops and re-migrates, running sync-db is idempotent — it
succeeds twice in a row and on a drifted/dirty schema. Use it when the
database has drifted from the YAML and you want YAML to win, or after a
schema change to re-seed from a clean slate. To check current divergence
WITHOUT modifying the database, use "migrate validate-db" instead.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			// Preflight BEFORE Migrate so an empty search_path (Supabase
			// transaction pooler) is reported clearly instead of surfacing as
			// golang-migrate's cryptic "converting NULL to string" panic.
			if err := printPreflightBanner(ctx, cfg, db, os.Stdout); err != nil {
				return err
			}

			// Rebuild the managed schema from scratch, then migrate. Replaces
			// the old in-place Migrate + TRUNCATE, which could not self-heal a
			// drifted or dirty schema (migration 022's DROP CONSTRAINT hit a
			// constraint the drifted schema names differently and hard-failed,
			// leaving schema_migrations dirty).
			fmt.Println("Step 1: Rebuilding langner-managed tables from scratch...")
			if err := rebuildManagedSchema(ctx, db, os.Stdout); err != nil {
				return err
			}
			if err := database.Migrate(db, schemas.Migrations, "migrations"); err != nil {
				return fmt.Errorf("apply schema migrations: %w", err)
			}
			fmt.Println("  Rebuild complete.")

			fmt.Println("Step 2: Importing source YAML into the empty database...")
			importer := newImporterFromConfig(cfg, db, io.Discard)
			if _, err := importer.ImportAll(ctx, datasync.ImportOptions{UpdateExisting: true}); err != nil {
				return err
			}
			fmt.Println("  Import complete.")

			if seeder := newStateSeederFromConfig(cfg, db, io.Discard); seeder != nil {
				fmt.Println("Step 3: Seeding DB-only state tables from YAML...")
				if _, err := seeder.SeedAll(ctx); err != nil {
					return fmt.Errorf("seed db-only state: %w", err)
				}
				fmt.Println("  Seed complete.")
			}

			fmt.Println("Step 4: Verifying the roundtrip is lossless...")
			return runRoundTripDiff(ctx, cfg, db, os.Stdout)
		},
	}

	return cmd
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
	exp = exp.WithEtymologyOrigins(notebook.NewDBEtymologyOriginRepository(db))
	return exp.WithDefinitionConcepts(
		notebook.NewDBDefinitionConceptRepository(db),
		notebook.NewYAMLDefinitionsBookSink(outputDir),
	)
}

// newStateSeederFromConfig wires the datasync.StateSeeder used by
// import-db and sync-db to populate the DB-only state tables from YAML.
// Returns nil when the notebook reader can't be constructed.
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
		notebook.NewDBGrammarCorrectionRepository(db),
		learning.NewDBLearningRepository(db),
		learning.NewYAMLLearningRepository(cfg.Notebooks.LearningNotesDirectory, nil),
		cfg.Notebooks.LearningNotesDirectory,
		writer,
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
