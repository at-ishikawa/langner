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

// OriginFamilyMember is one word of an origin family shown as post-answer
// reference on the Relearn origin card: the word and a short meaning/gloss.
type OriginFamilyMember struct {
	Word    string
	Meaning string
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
//	QuizTypeEtymologyOrigin   an origin-bearing miss (recognition OR reverse):
//	                          carries OriginText/OriginMeaning so the frontend
//	                          groups every card sharing an origin into one family
//	                          card. Direction selects how the word is drilled
//	                          within it — recognition (show word, ask meaning) or
//	                          reverse (show meaning + masked contexts, ask word).
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

	// Direction is set ONLY for a QuizTypeEtymologyOrigin card: the direction
	// the grouped word is drilled in — the quiz type it was missed in. It is
	// QuizTypeNotebook (recognition: show the word, ask its meaning, grade via
	// vocabCard) or QuizTypeReverse (production: show the meaning + masked
	// contexts, ask the word, grade via reverseCard). One origin family card can
	// mix directions. Empty for every non-etymology Format.
	Direction notebook.QuizType

	// Etymology display extras (empty for vocab cards). OriginText is the
	// source-language origin the missed word derives from (e.g. "facere") and
	// OriginMeaning is that origin's English gloss; together with OriginType and
	// Language they form the origin header of the Relearn family card. Every
	// etymology card sharing an OriginText/OriginMeaning is grouped by the
	// frontend into ONE family card listing the missed words under that origin.
	OriginType    string
	Language      string
	OriginText    string
	OriginMeaning string
	// EnglishForms are the origin's English combining-form spellings (e.g. the
	// Latin root "liber" surfaces as ["lib", "liv"]). Resolved from the full
	// EtymologyOrigin definition (the vocab word's origin_parts don't carry
	// them) and shown as chips on the origin header of the Relearn family card.
	// Study context only — never quizzed. Empty for non-etymology cards.
	EnglishForms []string
	// Literal is the etymology literal gloss (e.g. `de "down" + facere = "made
	// down"`), sourced from the word's definitions note (Note.Note). Shown on the
	// etymology Relearn feedback. Empty for other formats.
	Literal string
	// RelatedWords are the OTHER words sharing this card's origin that are NOT
	// drilled on this card — display-only reference (word + short meaning) shown
	// AFTER answering so the learner sees the wider family. Excludes the drilled
	// words and any word whose skipped_at exclude marker is set. Populated only
	// for QuizTypeEtymologyOrigin cards; never quizzed, never persisted.
	RelatedWords []OriginFamilyMember

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

	// Grading inputs — one populated per Format. An etymology-origin card is an
	// origin-bearing recognition miss, so it is graded through vocabCard too.
	vocabCard   Card
	reverseCard ReverseCard
	grammarCard GrammarBlank
}

