// Package dbseed provides small, composable helpers for inserting fixture rows
// into a Postgres langner database — the shared "fixtures library" used by Go
// integration tests and by the e2e seed step, replacing ad-hoc INSERT SQL
// scattered across tests and the heavyweight import pipeline for test setup.
//
// Design rule: STATE fixtures only (users, notebook ownership, learning
// history). Notebook CONTENT must still be loaded through the real reader/
// importer (see .claude/rules/verify-data-features-with-example-notebooks.md) —
// a hand-built content fixture hides path-divergence bugs. Learning-history
// seeding here goes through the REAL write path (learning.DBLearningRepository.
// Create), which resolves/creates the target note exactly as production does, so
// a seeded log can never drift from a runtime one.
package dbseed

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/learning"
)

// SeedUser inserts a users row and returns its id. username defaults to
// google_sub when empty.
func SeedUser(ctx context.Context, db *sqlx.DB, googleSub, username string) (int64, error) {
	if username == "" {
		username = googleSub
	}
	var id int64
	if err := db.GetContext(ctx, &id,
		`INSERT INTO users (google_sub, username) VALUES ($1, $2) RETURNING id`, googleSub, username); err != nil {
		return 0, fmt.Errorf("seed user %q: %w", googleSub, err)
	}
	return id, nil
}

