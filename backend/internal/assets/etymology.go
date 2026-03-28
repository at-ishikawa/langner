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
	Title   string
	Origins []EtymologyOriginEntry
	Words   []EtymologyWordEntry
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
