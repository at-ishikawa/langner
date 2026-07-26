package quiz

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// GrammarCard is a single grammar-correction quiz card. Content is the full
// journal post shown to the user, Incorrect is the span to fix within it, and
// Correct is the reference answer used only for grading. The mistake lives in
// a separate corrections notebook, merged with the post by id.
type GrammarCard struct {
	NotebookID   string
	NotebookName string
	EntryID      string // journal post id
	MistakeID    string // stable correction id
	Content      string // full post text (the frontend highlights Incorrect in it)
	Incorrect    string
	Correct      string
	Category     string
	Reason       string
	Line         int
	Status       string
}

// LoadGrammarCards loads the due grammar-correction cards for a journal
// notebook. It merges each post's prose with its corrections (by post id) and
// emits one card per correction. A correction is due when it has no learning
// history yet or its SM-2 forward review is due.
func (s *Service) LoadGrammarCards(notebookID string) ([]GrammarCard, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("newReader() > %w", err)
	}
	entries, err := reader.ReadJournalEntries(notebookID)
	if err != nil {
		return nil, fmt.Errorf("ReadJournalEntries(%s) > %w", notebookID, err)
	}
	correctionsByPost, err := reader.ReadJournalCorrections(notebookID)
	if err != nil {
		return nil, fmt.Errorf("ReadJournalCorrections(%s) > %w", notebookID, err)
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
	for _, entry := range entries {
		set, ok := correctionsByPost[entry.ID]
		if !ok {
			continue
		}
		content := strings.TrimRight(entry.Text, "\n")
		perLine := make(map[int]int)
		for _, c := range set.Corrections {
			perLine[c.Line]++
			id := c.DerivedID(entry.ID, perLine[c.Line])
			exp, seen := expByMistake[id]
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
				MistakeID:    id,
				Content:      content,
				Incorrect:    c.Incorrect,
				Correct:      c.Correct,
				Category:     c.Category,
				Reason:       c.Reason,
				Line:         c.Line,
				Status:       status,
			})
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
			// Post note-id-identity, a grammar entry is keyed by its stable ID
			// (the mistake id). Fall back to Expression for any legacy entry
			// written before ids were stamped.
			key := exp.ID
			if key == "" {
				key = exp.Expression
			}
			result[key] = exp
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
		entries, err := reader.ReadJournalEntries(id)
		if err != nil {
			// A single malformed journal notebook must not take down the whole
			// quiz-options page (which lists every notebook kind). Skip it with
			// a warning; `langner validate` surfaces the underlying problem.
			slog.Warn("skipping journal notebook in summaries", "notebook", id, "error", err)
			continue
		}
		correctionsByPost, err := reader.ReadJournalCorrections(id)
		if err != nil {
			slog.Warn("skipping journal corrections in summaries", "notebook", id, "error", err)
			continue
		}
		expByMistake := grammarExpressionsByID(learningHistories[id])

		count := 0
		var latestDate time.Time
		for _, entry := range entries {
			if entry.Date.After(latestDate) {
				latestDate = entry.Date
			}
			set, ok := correctionsByPost[entry.ID]
			if !ok {
				continue
			}
			perLine := make(map[int]int)
			for _, c := range set.Corrections {
				perLine[c.Line]++
				exp, seen := expByMistake[c.DerivedID(entry.ID, perLine[c.Line])]
				if grammarMistakeDue(exp, seen) {
					count++
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
		Sentence:       card.Content,
		Incorrect:      card.Incorrect,
		Correct:        card.Correct,
		UserAnswer:     answer,
		Note:           card.Reason,
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
		SenseID:          card.MistakeID,
		IsCorrect:        result.Correct,
		LearningNotesDir: s.notebooksConfig.LearningNotesDirectory,
	}
	if err := s.learningRepository.Create(ctx, log); err != nil {
		return fmt.Errorf("save grammar learning log for %q: %w", card.NotebookID, err)
	}
	return nil
}
