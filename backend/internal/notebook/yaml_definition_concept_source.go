package notebook

import (
	"context"
	"sort"
	"strings"
)

// DefinitionConceptForImport is one concept declaration parsed from a
// definitions YAML file. Within a book, the first declaration of a Head
// sets meaning; later declarations contribute only members. SessionTitle
// records the session that produced this entry (concepts can be declared
// across multiple sessions of the same book).
type DefinitionConceptForImport struct {
	NotebookID   string
	SessionTitle string
	Head         string
	Meaning      string
	Members      []string
}

// YAMLDefinitionConceptSource enumerates every concepts: block in every
// definitions YAML file (across all definitions books). Iteration order
// follows book-ID sort + per-book session order so the first declaration
// for a (book, head) tuple is deterministic.
type YAMLDefinitionConceptSource struct {
	reader *Reader
}

// NewYAMLDefinitionConceptSource constructs the source.
func NewYAMLDefinitionConceptSource(reader *Reader) *YAMLDefinitionConceptSource {
	return &YAMLDefinitionConceptSource{reader: reader}
}

// FindAll walks every definitions book and emits one entry per concept
// block declaration with its session title attached. Empty heads / empty
// member lists are skipped so the ingestion side doesn't have to filter.
func (s *YAMLDefinitionConceptSource) FindAll(_ context.Context) ([]DefinitionConceptForImport, error) {
	if s.reader == nil {
		return nil, nil
	}
	bookIDs := s.reader.GetDefinitionsBookIDs()
	sort.Strings(bookIDs)

	var rows []DefinitionConceptForImport
	for _, bookID := range bookIDs {
		defs, ok := s.reader.GetDefinitionsBook(bookID)
		if !ok {
			continue
		}
		for _, def := range defs {
			sessionTitle := def.Metadata.Title
			if sessionTitle == "" {
				sessionTitle = def.Metadata.Notebook
			}
			for _, c := range def.Concepts {
				head := strings.TrimSpace(c.Head)
				if head == "" {
					continue
				}
				members := make([]string, 0, len(c.Expressions))
				for _, e := range c.Expressions {
					e = strings.TrimSpace(e)
					if e == "" {
						continue
					}
					members = append(members, e)
				}
				if len(members) == 0 {
					continue
				}
				rows = append(rows, DefinitionConceptForImport{
					NotebookID:   bookID,
					SessionTitle: sessionTitle,
					Head:         head,
					Meaning:      c.Meaning,
					Members:      members,
				})
			}
		}
	}
	return rows, nil
}
