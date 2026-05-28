package assets

import (
	_ "embed"
	"fmt"
	"io"
)

//go:embed templates/etymology-notebook.md.go.tmpl
var fallbackEtymologyNotebookTemplate string

// EtymologyTemplate is the top-level data structure for etymology notebook templates
type EtymologyTemplate struct {
	Name     string
	Chapters []EtymologyChapter
}

// EtymologyChapter represents a chapter/session with origins and words
type EtymologyChapter struct {
	Title    string
	Origins  []EtymologyOriginEntry
	Words    []EtymologyWordEntry
	Sections []EtymologySection

	// Concepts lists the etymology-side concepts: declarations that
	// apply to this chapter, with members + relations resolved so the
	// template can render a Concepts section grouping origins like
	// sinister/gauche under "leftness" and showing antonym pairs to
	// related concepts. Empty when the session declares no concepts.
	Concepts []EtymologyConcept
}

// EtymologyConcept is one concept declaration for the chapter, with its
// member origins and any relations to other concepts.
type EtymologyConcept struct {
	Key       string                   // e.g. "leftness"
	Meaning   string                   // umbrella meaning shared by members
	Note      string                   // optional commentary (may be empty)
	Members   []EtymologyConceptMember // origins in declaration order
	Relations []EtymologyConceptRelation
}

// EtymologyConceptMember is one origin belonging to a concept.
type EtymologyConceptMember struct {
	Origin   string
	Language string
	Meaning  string // the origin's gloss, for the concept table's Meaning column
}

// EtymologyConceptRelation captures a relation to another concept (e.g.
// "antonym: rightness"). Type matches the YAML's relation `type` field;
// Other is the related concept's key.
type EtymologyConceptRelation struct {
	Type  string
	Other string
}

// EtymologySection represents a named group of words within a chapter (e.g., an origin topic)
type EtymologySection struct {
	Title string
	Words []EtymologyWordEntry
}

// EtymologyOriginEntry represents a single etymology origin for template rendering
type EtymologyOriginEntry struct {
	Origin   string
	Language string
	Meaning  string
}

// EtymologyWordEntry represents a word with its etymology for template rendering
type EtymologyWordEntry struct {
	Expression    string
	Definition    string
	Meaning       string
	Pronunciation string
	PartOfSpeech  string
	Note          string
	OriginParts   []EtymologyOriginRef

	// Concept context, populated by the etymology writer when this entry
	// represents a multi-member definitions concept after the group-by-
	// concept_key pass. When ConceptHead is non-empty, templates may
	// render an umbrella row + nested per-member rows (each member's
	// part-of-speech and meaning) instead of one row per word.
	ConceptHead    string
	ConceptMembers []ConceptMember
	ConceptMeaning string
}

// EtymologyOriginRef references an origin with its resolved meaning
type EtymologyOriginRef struct {
	Origin  string
	Meaning string
}

// WriteEtymologyNotebook renders an etymology notebook template to the given writer.
func WriteEtymologyNotebook(output io.Writer, templatePath string, templateData EtymologyTemplate) error {
	tmpl, err := parseTemplateWithFallback(templatePath, fallbackEtymologyNotebookTemplate)
	if err != nil {
		return fmt.Errorf("parseTemplateWithFallback() > %w", err)
	}
	if err := tmpl.Execute(output, templateData); err != nil {
		return fmt.Errorf("tmpl.Execute() > %w", err)
	}
	return nil
}
