package main

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// newMigrateBackfillSensesCommand builds `langner migrate backfill-senses`.
//
// It performs a one-shot, best-effort pass over every learning-history YAML
// file in the configured learning-notes directory, stamping each vocabulary
// entry's part_of_speech from its source note when the resolution is
// unambiguous (exactly one sense). Genuine homographs (2+ differing senses)
// are left untagged so no commingled history is guessed apart. No log is ever
// moved between entries.
//
// Register in the migrate command tree (main.go) with:
//
//	migrateCmd.AddCommand(newMigrateBackfillSensesCommand())
func newMigrateBackfillSensesCommand() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "backfill-senses",
		Short: "Stamp part_of_speech on single-sense learning-history entries (homographs left legacy)",
		Long: `Backfill the part_of_speech discriminator onto existing learning-history
YAML entries.

For each vocabulary entry, the source notes for that expression in the same
notebook are resolved:

  - Exactly one distinct sense -> the entry is stamped with it (non-homographs
    get full continuity and key identically to future writes).
  - Two or more distinct senses (a real homograph) -> the entry is left
    untagged (legacy). New sense-tagged answers create fresh per-sense series
    going forward.

No log is ever moved between entries, so no history is corrupted. Safe to
re-run: already-tagged entries are skipped.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			// YAML-only maintenance: read source notes and rewrite learning
			// histories in place. No database connection required.
			loader, err := config.NewConfigLoader(configFile)
			if err != nil {
				return fmt.Errorf("load config loader: %w", err)
			}
			cfg, err := loader.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			sourceNotes, err := readNotesFromDirs(ctx,
				cfg.Notebooks.StoriesDirectories,
				cfg.Notebooks.FlashcardsDirectories,
				cfg.Notebooks.BooksDirectories,
				cfg.Notebooks.DefinitionsDirectories,
			)
			if err != nil {
				return fmt.Errorf("read source notes: %w", err)
			}

			learningDir := cfg.Notebooks.LearningNotesDirectory
			histories, err := notebook.NewLearningHistories(learningDir)
			if err != nil {
				return fmt.Errorf("load learning histories: %w", err)
			}

			// Deterministic file order for stable output.
			names := make([]string, 0, len(histories))
			for name := range histories {
				names = append(names, name)
			}
			sort.Strings(names)

			var totalTagged, totalLegacy int
			for _, name := range names {
				fileHistories := histories[name]
				res := notebook.BackfillSenses(fileHistories, sourceNotes)
				totalTagged += res.Tagged
				totalLegacy += res.LeftLegacy
				if res.Tagged == 0 {
					continue
				}
				fmt.Printf("  %s: tagged %d, left legacy %d\n", name, res.Tagged, res.LeftLegacy)
				if dryRun {
					continue
				}
				path := filepath.Join(learningDir, name+".yml")
				if err := notebook.WriteYamlFile(path, fileHistories); err != nil {
					return fmt.Errorf("write %s: %w", path, err)
				}
			}

			verb := "Stamped"
			if dryRun {
				verb = "Would stamp"
			}
			fmt.Printf("%s %d entr(ies); left %d homograph entr(ies) as legacy.\n", verb, totalTagged, totalLegacy)
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would change without writing files")
	return cmd
}
