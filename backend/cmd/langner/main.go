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

	return migrateCmd
}

func newMigrateLearningHistoryCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "learning-history",
		Short: "Migrate learning history files to new SM-2 format",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			return cli.MigrateLearningHistory(cfg.Notebooks.LearningNotesDirectory)
		},
	}
}
