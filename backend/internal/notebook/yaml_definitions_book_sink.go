package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// YAMLDefinitionsBookSink writes concepts: blocks to the per-book
// definitions YAML output under exportDir/definitions/<bookID>.yml. One
// Definitions entry is emitted per session_title — the importer accepts
// either metadata.notebook or metadata.title as the session identifier,
// and we write `title:` so the round-trip is symmetric with the source
// shape produced when sessions are declared by title.
type YAMLDefinitionsBookSink struct {
	outputDir string
}

// NewYAMLDefinitionsBookSink constructs the sink.
func NewYAMLDefinitionsBookSink(outputDir string) *YAMLDefinitionsBookSink {
	return &YAMLDefinitionsBookSink{outputDir: outputDir}
}

// WriteConcepts serialises each book's concepts to
// <outputDir>/definitions/<bookID>.yml. Books with no concepts are
// skipped. Within a file, sessions sort by title and concepts within a
// session sort by head for deterministic output.
func (s *YAMLDefinitionsBookSink) WriteConcepts(perBook map[string]map[string][]DefinitionConcept) error {
	if s.outputDir == "" {
		return fmt.Errorf("definitions sink output directory not configured")
	}
	if len(perBook) == 0 {
		return nil
	}

	dir := filepath.Join(s.outputDir, "definitions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create definitions directory %s: %w", dir, err)
	}

	bookIDs := make([]string, 0, len(perBook))
	for id := range perBook {
		bookIDs = append(bookIDs, id)
	}
	sort.Strings(bookIDs)

	for _, bookID := range bookIDs {
		bySession := perBook[bookID]
		if len(bySession) == 0 {
			continue
		}

		sessionTitles := make([]string, 0, len(bySession))
		for t := range bySession {
			sessionTitles = append(sessionTitles, t)
		}
		sort.Strings(sessionTitles)

		var defs []Definitions
		for _, title := range sessionTitles {
			concepts := bySession[title]
			if len(concepts) == 0 {
				continue
			}
			sort.Slice(concepts, func(i, j int) bool {
				return concepts[i].Head < concepts[j].Head
			})
			// Strip the SessionTitle field (yaml:"-") on output by
			// reconstructing; the importer reattaches it from the
			// parent Definitions.Metadata at read time.
			out := make([]DefinitionConcept, len(concepts))
			for i, c := range concepts {
				out[i] = DefinitionConcept{
					Head:        c.Head,
					Meaning:     c.Meaning,
					Expressions: c.Expressions,
				}
			}
			defs = append(defs, Definitions{
				Metadata: DefinitionsMetadata{Title: title},
				Concepts: out,
			})
		}
		if len(defs) == 0 {
			continue
		}
		filePath := filepath.Join(dir, bookID+".yml")
		if err := WriteYamlFile(filePath, defs); err != nil {
			return fmt.Errorf("write definitions file %s: %w", filePath, err)
		}
	}
	return nil
}
