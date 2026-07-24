package quiz

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// GrammarCard is a single grammar-correction quiz card built from a journal
// mistake. Sentence is the entry text shown to the user, Incorrect is the span
// to fix, and Correct is the reference answer used only for grading.
type GrammarCard struct {
	NotebookID   string
	NotebookName string
	EntryID      string
	MistakeID    string
	Sentence     string
	Incorrect    string
	Correct      string
	Category     string
	Note         string
	Status       string
}

// LoadGrammarCards loads the due grammar-correction cards for a journal
// notebook. A mistake is due when it has no learning history yet or its SM-2
// forward review is due.
func (s *Service) LoadGrammarCards(notebookID string) ([]GrammarCard, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("newReader() > %w", err)
	}
	notebooks, err := reader.ReadJournalNotebooks(notebookID)
	if err != nil {
		return nil, fmt.Errorf("ReadJournalNotebooks(%s) > %w", notebookID, err)
	}

	name := notebookID
	if index, ok := reader.GetJournalIndexes()[notebookID]; ok && index.Name != "" {
		name = index.Name
	}

	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("NewLearningHistories() > %w", err)
	}
	expByMistake := grammarExpressionsByID(learningHistories[notebookID])

	cards := make([]GrammarCard, 0)
	for _, nb := range notebooks {
		for _, entry := range nb.Entries {
			for _, mistake := range entry.Mistakes {
				exp, seen := expByMistake[mistake.ID]
				if !grammarMistakeDue(exp, seen) {
					continue
				}
				status := string(notebook.LearnedStatusLearning)
				if seen {
					status = string(exp.GetLatestStatus())
				}
				cards = append(cards, GrammarCard{
					NotebookID:   notebookID,
					NotebookName: name,
					EntryID:      entry.ID,
					MistakeID:    mistake.ID,
					Sentence:     strings.TrimSpace(entry.Text),
					Incorrect:    mistake.Incorrect,
					Correct:      mistake.Correct,
					Category:     mistake.Category,
					Note:         mistake.Note,
					Status:       status,
				})
			}
		}
	}
	return cards, nil
}

// grammarMistakeDue reports whether a mistake is due for review: it is due when
// it has no learning history yet (seen == false) or its SM-2 forward review is
// due.
func grammarMistakeDue(exp notebook.LearningHistoryExpression, seen bool) bool {
	return !seen || exp.NeedsForwardReview()
}

// grammarExpressionsByID indexes a journal notebook's flat learning history by
// mistake id.
func grammarExpressionsByID(histories []notebook.LearningHistory) map[string]notebook.LearningHistoryExpression {
	result := make(map[string]notebook.LearningHistoryExpression)
	for _, h := range histories {
		if h.Metadata.Type != string(notebook.QuizTypeGrammar) {
			continue
		}
		for _, exp := range h.Expressions {
			result[exp.Expression] = exp
		}
	}
	return result
}

// LoadJournalNotebookSummaries returns one NotebookSummary per journal
// notebook, with GrammarReviewCount set to the number of mistakes currently due
// for the grammar quiz. Kind is "Journal" so the frontend can group these
// separately from vocabulary and etymology notebooks.
func (s *Service) LoadJournalNotebookSummaries() ([]NotebookSummary, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("newReader() > %w", err)
	}
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("NewLearningHistories() > %w", err)
	}

	var summaries []NotebookSummary
	for id, index := range reader.GetJournalIndexes() {
		notebooks, err := reader.ReadJournalNotebooks(id)
		if err != nil {
			return nil, fmt.Errorf("ReadJournalNotebooks(%s) > %w", id, err)
		}
		expByMistake := grammarExpressionsByID(learningHistories[id])

		count := 0
		var latestDate time.Time
		for _, nb := range notebooks {
			if nb.Date.After(latestDate) {
				latestDate = nb.Date
			}
			for _, entry := range nb.Entries {
				for _, mistake := range entry.Mistakes {
					exp, seen := expByMistake[mistake.ID]
					if grammarMistakeDue(exp, seen) {
						count++
					}
				}
			}
		}

		name := index.Name
		if name == "" {
			name = id
		}
		summaries = append(summaries, NotebookSummary{
			NotebookID:         id,
			Name:               name,
			GrammarReviewCount: count,
			Kind:               "Journal",
			LatestDate:         latestDate,
		})
	}
	return summaries, nil
}

// GradeGrammarAnswer grades a user's correction of a journal mistake.
func (s *Service) GradeGrammarAnswer(ctx context.Context, card GrammarCard, answer string, responseTimeMs int64) (GradeResult, error) {
	response, err := s.openaiClient.GradeCorrection(ctx, inference.GradeCorrectionRequest{
		Sentence:       card.Sentence,
		Incorrect:      card.Incorrect,
		Correct:        card.Correct,
		UserAnswer:     answer,
		Note:           card.Note,
		ResponseTimeMs: responseTimeMs,
	})
	if err != nil {
		return GradeResult{}, fmt.Errorf("GradeCorrection() > %w", err)
	}
	return GradeResult{
		Correct: response.Correct,
		Reason:  response.Reason,
		Quality: response.Quality,
	}, nil
}

// SaveGrammarResult records the grade in the journal notebook's learning
// history, keyed by mistake id under the flat "journal" bucket.
func (s *Service) SaveGrammarResult(ctx context.Context, card GrammarCard, result GradeResult, responseTimeMs int64) error {
	status := "misunderstood"
	if result.Correct {
		status = "understood"
	}
	log := &learning.LearningLog{
		Status:           status,
		LearnedAt:        time.Now(),
		Quality:          result.Quality,
		ResponseTimeMs:   int(responseTimeMs),
		QuizType:         string(notebook.QuizTypeGrammar),
		SourceNotebookID: card.NotebookID,
		NotebookName:     card.NotebookID,
		StoryTitle:       notebook.JournalStoryTitle,
		Expression:       card.MistakeID,
		IsCorrect:        result.Correct,
		LearningNotesDir: s.notebooksConfig.LearningNotesDirectory,
	}
	if err := s.learningRepository.Create(ctx, log); err != nil {
		return fmt.Errorf("save grammar learning log for %q: %w", card.NotebookID, err)
	}
	return nil
}
