package notebook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type LearningHistory struct {
	Metadata    LearningHistoryMetadata     `yaml:"metadata"`
	Scenes      []LearningScene             `yaml:"scenes,omitempty"`
	Expressions []LearningHistoryExpression `yaml:"expressions,omitempty"`
}

type LearningHistoryMetadata struct {
	NotebookID string `yaml:"id"`
	Title      string `yaml:"title"`
	Type       string `yaml:"type,omitempty"`
}

func NewLearningHistories(directory string) (map[string][]LearningHistory, error) {
	return loadYamlFilesAsMap[[]LearningHistory](directory, func(path string, info os.FileInfo) bool {
		return !info.IsDir() && filepath.Ext(path) == ".yml"
	})
}

func (h LearningHistory) GetLogs(
	notebookTitle, sceneTitle string, definition Note,
) []LearningRecord {
	if normalizeQuotes(h.Metadata.Title) != normalizeQuotes(notebookTitle) {
		return nil
	}

	// For flashcard type, search in expressions directly
	if h.Metadata.Type == "flashcard" {
		for _, expression := range h.Expressions {
			if expression.Expression != definition.Expression && expression.Expression != definition.Definition {
				continue
			}
			if len(expression.LearnedLogs) > 0 {
				return expression.LearnedLogs
			}
		}
		return nil
	}

	// Search through scenes for the matching scene title
	normalizedSceneTitle := normalizeQuotes(sceneTitle)
	for _, scene := range h.Scenes {
		if normalizeQuotes(scene.Metadata.Title) != normalizedSceneTitle {
			continue
		}

		// Search through expressions in this scene
		for _, expression := range scene.Expressions {
			if expression.Expression != definition.Expression && expression.Expression != definition.Definition {
				continue
			}
			if len(expression.LearnedLogs) > 0 {
				return expression.LearnedLogs
			}
		}
	}
	return nil
}

// GetReverseLogs returns the reverse-quiz learning logs for the matching
// expression. Used to hydrate Note.ReverseLogs for PDF/notebook export so the
// filter can consider both forward and reverse tracks.
func (h LearningHistory) GetReverseLogs(
	notebookTitle, sceneTitle string, definition Note,
) []LearningRecord {
	if normalizeQuotes(h.Metadata.Title) != normalizeQuotes(notebookTitle) {
		return nil
	}

	if h.Metadata.Type == "flashcard" {
		for _, expression := range h.Expressions {
			if expression.Expression != definition.Expression && expression.Expression != definition.Definition {
				continue
			}
			return expression.ReverseLogs
		}
		return nil
	}

	normalizedSceneTitle := normalizeQuotes(sceneTitle)
	for _, scene := range h.Scenes {
		if normalizeQuotes(scene.Metadata.Title) != normalizedSceneTitle {
			continue
		}
		for _, expression := range scene.Expressions {
			if expression.Expression != definition.Expression && expression.Expression != definition.Definition {
				continue
			}
			return expression.ReverseLogs
		}
	}
	return nil
}

type LearningScene struct {
	Metadata    LearningSceneMetadata `yaml:"metadata"`
	Expressions []LearningHistoryExpression
}

type LearningSceneMetadata struct {
	Title string `yaml:"title"`
}

// LearningRecord represents a single learning event for an expression
type LearningRecord struct {
	Status         LearnedStatus `yaml:"status,omitempty"`
	LearnedAt      Date          `yaml:"learned_at,omitempty"`
	Quality        int           `yaml:"quality,omitempty"`          // 0-5 grade
	ResponseTimeMs int64         `yaml:"response_time_ms,omitempty"` // milliseconds
	QuizType       string        `yaml:"quiz_type,omitempty"`        // "freeform" or "notebook"
	IntervalDays     int           `yaml:"interval_days,omitempty"`      // days until next review
	OverrideInterval int           `yaml:"override_interval,omitempty"` // manually-set interval (non-zero = user override)
}

