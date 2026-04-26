package quiz

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"
	"unicode"

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
	calculator         notebook.IntervalCalculator
}

// NewService creates a new Service.
// learningRepo is optional; pass nil when DB is not configured.
func NewService(notebooksConfig config.NotebooksConfig, openaiClient inference.Client, dictionaryMap map[string]rapidapi.Response, learningRepo learning.LearningRepository, quizCfg config.QuizConfig) *Service {
	return &Service{
		notebooksConfig:    notebooksConfig,
		openaiClient:       openaiClient,
		dictionaryMap:      dictionaryMap,
		learningRepository: learningRepo,
		calculator:         notebook.NewIntervalCalculator(quizCfg.Algorithm, quizCfg.FixedIntervals),
	}
}

func (s *Service) newReader() (*notebook.Reader, error) {
	return notebook.NewReader(
		s.notebooksConfig.StoriesDirectories,
		s.notebooksConfig.FlashcardsDirectories,
		s.notebooksConfig.BooksDirectories,
		s.notebooksConfig.DefinitionsDirectories,
		s.notebooksConfig.EtymologyDirectories,
		s.dictionaryMap,
	)
}

// NewReader creates a new notebook reader. Exported for use by handlers
// that need to pass a reader to multiple service methods.
func (s *Service) NewReader() (*notebook.Reader, error) {
	return s.newReader()
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
		etymCount := countStoryEtymologyDefinitions(stories)
		summaries = append(summaries, NotebookSummary{
			NotebookID:            id,
			Name:                  index.Name,
			ReviewCount:           countStoryDefinitions(filtered),
			ReverseReviewCount:    reverseCount,
			EtymologyReviewCount:  etymCount,
			LatestDate:            latestDate,
			Kind:                  kindFromIndex(index),
			HasContent:            storyHasContent(stories),
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
		etymCount := countFlashcardEtymologyCards(notebooks)
		var latestDate time.Time
		for _, n := range notebooks {
			if n.Date.After(latestDate) {
				latestDate = n.Date
			}
		}
		summaries = append(summaries, NotebookSummary{
			NotebookID:            id,
			Name:                  index.Name,
			ReviewCount:           countFlashcardCards(filtered),
			ReverseReviewCount:    reverseCount,
			EtymologyReviewCount:  etymCount,
			LatestDate:            latestDate,
		})
	}

	// Add definitions-only books (not already in story or flashcard indexes)
	storyIndexes := reader.GetStoryIndexes()
	flashcardIndexes := reader.GetFlashcardIndexes()
	for _, nbID := range reader.GetDefinitionsBookIDs() {
		if _, isStory := storyIndexes[nbID]; isStory {
			continue
		}
		if _, isFlashcard := flashcardIndexes[nbID]; isFlashcard {
			continue
		}
		defs, ok := reader.GetDefinitionsNotes(nbID)
		if !ok {
			continue
		}
		reviewCount := countDefinitionNotes(defs, learningHistories[nbID], false)
		reverseCount := countDefinitionNotes(defs, learningHistories[nbID], true)
		if reviewCount == 0 && reverseCount == 0 {
			continue
		}
		summaries = append(summaries, NotebookSummary{
			NotebookID:         nbID,
			Name:               nbID,
			ReviewCount:        reviewCount,
			ReverseReviewCount: reverseCount,
			Kind:               "Books",
			LatestDate:         reader.GetDefinitionsLatestDate(nbID),
		})
	}

	// Add etymology notebooks
	etymSummaries, err := s.LoadEtymologyNotebookSummaries()
	if err != nil {
		return nil, fmt.Errorf("failed to load etymology notebook summaries: %w", err)
	}
	summaries = append(summaries, etymSummaries...)

	return summaries, nil
}

// buildOriginMap builds a map of origin|language -> EtymologyOrigin from all etymology notebooks.
func buildOriginMap(reader *notebook.Reader) map[string]notebook.EtymologyOrigin {
	originMap := make(map[string]notebook.EtymologyOrigin)
	for id := range reader.GetEtymologyIndexes() {
		origins, err := reader.ReadEtymologyNotebook(id)
		if err != nil {
			continue
		}
		for _, o := range origins {
			key := strings.ToLower(o.Origin + "|" + o.Language)
			originMap[key] = o
		}
	}
	return originMap
}

// resolveOriginParts resolves OriginPartRef references to full WordOriginPart data.
func resolveOriginParts(refs []notebook.OriginPartRef, originMap map[string]notebook.EtymologyOrigin) []WordOriginPart {
	if len(refs) == 0 || len(originMap) == 0 {
		return nil
	}
	var parts []WordOriginPart
	for _, ref := range refs {
		key := strings.ToLower(ref.Origin + "|" + ref.Language)
		if o, ok := originMap[key]; ok {
			parts = append(parts, WordOriginPart{Origin: o.Origin, Type: o.Type, Language: o.Language, Meaning: o.Meaning})
		} else {
			// Try matching by origin only
			for k, o := range originMap {
				if strings.HasPrefix(k, strings.ToLower(ref.Origin)+"|") {
					parts = append(parts, WordOriginPart{Origin: o.Origin, Type: o.Type, Language: o.Language, Meaning: o.Meaning})
					break
				}
			}
		}
	}
	return parts
}

func buildWordDetail(note *notebook.Note, originMap map[string]notebook.EtymologyOrigin) WordDetail {
	return WordDetail{
		Origin:        note.Origin,
		Pronunciation: note.Pronunciation,
		PartOfSpeech:  note.PartOfSpeech,
		Synonyms:      note.Synonyms,
		Antonyms:      note.Antonyms,
		Memo:          note.Memo,
		OriginParts:   resolveOriginParts(note.OriginParts, originMap),
	}
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
	originMap := buildOriginMap(reader)

	var cards []Card

	for _, notebookID := range notebookIDs {
		_, isStory := storyIndexes[notebookID]
		_, isFlashcard := flashcardIndexes[notebookID]

		if !isStory && !isFlashcard {
			// Try definitions-only book as fallback
			defCards := loadDefinitionCards(reader, notebookID, learningHistories, originMap)
			if len(defCards) > 0 {
				cards = append(cards, defCards...)
				continue
			}
			return nil, &NotFoundError{NotebookID: notebookID}
		}

		if isStory {
			storyCards, err := s.loadStoryCards(reader, notebookID, learningHistories, includeUnstudied, originMap)
			if err != nil {
				return nil, fmt.Errorf("failed to load story cards for notebook %q: %w", notebookID, err)
			}
			cards = append(cards, storyCards...)
		}

		if isFlashcard {
			flashCards, err := s.loadFlashcardCards(reader, notebookID, learningHistories, includeUnstudied, originMap)
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
	originMap map[string]notebook.EtymologyOrigin,
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
					WordDetail:    buildWordDetail(&definition, originMap),
					Images:        definition.Images,
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
	originMap map[string]notebook.EtymologyOrigin,
) ([]Card, error) {
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
				WordDetail:    buildWordDetail(&card, originMap),
				Images:        card.Images,
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

// SaveResult updates learning history via the repository.
func (s *Service) SaveResult(ctx context.Context, card Card, result GradeResult, responseTimeMs int64) error {
	status := "misunderstood"
	if result.Correct { status = "understood" }
	log := &learning.LearningLog{
		Status: status, LearnedAt: time.Now(), Quality: result.Quality,
		ResponseTimeMs: int(responseTimeMs), QuizType: string(notebook.QuizTypeNotebook),
		SourceNotebookID: card.NotebookName, NotebookName: card.NotebookName,
		StoryTitle: card.StoryTitle, SceneTitle: card.SceneTitle,
		Expression: card.Entry, OriginalExpression: card.OriginalEntry,
		IsCorrect: result.Correct, LearningNotesDir: s.notebooksConfig.LearningNotesDirectory,
	}
	if err := s.learningRepository.Create(ctx, log); err != nil {
		return fmt.Errorf("save learning log for %q: %w", card.NotebookName, err)
	}
	return nil
}

// storyHasContent reports whether any scene carries prose or dialogue worth
// rendering in the content reader. Flashcards and definitions-only books
// return false because they have neither statements nor conversations.
func storyHasContent(stories []notebook.StoryNotebook) bool {
	for _, story := range stories {
		for _, scene := range story.Scenes {
			if len(scene.Statements) > 0 || len(scene.Conversations) > 0 {
				return true
			}
		}
	}
	return false
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

func countStoryEtymologyDefinitions(stories []notebook.StoryNotebook) int {
	count := 0
	for _, story := range stories {
		for _, scene := range story.Scenes {
			for _, def := range scene.Definitions {
				if len(def.OriginParts) > 0 {
					count++
				}
			}
		}
	}
	return count
}

func countFlashcardEtymologyCards(notebooks []notebook.FlashcardNotebook) int {
	count := 0
	for _, nb := range notebooks {
		for _, card := range nb.Cards {
			if len(card.OriginParts) > 0 {
				count++
			}
		}
	}
	return count
}

// isEligibleForReverseQuiz returns true if a note has the data needed for reverse quiz
// (shows meaning, asks for the word — requires a non-empty meaning and non-unusable level).
func isEligibleForReverseQuiz(note *notebook.Note) bool {
	return note.Meaning != "" && note.Level != notebook.ExpressionLevelUnusable
}

func countReverseStoryDefinitions(stories []notebook.StoryNotebook, histories []notebook.LearningHistory) int {
	seen := make(map[string]struct{})
	for _, story := range stories {
		for _, scene := range story.Scenes {
			for i := range scene.Definitions {
				def := &scene.Definitions[i]
				if !isEligibleForReverseQuiz(def) {
					continue
				}
				if needsReverseReview(histories, story.Event, scene.Title, def) {
					expr := def.Expression
					if def.Definition != "" {
						expr = def.Definition
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
			card := &nb.Cards[i]
			if !isEligibleForReverseQuiz(card) {
				continue
			}
			if needsReverseFlashcardReview(histories, nb.Title, card) {
				expr := card.Expression
				if card.Definition != "" {
					expr = card.Definition
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
	AltForm      string // alternate inflected form (Note.Definition when set), used for masking
	WordDetail   WordDetail
	Images       []string
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
	originMap := buildOriginMap(reader)

	var cards []ReverseCard

	for _, notebookID := range notebookIDs {
		_, isStory := storyIndexes[notebookID]
		_, isFlashcard := flashcardIndexes[notebookID]

		if !isStory && !isFlashcard {
			// Try definitions-only book as fallback
			defCards := loadDefinitionReverseCards(reader, notebookID, learningHistories, originMap)
			if len(defCards) > 0 {
				cards = append(cards, defCards...)
				continue
			}
			return nil, &NotFoundError{NotebookID: notebookID}
		}

		if isStory {
			reverseCards, err := s.loadStoryReverseCards(reader, notebookID, learningHistories, listMissingContext, originMap)
			if err != nil {
				return nil, fmt.Errorf("failed to load story reverse cards for notebook %q: %w", notebookID, err)
			}
			cards = append(cards, reverseCards...)
		}

		if isFlashcard {
			reverseCards, err := s.loadFlashcardReverseCards(reader, notebookID, learningHistories, listMissingContext, originMap)
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
	applyForwardMask(cards)
	return cards, nil
}

// applyForwardMask updates each card's MaskedContext to also hide the
// expressions of all cards that come AFTER it in the session order. This
// prevents shared example sentences from leaking the answers of upcoming
// cards. Words from cards that have already been asked remain visible.
//
// The current card's expression is masked with "______" (the standard quiz
// blank), while future cards' expressions use "[...]" so users can
// distinguish the blank they need to fill from words hidden for spoiler
// protection.
func applyForwardMask(cards []ReverseCard) {
	for i := range cards {
		for j := range cards[i].Contexts {
			ctx := cards[i].Contexts[j].Context
			// Mask the current card's expression with standard blank.
			ctx = maskOccurrences(ctx, cards[i].Expression)
			if cards[i].AltForm != "" {
				ctx = maskOccurrences(ctx, cards[i].AltForm)
			}
			// Mask future cards' expressions with a distinct marker.
			for k := i + 1; k < len(cards); k++ {
				ctx = maskOccurrencesAs(ctx, cards[k].Expression, "[...]")
				if cards[k].AltForm != "" {
					ctx = maskOccurrencesAs(ctx, cards[k].AltForm, "[...]")
				}
			}
			cards[i].Contexts[j].MaskedContext = ctx
		}
	}
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
	originMap map[string]notebook.EtymologyOrigin,
) ([]ReverseCard, error) {
	stories, err := reader.ReadStoryNotebooks(notebookID)
	if err != nil {
		return nil, fmt.Errorf("failed to read story notebook %q: %w", notebookID, err)
	}

	var cards []ReverseCard
	for _, story := range stories {
		for _, scene := range story.Scenes {
			for _, definition := range scene.Definitions {
				if !isEligibleForReverseQuiz(&definition) {
					continue
				}

				expression := definition.Expression
				altForm := ""
				if definition.Definition != "" {
					expression = definition.Definition
					altForm = definition.Expression
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
					AltForm:      altForm,
					WordDetail:   buildWordDetail(&definition, originMap),
					Images:       definition.Images,
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
	originMap map[string]notebook.EtymologyOrigin,
) ([]ReverseCard, error) {
	notebooks, err := reader.ReadFlashcardNotebooks(notebookID)
	if err != nil {
		return nil, fmt.Errorf("failed to read flashcard notebook %q: %w", notebookID, err)
	}

	var cards []ReverseCard
	for _, nb := range notebooks {
		for _, card := range nb.Cards {
			if !isEligibleForReverseQuiz(&card) {
				continue
			}

			expression := card.Expression
			altForm := ""
			if card.Definition != "" {
				expression = card.Definition
				altForm = card.Expression
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
				AltForm:      altForm,
				WordDetail:   buildWordDetail(&card, originMap),
				Images:       card.Images,
			})
		}
	}

	return cards, nil
}

func maskWord(context, expression, definition string) string {
	context = maskOccurrences(context, expression)
	if definition != "" {
		context = maskOccurrences(context, definition)
	}
	return context
}

// maskOccurrences replaces every case-insensitive occurrence of target in
// context with "______". It uses \b for word-character boundaries (so partial
// matches like "questioning" don't match "question") and falls back to a
// non-word/start-of-string boundary on either side when target itself starts
// or ends with a non-word character (e.g. "#1 fan").
func maskOccurrences(context, target string) string {
	return maskOccurrencesAs(context, target, "______")
}

func maskOccurrencesAs(context, target, replacement string) string {
	if target == "" {
		return context
	}
	runes := []rune(target)
	left := `\b`
	if !isWordChar(runes[0]) {
		left = `(?:^|[^\w])`
	}
	right := `\b`
	if !isWordChar(runes[len(runes)-1]) {
		right = `(?:[^\w]|$)`
	}
	re := regexp.MustCompile(`(?i)` + left + regexp.QuoteMeta(target) + right)
	targetLower := strings.ToLower(target)
	return re.ReplaceAllStringFunc(context, func(match string) string {
		idx := strings.Index(strings.ToLower(match), targetLower)
		if idx < 0 {
			return replacement
		}
		return match[:idx] + replacement + match[idx+len(target):]
	})
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// containsExpressionWord reports whether the text contains the expression or
// a close English inflection of it, case-insensitively. It is used to
// safety-net the grading model: self-definition reasons must never fire when
// the expression word (or its stem) is absent from the user's answer.
//
// For single-word expressions of length >= 5 it drops the last character to
// form a stem, so "happy" (stem "happ") matches both "happy" and "happiness".
// Multi-word expressions and short words are matched literally.
func containsExpressionWord(text, expression string) bool {
	text = strings.ToLower(text)
	expr := strings.ToLower(strings.TrimSpace(expression))
	if expr == "" {
		return false
	}
	stem := expr
	if !strings.Contains(expr, " ") && len([]rune(expr)) >= 5 {
		r := []rune(expr)
		stem = string(r[:len(r)-1])
	}
	return strings.Contains(text, stem)
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

				// Words must be answered in freeform first AND have at
				// least one correct answer before becoming eligible for
				// reverse quiz.
				if !expr.HasFreeformAnswer() || !expr.HasAnyCorrectAnswer() {
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

			// Words must be answered in freeform first AND have at
			// least one correct answer before becoming eligible for
			// reverse quiz.
			if !expr.HasFreeformAnswer() || !expr.HasAnyCorrectAnswer() {
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

// SaveReverseResult updates learning history via the repository.
func (s *Service) SaveReverseResult(ctx context.Context, card ReverseCard, result GradeResult, responseTimeMs int64) error {
	status := "misunderstood"
	if result.Correct { status = "understood" }
	log := &learning.LearningLog{
		Status: status, LearnedAt: time.Now(), Quality: result.Quality,
		ResponseTimeMs: int(responseTimeMs), QuizType: string(notebook.QuizTypeReverse),
		SourceNotebookID: card.NotebookName, NotebookName: card.NotebookName,
		StoryTitle: card.StoryTitle, SceneTitle: card.SceneTitle,
		Expression: card.Expression, OriginalExpression: card.Expression,
		IsCorrect: result.Correct, LearningNotesDir: s.notebooksConfig.LearningNotesDirectory,
	}
	if err := s.learningRepository.Create(ctx, log); err != nil {
		return fmt.Errorf("save learning log for %q: %w", card.NotebookName, err)
	}
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
	Images             []string
}

// LoadAllWords loads all words from all notebooks for freeform quiz.
func (s *Service) LoadAllWords() ([]FreeformCard, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}

	storyIndexes := reader.GetStoryIndexes()
	flashcardIndexes := reader.GetFlashcardIndexes()
	originMap := buildOriginMap(reader)

	var cards []FreeformCard

	for notebookID := range storyIndexes {
		words, err := s.loadStoryWords(reader, notebookID, originMap)
		if err != nil {
			continue
		}
		cards = append(cards, words...)
	}

	for notebookID := range flashcardIndexes {
		words, err := s.loadFlashcardWords(reader, notebookID, originMap)
		if err != nil {
			continue
		}
		cards = append(cards, words...)
	}

	// Also load from definitions-only books
	for _, nbID := range reader.GetDefinitionsBookIDs() {
		if _, isStory := storyIndexes[nbID]; isStory {
			continue
		}
		if _, isFlashcard := flashcardIndexes[nbID]; isFlashcard {
			continue
		}
		defWords := loadDefinitionWords(reader, nbID, originMap)
		cards = append(cards, defWords...)
	}

	return cards, nil
}

func (s *Service) loadStoryWords(reader *notebook.Reader, notebookID string, originMap map[string]notebook.EtymologyOrigin) ([]FreeformCard, error) {
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
					WordDetail:         buildWordDetail(&definition, originMap),
					Images:             definition.Images,
				})
			}
		}
	}

	return cards, nil
}

func (s *Service) loadFlashcardWords(reader *notebook.Reader, notebookID string, originMap map[string]notebook.EtymologyOrigin) ([]FreeformCard, error) {
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
				WordDetail:   buildWordDetail(&card, originMap),
				Images:       card.Images,
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

	// Safety net: the grading model occasionally flags an answer as
	// "self-definition" even though the user's meaning does not contain
	// the expression word at all. Self-definition by definition requires
	// the user to reuse the expression word itself, so if the expression
	// word is absent from the meaning, override the model's verdict.
	if !isCorrect && strings.Contains(strings.ToLower(reason), "self-definition") &&
		!containsExpressionWord(meaning, word) {
		isCorrect = true
		reason = "matches the expected meaning (self-definition reason was overridden: your answer does not contain the expression word)"
		if quality < 3 {
			quality = 3
		}
	}

	var context string
	var notebookName string
	if len(matchingCards) > 0 {
		if len(matchingCards[0].Contexts) > 0 {
			context = matchingCards[0].Contexts[0].Context
		}
		notebookName = matchingCards[0].NotebookName
	}

	// Prefer the notebook's reference meaning over OpenAI's canonical
	// meaning. The notebook is what the user studied, and showing the
	// model's context-derived meaning led to cases where "Expected
	// meaning" and "Reason" described different interpretations.
	expectedMeaning := matchingCards[0].Meaning
	if expectedMeaning == "" {
		expectedMeaning = result.Meaning
	}

	return FreeformGradeResult{
		Correct:      isCorrect,
		Word:         result.Expression,
		Meaning:      expectedMeaning,
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

// SaveFreeformResult updates learning history via the repository.
func (s *Service) SaveFreeformResult(ctx context.Context, card FreeformCard, result FreeformGradeResult, responseTimeMs int64) error {
	status := "misunderstood"
	if result.Correct { status = "understood" }
	log := &learning.LearningLog{
		Status: status, LearnedAt: time.Now(), Quality: result.Quality,
		ResponseTimeMs: int(responseTimeMs), QuizType: string(notebook.QuizTypeFreeform),
		SourceNotebookID: card.NotebookName, NotebookName: card.NotebookName,
		StoryTitle: card.StoryTitle, SceneTitle: card.SceneTitle,
		Expression: card.Expression, OriginalExpression: card.Expression,
		IsCorrect: result.Correct, LearningNotesDir: s.notebooksConfig.LearningNotesDirectory,
	}
	if err := s.learningRepository.Create(ctx, log); err != nil {
		return fmt.Errorf("save learning log for %q: %w", card.NotebookName, err)
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

	// Track which expressions have at least one due (or never-studied) card.
	// If any card for an expression is due, the expression should be available.
	due := make(map[string]bool)
	result := make(map[string]string)
	for _, card := range cards {
		exprKey := strings.ToLower(card.Expression)
		origKey := strings.ToLower(card.OriginalExpression)

		nextDate := freeformNextReviewDate(learningHistories[card.NotebookName], card)
		if nextDate == "" {
			// This card is due or never studied — mark the expression as due
			due[exprKey] = true
			if origKey != "" {
				due[origKey] = true
			}
		} else if !due[exprKey] {
			// Only record the not-due date if no card for this expression is due
			existing, ok := result[exprKey]
			if !ok || nextDate > existing {
				result[exprKey] = nextDate
			}
			if origKey != "" && !due[origKey] {
				existingOrig, okOrig := result[origKey]
				if !okOrig || nextDate > existingOrig {
					result[origKey] = nextDate
				}
			}
		}
	}
	// Remove any not-due dates for expressions that have at least one due card
	for key := range due {
		delete(result, key)
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

// computeNextReviewDate returns the next review date for an expression,
// or "" when the expression is due now (so callers — freeform's "not due
// until …" badge and the Submit-button gate — treat it as immediately
// answerable). A latest log of "misunderstood" always returns "" even
// when the SR calculator wrote a positive interval: the user just got
// the word wrong and must be able to retry without waiting, mirroring
// how NeedsForwardReview / NeedsReverseReview / NeedsEtymologyReview
// already special-case misunderstood for the other quiz modes.
func computeNextReviewDate(logs []notebook.LearningRecord) string {
	if len(logs) == 0 || logs[0].IntervalDays == 0 {
		return ""
	}
	if logs[0].Status == notebook.LearnedStatusMisunderstood {
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

	updater := notebook.NewLearningHistoryUpdater(learningHistories[notebookName], s.calculator)
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

// countDefinitionNotes counts notes in definitions-only books that need review.
func countDefinitionNotes(defs map[string]map[string][]notebook.Note, histories []notebook.LearningHistory, isReverse bool) int {
	count := 0
	for storyTitle, sceneDefs := range defs {
		for sceneTitle, notes := range sceneDefs {
			for i := range notes {
				note := &notes[i]
				if note.Meaning == "" {
					continue
				}
				if isReverse {
					if needsDefinitionReverseReview(histories, storyTitle, sceneTitle, note) {
						count++
					}
				} else {
					if needsDefinitionReview(histories, storyTitle, sceneTitle, note) {
						count++
					}
				}
			}
		}
	}
	return count
}

// loadDefinitionCards loads standard quiz cards from definitions-only books.
func loadDefinitionCards(reader *notebook.Reader, bookID string, learningHistories map[string][]notebook.LearningHistory, originMap map[string]notebook.EtymologyOrigin) []Card {
	defs, ok := reader.GetDefinitionsNotes(bookID)
	if !ok {
		return nil
	}

	var cards []Card
	for storyTitle, sceneDefs := range defs {
		for sceneTitle, notes := range sceneDefs {
			for _, note := range notes {
				if note.Meaning == "" {
					continue
				}
				if !needsDefinitionReview(learningHistories[bookID], storyTitle, sceneTitle, &note) {
					continue
				}
				entry := note.Definition
				originalEntry := ""
				if entry == "" {
					entry = note.Expression
				} else {
					originalEntry = note.Expression
				}
				cards = append(cards, Card{
					NotebookName:  bookID,
					StoryTitle:    storyTitle,
					SceneTitle:    sceneTitle,
					Entry:         entry,
					OriginalEntry: originalEntry,
					Meaning:       note.Meaning,
					WordDetail:    buildWordDetail(&note, originMap),
				})
			}
		}
	}
	return cards
}

// loadDefinitionReverseCards loads reverse quiz cards from definitions-only books.
func loadDefinitionReverseCards(reader *notebook.Reader, bookID string, learningHistories map[string][]notebook.LearningHistory, originMap map[string]notebook.EtymologyOrigin) []ReverseCard {
	defs, ok := reader.GetDefinitionsNotes(bookID)
	if !ok {
		return nil
	}

	var cards []ReverseCard
	for storyTitle, sceneDefs := range defs {
		for sceneTitle, notes := range sceneDefs {
			for _, note := range notes {
				if note.Meaning == "" {
					continue
				}
				if !needsDefinitionReverseReview(learningHistories[bookID], storyTitle, sceneTitle, &note) {
					continue
				}
				expression := note.Expression
				altForm := ""
				if note.Definition != "" {
					expression = note.Definition
					altForm = note.Expression
				}
				cards = append(cards, ReverseCard{
					NotebookName: bookID,
					StoryTitle:   storyTitle,
					SceneTitle:   sceneTitle,
					Meaning:      note.Meaning,
					Expression:   expression,
					AltForm:      altForm,
					WordDetail:   buildWordDetail(&note, originMap),
				})
			}
		}
	}
	return cards
}

// loadDefinitionWords loads freeform cards from definitions-only books.
func loadDefinitionWords(reader *notebook.Reader, bookID string, originMap map[string]notebook.EtymologyOrigin) []FreeformCard {
	defs, ok := reader.GetDefinitionsNotes(bookID)
	if !ok {
		return nil
	}

	var cards []FreeformCard
	for storyTitle, sceneDefs := range defs {
		for sceneTitle, notes := range sceneDefs {
			for _, note := range notes {
				if note.Meaning == "" {
					continue
				}
				expression := note.Expression
				if note.Definition != "" {
					expression = note.Definition
				}
				cards = append(cards, FreeformCard{
					NotebookName:       bookID,
					StoryTitle:         storyTitle,
					SceneTitle:         sceneTitle,
					Expression:         expression,
					OriginalExpression: note.Expression,
					Meaning:            note.Meaning,
					WordDetail:         buildWordDetail(&note, originMap),
				})
			}
		}
	}
	return cards
}

// needsDefinitionReview checks if a definition note needs forward quiz review.
func needsDefinitionReview(
	histories []notebook.LearningHistory,
	storyTitle, sceneTitle string,
	note *notebook.Note,
) bool {
	for _, h := range histories {
		if h.Metadata.Title != storyTitle {
			continue
		}
		for _, scene := range h.Scenes {
			if scene.Metadata.Title != sceneTitle {
				continue
			}
			for _, expr := range scene.Expressions {
				if expr.Expression != note.Expression && expr.Expression != note.Definition {
					continue
				}
				// Words must be answered in freeform first.
				if !expr.HasFreeformAnswer() {
					return false
				}
				return expr.NeedsForwardReview()
			}
		}
	}
	return false
}

// needsDefinitionReverseReview checks if a definition note needs reverse quiz review.
func needsDefinitionReverseReview(
	histories []notebook.LearningHistory,
	storyTitle, sceneTitle string,
	note *notebook.Note,
) bool {
	for _, h := range histories {
		if h.Metadata.Title != storyTitle {
			continue
		}
		for _, scene := range h.Scenes {
			if scene.Metadata.Title != sceneTitle {
				continue
			}
			for _, expr := range scene.Expressions {
				if expr.Expression != note.Expression && expr.Expression != note.Definition {
					continue
				}
				// Words must be answered in freeform first AND have at
				// least one correct answer before becoming eligible for
				// reverse quiz.
				if !expr.HasFreeformAnswer() || !expr.HasAnyCorrectAnswer() {
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
