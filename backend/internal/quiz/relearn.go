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
//	QuizTypeEtymologyOrigin   show Entry (origin), ask the Meaning
//	QuizTypeGrammar           correction: show Content with Incorrect struck
//	                          through, ask for the fix — the live grammar
//	                          quiz's own inline-correction card, reused as-is.
//
// A word failed in several quiz types yields one card per type. Nothing about a
// RelearnCard is ever persisted — the Relearn Quiz writes no learning history
// and no other state.
type RelearnCard struct {
	Format       notebook.QuizType
	Entry        string
	Meaning      string
	NotebookName string

	// Etymology display extras (empty for vocab cards).
	OriginType string
	Language   string
	// Literal is the etymology literal gloss (e.g. `de "down" + facere = "made
	// down"`), sourced from the word's definitions note (Note.Note). Shown on the
	// etymology-origin Relearn feedback, mirroring the quiz. Empty for other formats.
	Literal string

	// Grammar display extras (empty for vocab/etymology cards). Content is
	// the journal entry's full text; Incorrect is the mistaken span struck
	// through in it, exactly as the live grammar quiz renders one blank.
	Content   string
	Incorrect string

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
	grammarCard   GrammarBlank
}

// VocabCard, ReverseCard, EtymologyCard, GrammarCard return the card the
// matching pure grader consumes for this Format.
func (c RelearnCard) VocabCard() Card                    { return c.vocabCard }
func (c RelearnCard) ReverseCard() ReverseCard           { return c.reverseCard }
func (c RelearnCard) EtymologyCard() EtymologyOriginCard { return c.etymologyCard }
func (c RelearnCard) GrammarCard() GrammarBlank          { return c.grammarCard }

// IsEtymology reports whether the card's Format is the etymology mode.
func (c RelearnCard) IsEtymology() bool {
	return c.Format == notebook.QuizTypeEtymologyOrigin
}

// IsGrammar reports whether the card's Format is the grammar mode.
func (c RelearnCard) IsGrammar() bool {
	return c.Format == notebook.QuizTypeGrammar
}

// relearnKeySep separates the fields of the internal de-dup key ((format,
// notebook, expression)). It is the ASCII Unit Separator (0x1F), which cannot
// appear in notebook names or expressions.
const relearnKeySep = "\x1f"

// relearnCandidate is an intermediate per-format wrong-word record before it is
// resolved to a gradeable card.
type relearnCandidate struct {
	notebookName string
	expression   string
	id           string // stable source-entry identity of the failed entry; "" for legacy
	format       notebook.QuizType
	latestWrong  time.Time
}

