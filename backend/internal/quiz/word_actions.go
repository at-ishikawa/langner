package quiz

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// loadSingleLearningHistory reads only the YAML for the requested notebook
// instead of walking the entire learning_notes directory. The previous
// implementation called NewLearningHistories on every Skip/Resume RPC,
// re-parsing every notebook's YAML; toggling the "All" master in the UI
// fires 3 parallel RPCs, tripling the cost. Returns an empty slice if the
// notebook's history file doesn't exist yet (a freshly-imported word).
func loadSingleLearningHistory(dir, notebookName string) ([]notebook.LearningHistory, error) {
	path := filepath.Join(dir, notebookName+".yml")
	hist, err := notebook.ReadLearningHistoryFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return hist, err
}

// CardInfo holds the minimal information needed to identify a word
// in the learning history for skip/resume/override operations.
//
// OriginalExpression carries the original Note.Expression form when
// the card's Expression is actually a Definition (e.g. a definitions-
// style notebook with a longer explanatory key). The YAML stores
// learning history under Note.Expression, so override has to fall
// back to OriginalExpression when matching by Expression misses.
//
// LearnedAt and MarkCorrect carry the user's intent on
// OverrideAnswer: the specific log timestamp they clicked on (so the
// service flips THAT entry, not blindly the latest) and the desired
// correct/incorrect state (so the service idempotently applies the
// intent instead of toggling whatever's there).
type CardInfo struct {
	NotebookName       string
	StoryTitle         string
	SceneTitle         string
	Expression         string
	OriginalExpression string
	// PartOfSpeech is the sense discriminator (issue #32), set by the
	// override handler from the request so Mark-as-Correct / Undo target
	// the correct homograph sense. Empty matches a legacy (untagged) log.
	PartOfSpeech string
	LearnedAt    string
	MarkCorrect  *bool
	// NoteID is the DB primary key for the note this card represents,
	// set by the handler when override is routed to a DB-backed repo.
	// Zero when the card came from a YAML-only deployment (DB-side
	// updates are a no-op in that case).
	NoteID int64
	// Used only by UndoOverrideAnswer — the pre-override snapshot the
	// frontend captured when it first called OverrideAnswer, so the
	// restore can put the log back to where it was without re-deriving
	// quality from a now-flipped status.
	OriginalQuality      int
	OriginalStatus       string
	OriginalIntervalDays int
}

// CardInfoFromCard converts a Card to CardInfo. OriginalExpression
// preserves the Note.Expression form when the card was loaded with a
// separate Definition entry — see CardInfo for why this fallback is
// required.
func CardInfoFromCard(card Card) CardInfo {
	return CardInfo{
		NotebookName:       card.NotebookName,
		StoryTitle:         card.StoryTitle,
		SceneTitle:         card.SceneTitle,
		Expression:         card.Entry,
		OriginalExpression: card.OriginalEntry,
	}
}

// CardInfoFromFreeformCard converts a FreeformCard to CardInfo.
func CardInfoFromFreeformCard(card FreeformCard) CardInfo {
	return CardInfo{
		NotebookName:       card.NotebookName,
		StoryTitle:         card.StoryTitle,
		SceneTitle:         card.SceneTitle,
		Expression:         card.Expression,
		OriginalExpression: card.OriginalExpression,
	}
}

// CardInfoFromReverseCard converts a ReverseCard to CardInfo. The
// reverse-quiz prompt is the meaning, the user types the word, and
// ReverseCard.Expression already holds Note.Expression — there's no
// separate Definition-as-key form to disambiguate, so no
// OriginalExpression fallback is needed here.
func CardInfoFromReverseCard(card ReverseCard) CardInfo {
	return CardInfo{
		NotebookName: card.NotebookName,
		StoryTitle:   card.StoryTitle,
		SceneTitle:   card.SceneTitle,
		Expression:   card.Expression,
	}
}

