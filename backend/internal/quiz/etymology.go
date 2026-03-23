package quiz

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"

	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// EtymologyCard represents a card for etymology quiz.
type EtymologyCard struct {
	NotebookName string
	StoryTitle   string
	SceneTitle   string
	Expression   string
	Meaning      string
	OriginParts  []EtymologyOriginPart
}

// EtymologyOriginPart represents an origin part attached to a card.
type EtymologyOriginPart struct {
	Origin   string
	Type     string
	Language string
	Meaning  string
}

// EtymologyGradeResult holds the outcome of grading an etymology answer.
type EtymologyGradeResult struct {
	Correct      bool
	Reason       string
	Quality      int
	OriginGrades []inference.EtymologyOriginGrade
}

// RelatedDefinition holds info about a definition related to an etymology origin.
type RelatedDefinition struct {
	Expression   string
	Meaning      string
	NotebookName string
}

// LoadEtymologyCards loads etymology quiz cards from selected notebooks.
func (s *Service) LoadEtymologyCards(
	etymologyNotebookIDs []string,
	definitionNotebookIDs []string,
	includeUnstudied bool,
) ([]EtymologyCard, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}

	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	// Build a map of origin -> EtymologyOriginPart from all selected etymology notebooks
	originMap := make(map[string]EtymologyOriginPart)
	for _, etymID := range etymologyNotebookIDs {
		origins, err := reader.ReadEtymologyNotebook(etymID)
		if err != nil {
			return nil, fmt.Errorf("failed to read etymology notebook %q: %w", etymID, err)
		}
		for _, o := range origins {
			key := strings.ToLower(o.Origin + "|" + o.Language)
			originMap[key] = EtymologyOriginPart{
				Origin:   o.Origin,
				Type:     o.Type,
				Language: o.Language,
				Meaning:  o.Meaning,
			}
		}
	}

	// Collect all notebook IDs to search for definitions with origin_parts
	allNotebookIDs := collectAllNotebookIDs(reader, definitionNotebookIDs)

	var cards []EtymologyCard
	storyIndexes := reader.GetStoryIndexes()
	flashcardIndexes := reader.GetFlashcardIndexes()

	for _, nbID := range allNotebookIDs {
		if _, isStory := storyIndexes[nbID]; isStory {
			storyCards, err := s.loadEtymologyStoryCards(reader, nbID, originMap, learningHistories, includeUnstudied)
			if err != nil {
				return nil, fmt.Errorf("failed to load etymology story cards for %q: %w", nbID, err)
			}
			cards = append(cards, storyCards...)
		}
		if _, isFlashcard := flashcardIndexes[nbID]; isFlashcard {
			flashcardCards, err := s.loadEtymologyFlashcardCards(reader, nbID, originMap, learningHistories, includeUnstudied)
			if err != nil {
				return nil, fmt.Errorf("failed to load etymology flashcard cards for %q: %w", nbID, err)
			}
			cards = append(cards, flashcardCards...)
		}
	}

	// Also load from definitions-only books (not in story or flashcard indexes)
	defBookIDs := collectDefinitionsOnlyBookIDs(reader, storyIndexes, flashcardIndexes, definitionNotebookIDs)
	for _, nbID := range defBookIDs {
		defCards := loadEtymologyDefinitionsCards(reader, nbID, originMap, learningHistories, includeUnstudied)
		cards = append(cards, defCards...)
	}

	cards = deduplicateEtymologyCards(cards)
	rand.Shuffle(len(cards), func(i, j int) {
		cards[i], cards[j] = cards[j], cards[i]
	})
	return cards, nil
}

func collectAllNotebookIDs(reader *notebook.Reader, definitionNotebookIDs []string) []string {
	if len(definitionNotebookIDs) > 0 {
		return definitionNotebookIDs
	}

	var ids []string
	for id := range reader.GetStoryIndexes() {
		ids = append(ids, id)
	}
	for id := range reader.GetFlashcardIndexes() {
		ids = append(ids, id)
	}
	return ids
}

