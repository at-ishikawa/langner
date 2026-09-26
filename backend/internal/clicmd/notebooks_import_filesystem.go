package clicmd

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// NewNotebooksImportFilesystemCommand imports the configured filesystem notebook
// catalog into Postgres, storing each notebook's raw YAML blobs under its
// EXISTING id (design §13.3). Unlike a user `push` (which mints an nb_ id), the
// id is preserved, so learning_logs / notebook_notes keyed by that id keep
// resolving — no history is orphaned. A single composite id that appears under
// several family folders (e.g. a definitions + etymology book) is stored once,
// with each file tagged by its family, so the DB reader reconstructs it exactly
// as the filesystem walk did. Existing ownership rows (a prior `set-owner`) are
// preserved. Run this once per deploy so a serverless / no-filesystem host
// (Vercel) can serve the whole catalog from the database.
//
// It writes only the content blobs + the `notebooks` registry rows; the
// structured content tables (notes / notebook_notes / definitions_* /
// etymology_*) continue to come from `import-db`. Run `import-db` first, then
// this command.
func NewNotebooksImportFilesystemCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "import-filesystem",
		Short: "Import the configured filesystem notebook catalog into Postgres, preserving ids",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			repo := notebook.NewNotebookFileRepository(db)
			n, err := ImportFilesystemNotebooks(cmd.Context(), cfg, repo, cmd.OutOrStdout())
			if err != nil {
				return err
			}
			if n == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No filesystem notebooks found in the configured directories — nothing to import.")
			}
			return nil
		},
	}
}

// ImportFilesystemNotebooks is the testable core of `notebooks import-filesystem`
// (the RunE wraps it). It scans the configured *_directories the SAME way the
// runtime reader does, and stores every discovered notebook's raw blobs under
// its existing id via NotebookFileRepository.ImportShippedBundle, folding all of
// a composite id's families into one bundle. Returns the number of notebooks
// imported.
func ImportFilesystemNotebooks(ctx context.Context, cfg *config.Config, repo *notebook.NotebookFileRepository, w io.Writer) (int, error) {
	families := []struct {
		family string
		dirs   []string
	}{
		{"stories", cfg.Notebooks.StoriesDirectories},
		{"journals", cfg.Notebooks.JournalsDirectories},
		{"flashcards", cfg.Notebooks.FlashcardsDirectories},
		{"books", cfg.Notebooks.BooksDirectories},
		{"definitions", cfg.Notebooks.DefinitionsDirectories},
		{"etymology", cfg.Notebooks.EtymologyDirectories},
		{"grammars", cfg.Notebooks.GrammarsDirectories},
	}

	// Accumulate every family's files under the shared notebook id, so a
	// composite id is imported as ONE bundle (blobs are replaced per id).
	type bundle struct {
		kind, name string
		files      []notebook.NotebookFile
	}
	byID := map[string]*bundle{}
	var order []string

	for _, fam := range families {
		for _, dir := range fam.dirs {
			indexPaths, err := findIndexFiles(dir)
			if err != nil {
				return 0, fmt.Errorf("scan %s: %w", dir, err)
			}
			for _, indexPath := range indexPaths {
				files, meta, err := resolveBundle(filepath.Dir(indexPath))
				if err != nil {
					return 0, fmt.Errorf("read %s: %w", indexPath, err)
				}
				id := strings.TrimSpace(meta.ID)
				if id == "" {
					return 0, fmt.Errorf("notebook %s has no id: — every notebook needs a stable id", indexPath)
				}
				b := byID[id]
				if b == nil {
					b = &bundle{}
					byID[id] = b
					order = append(order, id)
				}
				for _, bf := range files {
					b.files = append(b.files, notebook.NotebookFile{
						NotebookID: id,
						Family:     fam.family,
						Path:       bf.RelPath,
						Content:    bf.Content,
					})
				}
				if b.kind == "" && strings.TrimSpace(meta.Kind) != "" {
					b.kind = strings.TrimSpace(meta.Kind)
				}
				if b.name == "" && strings.TrimSpace(meta.Name) != "" {
					b.name = strings.TrimSpace(meta.Name)
				}
			}
		}
	}

	for _, id := range order {
		b := byID[id]
		kind := b.kind
		if kind == "" && len(b.files) > 0 {
			kind = b.files[0].Family // display fallback; family placement is per-file, not from kind
		}
		hash := notebook.HashBundle(b.files)
		if err := repo.ImportShippedBundle(ctx, id, kind, b.name, hash, b.files); err != nil {
			return 0, fmt.Errorf("import %q: %w", id, err)
		}
		_, _ = fmt.Fprintf(w, "Imported %-40s %d file(s) across %s\n", id, len(b.files), familiesOf(b.files))
	}
	if len(order) > 0 {
		_, _ = fmt.Fprintf(w, "\nImported %d filesystem notebook(s) into Postgres (ids preserved; learning histories intact).\n", len(order))
	}
	return len(order), nil
}

// findIndexFiles returns every index.yml path under root (recursively). A
// non-existent configured directory is not an error — it simply contributes
// nothing (families a deployment doesn't use are commonly unset).
func findIndexFiles(root string) ([]string, error) {
	if root == "" {
		return nil, nil
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "index.yml" {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// familiesOf returns the distinct family set of a bundle's files, for the import
// log line (e.g. "definitions+etymology").
func familiesOf(files []notebook.NotebookFile) string {
	seen := map[string]bool{}
	var fams []string
	for _, f := range files {
		if !seen[f.Family] {
			seen[f.Family] = true
			fams = append(fams, f.Family)
		}
	}
	return strings.Join(fams, "+")
}
