package notebook

import (
	"bytes"
	"fmt"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Notebook content families: the folder a notebook's index.yml lives under.
// One notebook id can be registered under several families (a book plus its
// definitions plus their etymology).
const (
	FamilyStories     = "stories"
	FamilyBooks       = "books"
	FamilyJournals    = "journals"
	FamilyFlashcards  = "flashcards"
	FamilyDefinitions = "definitions"
	FamilyEtymology   = "etymology"
	FamilyGrammars    = "grammars"
)

// Families lists every content family.
var Families = []string{
	FamilyStories, FamilyBooks, FamilyJournals, FamilyFlashcards,
	FamilyDefinitions, FamilyEtymology, FamilyGrammars,
}

// IsFamily reports whether name is a content family.
func IsFamily(name string) bool {
	for _, f := range Families {
		if f == name {
			return true
		}
	}
	return false
}

// File shapes: how one content file is structured.
const (
	ShapeStory           = "story"
	ShapeFlashcard       = "flashcard"
	ShapeDefinitions     = "definitions"
	ShapeEtymology       = "etymology"
	ShapeEtymologyLegacy = "etymology_legacy"
	ShapeGrammar         = "grammar"
)

// NotebookContent is the parsed content of one notebook: every family it is
// registered under. It is what a push stores as rows and what a Reader reads
// back, so the server never parses YAML on the read path.
type NotebookContent struct {
	ID       string
	Families []FamilyContent
}

// FamilyContent is one family of a notebook: its index.yml metadata and the
// files the index lists, in index order.
type FamilyContent struct {
	Family string
	Kind   string
	Name   string
	Files  []ContentFile
}

// ContentFile is one parsed file. Exactly one of the value fields is set,
// according to Shape. A story file that does not parse is kept with no
// entries, matching the reader, which skips it.
type ContentFile struct {
	Path            string
	Shape           string
	Stories         []StoryNotebook
	Flashcards      []FlashcardNotebook
	Definitions     []Definitions
	Etymology       []EtymologyNotebookEntry
	EtymologyLegacy *etymologySessionFile
	Grammar         []GrammarEntry
}

// value returns the parsed file as the type the reader reads it as.
func (f ContentFile) value() any {
	switch f.Shape {
	case ShapeStory:
		return f.Stories
	case ShapeFlashcard:
		return f.Flashcards
	case ShapeDefinitions:
		return f.Definitions
	case ShapeEtymology:
		return f.Etymology
	case ShapeEtymologyLegacy:
		if f.EtymologyLegacy == nil {
			return etymologySessionFile{}
		}
		return *f.EtymologyLegacy
	case ShapeGrammar:
		return f.Grammar
	}
	return nil
}

// Family returns the content of one family, or nil.
func (c NotebookContent) Family(family string) *FamilyContent {
	for i := range c.Families {
		if c.Families[i].Family == family {
			return &c.Families[i]
		}
	}
	return nil
}

// contentIndex is the part of an index.yml every family shares.
type contentIndex struct {
	ID        string   `yaml:"id"`
	Kind      string   `yaml:"kind"`
	Name      string   `yaml:"name"`
	Notebooks []string `yaml:"notebooks"`
}

func decodeYAML[T any](data []byte) (T, error) {
	var result T
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&result); err != nil {
		return result, fmt.Errorf("yaml decode: %w", err)
	}
	return result, nil
}

// hasOriginsKey reports whether a file declares etymology origins (a top-level
// `origins:` line), which makes an index without a kind an etymology notebook.
func hasOriginsKey(data []byte) bool {
	for _, line := range bytes.Split(data, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if bytes.Equal(trimmed, []byte("origins:")) || bytes.HasPrefix(trimmed, []byte("origins: ")) {
			return true
		}
	}
	return false
}

// ParseBundle parses a notebook bundle (each family's index.yml and the files
// it lists) into NotebookContent. Files are keyed by family and their path
// relative to the family's index.yml. It applies the same rules as the
// directory reader: an index in stories/ or flashcards/ that is kind Etymology
// (or has no kind but files declaring origins) is an etymology notebook; an
// index in etymology/ that is neither is ignored.
func ParseBundle(id string, files []NotebookFile) (NotebookContent, error) {
	byFamily := make(map[string]map[string][]byte)
	for _, f := range files {
		if byFamily[f.Family] == nil {
			byFamily[f.Family] = make(map[string][]byte)
		}
		byFamily[f.Family][path.Clean(f.Path)] = f.Content
	}

	content := NotebookContent{ID: id}
	families := make([]string, 0, len(byFamily))
	for family := range byFamily {
		families = append(families, family)
	}
	sort.Strings(families)
	for _, family := range families {
		if !IsFamily(family) {
			return NotebookContent{}, fmt.Errorf("unknown notebook family %q", family)
		}
		fc, ok, err := parseFamily(family, byFamily[family])
		if err != nil {
			return NotebookContent{}, fmt.Errorf("%s/%s: %w", family, id, err)
		}
		if !ok {
			continue
		}
		if content.Family(fc.Family) != nil {
			return NotebookContent{}, fmt.Errorf("%s: notebook %s has two %s indexes", family, id, fc.Family)
		}
		content.Families = append(content.Families, fc)
	}
	return content, nil
}

