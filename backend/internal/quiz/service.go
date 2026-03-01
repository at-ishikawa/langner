package quiz

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// Service owns all quiz business logic shared between the CLI and RPC handler.
type Service struct {
	notebooksConfig config.NotebooksConfig
	openaiClient    inference.Client
	dictionaryMap   map[string]rapidapi.Response
}

// NewService creates a new Service.
func NewService(notebooksConfig config.NotebooksConfig, openaiClient inference.Client, dictionaryMap map[string]rapidapi.Response) *Service {
	return &Service{
		notebooksConfig: notebooksConfig,
		openaiClient:    openaiClient,
		dictionaryMap:   dictionaryMap,
	}
}

func (s *Service) newReader() (*notebook.Reader, error) {
	return notebook.NewReader(
		s.notebooksConfig.StoriesDirectories,
		s.notebooksConfig.FlashcardsDirectories,
		s.notebooksConfig.BooksDirectories,
		s.notebooksConfig.DefinitionsDirectories,
		s.dictionaryMap,
	)
}

// LoadNotebookSummaries returns all available notebooks with their review counts.
func (s *Service) LoadNotebookSummaries() ([]NotebookSummary, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}

	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	var summaries []NotebookSummary

	for id, index := range reader.GetStoryIndexes() {
		stories, err := reader.ReadStoryNotebooks(id)
		if err != nil {
			return nil, fmt.Errorf("failed to read story notebook %q: %w", id, err)
		}

		filtered, err := notebook.FilterStoryNotebooks(
			stories, learningHistories[id], s.dictionaryMap,
			false, true, true, false,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to filter story notebook %q: %w", id, err)
		}

		summaries = append(summaries, NotebookSummary{
			NotebookID:  id,
			Name:        index.Name,
			ReviewCount: countStoryDefinitions(filtered),
		})
	}

	for id, index := range reader.GetFlashcardIndexes() {
		notebooks, err := reader.ReadFlashcardNotebooks(id)
		if err != nil {
			return nil, fmt.Errorf("failed to read flashcard notebook %q: %w", id, err)
		}

		filtered, err := notebook.FilterFlashcardNotebooks(
			notebooks, learningHistories[id], s.dictionaryMap, false,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to filter flashcard notebook %q: %w", id, err)
		}

		summaries = append(summaries, NotebookSummary{
			NotebookID:  id,
			Name:        index.Name,
			ReviewCount: countFlashcardCards(filtered),
		})
	}

	return summaries, nil
}

// LoadCards returns filtered quiz cards for the given notebooks.
// Returns *NotFoundError if any notebook ID does not exist.
func (s *Service) LoadCards(notebookIDs []string, includeUnstudied bool) ([]Card, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}

	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	storyIndexes := reader.GetStoryIndexes()
	flashcardIndexes := reader.GetFlashcardIndexes()

	var cards []Card

	for _, notebookID := range notebookIDs {
		_, isStory := storyIndexes[notebookID]
		_, isFlashcard := flashcardIndexes[notebookID]

		if !isStory && !isFlashcard {
			return nil, &NotFoundError{NotebookID: notebookID}
		}

		if isStory {
			storyCards, err := s.loadStoryCards(reader, notebookID, learningHistories, includeUnstudied)
			if err != nil {
				return nil, fmt.Errorf("failed to load story cards for notebook %q: %w", notebookID, err)
			}
			cards = append(cards, storyCards...)
		}

		if isFlashcard {
			flashCards, err := s.loadFlashcardCards(reader, notebookID, learningHistories, includeUnstudied)
			if err != nil {
				return nil, fmt.Errorf("failed to load flashcard cards for notebook %q: %w", notebookID, err)
			}
			cards = append(cards, flashCards...)
		}
	}

	return cards, nil
}

func (s *Service) loadStoryCards(
	reader *notebook.Reader,
	notebookID string,
	learningHistories map[string][]notebook.LearningHistory,
	includeUnstudied bool,
) ([]Card, error) {
	stories, err := reader.ReadStoryNotebooks(notebookID)
	if err != nil {
		return nil, fmt.Errorf("failed to read story notebook %q: %w", notebookID, err)
	}

	filtered, err := notebook.FilterStoryNotebooks(
		stories, learningHistories[notebookID], s.dictionaryMap,
		false, includeUnstudied, true, false,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to filter story notebook %q: %w", notebookID, err)
	}

	var cards []Card
	for _, story := range filtered {
		for _, scene := range story.Scenes {
			for _, definition := range scene.Definitions {
				entry := definition.Definition
				originalEntry := ""
				if entry == "" {
					entry = definition.Expression
				} else {
					originalEntry = definition.Expression
				}

				examples, contexts := buildFromConversations(&scene, &definition)

				cards = append(cards, Card{
					NotebookName:  notebookID,
					StoryTitle:    story.Event,
					SceneTitle:    scene.Title,
					Entry:         entry,
					OriginalEntry: originalEntry,
					Meaning:       definition.Meaning,
					Examples:      examples,
					Contexts:      contexts,
				})
			}
		}
	}

	return cards, nil
}

