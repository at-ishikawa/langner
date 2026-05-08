package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/datasync"
	"github.com/at-ishikawa/langner/internal/notebook"
)

func newValidateCommand() *cobra.Command {
	var fix bool

	command := &cobra.Command{
		Use:   "validate",
		Short: "Validate learning notes and story notebooks for consistency and correctness",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			// Create validator with configured interval calculator
			calculator := notebook.NewIntervalCalculator(cfg.Quiz.Algorithm, cfg.Quiz.FixedIntervals)
			validator := notebook.NewValidator(
				cfg.Notebooks.LearningNotesDirectory,
				cfg.Notebooks.StoriesDirectories,
				cfg.Notebooks.FlashcardsDirectories,
				cfg.Notebooks.DefinitionsDirectories,
				cfg.Notebooks.EtymologyDirectories,
				cfg.Dictionaries.RapidAPI.CacheDirectory,
				calculator,
			)

			var result *notebook.ValidationResult

			if fix {
				// Run auto-fix
				fmt.Println("Running auto-fix...")
				fixResult, err := validator.Fix()
				if err != nil {
					return fmt.Errorf("validator.Fix() > %w", err)
				}

				// Display what was fixed
				fixCount := len(fixResult.Warnings)
				if fixCount > 0 {
					fmt.Printf("Auto-fix completed. %d change(s) made.\n", fixCount)
				} else {
					fmt.Println("Auto-fix completed. No changes needed.")
				}
				fmt.Println()

				// Re-validate to check for remaining issues
				fmt.Println("Re-validating files...")
				result, err = validator.Validate()
				if err != nil {
					return fmt.Errorf("validator.Validate() > %w", err)
				}
			} else {
				// Run validation only
				result, err = validator.Validate()
				if err != nil {
					return fmt.Errorf("validator.Validate() > %w", err)
				}
			}

			// Display results
			displayValidationResults(result)

			// DB consistency validation (auto-detect)
			dbResult, err := validateDBConsistency(cmd.Context(), cfg)
			if err != nil {
				fmt.Printf("Warning: DB consistency check failed: %v\n\n", err)
			}

			// Exit with error code if there are validation errors
			if result.HasErrors() {
				return fmt.Errorf("validation failed with %d error(s)",
					len(result.LearningNotesErrors)+len(result.ConsistencyErrors))
			}
			if dbResult != nil && dbResult.HasMismatches() {
				return fmt.Errorf("DB consistency validation failed with %d mismatch(es)", len(dbResult.Mismatches))
			}

			return nil
		},
	}

	command.Flags().BoolVar(&fix, "fix", false, "Automatically fix validation errors")

	return command
}

// isDBConfigured returns true if the database password is set.
func isDBConfigured(cfg config.DatabaseConfig) bool {
	return cfg.Password != ""
}

