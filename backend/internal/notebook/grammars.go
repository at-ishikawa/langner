package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// A grammars notebook annotates a story (the same prose format used by books
// and transcripts) with grammar mistakes to drill — the parallel of a
// definitions notebook, which annotates the same stories with vocabulary. A
// grammars file lists entries keyed to a story entry by its title (matching
// StoryNotebook.Event); each entry carries the corrections for that prose.

// Correction is one grammar fix on a span of a story entry's text: the
// incorrect span (an exact substring, so the quiz can highlight it in place),
// its fix, a free-form category, and the reason.
type Correction struct {
	// ID is the stable spaced-repetition identity. When empty it is derived
	// from the story id, entry title, and order; set it explicitly to make the
	// history edit-proof.
	ID        string `yaml:"id,omitempty"`
	Incorrect string `yaml:"incorrect"`
	Correct   string `yaml:"correct"`
	Category  string `yaml:"category,omitempty"`
	Reason    string `yaml:"reason,omitempty"`
}

// GrammarEntry is the set of corrections for one story entry, matched by title.
type GrammarEntry struct {
	Metadata    GrammarMetadata `yaml:"metadata"`
	Corrections []Correction    `yaml:"corrections"`
}

// GrammarMetadata identifies which story entry the corrections apply to.
type GrammarMetadata struct {
	Title string `yaml:"title"` // matches StoryNotebook.Event
}

// grammarsIndex is the index.yml of a grammars directory (id + notebook files).
type grammarsIndex struct {
	ID        string   `yaml:"id"`
	Notebooks []string `yaml:"notebooks"`
}

// DerivedCorrectionID returns a correction's stable id, deriving one from the
// story id, entry title, and per-entry sequence when no explicit id is set.
func DerivedCorrectionID(storyID, title string, seq int) string {
	return fmt.Sprintf("%s-%s-%d", storyID, slugifyTitle(title), seq)
}

func slugifyTitle(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, s)
	return strings.Trim(strings.ReplaceAll(s, "--", "-"), "-")
}

// StoryNotebookText renders a story entry as plain text — one statement or
// spoken quote per line — so the grammar quiz can show it whole and locate each
// incorrect span as a substring.
func StoryNotebookText(sn StoryNotebook) string {
	var lines []string
	for _, scene := range sn.Scenes {
		lines = append(lines, scene.Statements...)
		for _, conv := range scene.Conversations {
			if conv.Speaker != "" {
				lines = append(lines, fmt.Sprintf("%s: %s", conv.Speaker, conv.Quote))
			} else {
				lines = append(lines, conv.Quote)
			}
		}
	}
	return strings.Join(lines, "\n")
}

// LoadGrammars registers grammar-annotation index files from the given
// directories, keyed by story id then entry title. Opt-in, like the other
// domain loaders; a missing directory is tolerated.
func (f Reader) LoadGrammars(grammarsDirectories []string) error {
	for _, dir := range grammarsDirectories {
		if dir == "" {
			continue
		}
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
			index, err := readYamlFile[grammarsIndex](path)
			if err != nil {
				return err
			}
			byTitle := make(map[string][]Correction)
			for _, notebookPath := range index.Notebooks {
				full := filepath.Join(filepath.Dir(path), notebookPath)
				entries, err := readYamlFile[[]GrammarEntry](full)
				if err != nil {
					return fmt.Errorf("readYamlFile(%s) > %w", full, err)
				}
				for _, e := range entries {
					byTitle[e.Metadata.Title] = append(byTitle[e.Metadata.Title], e.Corrections...)
				}
			}
			f.grammarsMap[index.ID] = byTitle
			return nil
		})
		if err != nil {
			return fmt.Errorf("walk grammars(%s) > %w", dir, err)
		}
	}
	return nil
}

// GrammarStoryIDs returns the story ids that have a grammars notebook.
func (f Reader) GrammarStoryIDs() []string {
	ids := make([]string, 0, len(f.grammarsMap))
	for id := range f.grammarsMap {
		ids = append(ids, id)
	}
	return ids
}

// CorrectionsForEntry returns the corrections annotated for one story entry
// (by title), or nil if none.
func (f Reader) CorrectionsForEntry(storyID, title string) []Correction {
	byTitle, ok := f.grammarsMap[storyID]
	if !ok {
		return nil
	}
	return byTitle[title]
}
