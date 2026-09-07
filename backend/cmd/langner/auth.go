package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/notebook"
)

func newAuthCommand() *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication utilities",
	}
	authCmd.AddCommand(newAuthIssueTestCookieCommand())
	authCmd.AddCommand(newAuthProvisionCommand())
	return authCmd
}

// ensureUser find-or-creates the account for email, returning its id. It looks
// up by the blind-index email hash first (so a user already created by a real
// Google sign-in or by issue-test-cookie is reused, not duplicated — the
// email_hash UNIQUE constraint would otherwise reject a second row), and only
// upserts a provisioned row (with a deterministic "provision|<email>" subject)
// when none exists yet. name is used only when creating.
func ensureUser(ctx context.Context, users *auth.UserRepository, email, name string) (int64, error) {
	if existing, err := users.FindByEmail(ctx, email); err == nil {
		return existing.ID, nil
	}
	u, err := users.Upsert(ctx, "provision|"+auth.NormalizeEmail(email), email, name)
	if err != nil {
		return 0, fmt.Errorf("provision user %q: %w", email, err)
	}
	return u.ID, nil
}

// provisionAuth upserts the configured allowlist + initial-admin accounts and
// backfills every pre-auth (user_id IS NULL) learning-history row — learning
// logs and both skip-flag tables — to the initial admin's id, so history
// imported/seeded before auth existed becomes the admin's own (auth Phase 2).
// It is a no-op (returns nil) when auth is disabled or no initial_admin_email is
// configured, so a DB-less / auth-less dev import is unaffected. Idempotent:
// re-running only backfills rows still NULL and reuses existing user rows.
func provisionAuth(ctx context.Context, cfg *config.Config, db *sqlx.DB) error {
	if !cfg.Auth.Enabled() || cfg.Auth.InitialAdminEmail == "" {
		return nil
	}
	enc, err := auth.NewEncryptor(auth.DecodeKey(cfg.Auth.CredentialEncryptionKey))
	if err != nil {
		return fmt.Errorf("credential encryption key: %w", err)
	}
	users := auth.NewUserRepository(db, enc)

	// Upsert every allowlisted account plus the initial admin, so a learner can
	// sign in and (below) so the admin id is resolvable for the backfill.
	emails := append([]string{}, cfg.Auth.AllowedEmails...)
	emails = append(emails, cfg.Auth.InitialAdminEmail)
	for _, email := range emails {
		if email == "" {
			continue
		}
		if _, err := ensureUser(ctx, users, email, email); err != nil {
			return err
		}
	}

	adminID, err := ensureUser(ctx, users, cfg.Auth.InitialAdminEmail, cfg.Auth.InitialAdminEmail)
	if err != nil {
		return err
	}

	// Backfill pre-auth rows to the admin. Each table is scoped to user_id IS
	// NULL so an already-attributed row (a real user's runtime attempt) is never
	// reassigned.
	for _, table := range []string{"learning_logs", "note_skip_flags", "origin_skip_flags"} {
		if _, err := db.ExecContext(ctx,
			fmt.Sprintf("UPDATE %s SET user_id = $1 WHERE user_id IS NULL", table), adminID); err != nil {
			return fmt.Errorf("backfill %s.user_id: %w", table, err)
		}
	}

	// Assign notebook ownership/visibility from the notebook_ownership config
	// block (auth Phase 3). A notebook not listed keeps no row and stays public.
	if err := provisionNotebookOwnership(ctx, cfg, users, notebook.NewNotebookACLRepository(db)); err != nil {
		return err
	}
	return nil
}

// provisionNotebookOwnership upserts a notebooks overlay row for each
// notebook_ownership config entry, resolving owner_email to a user id (an owner
// must be an allowlist/admin account so it was ensured above; find-or-create is
// used defensively). An empty owner_email leaves the notebook unowned (NULL
// owner) — meaningful only for a public notebook. Idempotent: re-running just
// re-upserts the same rows.
func provisionNotebookOwnership(ctx context.Context, cfg *config.Config, users *auth.UserRepository, acl *notebook.NotebookACLRepository) error {
	for _, entry := range cfg.NotebookOwnership {
		if entry.NotebookID == "" {
			return fmt.Errorf("notebook_ownership entry is missing notebook_id")
		}
		visibility := entry.Visibility
		if visibility == "" {
			visibility = notebook.VisibilityPublic
		}
		var ownerID *int64
		if entry.OwnerEmail != "" {
			id, err := ensureUser(ctx, users, entry.OwnerEmail, entry.OwnerEmail)
			if err != nil {
				return fmt.Errorf("resolve owner %q for notebook %q: %w", entry.OwnerEmail, entry.NotebookID, err)
			}
			ownerID = &id
		}
		if err := acl.UpsertOwnership(ctx, entry.NotebookID, ownerID, visibility); err != nil {
			return fmt.Errorf("assign ownership for notebook %q: %w", entry.NotebookID, err)
		}
	}
	return nil
}

// newAuthProvisionCommand provisions the allowlist/admin accounts and backfills
// pre-auth learning history to the initial admin. It runs standalone and is also
// invoked automatically by `migrate import-db` so the e2e seed provisions
// without a separate step.
func newAuthProvisionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "provision",
		Short: "Upsert allowlist/admin users and backfill pre-auth learning history to the admin",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			if !cfg.Auth.Enabled() {
				return fmt.Errorf("auth is not enabled in config (session_signing_key is unset)")
			}
			if cfg.Auth.InitialAdminEmail == "" {
				return fmt.Errorf("auth.initial_admin_email is required to provision")
			}
			if err := provisionAuth(cmd.Context(), cfg, db); err != nil {
				return err
			}
			fmt.Printf("Provisioned auth accounts and backfilled pre-auth history to %q\n", cfg.Auth.InitialAdminEmail)
			return nil
		},
	}
}

// newAuthIssueTestCookieCommand mints a signed session cookie for an
// allowlisted email and prints its value to stdout. It is a TEST/e2e helper:
// the e2e harness runs it after import-db to authenticate every spec without a
// real Google round-trip. It upserts the user row (so the cookie's user id
// resolves) using the config's credential encryption key, then signs the
// cookie with the config's session signing key.
func newAuthIssueTestCookieCommand() *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:   "issue-test-cookie",
		Short: "Print a signed session cookie value for an allowlisted email (test/e2e only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" {
				return fmt.Errorf("--email is required")
			}
			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			if !cfg.Auth.Enabled() {
				return fmt.Errorf("auth is not enabled in config (session_signing_key is unset)")
			}
			enc, err := auth.NewEncryptor(auth.DecodeKey(cfg.Auth.CredentialEncryptionKey))
			if err != nil {
				return fmt.Errorf("credential encryption key: %w", err)
			}
			users := auth.NewUserRepository(db, enc)
			// Reuse an already-provisioned row for this email (find-or-create by
			// blind index) so issuing a cookie after `auth provision`/import-db
			// doesn't collide with the existing row on the email_hash UNIQUE
			// constraint.
			userID, err := ensureUser(cmd.Context(), users, email, "E2E Test User")
			if err != nil {
				return fmt.Errorf("upsert test user: %w", err)
			}

			sessions, err := auth.NewSessionSigner(auth.DecodeKey(cfg.Auth.SessionSigningKey))
			if err != nil {
				return fmt.Errorf("session signing key: %w", err)
			}
			value, err := sessions.Sign(auth.Session{
				UserID:    userID,
				ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
			})
			if err != nil {
				return fmt.Errorf("sign session: %w", err)
			}
			fmt.Println(value)
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "allowlisted email to mint a cookie for")
	return cmd
}
