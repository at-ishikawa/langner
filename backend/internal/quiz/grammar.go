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

// LoadAllGrammarPosts loads every due grammar post for the user across ALL
// grammar notebooks (no section filter). The stateless submit path uses it to
// re-resolve a blank by its stable id on a cold/different instance.
func (s *Service) LoadAllGrammarPosts(userID int64) ([]GrammarPost, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("new reader: %w", err)
	}
	return s.grammarPosts(reader, userID, reader.GrammarStoryIDs(), nil)
}

// LoadGrammarPosts loads the story entries that have at least one due mistake,
// each with its due blanks. It merges each entry's prose (a story notebook) with
// its corrections (the grammars notebook) by entry title, and filters blanks by
// SM-2 (due when unseen or forward review is due). When entryTitles is non-empty
// only those entries (by title) are included; empty means every entry.
// GrammarMistakeRef is a correction's stable identity plus its notebook id —
// everything the server needs to recompute the correction's stable note_id for
// cross-instance Override / Skip / Resume resolution. Unlike LoadAllGrammarPosts
// (which drops not-yet-due and excluded corrections), this covers EVERY
// correction, because an override targets a mistake that was just answered and
// is therefore no longer due.
type GrammarMistakeRef struct {
	NotebookID string
	EntryID    string
	SenseID    string
	Incorrect  string
}

// AllGrammarMistakeRefs enumerates every grammar correction across all journals
// the user can see, due or not, for id-based resolution on a cold instance.
func (s *Service) AllGrammarMistakeRefs(userID int64) ([]GrammarMistakeRef, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("new reader: %w", err)
	}
	byNotebook, err := s.grammarMistakes(reader, userID, reader.GrammarStoryIDs(), nil)
	if err != nil {
		return nil, err
	}
	var refs []GrammarMistakeRef
	for _, id := range reader.GrammarStoryIDs() {
		for _, m := range byNotebook[id] {
			refs = append(refs, GrammarMistakeRef{
				NotebookID: id,
				EntryID:    m.EntryID,
				SenseID:    m.SenseID,
				Incorrect:  m.Incorrect,
			})
		}
	}
	return refs, nil
}

func (s *Service) LoadGrammarPosts(userID int64, notebookID string, entryTitles []string) ([]GrammarPost, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("newReader() > %w", err)
	}
	return s.grammarPosts(reader, userID, []string{notebookID}, entryTitles)
}

// entryTitleFilter returns the set of entryTitles, or nil (every entry) when
// there are none.
func entryTitleFilter(entryTitles []string) map[string]struct{} {
	if len(entryTitles) == 0 {
		return nil
	}
	filter := make(map[string]struct{}, len(entryTitles))
	for _, t := range entryTitles {
		filter[t] = struct{}{}
	}
	return filter
}

// visibleGrammarStories returns the notebooks of notebookIDs the user can see,
// with their story entries and learning histories, read together: one
// visibility lookup, one content load and one history read however many
// notebooks there are.
func (s *Service) visibleGrammarStories(reader *notebook.Reader, userID int64, notebookIDs []string) ([]string, map[string][]notebook.StoryNotebook, map[string][]notebook.LearningHistory, error) {
	// A grammar notebook the user can't see yields nothing (auth Phase 3).
	// Gating here covers the grammar quiz and the Relearn enumeration in
	// relearnGrammarIndex, so neither leaks a private notebook.
	visible, err := s.visibleNotebooks(userID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve notebook visibility: %w", err)
	}
	ids := visibleIDs(notebookIDs, visible)
	if len(ids) == 0 {
		return nil, nil, nil, nil
	}
	if err := reader.Preload(ids...); err != nil {
		return nil, nil, nil, err
	}
	stories := make(map[string][]notebook.StoryNotebook, len(ids))
	for _, id := range ids {
		sn, err := reader.ReadStoryNotebooks(id)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("ReadStoryNotebooks(%s) > %w", id, err)
		}
		stories[id] = sn
	}
	histories, err := s.loadHistoriesForNotebooks(userID, ids...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loadHistories() > %w", err)
	}
	return ids, stories, histories, nil
}

