// Package learning provides learning log storage and retrieval.
package learning

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/database"
)

// UpdateLogInput identifies a single learning log entry and the
// override to apply. Different implementations use different subsets:
// the YAML repository looks up by (NotebookName + StoryTitle/SceneTitle
// + Expression/OriginalExpression + QuizType + LearnedAt); the DB
// repository looks up by (NoteID + QuizType + LearnedAt). Pass
// everything you have — unused fields are ignored.
//
// MarkCorrect carries the user's intent for a fresh override. When
// MirrorValues is non-nil the repo skips its own
// markCorrect→status/quality derivation and writes exactly what's
// inside — used by MultiLearningRepository so the secondary store
// applies the same bytes the primary just wrote.
type UpdateLogInput struct {
	NoteID int64
	// ID is the stable source-entry identity of the target card. When set,
	// the YAML repository resolves the entry by id (falling back to
	// Expression for legacy id-less data).
	ID                 string
	NotebookName       string
	StoryTitle         string
	SceneTitle         string
	Expression         string
	OriginalExpression string
	// Sense routes etymology-origin overrides to the (origin, sense) series;
	// for etymology cards StoryTitle carries the session title. Empty otherwise.
	Sense        string
	QuizType     string
	LearnedAt    time.Time
	MarkCorrect  *bool
	MirrorValues *UpdateLogMirror
}

// UpdateLogMirror carries the already-computed new values when a
// secondary store should byte-match a primary's override.
type UpdateLogMirror struct {
	Status       string
	Quality      int
	IntervalDays int
}

// UpdateLogResult reports the pre-change values and the recomputed
// next-review date. Found is false when no row/entry matched the
// lookup keys; callers must treat that as a no-op (matches the
// rows-affected=0 semantics of SQL UPDATE).
type UpdateLogResult struct {
	OriginalQuality      int
	OriginalStatus       string
	OriginalIntervalDays int
	NewQuality           int
	NewStatus            string
	NewIntervalDays      int
	NewNextReviewDate    string
	Found                bool
}

// LearningRepository defines operations for managing learning logs.
type LearningRepository interface {
	FindAll(ctx context.Context) ([]LearningLog, error)
	BatchCreate(ctx context.Context, logs []*LearningLog) error
	Create(ctx context.Context, log *LearningLog) error
	// UpdateLog rewrites the log identified by in's lookup keys
	// according to in.MarkCorrect. Used by OverrideAnswer. The DB
	// implementation can fall back to mirroring values computed
	// upstream when in.NewStatus/NewQuality are pre-filled (used by
	// MultiLearningRepository so YAML and DB stay byte-identical).
	UpdateLog(ctx context.Context, in UpdateLogInput) (UpdateLogResult, error)
	// BatchDelete removes rows whose IDs appear in ids. Used by the
	// reconcile pass to drop DB-only logs that no longer exist in YAML.
	BatchDelete(ctx context.Context, ids []int64) error
}

// DBLearningRepository implements LearningRepository using PostgreSQL.
type DBLearningRepository struct {
	db *sqlx.DB
}

// NewDBLearningRepository creates a new DBLearningRepository.
func NewDBLearningRepository(db *sqlx.DB) *DBLearningRepository {
	return &DBLearningRepository{db: db}
}

// selectLearningLogColumns lists the columns explicitly because note_id
// and origin_id are both nullable since migration 017 — COALESCE both
// to zero so the int64 fields scan cleanly (a plain SELECT * would fail
// to scan a NULL into int64).
const selectLearningLogColumns = `SELECT id, COALESCE(note_id, 0) AS note_id, COALESCE(origin_id, 0) AS origin_id, COALESCE(correction_id, 0) AS correction_id, status, learned_at, quality, response_time_ms, quiz_type, interval_days, concept_key, easiness_factor, source_notebook_id, created_at, updated_at FROM learning_logs`

// FindAll returns all learning logs.
func (r *DBLearningRepository) FindAll(ctx context.Context) ([]LearningLog, error) {
	var logs []LearningLog
	if err := r.db.SelectContext(ctx, &logs, selectLearningLogColumns+" ORDER BY id"); err != nil {
		return nil, fmt.Errorf("load all learning logs: %w", err)
	}
	return logs, nil
}

