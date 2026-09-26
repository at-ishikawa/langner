package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// ErrRefreshTokenNotFound is returned when no refresh row matches a hash.
var ErrRefreshTokenNotFound = errors.New("refresh token not found")

// CLIRefreshToken is one hashed refresh-token record.
type CLIRefreshToken struct {
	ID        int64        `db:"id"`
	UserID    int64        `db:"user_id"`
	TokenHash string       `db:"token_hash"`
	ExpiresAt time.Time    `db:"expires_at"`
	CreatedAt time.Time    `db:"created_at"`
	RevokedAt sql.NullTime `db:"revoked_at"`
}

// CLIRefreshTokenRepository persists hashed refresh tokens with rotation and
// family revocation, mirroring the UserRepository style.
type CLIRefreshTokenRepository struct {
	db *sqlx.DB
}

// NewCLIRefreshTokenRepository builds a CLIRefreshTokenRepository.
func NewCLIRefreshTokenRepository(db *sqlx.DB) *CLIRefreshTokenRepository {
	return &CLIRefreshTokenRepository{db: db}
}

// Create inserts a refresh-token row storing only its hash.
func (r *CLIRefreshTokenRepository) Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (*CLIRefreshToken, error) {
	var row CLIRefreshToken
	if err := r.db.GetContext(ctx, &row,
		`INSERT INTO cli_refresh_tokens (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)
		 RETURNING id, user_id, token_hash, expires_at, created_at, revoked_at`,
		userID, tokenHash, expiresAt); err != nil {
		return nil, fmt.Errorf("create refresh token: %w", err)
	}
	return &row, nil
}

// FindByHash loads a refresh-token row by its hash regardless of revoked/expiry
// state — the handler decides how to treat a revoked or expired match (a
// revoked match is the reuse-detection signal).
func (r *CLIRefreshTokenRepository) FindByHash(ctx context.Context, tokenHash string) (*CLIRefreshToken, error) {
	var row CLIRefreshToken
	err := r.db.GetContext(ctx, &row,
		`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at
		 FROM cli_refresh_tokens WHERE token_hash = $1`, tokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRefreshTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find refresh token: %w", err)
	}
	return &row, nil
}

// Revoke marks a single refresh row revoked (idempotent — a second revoke is a
// no-op). Used on rotation (old token) and on logout.
func (r *CLIRefreshTokenRepository) Revoke(ctx context.Context, id int64, at time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE cli_refresh_tokens SET revoked_at = $1 WHERE id = $2 AND revoked_at IS NULL`,
		at, id); err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

// RevokeByHash marks the row with the given hash revoked (logout path). It is
// idempotent and does not error when nothing matches (no signal whether the
// token existed).
func (r *CLIRefreshTokenRepository) RevokeByHash(ctx context.Context, tokenHash string, at time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE cli_refresh_tokens SET revoked_at = $1 WHERE token_hash = $2 AND revoked_at IS NULL`,
		at, tokenHash); err != nil {
		return fmt.Errorf("revoke refresh token by hash: %w", err)
	}
	return nil
}

// RevokeFamily revokes every still-live refresh token for a user — the
// compromise response when an already-rotated token is reused.
func (r *CLIRefreshTokenRepository) RevokeFamily(ctx context.Context, userID int64, at time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE cli_refresh_tokens SET revoked_at = $1 WHERE user_id = $2 AND revoked_at IS NULL`,
		at, userID); err != nil {
		return fmt.Errorf("revoke refresh token family: %w", err)
	}
	return nil
}
