package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/at-ishikawa/langner/internal/cli"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/spf13/cobra"
)

var (
	configFile string
)

func main() {
	var debugMode bool
	rootCommand := cobra.Command{
		Use:           "langner",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			setupLogger(debugMode)
			return nil
		},
	}
	rootCommand.PersistentFlags().StringVar(&configFile, "config", "", "config file path")
	rootCommand.PersistentFlags().BoolVar(&debugMode, "debug", false, "Enable debug mode")

	rootCommand.AddCommand(
		newAnalyzeCommand(),
		newDictionaryCommand(),
		newQuizCommand(),
		newNotebookCommand(),
		newValidateCommand(),
		newParseCommand(),
		newMigrateCommand(),
		newEbookCommand(),
	)
	if err := rootCommand.Execute(); err != nil {
		if _, fprintfErr := fmt.Fprintf(os.Stderr, "failed to execute a command: %+v\n", err); fprintfErr != nil {
			panic(fmt.Errorf("failed to output an error: %w. Reason: %w", err, fprintfErr))
		}
		os.Exit(1)
	}
	os.Exit(0)
}

// setupLogger configures the default logger based on debug mode
func setupLogger(debugMode bool) {
	logLevel := slog.LevelInfo
	if debugMode {
		logLevel = slog.LevelDebug
	}

	slog.SetDefault(
		slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level:     logLevel,
			AddSource: true,
		})),
	)
}

func newMigrateCommand() *cobra.Command {
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migration commands",
	}

	migrateCmd.AddCommand(newMigrateLearningHistoryCommand())
	migrateCmd.AddCommand(newMigrateSchemaCommand())
	migrateCmd.AddCommand(newMigrateImportDBCommand())
	migrateCmd.AddCommand(newExportDBCommand())
	migrateCmd.AddCommand(newValidateDBCommand())
	migrateCmd.AddCommand(newSyncDBCommand())
	migrateCmd.AddCommand(newRecalculateIntervalsCommand())
	migrateCmd.AddCommand(newExtractDefinitionsCommand())
	migrateCmd.AddCommand(newMigrateMergeConceptsCommand())
	migrateCmd.AddCommand(newMigrateEtymologyToScenesCommand())

	return migrateCmd
}

// newMigrateEtymologyToScenesCommand wires the `langner migrate
// etymology-to-scenes` CLI. The migration rewrites legacy flat-shape
// etymology session YAML files into the explicit event/scenes/origins
// shape. Each origin lands in the earliest scene whose vocab references
// it via origin_parts (earliest by session order in index.yml, then by
// scene order in the definitions file). Origins not referenced by any
// definition fall into a synthetic scene named after the origin.
//
// Idempotent: files already in the new shape are skipped. --dry-run
// reports what would change without writing anything.
func newMigrateEtymologyToScenesCommand() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "etymology-to-scenes",
		Short: "Rewrite legacy flat-shape etymology session files into the explicit event/scenes/origins shape",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			return cli.MigrateEtymologyToScenes(
				cfg.Notebooks.EtymologyDirectories,
				cfg.Notebooks.DefinitionsDirectories,
				dryRun,
			)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report planned migrations without writing files")
	return cmd
}

func newExtractDefinitionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extract-definitions",
		Short: "Extract definitions from story notebooks into separate definition files",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			if len(cfg.Notebooks.DefinitionsDirectories) == 0 {
				return fmt.Errorf("no definitions_directories configured")
			}

			return cli.ExtractDefinitions(
				cfg.Notebooks.StoriesDirectories,
				cfg.Notebooks.DefinitionsDirectories[0],
			)
		},
	}
	return cmd
}

func newMigrateLearningHistoryCommand() *cobra.Command {
	var recalculate bool
	cmd := &cobra.Command{
		Use:   "learning-history",
		Short: "Migrate learning history files to current format",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			calculator := notebook.NewIntervalCalculator(cfg.Quiz.Algorithm, cfg.Quiz.FixedIntervals)
			return cli.MigrateLearningHistory(cfg.Notebooks.LearningNotesDirectory, recalculate, calculator)
		},
	}
	cmd.Flags().BoolVar(&recalculate, "recalculate", false, "Force recalculation of intervals for all learning history entries using the configured algorithm")
	return cmd
}

// newMigrateMergeConceptsCommand wires the `langner migrate merge-concepts`
// CLI. The operation is destructive and one-way: every non-head member of a
// definitions concept has its learning history entry folded into the head
// and is then dropped from the YAML.
//
// Users must commit langner-data state before running so they can revert.
// --dry-run reports planned changes without writing files.
func newMigrateMergeConceptsCommand() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "merge-concepts",
		Short: "Merge concept member learning histories into the head expression (DESTRUCTIVE, one-way)",
		Long: `Destructively folds per-member learning history entries into the head
expression of each definitions concept. Members' logs are merged
chronologically into the head; skip timestamps union; the head's newest
log interval_days is rewritten to min across all members. Non-head
member entries are then removed.

This operation is ONE-WAY. Commit your learning_notes directory to
version control before running so you can revert if needed.

Use --dry-run to preview the changes without writing any files.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			return cli.MergeConcepts(
				cfg.Notebooks.LearningNotesDirectory,
				cfg.Notebooks.DefinitionsDirectories,
				dryRun,
			)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report planned merges without writing files")
	return cmd
}

func newRecalculateIntervalsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recalculate-intervals",
		Short: "Recalculate intervals for all learning history using the configured algorithm",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			return cli.RecalculateIntervals(
				cfg.Notebooks.LearningNotesDirectory,
				cfg.Quiz.Algorithm,
				cfg.Quiz.FixedIntervals,
			)
		},
	}
	return cmd
}
