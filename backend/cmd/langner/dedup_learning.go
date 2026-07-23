package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// newDedupLearningIDsCommand wires `langner migrate dedup-learning-ids`.
//
// It repairs learning histories where a quiz write forked a new id-less entry
// next to the existing id-bearing series for the same expression (the
// concept-redirect / lost-card-id bug). Each id-less fork's logs are merged
// into its id-bearing sibling and the duplicate is removed.
func newDedupLearningIDsCommand() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "dedup-learning-ids",
		Short: "Merge id-less duplicate learning entries into their id-bearing sibling",
		Long: `Repairs learning histories where a quiz created a new id-less entry
alongside the existing id-bearing series for the same expression. Within each
scene, when exactly one id-bearing entry and one or more id-less entries share
an expression, the id-less entries' logs are merged into the id-bearing entry
(newest-first) and the duplicates are removed. Homographs (two distinct ids)
and pure-legacy id-less entries are left untouched.

Commit your notebook data to version control first. Use --dry-run to preview.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			return dedupLearningIDs(cfg, dryRun, os.Stdout)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report counts without writing any files")
	return cmd
}

func dedupLearningIDs(cfg *config.Config, dryRun bool, w io.Writer) error {
	dir := cfg.Notebooks.LearningNotesDirectory
	if dir == "" {
		return fmt.Errorf("no learning_notes_directory configured")
	}
	histories, err := notebook.NewLearningHistories(dir)
	if err != nil {
		return fmt.Errorf("load learning histories: %w", err)
	}
	names := make([]string, 0, len(histories))
	for name := range histories {
		names = append(names, name)
	}
	sort.Strings(names)

	total := 0
	for _, name := range names {
		list := histories[name]
		n := notebook.MergeIDLessDuplicates(list, nil)
		if n == 0 {
			continue
		}
		total += n
		_, _ = fmt.Fprintf(w, "  merged %d id-less duplicate(s) in %s\n", n, name)
		if dryRun {
			continue
		}
		path := filepath.Join(dir, name+".yml")
		if err := notebook.WriteYamlFile(path, list); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	suffix := ""
	if dryRun {
		suffix = " (dry-run — nothing written)"
	}
	_, _ = fmt.Fprintf(w, "Merged %d id-less duplicate entr(y/ies)%s\n", total, suffix)
	return nil
}
