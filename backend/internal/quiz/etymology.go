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

// EtymologyOriginCard represents a single origin for the origin-based etymology quiz.
type EtymologyOriginCard struct {
	NotebookName  string
	NotebookTitle string
	Origin        string
	Type          string
	Language      string
	Meaning       string
}

// LoadEtymologyOriginCards loads individual origin cards from selected etymology notebooks.
// When skipEligibility is true the hard gate (freeform-first + has-correct-answer)
// is skipped — the freeform quiz needs this because it IS the entry point where
// origins are encountered for the first time.
func (s *Service) LoadEtymologyOriginCards(
	etymologyNotebookIDs []string,
	includeUnstudied bool,
	skipEligibility bool,
) ([]EtymologyOriginCard, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}

	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	etymIndexes := reader.GetEtymologyIndexes()

	seen := make(map[string]bool) // key: lowercase origin|language
	var cards []EtymologyOriginCard

	for _, etymID := range etymologyNotebookIDs {
		origins, err := reader.ReadEtymologyNotebook(etymID)
		if err != nil {
			return nil, fmt.Errorf("failed to read etymology notebook %q: %w", etymID, err)
		}

		nbTitle := etymID
		if idx, ok := etymIndexes[etymID]; ok {
			nbTitle = idx.Name
		}

		for _, o := range origins {
			key := strings.ToLower(o.Origin + "|" + o.Language)
			if seen[key] {
				continue
			}
			seen[key] = true

			// Hard eligibility gate for standard/reverse quizzes:
			// origins must have at least one etymology_freeform entry
			// AND at least one correct etymology answer. Skipped for
			// freeform mode, which is the entry point where new origins
			// are first encountered.
			if !skipEligibility && !isOriginEligible(learningHistories[etymID], nbTitle, o.Origin) {
				continue
			}
			// Soft SR check: skipped when includeUnstudied is true so
			// the user can still drill origins that are not due yet.
			if !includeUnstudied {
				if !needsOriginReview(learningHistories[etymID], nbTitle, o.Origin, notebook.QuizTypeEtymologyStandard) {
					continue
				}
			}

			cards = append(cards, EtymologyOriginCard{
				NotebookName:  etymID,
				NotebookTitle: nbTitle,
				Origin:        o.Origin,
				Type:          o.Type,
				Language:      o.Language,
				Meaning:       o.Meaning,
			})
		}
	}

	rand.Shuffle(len(cards), func(i, j int) {
		cards[i], cards[j] = cards[j], cards[i]
	})
	return cards, nil
}

