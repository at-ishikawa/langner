package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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

// toolingSubPrefix marks accounts minted by tooling (auth provision /
// issue-test-cookie / the e2e seed) rather than a real Google sign-in — their
// google_sub is syntheticSub(email) = "e2e-test|<email>". A real sign-in keys on
// Google's own opaque sub, so such rows never carry this prefix. claimHistory
// treats tooling-owned history as unowned so it can be reclaimed to a real
// account (there is no email stored to look an account up by).
const toolingSubPrefix = "e2e-test|"

// claimHistory assigns to ownerID every learning-history row that has no REAL
// owner yet — rows with user_id IS NULL (imported before auth) OR owned by a
// tooling account (google_sub 'e2e-test|...', e.g. from the e2e seed or an
// earlier mistaken run). It never touches a row owned by another real account,
// so it is safe on a live multi-user DB. Returns the number of rows moved.
func claimHistory(ctx context.Context, db *sqlx.DB, ownerID int64) (int64, error) {
	var total int64
	for _, table := range []string{"learning_logs", "note_skip_flags", "origin_skip_flags"} {
		res, err := db.ExecContext(ctx, fmt.Sprintf(
			`UPDATE %s SET user_id = $1
			 WHERE user_id IS NULL
			    OR user_id IN (SELECT id FROM users WHERE google_sub LIKE '%s%%')`,
			table, toolingSubPrefix), ownerID)
		if err != nil {
			return total, fmt.Errorf("claim %s.user_id: %w", table, err)
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// listAuthUsers prints the accounts so the owner can find their real id. Kept
// PII-free: it shows only the id, the auto-generated username, and whether the
// row is a real Google account or a tooling account.
func listAuthUsers(ctx context.Context, db *sqlx.DB) error {
	var users []struct {
		ID        int64  `db:"id"`
		GoogleSub string `db:"google_sub"`
		Username  string `db:"username"`
	}
	if err := db.SelectContext(ctx, &users, `SELECT id, google_sub, username FROM users ORDER BY id`); err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	if len(users) == 0 {
		fmt.Println("No accounts yet. Sign in with Google first, then re-run with --user-id <id>.")
		return nil
	}
	fmt.Println("Accounts — pass --user-id <id> of your REAL (google) account:")
	for _, u := range users {
		kind := "google"
		if strings.HasPrefix(u.GoogleSub, toolingSubPrefix) {
			kind = "tooling"
		}
		fmt.Printf("  id=%-4d %-8s username=%s\n", u.ID, kind, u.Username)
	}
	return nil
}

// newAuthBackfillCommand assigns unowned learning history to ONE real account —
// the one-time per-user cutover. "Unowned" is user_id IS NULL (imported before
// auth) plus history stuck on a tooling account. Because accounts store no email
// (no PII), it cannot look you up by address: sign in with Google FIRST so your
// account exists, then pass --user-id. Run with no flag to list accounts.
func newAuthBackfillCommand() *cobra.Command {
	var userID int64
	cmd := &cobra.Command{
		Use:   "backfill",
		Short: "Assign unowned learning history to your real account (the per-user cutover)",
		Long: `Assign learning history that has no real owner — rows with user_id NULL
(imported before auth) or stuck on a tooling account (google_sub 'e2e-test|...',
e.g. from the e2e seed or an earlier run) — to ONE real account.

Accounts store no email, so this cannot look you up by address. Sign in with
Google FIRST (creating your account), then:

  langner auth backfill --user-id <your id>

Run with no flag to list accounts and find your id. History already owned by a
different real account is never touched.`,
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

			if userID == 0 {
				return listAuthUsers(ctx, db)
			}

			var sub string
			if err := db.GetContext(ctx, &sub, `SELECT google_sub FROM users WHERE id = $1`, userID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return fmt.Errorf("no account with id %d — sign in with Google first, then run `auth backfill` with no flag to list ids", userID)
				}
				return fmt.Errorf("look up user %d: %w", userID, err)
			}
			if strings.HasPrefix(sub, toolingSubPrefix) {
				return fmt.Errorf("id %d is a tooling account, not a Google sign-in — pass the id of your real account (run `auth backfill` with no flag to find it)", userID)
			}

			n, err := claimHistory(ctx, db, userID)
			if err != nil {
				return err
			}
			fmt.Printf("Assigned %d row(s) of unowned history to user id %d.\n", n, userID)
			return nil
		},
	}
	cmd.Flags().Int64Var(&userID, "user-id", 0, "id of your real (Google) account to own all unowned history; omit to list accounts")
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
