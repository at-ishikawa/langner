package notebook

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// EtymologyOrigin represents a single etymology origin (root, prefix, or suffix).
type EtymologyOrigin struct {
	Origin   string `yaml:"origin"`
	Type     string `yaml:"type,omitempty"` // prefix, suffix, root ("" means root)
	Language string `yaml:"language"`       // Latin, Greek, etc.
	Meaning  string `yaml:"meaning"`

	// Sense optionally disambiguates same-session multi-sense origins. When a
	// session declares the same (origin, language) twice with different
	// meanings (e.g. pathos = feeling vs. pathos = disease, both in Session 9),
	// each entry needs its own Sense token so the unique key
	// (notebook_id, session_title, origin, language, sense) keeps them apart.
	// Single-sense origins leave Sense empty and behave exactly as before.
	Sense string `yaml:"sense,omitempty"`

	// Forms records inflectional / morphological variants of this origin
	// (Latin principal parts, French gender, Greek noun stems, …). See
	// examples/etymology/SCHEMA.md for usage.
	Forms []EtymologyOriginForm `yaml:"forms,omitempty"`

	// SessionTitle is the parent session's title. Set at read time from
	// the surrounding event/metadata block; not serialised.
	SessionTitle string `yaml:"-"`

	// SceneTitle is the scene the origin belongs to. For new-shape source
	// files (event/scenes/origins) it's the surrounding scene's title.
	// For legacy flat-shape files it's projected at read time by looking
	// up any definitions notebook scene whose `origin_parts` reference
	// this origin (same-notebook+session preferred, then any-notebook
	// fallback, finally a synthetic scene = session title). Not serialised.
	SceneTitle string `yaml:"-"`
}

// EtymologyNotebookEntry mirrors the story/definitions notebook structure.
// Each entry is one session containing scenes; each scene holds origins.
// The schema parallels DefinitionsScene so etymology source files share
// the same outer hierarchy as definitions notebooks. Concepts and
// Relations remain session-scoped (not scene-scoped) — they describe
// groupings across the whole session, mirroring the legacy flat shape.
type EtymologyNotebookEntry struct {
	Event     string                   `yaml:"event"`
	Date      time.Time                `yaml:"date,omitempty"`
	Scenes    []EtymologyNotebookScene `yaml:"scenes"`
	Concepts  []Concept                `yaml:"concepts,omitempty"`
	Relations []Relation               `yaml:"relations,omitempty"`
}

// EtymologyNotebookScene is one scene within a session, holding origins.
// The scene title is shared with the corresponding definitions file's
// scene of the same name — that's the key contract: a session's
// definitions and origins line up scene-by-scene under a common title.
type EtymologyNotebookScene struct {
	Scene   string            `yaml:"scene"`
	Origins []EtymologyOrigin `yaml:"origins"`
}

// EtymologyIndex represents an index file for etymology directories.
type EtymologyIndex struct {
	ID            string   `yaml:"id"`
	Kind          string   `yaml:"kind"` // "Etymology"
	Name          string   `yaml:"name"`
	NotebookPaths []string `yaml:"notebooks"`

	// internal fields
	Path       string            `yaml:"-"`
	Origins    []EtymologyOrigin `yaml:"-"`
	LatestDate time.Time         `yaml:"-"` // max session-file `date`, used to sort notebooks
}

// EtymologyDefinitionEntry is a definition with origin_parts in an etymology session file.
type EtymologyDefinitionEntry struct {
	Definition   string          `yaml:"definition"`
	Expression   string          `yaml:"expression"`
	Meaning      string          `yaml:"meaning"`
	PartOfSpeech string          `yaml:"part_of_speech"`
	Note         string          `yaml:"note"`
	OriginParts  []OriginPartRef `yaml:"origin_parts"`
	NotebookName string          `yaml:"-"` // set at read time
	SessionTitle string          `yaml:"-"` // set at read time from session metadata.title
}

