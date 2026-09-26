package notebook

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ContentDirs groups notebook directory roots by family. It is the seam that
// lets the Reader union the shipped filesystem catalog with DB-stored user
// notebooks: both are just directory roots the SAME walkers/parsers consume, so
// a shipped notebook and a user (nb_) notebook parse through one code path with
// no divergence (see .claude/rules/verify-data-features-with-example-notebooks).
type ContentDirs struct {
	Stories     []string
	Flashcards  []string
	Books       []string
	Definitions []string
	Etymology   []string
	Journals    []string
	Grammars    []string
}

// Merge appends other's directories onto d, returning the union. Used to fold a
// ContentSource's extra dirs into the configured filesystem dirs.
func (d ContentDirs) Merge(other ContentDirs) ContentDirs {
	return ContentDirs{
		Stories:     append(append([]string{}, d.Stories...), other.Stories...),
		Flashcards:  append(append([]string{}, d.Flashcards...), other.Flashcards...),
		Books:       append(append([]string{}, d.Books...), other.Books...),
		Definitions: append(append([]string{}, d.Definitions...), other.Definitions...),
		Etymology:   append(append([]string{}, d.Etymology...), other.Etymology...),
		Journals:    append(append([]string{}, d.Journals...), other.Journals...),
		Grammars:    append(append([]string{}, d.Grammars...), other.Grammars...),
	}
}

// ContentSource yields notebook directory roots to walk, grouped by family. The
// filesystem catalog is just the configured directories; user notebooks are
// materialized from DB blobs to a temp tree. The Reader treats both identically.
type ContentSource interface {
	// Dirs returns the directory roots this source contributes. Implementations
	// that materialize (DB) manage their own temp storage for the process
	// lifetime; the returned dirs stay valid until the next Dirs call refreshes.
	Dirs(ctx context.Context) (ContentDirs, error)
}

// familyDir maps a notebook `kind:` to the ContentDirs family a materialized
// blob directory belongs under. Case-insensitive; an unknown/empty kind falls
// back to the flashcard family (the simplest self-contained vocabulary shape).
func familyDir(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "story":
		return "stories"
	case "journal":
		return "journals"
	case "grammar":
		return "grammars"
	case "book":
		return "books"
	case "definitions", "definition":
		return "definitions"
	case "etymology":
		return "etymology"
	case "flashcard", "flashcards":
		return "flashcards"
	default:
		return "flashcards"
	}
}

// DBContentSource materializes every source='user' notebook's stored blobs to a
// temp directory tree so the SAME filesystem walkers/parsers parse them. It
// enumerates ALL user notebooks (visibility is enforced downstream by the same
// VisibleNotebookIDs predicate shipped notebooks use — the lower-risk default
// from design §5.5). Materialization is cached and refreshed only when the set
// of user notebooks changes (by content fingerprint), so a repeated reader
// build is cheap.
type DBContentSource struct {
	repo *NotebookFileRepository

	mu          sync.Mutex
	root        string // current temp root, or "" before first materialization
	fingerprint string
	dirs        ContentDirs
}

// NewDBContentSource constructs a DB content source over the file repository.
func NewDBContentSource(repo *NotebookFileRepository) *DBContentSource {
	return &DBContentSource{repo: repo}
}

// Dirs materializes (or reuses) the user-notebook blob tree and returns its
// per-family directory roots. It is safe for concurrent callers.
func (s *DBContentSource) Dirs(ctx context.Context) (ContentDirs, error) {
	fp, err := s.repo.Fingerprint(ctx)
	if err != nil {
		return ContentDirs{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root != "" && fp == s.fingerprint {
		return s.dirs, nil // unchanged — reuse the cached materialization
	}

	newRoot, dirs, err := s.materialize(ctx)
	if err != nil {
		return ContentDirs{}, err
	}
	oldRoot := s.root
	s.root = newRoot
	s.fingerprint = fp
	s.dirs = dirs
	if oldRoot != "" {
		_ = os.RemoveAll(oldRoot) // best-effort; refreshes are rare (post-push)
	}
	return dirs, nil
}

// materialize writes every user notebook's blobs under
// <tmp>/<family>/<notebook_id>/<path> and returns the family roots that exist.
func (s *DBContentSource) materialize(ctx context.Context) (string, ContentDirs, error) {
	notebooks, err := s.repo.ListAllUserNotebooks(ctx)
	if err != nil {
		return "", ContentDirs{}, err
	}
	root, err := os.MkdirTemp("", "langner-usernb-*")
	if err != nil {
		return "", ContentDirs{}, fmt.Errorf("create temp root: %w", err)
	}

	used := make(map[string]bool)
	for _, nb := range notebooks {
		files, err := s.repo.ListFiles(ctx, nb.NotebookID)
		if err != nil {
			_ = os.RemoveAll(root)
			return "", ContentDirs{}, err
		}
		family := familyDir(nb.Kind)
		nbDir := filepath.Join(root, family, nb.NotebookID)
		for _, f := range files {
			dest := filepath.Join(nbDir, filepath.Clean("/"+f.Path))
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				_ = os.RemoveAll(root)
				return "", ContentDirs{}, fmt.Errorf("mkdir for blob %s: %w", f.Path, err)
			}
			if err := os.WriteFile(dest, f.Content, 0o644); err != nil {
				_ = os.RemoveAll(root)
				return "", ContentDirs{}, fmt.Errorf("write blob %s: %w", f.Path, err)
			}
		}
		used[family] = true
	}

	dirs := ContentDirs{}
	add := func(family string, target *[]string) {
		if used[family] {
			*target = append(*target, filepath.Join(root, family))
		}
	}
	add("stories", &dirs.Stories)
	add("flashcards", &dirs.Flashcards)
	add("books", &dirs.Books)
	add("definitions", &dirs.Definitions)
	add("etymology", &dirs.Etymology)
	add("journals", &dirs.Journals)
	add("grammars", &dirs.Grammars)
	return root, dirs, nil
}