// VocabCard, ReverseCard, GrammarCard return the card the matching pure grader
// consumes for this Format.
func (c RelearnCard) VocabCard() Card           { return c.vocabCard }
func (c RelearnCard) ReverseCard() ReverseCard  { return c.reverseCard }
func (c RelearnCard) GrammarCard() GrammarBlank { return c.grammarCard }

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
// (recognition, reverse) whose most-recent log within [windowStart, now] has
// status "misunderstood", it emits one card that mirrors that series' quiz type.
// A word failed in several types produces several cards. A recognition miss of a
// word that carries an etymology origin is emitted as an origin-family card so
// the frontend can group missed words by their shared root.
//
// It reads the YAML learning histories directly — the source of truth — so the
// pool spans every notebook regardless of whether a database is
// configured. It writes nothing, and persists nothing: every in-window wrong
// item — vocabulary AND grammar — appears in every session until it ages out of
// the window or is answered correctly in a real quiz, so the learner can
// re-drill it as often as needed. Grammar corrections honor the SAME recent-miss
// window as vocabulary: an old grammar mistake the learner hasn't practiced
// lately ages out of the Relearn re-drill pool. Ageing out does NOT lose it — the
// live grammar quiz still serves a still-"misunderstood" correction (always due
// via NeedsForwardReview), so missing it again there re-records a fresh miss that
// brings it back into the window. A word deliberately excluded from a quiz mode
// (per-quiz-type skipped_at set via SkipWord) never enters the pool, matching the
// normal card loaders (quiz-ui-invariants U1).
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
			// A word deliberately EXCLUDED from a quiz mode carries a per-quiz-type
			// skipped_at marker (set only via SkipWord). Every normal card loader
			// filters those out, so the Relearn pool must too — an excluded word
			// must never be re-drilled (quiz-ui-invariants U1). This gates on the
			// ACTIVE exclude marker (notebook / reverse / grammar); it is distinct
			// from the vestigial "etymology_origin" marker the origin-family
			// grouping below deliberately ignores.
			if expr.SkippedAt.IsSkipped(sp.format) {
				continue
			}
			latest := sp.logs[0] // newest-first
			if latest.Status != notebook.LearnedStatusMisunderstood {
				continue
			}
			// Every series — vocabulary AND grammar — honors the recent-miss
			// window: a miss older than windowStart ages out of the Relearn
			// re-drill pool, so the learner is not re-shown stale problems they
			// have not practiced lately. This applies to grammar corrections too
			// (a still-"misunderstood" correction the learner hasn't touched in a
			// while drops out here). It is not lost: the live grammar quiz still
			// serves it (misunderstood is always due via NeedsForwardReview), and
			// missing it again there re-records a fresh miss that brings it back
			// into the window.
			if latest.LearnedAt.Before(windowStart) {
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

	// One failed WORD must be re-asked once, not once per track. A freeform miss
	// mirror-writes BOTH the recognition (LearnedLogs) and the reverse
	// (ReverseLogs) series (yaml_repository freeform dual-write), and a word can
	// also be genuinely missed in both the standard and the reverse quiz — either
	// way it is a single failed word. Collapse the two vocab series of the same
	// word into one candidate so K failed words yield exactly K re-ask cards,
	// deduped by expression (grammar candidates, keyed by correction span rather
	// than expression, are never merged). Origin-bearing words are already folded
	// once via etymByWord below; collapsing first leaves that path unchanged.
	candidates = collapseVocabDirections(candidates)

	candidatesFound := len(candidates)
	if candidatesFound == 0 {
		return nil, nil
	}

	words, err := s.LoadAllWords()
	if err != nil {
		return nil, fmt.Errorf("load words for relearn pool: %w", err)
	}
	vocabByID, vocabByExpr, vocabByNotebookExpr := relearnVocabIndex(words)
	// originFamilies groups every non-excluded word by the SAME origin its miss
	// would fold under (primaryOriginPart), so an origin card can show the wider
	// word family as post-answer reference. Excluded words (skipped_at) are left
	// out (fix #1: Relearn never surfaces excluded words).
	originFamilies := buildRelearnOriginFamilies(words)
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
	// originMap keys the full EtymologyOrigin definitions (origin|language,
	// lowered) — the same index the vocab load path uses (buildOriginMap) — so
	// an origin-family relearn card can carry the origin's english_forms, which
	// the word's own origin_parts do not hold.
	originMap := buildOriginMap(reader)
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

	// Origin-bearing vocabulary misses (recognition AND reverse) are folded into
	// origin family cards. A word missed in several directions must appear ONCE
	// in its family, drilled in a single direction, so collect its directions
	// here first and emit one card per word after the candidate loop. etymOrder
	// preserves first-seen order for a stable pool.
	etymByWord := map[string]*etymPending{}
	var etymOrder []string

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
		// Resolve by id first (mirrors MatchesEntry: an id-bearing failed
		// entry resolves to its own card, so same-spelling homographs never
		// collide). Fall back to the sense-less expression lookups for
		// legacy id-less candidates or an id miss.
		fc, ok := resolveWord(c)
		if !ok {
			continue // no vocab data to grade/display against
		}
		// A miss of a word that carries an etymology origin — in recognition OR
		// reverse — is re-drilled inside an origin FAMILY card: the frontend
		// groups every such card sharing an origin into one screen (origin +
		// meaning + the missed words), which is what helps the learner see the
		// shared root. Each word is still graded in the direction it was missed
		// (recognition: typed meaning vs gloss; reverse: typed word vs
		// expression). Directions are collected here and emitted as one card per
		// word after the loop so a word missed both ways is shown once.
		//
		// Grouping depends ONLY on the word carrying a resolvable origin. It is
		// deliberately NOT gated on a per-quiz-type "etymology_origin" skip
		// marker: since #41 the standalone etymology-origin quiz is gone, Relearn
		// has no Exclude control, and NOTHING deliberately sets or clears that
		// marker anymore (quiz-ui-invariants). Any surviving "etymology_origin"
		// skip is therefore vestigial — written only by the removed quiz or by
		// the old "Don't Know" bug (4c7fd4de/991816dd) — and honoring it here
		// only suppressed the family card for words whose origin resolves fine,
		// leaving them stuck as plain "Reverse — recall the word" cards forever
		// (the reported symptom). The normal-quiz skip filtering (notebook /
		// reverse / freeform skipped_at, enforced by the card loaders) is a
		// separate, correct path and is untouched.
		if c.format == notebook.QuizTypeNotebook || c.format == notebook.QuizTypeReverse {
			if op, hasOrigin := primaryOriginPart(fc); hasOrigin {
				key := strings.ToLower(c.notebookName) + relearnKeySep + c.id + relearnKeySep + strings.ToLower(strings.TrimSpace(c.expression))
				p, ok := etymByWord[key]
				if !ok {
					p = &etymPending{fc: fc, op: op, notebookName: c.notebookName}
					etymByWord[key] = p
					etymOrder = append(etymOrder, key)
				}
				if c.format == notebook.QuizTypeReverse {
					p.reverse = true
				}
				continue
			}
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
			card.Examples = relearnExampleHints(fc)
			card.vocabCard = Card{
				NotebookName: fc.NotebookName, StoryTitle: fc.StoryTitle, SceneTitle: fc.SceneTitle,
				Entry: fc.Expression, OriginalEntry: fc.OriginalExpression, Meaning: fc.Meaning,
				Contexts: relearnRecognitionContexts(fc), WordDetail: fc.WordDetail, Images: fc.Images,
			}
		}
		cards = append(cards, card)
	}

	// The set of DRILLED words per origin, so the related-words reference on each
	// family card excludes the words the card itself quizzes.
	drilledByOrigin := map[string]map[string]bool{}
	for _, key := range etymOrder {
		p := etymByWord[key]
		ok := originFamilyKey(p.op.Origin, p.op.Language)
		if drilledByOrigin[ok] == nil {
			drilledByOrigin[ok] = map[string]bool{}
		}
		drilledByOrigin[ok][strings.ToLower(strings.TrimSpace(p.fc.Expression))] = true
	}

	// Emit one origin family card per origin-bearing word, in the direction it
	// was missed. A word missed in BOTH directions is drilled ONCE, in the
	// reverse direction — producing the word is the stronger recall test — so it
	// is never rendered twice in one family card (design choice, see doc above).
	for _, key := range etymOrder {
		p := etymByWord[key]
		direction := notebook.QuizTypeNotebook
		if p.reverse {
			direction = notebook.QuizTypeReverse
		}
		card := buildEtymologyOriginCard(p.fc, p.op, direction, p.notebookName, originMap)
		originKey := originFamilyKey(p.op.Origin, p.op.Language)
		card.RelatedWords = relatedOriginWords(originFamilies[originKey], drilledByOrigin[originKey])
		cards = append(cards, card)
	}

	// One line so a short pool can be diagnosed from the server log: how many
	// wrong words were in the window vs. how many matched a gradeable card.
	slog.Info("relearn pool built", "in_window_misunderstood", candidatesFound, "matched_cards", len(cards))
	return cards, nil
}

