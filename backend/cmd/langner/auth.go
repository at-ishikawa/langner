package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/at-ishikawa/langner/internal/auth"
)

func newAuthCommand() *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication utilities",
	}
	authCmd.AddCommand(newAuthIssueTestCookieCommand())
	return authCmd
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
			user, err := users.Upsert(cmd.Context(), "e2e-test|"+auth.NormalizeEmail(email), email, "E2E Test User")
			if err != nil {
				return fmt.Errorf("upsert test user: %w", err)
			}

			sessions, err := auth.NewSessionSigner(auth.DecodeKey(cfg.Auth.SessionSigningKey))
			if err != nil {
				return fmt.Errorf("session signing key: %w", err)
			}
			value, err := sessions.Sign(auth.Session{
				UserID:    user.ID,
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