func (s *Service) loadEtymologyStoryCards(
	reader *notebook.Reader,
	notebookID string,
	originMap map[string]EtymologyOriginPart,
	learningHistories map[string][]notebook.LearningHistory,
	includeUnstudied bool,
) ([]EtymologyCard, error) {
	stories, err := reader.ReadStoryNotebooks(notebookID)
	if err != nil {
		return nil, err
	}

	var cards []EtymologyCard
	for _, story := range stories {
		for _, scene := range story.Scenes {
			for _, def := range scene.Definitions {
				if len(def.OriginParts) == 0 {
					continue
				}

				parts := resolveOriginParts(def.OriginParts, originMap)
				if len(parts) == 0 {
					continue
				}

				// Check SM-2 schedule
				if !includeUnstudied {
					if !needsEtymologyReview(learningHistories[notebookID], story.Event, scene.Title, &def, notebook.QuizTypeEtymologyBreakdown) {
						continue
					}
				}

				cards = append(cards, EtymologyCard{
					NotebookName: notebookID,
					StoryTitle:   story.Event,
					SceneTitle:   scene.Title,
					Expression:   def.Expression,
					Meaning:      def.Meaning,
					OriginParts:  parts,
				})
			}
		}
	}
	return cards, nil
}

func (s *Service) loadEtymologyFlashcardCards(
	reader *notebook.Reader,
	notebookID string,
	originMap map[string]EtymologyOriginPart,
	learningHistories map[string][]notebook.LearningHistory,
	includeUnstudied bool,
) ([]EtymologyCard, error) {
	notebooks, err := reader.ReadFlashcardNotebooks(notebookID)
	if err != nil {
		return nil, err
	}

	var cards []EtymologyCard
	for _, nb := range notebooks {
		for _, card := range nb.Cards {
			if len(card.OriginParts) == 0 {
				continue
			}

			parts := resolveOriginParts(card.OriginParts, originMap)
			if len(parts) == 0 {
				continue
			}

			if !includeUnstudied {
				if !needsEtymologyFlashcardReview(learningHistories[notebookID], nb.Title, &card, notebook.QuizTypeEtymologyBreakdown) {
					continue
				}
			}

			cards = append(cards, EtymologyCard{
				NotebookName: notebookID,
				StoryTitle:   "flashcards",
				Expression:   card.Expression,
				Meaning:      card.Meaning,
				OriginParts:  parts,
			})
		}
	}
	return cards, nil
}

func resolveOriginParts(refs []notebook.OriginPartRef, originMap map[string]EtymologyOriginPart) []EtymologyOriginPart {
	var parts []EtymologyOriginPart
	for _, ref := range refs {
		key := strings.ToLower(ref.Origin + "|" + ref.Language)
		if part, ok := originMap[key]; ok {
			parts = append(parts, part)
		} else {
			// Try matching by origin only (no language)
			for k, v := range originMap {
				if strings.HasPrefix(k, strings.ToLower(ref.Origin)+"|") {
					parts = append(parts, v)
					break
				}
			}
		}
	}
	return parts
}

func needsEtymologyReview(
	histories []notebook.LearningHistory,
	storyTitle, sceneTitle string,
	def *notebook.Note,
	quizType notebook.QuizType,
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
				if expr.Expression != def.Expression && expr.Expression != def.Definition {
					continue
				}
				return expr.NeedsEtymologyReview(quizType)
			}
		}
	}
	return true // No history found, needs review
}

func needsEtymologyFlashcardReview(
	histories []notebook.LearningHistory,
	flashcardTitle string,
	card *notebook.Note,
	quizType notebook.QuizType,
) bool {
	for _, h := range histories {
		if h.Metadata.Title != flashcardTitle {
			continue
		}
		for _, expr := range h.Expressions {
			if expr.Expression != card.Expression && expr.Expression != card.Definition {
				continue
			}
			return expr.NeedsEtymologyReview(quizType)
		}
	}
	return true
}

func collectDefinitionsOnlyBookIDs(
	reader *notebook.Reader,
	storyIndexes map[string]notebook.Index,
	flashcardIndexes map[string]notebook.FlashcardIndex,
	definitionNotebookIDs []string,
) []string {
	wanted := make(map[string]bool)
	if len(definitionNotebookIDs) > 0 {
		for _, id := range definitionNotebookIDs {
			wanted[id] = true
		}
	}

	var ids []string
	for _, nbID := range reader.GetDefinitionsBookIDs() {
		// Skip if already covered by story or flashcard indexes
		if _, isStory := storyIndexes[nbID]; isStory {
			continue
		}
		if _, isFlashcard := flashcardIndexes[nbID]; isFlashcard {
			continue
		}
		// If specific IDs requested, only include those
		if len(wanted) > 0 && !wanted[nbID] {
			continue
		}
		ids = append(ids, nbID)
	}
	return ids
}

