package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/at-ishikawa/langner/internal/cli"
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
	migrateCmd.AddCommand(newMigrateImportDBCommand())
	migrateCmd.AddCommand(newExportDBCommand())
	migrateCmd.AddCommand(newValidateDBCommand())
	migrateCmd.AddCommand(newRecalculateIntervalsCommand())

	return migrateCmd
}

func newMigrateLearningHistoryCommand() *cobra.Command {
	var recalculateSM2 bool
	cmd := &cobra.Command{
		Use:   "learning-history",
		Short: "Migrate learning history files to new SM-2 format",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			return cli.MigrateLearningHistory(cfg.Notebooks.LearningNotesDirectory, recalculateSM2)
		},
	}
	cmd.Flags().BoolVar(&recalculateSM2, "recalculate-sm2", false, "Force recalculation of SM-2 metrics (EF and intervals) for all learning history entries")
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
