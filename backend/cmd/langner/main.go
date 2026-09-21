package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/internal/clicmd"
)

// langner is the end-user CLI. It exposes only the commands a learner needs:
// validating their notebooks and managing cloned ebook repositories. The admin
// and e2e tooling (migrate, auth, notebooks set-owner) lives in the separate
// langner-admin binary.
func main() {
	var debugMode bool
	rootCommand := cobra.Command{
		Use:           "langner",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			clicmd.SetupLogger(debugMode)
			return nil
		},
	}
	rootCommand.PersistentFlags().StringVar(&clicmd.ConfigFile, "config", "", "config file path")
	rootCommand.PersistentFlags().BoolVar(&debugMode, "debug", false, "Enable debug mode")

	rootCommand.AddCommand(
		clicmd.NewValidateCommand(),
		clicmd.NewEbookCommand(),
		clicmd.NewLoginCommand(),
		clicmd.NewLogoutCommand(),
		clicmd.NewWhoamiCommand(),
	)
	if err := rootCommand.Execute(); err != nil {
		if _, fprintfErr := fmt.Fprintf(os.Stderr, "failed to execute a command: %+v\n", err); fprintfErr != nil {
			panic(fmt.Errorf("failed to output an error: %w. Reason: %w", err, fprintfErr))
		}
		os.Exit(1)
	}
	os.Exit(0)
}
