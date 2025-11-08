package main

import (
	"fmt"
	"strings"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/spf13/cobra"
)

func newValidateCommand() *cobra.Command {
	var fix bool

	command := &cobra.Command{
		Use:   "validate",
		Short: "Validate learning notes and story notebooks for consistency and correctness",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configFile)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			// Create validator
			validator := notebook.NewValidator(
				cfg.Notebooks.LearningNotesDirectory,
				cfg.Notebooks.StoriesDirectory,
				cfg.Dictionaries.RapidAPI.CacheDirectory,
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

			// Exit with error code if there are validation errors
			if result.HasErrors() {
				return fmt.Errorf("validation failed with %d error(s)",
					len(result.LearningNotesErrors)+len(result.ConsistencyErrors))
			}

			return nil
		},
	}

	command.Flags().BoolVar(&fix, "fix", false, "Automatically fix validation errors")

	return command
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