func (s *Service) loadFlashcardCards(
	reader *notebook.Reader,
	notebookID string,
	learningHistories map[string][]notebook.LearningHistory,
	includeUnstudied bool,
) ([]Card, error) {
	notebooks, err := reader.ReadFlashcardNotebooks(notebookID)
	if err != nil {
		return nil, fmt.Errorf("failed to read flashcard notebook %q: %w", notebookID, err)
	}

	filtered, err := notebook.FilterFlashcardNotebooks(
		notebooks, learningHistories[notebookID], s.dictionaryMap, includeUnstudied,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to filter flashcard notebook %q: %w", notebookID, err)
	}

	var cards []Card
	for _, nb := range filtered {
		for _, card := range nb.Cards {
			entry := card.Definition
			originalEntry := ""
			if entry == "" {
				entry = card.Expression
			} else {
				originalEntry = card.Expression
			}

			var examples []Example
			for _, ex := range card.Examples {
				examples = append(examples, Example{Text: ex})
			}

			cards = append(cards, Card{
				NotebookName:  notebookID,
				StoryTitle:    "flashcards",
				Entry:         entry,
				OriginalEntry: originalEntry,
				Meaning:       card.Meaning,
				Examples:      examples,
			})
		}
	}

	return cards, nil
}

// GradeNotebookAnswer grades a meaning answer and returns the result.
func (s *Service) GradeNotebookAnswer(ctx context.Context, card Card, answer string, responseTimeMs int64) (GradeResult, error) {
	results, err := s.openaiClient.AnswerMeanings(ctx, inference.AnswerMeaningsRequest{
		Expressions: []inference.Expression{
			{
				Expression:        card.Entry,
				Meaning:           answer,
				Contexts:          card.Contexts,
				IsExpressionInput: false,
				ResponseTimeMs:    responseTimeMs,
			},
		},
	})
	if err != nil {
		return GradeResult{}, fmt.Errorf("failed to grade answer: %w", err)
	}
	if len(results.Answers) == 0 {
		return GradeResult{}, fmt.Errorf("no results returned from inference")
	}

	result := results.Answers[0]
	isCorrect, reason, quality := extractAnswerResult(result)

	return GradeResult{
		Correct: isCorrect,
		Reason:  reason,
		Quality: quality,
	}, nil
}

// SaveResult updates the learning history file for the answered card.
func (s *Service) SaveResult(card Card, result GradeResult, responseTimeMs int64) error {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return fmt.Errorf("failed to load learning histories: %w", err)
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[card.NotebookName])
	updater.UpdateOrCreateExpressionWithQuality(
		card.NotebookName,
		card.StoryTitle,
		card.SceneTitle,
		card.Entry,
		card.OriginalEntry,
		result.Correct,
		true,
		result.Quality,
		responseTimeMs,
		notebook.QuizTypeNotebook,
	)

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, card.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return fmt.Errorf("failed to save learning history for %q: %w", card.NotebookName, err)
	}

	return nil
}

func countStoryDefinitions(stories []notebook.StoryNotebook) int {
	var count int
	for _, story := range stories {
		for _, scene := range story.Scenes {
			count += len(scene.Definitions)
		}
	}
	return count
}

func countFlashcardCards(notebooks []notebook.FlashcardNotebook) int {
	var count int
	for _, nb := range notebooks {
		count += len(nb.Cards)
	}
	return count
}

func buildFromConversations(scene *notebook.StoryScene, definition *notebook.Note) ([]Example, []inference.Context) {
	var examples []Example
	var contexts []inference.Context
	for _, conv := range scene.Conversations {
		if conv.Quote == "" {
			continue
		}

		quoteLower := strings.ToLower(conv.Quote)
		if !containsExpression(quoteLower, definition.Expression, definition.Definition) {
			continue
		}

		cleaned := notebook.ConvertMarkersInText(conv.Quote, nil, notebook.ConversionStylePlain, "")
		examples = append(examples, Example{
			Text:    cleaned,
			Speaker: conv.Speaker,
		})
		contexts = append(contexts, inference.Context{
			Context:             cleaned,
			ReferenceDefinition: definition.Meaning,
		})
	}
	return examples, contexts
}

func containsExpression(textLower, expression, definition string) bool {
	if strings.Contains(textLower, strings.ToLower(expression)) {
		return true
	}
	if definition != "" && strings.Contains(textLower, strings.ToLower(definition)) {
		return true
	}
	return false
}

func extractAnswerResult(result inference.AnswerMeaning) (isCorrect bool, reason string, quality int) {
	if len(result.AnswersForContext) == 0 {
		return false, "", 1
	}

	first := result.AnswersForContext[0]
	isCorrect = first.Correct
	reason = first.Reason
	quality = first.Quality

	if quality == 0 {
		if isCorrect {
			quality = 4
		} else {
			quality = 1
		}
	}

	return isCorrect, reason, quality
}
