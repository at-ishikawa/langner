package datasync

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/at-ishikawa/langner/internal/learning"
	"github.com/at-ishikawa/langner/internal/notebook"
)

// StateSeedResult summarises a SeedState run for the migrate import CLI.
type StateSeedResult struct {
	DefinitionsSessionsCreated int
	DefinitionsScenesCreated   int
	FlashcardDecksCreated      int
	NoteSkipFlagsCreated       int
	OriginSkipFlagsCreated     int
	EtymologyLogsCreated       int
}

// StateSeeder populates the migration-016 tables (definitions_sessions /
// scenes, flashcard_decks, note_skip_flags, origin_skip_flags) plus the
// etymology rows in learning_logs from the existing YAML. Idempotent:
// each phase upserts so a re-run skips already-seeded rows.
type StateSeeder struct {
	reader        *notebook.Reader
	noteRepo      notebook.NoteRepository
	originRepo    notebook.EtymologyOriginRepository
	defsRepo      notebook.DefinitionsRepository
	flashcardRepo notebook.FlashcardDeckRepository
	skipFlagRepo  notebook.SkipFlagRepository
	learningRepo  learning.LearningRepository
	learningSrc   LearningSource
	writer        io.Writer
}

// NewStateSeeder constructs a seeder. learningSrc is the YAML-side
// learning history reader used to pull skip flags and etymology logs.
func NewStateSeeder(
	reader *notebook.Reader,
	noteRepo notebook.NoteRepository,
	originRepo notebook.EtymologyOriginRepository,
	defsRepo notebook.DefinitionsRepository,
	flashcardRepo notebook.FlashcardDeckRepository,
	skipFlagRepo notebook.SkipFlagRepository,
	learningRepo learning.LearningRepository,
	learningSrc LearningSource,
	writer io.Writer,
) *StateSeeder {
	return &StateSeeder{
		reader:        reader,
		noteRepo:      noteRepo,
		originRepo:    originRepo,
		defsRepo:      defsRepo,
		flashcardRepo: flashcardRepo,
		skipFlagRepo:  skipFlagRepo,
		learningRepo:  learningRepo,
		learningSrc:   learningSrc,
		writer:        writer,
	}
}

// SeedAll runs every seed phase and returns aggregated counts.
func (s *StateSeeder) SeedAll(ctx context.Context) (*StateSeedResult, error) {
	result := &StateSeedResult{}

	if err := s.seedDefinitions(ctx, result); err != nil {
		return result, fmt.Errorf("seed definitions structure: %w", err)
	}
	if err := s.seedFlashcards(ctx, result); err != nil {
		return result, fmt.Errorf("seed flashcard decks: %w", err)
	}
	if err := s.seedSkipFlagsAndEtymologyLogs(ctx, result); err != nil {
		return result, fmt.Errorf("seed skip flags and etymology logs: %w", err)
	}
	return result, nil
}

func (s *StateSeeder) seedDefinitions(ctx context.Context, result *StateSeedResult) error {
	if s.reader == nil || s.defsRepo == nil {
		return nil
	}
	for _, bookID := range s.reader.GetDefinitionsBookIDs() {
		books, ok := s.reader.GetDefinitionsBook(bookID)
		if !ok {
			continue
		}
		for _, book := range books {
			title := book.Metadata.Title
			if title == "" {
				title = book.Metadata.Notebook
			}
			if title == "" {
				continue
			}
			var date *time.Time
			if !book.Metadata.Date.IsZero() {
				d := book.Metadata.Date
				date = &d
			}
			session, err := s.defsRepo.FindOrCreateSession(ctx, bookID, title, book.Metadata.Notebook, date)
			if err != nil {
				return fmt.Errorf("upsert session %q: %w", title, err)
			}
			if session.CreatedAt.Equal(session.UpdatedAt) {
				result.DefinitionsSessionsCreated++
			}
			for _, scene := range book.Scenes {
				idx := scene.Metadata.GetIndex()
				rec, err := s.defsRepo.FindOrCreateScene(ctx, session.ID, idx, scene.Metadata.Title)
				if err != nil {
					return fmt.Errorf("upsert scene idx=%d under %q: %w", idx, title, err)
				}
				if rec.CreatedAt.Equal(rec.UpdatedAt) {
					result.DefinitionsScenesCreated++
				}
			}
		}
	}
	return nil
}