type LearningHistoryExpression struct {
	Expression     string           `yaml:"expression"`
	LearnedLogs    []LearningRecord `yaml:"learned_logs"`
	EasinessFactor float64          `yaml:"-"` // derived on the fly from logs

	// Reverse quiz fields - track separately from regular quiz
	ReverseLogs           []LearningRecord `yaml:"reverse_logs,omitempty"`
	ReverseEasinessFactor float64          `yaml:"-"` // derived on the fly from logs

	// Etymology breakdown quiz fields - track separately
	EtymologyBreakdownLogs           []LearningRecord `yaml:"etymology_breakdown_logs,omitempty"`
	EtymologyBreakdownEasinessFactor float64          `yaml:"-"` // derived on the fly from logs

	// Etymology assembly quiz fields - track separately
	EtymologyAssemblyLogs           []LearningRecord `yaml:"etymology_assembly_logs,omitempty"`
	EtymologyAssemblyEasinessFactor float64          `yaml:"-"` // derived on the fly from logs

	// SkippedAt records, per quiz type, when the user excluded this expression
	// from that quiz mode. A word skipped only from `reverse` will still appear
	// in `notebook` (standard) and `freeform`. Legacy plain-string YAML values
	// (a single timestamp meaning "skipped from everywhere") are upgraded to a
	// fully-populated map at unmarshal time.
	SkippedAt SkippedAtMap `yaml:"skipped_at,omitempty"`
}

// SkippedAtMap maps a quiz type string (e.g. "reverse", "etymology_freeform")
// to an RFC3339 timestamp marking when that quiz mode's exclusion was set.
type SkippedAtMap map[string]string

// allQuizTypeKeys returns every quiz type string the system can record a
// skip against. Kept private to this file because the canonical list lives
// in quiz_type.go; this helper is only used by the legacy-format upgrade.
func allQuizTypeKeys() []string {
	return []string{
		string(QuizTypeNotebook),
		string(QuizTypeReverse),
		string(QuizTypeFreeform),
		string(QuizTypeEtymologyStandard),
		string(QuizTypeEtymologyReverse),
		string(QuizTypeEtymologyFreeform),
	}
}

// UnmarshalYAML accepts both the new map shape and the pre-migration plain
// string. A bare string upgrades to a map with the timestamp written under
// every quiz type key, matching the old "skipped from everywhere" semantics.
func (m *SkippedAtMap) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		return nil
	}
	switch value.Kind {
	case yaml.ScalarNode:
		if value.Value == "" {
			*m = nil
			return nil
		}
		out := make(SkippedAtMap, len(allQuizTypeKeys()))
		for _, k := range allQuizTypeKeys() {
			out[k] = value.Value
		}
		*m = out
		return nil
	case yaml.MappingNode:
		out := make(SkippedAtMap, len(value.Content)/2)
		for i := 0; i+1 < len(value.Content); i += 2 {
			k := value.Content[i].Value
			v := value.Content[i+1].Value
			if k != "" && v != "" {
				out[k] = v
			}
		}
		if len(out) == 0 {
			*m = nil
			return nil
		}
		*m = out
		return nil
	default:
		return fmt.Errorf("skipped_at must be a string or mapping, got kind %d", value.Kind)
	}
}

// IsSkipped reports whether the expression is excluded from the given quiz type.
func (m SkippedAtMap) IsSkipped(quizType QuizType) bool {
	if m == nil {
		return false
	}
	return m[string(quizType)] != ""
}

// IsSkippedAny reports whether the expression is excluded from at least one quiz type.
// Used for UI badges that summarise per-type skips.
func (m SkippedAtMap) IsSkippedAny() bool {
	for _, v := range m {
		if v != "" {
			return true
		}
	}
	return false
}

// SkippedTypes returns the quiz types this expression is excluded from, in
// the canonical order from allQuizTypeKeys.
func (m SkippedAtMap) SkippedTypes() []string {
	if len(m) == 0 {
		return nil
	}
	var out []string
	for _, k := range allQuizTypeKeys() {
		if m[k] != "" {
			out = append(out, k)
		}
	}
	return out
}

// Set records a skip for the given quiz type at the given timestamp.
// Returns the (possibly newly allocated) map so callers writing into a
// nil-valued field get the result back.
func (m SkippedAtMap) Set(quizType QuizType, at string) SkippedAtMap {
	if m == nil {
		m = make(SkippedAtMap)
	}
	m[string(quizType)] = at
	return m
}

// Clear removes the skip for the given quiz type (no-op if not set).
func (m SkippedAtMap) Clear(quizType QuizType) {
	if m == nil {
		return
	}
	delete(m, string(quizType))
}

func (exp LearningHistoryExpression) GetLatestStatus() LearnedStatus {
	if len(exp.LearnedLogs) == 0 {
		return LearnedStatusLearning
	}
	// Get the first element since new logs are prepended
	lastLog := exp.LearnedLogs[0]
	return lastLog.Status
}

