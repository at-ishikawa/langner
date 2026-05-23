package assets

import (
	_ "embed"
	"fmt"
	"io"
	"time"
)

//go:embed templates/story-notebook.md.go.tmpl
var fallbackStoryNotebookTemplate string

// StoryTemplate is the top-level data structure for story notebook templates
type StoryTemplate struct {
	Notebooks []StoryNotebook
}

// StoryNotebook represents a single story notebook with its event, metadata, and scenes
type StoryNotebook struct {
	Event    string
	Metadata Metadata
	Date     time.Time
	Scenes   []StoryScene
}

// Metadata contains optional metadata about the story (series, season, episode)
type Metadata struct {
	Series  string
	Season  int
	Episode int
}

// StoryScene represents a single scene with conversations and definitions
type StoryScene struct {
	Title         string
	Conversations []Conversation
	Statements    []string
	Type          string
	Definitions   []StoryNote
}

// Conversation represents a single dialog between characters
type Conversation struct {
	Speaker string
	Quote   string
}

// StoryNote represents a note/definition to be learned from the story
// This is a simplified version of notebook.Note for template rendering
type StoryNote struct {
	Definition    string
	Expression    string
	Meaning       string
	Examples      []string
	Pronunciation string
	PartOfSpeech  string
	Origin        string
	Note          string
	Synonyms      []string
	Antonyms      []string
	Images        []string

	// Concept context. When this note represents a multi-member
	// definitions concept (after the writer's group-by-concept_key pass),
	// ConceptHead is the head expression, ConceptMembers lists every
	// member in YAML declaration order, and ConceptMeaning is the
	// umbrella meaning. Templates may render a "Family: …" line when
	// ConceptHead is non-empty; standalone notes leave it empty.
	ConceptHead    string
	ConceptMembers []string
	ConceptMeaning string
}

func WriteStoryNotebook(output io.Writer, templatePath string, templateData StoryTemplate) error {
	tmpl, err := parseTemplateWithFallback(templatePath, fallbackStoryNotebookTemplate)
	if err != nil {
		return fmt.Errorf("parseTemplateWithFallback() > %w", err)
	}
	if err := tmpl.Execute(output, templateData); err != nil {
		return fmt.Errorf("tmpl.Execute() > %w", err)
	}
	return nil
}
