package clicmd

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

// SetupLogger configures the default slog logger based on debug mode. Both
// binaries call it from their root command's PersistentPreRunE.
func SetupLogger(debugMode bool) {
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

// NewMigrateCommand wires the admin `langner-admin migrate` command group with
// the schema/data lifecycle subcommands. The obsolete one-off data migrations
// (learning-history, assign-ids, dedup-learning-ids, extract-definitions,
// merge-concepts, etymology-to-scenes, recalculate-intervals) have been removed.
func NewMigrateCommand() *cobra.Command {
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migration commands",
	}

	migrateCmd.AddCommand(newMigrateSchemaCommand())
	migrateCmd.AddCommand(newMigrateRollbackCommand())
	migrateCmd.AddCommand(newMigrateImportDBCommand())
	migrateCmd.AddCommand(newMigrateResetDBCommand())
	migrateCmd.AddCommand(newExportDBCommand())
	migrateCmd.AddCommand(newValidateDBCommand())
	migrateCmd.AddCommand(newSyncDBCommand())

	return migrateCmd
}