// SkipWord excludes a word from each of the given quiz types in a single
// read-modify-write of the notebook's learning history YAML. Batching avoids
// the race that bit the per-type API: when the UI's "All" toggle issued one
// RPC per type concurrently, every handler read the same pre-update file
// and the last writer overwrote the others, dropping skips.
//
// The skip is recorded as a per-(expression, quiz_type) timestamp on
// SkippedAt; quiz card loaders filter against that field. The skipUntil
// parameter is accepted for RPC compatibility but is not honored —
// exclusion is permanent until ResumeWord clears the slot.
//
// If the expression has no learning history yet, SkipWord seeds an entry so
// the skip has somewhere to live, then writes the skips onto it.
//
// When the expression belongs to a definitions concept (see Card.ConceptHead),
// the skip propagates to every sibling member of the concept in the same
// notebook — that's the "skip union" guarantee the read-side collapse
// relies on. Until migration moves logs to the head, the simplest way to
// keep both reads-from-head and reads-from-members consistent is to write
// the skip on each member entry.
func (s *Service) SkipWord(info CardInfo, skipUntil string, quizTypes []notebook.QuizType) error {
	if len(quizTypes) == 0 {
		return fmt.Errorf("at least one quiz type is required to skip a word")
	}
	history, err := loadSingleLearningHistory(s.notebooksConfig.LearningNotesDirectory, info.NotebookName)
	if err != nil {
		return fmt.Errorf("failed to load learning history for %q: %w", info.NotebookName, err)
	}

	updater := notebook.NewLearningHistoryUpdater(history, s.calculator)

	expressions := s.conceptMembersOrSelf(info.NotebookName, info.Expression)

	// Create a learned-log-free stub for each member if the expression has
	// no history yet — SetSkippedAt needs an entry to attach to, but we
	// must not invent a fake "quality 5" review log just because the user
	// skipped the word.
	for _, expr := range expressions {
		updater.EnsureExpressionStubForSkip(info.NotebookName, info.StoryTitle, info.SceneTitle, expr, "")
	}

	skippedAt := time.Now().Format(time.RFC3339)
	for _, expr := range expressions {
		for _, qt := range quizTypes {
			if !updater.SetSkippedAt(expr, "", qt, skippedAt) {
				return fmt.Errorf("failed to record skip for expression %q (%s) in notebook %q", expr, qt, info.NotebookName)
			}
		}
	}

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, info.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return fmt.Errorf("failed to save learning history for %q: %w", info.NotebookName, err)
	}
	return nil
}

// conceptMembersOrSelf returns the list of concept-sibling expressions for
// expression in the given notebook, including expression itself. When the
// expression doesn't belong to any concept (or the reader fails to load),
// it returns [expression]. Used by SkipWord/ResumeWord to propagate skips
// across all members of the same concept.
func (s *Service) conceptMembersOrSelf(notebookName, expression string) []string {
	reader, err := s.newReader()
	if err != nil {
		return []string{expression}
	}
	index := buildConceptIndex(reader, notebookName)
	info, ok := index[expression]
	if !ok || info == nil {
		return []string{expression}
	}
	return append([]string(nil), info.Members...)
}

// ResumeWord clears skips for each of the given quiz types so the word
// reappears in those modes. Other quiz types' skips are left intact, so a
// word excluded from multiple modes only resumes the ones the caller lists.
// Batched into a single read-modify-write for the same race-free reason as
// SkipWord.
func (s *Service) ResumeWord(info CardInfo, quizTypes []notebook.QuizType) error {
	if len(quizTypes) == 0 {
		return fmt.Errorf("at least one quiz type is required to resume a word")
	}
	history, err := loadSingleLearningHistory(s.notebooksConfig.LearningNotesDirectory, info.NotebookName)
	if err != nil {
		return fmt.Errorf("failed to load learning history for %q: %w", info.NotebookName, err)
	}

	updater := notebook.NewLearningHistoryUpdater(history, s.calculator)
	for _, expr := range s.conceptMembersOrSelf(info.NotebookName, info.Expression) {
		for _, qt := range quizTypes {
			updater.ClearSkippedAt(expr, "", qt)
		}
	}

	notePath := filepath.Join(s.notebooksConfig.LearningNotesDirectory, info.NotebookName+".yml")
	if err := notebook.WriteYamlFile(notePath, updater.GetHistory()); err != nil {
		return fmt.Errorf("failed to save learning history for %q: %w", info.NotebookName, err)
	}
	return nil
}

// OverrideResult captures the pre-change values of the affected log
// plus the recomputed next-review date. Surfaces the original* fields
// the frontend needs to render an "Undo" button after a Mark-as-Correct.
type OverrideResult struct {
	NextReviewDate       string
	OriginalQuality      int
	OriginalStatus       string
	OriginalIntervalDays int
}