// Create inserts a single learning log. It resolves the row's target
// (note / origin / grammar correction) before inserting:
//   - CorrectionID already set (the StateSeeder path) → insert as-is.
//   - a grammar attempt (quiz_type=grammar with a SenseID, no target yet) →
//     find-or-create the grammar_corrections row and key on correction_id.
//     Grammar corrections are not notes, so they must NOT fall through to
//     ensureNoteExists (which would spawn a phantom note that the reader
//     never reconstructs).
//   - a vocab attempt (NoteID 0 with an Expression) → find-or-create the note.
//   - an etymology-origin attempt already carries OriginID (Expression empty).
func (r *DBLearningRepository) Create(ctx context.Context, log *LearningLog) error {
	switch {
	case log.CorrectionID != 0:
		// Target already resolved (seeder pre-sets correction_id).
	case log.QuizType == grammarQuizType && log.SenseID != "":
		correctionID, err := r.ensureGrammarCorrection(ctx, log.SourceNotebookID, log.SenseID)
		if err != nil {
			return fmt.Errorf("ensure grammar correction exists: %w", err)
		}
		log.CorrectionID = correctionID
	case log.NoteID == 0 && log.OriginID == 0 && log.Expression != "":
		noteID, err := r.ensureNoteExists(ctx, log)
		if err != nil {
			return fmt.Errorf("ensure note exists: %w", err)
		}
		log.NoteID = noteID
	}

	// Fail fast at the runtime write boundary: reject a structurally invalid log
	// (no target, uncomputed success interval, bad status/quiz_type/quality, …)
	// before it reaches the table. Every runtime save path funnels through
	// Create, so this one guard covers them all — hence enforceComputedInterval.
	if err := validateLearningLog(log, true); err != nil {
		return err
	}

	// NULLIF turns a zero ID into SQL NULL so exactly one of
	// (note_id, origin_id, correction_id) is set: vocab logs carry note_id,
	// etymology origin logs carry origin_id, grammar logs carry correction_id.
	query := `INSERT INTO learning_logs (note_id, origin_id, correction_id, status, learned_at, quality, response_time_ms, quiz_type, interval_days, source_notebook_id, concept_key)
		VALUES (NULLIF($1, 0::bigint), NULLIF($2, 0::bigint), NULLIF($3, 0::bigint), $4, $5, $6, $7, $8, $9, $10, $11)`
	_, err := r.db.ExecContext(ctx, query,
		log.NoteID, log.OriginID, log.CorrectionID, log.Status, log.LearnedAt, log.Quality, log.ResponseTimeMs, log.QuizType, log.IntervalDays, log.SourceNotebookID, log.ConceptKey)
	if err != nil {
		return fmt.Errorf("insert learning log: %w", err)
	}
	return nil
}