// GetExpression returns the expression, falling back to the definition field.
func (e EtymologyDefinitionEntry) GetExpression() string {
	if e.Expression != "" {
		return e.Expression
	}
	return e.Definition
}

// EtymologySessionMetadata contains required metadata for an etymology session
// YAML file. Title is used as the disambiguator for multi-sense origins (the
// same origin string can appear in multiple sessions with different meanings)
// and is the binding key for definitions referencing those origins.
type EtymologySessionMetadata struct {
	Title string `yaml:"title"`
}

// etymologySessionFile is the LEGACY wrapped format for etymology session
// YAML files (kept for back-compat with existing user data):
//
//	metadata:
//	  title: "Session 13"
//	date: 2025-01-15  # optional
//	origins:
//	  - origin: ...
//	definitions:        # optional
//	  - ...
//
// New-shape source files use the per-event/scenes structure mirroring
// definitions notebooks:
//
//	- event: "Session 13"
//	  date: 2025-01-15
//	  scenes:
//	    - scene: "ana (up, back)"
//	      origins:
//	        - origin: ...
//
// The reader detects which shape a file uses and normalises both into a
// unified in-memory representation where every origin carries a
// (SessionTitle, SceneTitle) pair.
type etymologySessionFile struct {
	Metadata    EtymologySessionMetadata   `yaml:"metadata"`
	Origins     []EtymologyOrigin          `yaml:"origins"`
	Definitions []EtymologyDefinitionEntry `yaml:"definitions"`
	Concepts    []Concept                  `yaml:"concepts,omitempty"`
	Relations   []Relation                 `yaml:"relations,omitempty"`
	Date        time.Time                  `yaml:"date,omitempty"`
}

// OriginSceneCandidate records one (notebookID, session, scene) location
// where a definition's origin_parts referenced a particular origin. The
// rank fields capture book/source position so pickBestSceneForOrigin
// can apply the "earliest in a book" canonical-scene rule: SessionRank
// is the position of the session file in the book's index.yml; SceneRank
// is the position of the scene within the session's definitions file;
// ExprRank is the position of the first expression in the scene whose
// origin_parts references this origin.
type OriginSceneCandidate struct {
	NotebookID   string
	SessionTitle string
	SceneTitle   string
	SessionRank  int
	SceneRank    int
	ExprRank     int
}

// ReadEtymologyNotebook reads the origins from an etymology notebook,
// supporting both source shapes:
//
//   - New (preferred): `- event: ... scenes: [{scene, origins}]`,
//     mirroring the definitions notebook layout. Origins inherit
//     SessionTitle from event and SceneTitle from scene.
//   - Legacy: `metadata.title + flat origins` (and optional
//     definitions). Origins inherit SessionTitle from metadata.title;
//     SceneTitle is projected at read time by looking up any
//     definition (across all notebooks) whose `origin_parts`
//     reference the origin. Same-notebook+session match wins; falls
//     back to same-notebook, then any-notebook, finally a synthetic
//     scene = session title.
//
// Multi-sense origins (same string in multiple sessions) are
// disambiguated by the (SessionTitle, SceneTitle) pair downstream.
func (r *Reader) ReadEtymologyNotebook(etymologyID string) ([]EtymologyOrigin, error) {
	index, ok := r.etymologyIndexes[etymologyID]
	if !ok {
		return nil, fmt.Errorf("etymology notebook %s not found", etymologyID)
	}

	if len(index.Origins) > 0 {
		return index.Origins, nil
	}

	candidates := r.buildOriginSceneIndex()

	var allOrigins []EtymologyOrigin
	for _, notebookPath := range index.NotebookPaths {
		path := filepath.Join(index.Path, notebookPath)
		origins, err := r.readEtymologySessionFile(path, etymologyID, candidates)
		if err != nil {
			return nil, err
		}
		allOrigins = append(allOrigins, origins...)
	}

	index.Origins = allOrigins
	r.etymologyIndexes[etymologyID] = index
	return allOrigins, nil
}