// etymPending accumulates the directions one origin-bearing word was missed in
// before it is emitted as a single origin family card (a word missed both ways
// is drilled once). fc and op are resolved once; they do not depend on the miss
// direction.
type etymPending struct {
	fc           FreeformCard
	op           WordOriginPart
	notebookName string
	// reverse is set when the word was missed in the reverse quiz. A word missed
	// only in recognition leaves it false (the default direction); a word missed
	// in reverse — with or without a recognition miss too — is drilled reverse.
	reverse bool
}

// buildEtymologyOriginCard assembles the origin family card for one missed
// word, drilled in the given direction. Recognition grades the typed meaning
// against the word's gloss (vocabCard); reverse grades the typed word against
// the expression (reverseCard) and shows the meaning + masked contexts. Both
// carry the same origin header and the same feedback context scenes, so a
// reverse word shows its example statements exactly like a recognition word
// (quiz-ui-invariants U3).
func buildEtymologyOriginCard(fc FreeformCard, op WordOriginPart, direction notebook.QuizType, notebookName string, originMap map[string]notebook.EtymologyOrigin) RelearnCard {
	card := RelearnCard{
		Format: notebook.QuizTypeEtymologyOrigin, Direction: direction,
		Entry: fc.Expression, Meaning: fc.Meaning, NotebookName: notebookName,
		OriginType: op.Type, Language: op.Language,
		OriginText: op.Origin, OriginMeaning: op.Meaning,
		EnglishForms: originEnglishForms(originMap, op.Origin, op.Language),
		WordDetail:   fc.WordDetail, Images: fc.Images, Literal: fc.Literal,
		ContextScenes: relearnScenesFromCard(fc),
	}
	if direction == notebook.QuizTypeReverse {
		masked := relearnMaskedContexts(fc)
		card.Contexts = masked
		card.reverseCard = ReverseCard{
			NotebookName: fc.NotebookName, StoryTitle: fc.StoryTitle, SceneTitle: fc.SceneTitle,
			Meaning: fc.Meaning, Contexts: masked, Expression: fc.Expression, AltForm: fc.OriginalExpression,
			WordDetail: fc.WordDetail, Images: fc.Images,
		}
		return card
	}
	card.Examples = relearnExampleHints(fc)
	card.vocabCard = Card{
		NotebookName: fc.NotebookName, StoryTitle: fc.StoryTitle, SceneTitle: fc.SceneTitle,
		Entry: fc.Expression, OriginalEntry: fc.OriginalExpression, Meaning: fc.Meaning,
		Contexts: relearnRecognitionContexts(fc), WordDetail: fc.WordDetail, Images: fc.Images,
	}
	return card
}