// GradeEtymologyStandardAnswer grades a standard answer (origin -> meaning) using ValidateWordForm.
func (s *Service) GradeEtymologyStandardAnswer(
	ctx context.Context,
	card EtymologyOriginCard,
	answer string,
	responseTimeMs int64,
) (GradeResult, error) {
	validation, err := s.openaiClient.ValidateWordForm(ctx, inference.ValidateWordFormRequest{
		Expected:       card.Meaning,
		UserAnswer:     answer,
		Meaning:        card.Meaning,
		ResponseTimeMs: responseTimeMs,
	})
	if err != nil {
		return GradeResult{}, fmt.Errorf("validate word form: %w", err)
	}

	isCorrect := validation.Classification != inference.ClassificationWrong
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

// GradeEtymologyReverseAnswer grades a reverse answer (meaning -> origin) using ValidateWordForm.
func (s *Service) GradeEtymologyReverseAnswer(
	ctx context.Context,
	card EtymologyOriginCard,
	answer string,
	responseTimeMs int64,
) (GradeResult, error) {
	validation, err := s.openaiClient.ValidateWordForm(ctx, inference.ValidateWordFormRequest{
		Expected:       card.Origin,
		UserAnswer:     answer,
		Meaning:        card.Meaning,
		ResponseTimeMs: responseTimeMs,
	})
	if err != nil {
		return GradeResult{}, fmt.Errorf("validate word form: %w", err)
	}

	isCorrect := validation.Classification != inference.ClassificationWrong
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

// SaveEtymologyOriginResult updates the learning history for an etymology origin quiz answer.
func (s *Service) SaveEtymologyOriginResult(
	card EtymologyOriginCard,
	quality int,
	correct bool,
	responseTimeMs int64,
	quizType notebook.QuizType,
	isKnownWord bool,
) error {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return fmt.Errorf("failed to load learning histories: %w", err)
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[card.NotebookName], s.calculator)
	updater.UpdateOrCreateExpressionWithQualityForEtymology(
		card.NotebookName,
		card.NotebookTitle,
		"", // no scene for etymology origins
		card.Origin,
		card.Origin,
		correct,
		isKnownWord,
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

// GetEtymologyOriginNextReviewDates returns a map of lowercase origin -> next review date.
func (s *Service) GetEtymologyOriginNextReviewDates(cards []EtymologyOriginCard) (map[string]string, error) {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	result := make(map[string]string)
	for _, card := range cards {
		nextDate := originNextReviewDate(learningHistories[card.NotebookName], card)
		if nextDate != "" {
			result[strings.ToLower(card.Origin)] = nextDate
		}
	}
	return result, nil
}

func originNextReviewDate(histories []notebook.LearningHistory, card EtymologyOriginCard) string {
	for _, hist := range histories {
		if hist.Metadata.Title != card.NotebookTitle {
			continue
		}
		for _, expr := range hist.Expressions {
			if strings.EqualFold(expr.Expression, card.Origin) {
				return computeNextReviewDate(expr.EtymologyBreakdownLogs)
			}
		}
	}
	return ""
}

// LoadEtymologyNotebookSummaries returns etymology notebook summaries with due origin counts.
func (s *Service) LoadEtymologyNotebookSummaries() ([]NotebookSummary, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}

	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	var summaries []NotebookSummary
	etymIndexes := reader.GetEtymologyIndexes()
	for id, index := range etymIndexes {
		origins, err := reader.ReadEtymologyNotebook(id)
		if err != nil {
			continue
		}

		dueCount := 0
		for _, o := range origins {
			if !isOriginEligible(learningHistories[id], index.Name, o.Origin) {
				continue
			}
			if needsOriginReview(learningHistories[id], index.Name, o.Origin, notebook.QuizTypeEtymologyStandard) {
				dueCount++
			}
		}

		summaries = append(summaries, NotebookSummary{
			NotebookID:           id,
			Name:                 index.Name,
			EtymologyReviewCount: dueCount,
			Kind:                 "Etymology",
			LatestDate:           index.LatestDate,
		})
	}

	return summaries, nil
}

// isOriginEligible is the hard gate that must always pass for an origin to
// appear in etymology standard or reverse quizzes. The user must have (1)
// attempted the origin in etymology freeform mode at least once, and (2)
// answered at least one etymology question about it correctly. Both checks
// are enforced even when "include unstudied" is selected.
func isOriginEligible(
	histories []notebook.LearningHistory,
	notebookTitle, origin string,
) bool {
	for _, h := range histories {
		if h.Metadata.Title != notebookTitle {
			continue
		}
		for _, expr := range h.Expressions {
			if !strings.EqualFold(expr.Expression, origin) {
				continue
			}
			return expr.HasEtymologyFreeformAnswer() && expr.HasCorrectEtymologyAnswer()
		}
	}
	return false
}

// needsOriginReview checks whether an origin is DUE for review under the
// spaced-repetition schedule. Callers must first verify eligibility via
// isOriginEligible; this function assumes the origin has already cleared
// the freeform-first and has-correct-answer gates.
func needsOriginReview(
	histories []notebook.LearningHistory,
	notebookTitle, origin string,
	quizType notebook.QuizType,
) bool {
	for _, h := range histories {
		if h.Metadata.Title != notebookTitle {
			continue
		}
		for _, expr := range h.Expressions {
			if !strings.EqualFold(expr.Expression, origin) {
				continue
			}
			return expr.NeedsEtymologyReview(quizType)
		}
	}
	return false
}