func (s *StateSeeder) seedFlashcards(ctx context.Context, result *StateSeedResult) error {
	if s.reader == nil || s.flashcardRepo == nil {
		return nil
	}
	for nbID := range s.reader.GetFlashcardIndexes() {
		decks, err := s.reader.ReadFlashcardNotebooks(nbID)
		if err != nil {
			continue
		}
		for _, deck := range decks {
			if deck.Title == "" {
				continue
			}
			var date *time.Time
			if !deck.Date.IsZero() {
				d := deck.Date
				date = &d
			}
			rec, ferr := s.flashcardRepo.FindOrCreate(ctx, nbID, deck.Title, deck.Description, date)
			if ferr != nil {
				return fmt.Errorf("upsert flashcard deck %q in %q: %w", deck.Title, nbID, ferr)
			}
			if rec.CreatedAt.Equal(rec.UpdatedAt) {
				result.FlashcardDecksCreated++
			}
		}
	}
	return nil
}

// seedSkipFlagsAndEtymologyLogs walks the on-disk LearningHistory tree
// once. Each expression contributes:
//   - one note_skip_flags row per (quizType, timestamp) in SkippedAt,
//     or one origin_skip_flags row when the expression is an origin
//   - one learning_logs row per EtymologyBreakdownLogs / EtymologyAssemblyLogs
//     entry (these are the rows the old SaveEtymologyOriginResult wrote
//     to YAML; migration 016 made them DB-backed via origin_id)
func (s *StateSeeder) seedSkipFlagsAndEtymologyLogs(ctx context.Context, result *StateSeedResult) error {
	if s.learningSrc == nil {
		return nil
	}
	notes, err := s.noteRepo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("load notes: %w", err)
	}
	noteIDByExpr := make(map[noteExprKey]int64, len(notes))
	for _, n := range notes {
		for _, nn := range n.NotebookNotes {
			key := noteExprKey{
				notebookID: nn.NotebookID,
				expression: strings.ToLower(strings.TrimSpace(n.Entry)),
			}
			noteIDByExpr[key] = n.ID
			if n.Usage != n.Entry {
				key.expression = strings.ToLower(strings.TrimSpace(n.Usage))
				noteIDByExpr[key] = n.ID
			}
		}
	}

	var origins []notebook.EtymologyOriginRecord
	if s.originRepo != nil {
		origins, err = s.originRepo.FindAll(ctx)
		if err != nil {
			return fmt.Errorf("load etymology origins: %w", err)
		}
	}
	originIDByKey := make(map[etyKey]int64, len(origins))
	for _, o := range origins {
		originIDByKey[etyKey{
			notebookID:   o.NotebookID,
			sessionTitle: o.SessionTitle,
			origin:       strings.ToLower(strings.TrimSpace(o.Origin)),
		}] = o.ID
	}

	// LearningSource only supports per-notebook reads. The reader's
	// indexes give us the universe of notebook IDs to walk.
	seenIDs := make(map[string]bool)
	collect := func(ids ...string) {
		for _, id := range ids {
			if id != "" {
				seenIDs[id] = true
			}
		}
	}
	if s.reader != nil {
		for id := range s.reader.GetStoryIndexes() {
			collect(id)
		}
		for id := range s.reader.GetFlashcardIndexes() {
			collect(id)
		}
		for _, id := range s.reader.GetDefinitionsBookIDs() {
			collect(id)
		}
		for id := range s.reader.GetEtymologyIndexes() {
			collect(id)
		}
	}

	for nbID := range seenIDs {
		exprs, err := s.learningSrc.FindByNotebookID(nbID)
		if err != nil || len(exprs) == 0 {
			continue
		}
		for _, expr := range exprs {
			if err := s.persistSkipFlagsForExpression(ctx, nbID, expr, noteIDByExpr, originIDByKey, result); err != nil {
				return err
			}
			if err := s.persistEtymologyLogsForExpression(ctx, nbID, expr, originIDByKey, result); err != nil {
				return err
			}
		}
	}
	return nil
}

type noteExprKey struct {
	notebookID string
	expression string
}

type etyKey struct {
	notebookID   string
	sessionTitle string
	origin       string
}

