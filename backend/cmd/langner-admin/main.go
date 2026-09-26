package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/internal/clicmd"
)

// langner-admin is the admin + e2e CLI. It exposes the schema/data lifecycle
// (migrate), auth account provisioning and test-cookie helpers (auth), and
// notebook ownership (notebooks set-owner) — the operations an operator or the
// e2e harness runs, kept out of the end-user langner binary.
func main() {
	var debugMode bool
	rootCommand := cobra.Command{
		Use:           "langner-admin",
		Short:         "Administrative and e2e tooling for langner (schema/data migrations, auth, notebook ownership)",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			clicmd.SetupLogger(debugMode)
			return nil
		},
	}
	rootCommand.PersistentFlags().StringVar(&clicmd.ConfigFile, "config", "", "config file path")
	rootCommand.PersistentFlags().BoolVar(&debugMode, "debug", false, "Enable debug mode")

	notebooksCommand := &cobra.Command{
		Use:   "notebooks",
		Short: "Notebook administration",
	}
	notebooksCommand.AddCommand(
		clicmd.NewNotebooksSetOwnerCommand(),
		clicmd.NewNotebooksImportFilesystemCommand(),
	)

	rootCommand.AddCommand(
		clicmd.NewMigrateCommand(),
		clicmd.NewAuthCommand(),
		notebooksCommand,
	)
	if err := rootCommand.Execute(); err != nil {
		if _, fprintfErr := fmt.Fprintf(os.Stderr, "failed to execute a command: %+v\n", err); fprintfErr != nil {
			panic(fmt.Errorf("failed to output an error: %w. Reason: %w", err, fprintfErr))
		}
		os.Exit(1)
	}
	os.Exit(0)
}