func loadEtymologyDefinitionsCards(
	reader *notebook.Reader,
	bookID string,
	originMap map[string]EtymologyOriginPart,
	learningHistories map[string][]notebook.LearningHistory,
	includeUnstudied bool,
) []EtymologyCard {
	defs, ok := reader.GetDefinitionsNotes(bookID)
	if !ok {
		return nil
	}

	var cards []EtymologyCard
	for title, sceneDefs := range defs {
		for _, notes := range sceneDefs {
			for _, note := range notes {
				if len(note.OriginParts) == 0 {
					continue
				}

				parts := resolveOriginParts(note.OriginParts, originMap)
				if len(parts) == 0 {
					continue
				}

				if !includeUnstudied {
					if !needsEtymologyReview(learningHistories[bookID], title, "", &note, notebook.QuizTypeEtymologyBreakdown) {
						continue
					}
				}

				cards = append(cards, EtymologyCard{
					NotebookName: bookID,
					StoryTitle:   title,
					Expression:   note.Expression,
					Meaning:      note.Meaning,
					OriginParts:  parts,
				})
			}
		}
	}
	return cards
}

func deduplicateEtymologyCards(cards []EtymologyCard) []EtymologyCard {
	seen := make(map[string]int)
	var result []EtymologyCard
	for _, card := range cards {
		key := strings.ToLower(card.Expression)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = len(result)
		result = append(result, card)
	}
	return result
}

// GradeEtymologyBreakdownAnswer grades a breakdown answer using OpenAI.
func (s *Service) GradeEtymologyBreakdownAnswer(
	ctx context.Context,
	card EtymologyCard,
	userOrigins []inference.EtymologyUserOrigin,
	responseTimeMs int64,
) (EtymologyGradeResult, error) {
	var expectedOrigins []inference.EtymologyExpectedOrigin
	for _, p := range card.OriginParts {
		expectedOrigins = append(expectedOrigins, inference.EtymologyExpectedOrigin{
			Origin:   p.Origin,
			Type:     p.Type,
			Language: p.Language,
			Meaning:  p.Meaning,
		})
	}

	resp, err := s.openaiClient.GradeEtymologyBreakdown(ctx, inference.GradeEtymologyBreakdownRequest{
		Expression:      card.Expression,
		ExpectedOrigins: expectedOrigins,
		UserOrigins:     userOrigins,
		ResponseTimeMs:  responseTimeMs,
	})
	if err != nil {
		return EtymologyGradeResult{}, fmt.Errorf("grade etymology breakdown: %w", err)
	}

	return EtymologyGradeResult{
		Correct:      resp.Correct,
		Reason:       resp.Reason,
		Quality:      resp.Quality,
		OriginGrades: resp.OriginGrades,
	}, nil
}

// GradeEtymologyAssemblyAnswer grades an assembly answer using ValidateWordForm.
func (s *Service) GradeEtymologyAssemblyAnswer(
	ctx context.Context,
	card EtymologyCard,
	answer string,
	responseTimeMs int64,
) (GradeResult, error) {
	validation, err := s.openaiClient.ValidateWordForm(ctx, inference.ValidateWordFormRequest{
		Expected:       card.Expression,
		UserAnswer:     answer,
		Meaning:        card.Meaning,
		ResponseTimeMs: responseTimeMs,
	})
	if err != nil {
		return GradeResult{}, fmt.Errorf("validate word form: %w", err)
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

// SaveEtymologyResult updates the learning history for an etymology quiz answer.
func (s *Service) SaveEtymologyResult(card EtymologyCard, quality int, correct bool, responseTimeMs int64, quizType notebook.QuizType) error {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return fmt.Errorf("failed to load learning histories: %w", err)
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[card.NotebookName], s.calculator)
	updater.UpdateOrCreateExpressionWithQualityForEtymology(
		card.NotebookName,
		card.StoryTitle,
		card.SceneTitle,
		card.Expression,
		card.Expression,
		correct,
		true,
		quality,
		responseTimeMs,
		quizType,
	)

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, card.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return fmt.Errorf("failed to save learning history for %q: %w", card.NotebookName, err)
	}

	return nil
}