func parseFamily(family string, files map[string][]byte) (FamilyContent, bool, error) {
	indexData, ok := files["index.yml"]
	if !ok {
		return FamilyContent{}, false, fmt.Errorf("index.yml is missing")
	}
	index, err := decodeYAML[contentIndex](indexData)
	if err != nil {
		return FamilyContent{}, false, fmt.Errorf("index.yml: %w", err)
	}
	fileData := func(p string) ([]byte, error) {
		data, ok := files[path.Clean(p)]
		if !ok {
			return nil, fmt.Errorf("%s is listed in index.yml but missing", p)
		}
		return data, nil
	}

	if family == FamilyStories || family == FamilyFlashcards || family == FamilyEtymology {
		isEtymology := index.Kind == "Etymology"
		if !isEtymology && index.Kind == "" {
			for _, p := range index.Notebooks {
				if data, ok := files[path.Clean(p)]; ok && hasOriginsKey(data) {
					isEtymology = true
					break
				}
			}
		}
		if isEtymology {
			family = FamilyEtymology
		} else if family == FamilyEtymology {
			return FamilyContent{}, false, nil
		}
	}

	fc := FamilyContent{Family: family, Kind: index.Kind, Name: index.Name}
	for _, p := range index.Notebooks {
		data, err := fileData(p)
		if err != nil {
			return FamilyContent{}, false, err
		}
		f, err := parseContentFile(family, p, data)
		if err != nil {
			return FamilyContent{}, false, fmt.Errorf("%s: %w", p, err)
		}
		fc.Files = append(fc.Files, f)
	}
	return fc, true, nil
}

func parseContentFile(family, p string, data []byte) (ContentFile, error) {
	f := ContentFile{Path: p}
	switch family {
	case FamilyStories, FamilyBooks, FamilyJournals:
		f.Shape = ShapeStory
		if stories, err := decodeYAML[[]StoryNotebook](data); err == nil {
			f.Stories = stories
		} else if legacy, err := decodeYAML[etymologySessionFile](data); err == nil {
			// A definitions-only session file listed by a story index: the
			// story reader skips it, the etymology definitions scan reads it.
			f.Shape = ShapeEtymologyLegacy
			f.EtymologyLegacy = &legacy
		}
	case FamilyFlashcards:
		decks, err := decodeYAML[[]FlashcardNotebook](data)
		if err != nil {
			return f, err
		}
		f.Shape, f.Flashcards = ShapeFlashcard, decks
	case FamilyDefinitions:
		defs, err := decodeYAML[[]Definitions](data)
		if err != nil {
			return f, err
		}
		f.Shape, f.Definitions = ShapeDefinitions, defs
	case FamilyEtymology:
		if entries, err := decodeYAML[[]EtymologyNotebookEntry](data); err == nil && hasNewShape(entries) {
			for _, e := range entries {
				if strings.TrimSpace(e.Event) == "" {
					return f, fmt.Errorf("etymology session: event title is empty")
				}
			}
			f.Shape, f.Etymology = ShapeEtymology, entries
			break
		}
		legacy, err := decodeYAML[etymologySessionFile](data)
		if err != nil {
			return f, err
		}
		if strings.TrimSpace(legacy.Metadata.Title) == "" {
			return f, fmt.Errorf("etymology session is missing metadata.title")
		}
		f.Shape, f.EtymologyLegacy = ShapeEtymologyLegacy, &legacy
	case FamilyGrammars:
		entries, err := decodeYAML[[]GrammarEntry](data)
		if err != nil {
			return f, err
		}
		f.Shape, f.Grammar = ShapeGrammar, entries
	default:
		return f, fmt.Errorf("unknown family %q", family)
	}
	return f, nil
}

// Bundle renders the content back into YAML files (each family's index.yml
// plus its files), the inverse of ParseBundle. Used by pull.
func (c NotebookContent) Bundle() ([]NotebookFile, error) {
	var files []NotebookFile
	for _, fc := range c.Families {
		index := contentIndex{ID: c.ID, Kind: fc.Kind, Name: fc.Name}
		for _, f := range fc.Files {
			index.Notebooks = append(index.Notebooks, f.Path)
		}
		data, err := yaml.Marshal(index)
		if err != nil {
			return nil, fmt.Errorf("marshal %s index: %w", fc.Family, err)
		}
		files = append(files, NotebookFile{NotebookID: c.ID, Family: fc.Family, Path: "index.yml", Content: data})
		for _, f := range fc.Files {
			data, err := yaml.Marshal(f.value())
			if err != nil {
				return nil, fmt.Errorf("marshal %s/%s: %w", fc.Family, f.Path, err)
			}
			files = append(files, NotebookFile{NotebookID: c.ID, Family: fc.Family, Path: path.Clean(f.Path), Content: data})
		}
	}
	return files, nil
}
