package notebook

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
)

// FlashcardNotebook represents a collection of vocabulary flashcards.
// It is a simpler format than StoryNotebook - it contains just a flat list
// of vocabulary cards without scenes or conversations.
type FlashcardNotebook struct {
	Title       string    `yaml:"title"`
	Description string    `yaml:"description,omitempty"`
	Date        time.Time `yaml:"date"`
	Cards       []Note    `yaml:"cards"`
}

// FlashcardIndex represents an index file for flashcard directories.
// It defines a collection of flashcard notebooks that can be loaded together.
type FlashcardIndex struct {
	ID            string   `yaml:"id"`
	Name          string   `yaml:"name"`
	NotebookPaths []string `yaml:"notebooks"`

	// internal fields (not loaded from YAML)
	path      string                `yaml:"-"` // directory containing this index
	Notebooks []FlashcardNotebook   `yaml:"-"` // loaded notebooks (populated by reader)
}

// Validate validates a FlashcardNotebook and returns any validation errors.
// It checks that:
// - Title is not empty
// - Each card has an expression
// - Each card has either a meaning or images (like Note validation)
func (notebook *FlashcardNotebook) Validate(location string) []ValidationError {
	var errors []ValidationError

	// Check title is not empty
	if strings.TrimSpace(notebook.Title) == "" {
		errors = append(errors, ValidationError{
			Location: location,
			Message:  "title is empty",
			Suggestions: []string{
				"add a title to the flashcard notebook",
			},
		})
	}

	// Validate each card
	for cardIdx, card := range notebook.Cards {
		cardLocation := fmt.Sprintf("%s -> card[%d]: %s", location, cardIdx, card.Expression)

		// Check expression is not empty
		if strings.TrimSpace(card.Expression) == "" {
			errors = append(errors, ValidationError{
				Location: cardLocation,
				Message:  "expression is empty",
				Suggestions: []string{
					"add an expression to the card",
				},
			})
			continue
		}

		// Check card has either meaning or images
		if card.Meaning == "" && len(card.Images) == 0 {
			errors = append(errors, ValidationError{
				Location: cardLocation,
				Message:  fmt.Sprintf("card %q has neither meaning nor images", card.Expression),
				Suggestions: []string{
					"add a meaning field to the card",
					"or add images to the card",
				},
			})
		}
	}

	return errors
}

// FilterFlashcardNotebooks filters flashcard notebooks based on learning history
// and spaced repetition algorithm, similar to FilterStoryNotebooks.
// It returns only the cards that need to be learned based on the learning history.
func FilterFlashcardNotebooks(
	flashcardNotebooks []FlashcardNotebook,
	learningHistory []LearningHistory,
	dictionaryMap map[string]rapidapi.Response,
	sortDesc bool,
) ([]FlashcardNotebook, error) {
	result := make([]FlashcardNotebook, 0)

	for _, notebook := range flashcardNotebooks {
		if len(notebook.Cards) == 0 {
			continue
		}

		cards := make([]Note, 0)
		for _, card := range notebook.Cards {
			// Populate learning logs from history
			for _, h := range learningHistory {
				logs := h.GetLogs(
					notebook.Title,
					"", // flashcards don't have scenes
					card,
				)
				if len(logs) == 0 {
					continue
				}

				card.LearnedLogs = logs
			}

			if strings.TrimSpace(card.Expression) == "" {
				return nil, fmt.Errorf("empty card.Expression: %v in flashcard notebook %s", card, notebook.Title)
			}

			// Check if card needs to be learned based on spaced repetition
			if !card.needsToLearn() {
				continue
			}

			// Set details from dictionary
			if err := card.setDetails(dictionaryMap, ""); err != nil {
				return nil, fmt.Errorf("card.setDetails() > %w", err)
			}

			cards = append(cards, card)
		}

		if len(cards) == 0 {
			continue
		}

		notebook.Cards = cards
		result = append(result, notebook)
	}

	// Sort by date
	if sortDesc {
		sort.Slice(result, func(i, j int) bool {
			return result[i].Date.After(result[j].Date)
		})
	} else {
		sort.Slice(result, func(i, j int) bool {
			return result[i].Date.Before(result[j].Date)
		})
	}

	return result, nil
}
