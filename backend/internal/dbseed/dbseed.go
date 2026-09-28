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

// LearningLogSpec is a compact learning-history fixture: a single attempt on a
// word in a notebook, for a given user, at a given time.
type LearningLogSpec struct {
	UserID     int64
	NotebookID string // source_notebook_id
	Expression string // resolved/created into a note by the real write path
	QuizType   string // "" → "notebook"
	Status     string // "" → "misunderstood" (a wrong attempt — what analytics Day Detail surfaces)
	Quality    int    // 0–5
	Interval   int    // interval_days; 0 → 1
	LearnedAt  time.Time
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
	return nil
}
