// Package cli — one-time migration that rewrites legacy flat-shape
// etymology session YAML files into the explicit event/scenes/origins
// shape using definitions notebooks to pick each origin's canonical
// scene (earliest scene in earliest session that references the origin
// via origin_parts).
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// MigrateEtymologyToScenes converts every legacy-shape etymology session
// file under etymologyDirs to the explicit event/scenes/origins shape.
// For each origin, the destination scene is the earliest (session-order
// in the book's index.yml, scene-order within the session's definitions
// file) scene whose vocab references the origin via origin_parts. Multi-
// sense origins (rows sharing origin+language but distinct sense:) are
// bucketed independently using their sense token. Origins not referenced
// by any definition land in a synthetic scene named after the origin so
// they remain visible in the migrated file.
//
// Files already in the new shape are skipped (idempotent). Returns an
// error on the first per-file failure so the caller sees a clean stack.
func MigrateEtymologyToScenes(etymologyDirs []string, definitionsDirs []string, dryRun bool) error {
	if len(etymologyDirs) == 0 {
		return fmt.Errorf("at least one etymology directory is required")
	}

	defsByBook, err := loadDefinitionsByBook(definitionsDirs)
	if err != nil {
		return fmt.Errorf("load definitions: %w", err)
	}

	var migrated, skipped int
	for _, dir := range etymologyDirs {
		books, err := findEtymologyBooks(dir)
		if err != nil {
			return fmt.Errorf("scan %s: %w", dir, err)
		}
		for _, book := range books {
			n, s, err := migrateBook(book, defsByBook[book.id], dryRun)
			if err != nil {
				return fmt.Errorf("migrate book %s: %w", book.id, err)
			}
			migrated += n
			skipped += s
		}
	}

	if dryRun {
		fmt.Printf("Dry-run complete: %d file(s) would migrate, %d already in new shape.\n", migrated, skipped)
		return nil
	}
	fmt.Printf("Migration complete: %d file(s) migrated, %d already in new shape.\n", migrated, skipped)
	return nil
}

// etymologyBook collects the per-book info the migrator needs from one
// etymology index.yml: the book id (matches the definitions book id),
// the directory holding the session files, and the ordered list of
// session file basenames.
type etymologyBook struct {
	id           string
	dir          string
	sessionPaths []string // file basenames relative to dir, in index.yml order
}

