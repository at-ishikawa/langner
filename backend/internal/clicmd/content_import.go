package clicmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/datasync"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// familyKinds is the notebooks.kind a single-family catalog notebook is
// registered with.
var familyKinds = map[string]string{
	notebook.FamilyStories:     "story",
	notebook.FamilyFlashcards:  "flashcard",
	notebook.FamilyBooks:       "book",
	notebook.FamilyDefinitions: "definitions",
	notebook.FamilyEtymology:   "etymology",
	notebook.FamilyJournals:    "journal",
	notebook.FamilyGrammars:    "grammar",
}

// configContentDirs returns the configured notebook directories.
func configContentDirs(cfg *config.Config) notebook.ContentDirs {
	return notebook.ContentDirs{
		Stories:     cfg.Notebooks.StoriesDirectories,
		Flashcards:  cfg.Notebooks.FlashcardsDirectories,
		Books:       cfg.Notebooks.BooksDirectories,
		Definitions: cfg.Notebooks.DefinitionsDirectories,
		Etymology:   cfg.Notebooks.EtymologyDirectories,
		Journals:    cfg.Notebooks.JournalsDirectories,
		Grammars:    cfg.Notebooks.GrammarsDirectories,
	}
}

// ImportShippedContent reads every notebook under the configured notebook
// directories (the shipped example catalog) and stores it in Postgres under its
// EXISTING id, with the notes its words need. The directories are only an
// import source; the server reads content from Postgres. A notebook is skipped
// when it is stored at the same bundle hash AND its content rows were built
// from that hash, unless its id is in force. Returns the number of notebooks
// stored (updated).
func ImportShippedContent(ctx context.Context, cfg *config.Config, db *sqlx.DB, out io.Writer, force ...string) (int, error) {
	bundles, err := notebook.CollectBundles(configContentDirs(cfg))
	if err != nil {
		return 0, err
	}
	store := notebook.NewNotebookStore(db)
	updated := 0
	for _, b := range bundles {
		if !slices.Contains(force, b.ID) {
			current, err := store.ContentCurrent(ctx, b.ID, notebook.HashBundle(b.Files))
			if err != nil {
				return updated, err
			}
			if current {
				continue
			}
		}
		kind := familyKinds[b.Families[0]]
		if len(b.Families) > 1 {
			kind = "composite"
		}
		name := b.Name
		if name == "" {
			name = b.ID
		}
		if err := storeShippedNotebook(ctx, db, store, kind, name, b.ID, b.Files); err != nil {
			return updated, err
		}
		updated++
		if out != nil {
			_, _ = fmt.Fprintf(out, "imported %s (%d file(s) across %d family/-ies)\n", b.ID, len(b.Files), len(b.Families))
		}
	}
	return updated, nil
}

// storeShippedNotebook parses a catalog notebook's files and stores them, with
// its notes.
func storeShippedNotebook(ctx context.Context, db *sqlx.DB, store *notebook.NotebookStore, kind, name, id string, files []notebook.NotebookFile) error {
	content, err := notebook.ParseBundle(id, files)
	if err != nil {
		return fmt.Errorf("parse notebook %q: %w", id, err)
	}
	if err := store.ImportShippedBundle(ctx, kind, name, notebook.HashBundle(files), notebook.BundleBytes(files), content); err != nil {
		return fmt.Errorf("import notebook %q: %w", id, err)
	}
	if _, err := datasync.EnsureContentNotes(ctx, db, content); err != nil {
		return fmt.Errorf("create notes of notebook %q: %w", id, err)
	}
	return nil
}

// ConvertStoredFiles stores notebooks kept as raw YAML files (notebook_files,
// before migration 038) in the content tables, keeping each notebook's
// registry row as it is. It also rebuilds a notebook whose rows were not built
// from its current stored files (see notebook.StoredFileBundles); notebooks
// whose rows are current are skipped, and the files are left in place as a
// backup. A notebook that fails to parse is reported. Returns the number
// converted.
func ConvertStoredFiles(ctx context.Context, db *sqlx.DB, out io.Writer) (int, error) {
	bundles, err := notebook.StoredFileBundles(ctx, db)
	if err != nil {
		return 0, err
	}
	ids := make([]string, 0, len(bundles))
	for id := range bundles {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	store := notebook.NewNotebookStore(db)
	var errs []error
	converted := 0
	for _, id := range ids {
		files := bundles[id]
		content, err := notebook.ParseBundle(id, files)
		if err == nil {
			err = store.ConvertBundle(ctx, notebook.HashBundle(files), notebook.BundleBytes(files), content)
		}
		if err == nil {
			_, err = datasync.EnsureContentNotes(ctx, db, content)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("notebook %q: %w", id, err))
			continue
		}
		converted++
		if out != nil {
			_, _ = fmt.Fprintf(out, "converted %s (%d file(s))\n", id, len(files))
		}
	}
	return converted, errors.Join(errs...)
}

// newMigrateImportContentCommand imports the shipped notebook catalog into
// Postgres. It is the standalone form of the step reset-db and seed-e2e run as
// part of their seed. It first rebuilds rows that don't match their stored
// files (ConvertStoredFiles), then stores every catalog notebook that changed.
func newMigrateImportContentCommand() *cobra.Command {
	var force []string
	cmd := &cobra.Command{
		Use:   "import-content",
		Short: "Import shipped notebook content (configured example dirs) into Postgres",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			if _, err := ConvertStoredFiles(ctx, db, cmd.OutOrStdout()); err != nil {
				return fmt.Errorf("convert stored notebook files: %w", err)
			}
			n, err := ImportShippedContent(ctx, cfg, db, cmd.OutOrStdout(), force...)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%d notebook(s) updated.\n", n)
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&force, "force", nil, "notebook ids to rebuild even when unchanged")
	return cmd
}