// relearnSeriesSpec describes one learning-log series to inspect for a wrong
// word, and the relearn card format it maps to.
type relearnSeriesSpec struct {
	logs   []notebook.LearningRecord
	format notebook.QuizType
}

// relearnSeries returns the independent log series an expression can carry,
// each mapped to the relearn card format that mirrors it. Notebook and freeform
// share LearnedLogs and both replay as recognition; ReverseLogs replay as a
// reverse card. Etymology is no longer a separate series — an etymology word is
// a normal vocabulary word, so a missed word that carries an origin surfaces
// through its recognition series and is regrouped by origin at pool-build time.
//
// metadataType is the owning LearningHistory's Metadata.Type — the same
// value flatTypeForStory (learning_history.go) derives at write time for the
// flat "journal" bucket. A grammar entry only ever writes LearnedLogs (see
// SaveGrammarBlank), so it gets a single series mapped to QuizTypeGrammar
// instead of the vocab series; reusing this one check (rather than re-deriving
// "is this a grammar entry" from the expression shape) is what keeps this
// classification symmetric with the writer (L2).
func relearnSeries(metadataType string, expr notebook.LearningHistoryExpression) []relearnSeriesSpec {
	if metadataType == string(notebook.QuizTypeGrammar) {
		return []relearnSeriesSpec{
			{logs: expr.LearnedLogs, format: notebook.QuizTypeGrammar},
		}
	}
	return []relearnSeriesSpec{
		{logs: expr.LearnedLogs, format: notebook.QuizTypeNotebook},
		{logs: expr.ReverseLogs, format: notebook.QuizTypeReverse},
	}
}