// readEtymologySessionFile reads a single etymology session file and
// returns its origins with SessionTitle and SceneTitle populated. Tries
// the new event/scenes/origins shape first; falls back to the legacy
// metadata+flat-origins shape if the file has no event entries.
func (r *Reader) readEtymologySessionFile(
	path, etymologyID string,
	candidates map[string][]OriginSceneCandidate,
) ([]EtymologyOrigin, error) {
	// Try new shape first: a top-level list of event entries.
	if newShape, err := readYamlFile[[]EtymologyNotebookEntry](path); err == nil && hasNewShape(newShape) {
		var out []EtymologyOrigin
		for _, entry := range newShape {
			session := strings.TrimSpace(entry.Event)
			if session == "" {
				return nil, fmt.Errorf("etymology session %s: event title is empty in new-shape entry", path)
			}
			for _, scene := range entry.Scenes {
				sceneTitle := strings.TrimSpace(scene.Scene)
				for _, o := range scene.Origins {
					o.SessionTitle = session
					o.SceneTitle = sceneTitle
					if o.SceneTitle == "" {
						o.SceneTitle = session
					}
					out = append(out, o)
				}
			}
		}
		if len(out) > 0 {
			return out, nil
		}
	}

	// Legacy shape: metadata + flat origins.
	wrapped, err := readYamlFile[etymologySessionFile](path)
	if err != nil {
		return nil, fmt.Errorf("read etymology session %s: %w", path, err)
	}
	session := strings.TrimSpace(wrapped.Metadata.Title)
	if session == "" {
		return nil, fmt.Errorf("etymology session %s missing required metadata.title", path)
	}
	out := make([]EtymologyOrigin, 0, len(wrapped.Origins))
	for _, o := range wrapped.Origins {
		o.SessionTitle = session
		o.SceneTitle = pickBestSceneForOrigin(candidates, o.Origin, etymologyID, session)
		out = append(out, o)
	}
	return out, nil
}

// hasNewShape returns true when the parsed YAML contains at least one
// event entry — the new etymology source shape. Empty parses are
// treated as "not new shape" so the legacy fallback can run.
func hasNewShape(entries []EtymologyNotebookEntry) bool {
	for _, e := range entries {
		if strings.TrimSpace(e.Event) != "" || len(e.Scenes) > 0 {
			return true
		}
	}
	return false
}

// IsNewShapeEtymologyFile reports whether an etymology session YAML at
// the given path already uses the new event/scenes/origins shape. Used
// by the one-time migrator to skip already-converted files. A parse
// failure against the new-shape struct (legacy files parse as a map,
// not a list) is treated as "not new shape" so the migrator can proceed
// to read the file as legacy.
func IsNewShapeEtymologyFile(path string) (bool, error) {
	entries, err := readYamlFile[[]EtymologyNotebookEntry](path)
	if err != nil {
		return false, nil
	}
	return hasNewShape(entries), nil
}

// LegacyEtymologySession is the public view of an etymology session YAML
// in the legacy metadata+flat-origins shape. Exported for tooling
// (migrators, validators) that need raw access without going through the
// Reader's scene-projection pipeline.
type LegacyEtymologySession struct {
	Metadata    EtymologySessionMetadata
	Origins     []EtymologyOrigin
	Definitions []EtymologyDefinitionEntry
	Concepts    []Concept
	Relations   []Relation
	Date        time.Time
}

// ReadLegacyEtymologySession reads one legacy-shape etymology session
// file. Callers that handle both shapes should use the Reader instead;
// this is for tooling that explicitly needs the unprocessed legacy view.
func ReadLegacyEtymologySession(path string) (LegacyEtymologySession, error) {
	wrapped, err := readYamlFile[etymologySessionFile](path)
	if err != nil {
		return LegacyEtymologySession{}, err
	}
	return LegacyEtymologySession(wrapped), nil
}

