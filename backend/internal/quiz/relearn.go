package quiz

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// RelearnConversationLine is one speaker/quote line of scene context.
type RelearnConversationLine struct {
	Speaker string
	Quote   string
}

// RelearnContextScene groups the prose statements and conversation lines in
// which a word appears within a single scene. Assembled read-only from
// notebook data for the Relearn feedback screen.
type RelearnContextScene struct {
	NotebookName  string
	SceneTitle    string
	Statements    []string
	Conversations []RelearnConversationLine
}

// RelearnCard is one pooled wrong word, resolved for grading and feedback,
// held in the handler's in-memory store for the life of a Relearn session.
//
// Each card mirrors the ONE quiz type it was failed in — Format decides how the
// frontend presents it and which pure grader the handler uses:
//
//	QuizTypeNotebook          recognition: show Entry, ask the Meaning
//	QuizTypeReverse           production:  show Meaning + masked Contexts, ask Entry
//	QuizTypeEtymologyStandard show Entry (origin), ask the Meaning
//	QuizTypeEtymologyReverse  show Meaning, ask Entry (origin)
//
// A word failed in several quiz types yields one card per type. Nothing about a
// RelearnCard is ever written to learning history; ClearKey is the only value
// that leaves the session, and only into the relearn_clears marker store.
type RelearnCard struct {
	Format       notebook.QuizType
	Entry        string
	Meaning      string
	NotebookName string
	ClearKey     string

	// Etymology display extras (empty for vocab cards).
	OriginType string
	Language   string

	// Answering-screen hints.
	Examples []Example        // recognition
	Contexts []ReverseContext // reverse (masked)

	// Rich feedback.
	WordDetail    WordDetail
	Images        []string
	ContextScenes []RelearnContextScene

	// Grading inputs — one populated per Format.
	vocabCard     Card
	reverseCard   ReverseCard
	etymologyCard EtymologyOriginCard
}

// VocabCard, ReverseCard, EtymologyCard return the card the matching pure grader
// consumes for this Format.
func (c RelearnCard) VocabCard() Card                    { return c.vocabCard }
func (c RelearnCard) ReverseCard() ReverseCard           { return c.reverseCard }
func (c RelearnCard) EtymologyCard() EtymologyOriginCard { return c.etymologyCard }

// IsEtymology reports whether the card's Format is one of the etymology modes.
func (c RelearnCard) IsEtymology() bool {
	return c.Format == notebook.QuizTypeEtymologyStandard || c.Format == notebook.QuizTypeEtymologyReverse
}

// relearnKeySep separates the fields of a relearn clear_key. It is the ASCII
// Unit Separator (0x1F): a valid UTF-8 byte that cannot appear in notebook
// names or expressions. A NUL byte (0x00) must NOT be used — Postgres rejects
// NUL in text/varchar values ("invalid byte sequence for encoding UTF8: 0x00"),
// which would silently fail the relearn_clears insert.
const relearnKeySep = "\x1f"

// RelearnClearKey builds the stable key identifying one relearn card in the
// relearn_clears marker store. The Format is part of the key so a word failed
// in several quiz types clears each format independently — recovering the
// recognition card does not clear the reverse card for the same word.
func RelearnClearKey(format notebook.QuizType, notebookName, expression string) string {
	return string(format) + relearnKeySep + notebookName + relearnKeySep + strings.ToLower(strings.TrimSpace(expression))
}

// relearnCandidate is an intermediate per-format wrong-word record before it is
// resolved to a gradeable card.
type relearnCandidate struct {
	notebookName string
	expression   string
	format       notebook.QuizType
	latestWrong  time.Time
}