// collapseVocabDirections folds the recognition (notebook) and reverse candidates
// of the SAME word (keyed by notebook + id + expression) into a single candidate,
// so one failed word is re-asked once rather than once per learning-history track.
// When a word was missed in both directions it is drilled in REVERSE — producing
// the word is the stronger recall test, the same both-directions rule the origin
// family card uses (buildEtymologyOriginCard). Grammar candidates are keyed by
// correction span, not expression, so they are passed through untouched (two
// distinct blanks of one post must each survive). latestWrong keeps the most
// recent of the merged series so window ageing is unaffected.
func collapseVocabDirections(candidates map[string]relearnCandidate) map[string]relearnCandidate {
	out := make(map[string]relearnCandidate, len(candidates))
	for _, c := range candidates {
		if c.format == notebook.QuizTypeGrammar {
			out[string(c.format)+relearnKeySep+c.notebookName+relearnKeySep+strings.ToLower(strings.TrimSpace(c.expression))+relearnKeySep+c.id] = c
			continue
		}
		wordKey := c.notebookName + relearnKeySep + c.id + relearnKeySep + strings.ToLower(strings.TrimSpace(c.expression))
		existing, ok := out[wordKey]
		if !ok {
			out[wordKey] = c
			continue
		}
		// Prefer the reverse direction; keep the most recent miss timestamp.
		merged := existing
		if c.format == notebook.QuizTypeReverse {
			merged.format = notebook.QuizTypeReverse
		}
		if c.latestWrong.After(merged.latestWrong) {
			merged.latestWrong = c.latestWrong
		}
		out[wordKey] = merged
	}
	return out
}

// relearnVocabIndex indexes the given vocabulary words by stable id (the
// canonical key), and — as a legacy fallback for id-less candidates — also by
// (notebook, expression) and by expression alone, so the pool can resolve a
// wrong word to its meaning and context. The caller loads the words once (with
// s.LoadAllWords) and shares the slice with the origin-family builder.
func relearnVocabIndex(words []FreeformCard) (byID map[string]FreeformCard, byExpr map[string]FreeformCard, byNotebookExpr map[string]FreeformCard) {
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
	return byID, byExpr, byNotebookExpr
}

// originFamilyKey is the shared (origin|language) key, lowercased, that both the
// origin-family index and the per-card lookup use, so grouping stays a
// single-rule, single-place decision (learning-history-invariants L2).
func originFamilyKey(origin, language string) string {
	return strings.ToLower(strings.TrimSpace(origin) + "|" + strings.TrimSpace(language))
}

// buildRelearnOriginFamilies groups every word by the SAME origin its miss would
// fold under (primaryOriginPart), so an origin card can show the wider word
// family as post-answer reference. This is display-only REFERENCE, so it does
// NOT filter skipped_at-excluded words: a word the learner excluded from quizzes
// (learned it, turned quizzing off) is exactly a known same-origin word worth
// showing. Only the quiz/drilled pool filters skipped_at (fix #1); this reference
// must not. Members are de-duplicated by expression within each origin. The
// drilled words are removed later by relatedOriginWords.
func buildRelearnOriginFamilies(words []FreeformCard) map[string][]OriginFamilyMember {
	families := map[string][]OriginFamilyMember{}
	seen := map[string]map[string]bool{}
	for _, w := range words {
		op, ok := primaryOriginPart(w)
		if !ok {
			continue
		}
		expr := strings.TrimSpace(w.Expression)
		if expr == "" {
			continue
		}
		// NOTE: the related-words list is display-only REFERENCE, so it does NOT
		// filter skipped_at-excluded words. A word the learner excluded from
		// quizzes (they learned it and turned quizzing off) is exactly a "known
		// word from this origin" worth showing as reference. Only the DRILLED /
		// quiz pool filters skipped_at (fix #1); this reference must not.
		key := originFamilyKey(op.Origin, op.Language)
		exprLow := strings.ToLower(expr)
		if seen[key] == nil {
			seen[key] = map[string]bool{}
		}
		if seen[key][exprLow] {
			continue
		}
		seen[key][exprLow] = true
		families[key] = append(families[key], OriginFamilyMember{Word: expr, Meaning: w.Meaning})
	}
	return families
}