// validateDBConsistency exports DB data to a temp directory and compares against YAML source.
// Returns nil result if DB is not configured.
func validateDBConsistency(ctx context.Context, cfg *config.Config) (*datasync.ValidateResult, error) {
	if !isDBConfigured(cfg.Database) {
		return nil, nil
	}

	fmt.Println("=== DB Consistency Validation ===")
	fmt.Println("Database is configured. Comparing YAML files against DB data...")

	db, err := openDB(cfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Export DB to temp dir (read-only: only SELECTs)
	exportDir, err := os.MkdirTemp("", "langner-validate-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(exportDir) }()

	exporter := newExporterFromConfig(cfg, db, exportDir, io.Discard)
	if _, err := exporter.ExportAll(ctx); err != nil {
		return nil, fmt.Errorf("export DB data: %w", err)
	}

	// Read source YAML data
	sourceNotes, err := readNotesFromDirs(ctx,
		cfg.Notebooks.StoriesDirectories,
		cfg.Notebooks.FlashcardsDirectories,
		cfg.Notebooks.BooksDirectories,
		cfg.Notebooks.DefinitionsDirectories,
	)
	if err != nil {
		return nil, fmt.Errorf("read source notes: %w", err)
	}

	// Read exported DB data
	exportedNotes, err := readNotesFromDirs(ctx,
		[]string{filepath.Join(exportDir, "stories")},
		[]string{filepath.Join(exportDir, "flashcards")},
		[]string{filepath.Join(exportDir, "books")},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("read exported notes: %w", err)
	}

	sourceLearning := readLearningByNotebook(sourceNotes, cfg.Notebooks.LearningNotesDirectory)
	exportedLearning := readLearningByNotebook(exportedNotes, filepath.Join(exportDir, "learning_notes"))

	sourceDictCount := countDictEntries(cfg.Dictionaries.RapidAPI.CacheDirectory)
	exportedDictCount := countDictEntries(filepath.Join(exportDir, "dictionaries", "rapidapi"))

	result := datasync.ValidateRoundTrip(
		sourceNotes, exportedNotes,
		sourceLearning, exportedLearning,
		sourceDictCount, exportedDictCount,
		os.Stdout,
	)

	return result, nil
}

func displayValidationResults(result *notebook.ValidationResult) {
	totalErrors := len(result.LearningNotesErrors) + len(result.ConsistencyErrors)
	totalWarnings := len(result.Warnings)

	fmt.Println("\n=== Validation Results ===")

	// Display learning notes errors
	if len(result.LearningNotesErrors) > 0 {
		fmt.Printf("✗ Learning Notes Validation Errors (%d):\n", len(result.LearningNotesErrors))
		for _, err := range result.LearningNotesErrors {
			fmt.Printf("  - %s\n", err.Error())
		}
		fmt.Println()
	}

	// Display consistency errors
	if len(result.ConsistencyErrors) > 0 {
		fmt.Printf("✗ Consistency Validation Errors (%d):\n", len(result.ConsistencyErrors))

		// Group errors by type
		orphanedErrors := []notebook.ValidationError{}
		duplicateErrors := []notebook.ValidationError{}
		missingSceneErrors := []notebook.ValidationError{}
		dictionaryErrors := []notebook.ValidationError{}
		otherErrors := []notebook.ValidationError{}

		for _, err := range result.ConsistencyErrors {
			if strings.Contains(err.Message, "orphaned learning note") {
				orphanedErrors = append(orphanedErrors, err)
			} else if strings.Contains(err.Message, "duplicate expression") {
				duplicateErrors = append(duplicateErrors, err)
			} else if strings.Contains(err.Message, "scene") && strings.Contains(err.Message, "not found") {
				missingSceneErrors = append(missingSceneErrors, err)
			} else if strings.Contains(err.Message, "dictionary") {
				dictionaryErrors = append(dictionaryErrors, err)
			} else {
				otherErrors = append(otherErrors, err)
			}
		}

		if len(orphanedErrors) > 0 {
			fmt.Printf("\n  Orphaned learning notes (%d):\n", len(orphanedErrors))
			for _, err := range orphanedErrors {
				fmt.Printf("    - %s\n", err.Error())
			}
		}

		if len(duplicateErrors) > 0 {
			fmt.Printf("\n  Duplicate expressions (%d):\n", len(duplicateErrors))
			for _, err := range duplicateErrors {
				fmt.Printf("    - %s\n", err.Error())
			}
		}

		if len(missingSceneErrors) > 0 {
			fmt.Printf("\n  Missing or mismatched scenes (%d):\n", len(missingSceneErrors))
			for _, err := range missingSceneErrors {
				fmt.Printf("    - %s\n", err.Error())
			}
		}

		if len(dictionaryErrors) > 0 {
			fmt.Printf("\n  Dictionary reference errors (%d):\n", len(dictionaryErrors))
			for _, err := range dictionaryErrors {
				fmt.Printf("    - %s\n", err.Error())
			}
		}

		if len(otherErrors) > 0 {
			fmt.Printf("\n  Other errors (%d):\n", len(otherErrors))
			for _, err := range otherErrors {
				fmt.Printf("    - %s\n", err.Error())
			}
		}

		fmt.Println()
	}

	// Display warnings
	if len(result.Warnings) > 0 {
		fmt.Printf("⚠ Warnings (%d):\n", len(result.Warnings))

		// Group warnings
		missingLearningNotes := []notebook.ValidationError{}
		noLogsWarnings := []notebook.ValidationError{}
		otherWarnings := []notebook.ValidationError{}

		for _, warn := range result.Warnings {
			if strings.Contains(warn.Message, "missing learning note") {
				missingLearningNotes = append(missingLearningNotes, warn)
			} else if strings.Contains(warn.Message, "no learned_logs") {
				noLogsWarnings = append(noLogsWarnings, warn)
			} else {
				otherWarnings = append(otherWarnings, warn)
			}
		}

		if len(missingLearningNotes) > 0 {
			fmt.Printf("\n  Missing learning notes (%d):\n", len(missingLearningNotes))
			// Show only first 10 to avoid cluttering output
			displayCount := len(missingLearningNotes)
			if displayCount > 10 {
				displayCount = 10
			}
			for i := 0; i < displayCount; i++ {
				fmt.Printf("    - %s\n", missingLearningNotes[i].Error())
			}
			if len(missingLearningNotes) > 10 {
				fmt.Printf("    ... and %d more\n", len(missingLearningNotes)-10)
			}
		}

		if len(noLogsWarnings) > 0 {
			fmt.Printf("\n  Expressions without learning logs (%d):\n", len(noLogsWarnings))
			for _, warn := range noLogsWarnings {
				fmt.Printf("    - %s\n", warn.Error())
			}
		}

		if len(otherWarnings) > 0 {
			fmt.Printf("\n  Other warnings (%d):\n", len(otherWarnings))
			for _, warn := range otherWarnings {
				fmt.Printf("    - %s\n", warn.Error())
			}
		}

		fmt.Println()
	}

	// Display summary
	fmt.Println("=== Summary ===")
	if totalErrors == 0 && totalWarnings == 0 {
		fmt.Println("✓ All validations passed!")
	} else {
		if totalErrors > 0 {
			fmt.Printf("✗ Total errors: %d\n", totalErrors)
		}
		if totalWarnings > 0 {
			fmt.Printf("⚠ Total warnings: %d\n", totalWarnings)
		}
	}
	fmt.Println()
}

