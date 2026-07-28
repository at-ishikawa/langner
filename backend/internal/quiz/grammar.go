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

// GrammarPost is one journal post shown in full, carrying the mistakes that are
// currently due to fix inline. Content is the whole post; each Blank is a
// mistake within it.
type GrammarPost struct {
	NotebookID   string
	NotebookName string
	EntryID      string
	Title        string
	Content      string
	Blanks       []GrammarBlank
}

// GrammarBlank is one mistake to correct within a post. Correct is the
// reference fix used only for grading (never sent to the client).
type GrammarBlank struct {
	SenseID   string // stable correction id
	Incorrect string
	Correct   string
	Category  string
	Reason    string
	Line      int
	Status    string
}

// LoadGrammarPosts loads the journal posts that have at least one due mistake,
// each with its due blanks. It merges each post's prose (journal notebook) with
// its corrections (journal-corrections notebook) by post id, and filters blanks
// by SM-2 (due when unseen or forward review is due).
func (s *Service) LoadGrammarPosts(notebookID string) ([]GrammarPost, error) {
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

	posts := make([]GrammarPost, 0)
	for _, entry := range entries {
		set, ok := correctionsByPost[entry.ID]
		if !ok {
			continue
		}
		blanks := make([]GrammarBlank, 0, len(set.Corrections))
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
			blanks = append(blanks, GrammarBlank{
				SenseID:   id,
				Incorrect: c.Incorrect,
				Correct:   c.Correct,
				Category:  c.Category,
				Reason:    c.Reason,
				Line:      c.Line,
				Status:    status,
			})
		}
		if len(blanks) == 0 {
			continue
		}
		posts = append(posts, GrammarPost{
			NotebookID:   notebookID,
			NotebookName: name,
			EntryID:      entry.ID,
			Title:        entry.Title,
			Content:      strings.TrimRight(entry.Text, "\n"),
			Blanks:       blanks,
		})
	}
	return posts, nil
}

// grammarMistakeDue reports whether a mistake is due for review: it is due when
// it has no learning history yet (seen == false) or its SM-2 forward review is
// due.
func grammarMistakeDue(exp notebook.LearningHistoryExpression, seen bool) bool {
	return !seen || exp.NeedsForwardReview()
}

// grammarExpressionsByID indexes a journal notebook's flat learning history by
// correction id (falling back to the expression for legacy entries).
func grammarExpressionsByID(histories []notebook.LearningHistory) map[string]notebook.LearningHistoryExpression {
	result := make(map[string]notebook.LearningHistoryExpression)
	for _, h := range histories {
		if h.Metadata.Type != string(notebook.QuizTypeGrammar) {
			continue
		}
		for _, exp := range h.Expressions {
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
// notebook, with GrammarReviewCount set to the number of mistakes currently due.
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
			// quiz-options page (which lists every notebook kind).
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

// GradeGrammarBlank grades a user's correction of one blank, using the full
// post as grading context.
func (s *Service) GradeGrammarBlank(ctx context.Context, content string, blank GrammarBlank, answer string, responseTimeMs int64) (GradeResult, error) {
	response, err := s.openaiClient.GradeCorrection(ctx, inference.GradeCorrectionRequest{
		Sentence:       content,
		Incorrect:      blank.Incorrect,
		Correct:        blank.Correct,
		UserAnswer:     answer,
		Note:           blank.Reason,
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

// SaveGrammarBlank records the grade for one blank in the notebook's learning
// history, keyed by the correction id under the flat "journal" bucket.
func (s *Service) SaveGrammarBlank(ctx context.Context, notebookID, senseID string, result GradeResult, responseTimeMs int64) error {
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
		SourceNotebookID: notebookID,
		NotebookName:     notebookID,
		StoryTitle:       notebook.JournalStoryTitle,
		Expression:       senseID,
		SenseID:          senseID,
		IsCorrect:        result.Correct,
		LearningNotesDir: s.notebooksConfig.LearningNotesDirectory,
	}
	if err := s.learningRepository.Create(ctx, log); err != nil {
		return fmt.Errorf("save grammar learning log for %q: %w", notebookID, err)
	}
	return nil
}
