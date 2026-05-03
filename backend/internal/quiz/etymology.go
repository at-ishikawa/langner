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
//
// SessionTitle disambiguates multi-sense origins: the same origin string can
// appear in multiple sessions with different meanings, so the card key is
// (NotebookName, SessionTitle, Origin) rather than Origin alone.
type EtymologyOriginCard struct {
	NotebookName  string
	NotebookTitle string
	SessionTitle  string
	Origin        string
	Type          string
	Language      string
	Meaning       string
}

// originDedupKey returns the canonical key used to deduplicate etymology
// origins within one session. The key is the trimmed, lowercased origin —
// inconsistent language metadata (e.g. "Greek" vs "" vs "greek ") for the
// same origin within a session collapses to one card. Multi-sense origins
// across sessions remain distinct because callers prefix the dedup key with
// the session title.
func originDedupKey(origin string) string {
	return strings.ToLower(strings.TrimSpace(origin))
}

// LoadEtymologyOriginCards loads individual origin cards from selected etymology notebooks.
//
// quizType determines which log set the SR-due check reads from
// (etymology_breakdown_logs for QuizTypeEtymologyStandard, etymology_assembly_logs
// for QuizTypeEtymologyReverse). It also gates per-type skip filtering.
//
// When skipEligibility is true the hard gate (freeform-first + has-correct-answer)
// is skipped — the freeform quiz needs this because it IS the entry point where
// origins are encountered for the first time.
func (s *Service) LoadEtymologyOriginCards(
	etymologyNotebookIDs []string,
	includeUnstudied bool,
	skipEligibility bool,
	quizType notebook.QuizType,
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

	// skipQuizType picks which slot in SkippedAt gates this load. It
	// matches the quiz mode being started; per-type skips let the user
	// exclude an origin from reverse without affecting standard, etc.
	skipQuizType := quizType
	if skipQuizType == "" {
		skipQuizType = notebook.QuizTypeEtymologyStandard
	}

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

		for _, o := range origins {
			// Per-session dedup: an origin appearing twice within the
			// same session (e.g. with inconsistent language metadata)
			// collapses to one card, but the same origin in another
			// session survives — that's how multi-sense origins (ana =
			// "up" vs ana = "negative") stay as separate drills.
			key := etymID + "\x00" + o.SessionTitle + "\x00" + originDedupKey(o.Origin)
			if seen[key] {
				continue
			}
			seen[key] = true

			// Per-type skip: drop origins the user has marked as
			// skipped from this quiz mode.
			if isOriginSkipped(learningHistories[etymID], nbTitle, o.SessionTitle, o.Origin, skipQuizType) {
				continue
			}

			// Hard eligibility gate for standard/reverse quizzes:
			// origins must have at least one etymology_freeform entry
			// AND at least one correct etymology answer. Skipped for
			// freeform mode, which is the entry point where new origins
			// are first encountered.
			if !skipEligibility && !isOriginEligible(learningHistories[etymID], nbTitle, o.SessionTitle, o.Origin) {
				continue
			}
			// Soft SR check: skipped when includeUnstudied is true so
			// the user can still drill origins that are not due yet.
			// Reads the log set matching the active quiz mode — fixes a
			// bug where reverse mode used the standard track and showed
			// origins the user had just answered correctly in reverse.
			if !includeUnstudied {
				if !needsOriginReview(learningHistories[etymID], nbTitle, o.SessionTitle, o.Origin, skipQuizType) {
					continue
				}
			}

			cards = append(cards, EtymologyOriginCard{
				NotebookName:  etymID,
				NotebookTitle: nbTitle,
				SessionTitle:  o.SessionTitle,
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
	// SessionTitle becomes the scene title — per-sense SR history is keyed by
	// (notebook, session_title, origin) so two different senses of the same
	// origin (ana = "up" in Session 13, "negative" in Session 16) get
	// independent learning curves.
	updater.UpdateOrCreateExpressionWithQualityForEtymology(
		card.NotebookName,
		card.NotebookTitle,
		card.SessionTitle,
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

// LoadEtymologyExampleWords returns example expressions for each card,
// keyed by lower(origin)\x00sessionTitle. Examples come from the
// consolidated definitions notebook (same notebook ID as the etymology
// notebook), narrowed by the parent session's metadata.title — so the
// "ana" card from Session 13 gets words that use the Session 13 sense
// of "ana", not Session 16's.
func (s *Service) LoadEtymologyExampleWords(cards []EtymologyOriginCard) (map[string][]string, error) {
	if len(cards) == 0 {
		return nil, nil
	}
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}

	type key struct {
		notebookID, sessionTitle, origin string
	}
	wanted := make(map[key]struct{})
	for _, c := range cards {
		k := key{c.NotebookName, c.SessionTitle, strings.ToLower(strings.TrimSpace(c.Origin))}
		wanted[k] = struct{}{}
	}

	result := make(map[string][]string)
	bookIDs := reader.GetDefinitionsBookIDs()
	for _, bookID := range bookIDs {
		// Examples come from the consolidated definitions notebook (Phase 2:
		// same ID as the etymology notebook).
		hasMatch := false
		for k := range wanted {
			if k.notebookID == bookID {
				hasMatch = true
				break
			}
		}
		if !hasMatch {
			continue
		}
		defs, ok := reader.GetDefinitionsNotes(bookID)
		if !ok {
			continue
		}
		for sessionTitle, sceneDefs := range defs {
			for _, notes := range sceneDefs {
				for _, note := range notes {
					for _, ref := range note.OriginParts {
						k := key{bookID, sessionTitle, strings.ToLower(strings.TrimSpace(ref.Origin))}
						if _, ok := wanted[k]; !ok {
							continue
						}
						resultKey := strings.ToLower(strings.TrimSpace(ref.Origin)) + "\x00" + sessionTitle
						expr := note.Expression
						if expr == "" {
							expr = note.Definition
						}
						if expr == "" {
							continue
						}
						result[resultKey] = append(result[resultKey], expr)
					}
				}
			}
		}
	}

	// Cap example list per card to keep prompts readable. The first 3 words
	// is plenty of signal for sense disambiguation.
	const maxExamples = 3
	for k, words := range result {
		if len(words) > maxExamples {
			result[k] = words[:maxExamples]
		}
	}
	return result, nil
}

// LoadEtymologyOriginSenses returns every recorded sense of the given origin
// across all selected etymology notebooks. Used by the freeform feedback
// screen so the user is shown all senses of a multi-sense origin after they
// answer.
func (s *Service) LoadEtymologyOriginSenses(cards []EtymologyOriginCard, origin string) []EtymologyOriginCard {
	var senses []EtymologyOriginCard
	for _, c := range cards {
		if !strings.EqualFold(c.Origin, origin) {
			continue
		}
		senses = append(senses, c)
	}
	return senses
}

// GetEtymologyOriginNextReviewDates returns a map of lowercase origin -> next review date.
//
// Multi-sense origins (e.g. "ana" = "up" and "ana" = "negative") share one
// entry in the freeform suggestion list, so the map aggregates by lowercase
// origin string. The freeform quiz blocks the user only when EVERY sense of
// an origin is scheduled for the future — if any sense is due (or has no
// history yet), the origin is omitted from the map so the user can drill it.
// When all senses are scheduled, the earliest date wins so the user knows
// when the next sense becomes due.
func (s *Service) GetEtymologyOriginNextReviewDates(cards []EtymologyOriginCard) (map[string]string, error) {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	type aggregate struct {
		anyDueNow bool
		earliest  string
	}
	groups := make(map[string]*aggregate)
	for _, card := range cards {
		key := strings.ToLower(card.Origin)
		g, ok := groups[key]
		if !ok {
			g = &aggregate{}
			groups[key] = g
		}
		if g.anyDueNow {
			continue
		}
		nextDate := originNextReviewDate(learningHistories[card.NotebookName], card)
		if nextDate == "" {
			g.anyDueNow = true
			continue
		}
		if g.earliest == "" || nextDate < g.earliest {
			g.earliest = nextDate
		}
	}

	result := make(map[string]string)
	for key, g := range groups {
		if g.anyDueNow {
			continue
		}
		if g.earliest != "" {
			result[key] = g.earliest
		}
	}
	return result, nil
}

func originNextReviewDate(histories []notebook.LearningHistory, card EtymologyOriginCard) string {
	for _, hist := range histories {
		if hist.Metadata.Title != card.NotebookTitle {
			continue
		}
		for _, scene := range hist.Scenes {
			if scene.Metadata.Title != card.SessionTitle {
				continue
			}
			for _, expr := range scene.Expressions {
				if strings.EqualFold(expr.Expression, card.Origin) {
					return computeNextReviewDate(expr.EtymologyBreakdownLogs)
				}
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
		seen := make(map[string]bool)
		for _, o := range origins {
			// Per-(session, origin) dedup so multi-sense origins each
			// contribute their own due-count slot. See
			// LoadEtymologyOriginCards for the keying rationale.
			key := o.SessionTitle + "\x00" + originDedupKey(o.Origin)
			if seen[key] {
				continue
			}
			seen[key] = true
			// Skipped-from-standard origins shouldn't inflate the
			// "due" badge — the start page is meant to surface
			// drillable items.
			if isOriginSkipped(learningHistories[id], index.Name, o.SessionTitle, o.Origin, notebook.QuizTypeEtymologyStandard) {
				continue
			}
			if !isOriginEligible(learningHistories[id], index.Name, o.SessionTitle, o.Origin) {
				continue
			}
			if needsOriginReview(learningHistories[id], index.Name, o.SessionTitle, o.Origin, notebook.QuizTypeEtymologyStandard) {
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
//
// Eligibility is per-(session_title, origin): two senses of the same origin
// are evaluated independently, so progress on Session 13's "ana" doesn't
// count toward Session 16's "ana".
func isOriginEligible(
	histories []notebook.LearningHistory,
	notebookTitle, sessionTitle, origin string,
) bool {
	for _, h := range histories {
		if h.Metadata.Title != notebookTitle {
			continue
		}
		for _, scene := range h.Scenes {
			if scene.Metadata.Title != sessionTitle {
				continue
			}
			for _, expr := range scene.Expressions {
				if !strings.EqualFold(expr.Expression, origin) {
					continue
				}
				return expr.HasEtymologyFreeformAnswer() && expr.HasCorrectEtymologyAnswer()
			}
		}
	}
	return false
}

// isOriginSkipped returns true when the origin's per-(notebook, session)
// learning history records a skip for the given quiz type. Origins outside
// the history (no record yet) are never skipped.
func isOriginSkipped(
	histories []notebook.LearningHistory,
	notebookTitle, sessionTitle, origin string,
	quizType notebook.QuizType,
) bool {
	for _, h := range histories {
		if h.Metadata.Title != notebookTitle {
			continue
		}
		for _, scene := range h.Scenes {
			if scene.Metadata.Title != sessionTitle {
				continue
			}
			for _, expr := range scene.Expressions {
				if !strings.EqualFold(expr.Expression, origin) {
					continue
				}
				return expr.SkippedAt.IsSkipped(quizType)
			}
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
	notebookTitle, sessionTitle, origin string,
	quizType notebook.QuizType,
) bool {
	for _, h := range histories {
		if h.Metadata.Title != notebookTitle {
			continue
		}
		for _, scene := range h.Scenes {
			if scene.Metadata.Title != sessionTitle {
				continue
			}
			for _, expr := range scene.Expressions {
				if !strings.EqualFold(expr.Expression, origin) {
					continue
				}
				return expr.NeedsEtymologyReview(quizType)
			}
		}
	}
	return false
}