// OverrideAnswer rewrites the log identified by (info, quizType,
// info.LearnedAt) according to info.MarkCorrect, on every configured
// storage backend.
//
// Storage routing: the service hands the override to
// s.learningRepository, which is whatever was wired at startup —
// YAML, DB, or MultiLearningRepository(YAML+DB). YAML reproduces the
// updater's full SM-2 recompute; DB does an UPDATE on
// learning_logs(note_id, quiz_type, learned_at). For multi-store
// setups, MultiLearningRepository runs the YAML write first and
// mirrors the exact values onto the DB so the two stores agree.
//
// info.LearnedAt MUST be the timestamp of the specific log the user
// clicked; the override targets THAT entry, not blindly logs[0]. When
// info.MarkCorrect is nil the call is a no-op for status/quality
// (kept for symmetry with the proto's optional field).
//
// Freeform variants flip the matching entry in both paired log lists
// in the same call so the two halves of one logical freeform answer
// stay consistent on disk.
//
// Returns the new next-review date as YYYY-MM-DD (empty when no
// matching log was found).
func (s *Service) OverrideAnswer(info CardInfo, quizType notebook.QuizType) (OverrideResult, error) {
	if s.learningRepository == nil {
		return OverrideResult{}, fmt.Errorf("no learning repository configured")
	}
	res, err := s.learningRepository.UpdateLog(context.Background(), learning.UpdateLogInput{
		NoteID:             info.NoteID,
		NotebookName:       info.NotebookName,
		StoryTitle:         info.StoryTitle,
		SceneTitle:         info.SceneTitle,
		Expression:         info.Expression,
		OriginalExpression: info.OriginalExpression,
		PartOfSpeech:       info.PartOfSpeech,
		QuizType:           string(quizType),
		LearnedAt:          parseLearnedAt(info.LearnedAt),
		MarkCorrect:        info.MarkCorrect,
	})
	if err != nil {
		return OverrideResult{}, fmt.Errorf("override learning log: %w", err)
	}
	return OverrideResult{
		NextReviewDate:       res.NewNextReviewDate,
		OriginalQuality:      res.OriginalQuality,
		OriginalStatus:       res.OriginalStatus,
		OriginalIntervalDays: res.OriginalIntervalDays,
	}, nil
}

// UndoOverrideAnswer restores a previously overridden log to the
// captured pre-override values (passed via info.OriginalQuality /
// OriginalStatus / OriginalIntervalDays). Returns the new next-review
// date and whether the restored entry is now considered correct.
//
// Implementation note: undo is just an override with MirrorValues
// pre-set to the originals — neither markCorrect nor the calculator
// is consulted, so the restored row is byte-identical to what it was
// before the user clicked Mark-as-Correct.
func (s *Service) UndoOverrideAnswer(info CardInfo, quizType notebook.QuizType) (correct bool, nextReview string, err error) {
	if s.learningRepository == nil {
		return false, "", fmt.Errorf("no learning repository configured")
	}
	res, err := s.learningRepository.UpdateLog(context.Background(), learning.UpdateLogInput{
		NoteID:             info.NoteID,
		NotebookName:       info.NotebookName,
		StoryTitle:         info.StoryTitle,
		SceneTitle:         info.SceneTitle,
		Expression:         info.Expression,
		OriginalExpression: info.OriginalExpression,
		PartOfSpeech:       info.PartOfSpeech,
		QuizType:           string(quizType),
		LearnedAt:          parseLearnedAt(info.LearnedAt),
		MirrorValues: &learning.UpdateLogMirror{
			Status:       info.OriginalStatus,
			Quality:      info.OriginalQuality,
			IntervalDays: info.OriginalIntervalDays,
		},
	})
	if err != nil {
		return false, "", fmt.Errorf("undo override learning log: %w", err)
	}
	correct = res.NewQuality >= 3
	return correct, res.NewNextReviewDate, nil
}

// parseLearnedAt accepts the YYYY-MM-DD or RFC3339 string the
// frontend sends and returns the time.Time the repos look up by. An
// unparseable string returns zero — UpdateLog implementations treat
// that as a no-op.
func parseLearnedAt(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t
	}
	return time.Time{}
}
