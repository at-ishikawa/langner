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
// SessionTitle disambiguates cross-session multi-sense origins (e.g.
// "ana" = "up" in Session 13 vs "ana" = "negative" in Session 16). Sense
// disambiguates same-session multi-sense origins (e.g. "pathos" = "feeling"
// AND "pathos" = "disease, suffering" both in Session 9). The full card
// key is (NotebookName, SessionTitle, Sense, Origin).
type EtymologyOriginCard struct {
	NotebookName  string
	NotebookTitle string
	SessionTitle  string
	Sense         string
	// SceneTitle is the canonical scene the origin belongs to, mirroring
	// the matching definitions file's scene of the same name. Populated
	// at read time by the reader's origin → scene projection (legacy
	// flat-shape source files) or directly from the source's
	// `event/scenes/origins` structure (new shape).
	SceneTitle string
	Origin     string
	Type       string
	Language   string
	Meaning    string
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
//
// sessionTitlesByID narrows the result to specific etymology sessions per
// notebook. A nil/empty list for a notebook means "all sessions".
func (s *Service) LoadEtymologyOriginCards(
	etymologyNotebookIDs []string,
	includeUnstudied bool,
	skipEligibility bool,
	quizType notebook.QuizType,
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
		sessionFilter := sessionTitlesByID[etymID]

		for _, o := range origins {
			if !inSectionFilter(sessionFilter, o.SessionTitle) {
				continue
			}
			// Per-session, per-sense dedup: an origin appearing twice
			// within the same session with inconsistent language metadata
			// collapses to one card, but distinct senses (e.g. pathos =
			// feeling vs pathos = disease, both in Session 9) stay as
			// separate drills. Cross-session multi-sense origins (ana =
			// "up" in Session 13 vs ana = "negative" in Session 16) are
			// handled by the SessionTitle component of the key.
			key := etymID + "\x00" + o.SessionTitle + "\x00" + o.Sense + "\x00" + originDedupKey(o.Origin)
			if seen[key] {
				continue
			}
			seen[key] = true

			// Freeform mode (skipEligibility=true) always returns every
			// origin so the typed-input lookup can find them; the
			// frontend gates re-drilling via the "Not until $date" banner.
			// Standard / reverse share their filter with the start-page
			// summary via shouldIncludeOrigin so the two counts stay
			// aligned.
			if !skipEligibility && !shouldIncludeOrigin(
				learningHistories[etymID], nbTitle, o.SessionTitle, o.Origin,
				includeUnstudied, skipQuizType,
			) {
				continue
			}

			cards = append(cards, EtymologyOriginCard{
				NotebookName:  etymID,
				NotebookTitle: nbTitle,
				SessionTitle:  o.SessionTitle,
				Sense:         o.Sense,
				SceneTitle:    o.SceneTitle,
				Origin:        o.Origin,
				Type:          o.Type,
				Language:      o.Language,
				Meaning:       o.Meaning,
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

// GradeEtymologyStandardAnswer grades a standard answer (origin -> meaning) using ValidateWordForm.
// Exact case-insensitive matches short-circuit OpenAI: the validator is
// occasionally flaky on trivial cases (e.g. answering "tome" when the
// expected meaning literally contains "tome"), and there's no honest way
// for the same string to be wrong.
func (s *Service) GradeEtymologyStandardAnswer(
	ctx context.Context,
	card EtymologyOriginCard,
	answer string,
	responseTimeMs int64,
) (GradeResult, error) {
	if isExactMatch(answer, card.Meaning) {
		return exactMatchResult(responseTimeMs), nil
	}
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
	return GradeResult{
		Correct:        isCorrect,
		Reason:         validation.Reason,
		Quality:        qualityFromResponseTime(isCorrect, responseTimeMs),
		Classification: string(validation.Classification),
	}, nil
}

// GradeEtymologyReverseAnswer grades a reverse answer (meaning -> origin)
// using ValidateWordForm. Two fast-paths bypass OpenAI:
//  1. Exact case-insensitive match against card.Origin.
//  2. The answer matches another origin in the same notebook + language
//     with the same meaning. The prompt is "give the origin for THIS
//     meaning (in THIS language)"; if two origins share that key (e.g.
//     Latin `par` and `aequus` both mean "equal"), either is a correct
//     answer and we shouldn't penalise the learner for picking the one
//     the card didn't happen to be drawn from.
func (s *Service) GradeEtymologyReverseAnswer(
	ctx context.Context,
	card EtymologyOriginCard,
	answer string,
	responseTimeMs int64,
) (GradeResult, error) {
	if isExactMatch(answer, card.Origin) {
		return exactMatchResult(responseTimeMs), nil
	}
	if s.isSameMeaningOrigin(card, answer) {
		return GradeResult{
			Correct:        true,
			Reason:         fmt.Sprintf("Accepted: %q is a synonymous origin in %s with the same meaning.", strings.TrimSpace(answer), card.Language),
			Quality:        qualityFromResponseTime(true, responseTimeMs),
			Classification: string(inference.ClassificationSynonym),
		}, nil
	}
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
	return GradeResult{
		Correct:        isCorrect,
		Reason:         validation.Reason,
		Quality:        qualityFromResponseTime(isCorrect, responseTimeMs),
		Classification: string(validation.Classification),
	}, nil
}

// isExactMatch reports whether two strings are equal after trimming
// surrounding whitespace and case-folding.
func isExactMatch(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// exactMatchResult builds a GradeResult for the trivial "answer equals
// expected" case: correct, with quality scaled by response time.
func exactMatchResult(responseTimeMs int64) GradeResult {
	return GradeResult{
		Correct:        true,
		Reason:         "Exact match.",
		Quality:        qualityFromResponseTime(true, responseTimeMs),
		Classification: string(inference.ClassificationSameWord),
	}
}

// qualityFromResponseTime maps a wall-clock response time into the SM-2
// quality scale. Correct answers earn 3–5 depending on speed; wrong
// answers always earn 1.
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

// isSameMeaningOrigin reports whether `answer` matches a different origin
// in the same notebook + language + meaning as the card. Used in reverse
// mode to accept synonymous-origin answers (e.g. "aequus" when the card
// was drawn for "par"). Failures here (reader, YAML) silently return
// false; the caller falls back to OpenAI.
func (s *Service) isSameMeaningOrigin(card EtymologyOriginCard, answer string) bool {
	want := strings.ToLower(strings.TrimSpace(answer))
	if want == "" {
		return false
	}
	reader, err := s.newReader()
	if err != nil {
		return false
	}
	origins, err := reader.ReadEtymologyNotebook(card.NotebookName)
	if err != nil {
		return false
	}
	cardMeaning := strings.ToLower(strings.TrimSpace(card.Meaning))
	for _, o := range origins {
		if o.Language != card.Language {
			continue
		}
		if strings.ToLower(strings.TrimSpace(o.Meaning)) != cardMeaning {
			continue
		}
		if strings.ToLower(strings.TrimSpace(o.Origin)) == want {
			return true
		}
	}
	return false
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
	// Etymology learning history shares one top-level block per session
	// with standard/reverse/freeform writes. Origins land in the same
	// scene the matching vocab definition references (or a synthetic
	// scene = session title when no match exists). Multi-sense origins
	// remain disambiguated because each session is its own top-level
	// block.
	sceneTitle := card.SceneTitle
	if sceneTitle == "" {
		sceneTitle = card.SessionTitle
	}
	updater.UpdateOrCreateExpressionWithQualityForEtymology(
		card.NotebookName,
		card.SessionTitle,
		sceneTitle,
		card.Origin,
		card.Origin,
		correct,
		isKnownWord,
		quality,
		responseTimeMs,
		quizType,
	)

	// Structural guard: after the in-memory update, refuse to persist
	// any state where an origin lives in two scenes of the same session.
	// Catches the "two logos sessions" class of bug at write time
	// instead of letting it accumulate silently in the YAML.
	if err := notebook.AssertNoDuplicateOriginsInSession(updater.GetHistory(), card.NotebookName, card.SessionTitle); err != nil {
		return fmt.Errorf("save etymology origin %q: %w", card.Origin, err)
	}

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, card.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return fmt.Errorf("failed to save learning history for %q: %w", card.NotebookName, err)
	}

	return nil
}

// LoadEtymologyExampleWords returns example expressions for each card,
// keyed by lower(origin)\x00sessionTitle\x00sense. Examples come from the
// consolidated definitions notebook (same notebook ID as the etymology
// notebook), narrowed by both the parent session's metadata.title and the
// card's Sense. So the pathos=feeling card in Session 9 gets sympathy and
// empathy; the pathos=disease card in the same session gets osteopath and
// psychopath. A definition's origin_parts ref pins a sense via Sense; refs
// that omit Sense match every sense of that origin (back-compat for
// definitions written before the sense field existed).
func (s *Service) LoadEtymologyExampleWords(cards []EtymologyOriginCard) (map[string][]string, error) {
	if len(cards) == 0 {
		return nil, nil
	}
	reader, err := s.newReader()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notebook reader: %w", err)
	}

	// Build a lookup of which senses each (notebook, session, origin) has
	// so an un-pinned ref can fan out to all of them, while a pinned ref
	// resolves to exactly the matching sense.
	type baseKey struct {
		notebookID, sessionTitle, origin string
	}
	sensesByBase := make(map[baseKey][]string)
	for _, c := range cards {
		bk := baseKey{c.NotebookName, c.SessionTitle, strings.ToLower(strings.TrimSpace(c.Origin))}
		sensesByBase[bk] = append(sensesByBase[bk], c.Sense)
	}

	result := make(map[string][]string)
	bookIDs := reader.GetDefinitionsBookIDs()
	for _, bookID := range bookIDs {
		hasMatch := false
		for bk := range sensesByBase {
			if bk.notebookID == bookID {
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
						bk := baseKey{bookID, sessionTitle, strings.ToLower(strings.TrimSpace(ref.Origin))}
						senses, ok := sensesByBase[bk]
						if !ok {
							continue
						}
						expr := note.Expression
						if expr == "" {
							expr = note.Definition
						}
						if expr == "" {
							continue
						}
						// Sense-aware fan-out:
						//   - ref.Sense == "" with a matching sense="" card  → attach to that card.
						//   - ref.Sense == "" with NO sense="" card but multiple sensed cards → attach to all (legacy/un-backfilled behavior).
						//   - ref.Sense != "" → attach only to the matching sense.
						hasEmptySense := false
						for _, s := range senses {
							if s == "" {
								hasEmptySense = true
								break
							}
						}
						for _, s := range senses {
							if ref.Sense != "" && s != ref.Sense {
								continue
							}
							if ref.Sense == "" && hasEmptySense && s != "" {
								continue
							}
							resultKey := strings.ToLower(strings.TrimSpace(ref.Origin)) + "\x00" + sessionTitle + "\x00" + s
							result[resultKey] = append(result[resultKey], expr)
						}
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
	// In test mode, never gate the freeform Submit button on a future review
	// date — the test suite submits the same origin repeatedly across scenarios.
	if s.disableShuffle {
		return map[string]string{}, nil
	}
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

// originNextReviewDate returns the soonest future review date this origin
// is scheduled for across every etymology log set (breakdown, assembly,
// freeform). Returns empty when no logs are scheduled for the future —
// either because the origin has never been answered, or because every
// logged mode is currently past its interval. Reading only breakdown was
// a bug: a word answered in reverse (assembly logs) or freeform with
// interval_days=30 would still read as "due now" in freeform and
// reappear the same day.
//
// Picks the soonest future date because the user wanting to drill the
// word again should wait at least until SOME mode says it's due again.
// If they recently answered in standard with a 30-day interval, freeform
// shouldn't unlock just because the freeform log set happens to be
// empty.
func originNextReviewDate(histories []notebook.LearningHistory, card EtymologyOriginCard) string {
	// Post-migration etymology learning history is keyed:
	//   history.metadata.title = SessionTitle   (e.g. "Session 2")
	//   scene.metadata.title   = SceneTitle     (e.g. "alter (other)")
	// Earlier callers compared against NotebookTitle / SessionTitle one
	// level off, so this loop never matched real data; see the migration
	// note in Validator.migrateEtymologyShape for the schema move.
	for _, hist := range histories {
		if hist.Metadata.Title != card.SessionTitle {
			continue
		}
		for _, scene := range hist.Scenes {
			if scene.Metadata.Title != card.SceneTitle {
				continue
			}
			for _, expr := range scene.Expressions {
				if !strings.EqualFold(expr.Expression, card.Origin) {
					continue
				}
				// Freeform writes into both BreakdownLogs and
				// AssemblyLogs (see SetLogsForQuizType), so the two
				// fields cover all three etymology modes.
				var soonest string
				for _, logs := range [][]notebook.LearningRecord{
					expr.EtymologyBreakdownLogs,
					expr.EtymologyAssemblyLogs,
				} {
					next := computeNextReviewDate(logs)
					if next == "" {
						continue
					}
					if soonest == "" || next < soonest {
						soonest = next
					}
				}
				return soonest
			}
		}
	}
	return ""
}

// LoadEtymologyNotebookSummaries returns etymology notebook summaries with
// per-mode due origin counts.
//
// includeUnstudied passes through to shouldIncludeOrigin so the counts match
// exactly what LoadEtymologyOriginCards would return for the same mode +
// toggle combination. Both standard and reverse counts are computed because
// the same origin can be due in one mode and not in the other.
func (s *Service) LoadEtymologyNotebookSummaries(includeUnstudied bool) ([]NotebookSummary, error) {
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

		var standardTotal, reverseTotal int
		seen := make(map[string]bool)
		seenSession := make(map[string]struct{})
		// sectionStandard / sectionReverse tally due origins per session in
		// the order sessions first appear in the file so the start page
		// lists them in document order rather than map-iteration order.
		var sessionOrder []string
		sectionStandard := make(map[string]int)
		sectionReverse := make(map[string]int)
		for _, o := range origins {
			if o.SessionTitle != "" {
				if _, ok := seenSession[o.SessionTitle]; !ok {
					seenSession[o.SessionTitle] = struct{}{}
					sessionOrder = append(sessionOrder, o.SessionTitle)
				}
			}
			// Per-(session, origin) dedup so multi-sense origins each
			// contribute their own due-count slot. See
			// LoadEtymologyOriginCards for the keying rationale.
			key := o.SessionTitle + "\x00" + originDedupKey(o.Origin)
			if seen[key] {
				continue
			}
			seen[key] = true
			if shouldIncludeOrigin(learningHistories[id], index.Name, o.SessionTitle, o.Origin, includeUnstudied, notebook.QuizTypeEtymologyStandard) {
				standardTotal++
				sectionStandard[o.SessionTitle]++
			}
			if shouldIncludeOrigin(learningHistories[id], index.Name, o.SessionTitle, o.Origin, includeUnstudied, notebook.QuizTypeEtymologyReverse) {
				reverseTotal++
				sectionReverse[o.SessionTitle]++
			}
		}

		var sections []NotebookSectionSummary
		for _, title := range sessionOrder {
			sections = append(sections, NotebookSectionSummary{
				Title:                       title,
				EtymologyReviewCount:        sectionStandard[title],
				EtymologyReverseReviewCount: sectionReverse[title],
			})
		}

		summaries = append(summaries, NotebookSummary{
			NotebookID:                  id,
			Name:                        index.Name,
			EtymologyReviewCount:        standardTotal,
			EtymologyReverseReviewCount: reverseTotal,
			Kind:                        "Etymology",
			LatestDate:                  index.LatestDate,
			Sections:                    sections,
		})
	}

	return summaries, nil
}

// shouldIncludeOrigin returns true when an origin must appear in the
// standard or reverse etymology quiz for the given quiz type and toggle.
// It is the single source of truth used by both LoadEtymologyOriginCards
// (the actual quiz load) and LoadEtymologyNotebookSummaries (the start-page
// count) so the count badge and the quiz can never disagree.
//
// Rules (mirroring the user-visible behaviour):
//
//   - per-type skip → exclude (a skip in one mode does not affect the other).
//   - never-seen (no logs in any track) → include iff includeUnstudied.
//   - has any logs → defer to the SR interval for the requested mode.
//     A `misunderstood` log triggers retry (interval 1d, status check in
//     NeedsEtymologyReview); standard/reverse show the correct answer on
//     the feedback screen, so a prior correct answer is not required.
func shouldIncludeOrigin(
	histories []notebook.LearningHistory,
	notebookTitle, sessionTitle, origin string,
	includeUnstudied bool,
	quizType notebook.QuizType,
) bool {
	if isOriginSkipped(histories, notebookTitle, sessionTitle, origin, quizType) {
		return false
	}
	expr := findOriginExpression(histories, notebookTitle, sessionTitle, origin)
	if expr == nil {
		return includeUnstudied
	}
	// "Never seen in THIS mode" — the origin has history but no logs in
	// the slot the requested mode reads from (etymology_breakdown for
	// standard, etymology_assembly for reverse) — goes through the
	// includeUnstudied gate, same as an origin with no history at all.
	// Without this, an origin studied only in standard/freeform floods
	// the reverse queue: needsOriginReview treats an empty same-mode
	// slot as "due now", so every cross-mode-only origin appears in the
	// reverse start-page count.
	if len(expr.GetLogsForQuizType(quizType)) == 0 {
		return includeUnstudied
	}
	return needsOriginReview(histories, notebookTitle, sessionTitle, origin, quizType)
}

// findOriginExpression returns the LearningHistoryExpression for an origin.
// Prefers entries explicitly typed as origin so a vocab entry sharing the
// name (e.g. "ego" the word vs the Latin root) isn't returned by mistake.
// Falls back to legacy type-empty entries that carry etymology logs (the
// pre-Type representation). Looks at canonical Shape B first, then legacy
// Shape A for unmigrated learning-history files.
//
// Eligibility is per-(session_title, origin) so multi-sense origins
// (ana = "up" in Session 13, "negative" in Session 16) keep independent
// learning curves — different sessions live in different top-level blocks.
func findOriginExpression(
	histories []notebook.LearningHistory,
	notebookTitle, sessionTitle, origin string,
) *notebook.LearningHistoryExpression {
	isOriginCandidate := func(expr *notebook.LearningHistoryExpression) bool {
		if expr.Type == notebook.LearningExpressionTypeOrigin {
			return true
		}
		if expr.Type == "" && (len(expr.EtymologyBreakdownLogs) > 0 || len(expr.EtymologyAssemblyLogs) > 0) {
			return true
		}
		return false
	}
	scan := func(h *notebook.LearningHistory) *notebook.LearningHistoryExpression {
		var typedHit, legacyHit *notebook.LearningHistoryExpression
		check := func(expr *notebook.LearningHistoryExpression) {
			if !strings.EqualFold(expr.Expression, origin) {
				return
			}
			if expr.Type == notebook.LearningExpressionTypeOrigin && typedHit == nil {
				typedHit = expr
				return
			}
			if isOriginCandidate(expr) && legacyHit == nil {
				legacyHit = expr
			}
		}
		for ei := range h.Expressions {
			check(&h.Expressions[ei])
		}
		for si := range h.Scenes {
			for ei := range h.Scenes[si].Expressions {
				check(&h.Scenes[si].Expressions[ei])
			}
		}
		if typedHit != nil {
			return typedHit
		}
		return legacyHit
	}

	// Canonical Shape B: top-level title is the session.
	for hi := range histories {
		if histories[hi].Metadata.Title != sessionTitle {
			continue
		}
		if hit := scan(&histories[hi]); hit != nil {
			return hit
		}
	}
	// Legacy Shape A fallback: notebook-named top-level block, sessions
	// as scenes.
	for hi := range histories {
		if histories[hi].Metadata.Title != notebookTitle {
			continue
		}
		for si := range histories[hi].Scenes {
			if histories[hi].Scenes[si].Metadata.Title != sessionTitle {
				continue
			}
			for ei := range histories[hi].Scenes[si].Expressions {
				expr := &histories[hi].Scenes[si].Expressions[ei]
				if strings.EqualFold(expr.Expression, origin) && isOriginCandidate(expr) {
					return expr
				}
			}
		}
	}
	return nil
}

// isOriginSkipped returns true when the origin's per-(notebook, session)
// learning history records a skip for the given quiz type.
func isOriginSkipped(
	histories []notebook.LearningHistory,
	notebookTitle, sessionTitle, origin string,
	quizType notebook.QuizType,
) bool {
	expr := findOriginExpression(histories, notebookTitle, sessionTitle, origin)
	if expr == nil {
		return false
	}
	return expr.SkippedAt.IsSkipped(quizType)
}

// needsOriginReview checks whether an origin is DUE for review under the
// spaced-repetition schedule. Callers must first verify eligibility.
func needsOriginReview(
	histories []notebook.LearningHistory,
	notebookTitle, sessionTitle, origin string,
	quizType notebook.QuizType,
) bool {
	expr := findOriginExpression(histories, notebookTitle, sessionTitle, origin)
	if expr == nil {
		return false
	}
	return expr.NeedsEtymologyReview(quizType)
}