// relatedOriginWords returns the family members that are NOT among the drilled
// words on this card (the card already quizzes those), preserving order.
func relatedOriginWords(family []OriginFamilyMember, drilled map[string]bool) []OriginFamilyMember {
	if len(family) == 0 {
		return nil
	}
	var out []OriginFamilyMember
	for _, m := range family {
		if drilled[strings.ToLower(strings.TrimSpace(m.Word))] {
			continue
		}
		out = append(out, m)
	}
	return out
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
// serve as a hint without giving away the answer. It draws from BOTH the card's
// story-scene Contexts AND its own `examples:` sentences (fc.Examples) — a plain
// definitions/flashcard word carries its usage only in Examples, so without this
// a reverse relearn card for such a word would show no hint at all. Each example
// is masked by its per-example Highlight (the exact surface form to hide) in
// addition to the Expression/OriginalExpression. An example whose answer masking
// leaves visible — an inflected surface form with no highlight (e.g. "evicted"
// for lemma "evict") — is DROPPED rather than shown (reverseHintContext), so the
// reverse hint never reveals the answer. Sentences are de-duplicated by text so a
// word carrying the same sentence in both lists is shown once.
func relearnMaskedContexts(fc FreeformCard) []ReverseContext {
	var out []ReverseContext
	seen := map[string]bool{}
	add := func(text, highlight string) {
		text = strings.TrimSpace(text)
		if text == "" || seen[strings.ToLower(text)] {
			return
		}
		seen[strings.ToLower(text)] = true
		// Only show the example if masking actually hid the answer; an inflected
		// form with no highlight would otherwise leak the word (reverse must never
		// reveal the answer).
		if rc, ok := reverseHintContext(text, fc.Expression, fc.OriginalExpression, highlight); ok {
			out = append(out, rc)
		}
	}
	for _, c := range fc.Contexts {
		add(c.Context, "")
	}
	for _, ex := range fc.Examples {
		add(ex.Text, ex.Highlight)
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

// relearnScenesFromCard turns a vocab card's usage sentences into a single
// context scene keyed by the card's scene, rendered as FULL prose (word visible)
// on the feedback screen. It draws from BOTH the story-scene Contexts AND the
// card's own `examples:` sentences (fc.Examples), so a definitions/flashcard word
// — whose sentences live only in Examples — shows its example in feedback, in
// both directions. Sentences are de-duplicated by text.
func relearnScenesFromCard(card FreeformCard) []RelearnContextScene {
	var statements []string
	seen := map[string]bool{}
	add := func(text string) {
		text = strings.TrimSpace(text)
		if text == "" || seen[strings.ToLower(text)] {
			return
		}
		seen[strings.ToLower(text)] = true
		statements = append(statements, text)
	}
	for _, c := range card.Contexts {
		add(c.Context)
	}
	for _, ex := range card.Examples {
		add(ex.Text)
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

// primaryOriginPart returns the etymology origin Relearn folds a missed
// origin-bearing word's family card under, and whether it has one.
//
// The origin family card exists to surface the shared ROOT a set of words derive
// from — the whole point is to group e.g. recipient, intercept, and capture under
// "capere", not to scatter them under whichever generic prefix (re, inter, …)
// each happens to list first. It resolves the root two ways, covering both
// notebook shapes:
//
//   - When a part is EXPLICITLY typed "root" (the well-typed shape: prefixes
//     typed prefix/suffix, root typed root), that part wins.
//   - When NO part is explicitly typed root — the common shape where origins
//     carry no `type` at all — the etymology convention is prefix(es) FIRST and
//     the ROOT LAST, so the LAST part is taken as the root. Using the last part
//     (not the first) is what keeps every prefixed sibling folding under the
//     shared root instead of scattering under its own prefix; picking the first
//     untyped part folded a prefixed word like "abduct" (ab + ducere) under "ab"
//     instead of "ducere", emptying the root's family (the reported bug).
//
// A single-part word returns that part (root-only, or affix-only when the root
// was undeclared and dropped by resolveOriginParts, so it still groups rather
// than dropping out of Relearn). The SAME result keys both the drilled-word
// folding and the related-words family, so a card always matches its family
// (learning-history-invariants L2: one rule, one place).
func primaryOriginPart(fc FreeformCard) (WordOriginPart, bool) {
	parts := make([]WordOriginPart, 0, len(fc.WordDetail.OriginParts))
	for _, op := range fc.WordDetail.OriginParts {
		if strings.TrimSpace(op.Origin) != "" {
			parts = append(parts, op)
		}
	}
	if len(parts) == 0 {
		return WordOriginPart{}, false
	}
	// Prefer a part EXPLICITLY typed "root" (the well-typed notebook shape:
	// prefixes are type "prefix"/"suffix", the root type "root").
	for _, op := range parts {
		if isExplicitRootType(op.Type) {
			return op, true
		}
	}
	// No part is explicitly typed root — the common shape where origins carry no
	// `type` at all. The etymology convention is prefix(es) FIRST and the ROOT
	// LAST, so the last part is the root. Using the last (not the first) part is
	// what keeps every prefixed sibling folding under the shared root instead of
	// scattering under its own prefix. A single-part word returns that part.
	return parts[len(parts)-1], true
}

// isExplicitRootType reports whether an origin's type explicitly denotes a root.
// Only the literal "root" counts — an EMPTY type is NOT treated as root here, so
// an untyped prefix (empty type) never wins over the root-last rule in
// primaryOriginPart. (Prefix/suffix are the other explicit types.)
func isExplicitRootType(originType string) bool {
	return strings.EqualFold(strings.TrimSpace(originType), "root")
}

// originEnglishForms looks up an origin's English combining-form spellings from
// the shared origin index, matching the origin the same way resolveOriginParts
// does: by (origin|language) first, then by origin alone (case-insensitive).
// Returns nil when the origin is unknown or has no english_forms.
func originEnglishForms(originMap map[string]notebook.EtymologyOrigin, origin, language string) []string {
	if len(originMap) == 0 || strings.TrimSpace(origin) == "" {
		return nil
	}
	key := strings.ToLower(origin + "|" + language)
	if o, ok := originMap[key]; ok {
		return o.EnglishForms
	}
	prefix := strings.ToLower(strings.TrimSpace(origin)) + "|"
	for k, o := range originMap {
		if strings.HasPrefix(k, prefix) {
			return o.EnglishForms
		}
	}
	return nil
}

// relearnExampleHints exposes the card's usage sentences as examples so the
// recognition answering screen can show a hint, like the standard quiz. It draws
// from BOTH the story-scene Contexts AND the card's own `examples:` sentences
// (fc.Examples) — so a definitions/flashcard word shows its example hint —
// carrying each example's Highlight through and de-duplicating by text.
func relearnExampleHints(fc FreeformCard) []Example {
	var out []Example
	seen := map[string]bool{}
	add := func(text, speaker, highlight string) {
		text = strings.TrimSpace(text)
		if text == "" || seen[strings.ToLower(text)] {
			return
		}
		seen[strings.ToLower(text)] = true
		out = append(out, Example{Text: text, Speaker: speaker, Highlight: highlight})
	}
	for _, c := range fc.Contexts {
		add(c.Context, "", "")
	}
	for _, ex := range fc.Examples {
		add(ex.Text, ex.Speaker, ex.Highlight)
	}
	return out
}
