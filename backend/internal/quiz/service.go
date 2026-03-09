package quiz

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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

		var latestDate time.Time
		for _, s := range stories {
			if s.Date.After(latestDate) {
				latestDate = s.Date
			}
		}
		summaries = append(summaries, NotebookSummary{
			NotebookID:      id,
			Name:            index.Name,
			ReviewCount:     countStoryDefinitions(filtered),
			LatestStoryDate: latestDate,
			Kind:            kindFromIndex(index),
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

	if !isCorrect && quality >= 3 {
		quality = 2
	}

	return isCorrect, reason, quality
}

// ReverseCard represents a reverse quiz card.
type ReverseCard struct {
	NotebookName string
	StoryTitle   string
	SceneTitle   string
	Meaning      string
	Contexts     []ReverseContext
	Expression   string // original expression to guess
}

// ReverseContext represents a context sentence with masking info.
type ReverseContext struct {
	Context       string
	MaskedContext string
}

// LoadReverseCards loads reverse quiz cards for the given notebooks.
func (s *Service) LoadReverseCards(notebookIDs []string, listMissingContext bool) ([]ReverseCard, error) {
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

	var cards []ReverseCard

	for _, notebookID := range notebookIDs {
		_, isStory := storyIndexes[notebookID]
		_, isFlashcard := flashcardIndexes[notebookID]

		if !isStory && !isFlashcard {
			return nil, &NotFoundError{NotebookID: notebookID}
		}

		if isStory {
			reverseCards, err := s.loadStoryReverseCards(reader, notebookID, learningHistories, listMissingContext)
			if err != nil {
				return nil, fmt.Errorf("failed to load story reverse cards for notebook %q: %w", notebookID, err)
			}
			cards = append(cards, reverseCards...)
		}

		if isFlashcard {
			reverseCards, err := s.loadFlashcardReverseCards(reader, notebookID, learningHistories, listMissingContext)
			if err != nil {
				return nil, fmt.Errorf("failed to load flashcard reverse cards for notebook %q: %w", notebookID, err)
			}
			cards = append(cards, reverseCards...)
		}
	}

	return cards, nil
}

func (s *Service) loadStoryReverseCards(
	reader *notebook.Reader,
	notebookID string,
	learningHistories map[string][]notebook.LearningHistory,
	listMissingContext bool,
) ([]ReverseCard, error) {
	stories, err := reader.ReadStoryNotebooks(notebookID)
	if err != nil {
		return nil, fmt.Errorf("failed to read story notebook %q: %w", notebookID, err)
	}

	filtered, err := notebook.FilterStoryNotebooks(
		stories, learningHistories[notebookID], s.dictionaryMap,
		false, true, true, false,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to filter story notebook %q: %w", notebookID, err)
	}

	var cards []ReverseCard
	for _, story := range filtered {
		for _, scene := range story.Scenes {
			for _, definition := range scene.Definitions {
				expression := definition.Expression
				if definition.Definition != "" {
					expression = definition.Definition
				}

				contexts := buildReverseContexts(&scene, &definition)

				if listMissingContext {
					if len(contexts) > 0 {
						continue
					}
				} else {
					needsReview := needsReverseReview(learningHistories[notebookID], story.Event, scene.Title, &definition)
					if !needsReview {
						continue
					}
				}

				cards = append(cards, ReverseCard{
					NotebookName: notebookID,
					StoryTitle:   story.Event,
					SceneTitle:   scene.Title,
					Meaning:      definition.Meaning,
					Contexts:     contexts,
					Expression:   expression,
				})
			}
		}
	}

	return cards, nil
}

func (s *Service) loadFlashcardReverseCards(
	reader *notebook.Reader,
	notebookID string,
	learningHistories map[string][]notebook.LearningHistory,
	listMissingContext bool,
) ([]ReverseCard, error) {
	notebooks, err := reader.ReadFlashcardNotebooks(notebookID)
	if err != nil {
		return nil, fmt.Errorf("failed to read flashcard notebook %q: %w", notebookID, err)
	}

	filtered, err := notebook.FilterFlashcardNotebooks(
		notebooks, learningHistories[notebookID], s.dictionaryMap, false,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to filter flashcard notebook %q: %w", notebookID, err)
	}

	var cards []ReverseCard
	for _, nb := range filtered {
		for _, card := range nb.Cards {
			expression := card.Expression
			if card.Definition != "" {
				expression = card.Definition
			}

			var contexts []ReverseContext
			for _, ex := range card.Examples {
				if strings.Contains(strings.ToLower(ex), strings.ToLower(card.Expression)) {
					masked := maskWord(ex, card.Expression, card.Definition)
					contexts = append(contexts, ReverseContext{
						Context:       ex,
						MaskedContext: masked,
					})
				}
			}

			if listMissingContext {
				if len(contexts) > 0 {
					continue
				}
			} else {
				needsReview := needsReverseFlashcardReview(learningHistories[notebookID], nb.Title, &card)
				if !needsReview {
					continue
				}
			}

			cards = append(cards, ReverseCard{
				NotebookName: notebookID,
				StoryTitle:   "flashcards",
				SceneTitle:   "",
				Meaning:      card.Meaning,
				Contexts:     contexts,
				Expression:   expression,
			})
		}
	}

	return cards, nil
}

func maskWord(context, expression, definition string) string {
	re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(expression) + `\b`)
	context = re.ReplaceAllString(context, "______")
	if definition != "" {
		re = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(definition) + `\b`)
		context = re.ReplaceAllString(context, "______")
	}
	return context
}