// validateLearningLog is the single fail-fast guard at the learning-log write
// boundary (Create + BatchCreate — every runtime save and every import/seed
// funnels through one of them). It rejects a structurally invalid log BEFORE it
// reaches the table, so a save path that forgets to populate a field fails
// loudly at once instead of silently writing a bad row (the "interval_days
// stored 0" bug this fixes).
//
// Every rule below was verified against the owner's real DB (all 24,022 rows)
// to have ZERO false positives — each valid row satisfies it. The rules:
//
//   - exactly ONE of note_id / origin_id / correction_id is set (every real row
//     has exactly one; this tightens the older "at least one" check);
//   - status is a known learning status;
//   - quiz_type is a known quiz type;
//   - quality is in the SM-2 grade range 0..5 (real data is 1..5);
//   - learned_at is non-zero;
//   - interval_days is non-negative, and a SUCCESSFUL attempt
//     (understood / usable / intuitive) has interval_days > 0 — both calculators
//     always return >= 1 for a success (SM-2's correct floor / the fixed-level
//     ladder's intervals[0]), so a success with 0 can only mean the interval was
//     never computed. Exactly one real row violates this: the reported bug row.
//
// The (target ↔ quiz_type) pairing is deliberately NOT enforced: the real data
// legitimately carries etymology_origin logs on note_id and notebook/reverse/
// freeform logs on origin_id (168 rows), so any fixed mapping would reject valid
// history. easiness_factor is likewise not required: every real row has it NULL
// (a fixed-interval user), and EF is derived from the log chain at read time —
// it is not persisted on writes and its absence is not data loss.
//
// enforceComputedInterval gates ONLY the success-interval rule. It is true for
// runtime writes (Create), where the calculator provably yields >= 1 for a
// success so a 0 can only be an uncomputed bug. It is false for BatchCreate — a
// faithful copy of arbitrary historical data (import/seed), where a legacy
// success row could carry an omitted/0 interval that import must not reject.
// Every OTHER rule applies to both paths (all are satisfied by all 24,022 real
// rows, including the one interval-bug row).
func validateLearningLog(log *LearningLog, enforceComputedInterval bool) error {
	target := logTargetDescription(log)

	targets := 0
	for _, id := range []int64{log.NoteID, log.OriginID, log.CorrectionID} {
		if id != 0 {
			targets++
		}
	}
	if targets != 1 {
		return fmt.Errorf("learning log for %s: must target exactly one of note_id/origin_id/correction_id, got %d", target, targets)
	}
	if !knownLearningStatus(log.Status) {
		return fmt.Errorf("learning log for %s: unknown status %q", target, log.Status)
	}
	if !knownQuizType(log.QuizType) {
		return fmt.Errorf("learning log for %s: unknown quiz_type %q", target, log.QuizType)
	}
	if log.Quality < 0 || log.Quality > 5 {
		return fmt.Errorf("learning log for %s: quality=%d out of range 0..5", target, log.Quality)
	}
	if log.LearnedAt.IsZero() {
		return fmt.Errorf("learning log for %s: learned_at is zero", target)
	}
	if log.IntervalDays < 0 {
		return fmt.Errorf("learning log for %s: interval_days=%d is negative", target, log.IntervalDays)
	}
	if enforceComputedInterval && isSuccessStatus(log.Status) && log.IntervalDays <= 0 {
		return fmt.Errorf("learning log for %s: %s attempt has interval_days=%d (interval was not computed)", target, log.Status, log.IntervalDays)
	}
	return nil
}

// knownLearningStatuses is the set of valid learning-log status values (the
// notebook.LearnedStatus constants). "" is the legacy "learning" status; the
// learning package keeps these as local literals so it needn't depend on the
// notebook package (see grammarQuizType).
var knownLearningStatuses = map[string]bool{
	"":              true, // learning (legacy)
	"misunderstood": true,
	"understood":    true,
	"usable":        true,
	"intuitive":     true,
}

// knownQuizTypes is the set of valid quiz_type values (the notebook.QuizType
// constants). etymology_origin remains valid for historical logs even though no
// standalone etymology quiz writes it at runtime any more.
var knownQuizTypes = map[string]bool{
	"notebook":         true,
	"reverse":          true,
	"freeform":         true,
	"grammar":          true,
	"etymology_origin": true,
}

func knownLearningStatus(status string) bool { return knownLearningStatuses[status] }
func knownQuizType(quizType string) bool     { return knownQuizTypes[quizType] }

// isSuccessStatus reports whether a learning-log status marks a correct attempt
// (understood / usable / intuitive). These are the statuses whose interval the
// calculator guarantees to be >= 1; "misunderstood" and legacy "learning" ("")
// are not success and are exempt from the interval-positivity guard.
func isSuccessStatus(status string) bool {
	switch status {
	case "understood", "usable", "intuitive":
		return true
	default:
		return false
	}
}

// logTargetDescription names the log's resolved target for error messages.
func logTargetDescription(log *LearningLog) string {
	switch {
	case log.NoteID != 0:
		return fmt.Sprintf("note %d", log.NoteID)
	case log.OriginID != 0:
		return fmt.Sprintf("origin %d", log.OriginID)
	case log.CorrectionID != 0:
		return fmt.Sprintf("correction %d", log.CorrectionID)
	default:
		return fmt.Sprintf("expression %q", log.Expression)
	}
}