// findEtymologyBooks walks an etymology directory and returns one
// etymologyBook per subdirectory that has an index.yml. Mirrors
// walkEtymologyIndexFiles but exposed for the migrator's needs (it
// requires the session file order from index.yml).
func findEtymologyBooks(root string) ([]etymologyBook, error) {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}
	var books []etymologyBook
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Base(path) != "index.yml" {
			return nil
		}
		idx, err := notebook.ReadEtymologyIndex(path)
		if err != nil {
			return fmt.Errorf("read index %s: %w", path, err)
		}
		books = append(books, etymologyBook{
			id:           idx.ID,
			dir:          filepath.Dir(path),
			sessionPaths: idx.NotebookPaths,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return books, nil
}

// definitionsByBook maps bookID -> sessionTitle -> ordered scenes.
// Each scene carries its title and the origins its vocab references,
// in the order they first appear (used by the earliest-rule).
type definitionsByBook map[string]map[string][]definitionScene

type definitionScene struct {
	title   string
	origins []originRef // unique, first-appearance order
}

// originRef is the bucketing key for assigning an origin to a scene.
// Sense disambiguates same-session multi-sense origins (e.g. pathos =
// feeling vs pathos = disease); language is part of the key so two
// origins sharing only a spelling stay separate.
type originRef struct {
	origin   string
	language string
	sense    string
}

// loadDefinitionsByBook walks every definitions directory and returns
// the per-book, per-session scene structure used by the earliest rule.
// Session order comes from each book's index.yml. Within a session,
// scenes preserve the order they appear in the YAML file.
func loadDefinitionsByBook(dirs []string) (definitionsByBook, error) {
	out := make(definitionsByBook)
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || filepath.Base(path) != "index.yml" {
				return nil
			}
			idx, err := notebook.ReadDefinitionsIndex(path)
			if err != nil {
				return fmt.Errorf("read defs index %s: %w", path, err)
			}
			bookDir := filepath.Dir(path)
			if out[idx.ID] == nil {
				out[idx.ID] = make(map[string][]definitionScene)
			}
			for _, nbPath := range idx.Notebooks {
				sessionPath := filepath.Join(bookDir, nbPath)
				bySession, err := loadDefinitionsSessions(sessionPath)
				if err != nil {
					return fmt.Errorf("read defs session %s: %w", sessionPath, err)
				}
				for sessionTitle, scenes := range bySession {
					out[idx.ID][sessionTitle] = append(out[idx.ID][sessionTitle], scenes...)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// loadDefinitionsSessions reads one definitions YAML (a list of
// session-shaped notebooks) and returns a map of sessionTitle -> ordered
// scenes. Each scene's origin list collects refs in their first-
// appearance order across the scene's vocab; duplicates within the same
// scene collapse to one entry.
func loadDefinitionsSessions(path string) (map[string][]definitionScene, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	parsed, err := notebook.ReadDefinitionsFromBytes(contents)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]definitionScene)
	for _, def := range parsed {
		sessionTitle := strings.TrimSpace(def.Metadata.Title)
		if sessionTitle == "" {
			sessionTitle = strings.TrimSpace(def.Metadata.Notebook)
		}
		if sessionTitle == "" {
			continue
		}
		for _, scene := range def.Scenes {
			ds := definitionScene{title: strings.TrimSpace(scene.Metadata.Title)}
			seen := make(map[originRef]bool)
			for _, expr := range scene.Expressions {
				for _, ref := range expr.OriginParts {
					key := originRef{
						origin:   strings.TrimSpace(ref.Origin),
						language: strings.TrimSpace(ref.Language),
						sense:    strings.TrimSpace(ref.Sense),
					}
					if key.origin == "" || seen[key] {
						continue
					}
					seen[key] = true
					ds.origins = append(ds.origins, key)
				}
			}
			out[sessionTitle] = append(out[sessionTitle], ds)
		}
	}
	return out, nil
}

// migrateBook processes every session file in a book, converting legacy
// shape to new shape using the book's definitions to assign scenes.
// Returns (migrated count, skipped count).
func migrateBook(book etymologyBook, defsBySession map[string][]definitionScene, dryRun bool) (int, int, error) {
	var migrated, skipped int
	for _, nbPath := range book.sessionPaths {
		sessionPath := filepath.Join(book.dir, nbPath)
		didMigrate, err := migrateSessionFile(sessionPath, book.id, defsBySession, dryRun)
		if err != nil {
			return migrated, skipped, fmt.Errorf("migrate %s: %w", sessionPath, err)
		}
		if didMigrate {
			migrated++
		} else {
			skipped++
		}
	}
	return migrated, skipped, nil
}

// migrateSessionFile rewrites one etymology session file. Returns true
// when a write happened (or would happen, under --dry-run), false when
// the file was already new-shape.
func migrateSessionFile(
	path string,
	bookID string,
	defsBySession map[string][]definitionScene,
	dryRun bool,
) (bool, error) {
	// Skip already-new-shape files (idempotent).
	if isNewShape, err := isNewShapeFile(path); err != nil {
		return false, err
	} else if isNewShape {
		return false, nil
	}

	legacy, err := notebook.ReadLegacyEtymologySession(path)
	if err != nil {
		return false, fmt.Errorf("read legacy: %w", err)
	}
	sessionTitle := strings.TrimSpace(legacy.Metadata.Title)
	if sessionTitle == "" {
		return false, fmt.Errorf("missing metadata.title")
	}

	defScenes := defsBySession[sessionTitle]
	entry := buildNewShapeEntry(legacy, sessionTitle, defScenes)

	if dryRun {
		fmt.Printf("[dry-run] would migrate %s (%d origin(s) across %d scene(s))\n",
			path, countOrigins(entry), len(entry.Scenes))
		return true, nil
	}

	if err := notebook.WriteYamlFile(path, []notebook.EtymologyNotebookEntry{entry}); err != nil {
		return false, fmt.Errorf("write: %w", err)
	}
	fmt.Printf("migrated %s (%d origin(s) across %d scene(s))\n", path, countOrigins(entry), len(entry.Scenes))
	return true, nil
}

// buildNewShapeEntry applies the earliest-rule: each origin is placed
// under the first scene whose vocab references it. Origins not
// referenced by any definition fall into a synthetic scene named after
// the origin so they aren't silently dropped (the user can move them by
// hand if desired).
func buildNewShapeEntry(
	legacy notebook.LegacyEtymologySession,
	sessionTitle string,
	defScenes []definitionScene,
) notebook.EtymologyNotebookEntry {
	// originPos: the (sceneIdx, exprPos) at which each origin first appears.
	// Smaller wins.
	type pos struct {
		sceneIdx int
		exprPos  int
	}
	originPos := make(map[originRef]pos)
	for si, scene := range defScenes {
		for ep, ref := range scene.origins {
			if existing, ok := originPos[ref]; ok {
				if existing.sceneIdx < si || (existing.sceneIdx == si && existing.exprPos <= ep) {
					continue
				}
			}
			originPos[ref] = pos{si, ep}
		}
	}

	// Bucket each declared origin into its destination scene.
	sceneOrder := make([]string, 0, len(defScenes))
	sceneIdxByTitle := make(map[string]int, len(defScenes))
	for si, scene := range defScenes {
		sceneIdxByTitle[scene.title] = si
		sceneOrder = append(sceneOrder, scene.title)
	}

	type sceneBucket struct {
		title   string
		origins []notebook.EtymologyOrigin
	}
	buckets := make(map[int]*sceneBucket)
	syntheticOrder := []string{} // preserve first-encounter order for synthetic scenes

	for _, o := range legacy.Origins {
		ref := originRef{
			origin:   strings.TrimSpace(o.Origin),
			language: strings.TrimSpace(o.Language),
			sense:    strings.TrimSpace(o.Sense),
		}
		var destSceneTitle string
		var destIdx int
		if p, ok := originPos[ref]; ok {
			destIdx = p.sceneIdx
			destSceneTitle = defScenes[destIdx].title
		} else {
			// Origin not referenced by any definition — synthetic scene.
			destSceneTitle = ref.origin
			if _, exists := sceneIdxByTitle[destSceneTitle]; !exists {
				destIdx = len(defScenes) + len(syntheticOrder)
				syntheticOrder = append(syntheticOrder, destSceneTitle)
			} else {
				destIdx = sceneIdxByTitle[destSceneTitle]
			}
		}
		b, ok := buckets[destIdx]
		if !ok {
			b = &sceneBucket{title: destSceneTitle}
			buckets[destIdx] = b
		}
		b.origins = append(b.origins, o)
	}

	// Emit scenes in book order: declared scenes first (in index order),
	// then any synthetic scenes (in first-encounter order).
	var scenes []notebook.EtymologyNotebookScene
	indices := make([]int, 0, len(buckets))
	for i := range buckets {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	for _, i := range indices {
		b := buckets[i]
		scenes = append(scenes, notebook.EtymologyNotebookScene{
			Scene:   b.title,
			Origins: b.origins,
		})
	}

	return notebook.EtymologyNotebookEntry{
		Event:     sessionTitle,
		Date:      legacy.Date,
		Scenes:    scenes,
		Concepts:  legacy.Concepts,
		Relations: legacy.Relations,
	}
}

func countOrigins(entry notebook.EtymologyNotebookEntry) int {
	var n int
	for _, s := range entry.Scenes {
		n += len(s.Origins)
	}
	return n
}

// isNewShapeFile reports whether a file already uses the new
// event/scenes/origins shape. Used to make the migrator idempotent.
func isNewShapeFile(path string) (bool, error) {
	return notebook.IsNewShapeEtymologyFile(path)
}
