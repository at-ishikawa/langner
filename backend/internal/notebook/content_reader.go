package notebook

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
)

// CatalogEntry describes one family of a stored notebook without its content:
// enough for a Reader to list notebooks and know which files each has.
type CatalogEntry struct {
	NotebookID string
	Family     string
	Kind       string
	Name       string
	Paths      []string
	// LatestDate is the newest session date (etymology families only).
	LatestDate time.Time
	// Version changes whenever the notebook's content is replaced.
	Version string
}

// ContentLoader is where a content Reader gets notebooks from: the Postgres
// content tables on the server, an in-memory set in tests.
type ContentLoader interface {
	Catalog(ctx context.Context) ([]CatalogEntry, error)
	Load(ctx context.Context, ids []string) ([]NotebookContent, error)
}

// DictionarySource returns the stored dictionary entries for words. A note
// whose meaning comes from the dictionary (dictionary_number set) resolves
// through it.
type DictionarySource interface {
	Responses(ctx context.Context, words []string) (map[string]rapidapi.Response, error)
}

// contentCache holds the notebooks a content Reader has loaded so far. A
// notebook is loaded the first time one of its files is read, together with
// the dictionary entries its words need.
type contentCache struct {
	ctx        context.Context
	loader     ContentLoader
	dictionary DictionarySource

	mu     sync.Mutex
	loaded map[string]*NotebookContent
	// words is the Reader's dictionary map, filled as notebooks load.
	words map[string]rapidapi.Response
}

func (c *contentCache) get(id string) (*NotebookContent, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if nc, ok := c.loaded[id]; ok {
		return nc, nil
	}
	if err := c.loadLocked([]string{id}); err != nil {
		return nil, err
	}
	return c.loaded[id], nil
}