// grammarPosts is LoadGrammarPosts for several notebooks at once.
func (s *Service) grammarPosts(reader *notebook.Reader, userID int64, notebookIDs []string, entryTitles []string) ([]GrammarPost, error) {
	entryFilter := entryTitleFilter(entryTitles)
	ids, storiesByID, learningHistories, err := s.visibleGrammarStories(reader, userID, notebookIDs)
	if err != nil {
		return nil, err
	}
	posts := make([]GrammarPost, 0)
	for _, notebookID := range ids {
		name := notebookID
		if index, ok := reader.GetStoryIndexes()[notebookID]; ok && index.Name != "" {
			name = index.Name
		}
		expByMistake := grammarExpressionsByID(learningHistories[notebookID])
		for _, sn := range storiesByID[notebookID] {
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
					id := notebook.CorrectionID(notebookID, sn.Event, sceneIdx, seq+1, c)
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
	}
	return posts, nil
}

// GrammarMistake is one grammar correction as a standalone review row. Unlike
// GrammarBlank (a live-session blank keyed by an ephemeral note_id), it carries
// the stable identity (SenseID, resolved against the journal's notebook id) and
// the full correct/reason text a review surface renders, plus IsExcluded so the
// row can show an "Excluded" state with a Resume action.
type GrammarMistake struct {
	SenseID    string
	EntryID    string
	Title      string
	Incorrect  string
	Correct    string
	Category   string
	Reason     string
	Status     string
	IsExcluded bool
}

// LoadGrammarMistakes enumerates ALL grammar corrections for a journal —
// both currently-due and already-excluded — for the standalone mistake review
// surface. It is the sibling of LoadGrammarPosts (which drops excluded and
// not-yet-due corrections via grammarMistakeDue); this one keeps every
// correction and marks IsExcluded from the SAME grammar skipped_at the quiz
// path filters on, so a mistake excluded here disappears from the live quiz and
// Relearn pool and reappears on Resume. When entryTitles is non-empty only
// those entries (by title) are included; empty means every entry.
func (s *Service) LoadGrammarMistakes(userID int64, notebookID string, entryTitles []string) ([]GrammarMistake, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("newReader() > %w", err)
	}
	byNotebook, err := s.grammarMistakes(reader, userID, []string{notebookID}, entryTitles)
	if err != nil {
		return nil, err
	}
	if byNotebook[notebookID] == nil {
		return []GrammarMistake{}, nil
	}
	return byNotebook[notebookID], nil
}

// grammarMistakes is LoadGrammarMistakes for several notebooks at once, keyed
// by notebook id.
func (s *Service) grammarMistakes(reader *notebook.Reader, userID int64, notebookIDs []string, entryTitles []string) (map[string][]GrammarMistake, error) {
	entryFilter := entryTitleFilter(entryTitles)
	ids, storiesByID, learningHistories, err := s.visibleGrammarStories(reader, userID, notebookIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]GrammarMistake, len(ids))
	for _, notebookID := range ids {
		expByMistake := grammarExpressionsByID(learningHistories[notebookID])
		mistakes := make([]GrammarMistake, 0)
		for _, sn := range storiesByID[notebookID] {
			if entryFilter != nil {
				if _, ok := entryFilter[sn.Event]; !ok {
					continue
				}
			}
			for sceneIdx := range sn.Scenes {
				corrections := reader.CorrectionsForScene(notebookID, sn.Event, sceneIdx)
				for seq, c := range corrections {
					id := notebook.CorrectionID(notebookID, sn.Event, sceneIdx, seq+1, c)
					exp, seen := expByMistake[id]
					status := string(notebook.LearnedStatusLearning)
					if seen {
						status = string(exp.GetLatestStatus())
					}
					mistakes = append(mistakes, GrammarMistake{
						SenseID:    id,
						EntryID:    fmt.Sprintf("%s#%d", sn.Event, sceneIdx),
						Title:      sn.Event,
						Incorrect:  c.Incorrect,
						Correct:    c.Correct,
						Category:   c.Category,
						Reason:     c.Reason,
						Status:     status,
						IsExcluded: seen && exp.SkippedAt.IsSkipped(notebook.QuizTypeGrammar),
					})
				}
			}
		}
		out[notebookID] = mistakes
	}
	return out, nil
}

