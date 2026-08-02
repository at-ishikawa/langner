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

// GrammarPost is one story entry shown in full, carrying the mistakes that are
// currently due to fix inline. Content is the whole entry's text; each Blank is
// a mistake within it.
type GrammarPost struct {
	NotebookID   string
	NotebookName string
	EntryID      string
	Title        string
	Content      string
	Blanks       []GrammarBlank
}

// GrammarBlank is one mistake to correct within an entry. Correct is the
// reference fix used only for grading (never sent to the client).
type GrammarBlank struct {
	SenseID   string // stable correction id
	Incorrect string
	Correct   string
	Category  string
	Reason    string
	Status    string
}

// correctionID returns a correction's stable spaced-repetition id: its explicit
// id when set, otherwise one derived from the story id, entry title, scene
// index, and order.
func correctionID(storyID, title string, sceneIndex, seq int, c notebook.Correction) string {
	if strings.TrimSpace(c.ID) != "" {
		return c.ID
	}
	return notebook.DerivedCorrectionID(storyID, title, sceneIndex, seq)
}

// LoadGrammarPosts loads the story entries that have at least one due mistake,
// each with its due blanks. It merges each entry's prose (a story notebook) with
// its corrections (the grammars notebook) by entry title, and filters blanks by
// SM-2 (due when unseen or forward review is due). When entryTitles is non-empty
// only those entries (by title) are included; empty means every entry.
func (s *Service) LoadGrammarPosts(notebookID string, entryTitles []string) ([]GrammarPost, error) {
	var entryFilter map[string]struct{}
	if len(entryTitles) > 0 {
		entryFilter = make(map[string]struct{}, len(entryTitles))
		for _, t := range entryTitles {
			entryFilter[t] = struct{}{}
		}
	}
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("newReader() > %w", err)
	}
	stories, err := reader.ReadStoryNotebooks(notebookID)
	if err != nil {
		return nil, fmt.Errorf("ReadStoryNotebooks(%s) > %w", notebookID, err)
	}

	name := notebookID
	if index, ok := reader.GetStoryIndexes()[notebookID]; ok && index.Name != "" {
		name = index.Name
	}

	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("NewLearningHistories() > %w", err)
	}
	expByMistake := grammarExpressionsByID(learningHistories[notebookID])

	posts := make([]GrammarPost, 0)
	for _, sn := range stories {
		if entryFilter != nil {
			if _, ok := entryFilter[sn.Event]; !ok {
				continue
			}
		}
		for sceneIdx, scene := range sn.Scenes {
			corrections := reader.CorrectionsForScene(notebookID, sn.Event, sceneIdx)
			if len(corrections) == 0 {
				continue
			}
			blanks := make([]GrammarBlank, 0, len(corrections))
			for seq, c := range corrections {
				id := correctionID(notebookID, sn.Event, sceneIdx, seq+1, c)
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
					Status:    status,
				})
			}
			if len(blanks) == 0 {
				continue
			}
			title := sn.Event
			if scene.Title != "" {
				title = fmt.Sprintf("%s · %s", sn.Event, scene.Title)
			}
			posts = append(posts, GrammarPost{
				NotebookID:   notebookID,
				NotebookName: name,
				EntryID:      fmt.Sprintf("%s#%d", sn.Event, sceneIdx),
				Title:        title,
				Content:      notebook.StorySceneText(scene),
				Blanks:       blanks,
			})
		}
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

// LoadGrammarStorySummaries returns one NotebookSummary per story that has a
// grammars notebook, with GrammarReviewCount set to the number of mistakes
// currently due.
func (s *Service) LoadGrammarStorySummaries() ([]NotebookSummary, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("newReader() > %w", err)
	}
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("NewLearningHistories() > %w", err)
	}
	storyIndexes := reader.GetStoryIndexes()

	var summaries []NotebookSummary
	for _, id := range reader.GrammarStoryIDs() {
		stories, err := reader.ReadStoryNotebooks(id)
		if err != nil {
			// A single malformed notebook must not take down the whole
			// quiz-options page (which lists every notebook kind).
			slog.Warn("skipping grammar story in summaries", "notebook", id, "error", err)
			continue
		}
		expByMistake := grammarExpressionsByID(learningHistories[id])

		count := 0
		var latestDate time.Time
		var sections []NotebookSectionSummary
		for _, sn := range stories {
			if sn.Date.After(latestDate) {
				latestDate = sn.Date
			}
			entryDue := 0
			for sceneIdx := range sn.Scenes {
				for seq, c := range reader.CorrectionsForScene(id, sn.Event, sceneIdx) {
					exp, seen := expByMistake[correctionID(id, sn.Event, sceneIdx, seq+1, c)]
					if grammarMistakeDue(exp, seen) {
						entryDue++
					}
				}
			}
			count += entryDue
			// One section per entry (e.g. w16, w17) so the start screen can
			// narrow to individual entries within a journal.
			if entryDue > 0 {
				sections = append(sections, NotebookSectionSummary{Title: sn.Event, GrammarReviewCount: entryDue})
			}
		}

		name := id
		if index, ok := storyIndexes[id]; ok && index.Name != "" {
			name = index.Name
		}
		summaries = append(summaries, NotebookSummary{
			NotebookID:         id,
			Name:               name,
			GrammarReviewCount: count,
			Kind:               "Grammar",
			LatestDate:         latestDate,
			Sections:           sections,
		})
	}
	return summaries, nil
}

// GradeGrammarBlank grades a user's correction of one blank, using the full
// post as grading context.
// normalizeCorrection lowercases, trims surrounding quotes/punctuation, and
// collapses internal whitespace so a typed answer can be compared to the
// reference correction without an LLM round-trip.
func normalizeCorrection(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Trim(s, ` .,!?;:"'`)
	return strings.Join(strings.Fields(s), " ")
}

// deterministicGrammarGrade decides a correction WITHOUT calling the model when
// the answer plainly applies the reference fix — it matches the correction (or
// contains it) and no longer contains the original mistake. It only ever
// returns a *correct* verdict: an answer that doesn't match may still be a valid
// alternative phrasing, so those fall through to the LLM grader. Returns
// (result, true) when decided, (zero, false) when the LLM should judge.
func deterministicGrammarGrade(answer, correct, incorrect string) (GradeResult, bool) {
	na := normalizeCorrection(answer)
	if na == "" {
		return GradeResult{}, false
	}
	nc := normalizeCorrection(correct)
	ni := normalizeCorrection(incorrect)
	matchesFix := na == nc || (nc != "" && strings.Contains(na, nc))
	stillWrong := ni != "" && strings.Contains(na, ni)
	if matchesFix && !stillWrong {
		return GradeResult{
			Correct: true,
			Reason:  "Matches the correction.",
			Quality: int(notebook.QualityCorrect),
		}, true
	}
	return GradeResult{}, false
}

func (s *Service) GradeGrammarBlank(ctx context.Context, content string, blank GrammarBlank, answer string, responseTimeMs int64) (GradeResult, error) {
	// Fast path: most answers match the known reference correction, so grade
	// them deterministically and skip the LLM entirely. Only answers that
	// differ (a possible valid alternative) pay for a model call.
	if result, ok := deterministicGrammarGrade(answer, blank.Correct, blank.Incorrect); ok {
		return result, nil
	}
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
