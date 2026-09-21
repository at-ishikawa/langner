package clicmd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/internal/notebook"
)

// resolveNotebookOwner validates userID as the notebook's owner and returns it,
// or nil when userID is 0 ("unowned"). It REJECTS a nonexistent id and a tooling
// account (google_sub 'e2e-test|...', minted by the e2e seed / provisioning), so
// a production notebook's ownership can never be assigned to a synthetic account
// nobody signs in as — the owner must be a real Google account. There is no
// email lookup by design: accounts store no email (no PII), so the owner is
// named by id (sign in with Google first, then `langner-admin auth backfill`
// with no flag lists the ids).
func resolveNotebookOwner(ctx context.Context, db *sqlx.DB, userID int64) (*int64, error) {
	if userID == 0 {
		return nil, nil
	}
	var sub string
	if err := db.GetContext(ctx, &sub, `SELECT google_sub FROM users WHERE id = $1`, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("no account with id %d — sign in with Google first, then run `langner-admin auth backfill` with no flag to list ids", userID)
		}
		return nil, fmt.Errorf("look up user %d: %w", userID, err)
	}
	if strings.HasPrefix(sub, toolingSubPrefix) {
		return nil, fmt.Errorf("id %d is a tooling account, not a Google sign-in — pass the id of your real account (run `langner-admin auth backfill` with no flag to find it)", userID)
	}
	return &userID, nil
}

// NewNotebooksSetOwnerCommand assigns a notebook's public/private visibility and,
// for a private notebook, its owner — a REAL account named by --user-id. Auth
// must be enabled (owners are user accounts in Postgres). A private notebook is
// visible only to its owner; a public notebook is visible to every logged-in
// user (owner optional). No --user-id leaves the notebook unowned (public only).
func NewNotebooksSetOwnerCommand() *cobra.Command {
	var notebookID, visibility string
	var userID int64
	cmd := &cobra.Command{
		Use:   "set-owner",
		Short: "Assign a notebook's public/private visibility and (for private) its owner",
		Long: `Assign a notebook's visibility, and for a private notebook its owner.

The owner is a REAL Google account named by --user-id (accounts store no email,
so there is no --owner-email): sign in with Google first so the account exists,
then run 'langner-admin auth backfill' with no flag to list account ids. A private
notebook is visible only to its owner; a public notebook is visible to every
logged-in user. Omitting --user-id leaves the notebook unowned (public only).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if notebookID == "" {
				return fmt.Errorf("--notebook-id is required")
			}
			if visibility != notebook.VisibilityPublic && visibility != notebook.VisibilityPrivate {
				return fmt.Errorf("--visibility must be %q or %q", notebook.VisibilityPublic, notebook.VisibilityPrivate)
			}
			if visibility == notebook.VisibilityPrivate && userID == 0 {
				return fmt.Errorf("--user-id is required for a private notebook (it is visible only to its owner); run `langner-admin auth backfill` with no flag to list account ids")
			}
			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			if !cfg.Auth.Enabled() {
				return fmt.Errorf("auth is not enabled in config (token_signing_key is unset)")
			}
			ctx := cmd.Context()

			ownerID, err := resolveNotebookOwner(ctx, db, userID)
			if err != nil {
				return err
			}
			if err := notebook.NewNotebookACLRepository(db).UpsertOwnership(ctx, notebookID, ownerID, visibility); err != nil {
				return err
			}
			owner := "none"
			if ownerID != nil {
				owner = fmt.Sprintf("user id %d", *ownerID)
			}
			fmt.Printf("Set notebook %q visibility=%s owner=%s\n", notebookID, visibility, owner)
			return nil
		},
	}
	cmd.Flags().StringVar(&notebookID, "notebook-id", "", "notebook id to assign")
	cmd.Flags().Int64Var(&userID, "user-id", 0, "id of the real (Google) account that owns the notebook (required for private)")
	cmd.Flags().StringVar(&visibility, "visibility", notebook.VisibilityPublic, "public or private")
	return cmd
}