// loadEtymologySessionAnyShape reads an etymology session file in either
// the new event/scenes/origins shape or the legacy metadata+flat-origins
// shape, and projects the result into the legacy etymologySessionFile
// struct so existing callers don't need to know which shape the file
// uses. SceneTitle on origins is intentionally dropped — callers that
// need it should go through Reader.ReadEtymologyNotebook.
func loadEtymologySessionAnyShape(path string) (etymologySessionFile, error) {
	if newShape, err := readYamlFile[[]EtymologyNotebookEntry](path); err == nil && hasNewShape(newShape) {
		var out etymologySessionFile
		for _, entry := range newShape {
			if out.Metadata.Title == "" {
				out.Metadata.Title = entry.Event
			}
			if !entry.Date.IsZero() && out.Date.IsZero() {
				out.Date = entry.Date
			}
			for _, scene := range entry.Scenes {
				out.Origins = append(out.Origins, scene.Origins...)
			}
			out.Concepts = append(out.Concepts, entry.Concepts...)
			out.Relations = append(out.Relations, entry.Relations...)
		}
		return out, nil
	}
	return readYamlFile[etymologySessionFile](path)
}

// ReadEtymologyIndex loads an etymology directory's index.yml.
func ReadEtymologyIndex(path string) (EtymologyIndex, error) {
	idx, err := readYamlFile[EtymologyIndex](path)
	if err != nil {
		return EtymologyIndex{}, err
	}
	idx.Path = filepath.Dir(path)
	return idx, nil
}

// ReadEtymologyFromBytes parses YAML bytes as a new-shape etymology
// session file ([]EtymologyNotebookEntry). Used by tests and tooling
// that inspect migrator output without going through the Reader.
func ReadEtymologyFromBytes(data []byte) ([]EtymologyNotebookEntry, error) {
	var entries []EtymologyNotebookEntry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("yaml.Unmarshal: %w", err)
	}
	return entries, nil
}

// pickBestSceneForOrigin selects the canonical scene title for an
// origin, applying the earliest-rule: within a book, the chosen scene
// is the one where the origin first appears (earliest session in the
// book's index.yml, then earliest scene within that session's
// definitions file, then earliest expression within that scene). This
// matches where a user would naturally place an origin's learning
// history — at its introduction point in the book — and stops the
// "two logos sessions" drift that alphabetical sorting caused when a
// combinator origin like logos is referenced from multiple scenes.
//
// Preference order: same-notebook+session (earliest scene/expr) >
// same-notebook (earliest session, then scene, then expr) > any
// notebook (earliest by notebook id, then session/scene/expr) >
// synthetic fallback = the origin's session title.
func pickBestSceneForOrigin(
	candidates map[string][]OriginSceneCandidate,
	origin, originNotebookID, originSessionTitle string,
) string {
	key := strings.ToLower(strings.TrimSpace(origin))
	matches := candidates[key]

	// Sort by (NotebookID, SessionRank, SceneRank, ExprRank). NotebookID
	// is alphabetical (books don't have a globally-ordered list); the
	// rank fields capture position-within-the-book and break ties by
	// "first introduction". The candidate accumulator (buildOriginSceneIndex)
	// records ranks during the walk so the sort is deterministic across
	// runs regardless of Go's map iteration order.
	sorted := make([]OriginSceneCandidate, len(matches))
	copy(sorted, matches)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].NotebookID != sorted[j].NotebookID {
			return sorted[i].NotebookID < sorted[j].NotebookID
		}
		if sorted[i].SessionRank != sorted[j].SessionRank {
			return sorted[i].SessionRank < sorted[j].SessionRank
		}
		if sorted[i].SceneRank != sorted[j].SceneRank {
			return sorted[i].SceneRank < sorted[j].SceneRank
		}
		return sorted[i].ExprRank < sorted[j].ExprRank
	})
	matches = sorted

	var bestSameSession, bestSameNotebook, bestAny string
	for _, c := range matches {
		if c.SceneTitle == "" {
			continue
		}
		if c.NotebookID == originNotebookID && c.SessionTitle == originSessionTitle {
			bestSameSession = c.SceneTitle
			break
		}
		if bestSameNotebook == "" && c.NotebookID == originNotebookID {
			bestSameNotebook = c.SceneTitle
		}
		if bestAny == "" {
			bestAny = c.SceneTitle
		}
	}
	switch {
	case bestSameSession != "":
		return bestSameSession
	case bestSameNotebook != "":
		return bestSameNotebook
	case bestAny != "":
		return bestAny
	default:
		// Synthetic fallback so every origin always has a non-empty
		// SceneTitle — downstream writers and the locator invariant
		// require it.
		return originSessionTitle
	}
}

