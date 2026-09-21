package clicmd

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
	"github.com/at-ishikawa/langner/internal/notebook"
)

func NewAuthCommand() *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication utilities",
	}
	authCmd.AddCommand(newAuthIssueTestTokenCommand())
	authCmd.AddCommand(newAuthProvisionCommand())
	authCmd.AddCommand(newAuthBackfillCommand())
	return authCmd
}

// syntheticSub derives the deterministic, PII-free google_sub for an account
// created by tooling (auth provision / issue-test-token) rather than a real
// Google sign-in. provision and issue-test-token derive it identically, so a
// provisioned account and the e2e cookie for the same email resolve to the SAME
// user row — the pre-auth history provision backfills therefore belongs to the
// cookie'd user. The email only seeds a stable, opaque subject; nothing PII is
// stored (migration 024 keeps only google_sub + username).
//
// (An account created here does NOT unify with a later REAL Google sign-in for
// the same person — that yields a distinct row keyed by Google's own sub — since
// the no-PII schema stores no email to match on. This matters only to the e2e
// harness, which authenticates via issue-test-token, not a real round-trip.)
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

// resolveSeedOwnerID resolves the user id that owns imported/seeded learning
// history. Seeded learning_logs and skip flags are attributed to it AT INSERT
// (user_id is NOT NULL, migration 028) — the same initial-admin account
// provisionAuth used to backfill NULL rows to, only stamped up front instead.
//
// Attribution keys off whether an OWNER is configured (initial_admin_email), NOT
// on whether the server's session auth is running (Auth.Enabled() =
// TOKEN_SIGNING_KEY) — those are orthogonal. import-db attributes imported
// history to the designated owner even when the HTTP auth server is off, which
// is exactly how the seed/validate steps run it (import-db with only DB_PASSWORD
// set, no signing key). Returns (0, nil) only when NO owner is configured; the
// seeder/importer then errors if there is actually history to attribute (see
// StateSeeder.requireOwner / validateLearningLog), leaving a content-only import
// with no configured owner intact.
func resolveSeedOwnerID(ctx context.Context, cfg *config.Config, db *sqlx.DB) (int64, error) {
	if cfg.Auth.InitialAdminEmail == "" {
		return 0, nil
	}
	users := auth.NewUserRepository(db)
	return ensureUser(ctx, users, cfg.Auth.InitialAdminEmail)
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
			id, err := ensureUser(ctx, users, entry.OwnerEmail)
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
// issue-test-token / the e2e seed) rather than a real Google sign-in — their
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
				return fmt.Errorf("auth is not enabled in config (token_signing_key is unset)")
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
				return fmt.Errorf("auth is not enabled in config (token_signing_key is unset)")
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

// newAuthIssueTestTokenCommand mints a langner access JWT for an allowlisted
// email and prints it to stdout. It is a TEST/e2e helper: the e2e harness runs
// it after import-db to authenticate every spec without a real Google
// round-trip, seeding the SPA's in-memory access token the same way the app
// receives it (there is no session cookie anymore). It upserts the user row (so
// the token's `sub` resolves) then signs a 24h access token with the config's
// TOKEN_SIGNING_KEY.
func newAuthIssueTestTokenCommand() *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:   "issue-test-token",
		Short: "Print a langner access JWT for an allowlisted email (test/e2e only)",
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
				return fmt.Errorf("auth is not enabled in config (token_signing_key is unset)")
			}
			users := auth.NewUserRepository(db)
			// Reuse the SAME row `auth provision`/import-db created for this email
			// (find-or-create by the deterministic synthetic sub), so a token
			// issued after provisioning resolves to the account that owns the
			// backfilled pre-auth history. No email is stored (migration 024 keeps
			// only google_sub + username — no PII).
			userID, err := ensureUser(cmd.Context(), users, email)
			if err != nil {
				return fmt.Errorf("upsert test user: %w", err)
			}

			tokens, err := auth.NewTokenSigner(auth.DecodeKey(cfg.Auth.TokenSigningKey))
			if err != nil {
				return fmt.Errorf("token signing key: %w", err)
			}
			value, err := tokens.Sign(userID, time.Now())
			if err != nil {
				return fmt.Errorf("sign access token: %w", err)
			}
			fmt.Println(value)
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "allowlisted email to mint an access token for")
	return cmd
}