// FindRelatedDefinitions finds definitions that share origin parts with the given card.
func (s *Service) FindRelatedDefinitions(reader *notebook.Reader, card EtymologyCard) []RelatedDefinition {
	originSet := make(map[string]bool)
	for _, p := range card.OriginParts {
		originSet[strings.ToLower(p.Origin)] = true
	}

	var related []RelatedDefinition
	seen := make(map[string]bool)

	// Search story notebooks
	for nbID := range reader.GetStoryIndexes() {
		stories, err := reader.ReadStoryNotebooks(nbID)
		if err != nil {
			continue
		}
		for _, story := range stories {
			for _, scene := range story.Scenes {
				for _, def := range scene.Definitions {
					if strings.EqualFold(def.Expression, card.Expression) {
						continue
					}
					if hasMatchingOrigin(def.OriginParts, originSet) {
						key := strings.ToLower(def.Expression)
						if !seen[key] {
							seen[key] = true
							related = append(related, RelatedDefinition{
								Expression:   def.Expression,
								Meaning:      def.Meaning,
								NotebookName: nbID,
							})
						}
					}
				}
			}
		}
	}

	// Search flashcard notebooks
	for nbID := range reader.GetFlashcardIndexes() {
		notebooks, err := reader.ReadFlashcardNotebooks(nbID)
		if err != nil {
			continue
		}
		for _, nb := range notebooks {
			for _, c := range nb.Cards {
				if strings.EqualFold(c.Expression, card.Expression) {
					continue
				}
				if hasMatchingOrigin(c.OriginParts, originSet) {
					key := strings.ToLower(c.Expression)
					if !seen[key] {
						seen[key] = true
						related = append(related, RelatedDefinition{
							Expression:   c.Expression,
							Meaning:      c.Meaning,
							NotebookName: nbID,
						})
					}
				}
			}
		}
	}

	// Search definitions-only books
	for _, nbID := range reader.GetDefinitionsBookIDs() {
		defs, ok := reader.GetDefinitionsNotes(nbID)
		if !ok {
			continue
		}
		for _, sceneDefs := range defs {
			for _, notes := range sceneDefs {
				for _, note := range notes {
					if strings.EqualFold(note.Expression, card.Expression) {
						continue
					}
					if hasMatchingOrigin(note.OriginParts, originSet) {
						key := strings.ToLower(note.Expression)
						if !seen[key] {
							seen[key] = true
							related = append(related, RelatedDefinition{
								Expression:   note.Expression,
								Meaning:      note.Meaning,
								NotebookName: nbID,
							})
						}
					}
				}
			}
		}
	}

	return related
}

func hasMatchingOrigin(refs []notebook.OriginPartRef, originSet map[string]bool) bool {
	for _, ref := range refs {
		if originSet[strings.ToLower(ref.Origin)] {
			return true
		}
	}
	return false
}

// LoadEtymologyFreeformExpressions loads all expressions with origin_parts for freeform quiz.
func (s *Service) LoadEtymologyFreeformExpressions(
	etymologyNotebookIDs []string,
	definitionNotebookIDs []string,
) ([]EtymologyCard, error) {
	// Same as LoadEtymologyCards but without SM-2 filtering
	return s.LoadEtymologyCards(etymologyNotebookIDs, definitionNotebookIDs, true)
}

// GetEtymologyNextReviewDates returns a map of lowercase expression -> next review date.
func (s *Service) GetEtymologyNextReviewDates(cards []EtymologyCard) (map[string]string, error) {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	result := make(map[string]string)
	for _, card := range cards {
		nextDate := etymologyNextReviewDate(learningHistories[card.NotebookName], card)
		if nextDate != "" {
			result[strings.ToLower(card.Expression)] = nextDate
		}
	}
	return result, nil
}

func etymologyNextReviewDate(histories []notebook.LearningHistory, card EtymologyCard) string {
	for _, hist := range histories {
		if hist.Metadata.Title != card.StoryTitle {
			continue
		}
		if hist.Metadata.Type == "flashcard" {
			for _, expr := range hist.Expressions {
				if strings.EqualFold(expr.Expression, card.Expression) {
					return computeNextReviewDate(expr.EtymologyBreakdownLogs)
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
					return computeNextReviewDate(expr.EtymologyBreakdownLogs)
				}
			}
		}
	}
	return ""
}

// LoadEtymologyNotebookSummaries returns etymology notebook summaries.
func (s *Service) LoadEtymologyNotebookSummaries() ([]NotebookSummary, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}

	var summaries []NotebookSummary
	for id, index := range reader.GetEtymologyIndexes() {
		origins, err := reader.ReadEtymologyNotebook(id)
		if err != nil {
			continue
		}

		summaries = append(summaries, NotebookSummary{
			NotebookID:  id,
			Name:        index.Name,
			ReviewCount: len(origins),
			Kind:        "Etymology",
		})
	}

	return summaries, nil
}
