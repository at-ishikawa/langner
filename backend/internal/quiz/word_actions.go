package quiz

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
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
	// Sense routes etymology-origin overrides to the exact (origin, sense)
	// series via the canonical lookup; StoryTitle carries the session title
	// for etymology cards. Empty for vocabulary cards.
	Sense string
	// ID is the note's stable sense id (issue #32), set by the override
	// handler from the request so Mark-as-Correct / Undo target the exact
	// entry. Empty resolves by expression (legacy, pre-migration).
	ID          string
	LearnedAt   string
	MarkCorrect *bool
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
func (s *Service) SkipWord(userID int64, info CardInfo, skipUntil string, quizTypes []notebook.QuizType) error {
	if len(quizTypes) == 0 {
		return fmt.Errorf("at least one quiz type is required to skip a word")
	}
	// DB mode: record the exclude marker in the DB skip-flag tables and DO NOT
	// touch the on-disk learning_notes YAML (DB-only writes). The marker is
	// stamped with userID so it excludes the word only for this account.
	if s.skipFlagRepo != nil {
		return s.setSkipDB(context.Background(), userID, info, quizTypes, true)
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

// setSkipDB is the DB-only-writes implementation of SkipWord/ResumeWord: it
// UPSERTs (skip=true) or DELETEs (skip=false) the per-quiz-type exclude marker
// in the DB skip-flag tables — note_skip_flags for vocabulary notes,
// origin_skip_flags for etymology origins — instead of writing the on-disk
// learning_notes YAML. It preserves the YAML path's concept-member propagation
// (the skip lands on every sibling) and per-quiz-type scoping. The KEY it
// writes under (note_id/origin_id + quiz_type) is the SAME key the read side
// (DBHistoryStore) reconstructs skip flags under, keeping the loaders' skip
// filter symmetric with the write (learning-history invariant L2).
func (s *Service) setSkipDB(ctx context.Context, userID int64, info CardInfo, quizTypes []notebook.QuizType, skip bool) error {
	// Grammar corrections are not notes or origins and have no DB skip-flag
	// table, so a grammar exclude cannot be persisted in DB mode today (its
	// read side is reconstructed from grammar_corrections, which carries no
	// skip). Drop it with a warning rather than fall through to note resolution
	// (a correction sense_id is not a note) or write frozen YAML.
	persisted := make([]notebook.QuizType, 0, len(quizTypes))
	for _, qt := range quizTypes {
		if qt == notebook.QuizTypeGrammar {
			slog.Warn("grammar exclude is not persisted in DB mode: no grammar skip-flag table exists",
				"notebook", info.NotebookName, "expression", info.Expression)
			continue
		}
		persisted = append(persisted, qt)
	}
	if len(persisted) == 0 {
		return nil
	}

	at := time.Now()
	for _, expr := range s.conceptMembersOrSelf(info.NotebookName, info.Expression) {
		noteID, originID, err := s.resolveSkipTarget(ctx, info, expr)
		if err != nil {
			return err
		}
		for _, qt := range persisted {
			var applyErr error
			switch {
			case noteID > 0 && skip:
				applyErr = s.skipFlagRepo.SkipNote(ctx, userID, noteID, string(qt), at)
			case noteID > 0:
				applyErr = s.skipFlagRepo.ResumeNote(ctx, userID, noteID, string(qt))
			case skip:
				applyErr = s.skipFlagRepo.SkipOrigin(ctx, userID, originID, string(qt), at)
			default:
				applyErr = s.skipFlagRepo.ResumeOrigin(ctx, userID, originID, string(qt))
			}
			if applyErr != nil {
				return fmt.Errorf("record exclude for %q (%s) in notebook %q: %w", expr, qt, info.NotebookName, applyErr)
			}
		}
	}
	return nil
}

// resolveSkipTarget maps a (notebook, expression) to the note_id or origin_id
// the read side keys skip flags under. It prefers the DB note_id the caller
// already carries (info.NoteID, set by the Learn-page handler) for the primary
// expression — homograph-safe — then a sense_id match, then the note's surface
// (usage/entry) within the notebook, exactly the mapping DBHistoryStore
// reconstructs. It falls back to an etymology origin. It returns an error
// (never a silent no-op) when the expression matches neither, or matches more
// than one note ambiguously (a homograph with no stable id) — naming the word
// so the failure is actionable rather than skipping the wrong sense.
func (s *Service) resolveSkipTarget(ctx context.Context, info CardInfo, expression string) (noteID, originID int64, err error) {
	if expression == info.Expression && info.NoteID > 0 {
		return info.NoteID, 0, nil
	}

	target := strings.ToLower(strings.TrimSpace(expression))
	if s.noteRepo != nil {
		notes, ferr := s.noteRepo.FindAll(ctx)
		if ferr != nil {
			return 0, 0, fmt.Errorf("load notes to resolve exclude target: %w", ferr)
		}
		seen := make(map[int64]bool)
		var matches []int64
		for i := range notes {
			n := notes[i]
			linked := false
			for _, nn := range n.NotebookNotes {
				if nn.NotebookID == info.NotebookName {
					linked = true
					break
				}
			}
			if !linked {
				continue
			}
			if info.ID != "" && n.SenseID == info.ID {
				return n.ID, 0, nil
			}
			if strings.ToLower(strings.TrimSpace(n.Entry)) == target || strings.ToLower(strings.TrimSpace(n.Usage)) == target {
				if !seen[n.ID] {
					seen[n.ID] = true
					matches = append(matches, n.ID)
				}
			}
		}
		if len(matches) == 1 {
			return matches[0], 0, nil
		}
		if len(matches) > 1 {
			return 0, 0, fmt.Errorf("cannot exclude %q in notebook %q: it matches %d notes (homograph), a stable note id is required to disambiguate", expression, info.NotebookName, len(matches))
		}
	}

	// Not a vocabulary note — try an etymology origin, bound per sense by the
	// session title the same way GetEtymologyNotebook keys origins.
	if s.originRepo != nil {
		origins, oerr := s.originRepo.FindAll(ctx)
		if oerr != nil {
			return 0, 0, fmt.Errorf("load origins to resolve exclude target: %w", oerr)
		}
		for _, o := range origins {
			if o.NotebookID != info.NotebookName {
				continue
			}
			if strings.ToLower(strings.TrimSpace(o.Origin)) != target {
				continue
			}
			if info.StoryTitle != "" && o.SessionTitle != info.StoryTitle {
				continue
			}
			return 0, o.ID, nil
		}
	}

	return 0, 0, fmt.Errorf("cannot exclude %q: no matching note or origin in notebook %q", expression, info.NotebookName)
}

// ResumeWord clears skips for each of the given quiz types so the word
// reappears in those modes. Other quiz types' skips are left intact, so a
// word excluded from multiple modes only resumes the ones the caller lists.
// Batched into a single read-modify-write for the same race-free reason as
// SkipWord.
func (s *Service) ResumeWord(userID int64, info CardInfo, quizTypes []notebook.QuizType) error {
	if len(quizTypes) == 0 {
		return fmt.Errorf("at least one quiz type is required to resume a word")
	}
	// DB mode: clear the exclude marker from the DB skip-flag tables and DO NOT
	// touch the on-disk learning_notes YAML (DB-only writes). Only this user's
	// marker is cleared.
	if s.skipFlagRepo != nil {
		return s.setSkipDB(context.Background(), userID, info, quizTypes, false)
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
func (s *Service) OverrideAnswer(userID int64, info CardInfo, quizType notebook.QuizType) (OverrideResult, error) {
	if s.learningRepository == nil {
		return OverrideResult{}, fmt.Errorf("no learning repository configured")
	}
	res, err := s.learningRepository.UpdateLog(context.Background(), learning.UpdateLogInput{
		UserID:             userID,
		NoteID:             info.NoteID,
		NotebookName:       info.NotebookName,
		StoryTitle:         info.StoryTitle,
		SceneTitle:         info.SceneTitle,
		Expression:         info.Expression,
		OriginalExpression: info.OriginalExpression,
		ID:                 info.ID,
		Sense:              info.Sense,
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
func (s *Service) UndoOverrideAnswer(userID int64, info CardInfo, quizType notebook.QuizType) (correct bool, nextReview string, err error) {
	if s.learningRepository == nil {
		return false, "", fmt.Errorf("no learning repository configured")
	}
	res, err := s.learningRepository.UpdateLog(context.Background(), learning.UpdateLogInput{
		UserID:             userID,
		NoteID:             info.NoteID,
		NotebookName:       info.NotebookName,
		StoryTitle:         info.StoryTitle,
		SceneTitle:         info.SceneTitle,
		Expression:         info.Expression,
		OriginalExpression: info.OriginalExpression,
		ID:                 info.ID,
		Sense:              info.Sense,
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