// LoadRelearnPool builds the Relearn Quiz pool: for every learning-log series
// (recognition, reverse, etymology breakdown, etymology assembly) whose
// most-recent log within [windowStart, now] has status "misunderstood", it
// emits one card that mirrors that series' quiz type — minus series already
// cleared more recently. A word failed in several types produces several cards.
//
// It reads the YAML learning histories directly — the source of truth and the
// only place etymology-origin results are stored — so the pool spans both
// vocabulary and etymology words regardless of whether a database is
// configured. It writes nothing.
//
// clears maps a RelearnClearKey to the time that (word, format) was last
// recovered; it is excluded when its clear time is not before its most-recent
// in-window wrong log.
func (s *Service) LoadRelearnPool(windowStart time.Time, clears map[string]time.Time) ([]RelearnCard, error) {
	histories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("load learning histories: %w", err)
	}

	// One candidate per (format, notebook, expression); the same expression can
	// recur across scenes (multi-sense etymology), so keep the most-recent wrong.
	candidates := make(map[string]relearnCandidate)
	consider := func(notebookName string, expr notebook.LearningHistoryExpression) {
		for _, sp := range relearnSeries(expr) {
			if len(sp.logs) == 0 {
				continue
			}
			latest := sp.logs[0] // newest-first
			if latest.LearnedAt.Before(windowStart) || latest.Status != notebook.LearnedStatusMisunderstood {
				continue
			}
			key := RelearnClearKey(sp.format, notebookName, expr.Expression)
			if existing, ok := candidates[key]; ok && !latest.LearnedAt.After(existing.latestWrong) {
				continue
			}
			candidates[key] = relearnCandidate{
				notebookName: notebookName,
				expression:   expr.Expression,
				format:       sp.format,
				latestWrong:  latest.LearnedAt.Time,
			}
		}
	}
	for notebookName, hs := range histories {
		for _, h := range hs {
			for _, expr := range h.Expressions { // flashcard-level
				consider(notebookName, expr)
			}
			for _, scene := range h.Scenes { // story/etymology scene-level
				for _, expr := range scene.Expressions {
					consider(notebookName, expr)
				}
			}
		}
	}

	// Drop series already recovered in a later Relearn session.
	candidatesFound := len(candidates)
	for key, c := range candidates {
		if clearedAt, ok := clears[key]; ok && !clearedAt.Before(c.latestWrong) {
			delete(candidates, key)
		}
	}
	afterClears := len(candidates)
	if len(candidates) == 0 {
		slog.Info("relearn pool empty",
			"in_window_misunderstood", candidatesFound, "after_clears", afterClears, "clears_in_store", len(clears))
		return nil, nil
	}

	vocabByExpr, vocabByNotebookExpr, err := s.relearnVocabIndex()
	if err != nil {
		return nil, err
	}
	etymByOrigin, err := s.relearnEtymologyIndex()
	if err != nil {
		return nil, err
	}

	cards := make([]RelearnCard, 0, len(candidates))
	for key, c := range candidates {
		if c.format == notebook.QuizTypeEtymologyStandard || c.format == notebook.QuizTypeEtymologyReverse {
			sense, ok := etymByOrigin[strings.ToLower(strings.TrimSpace(c.expression))]
			if !ok {
				continue // no origin data to grade/display against
			}
			cards = append(cards, RelearnCard{
				Format: c.format, Entry: c.expression, Meaning: sense.Meaning, NotebookName: c.notebookName,
				ClearKey: key, OriginType: sense.Type, Language: sense.Language, etymologyCard: sense,
			})
			continue
		}
		fc, ok := vocabByNotebookExpr[strings.ToLower(c.notebookName)+relearnKeySep+strings.ToLower(strings.TrimSpace(c.expression))]
		if !ok {
			fc, ok = vocabByExpr[strings.ToLower(strings.TrimSpace(c.expression))]
		}
		if !ok {
			continue // no vocab data to grade/display against
		}
		card := RelearnCard{
			Format: c.format, Entry: c.expression, Meaning: fc.Meaning, NotebookName: c.notebookName,
			ClearKey: key, WordDetail: fc.WordDetail, Images: fc.Images,
			ContextScenes: relearnScenesFromCard(fc),
		}
		if c.format == notebook.QuizTypeReverse {
			masked := relearnMaskedContexts(fc)
			card.Contexts = masked
			card.reverseCard = ReverseCard{
				NotebookName: fc.NotebookName, StoryTitle: fc.StoryTitle, SceneTitle: fc.SceneTitle,
				Meaning: fc.Meaning, Contexts: masked, Expression: fc.Expression, AltForm: fc.OriginalExpression,
				WordDetail: fc.WordDetail, Images: fc.Images,
			}
		} else {
			card.Examples = relearnExamplesFromContexts(fc.Contexts)
			card.vocabCard = Card{
				NotebookName: fc.NotebookName, StoryTitle: fc.StoryTitle, SceneTitle: fc.SceneTitle,
				Entry: fc.Expression, OriginalEntry: fc.OriginalExpression, Meaning: fc.Meaning,
				Contexts: relearnRecognitionContexts(fc), WordDetail: fc.WordDetail, Images: fc.Images,
			}
		}
		cards = append(cards, card)
	}
	// One line so an empty/short pool can be diagnosed from the server log:
	// how many wrong words were in the window, how many survived the clear
	// markers, and how many matched a gradeable card.
	slog.Info("relearn pool built",
		"in_window_misunderstood", candidatesFound, "after_clears", afterClears,
		"matched_cards", len(cards), "clears_in_store", len(clears))
	return cards, nil
}

// relearnSeriesSpec describes one learning-log series to inspect for a wrong
// word, and the relearn card format it maps to.
type relearnSeriesSpec struct {
	logs   []notebook.LearningRecord
	format notebook.QuizType
}

// relearnSeries returns the four independent log series an expression can carry,
// each mapped to the relearn card format that mirrors it. Notebook and freeform
// share LearnedLogs and both replay as recognition; etymology freeform shares
// the breakdown track and replays as etymology-standard.
func relearnSeries(expr notebook.LearningHistoryExpression) []relearnSeriesSpec {
	return []relearnSeriesSpec{
		{logs: expr.LearnedLogs, format: notebook.QuizTypeNotebook},
		{logs: expr.ReverseLogs, format: notebook.QuizTypeReverse},
		{logs: expr.EtymologyBreakdownLogs, format: notebook.QuizTypeEtymologyStandard},
		{logs: expr.EtymologyAssemblyLogs, format: notebook.QuizTypeEtymologyReverse},
	}
}