// SetNotebookOwner upserts a notebooks overlay row (ownership + visibility +
// source). ownerUserID nil stores NULL (unowned/public). source defaults to
// 'shipped'.
func SetNotebookOwner(ctx context.Context, db *sqlx.DB, notebookID string, ownerUserID *int64, visibility, source string) error {
	if source == "" {
		source = "shipped"
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO notebooks (notebook_id, owner_user_id, visibility, source) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (notebook_id) DO UPDATE SET
		   owner_user_id = EXCLUDED.owner_user_id,
		   visibility    = EXCLUDED.visibility,
		   source        = EXCLUDED.source`,
		notebookID, ownerUserID, visibility, source); err != nil {
		return fmt.Errorf("set owner for notebook %q: %w", notebookID, err)
	}
	return nil
}

// SeedEtymologyOrigin upserts an etymology_origins row and returns its id, so an
// etymology-origin learning log can target it. type/language/meaning default to
// generic non-empty values (analytics Day Detail only displays `origin`).
func SeedEtymologyOrigin(ctx context.Context, db *sqlx.DB, notebookID, origin, originType, language, meaning string) (int64, error) {
	if originType == "" {
		originType = "root"
	}
	if language == "" {
		language = "Latin"
	}
	if meaning == "" {
		meaning = origin
	}
	// The live unique key is (notebook_id, session_title, origin, language,
	// sense); session_title/sense default to '' and are irrelevant to what the
	// analytics Day Detail displays (the origin string), so seed them empty.
	var id int64
	if err := db.GetContext(ctx, &id,
		`INSERT INTO etymology_origins (notebook_id, session_title, origin, type, language, sense, meaning)
		 VALUES ($1, '', $2, $3, $4, '', $5)
		 ON CONFLICT (notebook_id, session_title, origin, language, sense) DO UPDATE SET meaning = EXCLUDED.meaning
		 RETURNING id`,
		notebookID, origin, originType, language, meaning); err != nil {
		return 0, fmt.Errorf("seed etymology origin %q/%q: %w", notebookID, origin, err)
	}
	return id, nil
}

// LearningLogSpec is a compact learning-history fixture: a single attempt for a
// user at a time, targeting exactly one of a note (Expression → real note write
// path), an etymology origin (OriginID from SeedEtymologyOrigin), or a grammar
// correction (SenseID with QuizType "grammar" → real ensureGrammarCorrection).
type LearningLogSpec struct {
	UserID     int64
	NotebookID string // source_notebook_id
	Expression string // note-targeted: resolved/created into a note by the real write path
	OriginID   int64  // etymology-origin-targeted
	// SenseID is dual-use, disambiguated by whether Expression is set:
	//   - vocab (Expression set): the note's content-derived stable id (a
	//     flashcard/definition `id:`); "" means an id-less note keyed by
	//     (usage, entry). Seeding it makes the seeded note the SAME row the
	//     runtime ensure-on-serve resolves (ON CONFLICT (sense_id)).
	//   - grammar (Expression empty, QuizType "grammar"): the correction sense_id.
	SenseID string
	QuizType   string // "" → "notebook"
	Status     string // "" → "misunderstood" (a wrong attempt — what analytics Day Detail surfaces)
	Quality    int    // 0–5
	Interval   int    // interval_days; 0 → 1
	LearnedAt  time.Time
	// NotebookType is the note-targeted membership kind ("flashcard",
	// "definition", "story"); "" → "flashcard". Ignored for origin/grammar
	// logs. See SeedLearningLog for why a membership row is required.
	NotebookType string
	// Group is the note-targeted membership's title (notebook_notes."group").
	// The history reconstruction keys a notebook's LearningHistory on it
	// (Metadata.Title), and the quiz loaders match content cards to that
	// history BY TITLE — so it MUST equal the on-disk notebook's display title
	// (e.g. a flashcard book's `title`), or the seeded logs won't attach to the
	// quiz's due/status counts. Ignored for origin/grammar logs.
	Group string
}

// SeedLearningLog writes one learning_logs row through the REAL write path
// (learning.DBLearningRepository.Create), which resolves/creates the target
// note from Expression — so a seeded attempt is indistinguishable from one a
// learner produced, with no hand-built-SQL divergence.
func SeedLearningLog(ctx context.Context, db *sqlx.DB, spec LearningLogSpec) error {
	quizType := spec.QuizType
	if quizType == "" {
		quizType = "notebook"
	}
	status := spec.Status
	if status == "" {
		status = "misunderstood"
	}
	interval := spec.Interval
	if interval == 0 {
		interval = 1
	}
	log := &learning.LearningLog{
		UserID:           spec.UserID,
		Expression:       spec.Expression,
		OriginID:         spec.OriginID,
		SenseID:          spec.SenseID,
		SourceNotebookID: spec.NotebookID,
		Status:           status,
		QuizType:         quizType,
		Quality:          spec.Quality,
		IntervalDays:     interval,
		LearnedAt:        spec.LearnedAt,
	}
	if err := learning.NewDBLearningRepository(db).Create(ctx, log); err != nil {
		return fmt.Errorf("seed learning log %q/%q: %w", spec.NotebookID, spec.Expression, err)
	}

	// A note-targeted attempt: the real write path resolved/created the note
	// but does NOT register its notebook membership — in production the note
	// already carries a notebook_notes link from the content import. In a
	// content-free seed the note is otherwise orphaned, and the DB history
	// reconstruction finds a notebook's vocab via notebook_notes
	// (DBNoteRepository.FindByNotebooks), so an unlinked note is invisible to
	// analytics Day Detail, the Learn page, and the due-count reads. Link it.
	// A note-targeted (vocab) attempt is exactly one with an Expression and no
	// origin — grammar carries no Expression (SenseID is its correction id) and
	// etymology carries an OriginID, so neither creates a note membership. A
	// vocab note MAY still carry a SenseID: the note's content-derived stable id
	// (e.g. a flashcard's `id:`). It must be seeded so the seeded note is the
	// SAME row the runtime ensure-on-serve resolves to (ON CONFLICT (sense_id)),
	// not a membership-less duplicate.
	if spec.Expression != "" && spec.OriginID == 0 {
		if err := ensureNotebookMembership(ctx, db, spec.NotebookID, spec.NotebookType, spec.Group, spec.SenseID, spec.Expression); err != nil {
			return err
		}
	}
	return nil
}

// ensureNotebookMembership upserts the notebook_notes link for a seeded vocab
// note so the notebook's history reconstruction can find it. notebookType
// defaults to "flashcard" (the flat, scene-less kind); group/subgroup are empty
// (flashcards have no scenes). The note is resolved by (usage, entry) exactly as
// the write path stored it (usage=entry=expression for a seeded id-less note).
func ensureNotebookMembership(ctx context.Context, db *sqlx.DB, notebookID, notebookType, group, senseID, expression string) error {
	if notebookType == "" {
		notebookType = "flashcard"
	}
	// Resolve the just-written note the SAME way ensureNoteExists keyed it: an
	// id-bearing card by its sense_id, an id-less card by (usage, entry).
	var noteID int64
	var err error
	if senseID != "" {
		err = db.GetContext(ctx, &noteID,
			`SELECT id FROM notes WHERE sense_id = $1 ORDER BY id LIMIT 1`, senseID)
	} else {
		err = db.GetContext(ctx, &noteID,
			`SELECT id FROM notes WHERE "usage" = $1 AND entry = $1 AND sense_id = '' ORDER BY id LIMIT 1`, expression)
	}
	if err != nil {
		return fmt.Errorf("resolve seeded note %q: %w", expression, err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO notebook_notes (note_id, notebook_type, notebook_id, "group", subgroup)
		 VALUES ($1, $2, $3, $4, '')
		 ON CONFLICT (note_id, notebook_type, notebook_id, "group", subgroup) DO NOTHING`,
		noteID, notebookType, notebookID, group); err != nil {
		return fmt.Errorf("link note %d to notebook %q: %w", noteID, notebookID, err)
	}
	return nil
}
