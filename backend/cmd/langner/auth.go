package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/config"
)

func newAuthCommand() *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication utilities",
	}
	authCmd.AddCommand(newAuthIssueTestCookieCommand())
	authCmd.AddCommand(newAuthProvisionCommand())
	authCmd.AddCommand(newAuthBackfillCommand())
	return authCmd
}

// syntheticSub derives the deterministic, PII-free google_sub for an account
// created by tooling (auth provision / issue-test-cookie) rather than a real
// Google sign-in. provision and issue-test-cookie derive it identically, so a
// provisioned account and the e2e cookie for the same email resolve to the SAME
// user row — the pre-auth history provision backfills therefore belongs to the
// cookie'd user. The email only seeds a stable, opaque subject; nothing PII is
// stored (migration 024 keeps only google_sub + username).
//
// (An account created here does NOT unify with a later REAL Google sign-in for
// the same person — that yields a distinct row keyed by Google's own sub — since
// the no-PII schema stores no email to match on. This matters only to the e2e
// harness, which authenticates via issue-test-cookie, not a real round-trip.)
func syntheticSub(email string) string {
	return "e2e-test|" + auth.NormalizeEmail(email)
}

// ensureUser find-or-creates the tooling account for email and returns its id.
// Upsert is idempotent on google_sub, so re-running reuses the existing row
// (preserving its auto-generated username) instead of creating a duplicate. No
// email or name is stored (no PII).
func ensureUser(ctx context.Context, users *auth.UserRepository, email string) (int64, error) {
	u, err := users.Upsert(ctx, syntheticSub(email))
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
	users := auth.NewUserRepository(db)

	// Upsert every allowlisted account plus the initial admin, so a learner can
	// sign in and (below) so the admin id is resolvable for the backfill.
	emails := append([]string{}, cfg.Auth.AllowedEmails...)
	emails = append(emails, cfg.Auth.InitialAdminEmail)
	for _, email := range emails {
		if email == "" {
			continue
		}
		if _, err := ensureUser(ctx, users, email); err != nil {
			return err
		}
	}

	adminID, err := ensureUser(ctx, users, cfg.Auth.InitialAdminEmail)
	if err != nil {
		return err
	}
	if _, err := backfillOwner(ctx, db, adminID); err != nil {
		return err
	}
	return nil
}

// backfillOwner assigns every pre-auth (user_id IS NULL) row in the three
// per-user STATE tables — learning_logs and both skip-flag tables — to ownerID,
// and returns how many rows were reassigned. The `user_id IS NULL` guard means
// an already-attributed row (a real user's runtime attempt) is never touched, so
// it is safe to re-run. This is the one-time cutover step that gives a
// single-user app's accumulated history an owner once auth is switched on.
func backfillOwner(ctx context.Context, db *sqlx.DB, ownerID int64) (int64, error) {
	var total int64
	for _, table := range []string{"learning_logs", "note_skip_flags", "origin_skip_flags"} {
		res, err := db.ExecContext(ctx,
			fmt.Sprintf("UPDATE %s SET user_id = $1 WHERE user_id IS NULL", table), ownerID)
		if err != nil {
			return total, fmt.Errorf("backfill %s.user_id: %w", table, err)
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// newAuthBackfillCommand assigns all pre-auth history (user_id IS NULL) to one
// owner account. It is the flag-driven counterpart to the config-based
// provisioning that `migrate import-db` runs for the e2e seed: a human running
// the one-time cutover names the owner on the command line, so no persistent
// `initial_admin_email` config is required. `--owner-email` find-or-creates the
// account (works even when no one has signed in yet); `--user-id` targets an
// existing account directly. With neither flag it falls back to the config's
// initial_admin_email so the automated path keeps working.
func newAuthBackfillCommand() *cobra.Command {
	var ownerEmail string
	var userID int64
	cmd := &cobra.Command{
		Use:   "backfill",
		Short: "Assign all pre-auth learning history (user_id IS NULL) to one owner account",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cfg, db, err := openConfigAndDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			if !cfg.Auth.Enabled() {
				return fmt.Errorf("auth is not enabled in config (session_signing_key is unset)")
			}

			ownerID := userID
			switch {
			case userID != 0 && ownerEmail != "":
				return fmt.Errorf("pass only one of --user-id or --owner-email")
			case userID != 0:
				// Target an existing account; verify it exists so a typo doesn't
				// orphan every row under a non-existent id.
				var exists bool
				if err := db.GetContext(ctx, &exists, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, userID); err != nil {
					return fmt.Errorf("check user id %d: %w", userID, err)
				}
				if !exists {
					return fmt.Errorf("no user with id %d (sign in first, or use --owner-email to create the account)", userID)
				}
			case ownerEmail != "":
				ownerID, err = ensureUser(ctx, auth.NewUserRepository(db), ownerEmail)
				if err != nil {
					return err
				}
			case cfg.Auth.InitialAdminEmail != "":
				ownerID, err = ensureUser(ctx, auth.NewUserRepository(db), cfg.Auth.InitialAdminEmail)
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("provide --owner-email or --user-id (or set auth.initial_admin_email in config)")
			}

			n, err := backfillOwner(ctx, db, ownerID)
			if err != nil {
				return err
			}
			fmt.Printf("Backfilled %d pre-auth row(s) to user id %d.\n", n, ownerID)
			return nil
		},
	}
	cmd.Flags().StringVar(&ownerEmail, "owner-email", "", "email of the account to own all pre-auth history (find-or-created)")
	cmd.Flags().Int64Var(&userID, "user-id", 0, "id of an existing account to own all pre-auth history")
	return cmd
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
			users := auth.NewUserRepository(db)
			// Reuse the SAME row `auth provision`/import-db created for this email
			// (find-or-create by the deterministic synthetic sub), so a cookie
			// issued after provisioning resolves to the account that owns the
			// backfilled pre-auth history. No email is stored (migration 024 keeps
			// only google_sub + username — no PII).
			userID, err := ensureUser(cmd.Context(), users, email)
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