// LoadRelearnPool builds the Relearn Quiz pool: for every learning-log series
// (recognition, reverse, etymology origin) whose
// most-recent log within [windowStart, now] has status "misunderstood", it
// emits one card that mirrors that series' quiz type. A word failed in several
// types produces several cards.
//
// It reads the YAML learning histories directly — the source of truth and the
// only place etymology-origin results are stored — so the pool spans both
// vocabulary and etymology words regardless of whether a database is
// configured. It writes nothing, and persists nothing: every in-window wrong
// word appears in every session until it ages out of the window or is answered
// correctly in a real quiz, so the learner can re-drill it as often as needed.
func (s *Service) LoadRelearnPool(windowStart time.Time) ([]RelearnCard, error) {
	histories, err := notebook.NewLearningHistories(s.notebooksConfig.LearningNotesDirectory)
	if err != nil {
		return nil, fmt.Errorf("load learning histories: %w", err)
	}

	// One candidate per (format, notebook, expression); the same expression can
	// recur across scenes (multi-sense etymology), so keep the most-recent wrong.
	candidates := make(map[string]relearnCandidate)
	consider := func(notebookName, metadataType string, expr notebook.LearningHistoryExpression) {
		for _, sp := range relearnSeries(metadataType, expr) {
			if len(sp.logs) == 0 {
				continue
			}
			latest := sp.logs[0] // newest-first
			if latest.LearnedAt.Before(windowStart) || latest.Status != notebook.LearnedStatusMisunderstood {
				continue
			}
			// Key by id when present so same-spelling homographs stay
			// distinct; legacy id-less entries fall back to the expression.
			key := string(sp.format) + relearnKeySep + notebookName + relearnKeySep + strings.ToLower(strings.TrimSpace(expr.Expression)) + relearnKeySep + expr.ID
			if existing, ok := candidates[key]; ok && !latest.LearnedAt.After(existing.latestWrong) {
				continue
			}
			candidates[key] = relearnCandidate{
				notebookName: notebookName,
				expression:   expr.Expression,
				id:           expr.ID,
				format:       sp.format,
				latestWrong:  latest.LearnedAt.Time,
			}
		}
	}
	for notebookName, hs := range histories {
		for _, h := range hs {
			for _, expr := range h.Expressions { // flashcard/grammar-level
				consider(notebookName, h.Metadata.Type, expr)
			}
			for _, scene := range h.Scenes { // story/etymology scene-level
				for _, expr := range scene.Expressions {
					consider(notebookName, h.Metadata.Type, expr)
				}
			}
		}
	}

	candidatesFound := len(candidates)
	if candidatesFound == 0 {
		return nil, nil
	}

	vocabByID, vocabByExpr, vocabByNotebookExpr, err := s.relearnVocabIndex()
	if err != nil {
		return nil, err
	}
	grammarByID, err := s.relearnGrammarIndex()
	if err != nil {
		return nil, err
	}

	// resolveWord maps a wrong-word candidate to its vocabulary card, mirroring
	// MatchesEntry: by stable id first (so same-spelling homographs never
	// collide), then by (notebook, expression), then by expression alone for
	// legacy id-less candidates. Used by both the recognition/reverse branch and
	// the etymology-origin branch, whose missed items are now WORDS resolved the
	// same way (invariant L2).
	resolveWord := func(c relearnCandidate) (FreeformCard, bool) {
		if c.id != "" {
			if fc, ok := vocabByID[c.id]; ok {
				return fc, true
			}
		}
		if fc, ok := vocabByNotebookExpr[strings.ToLower(c.notebookName)+relearnKeySep+strings.ToLower(strings.TrimSpace(c.expression))]; ok {
			return fc, true
		}
		if fc, ok := vocabByExpr[strings.ToLower(strings.TrimSpace(c.expression))]; ok {
			return fc, true
		}
		return FreeformCard{}, false
	}

	// A definitions concept member (e.g. "consummate", grouped with its
	// derived forms) is shown and graded by the standard quiz under the
	// concept HEAD and its umbrella meaning. Relearn must do the same, or it
	// resolves the member by last-write-wins and shows a different meaning
	// than the quiz it was failed in. Build the same family-concept index the
	// loaders use, lazily per notebook.
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("init reader for relearn concepts: %w", err)
	}
	conceptByNotebook := map[string]map[string]*conceptInfo{}
	conceptFor := func(notebookName, expression string) *conceptInfo {
		idx, ok := conceptByNotebook[notebookName]
		if !ok {
			idx = buildConceptIndex(reader, notebookName)
			conceptByNotebook[notebookName] = idx
		}
		if idx == nil {
			return nil
		}
		return idx[expression]
	}

	cards := make([]RelearnCard, 0, len(candidates))
	for _, c := range candidates {
		if c.format == notebook.QuizTypeGrammar {
			// A grammar candidate's id and expression are both the
			// correction's senseID (SaveGrammarBlank writes Expression:
			// senseID, SenseID: senseID) — try id first for symmetry with
			// the vocab branch below, falling back to expression for
			// legacy id-less candidates.
			entries, ok := grammarByID[c.id]
			if !ok {
				entries, ok = grammarByID[c.expression]
			}
			if !ok {
				continue // no due grammar post/blank to grade/display against
			}
			// Two DISTINCT corrections can share one senseID (a duplicate
			// explicit `id:`, or two titles that slugify alike), so a single
			// due series can back several blanks. Emit ONE card per blank —
			// each with its own mistaken span — rather than folding them to
			// last-write-wins, which silently dropped every blank but one.
			// relearnCardID folds the span in, so the cards get distinct
			// note_ids and grade independently.
			for _, entry := range entries {
				cards = append(cards, RelearnCard{
					// NotebookName carries the notebook ID (c.notebookName, from the
					// learning-history metadata) — not the display name — so a
					// deliberate Exclude (SkipWord) resolves to the correct
					// <notebookID>.yml, matching the live grammar quiz's grammarStore
					// (which also skips by notebook ID). The frontend never shows a
					// notebook name on a grammar relearn post.
					Format: c.format, Entry: c.expression, NotebookName: c.notebookName,
					Content: entry.post.Content, Incorrect: entry.blank.Incorrect,
					grammarCard: entry.blank,
				})
			}
			continue
		}
		if c.format == notebook.QuizTypeEtymologyOrigin {
			// The etymology-origin schedule is now per-WORD: a missed item is a
			// derived word (its EtymologyOriginLogs series lives on the word's
			// own learning-history entry), not the origin. Re-drill the word by
			// asking its meaning — exactly how the origin card quizzes it
			// (GradeEtymologyWordAnswer against the word's meaning) — resolving
			// the word through the same vocab index and skipping any word the
			// learner excluded from the etymology-origin quiz (invariant L2).
			if notebook.IsExpressionExcludedForQuizType(
				histories[c.notebookName], c.id, notebook.QuizTypeEtymologyOrigin, c.expression,
			) {
				continue
			}
			fc, ok := resolveWord(c)
			if !ok {
				continue // no word data to grade/display against
			}
			cards = append(cards, RelearnCard{
				Format: c.format, Entry: c.expression, Meaning: fc.Meaning, NotebookName: c.notebookName,
				WordDetail: fc.WordDetail, Images: fc.Images, Literal: fc.Literal,
				ContextScenes: relearnScenesFromCard(fc),
				// Grade the meaning against the word's own gloss, so a re-drill
				// matches how the origin card scored it.
				etymologyCard: EtymologyOriginCard{Meaning: fc.Meaning},
			})
			continue
		}
		// Resolve by id first (mirrors MatchesEntry: an id-bearing failed
		// entry resolves to its own card, so same-spelling homographs never
		// collide). Fall back to the sense-less expression lookups for
		// legacy id-less candidates or an id miss.
		fc, ok := resolveWord(c)
		if !ok {
			continue // no vocab data to grade/display against
		}
		// If this word is a family-concept member, present and grade it under
		// the concept head + umbrella meaning, exactly as the standard quiz
		// does — so a homograph folded into a concept (e.g. "consummate")
		// never shows one sense here and another there.
		displayEntry := c.expression
		if ci := conceptFor(c.notebookName, c.expression); ci != nil && ci.Head != "" {
			fc.Meaning = ci.Meaning
			fc.Expression = ci.Head
			displayEntry = ci.Head
		}
		card := RelearnCard{
			Format: c.format, Entry: displayEntry, Meaning: fc.Meaning, NotebookName: c.notebookName,
			WordDetail: fc.WordDetail, Images: fc.Images,
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
	// One line so a short pool can be diagnosed from the server log: how many
	// wrong words were in the window vs. how many matched a gradeable card.
	slog.Info("relearn pool built", "in_window_misunderstood", candidatesFound, "matched_cards", len(cards))
	return cards, nil
}

// relearnSeriesSpec describes one learning-log series to inspect for a wrong
// word, and the relearn card format it maps to.
type relearnSeriesSpec struct {
	logs   []notebook.LearningRecord
	format notebook.QuizType
}

// relearnSeries returns the independent log series an expression can carry,
// each mapped to the relearn card format that mirrors it. Notebook and freeform
// share LearnedLogs and both replay as recognition; the etymology-origin series
// (now per-WORD, not per-origin) replays as an etymology card that re-drills the
// word by asking its meaning.
//
// metadataType is the owning LearningHistory's Metadata.Type — the same
// value flatTypeForStory (learning_history.go) derives at write time for the
// flat "journal" bucket. A grammar entry only ever writes LearnedLogs (see
// SaveGrammarBlank), so it gets a single series mapped to QuizTypeGrammar
// instead of the vocab/etymology series; reusing this one check (rather than
// re-deriving "is this a grammar entry" from the expression shape) is what
// keeps this classification symmetric with the writer (L2).
func relearnSeries(metadataType string, expr notebook.LearningHistoryExpression) []relearnSeriesSpec {
	if metadataType == string(notebook.QuizTypeGrammar) {
		return []relearnSeriesSpec{
			{logs: expr.LearnedLogs, format: notebook.QuizTypeGrammar},
		}
	}
	return []relearnSeriesSpec{
		{logs: expr.LearnedLogs, format: notebook.QuizTypeNotebook},
		{logs: expr.ReverseLogs, format: notebook.QuizTypeReverse},
		{logs: expr.EtymologyOriginLogs, format: notebook.QuizTypeEtymologyOrigin},
	}
}

// relearnVocabIndex loads every vocabulary word once and indexes it by stable
// id (the canonical key), and — as a legacy fallback for id-less candidates —
// also by (notebook, expression) and by expression alone, so the pool can
// resolve a wrong word to its meaning and context.
func (s *Service) relearnVocabIndex() (byID map[string]FreeformCard, byExpr map[string]FreeformCard, byNotebookExpr map[string]FreeformCard, err error) {
	words, err := s.LoadAllWords()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load words for relearn pool: %w", err)
	}
	byID = make(map[string]FreeformCard, len(words))
	byExpr = make(map[string]FreeformCard, len(words))
	byNotebookExpr = make(map[string]FreeformCard, len(words))
	for _, w := range words {
		if w.ID != "" {
			byID[w.ID] = w
		}
		for _, e := range []string{w.Expression, w.OriginalExpression} {
			e = strings.ToLower(strings.TrimSpace(e))
			if e == "" {
				continue
			}
			byExpr[e] = w
			byNotebookExpr[strings.ToLower(w.NotebookName)+relearnKeySep+e] = w
		}
	}
	return byID, byExpr, byNotebookExpr, nil
}

