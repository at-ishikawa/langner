package main

import (
	"fmt"

	"github.com/at-ishikawa/langner/internal/cli"
	"github.com/spf13/cobra"
)

func newAnalyzeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze learning progress and statistics",
	}
	cmd.AddCommand(newAnalyzeReportCommand())
	return cmd
}

func newAnalyzeReportCommand() *cobra.Command {
	var year, month int

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Show monthly/yearly report of learning statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			if month != 0 && year == 0 {
				return fmt.Errorf("--month requires --year to be specified")
			}
			if month < 0 || month > 12 {
				return fmt.Errorf("--month must be between 1 and 12")
			}

			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			return cli.RunAnalyzeReport(cfg.Notebooks.LearningNotesDirectory, year, month)
		},
	}

	cmd.Flags().IntVar(&year, "year", 0, "Filter by year (e.g., 2025)")
	cmd.Flags().IntVar(&month, "month", 0, "Filter by month (1-12), requires --year")

	return cmd
}