// buildOriginSceneIndex scans every notebook source the reader knows
// about (story scenes, definitions notebooks, flashcards, etymology
// session-file definitions) and indexes which (notebook, session,
// scene) tuples reference each origin via origin_parts. Used by the
// legacy-shape projection in ReadEtymologyNotebook to attach a scene
// title to flat-shape origins.
//
// The result is keyed by lowered+trimmed origin string. Identical
// origins referenced from multiple scenes accumulate as separate
// candidates so pickBestSceneForOrigin can apply the contextual
// preference order.
func (r *Reader) buildOriginSceneIndex() map[string][]OriginSceneCandidate {
	add := func(
		out map[string][]OriginSceneCandidate,
		origin, notebookID, sessionTitle, sceneTitle string,
		sessionRank, sceneRank, exprRank int,
	) {
		key := strings.ToLower(strings.TrimSpace(origin))
		if key == "" {
			return
		}
		out[key] = append(out[key], OriginSceneCandidate{
			NotebookID:   notebookID,
			SessionTitle: sessionTitle,
			SceneTitle:   sceneTitle,
			SessionRank:  sessionRank,
			SceneRank:    sceneRank,
			ExprRank:     exprRank,
		})
	}
	out := make(map[string][]OriginSceneCandidate)

	// Story notebooks: each event has scenes with definitions carrying
	// origin_parts. The "session title" is the event name. Rank is taken
	// from the file's traversal order so the earliest-rule picks the
	// first introduction.
	for storyID, idx := range r.indexes {
		for nbRank, nbPath := range idx.NotebookPaths {
			path := filepath.Join(idx.Path, nbPath)
			notebooks, err := readYamlFile[[]StoryNotebook](path)
			if err != nil {
				continue
			}
			for _, nb := range notebooks {
				for sceneRank, scene := range nb.Scenes {
					for exprRank, def := range scene.Definitions {
						for _, op := range def.OriginParts {
							add(out, op.Origin, storyID, nb.Event, scene.Title, nbRank, sceneRank, exprRank)
						}
					}
				}
			}
		}
	}

	// Definitions notebooks: per-book per-session scenes with
	// expressions. SessionRank is the position of the session file in
	// the book's definitions index.yml — definitionsRaw[bookID] is
	// populated in index.yml order, so the slice index IS the rank.
	for bookID, defs := range r.definitionsRaw {
		for sessionRank, fileDefs := range defs {
			session := fileDefs.Metadata.Title
			for sceneRank, scene := range fileDefs.Scenes {
				sceneTitle := scene.Metadata.Title
				for exprRank, note := range scene.Expressions {
					for _, op := range note.OriginParts {
						add(out, op.Origin, bookID, session, sceneTitle, sessionRank, sceneRank, exprRank)
					}
				}
			}
		}
	}

	// Etymology session files (legacy): top-level Definitions with
	// origin_parts but no scene level. Those don't contribute scene
	// titles by themselves, but their session titles matter when the
	// fallback needs "any notebook, any scene" — we record them with
	// a synthetic scene = session title so they sort behind real
	// scene matches.
	for etymID, idx := range r.etymologyIndexes {
		for nbRank, nbPath := range idx.NotebookPaths {
			path := filepath.Join(idx.Path, nbPath)
			wrapped, err := loadEtymologySessionAnyShape(path)
			if err != nil {
				continue
			}
			session := wrapped.Metadata.Title
			for exprRank, def := range wrapped.Definitions {
				for _, op := range def.OriginParts {
					add(out, op.Origin, etymID, session, session, nbRank, 0, exprRank)
				}
			}
		}
	}

	return out
}

