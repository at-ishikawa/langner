package cli

import (
	"github.com/at-ishikawa/langner/internal/notebook"
)

// WordOccurrenceContext represents a context sentence with the actual word form used
type WordOccurrenceContext struct {
	Context string // The full conversation quote
	Usage   string // The actual form of the word as it appears in the context
}

// WordOccurrence represents a single word/phrase occurrence in a story
// Used by both FreeformQuizCLI and NotebookQuizCLI (single or multiple notebooks)
type WordOccurrence struct {
	NotebookName string                  // Which notebook this word is from
	Story        *notebook.StoryNotebook // Which story contains this word
	Scene        *notebook.StoryScene    // Which scene contains this word
	Definition   *notebook.Note          // The complete definition/note
	Contexts     []WordOccurrenceContext // Example sentences/conversations with word forms
}

// GetExpression returns the word/phrase to display to the user
// If Definition field is set, return that (it's the base form)
// Otherwise return Expression (the full phrase as it appears)
func (w *WordOccurrence) GetExpression() string {
	if w.Definition.Definition != "" {
		return w.Definition.Definition
	}
	return w.Definition.Expression
}

// GetMeaning returns the meaning/definition of the word
func (w *WordOccurrence) GetMeaning() string {
	return w.Definition.Meaning
}

// GetImages returns the list of image URLs for this word
func (w *WordOccurrence) GetImages() []string {
	return w.Definition.Images
}

// GetCleanContexts returns contexts with {{ }} markers removed
// These cleaned contexts should be used when sending to inference APIs
func (w *WordOccurrence) GetCleanContexts() []string {
	cleaned := make([]string, len(w.Contexts))
	for i, ctx := range w.Contexts {
		// Use existing ConvertMarkersInText with ConversionStylePlain to strip markers
		// Pass empty definitions slice since we want to strip all markers regardless
		cleaned[i] = notebook.ConvertMarkersInText(ctx.Context, nil, notebook.ConversionStylePlain, "")
	}
	return cleaned
}
