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

// GrammarEntry holds the corrections for one story entry, matched by title, and
// split by scene — the same shape as a definitions entry (scenes keyed by
// index), so a multi-scene story maps each scene's corrections to that scene.
type GrammarEntry struct {
	Metadata GrammarMetadata `yaml:"metadata"`
	Scenes   []GrammarScene  `yaml:"scenes"`
}

// GrammarMetadata identifies which story entry the corrections apply to.
type GrammarMetadata struct {
	Title string `yaml:"title"` // matches StoryNotebook.Event
}

// GrammarScene holds the corrections for one scene of a story entry.
type GrammarScene struct {
	Metadata    GrammarSceneMetadata `yaml:"metadata"`
	Corrections []Correction         `yaml:"corrections"`
}

// GrammarSceneMetadata identifies a scene, mirroring DefinitionsSceneMetadata:
// index (0-based position in the entry's scenes), an alternative `scene` field,
// and an optional human-readable title.
type GrammarSceneMetadata struct {
	Index int    `yaml:"index"`
	Scene *int   `yaml:"scene,omitempty"`
	Title string `yaml:"title,omitempty"`
}

// GetIndex returns the scene index, preferring Scene if set, otherwise Index.
func (m GrammarSceneMetadata) GetIndex() int {
	if m.Scene != nil {
		return *m.Scene
	}
	return m.Index
}

// grammarsIndex is the index.yml of a grammars directory (id + notebook files).
type grammarsIndex struct {
	ID        string   `yaml:"id"`
	Notebooks []string `yaml:"notebooks"`
}

// DerivedCorrectionID returns a correction's stable id, deriving one from the
// story id, entry title, scene index, and per-scene sequence when no explicit
// id is set.
func DerivedCorrectionID(storyID, title string, sceneIndex, seq int) string {
	return fmt.Sprintf("%s-%s-s%d-%d", storyID, slugifyTitle(title), sceneIndex, seq)
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

// sceneLine renders one statement or spoken quote as a line of plain text.
func sceneLine(conv Conversation) string {
	if conv.Speaker != "" {
		return fmt.Sprintf("%s: %s", conv.Speaker, conv.Quote)
	}
	return conv.Quote
}

// StorySceneText renders one scene as plain text — one statement or spoken quote
// per line — so the grammar quiz can show that scene whole and locate each
// incorrect span as a substring.
func StorySceneText(scene StoryScene) string {
	lines := append([]string{}, scene.Statements...)
	for _, conv := range scene.Conversations {
		lines = append(lines, sceneLine(conv))
	}
	return strings.Join(lines, "\n")
}

// LoadGrammars registers grammar-annotation index files from the given
// directories, keyed by story id → entry title → scene index → corrections.
// Opt-in, like the other domain loaders; a missing directory is tolerated.
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
			byTitle := make(map[string]map[int][]Correction)
			for _, notebookPath := range index.Notebooks {
				full := filepath.Join(filepath.Dir(path), notebookPath)
				entries, err := readYamlFile[[]GrammarEntry](full)
				if err != nil {
					return fmt.Errorf("readYamlFile(%s) > %w", full, err)
				}
				for _, e := range entries {
					byScene := byTitle[e.Metadata.Title]
					if byScene == nil {
						byScene = make(map[int][]Correction)
						byTitle[e.Metadata.Title] = byScene
					}
					for _, gs := range e.Scenes {
						idx := gs.Metadata.GetIndex()
						byScene[idx] = append(byScene[idx], gs.Corrections...)
					}
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

// CorrectionsForScene returns the corrections annotated for one scene of a
// story entry (by title + scene index), or nil if none.
func (f Reader) CorrectionsForScene(storyID, title string, sceneIndex int) []Correction {
	byTitle, ok := f.grammarsMap[storyID]
	if !ok {
		return nil
	}
	byScene, ok := byTitle[title]
	if !ok {
		return nil
	}
	return byScene[sceneIndex]
}
