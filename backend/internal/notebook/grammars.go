package notebook

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
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

// CorrectionID returns a correction's stable spaced-repetition id: its
// explicit id when set, otherwise one derived from the story id, entry
// title, scene index, and order. This is the single canonical derivation —
// every reader of a grammar correction's identity (the grammar quiz itself,
// the Relearn pool, the Analytics resolver) calls this instead of
// re-deriving the rule, so the id a correction is stored under never drifts
// from the id used to look it up back up (see .claude/rules/learning-
// history-invariants.md, L2).
func CorrectionID(storyID, title string, sceneIndex, seq int, c Correction) string {
	if strings.TrimSpace(c.ID) != "" {
		return c.ID
	}
	return DerivedCorrectionID(storyID, title, sceneIndex, seq)
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

// correctionContentKey is the stable, order-independent identity of a
// correction's CONTENT (independent of any assigned id): its mistake span, its
// fix, category, and reason. Two corrections with the same content key are the
// same correction; a different key means a genuinely distinct correction.
func correctionContentKey(c Correction) string {
	return strings.Join([]string{c.Incorrect, c.Correct, c.Category, c.Reason}, "\x00")
}

// correctionDiscriminator derives a short, stable suffix that disambiguates two
// DISTINCT corrections which would otherwise share one id. It depends solely on
// the correction's content, so the id a colliding correction ends up with is
// identical across process restarts and file re-reads, regardless of load order.
func correctionDiscriminator(c Correction) string {
	sum := sha256.Sum256([]byte(correctionContentKey(c)))
	return hex.EncodeToString(sum[:])[:8]
}

// correctionLocation points at a correction by its slot within a notebook's
// title → scene index → corrections structure, so the uniqueness pass can
// reassign an id or drop a duplicate in place.
type correctionLocation struct {
	title string
	scene int
	idx   int
}

// ensureUniqueCorrectionIDs guarantees every grammar correction in a notebook
// has a UNIQUE stable id, so two distinct corrections can never share one
// learning-log series (see .claude/rules/learning-history-invariants.md, L1 —
// one canonical storage key per learning log).
//
// A correction's id can collide two ways: two corrections carry the same
// explicit `id:`, or two entry titles slugify to the same string and land at
// the same scene index + sequence (DerivedCorrectionID). Either makes
// SaveGrammarBlank write, and every reader look up, ONE series for two
// corrections — so answering one correctly drops the other from every quiz and
// from the Relearn pool.
//
// The pass freezes each correction's CURRENT id (via the existing CorrectionID
// derivation) into an explicit ID first, so a later drop can't shift a
// sequence-derived id; then, for any id shared by more than one correction, it
// drops byte-identical duplicates and disambiguates the rest with a
// content-derived suffix. Non-colliding corrections keep their exact current id
// (their existing history is preserved); only the extra colliding corrections
// change id — their wrongly-conflated progress resets, which is the correct
// outcome.
func ensureUniqueCorrectionIDs(storyID string, byTitle map[string]map[int][]Correction) {
	// Phase 1: freeze every correction's current id into an explicit ID. The
	// value is position-based within a (title, scene) slice, so it is identical
	// regardless of the order we visit titles/scenes here.
	byID := map[string][]correctionLocation{}
	for _, title := range sortedKeys(byTitle) {
		byScene := byTitle[title]
		for _, sceneIdx := range sortedKeys(byScene) {
			corrections := byScene[sceneIdx]
			for i := range corrections {
				id := CorrectionID(storyID, title, sceneIdx, i+1, corrections[i])
				corrections[i].ID = id
				byID[id] = append(byID[id], correctionLocation{title, sceneIdx, i})
			}
		}
	}

	get := func(l correctionLocation) Correction { return byTitle[l.title][l.scene][l.idx] }

	// Phase 2: resolve every id shared by more than one correction.
	newIDs := map[correctionLocation]string{}
	drop := map[correctionLocation]bool{}
	for _, id := range sortedKeys(byID) { // deterministic warning order
		locs := byID[id]
		if len(locs) == 1 {
			continue
		}
		// Order members by content so both the base-keeper and any suffixes are
		// chosen from content alone — never from file/array order.
		slices.SortStableFunc(locs, func(a, b correctionLocation) int {
			return cmp.Compare(correctionContentKey(get(a)), correctionContentKey(get(b)))
		})
		var kept []correctionLocation
		for _, l := range locs {
			key := correctionContentKey(get(l))
			isDup := false
			for _, k := range kept {
				if correctionContentKey(get(k)) == key {
					isDup = true
					break
				}
			}
			if isDup {
				drop[l] = true
				slog.Warn("dropped duplicate grammar correction",
					"story", storyID, "id", id, "title", l.title, "incorrect", get(l).Incorrect)
				continue
			}
			kept = append(kept, l)
		}
		for i, l := range kept {
			if i == 0 {
				continue // the canonical occurrence keeps the base id
			}
			newID := id + "-" + correctionDiscriminator(get(l))
			newIDs[l] = newID
			slog.Warn("disambiguated colliding grammar correction id",
				"story", storyID, "base_id", id, "new_id", newID, "title", l.title, "incorrect", get(l).Incorrect)
		}
	}

	// Apply reassigned ids in place (slices still intact), then rebuild each
	// scene slice without the dropped duplicates (ids are frozen, so removing a
	// slot can no longer shift any surviving correction's id).
	for l, id := range newIDs {
		byTitle[l.title][l.scene][l.idx].ID = id
	}
	if len(drop) > 0 {
		for title, byScene := range byTitle {
			for sceneIdx, corrections := range byScene {
				filtered := make([]Correction, 0, len(corrections))
				for i, c := range corrections {
					if drop[correctionLocation{title, sceneIdx, i}] {
						continue
					}
					filtered = append(filtered, c)
				}
				byScene[sceneIdx] = filtered
			}
		}
	}

	warnResidualDuplicateCorrectionIDs(storyID, byTitle)
}

// warnResidualDuplicateCorrectionIDs is the load-time safety net that mirrors
// the learning-history duplicate guard: after ensureUniqueCorrectionIDs has run,
// no two corrections should share an id. If any residual duplicate remains (only
// possible via an astronomically-unlikely content-hash clash), log a clear
// WARNING naming the colliding titles and spans rather than failing the load, so
// the user's notebook still works.
func warnResidualDuplicateCorrectionIDs(storyID string, byTitle map[string]map[int][]Correction) {
	type where struct{ title, incorrect string }
	seen := map[string]where{}
	for _, title := range sortedKeys(byTitle) {
		byScene := byTitle[title]
		for _, sceneIdx := range sortedKeys(byScene) {
			for _, c := range byScene[sceneIdx] {
				if first, ok := seen[c.ID]; ok {
					slog.Warn("duplicate grammar correction id after disambiguation",
						"story", storyID, "id", c.ID,
						"title", title, "incorrect", c.Incorrect,
						"first_title", first.title, "first_incorrect", first.incorrect)
					continue
				}
				seen[c.ID] = where{title, c.Incorrect}
			}
		}
	}
}

// sortedKeys returns a map's keys in ascending order, for deterministic
// iteration.
func sortedKeys[K cmp.Ordered, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
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
			ensureUniqueCorrectionIDs(index.ID, byTitle)
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
