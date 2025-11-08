package cli

import (
	"strings"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// WordOccurrence represents a single word/phrase occurrence in a story
// Used by both FreeformQuizCLI (across multiple notebooks) and NotebookQuizCLI (single notebook)
type WordOccurrence struct {
	NotebookName string                  // Which notebook this word is from (empty for NotebookQuiz)
	Story        *notebook.StoryNotebook // Which story contains this word
	Scene        *notebook.StoryScene    // Which scene contains this word
	Definition   *notebook.Note          // The complete definition/note
	Contexts     []string                // Example sentences/conversations containing the word
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

// extractContextsFromConversations finds conversations containing the expression or definition
func extractContextsFromConversations(scene *notebook.StoryScene, expression, definition string) []string {
	var contexts []string

	for _, conversation := range scene.Conversations {
		if conversation.Quote == "" {
			continue
		}

		quoteLower := strings.ToLower(conversation.Quote)

		// Check if the conversation contains the expression
		if strings.Contains(quoteLower, strings.ToLower(expression)) {
			contexts = append(contexts, conversation.Quote)
			continue
		}

		// Also check for the definition if it exists
		if definition != "" && strings.Contains(quoteLower, strings.ToLower(definition)) {
			contexts = append(contexts, conversation.Quote)
		}
	}

	return contexts
}