func buildReverseContexts(scene *notebook.StoryScene, definition *notebook.Note) []ReverseContext {
	var contexts []ReverseContext
	for _, conv := range scene.Conversations {
		if conv.Quote == "" {
			continue
		}

		quoteLower := strings.ToLower(conv.Quote)
		if !containsExpression(quoteLower, definition.Expression, definition.Definition) {
			continue
		}

		cleaned := notebook.ConvertMarkersInText(conv.Quote, nil, notebook.ConversionStylePlain, "")
		masked := maskWord(cleaned, definition.Expression, definition.Definition)
		contexts = append(contexts, ReverseContext{
			Context:       cleaned,
			MaskedContext: masked,
		})
	}
	return contexts
}

func needsReverseReview(
	learningHistories []notebook.LearningHistory,
	storyTitle, sceneTitle string,
	definition *notebook.Note,
) bool {
	for _, h := range learningHistories {
		if h.Metadata.Title != storyTitle {
			continue
		}

		for _, scene := range h.Scenes {
			if scene.Metadata.Title != sceneTitle {
				continue
			}

			for _, expr := range scene.Expressions {
				if expr.Expression != definition.Expression && expr.Expression != definition.Definition {
					continue
				}

				if !expr.HasAnyCorrectAnswer() {
					continue
				}

				if len(expr.ReverseLogs) > 0 && !expr.NeedsReverseReview() {
					return false
				}
				return true
			}
		}
	}
	return false
}

func needsReverseFlashcardReview(
	learningHistories []notebook.LearningHistory,
	flashcardTitle string,
	card *notebook.Note,
) bool {
	for _, h := range learningHistories {
		if h.Metadata.Title != flashcardTitle {
			continue
		}

		for _, expr := range h.Expressions {
			if expr.Expression != card.Expression && expr.Expression != card.Definition {
				continue
			}

			if !expr.HasAnyCorrectAnswer() {
				continue
			}

			if len(expr.ReverseLogs) > 0 && !expr.NeedsReverseReview() {
				return false
			}
			return true
		}
	}
	return false
}

// GradeReverseAnswer grades a reverse quiz answer (user guesses the word from meaning/context).
func (s *Service) GradeReverseAnswer(ctx context.Context, card ReverseCard, answer string, responseTimeMs int64) (GradeResult, error) {
	var contextStr string
	if len(card.Contexts) > 0 {
		contextStr = card.Contexts[0].Context
	}

	validation, err := s.openaiClient.ValidateWordForm(ctx, inference.ValidateWordFormRequest{
		Expected:       card.Expression,
		UserAnswer:     answer,
		Meaning:        card.Meaning,
		Context:        contextStr,
		ResponseTimeMs: responseTimeMs,
	})
	if err != nil {
		return GradeResult{}, fmt.Errorf("failed to validate word: %w", err)
	}

	isCorrect := validation.Classification == inference.ClassificationSameWord
	quality := 1
	if isCorrect {
		if responseTimeMs < 3000 {
			quality = 5
		} else if responseTimeMs < 10000 {
			quality = 4
		} else {
			quality = 3
		}
	}

	return GradeResult{
		Correct:        isCorrect,
		Reason:         validation.Reason,
		Quality:        quality,
		Classification: string(validation.Classification),
	}, nil
}

