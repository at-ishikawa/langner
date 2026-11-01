package clicmd

import (
	"context"
	"errors"
	"fmt"
	"io"
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
// import source; the server reads content from Postgres. Returns the number of
// notebooks imported.
func ImportShippedContent(ctx context.Context, cfg *config.Config, db *sqlx.DB, out io.Writer) (int, error) {
	bundles, err := notebook.CollectBundles(configContentDirs(cfg))
	if err != nil {
		return 0, err
	}
	store := notebook.NewNotebookStore(db)
	for _, b := range bundles {
		kind := familyKinds[b.Families[0]]
		if len(b.Families) > 1 {
			kind = "composite"
		}
		name := b.Name
		if name == "" {
			name = b.ID
		}
		if err := storeShippedNotebook(ctx, db, store, kind, name, b.ID, b.Files); err != nil {
			return 0, err
		}
		if out != nil {
			_, _ = fmt.Fprintf(out, "imported %s (%d file(s) across %d family/-ies)\n", b.ID, len(b.Files), len(b.Families))
		}
	}
	return len(bundles), nil
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

// ConvertStoredFiles moves notebooks stored as raw YAML files (notebook_files,
// before migration 038) into the content tables, keeping each notebook's
// registry row as it is, and removes the converted files. A notebook that
// fails to parse keeps its files and is reported. Returns the number converted.
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
			err = store.ConvertBundle(ctx, notebook.BundleBytes(files), content)
		}
		if err == nil {
			_, err = datasync.EnsureContentNotes(ctx, db, content)
		}
		if err == nil {
			err = notebook.DeleteStoredFiles(ctx, db, id)
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
// part of their seed.
func newMigrateImportContentCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "import-content",
		Short: "Import shipped notebook content (configured example dirs) into Postgres",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			n, err := ImportShippedContent(ctx, cfg, db, cmd.OutOrStdout())
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Imported %d shipped notebook(s).\n", n)
			return nil
		},
	}
}