// GetLogsForQuizType returns learning logs for the specified quiz type.
// For etymology freeform, returns breakdown logs (the primary track).
func (exp LearningHistoryExpression) GetLogsForQuizType(quizType QuizType) []LearningRecord {
	switch quizType {
	case QuizTypeReverse:
		return exp.ReverseLogs
	case QuizTypeEtymologyStandard, QuizTypeEtymologyFreeform:
		return exp.EtymologyBreakdownLogs
	case QuizTypeEtymologyReverse:
		return exp.EtymologyAssemblyLogs
	default:
		return exp.LearnedLogs
	}
}

// SetLogsForQuizType sets learning logs for the specified quiz type.
// For etymology freeform, sets BOTH breakdown and assembly logs.
func (exp *LearningHistoryExpression) SetLogsForQuizType(quizType QuizType, logs []LearningRecord) {
	switch quizType {
	case QuizTypeReverse:
		exp.ReverseLogs = logs
	case QuizTypeEtymologyStandard:
		exp.EtymologyBreakdownLogs = logs
	case QuizTypeEtymologyReverse:
		exp.EtymologyAssemblyLogs = logs
	case QuizTypeEtymologyFreeform:
		exp.EtymologyBreakdownLogs = logs
		exp.EtymologyAssemblyLogs = logs
	default:
		exp.LearnedLogs = logs
	}
}

// NeedsForwardReview returns true if the expression needs forward quiz review
// based on spaced repetition algorithm
func (exp LearningHistoryExpression) NeedsForwardReview() bool {
	if len(exp.LearnedLogs) == 0 {
		return true
	}

	lastLog := exp.LearnedLogs[0]

	// Always include misunderstood expressions for review
	if lastLog.Status == LearnedStatusMisunderstood {
		return true
	}

	// Use stored interval if available
	threshold := lastLog.IntervalDays
	if threshold == 0 {
		// Fallback: calculate based on correct streak
		correctCount := 0
		for _, log := range exp.LearnedLogs {
			if log.Status != LearnedStatusMisunderstood && log.Status != LearnedStatusLearning {
				correctCount++
			}
		}
		threshold = GetThresholdDaysFromCount(correctCount)
	}

	// Calculate elapsed days since last review
	now := time.Now()
	elapsed := now.Sub(lastLog.LearnedAt.Time)
	elapsedDays := int(elapsed.Hours() / 24)

	// Need review if elapsed days >= threshold
	return elapsedDays >= threshold
}

// GetEasinessFactorForQuizType returns the easiness factor for the specified quiz type
func (exp LearningHistoryExpression) GetEasinessFactorForQuizType(quizType QuizType) float64 {
	switch quizType {
	case QuizTypeReverse:
		if exp.ReverseEasinessFactor == 0 {
			return DefaultEasinessFactor
		}
		return exp.ReverseEasinessFactor
	case QuizTypeEtymologyStandard:
		if exp.EtymologyBreakdownEasinessFactor == 0 {
			return DefaultEasinessFactor
		}
		return exp.EtymologyBreakdownEasinessFactor
	case QuizTypeEtymologyReverse:
		if exp.EtymologyAssemblyEasinessFactor == 0 {
			return DefaultEasinessFactor
		}
		return exp.EtymologyAssemblyEasinessFactor
	default:
		if exp.EasinessFactor == 0 {
			return DefaultEasinessFactor
		}
		return exp.EasinessFactor
	}
}

// AddRecordWithQualityForReverse adds a new learning record for reverse quiz with quality data.
func (exp *LearningHistoryExpression) AddRecordWithQualityForReverse(
	calculator IntervalCalculator,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) {
	status := LearnedStatusMisunderstood
	if isCorrect {
		if isKnownWord {
			status = LearnedStatusUnderstood
		} else {
			status = LearnedStatusCanBeUsed
		}
	}

	currentEF := calculator.DeriveEF(exp.ReverseLogs)

	nextInterval, _ := calculator.CalculateInterval(exp.ReverseLogs, quality, currentEF)

	newRecord := LearningRecord{
		Status:         status,
		LearnedAt:      NewDate(),
		Quality:        quality,
		ResponseTimeMs: responseTimeMs,
		QuizType:       string(quizType),
		IntervalDays:   nextInterval,
	}

	exp.ReverseLogs = append([]LearningRecord{newRecord}, exp.ReverseLogs...)
}

