package clicmd

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// shippedFamily pairs a configured directory list with the ContentDirs family
// and the index.yml `kind:` a notebook in that bucket validates as. It is the
// single mapping the importer walks so every family is imported the same way.
type shippedFamily struct {
	family string
	kind   string
	dirs   []string
}

// shippedNotebook accumulates one notebook id's files across every family it
// appears in. A composite notebook (the SAME id present under more than one
// family dir — e.g. a root-list that is both a definitions notebook and an
// etymology notebook) folds into ONE entry whose files carry their own family,
// so it registers across the family maps exactly like the filesystem walk.
type shippedNotebook struct {
	id       string
	name     string
	families map[string]bool
	kind     string // the single family's kind, or "composite" when >1 family
	files    []notebook.NotebookFile
}

// ImportShippedContent walks the configured notebook directories (the shipped
// example catalog) and imports every notebook's files into the notebook_files
// table under its EXISTING id, so the serving path reads notebook CONTENT from
// Postgres in every environment (dev / e2e / prod) — never from the filesystem.
// It is the write side that fills what DBContentSource serves; the directories
// are only an import source here, not a serving path. Returns the number of
// notebooks imported.
func ImportShippedContent(ctx context.Context, cfg *config.Config, db *sqlx.DB, out io.Writer) (int, error) {
	families := []shippedFamily{
		{"stories", "story", cfg.Notebooks.StoriesDirectories},
		{"flashcards", "flashcard", cfg.Notebooks.FlashcardsDirectories},
		{"books", "book", cfg.Notebooks.BooksDirectories},
		{"definitions", "definitions", cfg.Notebooks.DefinitionsDirectories},
		{"etymology", "etymology", cfg.Notebooks.EtymologyDirectories},
		{"journals", "journal", cfg.Notebooks.JournalsDirectories},
		{"grammars", "grammar", cfg.Notebooks.GrammarsDirectories},
	}

	byID := map[string]*shippedNotebook{}
	for _, fam := range families {
		for _, dir := range fam.dirs {
			if dir == "" {
				continue
			}
			if _, err := os.Stat(dir); err != nil {
				continue // a configured dir that isn't present on disk is simply skipped
			}
			if err := collectFamily(dir, fam, byID); err != nil {
				return 0, fmt.Errorf("collect %s content from %s: %w", fam.family, dir, err)
			}
		}
	}

	repo := notebook.NewNotebookFileRepository(db)
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids) // deterministic import order

	for _, id := range ids {
		nb := byID[id]
		kind := nb.kind
		if len(nb.families) > 1 {
			kind = "composite" // files carry their own family; kind is only the fallback
		}
		name := nb.name
		if name == "" {
			name = id
		}
		hash := notebook.HashBundle(nb.files)
		if err := repo.ImportShippedBundle(ctx, id, kind, name, hash, nb.files); err != nil {
			return 0, fmt.Errorf("import notebook %q: %w", id, err)
		}
		if out != nil {
			_, _ = fmt.Fprintf(out, "imported %s (%d file(s) across %d family/-ies)\n", id, len(nb.files), len(nb.families))
		}
	}
	return len(ids), nil
}

// collectFamily finds every notebook root (a directory containing index.yml)
// under dir, reads its bundle via the SAME resolver the push CLI uses, tags each
// file with fam's family, and folds it into byID keyed by the notebook's id.
func collectFamily(dir string, fam shippedFamily, byID map[string]*shippedNotebook) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if _, statErr := os.Stat(filepath.Join(path, "index.yml")); statErr != nil {
			return nil // not a notebook root; keep descending
		}

		files, meta, resErr := resolveBundle(path)
		if resErr != nil {
			return fmt.Errorf("read notebook at %s: %w", path, resErr)
		}
		id := strings.TrimSpace(meta.ID)
		if id == "" {
			id = filepath.Base(path) // id: omitted → fall back to the on-disk dir name
		}

		nb := byID[id]
		if nb == nil {
			nb = &shippedNotebook{id: id, families: map[string]bool{}, kind: fam.kind}
			byID[id] = nb
		}
		if nb.name == "" {
			nb.name = meta.Name
		}
		nb.families[fam.family] = true
		for _, bf := range files {
			nb.files = append(nb.files, notebook.NotebookFile{
				NotebookID: id,
				Family:     fam.family,
				Path:       bf.RelPath,
				Content:    bf.Content,
			})
		}
		return fs.SkipDir // a notebook root is not nested inside another notebook
	})
}

// newMigrateImportContentCommand imports the shipped notebook catalog into
// notebook_files so content is served from the DB. It is the standalone form of
// the step reset-db and seed-e2e run as part of their seed.
func newMigrateImportContentCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "import-content",
		Short: "Import shipped notebook content (configured example dirs) into notebook_files",
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
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Imported %d shipped notebook(s) into notebook_files.\n", n)
			return nil
		},
	}
}
