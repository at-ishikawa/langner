package quiz

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/dictionary/rapidapi"
	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// Service owns all quiz business logic shared between the CLI and RPC handler.
type Service struct {
	notebooksConfig    config.NotebooksConfig
	openaiClient       inference.Client
	dictionaryMap      map[string]rapidapi.Response
	learningRepository learning.LearningRepository
}

// NewService creates a new Service.
// learningRepo is optional; pass nil when DB is not configured.
func NewService(notebooksConfig config.NotebooksConfig, openaiClient inference.Client, dictionaryMap map[string]rapidapi.Response, learningRepo learning.LearningRepository) *Service {
	return &Service{
		notebooksConfig:    notebooksConfig,
		openaiClient:       openaiClient,
		dictionaryMap:      dictionaryMap,
		learningRepository: learningRepo,
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
			false, false, true, false,
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
		reverseCount := countReverseStoryDefinitions(stories, learningHistories[id])
		summaries = append(summaries, NotebookSummary{
			NotebookID:         id,
			Name:               index.Name,
			ReviewCount:        countStoryDefinitions(filtered),
			ReverseReviewCount: reverseCount,
			LatestStoryDate:    latestDate,
			Kind:               kindFromIndex(index),
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

		reverseCount := countReverseFlashcardCards(notebooks, learningHistories[id])
		summaries = append(summaries, NotebookSummary{
			NotebookID:         id,
			Name:               index.Name,
			ReviewCount:        countFlashcardCards(filtered),
			ReverseReviewCount: reverseCount,
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

	cards = deduplicateCards(cards)
	rand.Shuffle(len(cards), func(i, j int) {
		cards[i], cards[j] = cards[j], cards[i]
	})
	return cards, nil
}

func deduplicateCards(cards []Card) []Card {
	seen := make(map[string]int) // entry -> index in result
	var result []Card
	for _, card := range cards {
		key := strings.ToLower(card.Entry)
		if idx, ok := seen[key]; ok {
			// Keep the card with more examples/contexts
			if len(card.Examples) > len(result[idx].Examples) {
				result[idx] = card
			}
		} else {
			seen[key] = len(result)
			result = append(result, card)
		}
	}
	return result
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
					WordDetail: WordDetail{
						Origin:        definition.Origin,
						Pronunciation: definition.Pronunciation,
						PartOfSpeech:  definition.PartOfSpeech,
						Synonyms:      definition.Synonyms,
						Antonyms:      definition.Antonyms,
						Memo:          definition.Memo,
					},
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
				WordDetail: WordDetail{
					Origin:        card.Origin,
					Pronunciation: card.Pronunciation,
					PartOfSpeech:  card.PartOfSpeech,
					Synonyms:      card.Synonyms,
					Antonyms:      card.Antonyms,
					Memo:          card.Memo,
				},
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
func (s *Service) SaveResult(ctx context.Context, card Card, result GradeResult, responseTimeMs int64) error {
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

	s.saveLearningLogToDB(ctx, card.NotebookName, card.Entry, result.Correct, result.Quality, responseTimeMs, string(notebook.QuizTypeNotebook))
	return nil
}

func countStoryDefinitions(stories []notebook.StoryNotebook) int {
	seen := make(map[string]struct{})
	for _, story := range stories {
		for _, scene := range story.Scenes {
			for _, def := range scene.Definitions {
				entry := def.Definition
				if entry == "" {
					entry = def.Expression
				}
				seen[strings.ToLower(entry)] = struct{}{}
			}
		}
	}
	return len(seen)
}

func countReverseStoryDefinitions(stories []notebook.StoryNotebook, histories []notebook.LearningHistory) int {
	seen := make(map[string]struct{})
	for _, story := range stories {
		for _, scene := range story.Scenes {
			for i := range scene.Definitions {
				if needsReverseReview(histories, story.Event, scene.Title, &scene.Definitions[i]) {
					expr := scene.Definitions[i].Expression
					if scene.Definitions[i].Definition != "" {
						expr = scene.Definitions[i].Definition
					}
					seen[strings.ToLower(expr)] = struct{}{}
				}
			}
		}
	}
	return len(seen)
}

func countReverseFlashcardCards(notebooks []notebook.FlashcardNotebook, histories []notebook.LearningHistory) int {
	seen := make(map[string]struct{})
	for _, nb := range notebooks {
		for i := range nb.Cards {
			if needsReverseFlashcardReview(histories, nb.Title, &nb.Cards[i]) {
				expr := nb.Cards[i].Expression
				if nb.Cards[i].Definition != "" {
					expr = nb.Cards[i].Definition
				}
				seen[strings.ToLower(expr)] = struct{}{}
			}
		}
	}
	return len(seen)
}

func countFlashcardCards(notebooks []notebook.FlashcardNotebook) int {
	seen := make(map[string]struct{})
	for _, nb := range notebooks {
		for _, card := range nb.Cards {
			entry := card.Definition
			if entry == "" {
				entry = card.Expression
			}
			seen[strings.ToLower(entry)] = struct{}{}
		}
	}
	return len(seen)
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
	WordDetail   WordDetail
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

	cards = deduplicateReverseCards(cards)
	rand.Shuffle(len(cards), func(i, j int) {
		cards[i], cards[j] = cards[j], cards[i]
	})
	return cards, nil
}

func deduplicateReverseCards(cards []ReverseCard) []ReverseCard {
	seen := make(map[string]int) // expression -> index in result
	var result []ReverseCard
	for _, card := range cards {
		expr := strings.ToLower(card.Expression)
		if idx, ok := seen[expr]; ok {
			if len(card.Contexts) > len(result[idx].Contexts) {
				result[idx] = card
			}
		} else {
			seen[expr] = len(result)
			result = append(result, card)
		}
	}
	return result
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

	var cards []ReverseCard
	for _, story := range stories {
		for _, scene := range story.Scenes {
			for _, definition := range scene.Definitions {
				expression := definition.Expression
				if definition.Definition != "" {
					expression = definition.Definition
				}

				// Skip words marked as skipped
				if isExpressionSkippedInHistory(learningHistories[notebookID], story.Event, scene.Title, &definition) {
					continue
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
					WordDetail: WordDetail{
						Origin:        definition.Origin,
						Pronunciation: definition.Pronunciation,
						PartOfSpeech:  definition.PartOfSpeech,
						Synonyms:      definition.Synonyms,
						Antonyms:      definition.Antonyms,
						Memo:          definition.Memo,
					},
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

	var cards []ReverseCard
	for _, nb := range notebooks {
		for _, card := range nb.Cards {
			expression := card.Expression
			if card.Definition != "" {
				expression = card.Definition
			}

			// Skip words marked as skipped
			if isExpressionSkippedInHistory(learningHistories[notebookID], nb.Title, "", &card) {
				continue
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
				WordDetail: WordDetail{
					Origin:        card.Origin,
					Pronunciation: card.Pronunciation,
					PartOfSpeech:  card.PartOfSpeech,
					Synonyms:      card.Synonyms,
					Antonyms:      card.Antonyms,
					Memo:          card.Memo,
				},
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
func (s *Service) SaveReverseResult(ctx context.Context, card ReverseCard, result GradeResult, responseTimeMs int64) error {
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

	s.saveLearningLogToDB(ctx, card.NotebookName, card.Expression, result.Correct, result.Quality, responseTimeMs, string(notebook.QuizTypeReverse))
	return nil
}

// FreeformCard represents a freeform quiz card (user inputs word + meaning).
type FreeformCard struct {
	NotebookName       string
	StoryTitle         string
	SceneTitle         string
	Expression         string // canonical form (Definition if set, otherwise Expression)
	OriginalExpression string // text form as it appears in the story (Note.Expression)
	Meaning            string
	Contexts           []inference.Context
	WordDetail         WordDetail
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

	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	var cards []FreeformCard
	for _, story := range stories {
		for _, scene := range story.Scenes {
			for _, definition := range scene.Definitions {
				// Skip words marked as skipped
				if isExpressionSkippedInHistory(learningHistories[notebookID], story.Event, scene.Title, &definition) {
					continue
				}

				expression := definition.Expression
				if definition.Definition != "" {
					expression = definition.Definition
				}

				_, contexts := buildFromConversations(&scene, &definition)

				cards = append(cards, FreeformCard{
					NotebookName:       notebookID,
					StoryTitle:         story.Event,
					SceneTitle:         scene.Title,
					Expression:         expression,
					OriginalExpression: definition.Expression,
					Meaning:            definition.Meaning,
					Contexts:           contexts,
					WordDetail: WordDetail{
						Origin:        definition.Origin,
						Pronunciation: definition.Pronunciation,
						PartOfSpeech:  definition.PartOfSpeech,
						Synonyms:      definition.Synonyms,
						Antonyms:      definition.Antonyms,
						Memo:          definition.Memo,
					},
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

	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	var cards []FreeformCard
	for _, nb := range notebooks {
		for _, card := range nb.Cards {
			// Skip words marked as skipped
			if isExpressionSkippedInHistory(learningHistories[notebookID], nb.Title, "", &card) {
				continue
			}

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
				WordDetail: WordDetail{
					Origin:        card.Origin,
					Pronunciation: card.Pronunciation,
					PartOfSpeech:  card.PartOfSpeech,
					Synonyms:      card.Synonyms,
					Antonyms:      card.Antonyms,
					Memo:          card.Memo,
				},
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
func (s *Service) SaveFreeformResult(ctx context.Context, card FreeformCard, result FreeformGradeResult, responseTimeMs int64) error {
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

	s.saveLearningLogToDB(ctx, card.NotebookName, card.Expression, result.Correct, result.Quality, responseTimeMs, string(notebook.QuizTypeFreeform))
	return nil
}

// saveLearningLogToDB writes a learning log to the DB repository if configured.
// DB writes are best-effort; errors are logged but do not fail the request.
func (s *Service) saveLearningLogToDB(ctx context.Context, sourceNotebookID, entry string, isCorrect bool, quality int, responseTimeMs int64, quizType string) {
	if s.learningRepository == nil {
		return
	}

	status := string(notebook.LearnedStatusMisunderstood)
	if isCorrect {
		status = string(notebook.LearnedStatusUnderstood)
	}

	// NoteID is left as 0 because the quiz path doesn't have note IDs.
	// The data can be correlated via expression and source_notebook_id.
	log := &learning.LearningLog{
		Status:           status,
		LearnedAt:        time.Now(),
		Quality:          quality,
		ResponseTimeMs:   int(responseTimeMs),
		QuizType:         quizType,
		SourceNotebookID: sourceNotebookID,
	}

	if err := s.learningRepository.Create(ctx, log); err != nil {
		slog.Warn("failed to write learning log to DB", "entry", entry, "error", err)
	}
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
			if card.OriginalExpression != "" {
				result[strings.ToLower(card.OriginalExpression)] = nextDate
			}
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
	for _, card := range cards {
		if strings.EqualFold(card.Expression, word) || strings.EqualFold(card.OriginalExpression, word) {
			matches = append(matches, card)
		}
	}
	return matches
}

// GetLatestLearnedInfo returns the learned_at and next_review_date for the latest log
// of a given expression in a specific notebook.
func (s *Service) GetLatestLearnedInfo(notebookName, expression string, quizType notebook.QuizType) (learnedAt string, nextReviewDate string) {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return "", ""
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[notebookName])
	expr := updater.FindExpressionByName(expression)
	if expr == nil {
		return "", ""
	}

	logs := expr.GetLogsForQuizType(quizType)
	if len(logs) == 0 {
		return "", ""
	}

	latest := logs[0]
	learnedAt = latest.LearnedAt.Format("2006-01-02")
	if latest.IntervalDays > 0 {
		nextReviewDate = latest.LearnedAt.AddDate(0, 0, latest.IntervalDays).Format("2006-01-02")
	}
	return learnedAt, nextReviewDate
}

// isExpressionSkippedInHistory checks if a note is marked as skipped in the learning history.
func isExpressionSkippedInHistory(histories []notebook.LearningHistory, event, sceneTitle string, def *notebook.Note) bool {
	return notebook.IsExpressionSkipped(histories, event, sceneTitle, def.Expression, def.Definition)
}