// AddRecordWithQualityForEtymology adds a new learning record for etymology quiz with SM-2 quality data
func (exp *LearningHistoryExpression) AddRecordWithQualityForEtymology(
	calculator IntervalCalculator,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) {
	status := LearnedStatusMisunderstood
	if isCorrect {
		if isKnownWord {
			status = LearnedStatusUnderstood
		} else {
			status = LearnedStatusCanBeUsed
		}
	}

	addRecord := func(logs []LearningRecord) []LearningRecord {
		currentEF := calculator.DeriveEF(logs)
		nextInterval, _ := calculator.CalculateInterval(logs, quality, currentEF)
		newRecord := LearningRecord{
			Status:         status,
			LearnedAt:      NewDate(),
			Quality:        quality,
			ResponseTimeMs: responseTimeMs,
			QuizType:       string(quizType),
			IntervalDays:   nextInterval,
		}
		return append([]LearningRecord{newRecord}, logs...)
	}

	switch quizType {
	case QuizTypeEtymologyStandard:
		exp.EtymologyBreakdownLogs = addRecord(exp.EtymologyBreakdownLogs)
	case QuizTypeEtymologyReverse:
		exp.EtymologyAssemblyLogs = addRecord(exp.EtymologyAssemblyLogs)
	case QuizTypeEtymologyFreeform:
		exp.EtymologyBreakdownLogs = addRecord(exp.EtymologyBreakdownLogs)
		exp.EtymologyAssemblyLogs = addRecord(exp.EtymologyAssemblyLogs)
	}
}

// NeedsEtymologyReview returns true if the expression needs etymology quiz review
func (exp LearningHistoryExpression) NeedsEtymologyReview(quizType QuizType) bool {
	logs := exp.GetLogsForQuizType(quizType)
	if len(logs) == 0 {
		return true
	}

	lastLog := logs[0]
	if lastLog.Status == LearnedStatusMisunderstood {
		return true
	}

	threshold := lastLog.IntervalDays
	if threshold == 0 {
		correctCount := 0
		for _, log := range logs {
			if log.Status != LearnedStatusMisunderstood && log.Status != LearnedStatusLearning {
				correctCount++
			}
		}
		threshold = GetThresholdDaysFromCount(correctCount)
	}

	now := time.Now()
	elapsed := now.Sub(lastLog.LearnedAt.Time)
	elapsedDays := int(elapsed.Hours() / 24)
	return elapsedDays >= threshold
}

// HasAnyCorrectAnswer returns true if the expression has at least one correct answer
// in the forward quiz (LearnedLogs). This is used to determine if a word is ready
// for reverse quiz - words should be learned in forward direction first.
func (exp LearningHistoryExpression) HasAnyCorrectAnswer() bool {
	for _, log := range exp.LearnedLogs {
		if log.Status == LearnedStatusUnderstood ||
			log.Status == LearnedStatusCanBeUsed ||
			log.Status == learnedStatusIntuitivelyUsed {
			return true
		}
	}
	return false
}

// HasFreeformAnswer returns true if the expression has at least one freeform quiz
// answer recorded in LearnedLogs. Vocabulary words must be answered in freeform mode
// first before becoming eligible for standard or reverse quizzes.
func (exp LearningHistoryExpression) HasFreeformAnswer() bool {
	for _, log := range exp.LearnedLogs {
		if log.QuizType == string(QuizTypeFreeform) {
			return true
		}
	}
	return false
}

// HasEtymologyFreeformAnswer returns true if the expression has at least one
// etymology freeform answer recorded in EtymologyBreakdownLogs. Origins must be
// answered in etymology freeform mode first before becoming eligible for etymology
// standard or reverse quizzes.
func (exp LearningHistoryExpression) HasEtymologyFreeformAnswer() bool {
	for _, log := range exp.EtymologyBreakdownLogs {
		if log.QuizType == string(QuizTypeEtymologyFreeform) {
			return true
		}
	}
	return false
}

