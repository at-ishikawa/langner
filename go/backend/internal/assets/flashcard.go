package assets

import (
	_ "embed"
	"fmt"
	"io"
	"time"
)

//go:embed templates/flashcard-notebook.md.go.tmpl
var fallbackFlashcardNotebookTemplate string

// FlashcardTemplate is the top-level data structure for flashcard notebook templates
type FlashcardTemplate struct {
	Notebooks []FlashcardNotebook
}

// FlashcardNotebook represents a single flashcard notebook with its cards
type FlashcardNotebook struct {
	Title       string
	Description string
	Date        time.Time
	Cards       []FlashcardCard
}

// FlashcardCard represents a vocabulary card for template rendering
type FlashcardCard struct {
	Expression    string
	Definition    string
	Meaning       string
	Examples      []string
	Pronunciation string
	PartOfSpeech  string
	Origin        string
	Synonyms      []string
	Antonyms      []string
	Images        []string
}

func WriteFlashcardNotebook(output io.Writer, templatePath string, templateData FlashcardTemplate) error {
	tmpl, err := parseTemplateWithFallback(templatePath, fallbackFlashcardNotebookTemplate)
	if err != nil {
		return fmt.Errorf("parseTemplateWithFallback() > %w", err)
	}
	if err := tmpl.Execute(output, templateData); err != nil {
		return fmt.Errorf("tmpl.Execute() > %w", err)
	}
	return nil
}
