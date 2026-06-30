package notebook

import (
	"fmt"
	"strings"
	"time"
)

// normalizeQuotes replaces smart quotes with ASCII equivalents for comparison.
func normalizeQuotes(s string) string {
	r := strings.NewReplacer(
		"\u2018", "'", "\u2019", "'", // smart single quotes
		"\u201C", "\"", "\u201D", "\"", // smart double quotes
	)
	return r.Replace(s)
}

// LearningHistoryUpdater provides methods to update learning history
type LearningHistoryUpdater struct {
	history    []LearningHistory
	calculator IntervalCalculator
}

// NewLearningHistoryUpdater creates a new updater with the given history and calculator.
func NewLearningHistoryUpdater(history []LearningHistory, calculator IntervalCalculator) *LearningHistoryUpdater {
	if calculator == nil {
		calculator = &SM2Calculator{}
	}
	return &LearningHistoryUpdater{
		history:    history,
		calculator: calculator,
	}
}

// findOrCreateStory finds an existing story or creates a new one.
// flatType is a non-empty string (e.g. "flashcard", "etymology") when the
// history should use top-level Expressions instead of nested Scenes.
func (u *LearningHistoryUpdater) findOrCreateStory(notebookID, storyTitle, flatType string) int {
	normalizedTitle := normalizeQuotes(storyTitle)
	for i, h := range u.history {
		if normalizeQuotes(h.Metadata.Title) == normalizedTitle {
			return i
		}
	}

	newStory := LearningHistory{
		Metadata: LearningHistoryMetadata{
			NotebookID: notebookID,
			Title:      storyTitle,
		},
	}

	if flatType != "" {
		newStory.Metadata.Type = flatType
		newStory.Expressions = []LearningHistoryExpression{}
	} else {
		newStory.Scenes = []LearningScene{}
	}

	u.history = append(u.history, newStory)
	return len(u.history) - 1
}

// findOrCreateScene finds an existing scene or creates a new one.
// Uses normalizeQuotes so that titles with smart quotes (e.g. from book
// imports) match titles with ASCII apostrophes (from user input).
func (u *LearningHistoryUpdater) findOrCreateScene(storyIndex int, sceneTitle string) int {
	normalizedTitle := normalizeQuotes(sceneTitle)
	for i, s := range u.history[storyIndex].Scenes {
		if normalizeQuotes(s.Metadata.Title) == normalizedTitle {
			return i
		}
	}

	newScene := LearningScene{
		Metadata: LearningSceneMetadata{
			Title: sceneTitle,
		},
		Expressions: []LearningHistoryExpression{},
	}
	u.history[storyIndex].Scenes = append(u.history[storyIndex].Scenes, newScene)
	return len(u.history[storyIndex].Scenes) - 1
}

// GetHistory returns the updated history
func (u *LearningHistoryUpdater) GetHistory() []LearningHistory {
	return u.history
}

