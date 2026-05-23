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
	// member in YAML declaration order with per-member display data
	// (part of speech + per-member meaning), and ConceptMeaning is the
	// shared umbrella meaning. Templates may render a per-member block
	// when ConceptHead is non-empty; standalone notes leave it empty.
	ConceptHead    string
	ConceptMembers []ConceptMember
	ConceptMeaning string
}

// ConceptMember is one entry in a StoryNote's concept member list. Carries
// the per-member display fields the markdown / PDF template needs to render
// each member's part of speech and specific meaning alongside the family
// umbrella. Members are ordered in YAML declaration order.
type ConceptMember struct {
	Name         string
	PartOfSpeech string
	Meaning      string
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