func (s *StateSeeder) persistSkipFlagsForExpression(
	ctx context.Context,
	nbID string,
	expr notebook.LearningHistoryExpression,
	noteIDByExpr map[noteExprKey]int64,
	originIDByKey map[etyKey]int64,
	result *StateSeedResult,
) error {
	if len(expr.SkippedAt) == 0 {
		return nil
	}

	if expr.Type == notebook.LearningExpressionTypeOrigin {
		// Origin entries hung off etymology sessions — match against
		// every origin sharing the notebook + origin name. Without a
		// session title carried on the expression we widen the match.
		for k, id := range originIDByKey {
			if k.notebookID != nbID {
				continue
			}
			if k.origin != strings.ToLower(strings.TrimSpace(expr.Expression)) {
				continue
			}
			for quizType, ts := range expr.SkippedAt {
				at := parseSkippedTimestamp(ts)
				if err := s.skipFlagRepo.SkipOrigin(ctx, id, quizType, at); err != nil {
					return fmt.Errorf("seed origin skip flag: %w", err)
				}
				result.OriginSkipFlagsCreated++
			}
		}
		return nil
	}

	noteID := noteIDByExpr[noteExprKey{nbID, strings.ToLower(strings.TrimSpace(expr.Expression))}]
	if noteID == 0 {
		return nil
	}
	for quizType, ts := range expr.SkippedAt {
		at := parseSkippedTimestamp(ts)
		if err := s.skipFlagRepo.SkipNote(ctx, noteID, quizType, at); err != nil {
			return fmt.Errorf("seed note skip flag: %w", err)
		}
		result.NoteSkipFlagsCreated++
	}
	return nil
}

// persistEtymologyLogsForExpression writes every YAML log slot for an
// origin-typed expression against etymology_origins.id. Vocab
// expressions are handled by ImportLearningLogs; origin expressions
// are intentionally skipped there because attaching their logs to a
// note_id-keyed phantom note loses them on export (the note has no
// NotebookNotes to belong to, so the export's per-notebook YAML walk
// never visits it).
//
// All four slots — LearnedLogs, ReverseLogs, EtymologyBreakdownLogs,
// EtymologyAssemblyLogs — get the same treatment. Each record's
// quiz_type is preserved exactly, falling back to the per-slot default
// only when the record itself didn't specify one. That keeps round-
// trip lossless against YAML that parks freeform/etymology_freeform
// records in any slot.
func (s *StateSeeder) persistEtymologyLogsForExpression(
	ctx context.Context,
	nbID string,
	expr notebook.LearningHistoryExpression,
	originIDByKey map[etyKey]int64,
	result *StateSeedResult,
) error {
	if s.learningRepo == nil {
		return nil
	}
	if len(expr.LearnedLogs) == 0 && len(expr.ReverseLogs) == 0 &&
		len(expr.EtymologyBreakdownLogs) == 0 && len(expr.EtymologyAssemblyLogs) == 0 {
		return nil
	}
	// Match the origin by (notebookID, lower(origin)). Without a session
	// title in the expression we accept the first match — the YAML was
	// ambiguous anyway and the migrate command is one-shot.
	//
	// The expression doesn't need an explicit `type: origin` marker.
	// Some entries in the user's learning_notes (e.g. "ambi", "ascetic")
	// omit it but still refer to a Latin/Greek root that exists in
	// etymology_origins. Anything whose name resolves to an origin row
	// in the notebook gets routed here; ImportLearningLogs uses the
	// same predicate to skip those expressions on the note-side.
	var originID int64
	for k, id := range originIDByKey {
		if k.notebookID != nbID {
			continue
		}
		if k.origin != strings.ToLower(strings.TrimSpace(expr.Expression)) {
			continue
		}
		originID = id
		break
	}
	if originID == 0 {
		return nil
	}

	writeLogs := func(records []notebook.LearningRecord, defaultQuizType notebook.QuizType) error {
		for _, r := range records {
			quizType := string(r.QuizType)
			if quizType == "" {
				quizType = string(defaultQuizType)
			}
			log := &learning.LearningLog{
				OriginID:         originID,
				Status:           string(r.Status),
				LearnedAt:        r.LearnedAt.Time,
				Quality:          r.Quality,
				ResponseTimeMs:   int(r.ResponseTimeMs),
				QuizType:         quizType,
				IntervalDays:     r.IntervalDays,
				SourceNotebookID: nbID,
			}
			if err := s.learningRepo.Create(ctx, log); err != nil {
				return fmt.Errorf("insert etymology log: %w", err)
			}
			result.EtymologyLogsCreated++
		}
		return nil
	}

	if err := writeLogs(expr.LearnedLogs, notebook.QuizTypeNotebook); err != nil {
		return err
	}
	if err := writeLogs(expr.ReverseLogs, notebook.QuizTypeReverse); err != nil {
		return err
	}
	if err := writeLogs(expr.EtymologyBreakdownLogs, notebook.QuizTypeEtymologyStandard); err != nil {
		return err
	}
	if err := writeLogs(expr.EtymologyAssemblyLogs, notebook.QuizTypeEtymologyReverse); err != nil {
		return err
	}
	return nil
}

func parseSkippedTimestamp(raw string) time.Time {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t
	}
	return time.Now()
}