// grammarQuizType is the quiz_type string that routes a learning log to a
// grammar_corrections row instead of a note/origin. Kept as a local literal so
// the learning package needn't depend on notebook just for the constant.
const grammarQuizType = "grammar"

// ensureGrammarCorrection upserts the grammar_corrections row for
// (notebookID, senseID) and returns its id, so a runtime grammar attempt keys
// its log on correction_id. Mirrors ensureNoteExists' race-free upsert idiom.
func (r *DBLearningRepository) ensureGrammarCorrection(ctx context.Context, notebookID, senseID string) (int64, error) {
	var correctionID int64
	if err := r.db.GetContext(ctx, &correctionID, `
		INSERT INTO grammar_corrections (notebook_id, sense_id) VALUES ($1, $2)
		ON CONFLICT (notebook_id, sense_id) DO UPDATE SET sense_id = EXCLUDED.sense_id
		RETURNING id`, notebookID, senseID); err != nil {
		return 0, fmt.Errorf("insert grammar_correction: %w", err)
	}
	return correctionID, nil
}

// ensureNoteExists finds an existing note by usage/entry or creates one.
// Uses Definition as entry if set, otherwise Expression. Stores Expression as usage.
//
// Uses INSERT ... ON CONFLICT DO UPDATE ... RETURNING as a race-free
// upsert. The previous SELECT-then-INSERT pattern lost a race the user
// hit in production: a quiz answer whose note already existed in the
// DB slipped past the SELECT (either because SELECT scan failed
// silently, or two Create calls interleaved between one another's
// SELECT and INSERT) and the follow-up INSERT then violated the
// (usage, entry) unique constraint. ON CONFLICT DO UPDATE lets us
// atomically hand back the existing row's id — the no-op SET
// (setting a column to its own EXCLUDED value) is the standard
// Postgres idiom for "RETURNING the row whether it was inserted or
// already existed".
func (r *DBLearningRepository) ensureNoteExists(ctx context.Context, log *LearningLog) (int64, error) {
	entry := log.OriginalExpression
	if entry == "" {
		entry = log.Expression
	}
	usage := log.Expression

	// Id-bearing card: the note's identity is its sense_id, so find-or-create
	// keyed by sense_id (the partial unique index notes_sense_id_key). Two
	// distinct ids that share an (usage, entry) spelling stay two rows. The
	// no-op SET returns the existing row's id whether it was inserted or
	// already present — same idiom as the legacy branch below.
	if log.SenseID != "" {
		var noteID int64
		if err := r.db.GetContext(ctx, &noteID, `
			INSERT INTO notes ("usage", entry, meaning, sense_id) VALUES ($1, $2, $3, $4)
			ON CONFLICT (sense_id) WHERE sense_id <> '' DO UPDATE SET sense_id = EXCLUDED.sense_id
			RETURNING id`, usage, entry, "", log.SenseID); err != nil {
			return 0, fmt.Errorf("insert note by sense_id: %w", err)
		}
		return noteID, nil
	}

	// Legacy id-less card: upsert by (usage, entry). Since migration 022 the
	// (usage, entry) uniqueness is a PARTIAL index scoped to id-less rows
	// (WHERE sense_id = ''), so the conflict target must carry that same
	// predicate for Postgres to infer the index. The inserted row has the ''
	// sense_id default, so it falls under the partial index.
	var noteID int64
	if err := r.db.GetContext(ctx, &noteID, `
		INSERT INTO notes ("usage", entry, meaning) VALUES ($1, $2, $3)
		ON CONFLICT ("usage", entry) WHERE sense_id = '' DO UPDATE SET "usage" = EXCLUDED."usage"
		RETURNING id`, usage, entry, ""); err != nil {
		return 0, fmt.Errorf("insert note: %w", err)
	}
	return noteID, nil
}