// SaveReverseResult updates the learning history for a reverse quiz answer.
func (s *Service) SaveReverseResult(card ReverseCard, result GradeResult, responseTimeMs int64) error {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return fmt.Errorf("failed to load learning histories: %w", err)
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[card.NotebookName])
	updater.UpdateOrCreateExpressionWithQualityForReverse(
		card.NotebookName,
		card.StoryTitle,
		card.SceneTitle,
		card.Expression,
		card.Expression,
		result.Correct,
		true,
		result.Quality,
		responseTimeMs,
	)

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, card.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return fmt.Errorf("failed to save learning history for %q: %w", card.NotebookName, err)
	}

	return nil
}

// FreeformCard represents a freeform quiz card (user inputs word + meaning).
type FreeformCard struct {
	NotebookName string
	StoryTitle   string
	SceneTitle   string
	Expression   string
	Meaning      string
	Contexts     []inference.Context
}

// LoadAllWords loads all words from all notebooks for freeform quiz.
func (s *Service) LoadAllWords() ([]FreeformCard, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}

	storyIndexes := reader.GetStoryIndexes()
	flashcardIndexes := reader.GetFlashcardIndexes()

	var cards []FreeformCard

	for notebookID := range storyIndexes {
		words, err := s.loadStoryWords(reader, notebookID)
		if err != nil {
			continue
		}
		cards = append(cards, words...)
	}

	for notebookID := range flashcardIndexes {
		words, err := s.loadFlashcardWords(reader, notebookID)
		if err != nil {
			continue
		}
		cards = append(cards, words...)
	}

	return cards, nil
}

func (s *Service) loadStoryWords(reader *notebook.Reader, notebookID string) ([]FreeformCard, error) {
	stories, err := reader.ReadStoryNotebooks(notebookID)
	if err != nil {
		return nil, err
	}

	var cards []FreeformCard
	for _, story := range stories {
		for _, scene := range story.Scenes {
			for _, definition := range scene.Definitions {
				expression := definition.Expression
				if definition.Definition != "" {
					expression = definition.Definition
				}

				_, contexts := buildFromConversations(&scene, &definition)

				cards = append(cards, FreeformCard{
					NotebookName: notebookID,
					StoryTitle:   story.Event,
					SceneTitle:   scene.Title,
					Expression:   expression,
					Meaning:      definition.Meaning,
					Contexts:     contexts,
				})
			}
		}
	}

	return cards, nil
}

func (s *Service) loadFlashcardWords(reader *notebook.Reader, notebookID string) ([]FreeformCard, error) {
	notebooks, err := reader.ReadFlashcardNotebooks(notebookID)
	if err != nil {
		return nil, err
	}

	var cards []FreeformCard
	for _, nb := range notebooks {
		for _, card := range nb.Cards {
			expression := card.Expression
			if card.Definition != "" {
				expression = card.Definition
			}

			var contexts []inference.Context
			for _, ex := range card.Examples {
				contexts = append(contexts, inference.Context{
					Context:             ex,
					ReferenceDefinition: card.Meaning,
				})
			}

			cards = append(cards, FreeformCard{
				NotebookName: notebookID,
				StoryTitle:   "flashcards",
				SceneTitle:   "",
				Expression:   expression,
				Meaning:      card.Meaning,
				Contexts:     contexts,
			})
		}
	}

	return cards, nil
}

