package notebook

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// Visibility values stored in the notebooks overlay table.
const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

// VisibilityPredicate reports whether a notebook is visible to the user it was
// built for. It is the single object every read path consults so "can this user
// see this notebook?" is answered the same way everywhere (see
// .claude/rules/verify-data-features-with-example-notebooks.md — one unfiltered
// read path leaks a private notebook).
type VisibilityPredicate func(notebookID string) bool

// NotebookVisibility resolves which notebooks a user may see. The quiz service
// and notebook handler hold this interface; it is nil in YAML-only / no-DB dev,
// where every notebook is visible.
type NotebookVisibility interface {
	// VisibleNotebookIDs returns a predicate reporting whether a notebook is
	// visible to userID. Public and unlisted (no ownership row) notebooks and
	// the user's own private notebooks pass; someone else's private notebook —
	// or a private notebook with no owner — is hidden.
	VisibleNotebookIDs(ctx context.Context, userID int64) (VisibilityPredicate, error)
}

// AllVisible is the predicate used when no ACL repository is installed
// (YAML-only dev, no database): every notebook is visible.
func AllVisible(string) bool { return true }

// FilterHistoriesByVisibility drops every notebook_id key the predicate hides,
// returning a new map. Applied centrally in the quiz service's loadHistories so
// every count/status/relearn read that keys off the history map inherits the
// filter (defense in depth — the listing loaders also gate enumeration).
func FilterHistoriesByVisibility(histories map[string][]LearningHistory, visible VisibilityPredicate) map[string][]LearningHistory {
	if visible == nil {
		return histories
	}
	out := make(map[string][]LearningHistory, len(histories))
	for notebookID, hs := range histories {
		if visible(notebookID) {
			out[notebookID] = hs
		}
	}
	return out
}

// NotebookACLRepository is the PostgreSQL-backed NotebookVisibility. It reads the
// notebooks overlay table (migration 027) — a notebook_id absent from that table
// is public/unowned (unlisted = public).
type NotebookACLRepository struct {
	db *sqlx.DB
}

// NewNotebookACLRepository constructs the repository.
func NewNotebookACLRepository(db *sqlx.DB) *NotebookACLRepository {
	return &NotebookACLRepository{db: db}
}

// VisibleNotebookIDs computes the HIDDEN set for userID — the private notebooks
// that are NOT theirs — in one query, then returns visible(id) = id NOT IN that
// set. Framing it as "hidden" rather than "visible" is what makes an unlisted
// notebook (no ownership row) fall through to visible for free: only private,
// non-owned rows can hide a notebook, so public rows, missing rows, and the
// user's own private rows all pass without being enumerated.
//
// userID 0 (an unauthenticated / test caller with no user) owns nothing, so it
// sees only public + unlisted notebooks; every private notebook is hidden. In a
// gated deployment the auth interceptor guarantees a real userID > 0, so this is
// only the safe default for the ungated dev/test path.
func (r *NotebookACLRepository) VisibleNotebookIDs(ctx context.Context, userID int64) (VisibilityPredicate, error) {
	var hiddenIDs []string
	if err := r.db.SelectContext(ctx, &hiddenIDs,
		`SELECT notebook_id FROM notebooks
		 WHERE visibility = 'private' AND (owner_user_id IS NULL OR owner_user_id <> $1)`,
		userID,
	); err != nil {
		return nil, fmt.Errorf("select hidden notebooks: %w", err)
	}
	hidden := make(map[string]struct{}, len(hiddenIDs))
	for _, id := range hiddenIDs {
		hidden[id] = struct{}{}
	}
	return func(notebookID string) bool {
		_, isHidden := hidden[notebookID]
		return !isHidden
	}, nil
}

// UpsertOwnership sets a notebook's owner and visibility, inserting the overlay
// row when absent and updating it otherwise. ownerUserID nil stores NULL (an
// unowned notebook — meaningful only for a public one; a private+NULL notebook
// is visible to nobody). Used by `langner notebooks set-owner` and by
// notebook_ownership provisioning at import time.
func (r *NotebookACLRepository) UpsertOwnership(ctx context.Context, notebookID string, ownerUserID *int64, visibility string) error {
	if visibility != VisibilityPublic && visibility != VisibilityPrivate {
		return fmt.Errorf("invalid visibility %q (want %q or %q)", visibility, VisibilityPublic, VisibilityPrivate)
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO notebooks (notebook_id, owner_user_id, visibility) VALUES ($1, $2, $3)
		 ON CONFLICT (notebook_id) DO UPDATE SET owner_user_id = EXCLUDED.owner_user_id, visibility = EXCLUDED.visibility`,
		notebookID, ownerUserID, visibility,
	); err != nil {
		return fmt.Errorf("upsert notebook ownership: %w", err)
	}
	return nil
}