// BatchCreate inserts multiple learning logs in a single transaction using multi-row INSERTs.
// Rows are chunked to stay under PostgreSQL's 65535 parameter limit.
func (r *DBLearningRepository) BatchCreate(ctx context.Context, logs []*LearningLog) error {
	if len(logs) == 0 {
		return nil
	}

	// Same fail-fast guard as Create, minus the success-interval rule (import/
	// seed faithfully copies historical data — see validateLearningLog): reject
	// a structurally invalid row before any of the batch is inserted, so a bad
	// import row fails loudly (and atomically) instead of silently landing.
	for i, l := range logs {
		if err := validateLearningLog(l, false); err != nil {
			return fmt.Errorf("learning log %d/%d: %w", i+1, len(logs), err)
		}
	}

	columns := []string{"note_id", "origin_id", "correction_id", "status", "learned_at", "quality", "response_time_ms", "quiz_type", "interval_days", "source_notebook_id", "concept_key"}
	const chunkSize = 5000 // 5000 * 11 columns = 55000 placeholders, under 65535

	// Multi-row VALUES can't use NULLIF per-cell, so overwrite a zero ID
	// with a nil interface so the driver passes SQL NULL. Exactly one of
	// note_id / origin_id / correction_id ends up set per row.
	nullableID := func(id int64) interface{} {
		if id == 0 {
			return nil
		}
		return id
	}

	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		for i := 0; i < len(logs); i += chunkSize {
			end := i + chunkSize
			if end > len(logs) {
				end = len(logs)
			}
			chunk := logs[i:end]

			query := database.BuildMultiRowInsert("learning_logs", columns, len(chunk))
			var args []interface{}
			for _, l := range chunk {
				args = append(args, nullableID(l.NoteID), nullableID(l.OriginID), nullableID(l.CorrectionID), l.Status, l.LearnedAt, l.Quality, l.ResponseTimeMs, l.QuizType, l.IntervalDays, l.SourceNotebookID, l.ConceptKey)
			}
			if _, err := tx.ExecContext(ctx, query, args...); err != nil {
				return fmt.Errorf("insert learning logs: %w", err)
			}
		}
		return nil
	})
}

// UpdateLog rewrites a single learning_logs row identified by
// (note_id, quiz_type, learned_at) according to in.MarkCorrect (or
// in.MirrorValues when set). Returns the pre-update values plus the
// recomputed next-review date so the handler can hand them back to the
// frontend's Undo workflow.
//
// Note-id resolution: the frontend can't reliably know the DB's
// notes.id (StartQuiz returns handler-assigned sequential card IDs,
// not DB IDs). So we resolve the DB note first from Expression /
// OriginalExpression, then look up the matching log by (note_id,
// quiz_type, learned_at). This mirrors what ensureNoteExists does on
// the write side and keeps the frontend contract "send whatever
// card.noteId you got back from Start*Quiz" honest.
//
// If either the note or the log can't be found the call returns
// Found=false with no error — MultiLearningRepository treats that as
// a soft no-op so the YAML write's success isn't unwound.
func (r *DBLearningRepository) UpdateLog(ctx context.Context, in UpdateLogInput) (UpdateLogResult, error) {
	if in.QuizType == "" || in.LearnedAt.IsZero() {
		return UpdateLogResult{}, nil
	}

	// Resolve DB note id from expression/originalExpression. Fall
	// back to in.NoteID only when neither lookup key is present, so
	// callers that already know the real DB id (rare, but keeps the
	// API forward-compatible) can still use it.
	noteID := int64(0)
	if in.ID != "" {
		// Id-bearing override: resolve the note by its stable sense_id so we
		// target the exact sense the card carried (falls through to a soft
		// no-op when no row has that id yet — e.g. a fresh import).
		if err := r.db.GetContext(ctx, &noteID,
			`SELECT id FROM notes WHERE sense_id = $1`, in.ID); err != nil {
			return UpdateLogResult{}, nil
		}
	} else if in.Expression != "" {
		entry := in.OriginalExpression
		if entry == "" {
			entry = in.Expression
		}
		if err := r.db.GetContext(ctx, &noteID,
			`SELECT id FROM notes WHERE "usage" = $1 AND entry = $2`,
			in.Expression, entry); err != nil {
			// No note matches — soft no-op.
			return UpdateLogResult{}, nil
		}
	} else if in.NoteID > 0 {
		noteID = in.NoteID
	} else {
		return UpdateLogResult{}, nil
	}

	// Find the row + read originals. DATE() lets the frontend pass a
	// YYYY-MM-DD without losing matches to clock skew between the
	// answer write and the override click.
	type currentRow struct {
		ID           int64     `db:"id"`
		Status       string    `db:"status"`
		Quality      int       `db:"quality"`
		IntervalDays int       `db:"interval_days"`
		LearnedAt    time.Time `db:"learned_at"`
	}
	var cur currentRow
	err := r.db.GetContext(ctx, &cur, `
		SELECT id, status, quality, interval_days, learned_at
		FROM learning_logs
		WHERE note_id = $1 AND quiz_type = $2 AND DATE(learned_at) = $3::date
		ORDER BY learned_at DESC LIMIT 1`,
		noteID, in.QuizType, in.LearnedAt.Format("2006-01-02"))
	if err != nil {
		// No row matches — treat as soft no-op so callers in
		// MultiLearningRepository don't fail the whole override when
		// the YAML side succeeded but the DB doesn't have the log yet
		// (e.g. fresh import that hasn't replayed the latest quiz).
		return UpdateLogResult{}, nil
	}

	newStatus, newQuality, newIntervalDays := computeOverrideValues(in, cur.Status, cur.Quality, cur.IntervalDays)

	if _, err := r.db.ExecContext(ctx, `
		UPDATE learning_logs
		SET status = $1, quality = $2, interval_days = $3
		WHERE id = $4`,
		newStatus, newQuality, newIntervalDays, cur.ID); err != nil {
		return UpdateLogResult{}, fmt.Errorf("update learning_logs: %w", err)
	}

	return UpdateLogResult{
		OriginalQuality:      cur.Quality,
		OriginalStatus:       cur.Status,
		OriginalIntervalDays: cur.IntervalDays,
		NewQuality:           newQuality,
		NewStatus:            newStatus,
		NewIntervalDays:      newIntervalDays,
		NewNextReviewDate:    cur.LearnedAt.AddDate(0, 0, newIntervalDays).Format("2006-01-02"),
		Found:                true,
	}, nil
}

