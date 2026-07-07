package quiz

import (
	"fmt"
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

// RelearnCard is one pooled wrong word, fully resolved for grading and for the
// rich feedback response. It is held in the handler's in-memory store for the
// life of a Relearn session.
//
// Nothing about a RelearnCard is ever written to learning history. ClearKey is
// the only value that leaves the session, and only into the relearn_clears
// marker store — never into a learning log.
type RelearnCard struct {
	Entry          string
	NotebookName   string
	SourceQuizType notebook.QuizType
	IsEtymology    bool

	// ClearKey is the stable key used to record a relearn-clear when this
	// word is answered correctly, and the key the pool builder checks to
	// exclude already-recovered words. Kept on the card so the write side and
	// the read side always agree (mirrors the learning-history L2 rule, even
	// though this is not a learning log).
	ClearKey string

	Meaning       string
	WordDetail    WordDetail
	Images        []string
	Examples      []Example
	ContextScenes []RelearnContextScene

	// vocabCard / etymologyCard carry the grading inputs. Exactly one is
	// populated depending on IsEtymology; the handler grades with the matching
	// pure grader (GradeNotebookAnswer or GradeEtymologyStandardAnswer).
	vocabCard     Card
	etymologyCard EtymologyOriginCard
}

// VocabCard returns the card used to grade a non-etymology relearn word.
func (c RelearnCard) VocabCard() Card { return c.vocabCard }

// EtymologyCard returns the card used to grade an etymology-origin relearn word.
func (c RelearnCard) EtymologyCard() EtymologyOriginCard { return c.etymologyCard }

// RelearnClearKey builds the stable key identifying a word in the relearn_clears
// marker store. The kind prefix ("v"/"o") keeps a vocab word and an etymology
// origin that share a spelling in the same notebook distinct, mirroring how the
// learning history keys them by (name, type).
func RelearnClearKey(isEtymology bool, notebookName, expression string) string {
	kind := "v"
	if isEtymology {
		kind = "o"
	}
	return kind + "\x00" + notebookName + "\x00" + strings.ToLower(strings.TrimSpace(expression))
}

// relearnCandidate is an intermediate wrong-word record before it is resolved
// to a gradeable card.
type relearnCandidate struct {
	notebookName string
	expression   string
	isEtymology  bool
	sourceQT     notebook.QuizType
	latestWrong  time.Time
}

// LoadRelearnPool builds the Relearn Quiz pool: every word whose most-recent
// learning log within [windowStart, now] has status "misunderstood", across
// all quiz types, minus words cleared more recently than that wrong log.
//
// It reads the YAML learning histories directly — the source of truth and the
// only place etymology-origin results are stored — so the pool spans both
// vocabulary and etymology words regardless of whether a database is
// configured. It writes nothing.
//
// clears maps a RelearnClearKey to the time that word was last recovered in a
// Relearn session; a word is excluded when its clear time is not before its
// most-recent in-window wrong log.
func (s *Service) LoadRelearnPool(windowStart time.Time, clears map[string]time.Time) ([]RelearnCard, error) {
	histories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("load learning histories: %w", err)
	}

	// Collect the most-recent in-window wrong candidate per (kind, notebook,
	// expression). A word wrong in several quiz types collapses to one card,
	// keeping the most recent wrong log for the source label.
	candidates := make(map[string]relearnCandidate)
	consider := func(notebookName string, expr notebook.LearningHistoryExpression) {
		for _, sp := range relearnSeries(expr) {
			if len(sp.logs) == 0 {
				continue
			}
			latest := sp.logs[0] // newest-first
			if latest.LearnedAt.Before(windowStart) {
				continue
			}
			if latest.Status != notebook.LearnedStatusMisunderstood {
				continue
			}
			sourceQT := notebook.QuizType(latest.QuizType)
			if sourceQT == "" {
				sourceQT = sp.defaultQT
			}
			key := RelearnClearKey(sp.isEtymology, notebookName, expr.Expression)
			if existing, ok := candidates[key]; ok && !latest.LearnedAt.After(existing.latestWrong) {
				continue
			}
			candidates[key] = relearnCandidate{
				notebookName: notebookName,
				expression:   expr.Expression,
				isEtymology:  sp.isEtymology,
				sourceQT:     sourceQT,
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

	// Drop words already recovered in a later Relearn session.
	for key, c := range candidates {
		if clearedAt, ok := clears[key]; ok && !clearedAt.Before(c.latestWrong) {
			delete(candidates, key)
		}
	}
	if len(candidates) == 0 {
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
		if c.isEtymology {
			sense, ok := etymByOrigin[strings.ToLower(strings.TrimSpace(c.expression))]
			if !ok {
				continue // no origin data to grade/display against
			}
			cards = append(cards, RelearnCard{
				Entry: c.expression, NotebookName: c.notebookName, SourceQuizType: c.sourceQT,
				IsEtymology: true, ClearKey: key, Meaning: sense.Meaning, etymologyCard: sense,
			})
			continue
		}
		card, ok := vocabByNotebookExpr[strings.ToLower(c.notebookName)+"\x00"+strings.ToLower(strings.TrimSpace(c.expression))]
		if !ok {
			card, ok = vocabByExpr[strings.ToLower(strings.TrimSpace(c.expression))]
		}
		if !ok {
			continue // no vocab data to grade/display against
		}
		cards = append(cards, RelearnCard{
			Entry: c.expression, NotebookName: c.notebookName, SourceQuizType: c.sourceQT,
			IsEtymology: false, ClearKey: key, Meaning: card.Meaning,
			WordDetail: card.WordDetail, Images: card.Images,
			Examples:      relearnExamplesFromContexts(card.Contexts),
			ContextScenes: relearnScenesFromCard(card),
			vocabCard: Card{
				NotebookName: card.NotebookName, StoryTitle: card.StoryTitle, SceneTitle: card.SceneTitle,
				Entry: card.Expression, OriginalEntry: card.OriginalExpression, Meaning: card.Meaning,
				Contexts: card.Contexts, WordDetail: card.WordDetail, Images: card.Images,
			},
		})
	}
	return cards, nil
}

// relearnSeriesSpec describes one learning-log series to inspect for a wrong word.
type relearnSeriesSpec struct {
	logs        []notebook.LearningRecord
	isEtymology bool
	defaultQT   notebook.QuizType
}

// relearnSeries returns the four independent log series an expression can carry.
// Notebook and freeform share LearnedLogs; the recorded QuizType on the log
// distinguishes them for the source label.
func relearnSeries(expr notebook.LearningHistoryExpression) []relearnSeriesSpec {
	return []relearnSeriesSpec{
		{logs: expr.LearnedLogs, isEtymology: false, defaultQT: notebook.QuizTypeNotebook},
		{logs: expr.ReverseLogs, isEtymology: false, defaultQT: notebook.QuizTypeReverse},
		{logs: expr.EtymologyBreakdownLogs, isEtymology: true, defaultQT: notebook.QuizTypeEtymologyStandard},
		{logs: expr.EtymologyAssemblyLogs, isEtymology: true, defaultQT: notebook.QuizTypeEtymologyReverse},
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
			byNotebookExpr[strings.ToLower(w.NotebookName)+"\x00"+e] = w
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

// relearnScenesFromCard turns a vocab card's contexts into a single
// context scene keyed by the card's scene. The expression's sentences render
// as prose statements on the feedback screen.
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
// so the answering screen can show a hint, matching the standard quiz card.
func relearnExamplesFromContexts(contexts []inference.Context) []Example {
	var out []Example
	for _, c := range contexts {
		if s := strings.TrimSpace(c.Context); s != "" {
			out = append(out, Example{Text: s})
		}
	}
	return out
}
