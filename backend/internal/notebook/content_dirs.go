package notebook

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ContentDirs groups notebook directory roots by family: where the CLI reads a
// local notebook catalog from before importing it.
type ContentDirs struct {
	Stories     []string
	Flashcards  []string
	Books       []string
	Definitions []string
	Etymology   []string
	Journals    []string
	Grammars    []string
}

// byFamily returns the directory roots keyed by family.
func (d ContentDirs) byFamily() map[string][]string {
	return map[string][]string{
		FamilyStories:     d.Stories,
		FamilyFlashcards:  d.Flashcards,
		FamilyBooks:       d.Books,
		FamilyDefinitions: d.Definitions,
		FamilyEtymology:   d.Etymology,
		FamilyJournals:    d.Journals,
		FamilyGrammars:    d.Grammars,
	}
}

// Bundle is one notebook's YAML files gathered from disk, across every family
// it is registered under.
type Bundle struct {
	ID string
	// Name is the first index.yml `name:` found.
	Name     string
	Families []string
	Files    []NotebookFile
}

// ReadBundleDir reads a notebook directory: its index.yml plus every file the
// index lists, with paths relative to the directory. It also returns the
// index's id, kind and name.
func ReadBundleDir(root string) (files []NotebookFile, id, kind, name string, err error) {
	indexBytes, err := os.ReadFile(filepath.Join(root, "index.yml"))
	if err != nil {
		return nil, "", "", "", fmt.Errorf("read index.yml: %w", err)
	}
	var index contentIndex
	if err := yaml.Unmarshal(indexBytes, &index); err != nil {
		return nil, "", "", "", fmt.Errorf("parse index.yml: %w", err)
	}
	files = []NotebookFile{{Path: "index.yml", Content: indexBytes}}
	seen := map[string]bool{"index.yml": true}
	for _, nb := range index.Notebooks {
		rel := filepath.ToSlash(filepath.Clean(strings.TrimPrefix(nb, "./")))
		if seen[rel] {
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, "", "", "", fmt.Errorf("read referenced file %s: %w", rel, err)
		}
		files = append(files, NotebookFile{Path: rel, Content: content})
		seen[rel] = true
	}
	return files, index.ID, index.Kind, index.Name, nil
}

// CollectBundles finds every notebook (a directory containing index.yml) under
// the given roots and groups the files by notebook id. A notebook without an
// id is named after its directory.
func CollectBundles(dirs ContentDirs) ([]Bundle, error) {
	byID := make(map[string]*Bundle)
	for _, family := range Families {
		for _, dir := range dirs.byFamily()[family] {
			if dir == "" {
				continue
			}
			if _, err := os.Stat(dir); err != nil {
				continue // a configured dir that isn't on disk is skipped
			}
			err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() {
					return nil
				}
				if _, err := os.Stat(filepath.Join(p, "index.yml")); err != nil {
					return nil // not a notebook root; keep descending
				}
				files, id, _, name, err := ReadBundleDir(p)
				if err != nil {
					return fmt.Errorf("read notebook at %s: %w", p, err)
				}
				id = strings.TrimSpace(id)
				if id == "" {
					id = filepath.Base(p)
				}
				b := byID[id]
				if b == nil {
					b = &Bundle{ID: id}
					byID[id] = b
				}
				if b.Name == "" {
					b.Name = name
				}
				b.Families = append(b.Families, family)
				for _, f := range files {
					f.NotebookID, f.Family = id, family
					b.Files = append(b.Files, f)
				}
				return fs.SkipDir // notebooks don't nest
			})
			if err != nil {
				return nil, fmt.Errorf("collect %s notebooks from %s: %w", family, dir, err)
			}
		}
	}
	out := make([]Bundle, 0, len(byID))
	for _, b := range byID {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// LoadDirectoryContent parses every notebook under the given roots, the way a
// push would store them.
func LoadDirectoryContent(dirs ContentDirs) ([]NotebookContent, error) {
	bundles, err := CollectBundles(dirs)
	if err != nil {
		return nil, err
	}
	out := make([]NotebookContent, 0, len(bundles))
	for _, b := range bundles {
		nc, err := ParseBundle(b.ID, b.Files)
		if err != nil {
			return nil, fmt.Errorf("parse notebook %s: %w", b.ID, err)
		}
		out = append(out, nc)
	}
	return out, nil
}