// computeOverrideValues derives the post-override status/quality/
// interval. If the caller provided MirrorValues (the YAML primary's
// already-computed values, in the Multi flow), those win — keeping
// YAML and DB byte-identical. Otherwise we apply the markCorrect
// shorthand directly: quality 1/misunderstood or 4/understood with
// interval reset to 1 on misunderstood, otherwise carry the prior
// interval. The DB doesn't have the calculator's full SM-2 chain at
// hand; for the rare DB-only deployment that's a small drift the
// next quiz answer will replay through the YAML calculator anyway.
func computeOverrideValues(in UpdateLogInput, curStatus string, curQuality, curInterval int) (status string, quality, intervalDays int) {
	if in.MirrorValues != nil {
		return in.MirrorValues.Status, in.MirrorValues.Quality, in.MirrorValues.IntervalDays
	}
	if in.MarkCorrect == nil {
		return curStatus, curQuality, curInterval
	}
	if *in.MarkCorrect {
		newStatus := "understood"
		if in.QuizType == "freeform" || in.QuizType == "etymology_freeform" {
			newStatus = "usable"
		}
		return newStatus, 4, max(curInterval, 1)
	}
	return "misunderstood", 1, 1
}

// BatchDelete removes the rows whose IDs are in the slice. Used by the
// importer's reconcile pass to drop DB-only logs whose YAML counterpart
// has disappeared.
func (r *DBLearningRepository) BatchDelete(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	const chunkSize = 5000
	return database.RunInTx(ctx, r.db, func(ctx context.Context, tx *sqlx.Tx) error {
		for i := 0; i < len(ids); i += chunkSize {
			end := i + chunkSize
			if end > len(ids) {
				end = len(ids)
			}
			chunk := ids[i:end]
			query, args, err := sqlx.In("DELETE FROM learning_logs WHERE id IN (?)", chunk)
			if err != nil {
				return fmt.Errorf("build delete query: %w", err)
			}
			if _, err := tx.ExecContext(ctx, tx.Rebind(query), args...); err != nil {
				return fmt.Errorf("delete learning logs: %w", err)
			}
		}
		return nil
	})
}