// HasCorrectEtymologyAnswer returns true if the expression has at least one
// correct answer recorded in EtymologyBreakdownLogs. Origins must have been
// answered correctly at least once (in any etymology mode) before becoming
// eligible for etymology standard or reverse quizzes — otherwise the user
// would be drilled on origins they have never actually understood.
func (exp LearningHistoryExpression) HasCorrectEtymologyAnswer() bool {
	for _, log := range exp.EtymologyBreakdownLogs {
		if log.Status == LearnedStatusUnderstood ||
			log.Status == LearnedStatusCanBeUsed ||
			log.Status == learnedStatusIntuitivelyUsed {
			return true
		}
	}
	return false
}

// NeedsReverseReview returns true if the expression needs reverse quiz review
// based on spaced repetition algorithm
func (exp LearningHistoryExpression) NeedsReverseReview() bool {
	if len(exp.ReverseLogs) == 0 {
		return true
	}

	lastLog := exp.ReverseLogs[0]

	// Always include misunderstood expressions for review
	if lastLog.Status == LearnedStatusMisunderstood {
		return true
	}

	// Use stored interval if available
	threshold := lastLog.IntervalDays
	if threshold == 0 {
		// Fallback: calculate based on correct streak
		correctCount := 0
		for _, log := range exp.ReverseLogs {
			if log.Status != LearnedStatusMisunderstood && log.Status != LearnedStatusLearning {
				correctCount++
			}
		}
		threshold = GetThresholdDaysFromCount(correctCount)
	}

	// Calculate elapsed days since last review
	// LearnedAt is stored as RFC3339 timestamp, so we can calculate actual elapsed time
	now := time.Now()
	elapsed := now.Sub(lastLog.LearnedAt.Time)
	elapsedDays := int(elapsed.Hours() / 24)

	// Need review if elapsed days >= threshold
	return elapsedDays >= threshold
}

// AddRecordWithQuality adds a new learning record with quality data.
func (exp *LearningHistoryExpression) AddRecordWithQuality(
	calculator IntervalCalculator,
	isCorrect, isKnownWord bool,
	quality int,
	responseTimeMs int64,
	quizType QuizType,
) {
	status := LearnedStatusMisunderstood
	if isCorrect {
		if isKnownWord {
			status = LearnedStatusUnderstood
		} else {
			status = LearnedStatusCanBeUsed
		}
	}

	currentEF := calculator.DeriveEF(exp.LearnedLogs)

	nextInterval, _ := calculator.CalculateInterval(exp.LearnedLogs, quality, currentEF)

	newRecord := LearningRecord{
		Status:         status,
		LearnedAt:      NewDate(),
		Quality:        quality,
		ResponseTimeMs: responseTimeMs,
		QuizType:       string(quizType),
		IntervalDays:   nextInterval,
	}

	exp.LearnedLogs = append([]LearningRecord{newRecord}, exp.LearnedLogs...)
}

// IsExpressionSkipped checks whether a note is excluded from the given quiz
// type. Per-type skips replaced the old "skipped from everything" string —
// each call site supplies the quiz mode it's filtering for.
func IsExpressionSkipped(histories []LearningHistory, event, sceneTitle string, expression, definition string, quizType QuizType) bool {
	for _, hist := range histories {
		if hist.Metadata.Title != event {
			continue
		}
		if hist.Metadata.Type == "flashcard" {
			for _, expr := range hist.Expressions {
				if expr.Expression == expression || expr.Expression == definition {
					return expr.SkippedAt.IsSkipped(quizType)
				}
			}
			continue
		}
		for _, scene := range hist.Scenes {
			if scene.Metadata.Title != sceneTitle {
				continue
			}
			for _, expr := range scene.Expressions {
				if expr.Expression == expression || expr.Expression == definition {
					return expr.SkippedAt.IsSkipped(quizType)
				}
			}
		}
	}
	return false
}

