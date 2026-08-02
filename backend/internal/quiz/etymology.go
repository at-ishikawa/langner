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

// EtymologyFamilyWord is one derived word shown on an origin card. The user
// types Meaning; Expression is the word displayed as context.
type EtymologyFamilyWord struct {
	Expression string
	Meaning    string
}

// EtymologyOriginCard is one screen of the etymology-origin quiz: a single
// (origin, sense) shown together with the full session-scoped family of words
// that derive from it. The user types each family word's meaning while the
// origin and the whole family stay visible as context. The origin carries
// exactly one learning-log series (invariants L1/L4) — the family words are
// context and per-word feedback, not separate log series.
//
// The full key is (NotebookName, SessionTitle, Sense, Origin): SessionTitle
// disambiguates the same origin string across sessions, Sense disambiguates
// same-session multi-sense origins.
type EtymologyOriginCard struct {
	NotebookName  string
	NotebookTitle string
	SessionTitle  string
	Sense         string
	Origin        string
	Type          string
	Language      string
	Meaning       string
	// Forms records inflectional / morphological variants of the origin
	// (e.g. the Latin principal parts), shown in the origin header.
	Forms []notebook.EtymologyOriginForm
	// Words is the full session-scoped family of derived words the user is
	// asked to give meanings for.
	Words []EtymologyFamilyWord
}

// originDedupKey returns the canonical key used to deduplicate etymology
// origins within one (session, sense): the trimmed, lowercased origin.
func originDedupKey(origin string) string {
	return strings.ToLower(strings.TrimSpace(origin))
}

// LoadEtymologyOriginCards loads one card per (origin, sense) from the selected
// etymology notebooks, each carrying the full session-scoped family of derived
// words. Origins with no derived words in their session are omitted — there is
// nothing to type on such a screen.
//
// When skipEligibility is true the SR-due gate is skipped (used by the relearn
// pool, which enumerates every origin regardless of schedule).
//
// sessionTitlesByID narrows the result to specific sessions per notebook; a
// nil/empty list for a notebook means "all sessions".
func (s *Service) LoadEtymologyOriginCards(
	etymologyNotebookIDs []string,
	includeUnstudied bool,
	skipEligibility bool,
	sessionTitlesByID map[string][]string,
) ([]EtymologyOriginCard, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	families := buildOriginFamilies(reader)
	etymIndexes := reader.GetEtymologyIndexes()

	seen := make(map[string]bool)
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
		sessionFilter := sessionTitlesByID[etymID]

		for _, o := range origins {
			if !inSectionFilter(sessionFilter, o.SessionTitle) {
				continue
			}
			key := etymID + "\x00" + o.SessionTitle + "\x00" + o.Sense + "\x00" + originDedupKey(o.Origin)
			if seen[key] {
				continue
			}
			seen[key] = true

			words := families[originFamilyKey(etymID, o.SessionTitle, o.Sense, o.Origin)]
			if len(words) == 0 {
				continue
			}
			if !skipEligibility && !shouldIncludeOrigin(
				learningHistories[etymID], o.SessionTitle, o.Origin, o.Sense, includeUnstudied,
			) {
				continue
			}

			cards = append(cards, EtymologyOriginCard{
				NotebookName:  etymID,
				NotebookTitle: nbTitle,
				SessionTitle:  o.SessionTitle,
				Sense:         o.Sense,
				Origin:        o.Origin,
				Type:          o.Type,
				Language:      o.Language,
				Meaning:       o.Meaning,
				Forms:         o.Forms,
				Words:         words,
			})
		}
	}

	if !s.disableShuffle {
		rand.Shuffle(len(cards), func(i, j int) {
			cards[i], cards[j] = cards[j], cards[i]
		})
	}
	return cards, nil
}

// originFamilyKey is the canonical index key for an origin's derived-word
// family: (notebookID, session, sense, lower(origin)).
func originFamilyKey(notebookID, sessionTitle, sense, origin string) string {
	return notebookID + "\x00" + sessionTitle + "\x00" + sense + "\x00" + strings.ToLower(strings.TrimSpace(origin))
}