// relearnVocabIndex loads every vocabulary word once and indexes it both by
// (notebook, expression) and by expression alone so the pool can resolve a
// wrong word to its meaning and context.
func (s *Service) relearnVocabIndex() (byExpr map[string]FreeformCard, byNotebookExpr map[string]FreeformCard, err error) {
	words, err := s.LoadAllWords()
	if err != nil {
		return nil, nil, fmt.Errorf("load words for relearn pool: %w", err)
	}
	byExpr = make(map[string]FreeformCard, len(words))
	byNotebookExpr = make(map[string]FreeformCard, len(words))
	for _, w := range words {
		for _, e := range []string{w.Expression, w.OriginalExpression} {
			e = strings.ToLower(strings.TrimSpace(e))
			if e == "" {
				continue
			}
			byExpr[e] = w
			byNotebookExpr[strings.ToLower(w.NotebookName)+relearnKeySep+e] = w
		}
	}
	return byExpr, byNotebookExpr, nil
}

// relearnEtymologyIndex loads every etymology origin once and indexes the
// first sense per origin spelling for grading and display.
func (s *Service) relearnEtymologyIndex() (map[string]EtymologyOriginCard, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("init reader for relearn etymology pool: %w", err)
	}
	var etymIDs []string
	for id := range reader.GetEtymologyIndexes() {
		etymIDs = append(etymIDs, id)
	}
	byOrigin := make(map[string]EtymologyOriginCard)
	if len(etymIDs) == 0 {
		return byOrigin, nil
	}
	cards, err := s.LoadEtymologyOriginCards(etymIDs, true, true, notebook.QuizTypeEtymologyStandard, nil)
	if err != nil {
		return nil, fmt.Errorf("load etymology origins for relearn pool: %w", err)
	}
	for _, c := range cards {
		k := strings.ToLower(strings.TrimSpace(c.Origin))
		if _, ok := byOrigin[k]; !ok {
			byOrigin[k] = c
		}
	}
	return byOrigin, nil
}

// relearnMaskedContexts builds reverse-quiz-style masked contexts from a vocab
// card: the sentences the word appears in, with the word blanked out so it can
// serve as a hint without giving away the answer.
func relearnMaskedContexts(fc FreeformCard) []ReverseContext {
	var out []ReverseContext
	for _, c := range fc.Contexts {
		text := strings.TrimSpace(c.Context)
		if text == "" {
			continue
		}
		out = append(out, ReverseContext{
			Context:       text,
			MaskedContext: maskWord(text, fc.Expression, fc.OriginalExpression),
		})
	}
	return out
}

// relearnRecognitionContexts builds the contexts the meaning grader
// (GradeNotebookAnswer -> AnswerMeanings) sees for a recognition card. It:
//
//  1. Sets reference_definition to the word's known meaning on every context.
//     The grader treats a non-empty reference_definition as authoritative
//     ground truth and grades the user's answer against it — far more lenient
//     and accurate than re-deriving the meaning from a sentence (e.g. it
//     accepts "does not pursue pleasure of flesh" for "ascetic").
//  2. Guarantees at least one context. Vocabulary words with no example
//     sentences (e.g. plain definition entries) would otherwise be sent with
//     zero contexts, and the grader returns zero answers — which
//     extractAnswerResult treats as INCORRECT no matter what the learner types,
//     trapping the word in the relearn loop.
func relearnRecognitionContexts(fc FreeformCard) []inference.Context {
	out := make([]inference.Context, 0, len(fc.Contexts)+1)
	for _, c := range fc.Contexts {
		c.ReferenceDefinition = fc.Meaning
		out = append(out, c)
	}
	if len(out) == 0 {
		out = append(out, inference.Context{ReferenceDefinition: fc.Meaning})
	}
	return out
}

// relearnScenesFromCard turns a vocab card's contexts into a single context
// scene keyed by the card's scene, rendered as prose on the feedback screen.
func relearnScenesFromCard(card FreeformCard) []RelearnContextScene {
	var statements []string
	for _, c := range card.Contexts {
		if s := strings.TrimSpace(c.Context); s != "" {
			statements = append(statements, s)
		}
	}
	if len(statements) == 0 {
		return nil
	}
	return []RelearnContextScene{{
		NotebookName: card.NotebookName,
		SceneTitle:   card.SceneTitle,
		Statements:   statements,
	}}
}

// relearnExamplesFromContexts exposes the card's context sentences as examples
// so the recognition answering screen can show a hint, like the standard quiz.
func relearnExamplesFromContexts(contexts []inference.Context) []Example {
	var out []Example
	for _, c := range contexts {
		if s := strings.TrimSpace(c.Context); s != "" {
			out = append(out, Example{Text: s})
		}
	}
	return out
}