// Validate validates a LearningHistoryExpression
func (exp *LearningHistoryExpression) Validate(location string) []ValidationError {
	var errors []ValidationError

	// Check if expression is not empty
	if strings.TrimSpace(exp.Expression) == "" {
		errors = append(errors, ValidationError{
			Location: location,
			Message:  "expression field is empty",
		})
		return errors
	}

	// Skip validation if no learned_logs
	if len(exp.LearnedLogs) == 0 {
		return errors
	}

	// Validate learned logs
	validStatuses := map[LearnedStatus]bool{
		LearnedStatusLearning:        true,
		LearnedStatusMisunderstood:   true,
		LearnedStatusUnderstood:      true,
		LearnedStatusCanBeUsed:       true,
		learnedStatusIntuitivelyUsed: true,
	}

	var prevDate time.Time

	for logIdx, log := range exp.LearnedLogs {
		logLocation := fmt.Sprintf("%s -> log[%d]", location, logIdx)

		// Validate status
		if !validStatuses[log.Status] {
			errors = append(errors, ValidationError{
				Location: logLocation,
				Message:  fmt.Sprintf("invalid status: %q", log.Status),
				Suggestions: []string{
					"valid statuses are: '', 'misunderstood', 'understood', 'usable', 'intuitive'",
				},
			})
		}

		// Validate date format and required field
		if log.LearnedAt.IsZero() {
			errors = append(errors, ValidationError{
				Location: logLocation,
				Message:  "learned_at is required but missing or invalid",
				Suggestions: []string{
					"use format YYYY-MM-DD",
				},
			})
			continue
		}

		// Check chronological order (logs should be sorted newest first)
		if logIdx > 0 && log.LearnedAt.After(prevDate) {
			errors = append(errors, ValidationError{
				Location: logLocation,
				Message:  fmt.Sprintf("learned_logs not in chronological order (newest first): %s comes after %s", log.LearnedAt.Format("2006-01-02"), prevDate.Format("2006-01-02")),
				Suggestions: []string{
					"sort learned_logs by date in descending order (newest first)",
				},
			})
		}
		prevDate = log.LearnedAt.Time
	}

	return errors
}

// Validate validates a LearningScene
func (scene *LearningScene) Validate(location string) []ValidationError {
	var errors []ValidationError

	for exprIdx, expr := range scene.Expressions {
		exprLocation := fmt.Sprintf("%s -> expression[%d]: %s", location, exprIdx, expr.Expression)
		if exprErrors := expr.Validate(exprLocation); len(exprErrors) > 0 {
			errors = append(errors, exprErrors...)
		}
	}

	return errors
}

// Validate validates a LearningHistory
func (h *LearningHistory) Validate(location string) []ValidationError {
	var errors []ValidationError

	// For flashcard type, validate expressions directly
	if h.Metadata.Type == "flashcard" {
		for exprIdx, expr := range h.Expressions {
			exprLocation := fmt.Sprintf("%s -> expression[%d]: %s", location, exprIdx, expr.Expression)
			if exprErrors := expr.Validate(exprLocation); len(exprErrors) > 0 {
				errors = append(errors, exprErrors...)
			}
		}

		// Check for duplicate expressions in flashcard format
		expressionSeen := make(map[string]bool)
		for _, expr := range h.Expressions {
			expression := strings.TrimSpace(expr.Expression)
			if expression == "" {
				continue
			}
			if expressionSeen[expression] {
				errors = append(errors, ValidationError{
					Location: location,
					Message:  fmt.Sprintf("duplicate expression %q in flashcard format", expression),
				})
			}
			expressionSeen[expression] = true
		}

		return errors
	}

	// For story type (default), validate scenes
	for sceneIdx, scene := range h.Scenes {
		sceneLocation := fmt.Sprintf("%s -> scene[%d]: %s", location, sceneIdx, scene.Metadata.Title)
		if sceneErrors := scene.Validate(sceneLocation); len(sceneErrors) > 0 {
			errors = append(errors, sceneErrors...)
		}
	}

	// Etymology histories use scene titles to disambiguate multi-sense origins
	// (e.g. "ana" = "up" in one session, "negative" in another). The same
	// expression appearing in two scenes is intentional, not a duplicate.
	if h.Metadata.Type == "etymology" {
		return errors
	}

	// Check for duplicate expressions across different scenes in the same episode
	episodeExpressions := make(map[string][]string) // expression -> list of scene titles
	for _, scene := range h.Scenes {
		sceneTitle := strings.TrimSpace(scene.Metadata.Title)
		for _, expr := range scene.Expressions {
			expression := strings.TrimSpace(expr.Expression)
			if expression == "" {
				continue
			}
			episodeExpressions[expression] = append(episodeExpressions[expression], sceneTitle)
		}
	}

	// Report duplicates across scenes
	for expression, scenes := range episodeExpressions {
		if len(scenes) > 1 {
			errors = append(errors, ValidationError{
				Location: location,
				Message:  fmt.Sprintf("expression %q appears in multiple scenes: %v", expression, scenes),
				Suggestions: []string{
					"run validate --fix to merge duplicate expressions",
				},
			})
		}
	}

	return errors
}