// GradeFreeformAnswer grades a freeform quiz answer (user provides word + meaning).
func (s *Service) GradeFreeformAnswer(ctx context.Context, word, meaning string, responseTimeMs int64, cards []FreeformCard) (FreeformGradeResult, error) {
	matchingCards := findMatchingCards(cards, word)

	if len(matchingCards) == 0 {
		return FreeformGradeResult{
			Correct: false,
			Word:    word,
			Meaning: meaning,
			Reason:  fmt.Sprintf("Word '%s' not found in any notebook", word),
		}, nil
	}

	results, err := s.openaiClient.AnswerMeanings(ctx, inference.AnswerMeaningsRequest{
		Expressions: []inference.Expression{
			{
				Expression:        word,
				Meaning:           meaning,
				Contexts:          matchingCards[0].Contexts,
				IsExpressionInput: true,
				ResponseTimeMs:    responseTimeMs,
			},
		},
	})
	if err != nil {
		return FreeformGradeResult{}, fmt.Errorf("failed to grade answer: %w", err)
	}

	if len(results.Answers) == 0 {
		return FreeformGradeResult{}, fmt.Errorf("no results returned from inference")
	}

	result := results.Answers[0]
	isCorrect, reason, quality := extractAnswerResult(result)

	var context string
	var notebookName string
	if len(matchingCards) > 0 {
		if len(matchingCards[0].Contexts) > 0 {
			context = matchingCards[0].Contexts[0].Context
		}
		notebookName = matchingCards[0].NotebookName
	}

	return FreeformGradeResult{
		Correct:      isCorrect,
		Word:         result.Expression,
		Meaning:      result.Meaning,
		Reason:       reason,
		Context:      context,
		NotebookName: notebookName,
		Quality:      quality,
		MatchedCard:  &matchingCards[0],
	}, nil
}

// FreeformGradeResult holds the outcome of grading a freeform answer.
type FreeformGradeResult struct {
	Correct      bool
	Word         string
	Meaning      string
	Reason       string
	Context      string
	NotebookName string
	Quality      int
	MatchedCard  *FreeformCard
}

// SaveFreeformResult updates the learning history for a freeform quiz answer.
func (s *Service) SaveFreeformResult(card FreeformCard, result FreeformGradeResult, responseTimeMs int64) error {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return fmt.Errorf("failed to load learning histories: %w", err)
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[card.NotebookName])
	updater.UpdateOrCreateExpressionWithQuality(
		card.NotebookName,
		card.StoryTitle,
		card.SceneTitle,
		card.Expression,
		card.Expression,
		result.Correct,
		true,
		result.Quality,
		responseTimeMs,
		notebook.QuizTypeFreeform,
	)

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, card.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return fmt.Errorf("failed to save learning history for %q: %w", card.NotebookName, err)
	}

	return nil
}

// kindFromIndex returns the kind string for a notebook index.
// Books loaded from books_directories have IsBook=true but an empty Kind field,
// so we fall back to "Books" when IsBook is set.
func kindFromIndex(index notebook.Index) string {
	if index.Kind != "" {
		return index.Kind
	}
	if index.IsBook {
		return "Books"
	}
	return ""
}

// GetFreeformNextReviewDates returns a map of lowercase expression -> next review date ("YYYY-MM-DD").
// Only expressions that are NOT yet due are included; due or never-studied expressions are omitted.
func (s *Service) GetFreeformNextReviewDates(cards []FreeformCard) (map[string]string, error) {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	result := make(map[string]string)
	for _, card := range cards {
		nextDate := freeformNextReviewDate(learningHistories[card.NotebookName], card)
		if nextDate != "" {
			result[strings.ToLower(card.Expression)] = nextDate
		}
	}
	return result, nil
}

func freeformNextReviewDate(histories []notebook.LearningHistory, card FreeformCard) string {
	for _, hist := range histories {
		if hist.Metadata.Title != card.StoryTitle {
			continue
		}
		if hist.Metadata.Type == "flashcard" {
			for _, expr := range hist.Expressions {
				if strings.EqualFold(expr.Expression, card.Expression) {
					return computeNextReviewDate(expr.LearnedLogs)
				}
			}
			continue
		}
		for _, scene := range hist.Scenes {
			if scene.Metadata.Title != card.SceneTitle {
				continue
			}
			for _, expr := range scene.Expressions {
				if strings.EqualFold(expr.Expression, card.Expression) {
					return computeNextReviewDate(expr.LearnedLogs)
				}
			}
		}
	}
	return ""
}

func computeNextReviewDate(logs []notebook.LearningRecord) string {
	if len(logs) == 0 || logs[0].IntervalDays == 0 {
		return ""
	}
	nextDate := logs[0].LearnedAt.AddDate(0, 0, logs[0].IntervalDays)
	if !time.Now().Before(nextDate) {
		return ""
	}
	return nextDate.Format("2006-01-02")
}

func findMatchingCards(cards []FreeformCard, word string) []FreeformCard {
	var matches []FreeformCard
	wordLower := strings.ToLower(word)
	for _, card := range cards {
		if strings.EqualFold(card.Expression, wordLower) {
			matches = append(matches, card)
		}
	}
	return matches
}