// relearnGrammarEntry pairs a due grammar blank with the post it belongs to,
// so the relearn card can show the whole entry (Content) with the missed
// span (Incorrect) struck through — exactly like the live grammar quiz.
type relearnGrammarEntry struct {
	post  GrammarPost
	blank GrammarBlank
}

// relearnGrammarIndex loads every grammar-drilled journal once and indexes the
// due blanks by their stable correction id (senseID), for grading and display.
// It reuses LoadGrammarPosts — the same loader the live grammar quiz calls — so
// a just-missed correction (status "misunderstood") is always "due"
// (NeedsForwardReview treats misunderstood as always-due) and therefore always
// present in the index the moment it lands in the relearn pool.
//
// The value is a SLICE, not a single entry: two DISTINCT corrections can share
// one senseID (a duplicate explicit `id:`, or two titles that slugify alike),
// and each must survive as its own blank. Keying last-write-wins collapsed them
// to a single card, so every blank of the post but one silently vanished from
// Relearn.
func (s *Service) relearnGrammarIndex() (map[string][]relearnGrammarEntry, error) {
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("init reader for relearn grammar pool: %w", err)
	}
	byID := make(map[string][]relearnGrammarEntry)
	for _, storyID := range reader.GrammarStoryIDs() {
		posts, err := s.LoadGrammarPosts(storyID, nil)
		if err != nil {
			return nil, fmt.Errorf("load grammar posts for relearn pool (%s): %w", storyID, err)
		}
		for _, post := range posts {
			for _, blank := range post.Blanks {
				byID[blank.SenseID] = append(byID[blank.SenseID], relearnGrammarEntry{post: post, blank: blank})
			}
		}
	}
	return byID, nil
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
			MaskedContext: maskWord(text, fc.Expression, fc.OriginalExpression, ""),
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