// buildOriginFamilies scans every definitions notebook once and returns, per
// (notebookID, session, sense, origin), the full list of derived words (with
// meanings) whose origin_parts reference that origin sense. This is the
// uncapped, session-scoped generalisation of the old example-word helper.
//
// A definition's origin_parts ref pins a sense via ref.Sense; the family key
// includes that sense, so a card for a specific sense only collects the words
// whose ref names that sense (or the empty sense for single-sense origins).
func buildOriginFamilies(reader *notebook.Reader) map[string][]EtymologyFamilyWord {
	result := make(map[string][]EtymologyFamilyWord)
	for _, bookID := range reader.GetDefinitionsBookIDs() {
		defs, ok := reader.GetDefinitionsNotes(bookID)
		if !ok {
			continue
		}
		for sessionTitle, sceneDefs := range defs {
			for _, notes := range sceneDefs {
				for _, note := range notes {
					expr := note.Expression
					if expr == "" {
						expr = note.Definition
					}
					if expr == "" {
						continue
					}
					word := EtymologyFamilyWord{Expression: expr, Meaning: note.Meaning}
					for _, ref := range note.OriginParts {
						key := originFamilyKey(bookID, sessionTitle, ref.Sense, ref.Origin)
						result[key] = append(result[key], word)
					}
				}
			}
		}
	}
	return result
}

// shouldIncludeOrigin reports whether an origin sense must appear in the
// etymology quiz for the given toggle. It is the single source of truth used
// by both LoadEtymologyOriginCards and LoadEtymologyNotebookSummaries so the
// count badge and the quiz can never disagree.
//
//   - skipped → exclude.
//   - never-seen (no origin logs) → include iff includeUnstudied.
//   - has logs → defer to the SR interval.
func shouldIncludeOrigin(
	histories []notebook.LearningHistory,
	sessionTitle, origin, sense string,
	includeUnstudied bool,
) bool {
	expr := notebook.FindOriginExpression(histories, sessionTitle, origin, sense)
	if expr == nil {
		return includeUnstudied
	}
	if expr.SkippedAt.IsSkipped(notebook.QuizTypeEtymologyOrigin) {
		return false
	}
	return expr.NeedsEtymologyReview(notebook.QuizTypeEtymologyOrigin)
}

// GradeEtymologyWordAnswer grades one family word's typed meaning against the
// word's recorded meaning. Exact case-insensitive matches short-circuit OpenAI.
func (s *Service) GradeEtymologyWordAnswer(
	ctx context.Context,
	word EtymologyFamilyWord,
	answer string,
	responseTimeMs int64,
) (GradeResult, error) {
	if isExactMatch(answer, word.Meaning) {
		return exactMatchResult(responseTimeMs), nil
	}
	validation, err := s.openaiClient.ValidateWordForm(ctx, inference.ValidateWordFormRequest{
		Expected:       word.Meaning,
		UserAnswer:     answer,
		Meaning:        word.Meaning,
		ResponseTimeMs: responseTimeMs,
	})
	if err != nil {
		return GradeResult{}, fmt.Errorf("validate word form: %w", err)
	}
	isCorrect := validation.Classification != inference.ClassificationWrong
	return GradeResult{
		Correct:        isCorrect,
		Reason:         validation.Reason,
		Quality:        qualityFromResponseTime(isCorrect, responseTimeMs),
		Classification: string(validation.Classification),
	}, nil
}

// GradeEtymologyOriginMeaning grades an answer against the origin's own gloss.
// Used by the Relearn quiz, which re-drills a missed origin by showing the
// origin and asking its meaning.
func (s *Service) GradeEtymologyOriginMeaning(
	ctx context.Context,
	card EtymologyOriginCard,
	answer string,
	responseTimeMs int64,
) (GradeResult, error) {
	return s.GradeEtymologyWordAnswer(ctx, EtymologyFamilyWord{Meaning: card.Meaning}, answer, responseTimeMs)
}

// isExactMatch reports whether two strings are equal after trimming and case-folding.
func isExactMatch(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// exactMatchResult builds a GradeResult for the trivial "answer equals expected" case.
func exactMatchResult(responseTimeMs int64) GradeResult {
	return GradeResult{
		Correct:        true,
		Reason:         "Exact match.",
		Quality:        qualityFromResponseTime(true, responseTimeMs),
		Classification: string(inference.ClassificationSameWord),
	}
}

// qualityFromResponseTime maps a wall-clock response time into the SM-2 quality
// scale. Correct answers earn 3–5 by speed; wrong answers always earn 1.
func qualityFromResponseTime(correct bool, responseTimeMs int64) int {
	if !correct {
		return 1
	}
	switch {
	case responseTimeMs < 3000:
		return 5
	case responseTimeMs < 10000:
		return 4
	default:
		return 3
	}
}

// SaveEtymologyOriginResult records ONE learning-log entry for the origin's
// (session, sense) series — never one per derived word (invariants L1/L4).
// wordResults carries this attempt's per-word grading outcome as inline data
// on that one entry (see notebook.LearningRecord.WordResults); pass nil for
// callers (e.g. the relearn quiz) that don't grade individual family words.
func (s *Service) SaveEtymologyOriginResult(
	card EtymologyOriginCard,
	quality int,
	correct bool,
	responseTimeMs int64,
	isKnownWord bool,
	wordResults []notebook.EtymologyWordLog,
) error {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return fmt.Errorf("failed to load learning histories: %w", err)
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[card.NotebookName], s.calculator)
	updater.UpdateOrCreateExpressionWithQualityForEtymology(
		card.NotebookName,
		card.SessionTitle,
		card.Origin,
		card.Sense,
		correct,
		isKnownWord,
		quality,
		responseTimeMs,
		wordResults,
	)

	// L1 structural guard: refuse to persist a state where one (origin, sense)
	// forks into two entries within the same session.
	if err := notebook.AssertNoDuplicateOriginsInSession(updater.GetHistory(), card.NotebookName, card.SessionTitle); err != nil {
		return fmt.Errorf("save etymology origin %q: %w", card.Origin, err)
	}

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, card.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return fmt.Errorf("failed to save learning history for %q: %w", card.NotebookName, err)
	}
	return nil
}

