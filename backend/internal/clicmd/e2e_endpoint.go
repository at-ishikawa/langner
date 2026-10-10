package clicmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/schemas"
)

// PrepareE2EDatabase brings a database to the deterministic e2e baseline WITHOUT
// importing notebook content: it applies the schema (idempotent), provisions the
// allowlist/admin accounts, and resets the learning STATE to the seeded fixtures
// (truncate e2eStateTables + reseed the history from the Go fixtures in
// seed_e2e.go — no filesystem read). It is the shared core of the preview e2e
// seed endpoint, where the deployed server has NO fixture files on disk.
//
// Notebook CONTENT (notebook_files) is deliberately NOT touched: it is a
// one-time baseline that survives a reset (it is not in e2eStateTables) and must
// be imported once against this database with `migrate seed-e2e`. If it is
// missing this returns a clear error rather than letting every scenario fail
// opaquely. Idempotent: safe to call on every PR run.
func PrepareE2EDatabase(ctx context.Context, cfg *config.Config, db *sqlx.DB) error {
	if err := database.Migrate(db, schemas.Migrations, "migrations"); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	if err := provisionAuth(ctx, cfg, db); err != nil {
		return fmt.Errorf("provision auth: %w", err)
	}
	ownerID, err := resolveSeedOwnerID(ctx, cfg, db)
	if err != nil {
		return fmt.Errorf("resolve seed owner: %w", err)
	}
	if _, err := db.ExecContext(ctx,
		"TRUNCATE "+strings.Join(e2eStateTables, ", ")+" RESTART IDENTITY CASCADE"); err != nil {
		return fmt.Errorf("truncate e2e state: %w", err)
	}
	if err := seedE2EHistory(ctx, db, ownerID); err != nil {
		return fmt.Errorf("seed e2e history: %w", err)
	}
	// The one-time content import is a prerequisite: without it there is nothing
	// to quiz, so fail with an actionable message instead of opaque e2e failures.
	var files int
	if err := db.GetContext(ctx, &files, "SELECT COUNT(*) FROM notebook_files"); err != nil {
		return fmt.Errorf("check notebook content: %w", err)
	}
	if files == 0 {
		return fmt.Errorf("notebook_files is empty — import e2e content ONCE against this database with `langner-admin migrate seed-e2e --config config.e2e.yml`")
	}
	return nil
}

// IssueTestToken mints a langner access JWT for email (upserting its user row via
// the same deterministic synthetic sub `auth provision` uses, so the token
// resolves to the account that owns the seeded history), signed with cfg's
// TOKEN_SIGNING_KEY. Shared by `auth issue-test-token` and the seed endpoint.
func IssueTestToken(ctx context.Context, cfg *config.Config, db *sqlx.DB, email string) (string, error) {
	if email == "" {
		return "", fmt.Errorf("email is required")
	}
	if !cfg.Auth.Enabled() {
		return "", fmt.Errorf("auth is not enabled (token_signing_key unset)")
	}
	users := auth.NewUserRepository(db)
	userID, err := ensureUser(ctx, users, email)
	if err != nil {
		return "", fmt.Errorf("upsert test user: %w", err)
	}
	tokens, err := auth.NewTokenSigner(auth.DecodeKey(cfg.Auth.TokenSigningKey))
	if err != nil {
		return "", fmt.Errorf("token signing key: %w", err)
	}
	value, err := tokens.Sign(userID, time.Now())
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return value, nil
}
