package cli

import (
	"strings"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// WordOccurrenceContext represents a context sentence with the actual word form used
type WordOccurrenceContext struct {
	Context string // The full conversation quote
	Usage   string // The actual form of the word as it appears in the context
}

// WordOccurrence represents a single word/phrase occurrence in a story
// Used by both FreeformQuizCLI (across multiple notebooks) and NotebookQuizCLI (single notebook)
type WordOccurrence struct {
	NotebookName string                  // Which notebook this word is from (empty for NotebookQuiz)
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

// extractContextsFromConversations finds conversations containing the expression or definition
// and returns them with the actual word form used in each context
func extractContextsFromConversations(scene *notebook.StoryScene, expression, definition string) []WordOccurrenceContext {
	var contexts []WordOccurrenceContext

	for _, conversation := range scene.Conversations {
		if conversation.Quote == "" {
			continue
		}

		quoteLower := strings.ToLower(conversation.Quote)

		// Check if the conversation contains the expression
		if strings.Contains(quoteLower, strings.ToLower(expression)) {
			contexts = append(contexts, WordOccurrenceContext{
				Context: conversation.Quote,
				Usage:   expression, // The expression form is what's used in the context
			})
			continue
		}

		// Also check for the definition if it exists
		if definition != "" && strings.Contains(quoteLower, strings.ToLower(definition)) {
			contexts = append(contexts, WordOccurrenceContext{
				Context: conversation.Quote,
				Usage:   definition, // The definition form is what's used in the context
			})
		}
	}

	return contexts
}