// preload loads every id not loaded yet in one round of queries.
func (c *contentCache) preload(ids []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var missing []string
	for _, id := range ids {
		if _, ok := c.loaded[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return c.loadLocked(missing)
}

func (c *contentCache) loadLocked(ids []string) error {
	contents, err := c.loader.Load(c.ctx, ids)
	if err != nil {
		return fmt.Errorf("load notebook content %v: %w", ids, err)
	}
	for i := range contents {
		c.loaded[contents[i].ID] = &contents[i]
	}
	if c.dictionary != nil {
		if err := c.loadDictionaryLocked(contents); err != nil {
			return err
		}
	}
	for _, id := range ids {
		if _, ok := c.loaded[id]; !ok {
			c.loaded[id] = &NotebookContent{ID: id} // deleted since the catalog was read
		}
	}
	return nil
}

// loadDictionaryLocked fetches the dictionary entries the loaded notebooks'
// dictionary-backed words need.
func (c *contentCache) loadDictionaryLocked(contents []NotebookContent) error {
	var words []string
	add := func(n Note) {
		if n.DictionaryNumber <= 0 {
			return
		}
		w := n.Definition
		if w == "" {
			w = n.Expression
		}
		if _, ok := c.words[w]; !ok {
			words = append(words, w)
		}
	}
	for _, nc := range contents {
		for _, fc := range nc.Families {
			for _, f := range fc.Files {
				for _, nb := range f.Stories {
					for _, scene := range nb.Scenes {
						for _, n := range scene.Definitions {
							add(n)
						}
					}
				}
				for _, deck := range f.Flashcards {
					for _, n := range deck.Cards {
						add(n)
					}
				}
				for _, def := range f.Definitions {
					for _, scene := range def.Scenes {
						for _, n := range scene.Expressions {
							add(n)
						}
					}
				}
			}
		}
	}
	if len(words) == 0 {
		return nil
	}
	found, err := c.dictionary.Responses(c.ctx, words)
	if err != nil {
		return fmt.Errorf("load dictionary entries: %w", err)
	}
	for w, resp := range found {
		c.words[w] = resp
	}
	return nil
}

// file returns the parsed file at a content path: <family>/<notebook id>/<file>.
func (c *contentCache) file(p string) (ContentFile, error) {
	parts := strings.SplitN(path.Clean(p), "/", 3)
	if len(parts) != 3 {
		return ContentFile{}, fmt.Errorf("content path %s: want <family>/<id>/<file>", p)
	}
	nc, err := c.get(parts[1])
	if err != nil {
		return ContentFile{}, err
	}
	fc := nc.Family(parts[0])
	if fc == nil {
		return ContentFile{}, fmt.Errorf("content path %s: notebook has no %s", p, parts[0])
	}
	for _, f := range fc.Files {
		if path.Clean(f.Path) == parts[2] {
			return f, nil
		}
	}
	return ContentFile{}, fmt.Errorf("content path %s: no such file", p)
}

// readContent reads a notebook file as T: from the content cache for a content
// Reader, from disk for a directory Reader. Reading a file as a type it is not
// fails, exactly as decoding the YAML into the wrong type does.
func readContent[T any](r *Reader, p string) (T, error) {
	if r == nil || r.content == nil {
		return readYamlFile[T](p)
	}
	var zero T
	f, err := r.content.file(p)
	if err != nil {
		return zero, err
	}
	v, ok := f.value().(T)
	if !ok {
		return zero, fmt.Errorf("content path %s is a %s file", p, f.Shape)
	}
	// Loaded content can be shared (CachedContent), and readers modify what
	// they read, so each read gets its own copy.
	return deepCopy(v), nil
}

// NewContentReader builds a Reader over stored notebook content. It reads only
// the catalog up front; a notebook's content is loaded the first time it is
// read (or by Preload), with the dictionary entries its words need from
// dictionary (nil: none), so a request touches only the notebooks it uses.
func NewContentReader(ctx context.Context, loader ContentLoader, dictionary DictionarySource) (*Reader, error) {
	catalog, err := loader.Catalog(ctx)
	if err != nil {
		return nil, fmt.Errorf("read notebook catalog: %w", err)
	}
	dictionaryMap := make(map[string]rapidapi.Response)
	r := &Reader{
		indexes:          make(map[string]Index),
		flashcardIndexes: make(map[string]FlashcardIndex),
		etymologyIndexes: make(map[string]EtymologyIndex),
		grammarsMap:      make(map[string]map[string]map[int][]Correction),
		journalIDs:       make(map[string]struct{}),
		dictionaryMap:    dictionaryMap,
		definitionsMap:   make(DefinitionsMap),
		definitionsRaw:   make(map[string][]Definitions),
		definitionsDates: make(map[string]time.Time),
		content: &contentCache{
			ctx:        ctx,
			loader:     loader,
			dictionary: dictionary,
			loaded:     make(map[string]*NotebookContent),
			words:      dictionaryMap,
		},
		definitionIDs:   make(map[string]bool),
		grammarIDs:      make(map[string]bool),
		definitionsDone: make(map[string]bool),
	}
	for _, e := range catalog {
		dir := e.Family + "/" + e.NotebookID
		switch e.Family {
		case FamilyStories, FamilyBooks, FamilyJournals:
			r.indexes[e.NotebookID] = Index{
				Path:          dir,
				IsBook:        e.Family == FamilyBooks,
				Kind:          e.Kind,
				ID:            e.NotebookID,
				Name:          e.Name,
				NotebookPaths: e.Paths,
			}
			if e.Family == FamilyJournals {
				r.journalIDs[e.NotebookID] = struct{}{}
			}
		case FamilyFlashcards:
			r.flashcardIndexes[e.NotebookID] = FlashcardIndex{
				ID:            e.NotebookID,
				Name:          e.Name,
				NotebookPaths: e.Paths,
				Path:          dir,
			}
		case FamilyEtymology:
			r.etymologyIndexes[e.NotebookID] = EtymologyIndex{
				ID:            e.NotebookID,
				Kind:          e.Kind,
				Name:          e.Name,
				NotebookPaths: e.Paths,
				Path:          dir,
				LatestDate:    e.LatestDate,
			}
		case FamilyDefinitions:
			r.definitionIDs[e.NotebookID] = true
		case FamilyGrammars:
			r.grammarIDs[e.NotebookID] = true
		}
	}
	for id := range r.etymologyIndexes {
		delete(r.indexes, id)
	}
	return r, nil
}

// Dictionary returns the dictionary entries for the words of the notebooks
// read so far (a directory Reader: the dictionary it was built with).
func (r *Reader) Dictionary() map[string]rapidapi.Response {
	return r.dictionaryMap
}

// Preload loads the given notebooks' content in one round of queries, so a
// caller about to read several notebooks does not load them one at a time.
// A directory Reader has everything on disk already.
func (r *Reader) Preload(ids ...string) error {
	if r.content == nil {
		return nil
	}
	return r.content.preload(ids)
}

// PreloadMatching loads, in one round of queries, every notebook in the
// catalog that keep accepts: for a caller about to read all of them.
func (r *Reader) PreloadMatching(keep func(notebookID string) bool) error {
	if r.content == nil {
		return nil
	}
	ids := make(map[string]bool)
	for id := range r.indexes {
		ids[id] = true
	}
	for id := range r.flashcardIndexes {
		ids[id] = true
	}
	for id := range r.etymologyIndexes {
		ids[id] = true
	}
	for id := range r.definitionIDs {
		ids[id] = true
	}
	for id := range r.grammarIDs {
		ids[id] = true
	}
	list := make([]string, 0, len(ids))
	for id := range ids {
		if keep(id) {
			list = append(list, id)
		}
	}
	return r.content.preload(list)
}

// ensureDefinitions loads a definitions book into the reader's definitions
// maps the first time it is needed (content Readers only; a directory Reader
// loads every book up front).
func (r *Reader) ensureDefinitions(bookID string) {
	if r.content == nil || !r.definitionIDs[bookID] {
		return
	}
	r.content.mu.Lock()
	done := r.definitionsDone[bookID]
	r.content.mu.Unlock()
	if done {
		return
	}
	nc, err := r.content.get(bookID)
	if err == nil {
		if fc := nc.Family(FamilyDefinitions); fc != nil {
			for _, f := range fc.Files {
				addDefinitions(deepCopy(f.Definitions), bookID, r.definitionsMap, r.definitionsRaw, r.definitionsDates)
			}
		}
	}
	r.content.mu.Lock()
	r.definitionsDone[bookID] = true
	r.content.mu.Unlock()
}

// PreloadEtymology loads every etymology notebook in one round of queries.
func (r *Reader) PreloadEtymology() error {
	ids := make([]string, 0, len(r.etymologyIndexes))
	for id := range r.etymologyIndexes {
		ids = append(ids, id)
	}
	return r.Preload(ids...)
}

// ensureAllDefinitions loads every definitions book.
func (r *Reader) ensureAllDefinitions() {
	if r.content == nil {
		return
	}
	ids := make([]string, 0, len(r.definitionIDs))
	for id := range r.definitionIDs {
		ids = append(ids, id)
	}
	_ = r.content.preload(ids)
	for _, id := range ids {
		r.ensureDefinitions(id)
	}
}

// ensureGrammars loads a story's grammar corrections the first time they are
// needed (content Readers only).
func (r *Reader) ensureGrammars(storyID string) {
	if r.content == nil || !r.grammarIDs[storyID] {
		return
	}
	r.content.mu.Lock()
	_, done := r.grammarsMap[storyID]
	r.content.mu.Unlock()
	if done {
		return
	}
	byTitle := make(map[string]map[int][]Correction)
	if nc, err := r.content.get(storyID); err == nil {
		if fc := nc.Family(FamilyGrammars); fc != nil {
			for _, f := range fc.Files {
				addGrammarEntries(byTitle, deepCopy(f.Grammar))
			}
		}
	}
	ensureUniqueCorrectionIDs(storyID, byTitle)
	r.content.mu.Lock()
	r.grammarsMap[storyID] = byTitle
	r.content.mu.Unlock()
}

// MemoryContent is an in-memory ContentLoader over parsed notebooks.
type MemoryContent struct {
	notebooks []NotebookContent
}

// NewMemoryContent returns a loader serving the given notebooks.
func NewMemoryContent(notebooks []NotebookContent) *MemoryContent {
	return &MemoryContent{notebooks: notebooks}
}

// Catalog implements ContentLoader.
func (m *MemoryContent) Catalog(_ context.Context) ([]CatalogEntry, error) {
	var out []CatalogEntry
	for _, nc := range m.notebooks {
		out = append(out, catalogOf(nc)...)
	}
	return out, nil
}

// Load implements ContentLoader.
func (m *MemoryContent) Load(_ context.Context, ids []string) ([]NotebookContent, error) {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	var out []NotebookContent
	for _, nc := range m.notebooks {
		if want[nc.ID] {
			out = append(out, nc)
		}
	}
	return out, nil
}

// catalogOf returns the catalog rows describing nc.
func catalogOf(nc NotebookContent) []CatalogEntry {
	out := make([]CatalogEntry, 0, len(nc.Families))
	for _, fc := range nc.Families {
		e := CatalogEntry{NotebookID: nc.ID, Family: fc.Family, Kind: fc.Kind, Name: fc.Name}
		for _, f := range fc.Files {
			e.Paths = append(e.Paths, f.Path)
			if d := f.sessionDate(); d.After(e.LatestDate) {
				e.LatestDate = d
			}
		}
		out = append(out, e)
	}
	return out
}

// sessionDate is an etymology file's date: the first dated entry of a new-shape
// file, or a legacy file's date (zero for other shapes).
func (f ContentFile) sessionDate() time.Time {
	for _, entry := range f.Etymology {
		if !entry.Date.IsZero() {
			return entry.Date
		}
	}
	if f.EtymologyLegacy != nil {
		return f.EtymologyLegacy.Date
	}
	return time.Time{}
}

// MapDictionary is a DictionarySource over an in-memory dictionary.
type MapDictionary map[string]rapidapi.Response

// Responses implements DictionarySource.
func (m MapDictionary) Responses(_ context.Context, words []string) (map[string]rapidapi.Response, error) {
	out := make(map[string]rapidapi.Response)
	for _, w := range words {
		if resp, ok := m[w]; ok {
			out[w] = resp
		}
	}
	return out, nil
}