// ReadAllEtymologyDefinitions reads definitions with origin_parts from ALL
// notebook session files: etymology notebooks, story notebooks, and flashcard notebooks.
//
// SessionTitle on each returned entry is set from the session file's
// metadata.title — the binding key used by Phase 5 to disambiguate which sense
// of an origin a definition refers to.
func (r *Reader) ReadAllEtymologyDefinitions() []EtymologyDefinitionEntry {
	var allDefs []EtymologyDefinitionEntry
	seen := make(map[string]bool) // track scanned paths to avoid duplicates

	scanSessionFiles := func(dir string, paths []string, notebookName string) {
		for _, nbPath := range paths {
			path := filepath.Join(dir, nbPath)
			if seen[path] {
				continue
			}
			seen[path] = true
			wrapped, err := readYamlFile[etymologySessionFile](path)
			if err != nil {
				continue
			}
			sessionTitle := strings.TrimSpace(wrapped.Metadata.Title)
			for _, def := range wrapped.Definitions {
				if len(def.OriginParts) > 0 {
					def.NotebookName = notebookName
					def.SessionTitle = sessionTitle
					allDefs = append(allDefs, def)
				}
			}
		}
	}

	// Scan etymology notebook session files
	for _, index := range r.etymologyIndexes {
		scanSessionFiles(index.Path, index.NotebookPaths, index.Name)
	}

	// Scan story notebook session files
	for _, index := range r.indexes {
		scanSessionFiles(index.Path, index.NotebookPaths, index.Name)
	}

	// Scan flashcard notebook session files
	for _, index := range r.flashcardIndexes {
		scanSessionFiles(index.Path, index.NotebookPaths, index.Name)
	}

	return allDefs
}

// GetEtymologyIndexes returns the etymology indexes map.
func (r *Reader) GetEtymologyIndexes() map[string]EtymologyIndex {
	return r.etymologyIndexes
}

// GetEtymologyConceptsBySession returns each session file's concepts +
// relations keyed by the session title (the YAML event or
// metadata.title). Used by the quiz-review writer to render the
// concept block sitting in the session a failed origin or word came
// from, without paying the cost of book-wide aggregation. Returns nil
// maps when the book is unknown.
func (r *Reader) GetEtymologyConceptsBySession(etymologyID string) (concepts map[string][]Concept, relations map[string][]Relation) {
	index, ok := r.etymologyIndexes[etymologyID]
	if !ok {
		return nil, nil
	}
	concepts = make(map[string][]Concept)
	relations = make(map[string][]Relation)
	for _, nb := range index.NotebookPaths {
		path := filepath.Join(index.Path, nb)
		title := sessionTitleFromFile(path)
		if title == "" {
			continue
		}
		sessionConcepts, sessionRelations := readSessionConceptsAndRelations(path)
		concepts[title] = append(concepts[title], sessionConcepts...)
		relations[title] = append(relations[title], sessionRelations...)
	}
	return concepts, relations
}

// sessionTitleFromFile reads only enough of an etymology session YAML
// to discover the session title (the new-shape `event:` field or the
// legacy `metadata.title`). Both shapes are tried; the first that
// yields a non-empty title wins. Returns "" when neither is present.
func sessionTitleFromFile(path string) string {
	if newShape, err := readYamlFile[[]EtymologyNotebookEntry](path); err == nil {
		for _, entry := range newShape {
			if t := strings.TrimSpace(entry.Event); t != "" {
				return t
			}
		}
	}
	if wrapped, err := readYamlFile[etymologySessionFile](path); err == nil {
		return strings.TrimSpace(wrapped.Metadata.Title)
	}
	return ""
}

