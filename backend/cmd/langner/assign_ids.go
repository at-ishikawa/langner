package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// newAssignIDsCommand wires `langner migrate assign-ids`.
//
// It assigns a stable, globally-unique id to every source vocabulary entry
// that lacks one, writes those ids back into the hand-authored source YAML
// add-only (only new `id:` lines appear in the diff — see
// notebook.AddIDsToSourceYAML), and then best-effort re-keys the existing
// learning histories so pre-migration logs attach to the right id.
//
// --dry-run reports what would be assigned and writes nothing.
func newAssignIDsCommand() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "assign-ids",
		Short: "Assign stable ids to source vocabulary entries and re-key learning history",
		Long: `Assigns a globally-unique, readable slug id to every source vocabulary
entry that doesn't already have one, writing the ids back into the source
YAML add-only (no other line is reformatted). Then best-effort re-keys the
learning histories: an id-less learning entry whose expression matches
exactly one source entry in its notebook is stamped with that entry's id;
ambiguous duplicates are left id-less and split on the next answer.

Commit your notebook data to version control before running so you can
revert. Use --dry-run to preview the counts without writing any files.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			return assignIDs(cmd.Context(), cfg, dryRun, os.Stdout)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report counts without writing any files")
	return cmd
}

// assignIDs implements the assign-ids command against a loaded config.
func assignIDs(ctx context.Context, cfg *config.Config, dryRun bool, w io.Writer) error {
	// Source vocabulary lives under the stories, flashcards, books, and
	// definitions directories. Etymology YAML has a different shape and is
	// left untouched.
	var sourceDirs []string
	sourceDirs = append(sourceDirs, cfg.Notebooks.StoriesDirectories...)
	sourceDirs = append(sourceDirs, cfg.Notebooks.FlashcardsDirectories...)
	sourceDirs = append(sourceDirs, cfg.Notebooks.BooksDirectories...)
	sourceDirs = append(sourceDirs, cfg.Notebooks.DefinitionsDirectories...)

	files, err := collectYAMLFiles(sourceDirs)
	if err != nil {
		return fmt.Errorf("collect source files: %w", err)
	}

	// Seed the used-set with every existing id across all files first so a new
	// slug never collides with an id another file already carries.
	used := make(map[string]bool)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		ids, err := notebook.CollectExistingIDs(data)
		if err != nil {
			return fmt.Errorf("scan ids in %s: %w", f, err)
		}
		for _, id := range ids {
			used[id] = true
		}
	}

	// Assign + write ids into the source YAML add-only.
	totalAdded := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		out, added, err := notebook.AddIDsToSourceYAML(data, used)
		if err != nil {
			return fmt.Errorf("assign ids in %s: %w", f, err)
		}
		if added == 0 {
			continue
		}
		totalAdded += added
		if !dryRun {
			// Cloud-mount safe: AddIDsToSourceYAML returned a fully-encoded
			// buffer; a single WriteFile replaces the file.
			if err := os.WriteFile(f, out, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", f, err)
			}
		}
		_, _ = fmt.Fprintf(w, "  +%d id(s) in %s\n", added, f)
	}

	suffix := ""
	if dryRun {
		suffix = " (dry-run — nothing written)"
	}
	_, _ = fmt.Fprintf(w, "Assigned %d new source id(s)%s\n", totalAdded, suffix)

	if dryRun {
		_, _ = fmt.Fprintln(w, "Skipping learning-history re-key in dry-run (it runs against the freshly-written source ids).")
		return nil
	}

	return rekeyLearningHistories(ctx, cfg, w)
}

// rekeyLearningHistories reads the (now id-bearing) source notebooks, builds a
// per-notebook expression->ids index, and stamps ids onto matching id-less
// learning-history entries.
func rekeyLearningHistories(ctx context.Context, cfg *config.Config, w io.Writer) error {
	learningDir := cfg.Notebooks.LearningNotesDirectory
	if learningDir == "" {
		_, _ = fmt.Fprintln(w, "No learning_notes_directory configured — skipping re-key.")
		return nil
	}

	reader, err := notebook.NewReader(
		cfg.Notebooks.StoriesDirectories,
		cfg.Notebooks.FlashcardsDirectories,
		cfg.Notebooks.BooksDirectories,
		cfg.Notebooks.DefinitionsDirectories,
		cfg.Notebooks.EtymologyDirectories,
		nil,
	)
	if err != nil {
		return fmt.Errorf("build reader: %w", err)
	}
	records, err := notebook.NewYAMLNoteRepository(reader).FindAll(ctx)
	if err != nil {
		return fmt.Errorf("load source notes: %w", err)
	}

	// idByExpr[notebookID][lowercased expression] = distinct source ids in that
	// notebook. Two entries sharing an expression yield two ids -> the re-key
	// leaves them id-less (ambiguous).
	idByExpr := make(map[string]map[string][]string)
	add := func(notebookID, expr, id string) {
		if id == "" || expr == "" {
			return
		}
		key := strings.ToLower(strings.TrimSpace(expr))
		m := idByExpr[notebookID]
		if m == nil {
			m = make(map[string][]string)
			idByExpr[notebookID] = m
		}
		for _, existing := range m[key] {
			if existing == id {
				return
			}
		}
		m[key] = append(m[key], id)
	}
	for _, rec := range records {
		for _, nn := range rec.NotebookNotes {
			add(nn.NotebookID, rec.Entry, rec.SenseID)
			if rec.Usage != rec.Entry {
				add(nn.NotebookID, rec.Usage, rec.SenseID)
			}
		}
	}

	histories, err := notebook.NewLearningHistories(learningDir)
	if err != nil {
		return fmt.Errorf("load learning histories: %w", err)
	}
	names := make([]string, 0, len(histories))
	for name := range histories {
		names = append(names, name)
	}
	sort.Strings(names)

	totalRekeyed := 0
	for _, name := range names {
		m := idByExpr[name]
		if len(m) == 0 {
			continue
		}
		list := histories[name]
		n := notebook.RekeyLearningHistories(list, m)
		if n == 0 {
			continue
		}
		path := filepath.Join(learningDir, name+".yml")
		if err := notebook.WriteYamlFile(path, list); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		totalRekeyed += n
		_, _ = fmt.Fprintf(w, "  re-keyed %d learning entr(y/ies) in %s\n", n, name)
	}
	_, _ = fmt.Fprintf(w, "Re-keyed %d learning-history entr(y/ies)\n", totalRekeyed)
	return nil
}

// collectYAMLFiles returns every .yml file under the given directories,
// de-duplicated and sorted for deterministic processing.
func collectYAMLFiles(dirs []string) ([]string, error) {
	seen := make(map[string]bool)
	var files []string
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		info, err := os.Stat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".yml" {
				return nil
			}
			if !seen[path] {
				seen[path] = true
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}