// UpdateOrCreateExpressionWithQuality updates or creates an expression with SM-2 quality assessment.
// originalExpression is the original expression form (e.g., Note.Expression) which may differ from
// expression (e.g., Note.Definition) when a definition is used as the lookup key. If originalExpression
// is non-empty, both forms are checked when matching existing entries to avoid duplicates.
func (u *LearningHistoryUpdater) UpdateOrCreateExpressionWithQuality(
	notebookID, storyTitle, sceneTitle, expression, originalExpression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) bool {
	isFlashcard := storyTitle == "flashcards" && sceneTitle == ""
	normalizedSceneTitle := normalizeQuotes(sceneTitle)

	for hi, h := range u.history {
		if normalizeQuotes(h.Metadata.Title) != normalizeQuotes(storyTitle) {
			continue
		}

		if isFlashcard || h.Metadata.Type == "flashcard" {
			for ei, exp := range h.Expressions {
				if exp.Expression != expression && (originalExpression == "" || exp.Expression != originalExpression) {
					continue
				}
				exp.AddRecordWithQuality(u.calculator, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
				u.history[hi].Expressions[ei] = exp
				return true
			}
			continue
		}

		for si, s := range h.Scenes {
			if normalizeQuotes(s.Metadata.Title) != normalizedSceneTitle {
				continue
			}

			for ei, exp := range s.Expressions {
				if exp.Expression != expression && (originalExpression == "" || exp.Expression != originalExpression) {
					continue
				}
				exp.AddRecordWithQuality(u.calculator, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
				u.history[hi].Scenes[si].Expressions[ei] = exp
				return true
			}
		}
	}

	u.createNewExpressionWithQuality(notebookID, storyTitle, sceneTitle, expression, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
	return false
}

// UpdateOrCreateExpressionWithQualityForReverse updates or creates an expression with SM-2 quality assessment for reverse quiz.
// originalExpression is the original expression form (e.g., Note.Expression) which may differ from
// expression (e.g., Note.Definition) when a definition is used as the lookup key. If originalExpression
// is non-empty, both forms are checked when matching existing entries to avoid duplicates.
func (u *LearningHistoryUpdater) UpdateOrCreateExpressionWithQualityForReverse(
	notebookID, storyTitle, sceneTitle, expression, originalExpression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) bool {
	isFlashcard := storyTitle == "flashcards" && sceneTitle == ""
	normalizedSceneTitle := normalizeQuotes(sceneTitle)

	for hi, h := range u.history {
		if normalizeQuotes(h.Metadata.Title) != normalizeQuotes(storyTitle) {
			continue
		}

		if isFlashcard || h.Metadata.Type == "flashcard" {
			for ei, exp := range h.Expressions {
				if exp.Expression != expression && (originalExpression == "" || exp.Expression != originalExpression) {
					continue
				}
				exp.AddRecordWithQualityForReverse(u.calculator, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
				u.history[hi].Expressions[ei] = exp
				return true
			}
			continue
		}

		for si, s := range h.Scenes {
			if normalizeQuotes(s.Metadata.Title) != normalizedSceneTitle {
				continue
			}

			for ei, exp := range s.Expressions {
				if exp.Expression != expression && (originalExpression == "" || exp.Expression != originalExpression) {
					continue
				}
				exp.AddRecordWithQualityForReverse(u.calculator, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
				u.history[hi].Scenes[si].Expressions[ei] = exp
				return true
			}
		}
	}

	u.createNewExpressionWithQualityForReverse(notebookID, storyTitle, sceneTitle, expression, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
	return false
}

// createNewExpressionWithQualityForReverse creates a new expression entry with quality data for reverse quiz
func (u *LearningHistoryUpdater) createNewExpressionWithQualityForReverse(
	notebookID, storyTitle, sceneTitle, expression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) {
	flatType := ""
	if storyTitle == "flashcards" && sceneTitle == "" {
		flatType = "flashcard"
	}
	storyIndex := u.findOrCreateStory(notebookID, storyTitle, flatType)

	newExpression := LearningHistoryExpression{
		Expression:  expression,
		LearnedLogs: []LearningRecord{},
		ReverseLogs: []LearningRecord{},
	}
	newExpression.AddRecordWithQualityForReverse(u.calculator, isCorrect, isKnownWord, quality, responseTimeMs, quizType)

	if len(newExpression.ReverseLogs) == 0 {
		return
	}

	if flatType != "" || u.history[storyIndex].Metadata.Type == "flashcard" {
		u.history[storyIndex].Expressions = append(
			u.history[storyIndex].Expressions,
			newExpression,
		)
		return
	}

	sceneIndex := u.findOrCreateScene(storyIndex, sceneTitle)
	u.history[storyIndex].Scenes[sceneIndex].Expressions = append(
		u.history[storyIndex].Scenes[sceneIndex].Expressions,
		newExpression,
	)
}

// createNewExpressionWithQuality creates a new expression entry with quality data
func (u *LearningHistoryUpdater) createNewExpressionWithQuality(
	notebookID, storyTitle, sceneTitle, expression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) {
	flatType := ""
	if storyTitle == "flashcards" && sceneTitle == "" {
		flatType = "flashcard"
	}
	storyIndex := u.findOrCreateStory(notebookID, storyTitle, flatType)

	newExpression := LearningHistoryExpression{
		Expression:  expression,
		LearnedLogs: []LearningRecord{},
	}
	newExpression.AddRecordWithQuality(u.calculator, isCorrect, isKnownWord, quality, responseTimeMs, quizType)

	if len(newExpression.LearnedLogs) == 0 {
		return
	}

	if flatType != "" || u.history[storyIndex].Metadata.Type == "flashcard" {
		u.history[storyIndex].Expressions = append(
			u.history[storyIndex].Expressions,
			newExpression,
		)
		return
	}

	sceneIndex := u.findOrCreateScene(storyIndex, sceneTitle)
	u.history[storyIndex].Scenes[sceneIndex].Expressions = append(
		u.history[storyIndex].Scenes[sceneIndex].Expressions,
		newExpression,
	)
}

// FindExpressionByName searches for an expression across all histories, returning
// a pointer to the expression. Returns nil if not found.
func (u *LearningHistoryUpdater) FindExpressionByName(expression string) *LearningHistoryExpression {
	return u.FindExpressionByAnyName(expression)
}

// FindExpressionByAnyName tries every candidate name in order and returns
// the first matching expression across all histories. Used for the
// definitions-style case where a card's Expression is the Definition
// form but the YAML stores the entry under the original Note.Expression
// — callers pass both forms so the fallback succeeds.
func (u *LearningHistoryUpdater) FindExpressionByAnyName(names ...string) *LearningHistoryExpression {
	for _, name := range names {
		if name == "" {
			continue
		}
		for hi := range u.history {
			h := &u.history[hi]
			// Always search top-level expressions first (flashcard, etymology, etc.)
			for ei := range h.Expressions {
				if strings.EqualFold(h.Expressions[ei].Expression, name) {
					return &h.Expressions[ei]
				}
			}
			// Then search scenes
			for si := range h.Scenes {
				for ei := range h.Scenes[si].Expressions {
					if strings.EqualFold(h.Scenes[si].Expressions[ei].Expression, name) {
						return &h.Scenes[si].Expressions[ei]
					}
				}
			}
		}
	}
	return nil
}

// OverrideLogInput carries everything OverrideLog needs to locate and
// rewrite a single learning log entry.
//
//   - Expression / OriginalExpression: the YAML lookup key. Definitions-
//     style cards expose the long Definition form as the card's Entry
//     while the YAML keeps the original word; passing both lets the
//     updater fall back. Either field may be empty.
//   - QuizType: which per-quiz-type log list to scan. For Freeform
//     variants, OverrideLog mirrors the write side (Create) and flips
//     the matching entry in BOTH paired lists.
//   - LearnedAt: YYYY-MM-DD or RFC3339; identifies the specific log
//     within the list (never blindly logs[0]).
//   - MarkCorrect: nil = no status change; non-nil = set the entry to
//     the explicit correct/incorrect state. Toggling is not a primitive
//     here — callers that want toggle should compute it themselves and
//     pass the desired state, which is what the frontend already does.
type OverrideLogInput struct {
	Expression         string
	OriginalExpression string
	QuizType           QuizType
	LearnedAt          string
	MarkCorrect        *bool
}

// OverrideLogResult reports the pre-change values of the affected log
// plus the recomputed next-review date. Found is false when no log
// matched the (expression, quiz_type, learned_at) tuple — callers must
// treat that as a soft no-op (the same way Update returns rows-affected
// 0 in SQL).
type OverrideLogResult struct {
	OriginalQuality      int
	OriginalStatus       string
	OriginalIntervalDays int
	NewNextReviewDate    string
	Found                bool
}

// OverrideLog rewrites the log identified by (expression/originalExpression,
// quizType, learnedAt) according to MarkCorrect. When the quiz type is one
// of the freeform variants — which write the same answer into two paired
// log lists at submit time — the override is applied to both lists in the
// same call so the two halves of one logical answer stay in sync.
func (u *LearningHistoryUpdater) OverrideLog(in OverrideLogInput) OverrideLogResult {
	expr := u.FindExpressionByAnyName(in.Expression, in.OriginalExpression)
	if expr == nil {
		return OverrideLogResult{}
	}

	primary := in.QuizType.PrimaryLogList()
	logs := getLogsByList(expr, primary)
	idx := indexLogByLearnedAt(logs, in.LearnedAt)
	if idx < 0 {
		return OverrideLogResult{}
	}

	result := OverrideLogResult{
		OriginalQuality:      logs[idx].Quality,
		OriginalStatus:       string(logs[idx].Status),
		OriginalIntervalDays: logs[idx].IntervalDays,
	}

	if in.MarkCorrect != nil {
		applyMark(&logs[idx], *in.MarkCorrect, in.QuizType)
		var previous []LearningRecord
		if idx+1 < len(logs) {
			previous = logs[idx+1:]
		}
		newInterval, _ := u.calculator.NextIntervalForWrite(previous, logs[idx])
		logs[idx].IntervalDays = newInterval
	}
	setLogsByList(expr, primary, logs)
	mirrorOverrideToPair(expr, in.QuizType, logs[idx])

	result.NewNextReviewDate = logs[idx].LearnedAt.AddDate(0, 0, logs[idx].IntervalDays).Format("2006-01-02")
	result.Found = true
	return result
}

// UndoOverrideLogInput is the symmetric input for UndoOverrideLog.
type UndoOverrideLogInput struct {
	Expression           string
	OriginalExpression   string
	QuizType             QuizType
	LearnedAt            string
	OriginalQuality      int
	OriginalStatus       string
	OriginalIntervalDays int
}

// UndoOverrideLogResult mirrors OverrideLogResult for the undo path.
type UndoOverrideLogResult struct {
	Correct           bool
	NewNextReviewDate string
	Found             bool
}

// UndoOverrideLog restores a log entry to a previously captured state.
// Used by the Analytics UI's "Undo" button after a manual override.
// Like OverrideLog, the freeform variants mirror the restore across
// both paired log lists.
func (u *LearningHistoryUpdater) UndoOverrideLog(in UndoOverrideLogInput) UndoOverrideLogResult {
	expr := u.FindExpressionByAnyName(in.Expression, in.OriginalExpression)
	if expr == nil {
		return UndoOverrideLogResult{}
	}

	primary := in.QuizType.PrimaryLogList()
	logs := getLogsByList(expr, primary)
	idx := indexLogByLearnedAt(logs, in.LearnedAt)
	if idx < 0 {
		return UndoOverrideLogResult{}
	}

	logs[idx].Quality = in.OriginalQuality
	logs[idx].Status = LearnedStatus(in.OriginalStatus)
	logs[idx].IntervalDays = in.OriginalIntervalDays
	logs[idx].OverrideInterval = 0
	setLogsByList(expr, primary, logs)
	mirrorOverrideToPair(expr, in.QuizType, logs[idx])

	return UndoOverrideLogResult{
		Correct:           logs[idx].Quality >= 3,
		NewNextReviewDate: logs[idx].LearnedAt.AddDate(0, 0, logs[idx].IntervalDays).Format("2006-01-02"),
		Found:             true,
	}
}

// applyMark writes the desired correct/incorrect state onto a log.
// Standard / reverse / etymology-non-freeform use the binary
// understood/misunderstood pair; freeform variants use can-be-used to
// match the higher bar the freeform write path sets (active recall).
func applyMark(log *LearningRecord, markCorrect bool, quizType QuizType) {
	if !markCorrect {
		log.Quality = 1
		log.Status = LearnedStatusMisunderstood
		return
	}
	log.Quality = 4
	if quizType == QuizTypeFreeform || quizType == QuizTypeEtymologyFreeform {
		log.Status = LearnedStatusCanBeUsed
	} else {
		log.Status = LearnedStatusUnderstood
	}
}

// indexLogByLearnedAt returns the index of the log entry whose LearnedAt
// matches the given YYYY-MM-DD or RFC3339 string, or -1 if none match.
func indexLogByLearnedAt(logs []LearningRecord, learnedAt string) int {
	if learnedAt == "" {
		return -1
	}
	for i, log := range logs {
		if log.LearnedAt.Format("2006-01-02") == learnedAt ||
			log.LearnedAt.Format(time.RFC3339) == learnedAt {
			return i
		}
	}
	return -1
}

// mirrorOverrideToPair handles the freeform-pair case: when QuizType is
// Freeform / EtymologyFreeform, the write path appends the same answer
// into two log lists (LearnedLogs+ReverseLogs, or EtymologyBreakdown+
// EtymologyAssembly). After overriding one half we must apply the same
// mutation to the matching entry in the other half so the two stay
// consistent. The same-day match-by-learnedAt is how the lookup pairs
// them — the freeform writer stamps both with the same timestamp.
func mirrorOverrideToPair(expr *LearningHistoryExpression, quizType QuizType, updated LearningRecord) {
	var pair logList
	switch quizType {
	case QuizTypeFreeform:
		pair = listReverse
	case QuizTypeEtymologyFreeform:
		pair = listEtymologyAssembly
	default:
		return
	}
	pairLogs := getLogsByList(expr, pair)
	idx := indexLogByLearnedAt(pairLogs, updated.LearnedAt.Format(time.RFC3339))
	if idx < 0 {
		idx = indexLogByLearnedAt(pairLogs, updated.LearnedAt.Format("2006-01-02"))
	}
	if idx < 0 {
		return
	}
	pairLogs[idx].Quality = updated.Quality
	pairLogs[idx].Status = updated.Status
	pairLogs[idx].IntervalDays = updated.IntervalDays
	pairLogs[idx].OverrideInterval = updated.OverrideInterval
	setLogsByList(expr, pair, pairLogs)
}

// logList is an internal enum picking which log slice on a
// LearningHistoryExpression to read/write. Kept private to this file so
// OverrideLog / UndoOverrideLog / mirrorOverrideToPair speak the same
// language without exposing the discriminator to the rest of the package.
type logList int

const (
	listLearned logList = iota
	listReverse
	listEtymologyBreakdown
	listEtymologyAssembly
)

// PrimaryLogList returns the log list a single OverrideLog call mutates
// for the given quiz type. Freeform variants identify their primary
// list here; the secondary (paired) list is handled by
// mirrorOverrideToPair after the primary has been written.
func (qt QuizType) PrimaryLogList() logList {
	switch qt {
	case QuizTypeReverse:
		return listReverse
	case QuizTypeEtymologyStandard, QuizTypeEtymologyFreeform:
		return listEtymologyBreakdown
	case QuizTypeEtymologyReverse:
		return listEtymologyAssembly
	default:
		return listLearned
	}
}

func getLogsByList(expr *LearningHistoryExpression, list logList) []LearningRecord {
	switch list {
	case listReverse:
		return expr.ReverseLogs
	case listEtymologyBreakdown:
		return expr.EtymologyBreakdownLogs
	case listEtymologyAssembly:
		return expr.EtymologyAssemblyLogs
	default:
		return expr.LearnedLogs
	}
}

func setLogsByList(expr *LearningHistoryExpression, list logList, logs []LearningRecord) {
	switch list {
	case listReverse:
		expr.ReverseLogs = logs
	case listEtymologyBreakdown:
		expr.EtymologyBreakdownLogs = logs
	case listEtymologyAssembly:
		expr.EtymologyAssemblyLogs = logs
	default:
		expr.LearnedLogs = logs
	}
}

// SetSkippedAt records a skip for the given quiz type at the given timestamp.
// Returns false if the expression isn't found in any history.
func (u *LearningHistoryUpdater) SetSkippedAt(expression string, quizType QuizType, skippedAt string) bool {
	expr := u.FindExpressionByName(expression)
	if expr == nil {
		return false
	}
	expr.SkippedAt = expr.SkippedAt.Set(quizType, skippedAt)
	return true
}

// EnsureExpressionStubForSkip creates a learned-log-free stub for the
// expression at (notebookID, storyTitle, sceneTitle) when no entry exists
// yet. The stub holds only the expression name; SetSkippedAt then writes
// the skip timestamp onto it. This is the path the notebook detail page's
// per-type skip checkboxes take when the user clicks Skip on a word that
// hasn't been studied yet — without it, the only way to record a skip
// was UpdateOrCreateExpressionWithQuality, which fabricated a fake
// "quality 5" learned_log entry that pretended the user had answered
// the word correctly.
//
// If the expression already exists anywhere in the history, this is a
// no-op and SetSkippedAt updates the existing record in place.
//
// sceneTitle "" stores the stub at the top-level Expressions list
// (flashcard-style); a non-empty value nests it under that scene.
func (u *LearningHistoryUpdater) EnsureExpressionStubForSkip(
	notebookID, storyTitle, sceneTitle, expression string,
) {
	if u.FindExpressionByName(expression) != nil {
		return
	}
	stub := LearningHistoryExpression{Expression: expression}
	if sceneTitle == "" {
		idx := u.findOrCreateStory(notebookID, storyTitle, "flashcard")
		u.history[idx].Expressions = append(u.history[idx].Expressions, stub)
		return
	}
	storyIdx := u.findOrCreateStory(notebookID, storyTitle, "")
	sceneIdx := u.findOrCreateScene(storyIdx, sceneTitle)
	u.history[storyIdx].Scenes[sceneIdx].Expressions = append(
		u.history[storyIdx].Scenes[sceneIdx].Expressions, stub,
	)
}

// UpdateOrCreateExpressionWithQualityForEtymology updates or creates an
// origin entry. Lookup matches on (session, expression, Type=origin)
// across EVERY scene in the matching session — scene title is not part
// of the key. This is what stops an origin's learning history from
// splitting into two entries when the scene title pickBestSceneForOrigin
// derives drifts over time: today's writer might be told to use
// "derma (skin)" while the prior writer used "gyne (woman)", but if the
// origin already lives under "gyne (woman)" we update there. Vocab
// entries are filtered out so an etymology log can't pollute a same-
// named word; legacy type-empty entries that already carry etymology
// logs are upgraded in place to Type=origin so re-runs converge on the
// typed shape.
//
// Only when no existing origin entry is found do we create a new one,
// and only then does sceneTitle matter (it's the location for the new
// entry, derived by pickBestSceneForOrigin).
func (u *LearningHistoryUpdater) UpdateOrCreateExpressionWithQualityForEtymology(
	notebookID, storyTitle, sceneTitle, expression, originalExpression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) bool {
	for hi, h := range u.history {
		if normalizeQuotes(h.Metadata.Title) != normalizeQuotes(storyTitle) {
			continue
		}

		for si, s := range h.Scenes {
			for ei, exp := range s.Expressions {
				if exp.Expression != expression && (originalExpression == "" || exp.Expression != originalExpression) {
					continue
				}
				// Skip vocab entries — only update origin entries (or
				// legacy type-empty entries that already carry etymology
				// logs, which we can safely upgrade).
				if exp.Type != LearningExpressionTypeOrigin {
					if exp.Type != "" {
						continue
					}
					if len(exp.EtymologyBreakdownLogs) == 0 && len(exp.EtymologyAssemblyLogs) == 0 {
						continue
					}
				}
				if exp.Type == "" {
					exp.Type = LearningExpressionTypeOrigin
				}
				exp.AddRecordWithQualityForEtymology(u.calculator, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
				u.history[hi].Scenes[si].Expressions[ei] = exp
				return true
			}
		}
	}

	u.createNewExpressionWithQualityForEtymology(notebookID, storyTitle, sceneTitle, expression, isCorrect, isKnownWord, quality, responseTimeMs, quizType)
	return false
}

// createNewExpressionWithQualityForEtymology creates a new expression entry
// with quality data for etymology quizzes. The entry is tagged
// Type=origin so it never collides with a vocab entry sharing the same
// name in the same scene (e.g., "ego" the word vs the Latin root).
func (u *LearningHistoryUpdater) createNewExpressionWithQualityForEtymology(
	notebookID, storyTitle, sceneTitle, expression string,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) {
	storyIndex := u.findOrCreateStory(notebookID, storyTitle, "")

	newExpression := LearningHistoryExpression{
		Expression:  expression,
		Type:        LearningExpressionTypeOrigin,
		LearnedLogs: []LearningRecord{},
	}
	newExpression.AddRecordWithQualityForEtymology(u.calculator, isCorrect, isKnownWord, quality, responseTimeMs, quizType)

	logs := newExpression.GetLogsForQuizType(quizType)
	if len(logs) == 0 {
		return
	}

	sceneIndex := u.findOrCreateScene(storyIndex, sceneTitle)
	u.history[storyIndex].Scenes[sceneIndex].Expressions = append(
		u.history[storyIndex].Scenes[sceneIndex].Expressions,
		newExpression,
	)
}

// AssertNoDuplicateOriginsInSession returns a non-nil error if the given
// session block holds the same origin expression under more than one
// scene. Used by SaveEtymologyOriginResult as a structural guard right
// before WriteYamlFile so any write that would re-introduce the "two
// logos sessions" class of bug fails loudly instead of silently
// corrupting the YAML. After the etymology source migration this
// invariant cannot fail in normal operation — the writer always
// addresses scenes the source declares — so a trip indicates either a
// real regression or hand-edited data that needs reconciliation.
func AssertNoDuplicateOriginsInSession(history []LearningHistory, notebookID, sessionTitle string) error {
	normalised := normalizeQuotes(sessionTitle)
	for _, h := range history {
		if normalizeQuotes(h.Metadata.Title) != normalised {
			continue
		}
		scenesByOrigin := make(map[string][]string)
		for _, scene := range h.Scenes {
			for _, expr := range scene.Expressions {
				if expr.Type != LearningExpressionTypeOrigin {
					continue
				}
				name := strings.TrimSpace(expr.Expression)
				if name == "" {
					continue
				}
				scenesByOrigin[name] = append(scenesByOrigin[name], scene.Metadata.Title)
			}
		}
		for origin, scenes := range scenesByOrigin {
			if len(scenes) > 1 {
				return fmt.Errorf(
					"invariant violation: origin %q appears in %d scenes (%v) within notebook %q session %q — refusing to write",
					origin, len(scenes), scenes, notebookID, sessionTitle,
				)
			}
		}
	}
	return nil
}

// ClearSkippedAt removes the skip for the given quiz type. The expression
// remains skipped for any other quiz types still set in its SkippedAt map.
func (u *LearningHistoryUpdater) ClearSkippedAt(expression string, quizType QuizType) bool {
	expr := u.FindExpressionByName(expression)
	if expr == nil {
		return false
	}
	expr.SkippedAt.Clear(quizType)
	return true
}