// grammarMistakeDue reports whether a mistake is due for review: it is due when
// it has no learning history yet (seen == false) or its SM-2 forward review is
// due. A correction the learner deliberately Excluded from the grammar quiz
// (skipped_at set via SkipWord) is never due — this is the loader-side filter
// that makes Exclude remove the correction from both the live grammar quiz and
// the Relearn pool (see .claude/rules/quiz-ui-invariants.md).
func grammarMistakeDue(exp notebook.LearningHistoryExpression, seen bool) bool {
	if seen && exp.SkippedAt.IsSkipped(notebook.QuizTypeGrammar) {
		return false
	}
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
func (s *Service) LoadGrammarStorySummaries(userID int64) ([]NotebookSummary, error) {
	reader, err := s.newSummaryReader()
	if err != nil {
		return nil, fmt.Errorf("newReader() > %w", err)
	}
	return s.grammarStorySummaries(reader, userID)
}

// grammarStorySummaries is LoadGrammarStorySummaries over a given reader.
func (s *Service) grammarStorySummaries(reader *notebook.Reader, userID int64) ([]NotebookSummary, error) {
	learningHistories, err := s.loadHistoriesForNotebooks(userID, reader.GrammarStoryIDs()...)
	if err != nil {
		return nil, fmt.Errorf("loadHistories() > %w", err)
	}
	visible, err := s.visibleNotebooks(userID)
	if err != nil {
		return nil, fmt.Errorf("resolve notebook visibility: %w", err)
	}
	storyIndexes := reader.GetStoryIndexes()

	var summaries []NotebookSummary
	for _, id := range reader.GrammarStoryIDs() {
		if !visible(id) {
			continue
		}
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
					exp, seen := expByMistake[notebook.CorrectionID(id, sn.Event, sceneIdx, seq+1, c)]
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
		// An empty answer can never be the correction — grade it wrong
		// deterministically (no LLM call). This is the "unanswered → incorrect"
		// path: revealing answers for a blank the learner never typed records
		// the same miss a wrong typed answer would, never a skip.
		return GradeResult{
			Correct: false,
			Reason:  "No answer provided.",
			Quality: int(notebook.QualityWrong),
		}, true
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

// GradeGrammarBlank grades one grammar-correction blank.
// userID selects whose LLM credential backs the grade (resolved per call).
func (s *Service) GradeGrammarBlank(ctx context.Context, userID int64, content string, blank GrammarBlank, answer string, responseTimeMs int64) (GradeResult, error) {
	// Fast path: most answers match the known reference correction, so grade
	// them deterministically and skip the LLM entirely. Only answers that
	// differ (a possible valid alternative) pay for a model call.
	if result, ok := deterministicGrammarGrade(answer, blank.Correct, blank.Incorrect); ok {
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, interactiveGradeTimeout)
	defer cancel()
	client, err := s.clientResolver.ResolveClient(ctx, userID)
	if err != nil {
		return GradeResult{}, err
	}
	response, err := client.GradeCorrection(ctx, inference.GradeCorrectionRequest{
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
func (s *Service) SaveGrammarBlank(ctx context.Context, userID int64, notebookID, senseID string, result GradeResult, responseTimeMs int64) error {
	_, _, err := s.SaveGrammarBlankInfo(ctx, userID, notebookID, senseID, result, responseTimeMs)
	return err
}

// SaveGrammarBlankInfo is SaveGrammarBlank that also returns the learnedAt/
// nextReviewDate it persisted (see SaveResultInfo).
func (s *Service) SaveGrammarBlankInfo(ctx context.Context, userID int64, notebookID, senseID string, result GradeResult, responseTimeMs int64) (learnedAt, nextReviewDate string, err error) {
	status := "misunderstood"
	if result.Correct {
		status = "understood"
	}
	log := &learning.LearningLog{
		UserID:           userID,
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
	log.IntervalDays = s.nextIntervalDays(ctx, userID, notebookID, senseID, notebook.QuizTypeGrammar, result.Correct, result.Quality, responseTimeMs, log.LearnedAt, senseID)
	if err := s.learningRepository.Create(ctx, log); err != nil {
		return "", "", fmt.Errorf("save grammar learning log for %q: %w", notebookID, err)
	}
	la, nr := formatLearnedInfo(log.LearnedAt, log.IntervalDays)
	return la, nr, nil
}