// OverrideEtymologyWordResult flips one derived family word's correctness
// and/or excluded flag within the origin's existing (session, sense)
// learning-log entry. It never creates a second log series for the word —
// see notebook.LearningHistoryUpdater.OverrideEtymologyWordResult, the
// single function this delegates to, for the L1/L2 canonicalization this
// relies on.
func (s *Service) OverrideEtymologyWordResult(
	notebookName, sessionTitle, origin, sense, learnedAt, wordExpression string,
	correct, excluded *bool,
) error {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return fmt.Errorf("failed to load learning histories: %w", err)
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[notebookName], s.calculator)
	if !updater.OverrideEtymologyWordResult(sessionTitle, origin, sense, learnedAt, wordExpression, correct, excluded) {
		return fmt.Errorf("etymology word %q not found for origin %q (session %q, sense %q)", wordExpression, origin, sessionTitle, sense)
	}

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, notebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return fmt.Errorf("failed to save learning history for %q: %w", notebookName, err)
	}
	return nil
}

// GetLatestOriginLearnedInfo returns learned_at and next_review_date for the
// latest log of an origin's (session, sense) series. It uses the SAME
// canonical lookup the write path uses (invariant L2), so a fresh submit is
// immediately visible via this read.
func (s *Service) GetLatestOriginLearnedInfo(notebookName, sessionTitle, origin, sense string) (learnedAt string, nextReviewDate string) {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return "", ""
	}
	expr := notebook.FindOriginExpression(learningHistories[notebookName], sessionTitle, origin, sense)
	if expr == nil {
		return "", ""
	}
	logs := expr.GetLogsForQuizType(notebook.QuizTypeEtymologyOrigin)
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

// LoadEtymologyNotebookSummaries returns etymology notebook summaries with the
// per-mode due origin count. includeUnstudied passes through to
// shouldIncludeOrigin so the counts match what LoadEtymologyOriginCards would
// return for the same toggle.
func (s *Service) LoadEtymologyNotebookSummaries(includeUnstudied bool) ([]NotebookSummary, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	families := buildOriginFamilies(reader)

	var summaries []NotebookSummary
	for id, index := range reader.GetEtymologyIndexes() {
		origins, err := reader.ReadEtymologyNotebook(id)
		if err != nil {
			continue
		}

		var total int
		seen := make(map[string]bool)
		seenSession := make(map[string]struct{})
		var sessionOrder []string
		sectionCounts := make(map[string]int)
		for _, o := range origins {
			if o.SessionTitle != "" {
				if _, ok := seenSession[o.SessionTitle]; !ok {
					seenSession[o.SessionTitle] = struct{}{}
					sessionOrder = append(sessionOrder, o.SessionTitle)
				}
			}
			key := o.SessionTitle + "\x00" + o.Sense + "\x00" + originDedupKey(o.Origin)
			if seen[key] {
				continue
			}
			seen[key] = true
			if len(families[originFamilyKey(id, o.SessionTitle, o.Sense, o.Origin)]) == 0 {
				continue
			}
			if shouldIncludeOrigin(learningHistories[id], o.SessionTitle, o.Origin, o.Sense, includeUnstudied) {
				total++
				sectionCounts[o.SessionTitle]++
			}
		}

		var sections []NotebookSectionSummary
		for _, title := range sessionOrder {
			sections = append(sections, NotebookSectionSummary{
				Title:                title,
				EtymologyReviewCount: sectionCounts[title],
			})
		}

		summaries = append(summaries, NotebookSummary{
			NotebookID:           id,
			Name:                 index.Name,
			EtymologyReviewCount: total,
			Kind:                 "Etymology",
			LatestDate:           index.LatestDate,
			Sections:             sections,
		})
	}

	return summaries, nil
}
