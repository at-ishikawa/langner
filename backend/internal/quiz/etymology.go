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
// types Meaning; Expression is the word displayed as context. Pronunciation,
// Examples, and Literal are per-word study context (never quizzed):
// Pronunciation is shown as a typing hint; Examples (example sentences) and
// Literal (the assembled literal gloss, e.g. `de "down" + facere = "made
// down"`) are revealed only on the feedback screen so they don't leak the
// meaning while answering.
type EtymologyFamilyWord struct {
	Expression    string
	Meaning       string
	Pronunciation string
	Examples      []string
	Literal       string
	// Identity + on-disk location of the word's OWN per-word etymology-origin
	// learning series (invariants L1/L4). NoteID (preferred) then Expression /
	// Definition resolve the word's entry via FindExpressionInHistories — the
	// SAME lookup the exclude check uses (L2). SessionTitle / SceneTitle place a
	// never-studied word's new entry exactly where the standard/reverse quizzes
	// write it, so a word keeps a single entry carrying all its quiz series.
	NoteID       string
	Definition   string
	SessionTitle string
	SceneTitle   string
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
	// EnglishForms are the English combining-form spellings the origin
	// surfaces as in English words (e.g. fac, fic, fect). Study context
	// shown in the origin header — never quizzed.
	EnglishForms []string
	// Note is the origin's free-text pedagogical hint. Study context.
	Note string
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

	families := buildOriginFamilies(reader, learningHistories)
	etymIndexes := reader.GetEtymologyIndexes()

	seen := make(map[string]bool)
	// placed tracks words already put on an EARLIER card this session, so a word
	// whose origin_parts span several origins (e.g. deficient → de, facere) is
	// asked exactly once per deck instead of once per origin. Applied only to the
	// real quiz; the relearn enumeration (skipEligibility) keeps every origin's
	// full family so it can index each origin for grading.
	placed := make(map[string]bool)
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
			if !skipEligibility {
				// Keep only this origin's DUE, not-yet-placed words. A word due
				// under two origins lands on the first card only; an origin whose
				// due words were all placed earlier becomes empty and is dropped.
				words = dueUnplacedWords(learningHistories[etymID], etymID, words, includeUnstudied, placed)
				if len(words) == 0 {
					continue
				}
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
				EnglishForms:  o.EnglishForms,
				Note:          o.Note,
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
//
// A word the learner deliberately excluded from the etymology-origin quiz
// (skipped_at set for QuizTypeEtymologyOrigin, via SkipWord) is dropped from
// every family here — per-word exclusion. An origin whose whole family is
// excluded ends up with an empty family and is therefore never offered by
// LoadEtymologyOriginCards / counted by LoadEtymologyNotebookSummaries.
func buildOriginFamilies(reader *notebook.Reader, learningHistories map[string][]notebook.LearningHistory) map[string][]EtymologyFamilyWord {
	result := make(map[string][]EtymologyFamilyWord)
	for _, bookID := range reader.GetDefinitionsBookIDs() {
		// GetDefinitionsNotesByTitle keys scenes by their HUMAN title (not the
		// __index_N key) — the same (sessionTitle, sceneTitle) the standard /
		// reverse quizzes and the skip path write a word's learning history
		// under. Reading through it here lets a word's per-word etymology series
		// land in the very same entry those quizzes use (invariants L1/L4/L2).
		defs, ok := reader.GetDefinitionsNotesByTitle(bookID)
		if !ok {
			continue
		}
		for sessionTitle, sceneDefs := range defs {
			for sceneTitle, notes := range sceneDefs {
				for _, note := range notes {
					expr := note.Expression
					if expr == "" {
						expr = note.Definition
					}
					if expr == "" {
						continue
					}
					// Per-word exclusion: skip a word the learner excluded from
					// the etymology-origin quiz. Read via the same key SkipWord
					// wrote (invariant L2).
					if notebook.IsExpressionExcludedForQuizType(
						learningHistories[bookID], note.ID, notebook.QuizTypeEtymologyOrigin, note.Expression, note.Definition,
					) {
						continue
					}
					word := EtymologyFamilyWord{
						Expression:    expr,
						Meaning:       note.Meaning,
						Pronunciation: note.Pronunciation,
						Examples:      note.Examples,
						// Literal is the assembled literal gloss the converter
						// stores in the word's free-text note field (Note.Note).
						Literal:      note.Note,
						NoteID:       note.ID,
						Definition:   note.Definition,
						SessionTitle: sessionTitle,
						SceneTitle:   sceneTitle,
					}
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

// wordScheduleKey is the per-session dedup / distinct-count key for one derived
// word: its notebook plus its stable identity (NoteID when present, else the
// lower-cased expression). A word that appears under several origins shares one
// key, so it is placed on exactly one card and counted once.
func wordScheduleKey(notebookID string, w EtymologyFamilyWord) string {
	id := w.NoteID
	if id == "" {
		id = strings.ToLower(strings.TrimSpace(w.Expression))
	}
	return notebookID + "\x00" + id
}

// etymologyWordDue reports whether a derived word is due for the etymology-origin
// quiz. It is the single source of truth used by the card loader and the
// notebook-summary count so the badge and the quiz can never disagree. It reads
// the word's OWN per-word series (invariants L1/L4) via the same lookup the
// exclude check uses (L2):
//
//   - excluded (skipped_at set) → not due.
//   - no etymology-origin logs yet → due iff includeUnstudied.
//   - has logs → defer to the word's own SR interval.
func etymologyWordDue(
	histories []notebook.LearningHistory, w EtymologyFamilyWord, includeUnstudied bool,
) bool {
	expr := notebook.FindExpressionInHistories(histories, w.NoteID, w.Expression, w.Definition)
	if expr == nil {
		return includeUnstudied
	}
	if expr.SkippedAt.IsSkipped(notebook.QuizTypeEtymologyOrigin) {
		return false
	}
	if len(expr.EtymologyOriginLogs) == 0 {
		return includeUnstudied
	}
	return expr.NeedsEtymologyReview(notebook.QuizTypeEtymologyOrigin)
}

// dueUnplacedWords returns the subset of an origin's family that is due AND has
// not already been placed on an earlier card this session, marking each returned
// word as placed. This is what makes a multi-origin word appear exactly once per
// deck (within-session dedup) while still honouring the per-word SR schedule.
func dueUnplacedWords(
	histories []notebook.LearningHistory,
	notebookID string,
	words []EtymologyFamilyWord,
	includeUnstudied bool,
	placed map[string]bool,
) []EtymologyFamilyWord {
	var out []EtymologyFamilyWord
	for _, w := range words {
		k := wordScheduleKey(notebookID, w)
		if placed[k] {
			continue
		}
		if !etymologyWordDue(histories, w, includeUnstudied) {
			continue
		}
		placed[k] = true
		out = append(out, w)
	}
	return out
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

// EtymologyWordGrade is one derived family word's graded outcome for a submitted
// origin card, carried into SaveEtymologyWordResults so each word advances its
// OWN per-word series (invariants L1/L4).
type EtymologyWordGrade struct {
	Word    EtymologyFamilyWord
	Correct bool
	Quality int
}

// SaveEtymologyWordResults records ONE learning-log entry per answered derived
// word, each on the WORD's own EtymologyOriginLogs series (invariants L1/L4) —
// never a per-origin series. The origin is presentation grouping only. Every
// word's entry lives in the definitions book's learning-history file
// (card.NotebookName == that book id), the SAME file the exclude check reads
// (invariant L2). It returns the aggregate learned_at (the answer date) and
// next_review_date (the earliest per-word next review) the submit response
// surfaces so the existing UI keeps working without a per-origin schedule.
func (s *Service) SaveEtymologyWordResults(
	card EtymologyOriginCard,
	grades []EtymologyWordGrade,
	responseTimeMs int64,
) (learnedAt string, nextReviewDate string, err error) {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return "", "", fmt.Errorf("failed to load learning histories: %w", err)
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[card.NotebookName], s.calculator)
	for _, g := range grades {
		updater.UpsertWordEtymologyOriginResult(
			card.NotebookName,
			g.Word.SessionTitle,
			g.Word.SceneTitle,
			g.Word.Expression,
			g.Word.Definition,
			g.Word.NoteID,
			g.Correct,
			true,
			g.Quality,
			responseTimeMs,
		)
	}

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, card.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return "", "", fmt.Errorf("failed to save learning history for %q: %w", card.NotebookName, err)
	}

	// Aggregate learned_at/next_review across the just-written words: learned_at
	// is the answer date (all share it); next_review_date is the earliest word's
	// next review, so the card surfaces the soonest it should be seen again.
	var earliest string
	for _, g := range grades {
		expr := notebook.FindExpressionInHistories(updater.GetHistory(), g.Word.NoteID, g.Word.Expression, g.Word.Definition)
		if expr == nil || len(expr.EtymologyOriginLogs) == 0 {
			continue
		}
		latest := expr.EtymologyOriginLogs[0]
		if learnedAt == "" {
			learnedAt = latest.LearnedAt.Format("2006-01-02")
		}
		next := latest.LearnedAt.AddDate(0, 0, latest.IntervalDays).Format("2006-01-02")
		if earliest == "" || next < earliest {
			earliest = next
		}
	}
	return learnedAt, earliest, nil
}

// OverrideEtymologyWordResult flips ONE derived word's stored etymology-origin
// result (Mark-as-Correct / Incorrect on the feedback screen). The word owns its
// series now, so this is a normal override on that word's EtymologyOriginLogs:
// it resolves the word by expression — the SAME key the exclude path uses
// (invariant L2) — recomputes its interval, and never forks a second series
// (L1). learnedAt selects the specific attempt.
func (s *Service) OverrideEtymologyWordResult(
	notebookName, learnedAt, wordExpression string,
	correct *bool,
) error {
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return fmt.Errorf("failed to load learning histories: %w", err)
	}

	updater := notebook.NewLearningHistoryUpdater(learningHistories[notebookName], s.calculator)
	res := updater.OverrideLog(notebook.OverrideLogInput{
		QuizType:    notebook.QuizTypeEtymologyOrigin,
		Expression:  wordExpression,
		LearnedAt:   learnedAt,
		MarkCorrect: correct,
	})
	if !res.Found {
		return fmt.Errorf("etymology word %q not found for notebook %q at %q", wordExpression, notebookName, learnedAt)
	}

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, notebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return fmt.Errorf("failed to save learning history for %q: %w", notebookName, err)
	}
	return nil
}

// LoadEtymologyNotebookSummaries returns etymology notebook summaries with the
// per-mode due count — now the number of DISTINCT due derived words (a word in
// several origins is counted once), matching what LoadEtymologyOriginCards would
// offer for the same toggle.
func (s *Service) LoadEtymologyNotebookSummaries(includeUnstudied bool) ([]NotebookSummary, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}
	learningHistories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to load learning histories: %w", err)
	}

	families := buildOriginFamilies(reader, learningHistories)

	var summaries []NotebookSummary
	for id, index := range reader.GetEtymologyIndexes() {
		origins, err := reader.ReadEtymologyNotebook(id)
		if err != nil {
			continue
		}

		seenSession := make(map[string]struct{})
		var sessionOrder []string
		// Count DISTINCT due words per notebook and per session: a word that
		// derives from several origins is counted once (dedup by wordScheduleKey),
		// so the badge no longer double-counts a multi-origin word.
		countedTotal := make(map[string]bool)
		countedSection := make(map[string]map[string]bool)
		for _, o := range origins {
			if o.SessionTitle != "" {
				if _, ok := seenSession[o.SessionTitle]; !ok {
					seenSession[o.SessionTitle] = struct{}{}
					sessionOrder = append(sessionOrder, o.SessionTitle)
				}
			}
			for _, w := range families[originFamilyKey(id, o.SessionTitle, o.Sense, o.Origin)] {
				if !etymologyWordDue(learningHistories[id], w, includeUnstudied) {
					continue
				}
				k := wordScheduleKey(id, w)
				countedTotal[k] = true
				if countedSection[o.SessionTitle] == nil {
					countedSection[o.SessionTitle] = make(map[string]bool)
				}
				countedSection[o.SessionTitle][k] = true
			}
		}
		total := len(countedTotal)

		var sections []NotebookSectionSummary
		for _, title := range sessionOrder {
			sections = append(sections, NotebookSectionSummary{
				Title:                title,
				EtymologyReviewCount: len(countedSection[title]),
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