// GetEtymologyConceptsAndRelations aggregates concepts and relations
// declared across every session file in the etymology book. Concepts
// are merged by Key (per the schema's "same key across sessions =
// member-union" rule). Relations are returned as a flat slice in
// declaration order. Returns (nil, nil) when the book is unknown.
//
// Used by the analytics resolver to surface the concept graph next to
// each wrong word: definitions-book sibling expressions, sibling
// origins under the same etymology concept, and related-concept members
// connected by antonym / synonym / hyponym / similar relations.
func (r *Reader) GetEtymologyConceptsAndRelations(etymologyID string) ([]Concept, []Relation) {
	index, ok := r.etymologyIndexes[etymologyID]
	if !ok {
		return nil, nil
	}
	conceptByKey := make(map[string]int)
	var concepts []Concept
	var relations []Relation
	seenMember := make(map[string]map[conceptMemberKey]bool)
	for _, nb := range index.NotebookPaths {
		path := filepath.Join(index.Path, nb)
		sessionConcepts, sessionRelations := readSessionConceptsAndRelations(path)
		for _, c := range sessionConcepts {
			if c.Key == "" {
				continue
			}
			idx, ok := conceptByKey[c.Key]
			if !ok {
				concepts = append(concepts, Concept{
					Key:     c.Key,
					Meaning: c.Meaning,
					Note:    c.Note,
				})
				idx = len(concepts) - 1
				conceptByKey[c.Key] = idx
				seenMember[c.Key] = make(map[conceptMemberKey]bool)
			}
			for _, m := range c.Members {
				k := conceptMemberKey(m)
				if seenMember[c.Key][k] {
					continue
				}
				seenMember[c.Key][k] = true
				concepts[idx].Members = append(concepts[idx].Members, m)
			}
		}
		relations = append(relations, sessionRelations...)
	}
	return concepts, relations
}

// sessionHasOriginsKey checks if a session YAML file has a top-level "origins:" key,
// indicating it defines etymology origins (not just references them via origin_parts).
func sessionHasOriginsKey(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// Check for "origins:" at the start of a line (top-level YAML key)
	for _, line := range bytes.Split(data, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if bytes.Equal(trimmed, []byte("origins:")) || bytes.HasPrefix(trimmed, []byte("origins: ")) {
			return true
		}
	}
	return false
}

// walkEtymologyIndexFiles walks a directory and loads etymology index.yml files.
// It loads indexes with kind: Etymology, and also indexes without a kind if their
// session files contain origin_parts (indicating etymology data).
func walkEtymologyIndexFiles(rootDir string, indexMap map[string]EtymologyIndex) error {
	if rootDir == "" {
		return nil
	}

	if _, err := os.Stat(rootDir); os.IsNotExist(err) {
		return nil
	}

	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) != "index.yml" {
			return nil
		}

		index, err := readYamlFile[EtymologyIndex](path)
		if err != nil {
			return err
		}

		dir := filepath.Dir(path)
		isEtymology := index.Kind == "Etymology"

		// For indexes without kind: Etymology, check if session files define origins
		if !isEtymology && index.Kind == "" {
			for _, nbPath := range index.NotebookPaths {
				if sessionHasOriginsKey(filepath.Join(dir, nbPath)) {
					isEtymology = true
					break
				}
			}
		}

		if !isEtymology {
			return nil
		}

		index.Path = dir

		// Read the `date` field from each session file and keep the latest.
		// Each session file must have metadata.title — fail fast otherwise so
		// the operator notices missing metadata at startup rather than at quiz
		// time when origins disappear from the deduplicated set.
		for _, nbPath := range index.NotebookPaths {
			sessionPath := filepath.Join(dir, nbPath)
			wrapped, err := loadEtymologySessionAnyShape(sessionPath)
			if err != nil {
				return fmt.Errorf("read etymology session %s: %w", sessionPath, err)
			}
			if strings.TrimSpace(wrapped.Metadata.Title) == "" {
				return fmt.Errorf("etymology session %s missing required metadata.title", sessionPath)
			}
			if wrapped.Date.After(index.LatestDate) {
				index.LatestDate = wrapped.Date
			}
		}

		indexMap[index.ID] = index
		return nil
	})
}
